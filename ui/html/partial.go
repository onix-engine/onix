package html

import (
	"cmp"
	"slices"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/internal/dom"
	"github.com/diiyw/onix/internal/layout"
	"github.com/diiyw/onix/internal/style"
	onixtext "github.com/diiyw/onix/text"
)

// A box owns only the primitives it emits, excluding its child boxes. This
// index lets local updates retire missing fragments without sweeping all groups.
type htmlBoxRendering struct {
	Box                  *layout.Box
	Parent               *layout.Box
	Stack                int
	PaintEnd             int
	ContextParent        *htmlBoxRendering
	OwnsContext          bool
	Control, FocusOwner  ecs.Entity
	ControlRect          layout.Rect
	Groups               []htmlRenderIdentity
	Entities             []ecs.Entity
	OwnerStart, OwnerEnd int
	OwnerReordered       bool
	ZIndex               style.Value
	Visibility           style.Value
	OverflowX, OverflowY style.Value
	Clip                 ecs.Entity
	Depth                int
	ExtentRect           layout.Rect
	Window               htmlPaintWindow
	OuterWindow          htmlPaintWindow
	CullRect             layout.Rect
	PaintBounds          layout.Rect
	EffectBounds         layout.Rect
	StyleGeneration      uint64
	CompositeRoot        bool
	CullChanged          bool
	ExtentDirty          bool
	SharedRendering      bool
	Composite            bool
	Retiring             bool
	Queued               bool
	ClipChanged          bool
	StackChanged         bool
	ClipMoved, Expanded  bool
}

func beginHTMLBoxRendering(state *htmlRenderState, box *layout.Box, inheritedControl, inheritedFocus ecs.Entity) *htmlBoxRendering {
	if state.BoxRenderings == nil {
		state.BoxRenderings = make(map[*dom.Node]*htmlBoxRendering)
	}
	rendered := state.BoxRenderings[box.Node]
	if rendered == nil {
		rendered = &htmlBoxRendering{}
		state.BoxRenderings[box.Node] = rendered
	}
	if !state.LocalReconcile {
		clear(rendered.Groups)
		rendered.Groups = rendered.Groups[:0]
		rendered.Box, rendered.Parent, rendered.ZIndex = box, box.Parent, box.Style.ZIndex
		rendered.OverflowX, rendered.OverflowY = box.Style.OverflowX, box.Style.OverflowY
		rendered.Clip, rendered.Depth = 0, 0
		rendered.ExtentRect, rendered.ExtentDirty = box.BorderBox(), false
		if box.Parent != nil {
			rendered.Depth = state.BoxRenderings[box.Parent.Node].Depth + 1
		}
		rendered.SharedRendering = false
	}
	rendered.Visibility = box.Style.Visibility
	rendered.OwnsContext = false
	rendered.ContextParent = nil
	if inheritedControl != 0 || inheritedFocus != 0 {
		rendered.ContextParent = state.BoxRenderings[box.Parent.Node]
	}
	state.EmittingBox = rendered
	state.SyncStats.ReconciledBoxes++
	return rendered
}

func queueHTMLBox(state *htmlRenderState, box *layout.Box) bool {
	rendered := state.BoxRenderings[box.Node]
	if rendered == nil || rendered.SharedRendering || rendered.Composite || rendered.Box != box {
		return false
	}
	if rendered.Queued {
		return true
	}
	previous, exists := state.OverflowBindings[box.Node]
	rect := box.PaddingBox()
	rendered.Queued = true
	rendered.StackChanged = rendered.ZIndex != box.Style.ZIndex
	// Mode and parent changes invalidate routes even when descendant geometry stays equal.
	rendered.ClipChanged = rendered.Parent != box.Parent || exists != htmlBoxNeedsOverflowClip(state, box) || rendered.OverflowX != box.Style.OverflowX || rendered.OverflowY != box.Style.OverflowY
	rendered.ClipMoved = exists && (previous.rect.X != rect.X || previous.rect.Y != rect.Y)
	state.RenderQueue = append(state.RenderQueue, rendered)
	if htmlOverflowClipsAxis(rendered.OverflowX) != htmlOverflowClipsAxis(box.Style.OverflowX) || htmlOverflowClipsAxis(rendered.OverflowY) != htmlOverflowClipsAxis(box.Style.OverflowY) {
		for parent := box.Parent; parent != nil; parent = parent.Parent {
			if !queueHTMLBox(state, parent) {
				return false
			}
			state.BoxRenderings[parent.Node].ExtentDirty = true
		}
	}
	return true
}

// Partial reconciliation updates painted boxes and stable clip/event contexts.
// Structural changes preflight retained ownership; unsupported topology and
// shared primitive ownership retain the full path.
func reconcileHTMLPartial(world *ecs.World, owner ecs.Entity, state *htmlRenderState, needsLayout bool, fontValue, boldFont *onixtext.Font, dpi float64, stackIndex int, passThrough, visible bool) bool {
	if !state.PartialValid || !state.VisibilityKnown || state.Visible != visible {
		return false
	}
	structure := false
	for _, kind := range state.Mutations {
		structure = structure || kind&htmlMutationStructure != 0
		if kind&(htmlMutationContent|htmlMutationAttribute) != 0 {
			return false
		}
	}
	// Actual topology changes require structural ownership preflight.
	// CSS order uses the existing box tree and the ordinary local queue.
	structure = structure || state.LayoutTopology
	defer func() {
		for index, removed := range state.RemovedBoxes {
			removed.Retiring = false
			state.RemovedBoxes[index] = nil
		}
		state.RemovedBoxes = state.RemovedBoxes[:0]
		for _, rendered := range state.RenderQueue {
			rendered.Queued, rendered.StackChanged = false, false
			rendered.ClipMoved, rendered.ClipChanged, rendered.Expanded = false, false, false
			rendered.ExtentDirty = false
		}
		clear(state.RenderQueue)
		state.RenderQueue = state.RenderQueue[:0]
	}()
	if structure && !prepareHTMLStructure(state, state.LayoutCache.Root) {
		return false
	}
	if needsLayout {
		for _, box := range state.LayoutCache.Changed {
			if !queueHTMLBox(state, box) {
				return false
			}
		}
	}
	for _, change := range state.StyleChanges {
		if box := htmlNearestBox(state, change.Node); box != nil && !queueHTMLBox(state, box) {
			state.NodeAncestors.clear()
			return false
		}
	}
	state.NodeAncestors.clear()
	clipChanged := false
	// Rebuild the owner of inherited control/focus routes before its users.
	// A focus target unions contributions from the whole owning context.
	for i := 0; i < len(state.RenderQueue); i++ {
		rendered := state.RenderQueue[i]
		if rendered.Visibility != rendered.Box.Style.Visibility && rendered.Parent != nil && htmlOverflowClipsStyle(rendered.Parent.Style) {
			if !queueHTMLBox(state, rendered.Parent) {
				return false
			}
		}
		clipChanged = clipChanged || rendered.ClipChanged
		if rendered.Parent != rendered.Box.Parent {
			// Refresh both former and new context owners before their users.
			if !queueHTMLRetainedContext(state, state.BoxRenderings[rendered.Parent.Node]) || !queueHTMLBox(state, rendered.Box.Parent) {
				return false
			}
		}
		if !structure && rendered.StackChanged && !htmlStackChangedAncestor(state, rendered.Box) && !htmlStackSubtreeIndependent(state, rendered.Box) {
			return false
		}
		if parent := rendered.ContextParent; parent != nil && !queueHTMLRetainedContext(state, parent) {
			return false
		}
		_, focusable := htmlFocusableFromNode(owner, rendered.Box.Node)
		if rendered.OwnsContext || isHTMLControlNode(rendered.Box.Node) || focusable || rendered.ClipMoved || rendered.ClipChanged || htmlPaintWindowChanged(state, rendered) {
			if !queueHTMLContextDescendants(state, rendered) {
				return false
			}
		}
	}
	// Context owners precede descendants; extents use the reverse order.
	slices.SortFunc(state.RenderQueue, func(a, b *htmlBoxRendering) int { return cmp.Compare(a.Depth, b.Depth) })
	// Keep extents current even before the first clip is inserted. A later
	// structural update can reuse an otherwise unchanged subtree's extent.
	if needsLayout || clipChanged {
		for i := len(state.RenderQueue) - 1; i >= 0; i-- {
			rendered := state.RenderQueue[i]
			if structure || rendered.ExtentDirty || rendered.ExtentRect != rendered.Box.BorderBox() {
				updateHTMLOverflowExtent(state, rendered)
			}
		}
	}
	for _, rendered := range state.RenderQueue {
		updateHTMLControlSnapshots(world, state, rendered.Entities)
	}
	state.RenderGeneration++
	state.Reconciling, state.LocalReconcile = true, true
	children := state.Children
	ownerMembershipChanged := false
	for _, rendered := range state.RenderQueue {
		delete(state.EventTargets, rendered.Box.Node)
		previousCount := len(rendered.Entities)
		state.Children = rendered.Entities[:0]
		var control, focusOwner ecs.Entity
		var controlRect layout.Rect
		if parent := rendered.Box.Parent; parent != nil {
			inherited := state.BoxRenderings[parent.Node]
			control, focusOwner, controlRect = inherited.Control, inherited.FocusOwner, inherited.ControlRect
		}
		htmlSpawnBoxes(world, owner, state, rendered.Box, fontValue, boldFont, dpi, stackIndex, passThrough, visible, control, focusOwner, controlRect)
		rendered.Entities = state.Children
		ownerMembershipChanged = ownerMembershipChanged || len(rendered.Entities) != previousCount
		if ownerMembershipChanged || rendered.SharedRendering {
			state.OwnerSpansValid = false
		}
		state.ControlStylesChanged = restoreHTMLControlEntities(world, state, rendered.Entities) || state.ControlStylesChanged
	}
	prepareHTMLNodeBindings(world, state)
	if clipChanged {
		clearHTMLChangedClipRoutes(state)
	}
	// Focus bounds must finish unioning before converting to clip coordinates.
	for _, rendered := range state.RenderQueue {
		if structure || clipChanged || len(state.OverflowBindings) != 0 {
			refreshHTMLBoxOverflow(world, owner, state, rendered, stackIndex, visible)
		}
		finishHTMLBoxReconcile(world, state, rendered)
		applyHTMLNodeBindings(world, state, rendered.Entities)
		if rendered.Clip != 0 {
			applyHTMLNodeBindings(world, state, []ecs.Entity{rendered.Clip})
		}
	}
	if structure {
		for _, removed := range state.RemovedBoxes {
			finishHTMLBoxReconcile(world, state, removed)
			delete(state.BoxRenderings, removed.Box.Node)
			delete(state.BoxesByNode, removed.Box.Node)
			delete(state.EventTargets, removed.Box.Node)
			delete(state.OverflowExtents, removed.Box)
			delete(state.OverflowBindings, removed.Box.Node)
			delete(state.PaintCullOffsets, removed.Box.Node)
			delete(state.OverflowEntities, removed.Clip)
		}
	}
	state.Children = children
	state.Reconciling, state.LocalReconcile = false, false
	state.EmittingBox = nil
	if structure {
		state.Children = state.Children[:0]
		state.OwnerSpansValid = true
		appendHTMLStructureEntities(world, state, state.LayoutCache.Root, stackIndex)
	} else {
		updateHTMLChangedStackOrders(world, state, stackIndex)
		for i := len(state.RenderQueue) - 1; i >= 0; i-- {
			for rendered := state.RenderQueue[i]; rendered != nil; rendered = state.BoxRenderings[rendered.Box.Parent.Node] {
				if !updateHTMLScrollbarPaintOrder(world, state, rendered) || rendered.Box.Parent == nil {
					break
				}
			}
		}
	}
	if !structure {
		if state.SyncStats.Spawned != 0 || state.SyncStats.Despawned != 0 || ownerMembershipChanged {
			rebuildHTMLOwnerList(state)
		} else if len(state.LayoutCache.Reordered) != 0 {
			if state.OwnerSpansValid {
				reorderHTMLOwnerSpans(state)
			} else {
				rebuildHTMLOwnerList(state)
			}
		}
	}
	state.SyncStats.PartialReconciles++
	state.Textures.sweep()
	return true
}

func queueHTMLContextDescendants(state *htmlRenderState, rendered *htmlBoxRendering) bool {
	if rendered.Expanded {
		return true
	}
	rendered.Expanded = true
	for _, child := range rendered.Box.Children {
		if !queueHTMLBox(state, child) || !queueHTMLContextDescendants(state, state.BoxRenderings[child.Node]) {
			return false
		}
	}
	return true
}

func finishHTMLBoxReconcile(world *ecs.World, state *htmlRenderState, rendered *htmlBoxRendering) {
	if rendered.Retiring {
		delete(state.Textures.rasters, rendered.Box.Node)
	}
	rendered.Parent = rendered.Box.Parent
	rendered.ZIndex = rendered.Box.Style.ZIndex
	rendered.OverflowX, rendered.OverflowY = rendered.Box.Style.OverflowX, rendered.Box.Style.OverflowY
	live := rendered.Groups[:0]
	for _, key := range rendered.Groups {
		group := state.EntityGroups[key]
		if group == nil || group.Box != rendered {
			continue
		}
		if trimHTMLGroup(world, state, key, group) != 0 {
			live = append(live, key)
		}
		if node := state.RenderNodes[key.Node]; node != nil && node.Generation != state.RenderGeneration {
			delete(state.RenderNodes, key.Node)
		}
	}
	clear(rendered.Groups[len(live):])
	rendered.Groups = live
}

func appendHTMLBoxEntities(state *htmlRenderState, box *layout.Box) {
	if box == nil {
		return
	}
	rendered := state.BoxRenderings[box.Node]
	beginHTMLOwnerSpan(state, rendered)
	state.Children = append(state.Children, rendered.Entities...)
	if rendered.Clip != 0 {
		state.Children = append(state.Children, rendered.Clip)
	}
	if binding, ok := state.OverflowBindings[box.Node]; ok {
		state.Children = append(state.Children, binding.parts...)
	}
	for _, child := range box.Children {
		appendHTMLBoxEntities(state, child)
	}
	rendered.OwnerEnd = len(state.Children)
}

// Extents are recomputed only for changed boxes. Layout invalidation includes
// their ancestors, and the queue is processed deepest first.
func updateHTMLOverflowExtent(state *htmlRenderState, rendered *htmlBoxRendering) {
	box := rendered.Box
	r := box.PaddingBox()
	extent := htmlOverflowExtent{width: r.Width, height: r.Height}
	for _, child := range box.Children {
		childExtent := htmlOverflowChildExtent(child, state.OverflowExtents[child])
		childRect := child.BorderBox()
		extent.width = max(extent.width, childRect.X-r.X+childExtent.width)
		extent.height = max(extent.height, childRect.Y-r.Y+childExtent.height)
	}
	border := box.BorderBox()
	if box.Parent != nil && (state.OverflowExtents[box] != extent || rendered.ExtentRect != border) {
		state.BoxRenderings[box.Parent.Node].ExtentDirty = true
	}
	rendered.ExtentRect = border
	state.OverflowExtents[box] = extent
	state.SyncStats.ExtentBoxes++
}

func refreshHTMLBoxOverflow(world *ecs.World, owner ecs.Entity, state *htmlRenderState, rendered *htmlBoxRendering, stackIndex int, visible bool) {
	box := rendered.Box
	var parent htmlOverflowClipBinding
	for ancestor := box.Parent; ancestor != nil; ancestor = ancestor.Parent {
		if binding, exists := state.OverflowBindings[ancestor.Node]; exists {
			parent = binding
			break
		}
	}
	// Ancestor-owned focus routes are kept unless that clipping context changed.
	setHTMLOverflowRoute(state, box.Node, parent)
	current := parent
	if htmlBoxNeedsOverflowClip(state, box) {
		state.EmittingBox = rendered
		current = emitHTMLOverflowClip(world, owner, state, box, parent, stackIndex, visible)
		rendered.Clip = current.entity
		state.EmittingBox = nil
	} else if rendered.Clip != 0 {
		delete(state.OverflowBindings, box.Node)
		delete(state.OverflowEntities, rendered.Clip)
		rendered.Clip = 0
		// The old overflow group is retired by finishHTMLBoxReconcile.
	}
	if isHTMLControlNode(box.Node) {
		setHTMLOverflowRoute(state, box.Node, current)
	}
	for _, fragment := range box.TextFragments {
		setHTMLOverflowRoute(state, fragment.Node, current)
	}
	for _, fragment := range box.ImageFragments {
		setHTMLOverflowRoute(state, fragment.Node, current)
	}
	for _, fragment := range box.ControlFragments {
		setHTMLOverflowRoute(state, fragment.Node, current)
	}
	routeHTMLOverflowEntities(world, owner, state, rendered.Entities)
	if rendered.Clip != 0 {
		// The clip is stored separately from the box's paint entities. Re-emitting
		// it also clears ScrollContent, so restore its enclosing scroll route.
		routeHTMLOverflowEntities(world, owner, state, []ecs.Entity{rendered.Clip})
	}
}

func removeHTMLFocusTarget(state *htmlRenderState, node *dom.Node) {
	index, exists := state.FocusByNode[node]
	if !exists {
		return
	}
	last := len(state.FocusTargets) - 1
	if index != last {
		state.FocusTargets[index] = state.FocusTargets[last]
		state.FocusByNode[state.FocusTargets[index].Node] = index
	}
	state.FocusTargets[last] = htmlFocusTargetBinding{}
	state.FocusTargets = state.FocusTargets[:last]
	delete(state.FocusByNode, node)
}

func htmlBoxNeedsOverflowClip(state *htmlRenderState, box *layout.Box) bool {
	if !htmlOverflowClipsStyle(box.Style) || htmlNodeHasScrollBinding(state, box.Node) {
		return false
	}
	rect := box.PaddingBox()
	return rect.Width > 0 && rect.Height > 0
}

func setHTMLOverflowRoute(state *htmlRenderState, node *dom.Node, binding htmlOverflowClipBinding) {
	if binding.entity == 0 {
		delete(state.OverflowRoutes, node)
	} else {
		state.OverflowRoutes[node] = binding
	}
}

// Coalesce changed clip roots so nested transitions clear each DOM subtree
// only once, including inline nodes that no longer emit visible fragments.
func clearHTMLChangedClipRoutes(state *htmlRenderState) {
	defer state.BoxFlags.clear()
	changedAncestor := func(box *layout.Box, covered bool) bool {
		return covered || state.BoxRenderings[box.Node].ClipChanged
	}
	for _, rendered := range state.RenderQueue {
		if !rendered.ClipChanged {
			continue
		}
		if !state.BoxFlags.resolve(rendered.Box.Parent, htmlBoxParent, changedAncestor) {
			clearHTMLOverflowRoutes(state, rendered.Box.Node)
		}
	}
}

func clearHTMLOverflowRoutes(state *htmlRenderState, node *dom.Node) {
	delete(state.OverflowRoutes, node)
	for _, child := range node.Children {
		clearHTMLOverflowRoutes(state, child)
	}
}

// Former context owners may retire in the same pass as a surviving subtree
// moves. They must be cleaned up, not emitted or expanded via their stale boxes.
func queueHTMLRetainedContext(state *htmlRenderState, rendered *htmlBoxRendering) bool {
	return rendered != nil && (rendered.Retiring || queueHTMLBox(state, rendered.Box))
}
