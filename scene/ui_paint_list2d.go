package scene

import (
	"slices"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/graphics"
	onixtext "github.com/onix-engine/onix/text"
)

// UIStaticText2D describes a single prelaid-out run. The glyph positions are
// borrowed from the layout producer and must stay valid until the next update.
type UIStaticText2D struct {
	Font          *onixtext.Font
	Layout        TextPrelayout2D
	FontSize, DPI float64
	Color         graphics.ColorRGBA
	X, Y          float32
	LCD           bool
}

// UIBoxPaint2D holds the paint of one passive UI box in surface coordinates.
// The solid border uses the shape's border parameters.
type UIBoxPaint2D struct {
	Text                UIStaticText2D
	Stack               int
	Shape               Shape2D
	X, Y, Width, Height float32
	Order               uint32
}

type uiBoxPaintSlot2D struct {
	text    textPaint2D
	box     UIBoxPaint2D
	changed uiPaintChange2D
}

// UIPaintList2D stores paint on its surface entity. Slots are addressed by the
// layout producer and may be replaced without moving other boxes.
type UIPaintList2D struct {
	boxes          []uiBoxPaintSlot2D
	free           []uint32
	dirty          []uint32
	base, revision uint64
}

type uiPaintChange2D uint8

const (
	uiShapeChanged uiPaintChange2D = 1 << iota
	uiTextChanged
)

// Retained extractors that missed a consumed journal rebuild their own index.
func (list *UIPaintList2D) acknowledge() {
	for _, id := range list.dirty {
		list.boxes[id-1].changed = 0
	}
	list.dirty = list.dirty[:0]
	list.base = list.revision
}

func (list *UIPaintList2D) invalidateSlots() {
	list.revision++
	list.acknowledge()
}

// Capacity reports the capacity of the slot table, free list, and glyph runs.
func (list *UIPaintList2D) Capacity() (boxes, free, glyphs int) {
	for i := range list.boxes {
		glyphs += cap(list.boxes[i].text.glyphs)
	}
	return cap(list.boxes), cap(list.free), glyphs
}

func (list *UIPaintList2D) Len() int { return len(list.boxes) - len(list.free) }

// Boxes iterates live paint slots. The layout glyph slices are borrowed for the visit.
func (list *UIPaintList2D) Boxes(yield func(UIBoxPaint2D) bool) {
	for i := range list.boxes {
		box := &list.boxes[i].box
		if (box.Shape.Kind != 0 || box.Text.Font != nil) && !yield(*box) {
			return
		}
	}
}

func (list *UIPaintList2D) Reset() {
	list.invalidateSlots()
	clear(list.boxes)
	list.boxes, list.free = list.boxes[:0], list.free[:0]
}

// Compact reclaims slot storage after substantial shrinkage and reports the
// new IDs so the layout producer can keep its surviving slots addressable.
func (list *UIPaintList2D) Compact() []uint32 {
	live := list.Len()
	if cap(list.boxes) <= max(64, live*4) {
		return nil
	}
	if len(list.free) == 0 {
		list.boxes = slices.Clone(list.boxes)
		list.free = nil
		return nil
	}
	list.invalidateSlots()
	list.dirty = nil
	relocated := make([]uint32, len(list.boxes)+1)
	boxes := make([]uiBoxPaintSlot2D, 0, live)
	for i := range list.boxes {
		slot := &list.boxes[i]
		if slot.box.Shape.Kind != 0 || slot.box.Text.Font != nil {
			boxes = append(boxes, *slot)
			relocated[i+1] = uint32(len(boxes))
		}
	}
	list.boxes, list.free = boxes, nil
	return relocated
}

// SetBox updates a slot and returns its ID. Zero creates a slot.
func (list *UIPaintList2D) SetBox(id uint32, box UIBoxPaint2D) uint32 {
	created := id == 0
	if created {
		list.invalidateSlots()
		if n := len(list.free); n != 0 {
			id = list.free[n-1]
			list.free = list.free[:n-1]
		} else {
			list.boxes = append(list.boxes, uiBoxPaintSlot2D{})
			id = uint32(len(list.boxes))
		}
	}
	slot := &list.boxes[id-1]
	old, next := &slot.box.Text, &box.Text
	layoutChanged := old.Font != next.Font || old.FontSize != next.FontSize || old.DPI != next.DPI ||
		old.Layout.Revision != next.Layout.Revision || old.Layout.OriginX != next.Layout.OriginX || old.Layout.OriginY != next.Layout.OriginY
	if next.Font == nil {
		slot.text = textPaint2D{}
	} else if layoutChanged {
		slot.text.cache = nil
	}
	if !created {
		prev := &slot.box
		if prev.Stack != box.Stack || prev.Order != box.Order {
			list.invalidateSlots()
		} else {
			var changed uiPaintChange2D
			if prev.Shape != box.Shape || prev.X != box.X || prev.Y != box.Y || prev.Width != box.Width || prev.Height != box.Height {
				changed |= uiShapeChanged
			}
			if layoutChanged || old.Color != next.Color || old.LCD != next.LCD || old.X != next.X || old.Y != next.Y ||
				old.Layout.Width != next.Layout.Width || old.Layout.Height != next.Layout.Height {
				changed |= uiTextChanged
			}
			if changed != 0 {
				if slot.changed == 0 {
					list.dirty = append(list.dirty, id)
				}
				slot.changed |= changed
				list.revision++
			}
		}
	}
	slot.box = box
	return id
}

// SetOrder updates the stacking position and document order of a slot.
func (list *UIPaintList2D) SetOrder(id uint32, stack int, order uint32) {
	box := &list.boxes[id-1].box
	if box.Stack != stack || box.Order != order {
		list.invalidateSlots()
		box.Stack, box.Order = stack, order
	}
}

func (list *UIPaintList2D) RemoveBox(id uint32) {
	list.invalidateSlots()
	list.boxes[id-1] = uiBoxPaintSlot2D{}
	list.free = append(list.free, id)
}

func collectUIPaintLists2D(c *graphics.Canvas2D, world *ecs.World, snapshots *renderSnapshots2D, hierarchy *renderHierarchy2DCache, items []renderItem2D, cull bool) []renderItem2D {
	viewport := renderViewport2DForSpace(c, world, true)
	scale := textRasterScale(c)
	textRegistered := RenderWorldFor(world).collectors2D[renderCollectorText2D] != nil
	for entity, access := range world.Query[*UIPaintList2D]().All {
		list := access.Get()
		if visibility := world.GetRef[Visibility](entity); list == nil || visibility != nil && !visibility.Visible {
			continue
		}
		matrix, clip := mgl32.Ident4(), clipRect2D{}
		if transform := world.GetRef[UIGlobalTransform](entity); transform != nil && world.Has[UITransform](entity) {
			matrix, clip = transform.Matrix, transform.clip
		}
		info := renderEntityInfo2D(world, entity, true, 0, hierarchy)
		for i := range list.boxes {
			slot := &list.boxes[i]
			box := &slot.box
			if box.Shape.Kind != 0 {
				transform := matrix.Mul4(mgl32.Translate3D(box.X, box.Y, 0))
				if !cull || renderItemVisible2D(viewport, transform, AnchorTopLeft, box.Width, box.Height, true, clip, true) {
					sprite := snapshots.addSprite(Sprite2D{Shape: box.Shape, Width: box.Width, Height: box.Height, Alpha: 1})
					item := newSpriteRenderItem2D(uint32(len(items)), entity, sprite, nil, AnchorTopLeft, transform, clip, box.Width, box.Height, true, box.Stack)
					item.entity, item.layers, item.target, item.scene, item.group = entity, info.layers, info.target, info.scene, info.group
					item.localOrder, item.paintSlot = box.Order, uint32(i+1)
					items = append(items, item)
				}
			}
			if text := &box.Text; textRegistered && text.Font != nil && text.Color.A > 0 {
				transform := matrix.Mul4(mgl32.Translate3D(text.X, text.Y, 0))
				width, height := text.Layout.Width, text.Layout.Height
				if !cull || renderItemVisible2D(viewport, transform, AnchorTopLeft, width, height, true, clip, true) {
					items = slot.appendText(items, snapshots, entity, info, transform, clip, scale, uint32(i+1))
				}
			}
		}
	}
	return items
}

func (slot *uiBoxPaintSlot2D) appendText(items []renderItem2D, snapshots *renderSnapshots2D, entity ecs.Entity, info renderEntity2DInfo, transform mgl32.Mat4, clip clipRect2D, scale float64, slotID uint32) []renderItem2D {
	run := &slot.box.Text
	paint := &slot.text
	if paint.cache == nil || paint.rasterScale != scale {
		paint.cache = run.Font.GlyphCache(run.FontSize*scale, run.DPI)
		paint.rasterScale = scale
		previous := paint.glyphs
		paint.glyphs, _ = appendPrelayoutGlyphs(resetLayoutBuffer(previous, len(run.Layout.Glyphs)), nil, run.Layout, paint.cache, scale, "", 0)
		clearSnapshotTail(paint.glyphs, len(previous))
		paint.cache.FlushUploads()
	}
	paint.Alpha, paint.LCD = 1, run.LCD
	if !snapshots.borrow {
		snapshots.texts = append(snapshots.texts, *paint)
		paint = &snapshots.texts[len(snapshots.texts)-1]
	}
	draw := Text2D{cache: paint.cache, glyphs: paint.glyphs, Color: run.Color}
	start := len(items)
	items = draw.appendStyledTextRuns(items, renderOrder2D(entity), transform, clip, true, slot.box.Stack+2, paint)
	for i := start; i < len(items); i++ {
		item := &items[i]
		item.entity, item.layers, item.target, item.scene, item.group = entity, info.layers, info.target, info.scene, info.group
		item.width, item.height = run.Layout.Width, run.Layout.Height
		item.localOrder, item.paintSlot = slot.box.Order, slotID
	}
	return items
}

// Positions are one-based. Atlas runs of a slot have the same sort key and
// consecutive emission sequence, so its text occupies a contiguous range.
type uiPaintPositions2D struct {
	shape, text, runs uint32
}

type retainedPaintList2D struct {
	list     *UIPaintList2D
	slots    []uiPaintPositions2D
	revision uint64
}

func (r *retainedPaintList2D) reset(list *UIPaintList2D) {
	size := len(list.boxes)
	if cap(r.slots) > max(64, size*4) {
		r.slots = nil
	}
	r.slots = slices.Grow(r.slots[:0], size)[:size]
	clear(r.slots)
	r.list, r.revision = list, list.revision
	list.acknowledge()
}

func (r *retainedPaintList2D) record(item *renderItem2D, position int) {
	slot := &r.slots[item.paintSlot-1]
	if item.kind == renderItemShape2D {
		slot.shape = uint32(position + 1)
	} else {
		if slot.runs == 0 {
			slot.text = uint32(position + 1)
		}
		slot.runs++
	}
}

type paintListTextRunsScratch struct{}

// Patch content in the existing snapshots. A different number of outputs or
// a different paint order goes through ordinary collection and sorting.
func (cache *renderTree2D) patchPaintList(c *graphics.Canvas2D, world *ecs.World, entity ecs.Entity, node *retainedEntity2D) bool {
	index := node.paintList
	binding := world.GetRef[*UIPaintList2D](entity)
	if index == nil || binding == nil || *binding != index.list || index.revision != index.list.base {
		return false
	}
	list := index.list
	matrix, clip := mgl32.Ident4(), clipRect2D{}
	if transform := world.GetRef[UIGlobalTransform](entity); transform != nil && world.Has[UITransform](entity) {
		matrix, clip = transform.Matrix, transform.clip
	}
	info := renderEntityInfo2D(world, entity, true, 0, frameRenderHierarchy2DCachePtr(world))
	updated := 0
	for _, id := range list.dirty {
		slot, positions := &list.boxes[id-1], index.slots[id-1]
		box := &slot.box
		if slot.changed&uiShapeChanged != 0 {
			if (positions.shape != 0) != (box.Shape.Kind != 0) {
				return false
			}
			if positions.shape != 0 {
				position := int(positions.shape - 1)
				item := &cache.items[position]
				transform := matrix.Mul4(mgl32.Translate3D(box.X, box.Y, 0))
				if transform[14] != item.depth {
					return false
				}
				cache.recordItemDamage(position)
				sprite := item.snapshot[Sprite2D]()
				sprite.Shape, sprite.Width, sprite.Height = box.Shape, box.Width, box.Height
				item.width, item.height = box.Width, box.Height
				item.transform = transform
				*item.paint = preparePaintItem2D(item, c.Device())
				updated++
			}
		}
		if slot.changed&uiTextChanged != 0 {
			scratch := world.FrameScratch[paintListTextRunsScratch, renderItem2D]()
			run := &box.Text
			if run.Font != nil && run.Color.A > 0 && RenderWorldFor(world).collectors2D[renderCollectorText2D] != nil {
				transform := matrix.Mul4(mgl32.Translate3D(run.X, run.Y, 0))
				snapshots := renderSnapshots2D{borrow: true}
				scratch.Values = slot.appendText(scratch.Values, &snapshots, entity, info, transform, clip, textRasterScale(c), id)
			}
			matches := len(scratch.Values) == int(positions.runs)
			if matches {
				for i, next := range scratch.Values {
					position := int(positions.text-1) + i
					item := &cache.items[position]
					if next.depth != item.depth {
						matches = false
						break
					}
					cache.recordItemDamage(position)
					paint := item.snapshot[textPaint2D]()
					*paint = slot.text
					next.content, next.paint, next.part, next.sequence = paint, item.paint, item.part, item.sequence
					previousTexture := item.texture
					*item = next
					*item.paint = preparePaintItem2D(item, c.Device())
					cache.replaceTexture(position, previousTexture)
					updated++
				}
			}
			clear(scratch.Values)
			scratch.ReleaseUnused()
			if !matches {
				return false
			}
		}
	}
	index.revision = list.revision
	list.acknowledge()
	cache.stats.Extracted += updated
	return true
}
