package html

import "github.com/diiyw/onix/internal/layout"

// The owner list uses box preorder, including each box's clip before its
// descendants. Full, structural and local membership rebuilds share this order.
func beginHTMLOwnerSpan(state *htmlRenderState, rendered *htmlBoxRendering) {
	rendered.OwnerStart = len(state.Children)
	state.OwnerSpansValid = state.OwnerSpansValid && !rendered.SharedRendering
	state.SyncStats.OwnerListBoxes++
}

func rebuildHTMLOwnerList(state *htmlRenderState) {
	state.Children = state.Children[:0]
	state.OwnerSpansValid = true
	appendHTMLBoxEntities(state, state.Boxes)
}

// Reordering equal-size subtrees leaves every outside span unchanged. Local
// emission invalidates these offsets if per-box entity counts change.
func reorderHTMLOwnerSpans(state *htmlRenderState) {
	defer state.BoxFlags.clear()
	reorderedAncestor := func(box *layout.Box, covered bool) bool {
		return covered || state.BoxRenderings[box.Node].OwnerReordered
	}
	for _, box := range state.LayoutCache.Reordered {
		state.BoxRenderings[box.Node].OwnerReordered = true
	}
	defer func() {
		for _, box := range state.LayoutCache.Reordered {
			state.BoxRenderings[box.Node].OwnerReordered = false
		}
	}()
	children := state.Children
	for _, box := range state.LayoutCache.Reordered {
		if state.BoxFlags.resolve(box.Parent, htmlBoxParent, reorderedAncestor) {
			continue
		}
		rendered := state.BoxRenderings[box.Node]
		start, end := rendered.OwnerStart, rendered.OwnerEnd
		// Restrict capacity to the old span so an unexpected size change cannot
		// overwrite an unrelated subtree; recover through a full membership rebuild.
		state.Children = children[:start:end]
		appendHTMLBoxEntities(state, box)
		if len(state.Children) != end {
			state.Children = children
			rebuildHTMLOwnerList(state)
			return
		}
	}
	state.Children = children
}
