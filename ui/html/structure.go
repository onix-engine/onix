package html

import (
	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/html/layout"
	"github.com/onix-engine/onix/html/style"
	"github.com/onix-engine/onix/scene"
)

// Structural updates keep the global topology/index pass but avoid re-emitting
// unchanged boxes. Shared ownership retains the full path until its
// dependencies can be bounded safely.

func prepareHTMLStructure(state *htmlRenderState, root *layout.Box) bool {
	if root != state.Boxes {
		return false
	}
	for _, change := range state.StyleChanges {
		if change.Tree && !change.Reorder && (state.boxForNode(change.Node) != nil || state.RenderNodes[change.Node] != nil) {
			return false
		}
	}
	if !prepareHTMLStructureBox(state, root) {
		return false
	}
	for _, box := range state.LayoutCache.Removed {
		rendered := state.BoxRenderings[box.Node]
		if rendered == nil {
			continue
		}
		// A still-attached node losing its box changes display ownership.
		if rendered.SharedRendering || box.Node.Within(state.Root) {
			return false
		}
		rendered.Retiring = true
		state.RemovedBoxes = append(state.RemovedBoxes, rendered)
	}
	return true
}

func prepareHTMLStructureBox(state *htmlRenderState, box *layout.Box) bool {
	rendered := state.BoxRenderings[box.Node]
	if rendered == nil {
		rendered = &htmlBoxRendering{Box: box, Parent: box.Parent, ZIndex: box.Style.ZIndex.Number, ZIndexSpecified: box.Style.ZIndex.Kind == style.ValueNumber, OverflowX: htmlOverflowModeFor(box.Style.OverflowX), OverflowY: htmlOverflowModeFor(box.Style.OverflowY), ExtentRect: box.BorderBox()}
		state.BoxRenderings[box.Node] = rendered
		// New boxes must emit even if their first layout produced no geometry.
		if !queueHTMLBox(state, box) {
			return false
		}
	} else if rendered.SharedRendering || rendered.Box != box {
		return false
	}
	// Refresh depth in the existing topology walk, including descendants whose
	// immediate parent stayed equal when their whole subtree moved.
	rendered.Depth = 0
	if box.Parent != nil {
		rendered.Depth = state.BoxRenderings[box.Parent.Node].Depth + 1
	}
	if rendered.Parent != box.Parent {
		if rendered.Parent == nil || !queueHTMLBox(state, box) {
			return false
		}
	}
	for _, child := range box.Children {
		if !prepareHTMLStructureBox(state, child) {
			return false
		}
	}
	return true
}

// Reserve scrollbar paint slots after the complete content subtree. Cached
// child endpoints keep nested scrolling from repeatedly walking descendants.
func updateHTMLScrollbarPaintOrder(world *ecs.World, state *htmlRenderState, rendered *htmlBoxRendering) bool {
	end := rendered.Stack + 4
	for _, child := range rendered.Box.Children {
		end = max(end, state.BoxRenderings[child.Node].PaintEnd)
	}
	return setHTMLScrollbarPaintEnd(world, state, rendered, end)
}

func setHTMLScrollbarPaintEnd(world *ecs.World, state *htmlRenderState, rendered *htmlBoxRendering, end int) bool {
	if htmlOverflowScrollsAxis(rendered.Box.Style.OverflowX) || htmlOverflowScrollsAxis(rendered.Box.Style.OverflowY) {
		end += 2
	}
	changed := end != rendered.PaintEnd
	rendered.PaintEnd = end
	if binding, ok := state.OverflowBindings[rendered.Box.Node]; ok {
		for _, entity := range binding.parts {
			part, _ := world.Get[ScrollbarPart](entity)
			next := scene.UIStackIndex(end - 1 + int(part.Kind))
			if current, ok := world.Get[scene.UIStackIndex](entity); ok && current != next {
				*world.GetMut[scene.UIStackIndex](entity) = next
				state.SyncStats.OrderUpdated++
			}
		}
	}
	return changed
}

func updateHTMLBoxOrder(world *ecs.World, state *htmlRenderState, rendered *htmlBoxRendering, next int) {
	if !rendered.Queued || !rendered.PaintDirty {
		delta := scene.UIStackIndex(next - rendered.Stack)
		orderChanged := rendered.InlineFocus || rendered.Order != rendered.Box.Node.Order
		if delta == 0 && !orderChanged {
			return
		}
		if rendered.PaintSlot != 0 {
			state.PaintList.SetOrder(rendered.PaintSlot, next, rendered.Box.Node.Order)
			state.PaintListDirty = true
		}
		for _, entity := range rendered.Entities {
			if delta != 0 {
				if stack := world.GetMut[scene.UIStackIndex](entity); stack != nil {
					*stack += delta
					state.SyncStats.OrderUpdated++
				}
			}
			if orderChanged {
				if focus, ok := world.Get[Focusable](entity); ok {
					order := htmlEntityNode(world, entity).Order
					if focus.Order != order {
						world.GetMut[Focusable](entity).Order = order
					}
				}
			}
		}
		if binding := rendered.Overflow; binding != nil && delta != 0 {
			if stack := world.GetMut[scene.UIStackIndex](binding.entity); stack != nil {
				*stack += delta
				state.SyncStats.OrderUpdated++
			}
		}
		rendered.Stack = next
		rendered.Order = rendered.Box.Node.Order
	}
}

// Coalesce nested z-index changes. A changed ancestor can alter every nested
// stacking context, including descendants whose style and geometry stayed equal.
func updateHTMLChangedStackOrders(world *ecs.World, state *htmlRenderState, stackIndex int) {
	for _, rendered := range state.RenderQueue {
		if !rendered.StackChanged {
			continue
		}
		if !htmlStackChangedAncestor(state, rendered.Box) {
			updateHTMLStackSubtree(world, state, rendered.Box, stackIndex, htmlStackContextFor(rendered.Box.Parent), false)
			for parent := rendered.Box.Parent; parent != nil; parent = parent.Parent {
				if !updateHTMLScrollbarPaintOrder(world, state, state.BoxRenderings[parent.Node]) {
					break
				}
			}
		}
	}
}

// Structural changes also rebuild owner spans in paint order during this walk.
func updateHTMLStackSubtree(world *ecs.World, state *htmlRenderState, box *layout.Box, stackIndex int, context htmlStackContext, rebuildOwners bool) int {
	rendered := state.BoxRenderings[box.Node]
	context = context.enter(box)
	updateHTMLBoxOrder(world, state, rendered, context.stack(box, stackIndex))
	if rebuildOwners {
		appendHTMLOwnerEntities(state, rendered)
	}
	end := rendered.Stack + 4
	for _, child := range box.Children {
		end = max(end, updateHTMLStackSubtree(world, state, child, stackIndex, context, rebuildOwners))
	}
	setHTMLScrollbarPaintEnd(world, state, rendered, end)
	if rebuildOwners {
		rendered.OwnerEnd = len(state.Children)
	}
	return rendered.PaintEnd
}

// Shared primitives can belong to more than one box; changing their order
// requires full ownership reconciliation before any entity is rewritten.
func htmlStackSubtreeIndependent(state *htmlRenderState, box *layout.Box) bool {
	rendered := state.BoxRenderings[box.Node]
	if rendered == nil || rendered.SharedRendering {
		return false
	}
	for _, child := range box.Children {
		if !htmlStackSubtreeIndependent(state, child) {
			return false
		}
	}
	return true
}

func htmlStackChangedAncestor(state *htmlRenderState, box *layout.Box) bool {
	for parent := box.Parent; parent != nil; parent = parent.Parent {
		if state.BoxRenderings[parent.Node].StackChanged {
			return true
		}
	}
	return false
}
