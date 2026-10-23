package html

import (
	"strings"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/layout"
)

type htmlMutation uint8

const (
	htmlMutationStyle htmlMutation = 1 << iota
	htmlMutationContent
	htmlMutationStructure
	htmlMutationText
	htmlMutationDefinitions
	htmlMutationAttribute
)

// Counters describe work performed by the last sync, for regression tests and
// benchmarks without timing-dependent assertions.
type htmlSyncStats struct {
	UpdatedControls, SnapshottedControls             int
	UpdatedClips, RoutedEntities, ExtentBoxes        int
	Styled, PaintUpdated, Spawned, Despawned, Reused int
	LayoutWork                                       layout.Work
	Layouts                                          int
	ReconciledBoxes, PartialReconciles               int
	BindingEntities                                  int
	OrderUpdated                                     int
	OwnerListBoxes                                   int
	PaintCullBoxes                                   int
}

func (n Node) markMutation(kind htmlMutation) {
	state := ecs.GetMut[htmlRenderState](n.world, n.owner)
	if state == nil {
		return
	}
	if state.Mutations == nil {
		state.Mutations = make(map[*dom.Node]htmlMutation)
	}
	state.Mutations[n.node] |= kind
	state.Revision++
	if kind&(htmlMutationContent|htmlMutationDefinitions) != 0 {
		state.StyleCache.InvalidateDefinitions()
	}
	if kind&htmlMutationStructure != 0 {
		dom.SynchronizeSelectChildren(n.node)
		state.IndexDirty = true
	}
}

// Structure mutations are rooted at the parent whose child list changes. This
// includes its :empty state and all positional selectors on surviving children.
// Inspect removed subtrees before detaching them so document-wide SVG resources
// are invalidated even when their defining nodes no longer belong to the DOM.
func (n Node) structureMutationKind(changed *dom.Node) htmlMutation {
	for ancestor := n.node; ancestor != nil; ancestor = ancestor.Parent {
		if htmlDefinesFilter(ancestor) {
			return htmlMutationStructure | htmlMutationDefinitions
		}
	}
	if changed != nil && htmlSubtreeDefinesFilter(changed) {
		return htmlMutationStructure | htmlMutationDefinitions
	}
	return htmlMutationStructure
}

// Classify resources while detaching the direct children, sharing the traversal
// used by Clear and structural SetText instead of scanning the parent twice.
func (n Node) detachChildrenMutationKind() htmlMutation {
	kind := n.structureMutationKind(nil)
	for _, child := range n.node.Children {
		if kind&htmlMutationDefinitions == 0 && htmlSubtreeDefinesFilter(child) {
			kind |= htmlMutationDefinitions
		}
		child.Parent = nil
	}
	children := n.node.Children
	clear(children[:cap(children)])
	n.node.Children = children[:0]
	return kind
}

func htmlDefinesFilter(node *dom.Node) bool {
	return node.Type == dom.ElementNode && strings.EqualFold(node.Data, "filter")
}

func htmlSubtreeDefinesFilter(node *dom.Node) bool {
	if htmlDefinesFilter(node) {
		return true
	}
	for _, child := range node.Children {
		if htmlSubtreeDefinesFilter(child) {
			return true
		}
	}
	return false
}

func (n Node) markAttributeDirty(name string) {
	// These attributes only affect selector matching/cascade. Other attributes
	// may also change native controls, image sources or intrinsic measurements.
	switch name {
	case "class", "style":
		n.markMutation(htmlMutationStyle)
	case "id", "src", "alt", "title", "hidden":
		kind := htmlMutationAttribute
		for ancestor := n.node; ancestor != nil; ancestor = ancestor.Parent {
			if htmlDefinesFilter(ancestor) {
				kind |= htmlMutationDefinitions
				break
			}
		}
		n.markMutation(kind)
	default:
		if strings.HasPrefix(name, "aria-") || strings.HasPrefix(name, "data-") {
			n.markMutation(htmlMutationStyle)
		} else {
			n.markMutation(htmlMutationContent)
		}
	}
	if name == "id" {
		if state := ecs.GetMut[htmlRenderState](n.world, n.owner); state != nil {
			state.IndexDirty = true
		}
	}
}

func syncHTMLDocumentIndex(state *htmlRenderState) {
	if state.NodeByID == nil {
		state.NodeByID = make(map[string]*dom.Node)
	} else {
		clear(state.NodeByID)
	}
	var order uint32
	var visit func(*dom.Node)
	visit = func(node *dom.Node) {
		if node == nil {
			return
		}
		node.Order = order
		order++
		if id, ok := node.Attr("id"); ok && id != "" {
			if _, exists := state.NodeByID[id]; !exists {
				state.NodeByID[id] = node
			}
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	visit(state.Root)
	state.IndexDirty = false
}

func prepareHTMLMutationRoots(state *htmlRenderState) bool {
	clear(state.MutationRoots)
	state.MutationRoots = state.MutationRoots[:0]
	if state.MutationRootSet == nil {
		state.MutationRootSet = make(map[*dom.Node]struct{})
	}
	defer clear(state.MutationRootSet)
	defer state.NodeFlags.clear()
	defer state.NodeAncestors.clear()
	withinDocument := func(node *dom.Node, inside bool) bool { return inside || node == state.Root }
	document := state.StyleCache.NeedsDocumentStyle(state.Stylesheet)
	siblingStyle := state.StyleCache.NeedsSiblingStyle(state.Stylesheet)
	layoutDirty := false
	for node, kind := range state.Mutations {
		if kind&(htmlMutationContent|htmlMutationStructure|htmlMutationText|htmlMutationAttribute) != 0 {
			state.LayoutNodes = append(state.LayoutNodes, node)
		}
		state.LayoutTopology = state.LayoutTopology || kind&htmlMutationStructure != 0
		if kind&(htmlMutationText|htmlMutationStructure|htmlMutationAttribute) != 0 {
			layoutDirty = true
		}
		if kind&(htmlMutationContent|htmlMutationDefinitions) != 0 {
			layoutDirty, document = true, true
		}
		if node == state.Root || node.Data == "html" {
			document = true
		}
		if document || !state.NodeFlags.resolve(node, htmlNodeParent, withinDocument) {
			continue
		}
		mutationRoot := node
		if siblingStyle && kind&(htmlMutationStyle|htmlMutationStructure|htmlMutationText|htmlMutationAttribute) != 0 && node.Parent != nil {
			mutationRoot = node.Parent
		}
		state.MutationRootSet[mutationRoot] = struct{}{}
	}
	if document {
		state.MutationRoots = append(state.MutationRoots, state.Root)
		return layoutDirty
	}
	selectedAncestor := func(node, ancestor *dom.Node) *dom.Node {
		if _, selected := state.MutationRootSet[node]; selected {
			return node
		}
		return ancestor
	}
	// All candidates are known before coverage is resolved, so map iteration
	// order cannot cause a subtree to be restyled twice.
	for node := range state.MutationRootSet {
		if state.NodeAncestors.resolve(node.Parent, htmlNodeParent, selectedAncestor) == nil {
			state.MutationRoots = append(state.MutationRoots, node)
		}
	}
	return layoutDirty
}

func clearHTMLMutations(state *htmlRenderState) {
	state.LayoutTopology = false
	clear(state.Mutations)
	clear(state.MutationRoots)
	state.MutationRoots = state.MutationRoots[:0]
	clear(state.LayoutOrderNodes)
	state.LayoutOrderNodes = state.LayoutOrderNodes[:0]
	clear(state.LayoutNodes)
	state.LayoutNodes = state.LayoutNodes[:0]
	clear(state.StyleChanges)
	state.StyleChanges = state.StyleChanges[:0]
}

func sameHTMLRenderEnvironment(left, right htmlRenderKey) bool {
	left.Revision, right.Revision = 0, 0
	left.EventRevision, right.EventRevision = 0, 0
	return left == right
}

// Track surfaces rather than scanning every generated primitive for orphans.
type htmlSurfaceOwners struct{ Children map[ecs.Entity][]ecs.Entity }

func htmlOwners(world *ecs.World) *htmlSurfaceOwners {
	owners, ok := ecs.GetResource[*htmlSurfaceOwners](world)
	if !ok {
		owners = &htmlSurfaceOwners{Children: make(map[ecs.Entity][]ecs.Entity)}
		ecs.InsertResource(world, owners)
		ecs.OnComponentRemove[htmlRenderState](world, cleanupHTMLTextures)
		ecs.OnComponentRemove[Surface](world, cleanupHTMLSurface)
	}
	return owners
}

func syncHTMLVisibility(world *ecs.World, owner ecs.Entity, state *htmlRenderState) {
	visible := htmlVisible(world, owner)
	if !state.VisibilityKnown || state.Visible != visible {
		setHTMLChildrenVisible(world, state, state.Children, visible)
		state.Visible, state.VisibilityKnown = visible, true
	}
}
