package html

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/internal/dom"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

// Identity is independent of document order. Slots identify fragments of one
// node and one purpose; inserting a sibling cannot renumber another node.
type htmlRenderRole uint8

const (
	htmlRoleSurface htmlRenderRole = iota
	htmlRoleShadow
	htmlRoleBackgroundLayer
	htmlRoleBorderTop
	htmlRoleBorderRight
	htmlRoleBorderBottom
	htmlRoleBorderLeft
	htmlRoleDecoration
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
	Node    *dom.Node
	Role    htmlRenderRole
	Variant string
	Context ecs.Entity
}

type htmlEntityBinding struct {
	Entity   ecs.Entity
	Optional uint16
}

type htmlEntityGroup struct {
	Box               *htmlBoxRendering
	TrackedGeneration uint64
	Bindings          []htmlEntityBinding
	Generation        uint64
	Used              int
}

type htmlNodeRendering struct {
	Box        *htmlBoxRendering
	Entities   []ecs.Entity
	Generation uint64
}

type htmlEntityWriter struct {
	world       *ecs.World
	state       *htmlRenderState
	node        *dom.Node
	role        htmlRenderRole
	group       *htmlEntityGroup
	slot        int
	entity      ecs.Entity
	bundle      ecs.Bundle
	optional    uint16
	oldOptional uint16
}

func beginHTMLEntity(world *ecs.World, owner ecs.Entity, node *dom.Node, role htmlRenderRole, variant string, context ecs.Entity, capacity int) htmlEntityWriter {
	state := ecs.GetMut[htmlRenderState](world, owner)
	writer := htmlEntityWriter{world: world, state: state, node: node, role: role}
	if state.EntityGroups == nil {
		state.EntityGroups = make(map[htmlRenderIdentity]*htmlEntityGroup)
	}
	key := htmlRenderIdentity{Node: node, Role: role, Variant: variant, Context: context}
	group := state.EntityGroups[key]
	if group == nil {
		group = &htmlEntityGroup{}
		state.EntityGroups[key] = group
	}
	if active := state.EmittingBox; active != nil {
		if role == htmlRoleControl || role == htmlRoleFocusable {
			active.OwnsContext = true
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
	if writer.entity == 0 {
		writer.bundle = ecs.NewBundle(capacity)
	} else {
		// These links are added after the primitive is emitted and may change when
		// its DOM ancestor becomes or ceases to be a control/editing host.
		ecs.Remove[FocusOwner](world, writer.entity)
		ecs.Remove[EditableText](world, writer.entity)
		ecs.Remove[ScrollContent](world, writer.entity)
	}
	return writer
}

func htmlOptionalBit[T any]() uint16 {
	switch any((*T)(nil)).(type) {
	case *scene.Sprite2D:
		return 1 << 0
	case *scene.SpriteLayers2D:
		return 1 << 1
	case *scene.ClipRect2D:
		return 1 << 2
	case *scene.InputTarget2D:
		return 1 << 3
	case *scene.InputBlocker2D:
		return 1 << 4
	case *Focusable:
		return 1 << 5
	case *Editable:
		return 1 << 6
	case *htmlEditableNodeRef:
		return 1 << 7
	case *ControlText:
		return 1 << 8
	case *htmlControlTextNode:
		return 1 << 9
	case *scene.UIScroll:
		return 1 << 10
	case *ScrollbarPart:
		return 1 << 11
	case *scene.ClipExempt2D:
		return 1 << 12
	}
	return 0
}

// The generic writer uses ordinary bundles only for new entities. Updating a
// retained primitive does not box every component in a freshly allocated bundle.
func putHTML[T any](writer *htmlEntityWriter, value T) {
	writer.optional |= htmlOptionalBit[T]()
	if writer.entity == 0 {
		ecs.Put(&writer.bundle, value)
		return
	}
	world, entity := writer.world, writer.entity
	switch next := any(value).(type) {
	case scene.InputState2D:
		if ecs.Has[scene.InputState2D](world, entity) {
			return
		}
	case scene.UIGlobalTransform:
		if ecs.Has[scene.UIGlobalTransform](world, entity) {
			return
		}
	case Editable:
		if old, ok := ecs.Get[Editable](world, entity); ok && old.Value == next.Value {
			next.Anchor, next.Focus = old.Anchor, old.Focus
			value = any(next).(T)
		}
	case scene.UIScroll:
		if old, ok := ecs.Get[scene.UIScroll](world, entity); ok {
			old.ViewportSize, old.ContentSize = next.ViewportSize, next.ContentSize
			old.Horizontal, old.Vertical = next.Horizontal, next.Vertical
			old.ConsumeWheel = next.ConsumeWheel
			old.DisableInput = next.DisableInput
			old.Position[0] = min(max(old.Position[0], -max(next.ContentSize[0]-next.ViewportSize[0], 0)), 0)
			old.Position[1] = min(max(old.Position[1], -max(next.ContentSize[1]-next.ViewportSize[1], 0)), 0)
			value = any(old).(T)
		}
	case htmlChild:
		if old, ok := ecs.Get[htmlChild](world, entity); ok {
			// The primitive is emitted at its authored layout position on every
			// reconciliation. External offsets and scroll parents are replayed by
			// applyHTMLNodeBindings after emission; only the CSS transform state
			// remains valid across the retained entity update.
			next.transformOffset = old.transformOffset
			value = any(next).(T)
		}
	case scene.Text2D:
		if old, ok := ecs.Get[scene.Text2D](world, entity); ok {
			// The authored fragment never carries WrapWidth, so keeping its
			// WrapMode would drop the wrap a widget computes for its own text.
			next.WrapMode, next.WrapWidth = old.WrapMode, old.WrapWidth
			old.Value, old.Font, old.FontSize, old.DPI = next.Value, next.Font, next.FontSize, next.DPI
			old.WrapMode, old.MaxWidth, old.LineHeight, old.Color = next.WrapMode, next.MaxWidth, next.LineHeight, next.Color
			old.Alpha, old.LCD = next.Alpha, next.LCD
			old.TabWidth = next.TabWidth
			value = any(old).(T)
		}
	}
	_ = ecs.Insert(world, entity, value)
}

func removeMissingHTML[T any](writer *htmlEntityWriter) {
	bit := htmlOptionalBit[T]()
	if writer.oldOptional&bit != 0 && writer.optional&bit == 0 {
		ecs.Remove[T](writer.world, writer.entity)
	}
}

func (writer *htmlEntityWriter) finish() ecs.Entity {
	fresh := writer.entity == 0
	if fresh {
		writer.entity = writer.world.Spawn(writer.bundle)
		writer.state.SyncStats.Spawned++
	} else {
		removeMissingHTML[scene.Sprite2D](writer)
		removeMissingHTML[scene.SpriteLayers2D](writer)
		removeMissingHTML[scene.ClipRect2D](writer)
		removeMissingHTML[scene.InputTarget2D](writer)
		removeMissingHTML[scene.InputBlocker2D](writer)
		removeMissingHTML[Focusable](writer)
		removeMissingHTML[Editable](writer)
		removeMissingHTML[htmlEditableNodeRef](writer)
		removeMissingHTML[ControlText](writer)
		removeMissingHTML[htmlControlTextNode](writer)
		removeMissingHTML[scene.UIScroll](writer)
		removeMissingHTML[ScrollbarPart](writer)
		removeMissingHTML[scene.ClipExempt2D](writer)
		writer.state.SyncStats.Reused++
	}
	binding := htmlEntityBinding{writer.entity, writer.optional}
	if writer.slot == len(writer.group.Bindings) {
		writer.group.Bindings = append(writer.group.Bindings, binding)
	} else {
		writer.group.Bindings[writer.slot] = binding
	}
	state := writer.state
	if sprite, ok := ecs.Get[scene.Sprite2D](writer.world, writer.entity); ok {
		state.Textures.bind(writer.entity, sprite.Texture)
	} else {
		state.Textures.bind(writer.entity, nil)
	}
	if state.RenderNodes == nil {
		state.RenderNodes = make(map[*dom.Node]*htmlNodeRendering)
	}
	rendered := state.RenderNodes[writer.node]
	if rendered == nil {
		rendered = &htmlNodeRendering{}
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
	if child := ecs.GetMut[htmlChild](writer.world, writer.entity); child != nil {
		child.transformOffset = mgl32.Vec2{}
		child.role = writer.role
	}
	markHTMLChildNode(writer.world, writer.entity, writer.node)
	return writer.entity
}

func htmlEntityNode(world *ecs.World, entity ecs.Entity) *dom.Node {
	child, _ := ecs.Get[htmlChild](world, entity)
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
	for node := range state.BoxRenderings {
		if state.BoxesByNode[node] == nil {
			delete(state.BoxRenderings, node)
			delete(state.Textures.rasters, node)
		}
	}
	for node := range state.Textures.rasters {
		if state.BoxesByNode[node] == nil {
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
		if world.Despawn(binding.Entity) {
			state.SyncStats.Despawned++
		}
	}
	clear(group.Bindings[used:])
	group.Bindings = group.Bindings[:used]
	if used == 0 {
		if state.LocalReconcile {
			if key.Role == htmlRoleControl {
				delete(state.ControlSnapshots, key.Node)
			}
			if key.Role == htmlRoleFocusable {
				removeHTMLFocusTarget(state, key.Node)
			}
		}
		delete(state.EntityGroups, key)
	}
	return used
}
