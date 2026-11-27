package html

import (
	"math/bits"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/graphics"
	"github.com/onix-engine/onix/html/element"
	"github.com/onix-engine/onix/scene"
	uinode "github.com/onix-engine/onix/scene/node"
	"github.com/onix-engine/onix/ui/internal/textfield"
)

// Identity is independent of document order. Slots identify fragments of one
// node and one purpose; inserting a sibling cannot renumber another node.
type htmlRenderRole uint8

const (
	htmlRoleSurface htmlRenderRole = iota
	htmlRoleShadow
	htmlRoleBackgroundLayer
	htmlRoleBorderList
	htmlRoleInlineBackgroundList
	htmlRoleDecoration
	htmlRoleInlineBackground
	htmlRoleText
	htmlRoleImage
	htmlRoleControl
	htmlRoleFocusable
	htmlRoleEvent
	htmlRoleCheckbox
	htmlRoleIndicator
	htmlRoleControlVisual
	htmlRoleEditableVisual
	htmlRoleOverflow
	htmlRoleCanvas
	htmlRoleScrollbarTrack
	htmlRoleScrollbarThumb
)

type htmlRenderIdentity struct {
	Node    *uinode.Node
	Variant string
	Context ecs.Entity
	Role    htmlRenderRole
}

type htmlEntityBinding struct {
	Entity   ecs.Entity
	Optional uint16
}

type htmlEntityGroup struct {
	Box               *htmlBoxRendering
	Bindings          []htmlEntityBinding
	TrackedGeneration uint64
	Generation        uint64
	Used              int
}

type htmlNodeRendering struct {
	Box        *htmlBoxRendering
	Entities   []ecs.Entity
	Generation uint64
}

type htmlPaintSpawners struct {
	surfaces    ecs.Spawner12[scene.UITransform, scene.UIGlobalTransform, scene.Visibility, scene.Anchor, scene.UIStackIndex, scene.Parent, scene.Sprite2D, scene.Size2D, htmlChild, htmlElementSurface, scene.InputTarget2D, scene.InputState2D]
	texts       ecs.Spawner11[scene.UITransform, scene.UIGlobalTransform, scene.Visibility, scene.Anchor, scene.UIStackIndex, scene.Parent, scene.Text2D, scene.Size2D, htmlChild, scene.InputTarget2D, scene.InputState2D]
	initialized bool
}

func htmlPaintSpawnersFor(world *ecs.World) *htmlPaintSpawners {
	spawners := world.FrameValue[htmlPaintSpawners, htmlPaintSpawners]()
	if !spawners.initialized {
		spawners.surfaces = world.NewSpawner12[scene.UITransform, scene.UIGlobalTransform, scene.Visibility, scene.Anchor, scene.UIStackIndex, scene.Parent, scene.Sprite2D, scene.Size2D, htmlChild, htmlElementSurface, scene.InputTarget2D, scene.InputState2D]()
		spawners.texts = world.NewSpawner11[scene.UITransform, scene.UIGlobalTransform, scene.Visibility, scene.Anchor, scene.UIStackIndex, scene.Parent, scene.Text2D, scene.Size2D, htmlChild, scene.InputTarget2D, scene.InputState2D]()
		spawners.initialized = true
	}
	return spawners
}

type htmlEntityWriter struct {
	world       *ecs.World
	state       *htmlRenderState
	node        *uinode.Node
	group       *htmlEntityGroup
	bundle      ecs.Bundle
	slot        int
	capacity    int
	entity      ecs.Entity
	optional    uint16
	oldOptional uint16
	role        htmlRenderRole
}

func beginHTMLEntity(world *ecs.World, owner ecs.Entity, node *uinode.Node, role htmlRenderRole, variant string, context ecs.Entity, capacity int) htmlEntityWriter {
	state := world.GetMut[htmlRenderState](owner)
	writer := htmlEntityWriter{world: world, state: state, node: node, role: role, capacity: capacity}
	if state.EntityGroups == nil {
		state.EntityGroups = make(map[htmlRenderIdentity]*htmlEntityGroup)
	}
	key := htmlRenderIdentity{Node: node, Role: role, Variant: variant, Context: context}
	group := state.EntityGroups[key]
	if group == nil {
		if n := len(state.PaintCullGroups); n != 0 {
			group = state.PaintCullGroups[n-1]
			state.PaintCullGroups[n-1] = nil
			state.PaintCullGroups = state.PaintCullGroups[:n-1]
		} else {
			group = &htmlEntityGroup{}
		}
		state.EntityGroups[key] = group
	}
	if active := state.EmittingBox; active != nil {
		if role == htmlRoleControl || role == htmlRoleFocusable {
			active.OwnsContext = true
			active.InlineFocus = active.InlineFocus || node != active.Box.Node
		}
		if group.TrackedGeneration == state.RenderGeneration && group.Box != nil && group.Box != active {
			group.Box.SharedRendering, active.SharedRendering = true, true
		}
		if (!state.LocalReconcile && group.TrackedGeneration != state.RenderGeneration) || group.Box != active {
			active.Groups = append(active.Groups, key)
		}
		group.Box, group.TrackedGeneration = active, state.RenderGeneration
	}
	if group.Generation != state.RenderGeneration {
		group.Generation = state.RenderGeneration
		group.Used = 0
	}
	slot := group.Used
	if !state.Reconciling {
		slot = 0
	} else {
		group.Used++
	}
	writer.group, writer.slot = group, slot
	if slot < len(group.Bindings) {
		binding := group.Bindings[slot]
		if world.IsAlive(binding.Entity) {
			writer.entity, writer.oldOptional = binding.Entity, binding.Optional
		}
	}
	if writer.entity != 0 {
		// These links are added after the primitive is emitted and may change when
		// its DOM ancestor becomes or ceases to be a control/editing host.
		world.Discard(writer.entity, ecs.Type[FocusOwner]())
		world.Discard(writer.entity, ecs.Type[EditableText]())
		// Routing below claims the existing component if this primitive still
		// scrolls. Discarding it here moves every retained row twice.
		if !state.Reconciling {
			// Event-only emission has no scroll routing pass.
			world.Discard(writer.entity, ecs.Type[ScrollContent]())
		} else if scroll := world.GetMut[ScrollContent](writer.entity); scroll != nil && scroll.Area != 0 {
			scroll.Area = 0
			state.pendingScroll = append(state.pendingScroll, writer.entity)
		}
	}
	return writer
}

func (state *htmlRenderState) finishScrollBindings(world *ecs.World) {
	for _, entity := range state.pendingScroll {
		if content := world.GetRef[ScrollContent](entity); content != nil && content.Area == 0 {
			world.Discard(entity, ecs.Type[ScrollContent]())
		}
	}
	state.pendingScroll = state.pendingScroll[:0]
}

// Each optional component owns one bit in a retained entity binding.
type htmlComponentSlot[T any] uint8

const (
	spriteSlot          htmlComponentSlot[scene.Sprite2D]                          = iota
	spriteLayersSlot    htmlComponentSlot[scene.SpriteLayers2D]                    = iota
	clipRectSlot        htmlComponentSlot[scene.ClipRect2D]                        = iota
	inputTargetSlot     htmlComponentSlot[scene.InputTarget2D]                     = iota
	inputBlockerSlot    htmlComponentSlot[scene.InputBlocker2D]                    = iota
	focusableSlot       htmlComponentSlot[Focusable]                               = iota
	editableSlot        htmlComponentSlot[Editable]                                = iota
	editableNodeSlot    htmlComponentSlot[htmlEditableNodeRef]                     = iota
	controlTextSlot     htmlComponentSlot[ControlText]                             = iota
	controlTextNodeSlot htmlComponentSlot[htmlControlTextNode]                     = iota
	scrollSlot          htmlComponentSlot[scene.UIScroll]                          = iota
	scrollbarSlot       htmlComponentSlot[ScrollbarPart]                           = iota
	clipExemptSlot      htmlComponentSlot[scene.ClipExempt2D]                      = iota
	materialShaderSlot  htmlComponentSlot[scene.Shader[graphics.Material2DShader]] = iota
	elementSurfaceSlot  htmlComponentSlot[htmlElementSurface]                      = iota
	opacitySurfaceSlot  htmlComponentSlot[htmlOpacitySurface]                      = iota
)

func (slot htmlComponentSlot[T]) mask() uint16 { return 1 << slot }

func (writer *htmlEntityWriter) putOptional[T any](slot htmlComponentSlot[T], value T) {
	writer.optional |= slot.mask()
	writer.putHTML(value)
}

// putTextField updates authored configuration while retaining native interaction
// state. Fields retired by a display change move back from the document here.
func (writer *htmlEntityWriter) putTextField(value, placeholder string, maxLength int, password, editable bool) {
	field, present := writer.world.Get[textfield.Field](writer.entity)
	if !present {
		field, present = writer.state.DetachedFields[writer.node]
		delete(writer.state.DetachedFields, writer.node)
	}
	dirty, _ := writer.node.States.Get(element.StateDirtyValue)
	if !present || (editable || !dirty) && field.Buffer == nil && field.Value != value {
		field.SetValue(value)
		field.CommittedValue = value
	}
	if !present {
		field.Caret = field.Text().Runes()
		field.Anchor = field.Caret
	}
	field.Placeholder, field.Password, field.MaxRunes = placeholder, password, maxLength
	writer.putHTML(field)
}

// The generic writer uses ordinary bundles only for new entities. Updating a
// retained primitive does not box every component in a freshly allocated bundle.
func (writer *htmlEntityWriter) putHTML[T any](value T) {
	if writer.entity == 0 {
		writer.spawnBundle().Put(value)
		return
	}
	_ = writer.world.Insert(writer.entity, value)
}

// Allocate only for the dynamic spawn path. Retained entities and typed
// paint spawners never need this bundle.
//
//go:noinline
func (writer *htmlEntityWriter) spawnBundle() *ecs.Bundle {
	if writer.capacity != 0 {
		writer.bundle = ecs.NewBundle(writer.capacity)
		writer.capacity = 0
	}
	return &writer.bundle
}

// The layout engine authors every primitive's local placement. Input and global
// transform systems own their live state after the primitive is first emitted.
func (writer *htmlEntityWriter) putPlacement(parent ecs.Entity, x, y float32, stack int, visible bool) {
	writer.putHTML(scene.UITransformFromXY(x, y))
	writer.ensureComponent[scene.UIGlobalTransform]()
	writer.putHTML(scene.Visibility{Visible: visible})
	writer.putHTML(scene.AnchorTopLeft)
	writer.putHTML(scene.UIStackIndex(stack))
	writer.putHTML(scene.Parent{Entity: parent})
}

func (writer *htmlEntityWriter) ensureComponent[T any]() {
	if writer.entity == 0 || !writer.world.Has[T](writer.entity) {
		var value T
		writer.putHTML(value)
	}
}

func (writer *htmlEntityWriter) putScroll(next scene.UIScroll) {
	if writer.entity != 0 {
		if old, ok := writer.world.Get[scene.UIScroll](writer.entity); ok {
			old.ViewportSize, old.ContentSize = next.ViewportSize, next.ContentSize
			old.Horizontal, old.Vertical = next.Horizontal, next.Vertical
			old.ScrollbarOverlay, old.ScrollbarSize = next.ScrollbarOverlay, next.ScrollbarSize
			old.ScrollbarMinThumbSize, old.ScrollbarThin = next.ScrollbarMinThumbSize, next.ScrollbarThin
			old.ConsumeWheel, old.DisableInput = next.ConsumeWheel, next.DisableInput
			old.Position[0] = min(max(old.Position[0], -max(next.ContentSize[0]-next.ViewportSize[0], 0)), 0)
			old.Position[1] = min(max(old.Position[1], -max(next.ContentSize[1]-next.ViewportSize[1], 0)), 0)
			next = old
		}
	}
	writer.putOptional(scrollSlot, next)
}

func (writer *htmlEntityWriter) putChild(next htmlChild) {
	if old := writer.world.GetRef[htmlChild](writer.entity); old != nil {
		// External offsets and scroll parents are replayed after emission.
		next.transformOffset = old.transformOffset
	}
	writer.putHTML(next)
}

func (writer *htmlEntityWriter) putText(next scene.Text2D) {
	if old, ok := writer.world.Get[scene.Text2D](writer.entity); ok {
		// Keep the widget's wrapping settings and the retained text caches.
		old.Value, old.Font, old.FontSize, old.DPI = next.Value, next.Font, next.FontSize, next.DPI
		old.MaxWidth, old.LineHeight, old.Color = next.MaxWidth, next.LineHeight, next.Color
		old.Alpha, old.LCD = next.Alpha, next.LCD
		old.TabWidth = next.TabWidth
		old.Prelayout = next.Prelayout
		next = old
	}
	writer.putHTML(next)
}

// Resolve the ECS type only when its component is removed.
var htmlOptionalTypes = [...]func() ecs.ComponentType{
	spriteSlot:          ecs.Type[scene.Sprite2D],
	spriteLayersSlot:    ecs.Type[scene.SpriteLayers2D],
	clipRectSlot:        ecs.Type[scene.ClipRect2D],
	inputTargetSlot:     ecs.Type[scene.InputTarget2D],
	inputBlockerSlot:    ecs.Type[scene.InputBlocker2D],
	focusableSlot:       ecs.Type[Focusable],
	editableSlot:        ecs.Type[Editable],
	editableNodeSlot:    ecs.Type[htmlEditableNodeRef],
	controlTextSlot:     ecs.Type[ControlText],
	controlTextNodeSlot: ecs.Type[htmlControlTextNode],
	scrollSlot:          ecs.Type[scene.UIScroll],
	scrollbarSlot:       ecs.Type[ScrollbarPart],
	clipExemptSlot:      ecs.Type[scene.ClipExempt2D],
	materialShaderSlot:  ecs.Type[scene.Shader[graphics.Material2DShader]],
	elementSurfaceSlot:  ecs.Type[htmlElementSurface],
	opacitySurfaceSlot:  ecs.Type[htmlOpacitySurface],
}

func (writer *htmlEntityWriter) removeMissingHTML() {
	for removed := writer.oldOptional &^ writer.optional; removed != 0; removed &= removed - 1 {
		component := htmlOptionalTypes[bits.TrailingZeros16(removed)]()
		writer.world.Discard(writer.entity, component)
	}
}

func (writer *htmlEntityWriter) finish() ecs.Entity {
	if writer.entity == 0 {
		return writer.finishSpawn(writer.world.Spawn(writer.bundle))
	}
	writer.removeMissingHTML()
	writer.state.SyncStats.Reused++
	return writer.bind(false)
}

func (writer *htmlEntityWriter) finishSpawn(entity ecs.Entity) ecs.Entity {
	writer.entity = entity
	writer.state.SyncStats.Spawned++
	return writer.bind(true)
}

func (writer *htmlEntityWriter) bind(fresh bool) ecs.Entity {
	binding := htmlEntityBinding{writer.entity, writer.optional}
	if writer.slot == len(writer.group.Bindings) {
		writer.group.Bindings = append(writer.group.Bindings, binding)
	} else {
		writer.group.Bindings[writer.slot] = binding
	}
	state := writer.state
	if sprite, ok := writer.world.Get[scene.Sprite2D](writer.entity); ok {
		state.Textures.bind(writer.entity, sprite.Texture)
	} else {
		state.Textures.bind(writer.entity, nil)
	}
	if state.RenderNodes == nil {
		state.RenderNodes = make(map[*uinode.Node]*htmlNodeRendering)
	}
	rendered := state.RenderNodes[writer.node]
	if rendered == nil {
		if n := len(state.PaintCullNodes); n != 0 {
			rendered = state.PaintCullNodes[n-1]
			state.PaintCullNodes[n-1] = nil
			state.PaintCullNodes = state.PaintCullNodes[:n-1]
		} else {
			rendered = &htmlNodeRendering{}
		}
		state.RenderNodes[writer.node] = rendered
	}
	if active := state.EmittingBox; active != nil {
		if rendered.Generation == state.RenderGeneration && rendered.Box != nil && rendered.Box != active {
			rendered.Box.SharedRendering, active.SharedRendering = true, true
		}
		rendered.Box = active
	}
	if rendered.Generation != state.RenderGeneration {
		rendered.Generation = state.RenderGeneration
		rendered.Entities = rendered.Entities[:0]
	}
	if state.Reconciling || fresh {
		rendered.Entities = append(rendered.Entities, writer.entity)
	}
	if child := writer.world.GetMut[htmlChild](writer.entity); child != nil {
		child.transformOffset = mgl32.Vec2{}
		child.role = writer.role
	}
	markHTMLChildNode(writer.world, writer.entity, writer.node)
	return writer.entity
}

func htmlEntityNode(world *ecs.World, entity ecs.Entity) *uinode.Node {
	child, _ := world.Get[htmlChild](entity)
	return child.Node
}

func finishHTMLReconcile(world *ecs.World, state *htmlRenderState) {
	for key, group := range state.EntityGroups {
		trimHTMLGroup(world, state, key, group)
	}
	for node, rendered := range state.RenderNodes {
		if rendered.Generation != state.RenderGeneration {
			delete(state.RenderNodes, node)
		}
	}
	for node, rendered := range state.BoxRenderings {
		if rendered.Generation != state.RenderGeneration {
			removeHTMLPaintListBox(state, rendered)
			delete(state.BoxRenderings, node)
			delete(state.Textures.rasters, node)
		}
	}
	for node := range state.Textures.rasters {
		if state.BoxRenderings[node] == nil {
			delete(state.Textures.rasters, node)
		}
	}
	state.Reconciling = false
	state.Textures.sweep()
}

// Retire slots not emitted this generation, for either a local box or full pass.
func trimHTMLGroup(world *ecs.World, state *htmlRenderState, key htmlRenderIdentity, group *htmlEntityGroup) int {
	used := group.Used
	if group.Generation != state.RenderGeneration {
		used = 0
	}
	for _, binding := range group.Bindings[used:] {
		state.Textures.bind(binding.Entity, nil)
		parkHTMLField(world, state, key, binding.Entity)
		if world.Despawn(binding.Entity) {
			state.SyncStats.Despawned++
		}
	}
	clear(group.Bindings[used:])
	group.Bindings = group.Bindings[:used]
	if used == 0 {
		if state.LocalReconcile {
			if key.Role == htmlRoleFocusable {
				removeHTMLFocusTarget(state, key.Node)
			}
		}
		delete(state.EntityGroups, key)
		if state.PaintCullSync && len(state.PaintCullGroups) < 128 && cap(group.Bindings) <= 8 {
			*group = htmlEntityGroup{Bindings: group.Bindings[:0]}
			state.PaintCullGroups = append(state.PaintCullGroups, group)
		}
	}
	return used
}

// Park the field only while its logical node is still part of this document.
func parkHTMLField(world *ecs.World, state *htmlRenderState, key htmlRenderIdentity, entity ecs.Entity) {
	if key.Role != htmlRoleControl && key.Role != htmlRoleFocusable {
		return
	}
	field, ok := world.Get[textfield.Field](entity)
	if !ok || !key.Node.Within(state.Root) {
		return
	}
	if key.Role == htmlRoleControl {
		kind, _ := key.Node.Attr("type")
		if kind != key.Variant || !isHTMLEditableControlNode(key.Node) {
			return
		}
	} else if !element.IsEditingHost(key.Node) {
		return
	}
	field.Focused, field.Selecting, field.Composition = false, false, ""
	if state.DetachedFields == nil {
		state.DetachedFields = make(map[*uinode.Node]textfield.Field)
	}
	state.DetachedFields[key.Node] = field
}
