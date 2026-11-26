package scene

import (
	"cmp"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/graphics"
)

// Filtered views store only positions in the immutable extracted item array.
type renderItemList2D struct {
	items   []renderItem2D
	indices []uint32
	count   int
}

func (items renderItemList2D) len() int { return items.count }
func (items renderItemList2D) at(index int) *renderItem2D {
	if items.indices != nil {
		index = int(items.indices[index])
	}
	return &items.items[index]
}
func (items renderItemList2D) slice(start, end int) renderItemList2D {
	if items.indices != nil {
		items.indices = items.indices[start:end]
	} else {
		items.items = items.items[start:end]
	}
	items.count = end - start
	return items
}

type renderSortKey2D struct {
	scene      uint64
	group      uint64
	sortIndex  int
	order      uint64
	depth      float32
	sequence   uint32
	localOrder uint32
	ui         bool
}

type renderBatch2D struct {
	start, end int
}

func compareRenderPhaseItem2D(a, b renderItem2D) int {
	if order := cmp.Compare(a.scene.Order, b.scene.Order); order != 0 {
		return order
	}
	if a.ui != b.ui {
		if a.ui {
			return 1
		}
		return -1
	}
	if order := cmp.Compare(a.group, b.group); order != 0 {
		return order
	}
	if order := cmp.Compare(a.sortIndex, b.sortIndex); order != 0 {
		return order
	}
	if a.depth < b.depth {
		return -1
	}
	if a.depth > b.depth {
		return 1
	}
	if order := cmp.Compare(a.order, b.order); order != 0 {
		return order
	}
	if order := cmp.Compare(a.localOrder, b.localOrder); order != 0 {
		return order
	}
	return cmp.Compare(a.sequence, b.sequence)
}

func sameRenderBatch2D(a, b *renderItem2D) bool {
	return a.scene.Order == b.scene.Order && a.group == b.group && a.ui == b.ui &&
		a.sortIndex == b.sortIndex && a.material == b.material
}

// drawRenderPhaseList2D groups consecutive items for renderers that draw batches.
func drawRenderPhaseList2D(c *graphics.Canvas2D, world *ecs.World, items renderItemList2D, views DrawViews2D, cull bool, batched func(*renderItem2D) bool, draw func(renderBatch2D, renderViewport2D, bool)) {
	var viewport renderViewport2D
	var previous *renderItem2D
	for start := 0; start < items.count; {
		item := items.at(start)
		if previous == nil || previous.ui != item.ui {
			if cull {
				viewport = renderViewport2DForSpace(c, world, item.ui)
				if item.ui {
					c.ResetProjectionMatrix()
					c.SetViewMatrix(mgl32.Ident4())
				} else if camera, projection, transform, ok := activeCamera2DComponents(world); ok {
					viewport := camera.ResolvedViewport()
					c.SetProjectionMatrix(projection.Matrix(viewport.Width, viewport.Height))
					c.SetViewMatrix(transform.Matrix.Inv())
				}
			} else {
				c.ResetProjectionMatrix()
				if item.ui {
					c.SetViewMatrix(views.UI)
				} else {
					c.SetViewMatrix(views.World)
				}
				viewport = renderViewport2D{clipFromLayer: c.ProjectionMatrix().Mul4(c.ViewMatrix()), active: true}
			}
		}
		begin := previous == nil || !sameRenderBatch2D(previous, item)
		batch := renderBatch2D{start: start, end: start + 1}
		if begin && batched != nil && batched(item) {
			for batch.end < items.count && sameRenderBatch2D(item, items.at(batch.end)) {
				batch.end++
			}
		}
		draw(batch, viewport, begin)
		previous = item
		start = batch.end
	}
}
