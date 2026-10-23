package layout

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"

	"github.com/diiyw/onix/font"
	"github.com/diiyw/onix/internal/dom"
	"github.com/diiyw/onix/internal/style"
)

// Work counts formatting calls separately from reused and translated subtrees.
// A reused subtree is counted once, without visiting its descendants.
type Work struct {
	Visited, Reflowed, Reused, Translated, Created int
	LocalFlexReflows                               int
	TopologyVisited, OrderContainers               int
}

// Cache owns a retained layout tree and scratch storage for one document.
// Styles and DOM mutations are explicit inputs; callers must supply all nodes
// whose content or geometry changed, including inherited style changes.
type Cache struct {
	Root           *Box
	nodes          map[*dom.Node]*Box
	document       *dom.Node
	generation     uint64
	pass           uint64
	face, boldFace *font.Face
	viewport       Rect
	context        layoutContext
	Work           Work
	// Changed contains boxes needing render synchronization: reflowed or
	// translated boxes, plus ancestors whose descendant bounds may have changed.
	// Each box appears once per pass; callers can sort by current tree depth.
	Changed []*Box
	// Reordered contains flex containers whose child order actually changed.
	Reordered      []*Box
	localFlexRoots []*Box
	// Removed contains boxes retired by this topology pass.
	Removed     []*Box
	restylePass uint64
}

type boxLayoutCache struct {
	pass                              uint64
	orderPass                         uint64
	restylePass                       uint64
	reorderChangedPass, localFlexPass uint64
	changedPass                       uint64
	dirtyPass                         uint64
	containing, absolute              Rect
	valid, dirty, shiftable           bool
	placementReusable                 bool
	measurement                       *style.ComputedStyle
	width, height                     style.Value
	placementX, placementY            float64
	generation                        uint64
	parentID                          string
	index                             int
}

// Update reconciles topology only when needed, then propagates changed layout
// inputs through the existing formatting algorithm. Force invalidates all
// geometry for environment/resource changes. Reordered nodes only require
// sorting their retained flex parent; topology handles DOM/display changes.
func (cache *Cache) Update(root *dom.Node, styles style.Map, face, boldFace *font.Face, width, height float64, dirty, reordered []*dom.Node, topology, force bool) (*Box, error) {
	if root == nil {
		return nil, fmt.Errorf("layout root is nil")
	}
	cache.Work = Work{}
	clear(cache.Reordered)
	cache.Reordered = cache.Reordered[:0]
	clear(cache.Changed)
	cache.Changed = cache.Changed[:0]
	clear(cache.Removed)
	cache.Removed = cache.Removed[:0]
	cache.pass++
	cache.restylePass++
	defer func() {
		clear(cache.localFlexRoots)
		cache.localFlexRoots = cache.localFlexRoots[:0]
		clear(cache.context.inlineSegments[:cap(cache.context.inlineSegments)])
		clear(cache.context.textTokens[:cap(cache.context.textTokens)])
		clear(cache.context.controlTokens[:cap(cache.context.controlTokens)])
		for index := range cache.context.textScratch {
			scratch := &cache.context.textScratch[index]
			clear(scratch.lineItems[:cap(scratch.lineItems)])
			clear(scratch.inlineSegments[:cap(scratch.inlineSegments)])
			clear(scratch.textTokens[:cap(scratch.textTokens)])
			clear(scratch.controlTokens[:cap(scratch.controlTokens)])
		}
	}()
	viewport := Rect{Width: width, Height: height}
	if cache.document != root {
		cache.Root = nil
		clear(cache.nodes)
		cache.document = root
		cache.context = layoutContext{}
		topology, force = true, true
	}
	if cache.face != face || cache.boldFace != boldFace || cache.viewport != viewport {
		force = true
	}
	cache.face, cache.boldFace, cache.viewport = face, boldFace, viewport
	cache.context.boldFace, cache.context.err, cache.context.retained = boldFace, nil, cache
	// Resource dimensions may have changed in an environment-wide update.
	if force {
		clear(cache.context.imageSizes)
		topology = true
	}
	if topology || cache.Root == nil {
		if cache.nodes == nil {
			cache.nodes = make(map[*dom.Node]*Box)
		}
		cache.generation++
		cache.Root = cache.reconcile(root, nil, "", 0, styles, force)
		for node, box := range cache.nodes {
			if box.reflow.generation != cache.generation {
				cache.Removed = append(cache.Removed, box)
				delete(cache.nodes, node)
			}
		}
	} else {
		cache.Restyle(reordered, styles)
		cache.reorderFlexContainers(reordered)
	}
	if cache.Root == nil {
		return nil, fmt.Errorf("no block layout tree could be built")
	}
	for _, node := range dirty {
		for current := node; current != nil; current = current.Parent {
			if box := cache.nodes[current]; box != nil {
				cache.restyle(box, styles)
				cache.markLayoutDirty(box)
				break
			}
		}
	}
	if topology || force || !cache.layoutLocalFlexOrder(dirty, styles, face) {
		layoutBlock(cache.Root, viewport, viewport, face, styles, &cache.context)
	}
	if cache.context.err != nil {
		// A failed pass may have modified retained fragments. Never reuse them on retry.
		for _, box := range cache.nodes {
			box.reflow.valid = false
		}
		return nil, cache.context.err
	}
	cache.Root.Viewport = viewport
	return cache.Root, nil
}

// Restyle repoints retained boxes and fragments at the instances the cascade
// chose for changed nodes. Interning can move a node to another pointer.
func (cache *Cache) Restyle(nodes []*dom.Node, styles style.Map) {
	cache.restylePass++
	for _, node := range nodes {
		// A changed inline or text node has no box of its own; its fragments
		// live in the nearest retained ancestor box.
		for current := node; current != nil; current = current.Parent {
			box := cache.nodes[current]
			if box == nil {
				continue
			}
			cache.restyle(box, styles)
			break
		}
	}
}

// restyle adopts the current instance for a box and every fragment it retains.
func (cache *Cache) restyle(box *Box, styles style.Map) {
	if box.reflow.restylePass == cache.restylePass {
		return
	}
	box.reflow.restylePass = cache.restylePass
	box.Restyle(styles)
}

// Restyle adopts current paint styles without rebuilding geometry.
func (box *Box) Restyle(styles style.Map) {
	box.InvalidatePaintHash()
	box.Style = styleInstance(styles, box.Node, box.Style)
	for index := range box.TextFragments {
		fragment := &box.TextFragments[index]
		fragment.Style = styleInstance(styles, fragment.Node, fragment.Style)
	}
	for index := range box.ImageFragments {
		fragment := &box.ImageFragments[index]
		fragment.Style = styleInstance(styles, fragment.Node, fragment.Style)
	}
	for index := range box.ControlFragments {
		fragment := &box.ControlFragments[index]
		fragment.Style = styleInstance(styles, fragment.Node, fragment.Style)
		for nested := range fragment.TextFragments {
			text := &fragment.TextFragments[nested]
			text.Style = styleInstance(styles, text.Node, text.Style)
		}
		for nested := range fragment.ImageFragments {
			image := &fragment.ImageFragments[nested]
			image.Style = styleInstance(styles, image.Node, image.Style)
		}
	}
}

func styleInstance(styles style.Map, node *dom.Node, current *style.ComputedStyle) *style.ComputedStyle {
	if computed := styles[node]; computed != nil {
		return computed
	}
	return current
}

func (cache *Cache) reconcile(node *dom.Node, parent *Box, parentID string, index int, styles style.Map, force bool) *Box {
	cache.Work.TopologyVisited++
	if !nodeCreatesBox(node, styles) {
		return nil
	}
	box := cache.nodes[node]
	if box == nil {
		box = &Box{Node: node}
		cache.nodes[node] = box
		cache.Work.Created++
	}
	computed := styles[node]
	parentChanged := box.Parent != parent
	box.Parent = parent
	if parentChanged {
		box.reflow.dirtyPass = 0
	}
	if force || box.Style != computed || parentChanged {
		cache.markLayoutDirty(box)
		box.reflow.valid = false
	}
	box.Style = computed
	if box.StableID == "" || box.reflow.parentID != parentID || box.reflow.index != index {
		if parent == nil {
			box.StableID = "0"
		} else {
			box.StableID = parentID + "." + strconv.Itoa(index)
		}
		box.reflow.parentID, box.reflow.index = parentID, index
	}
	box.reflow.generation = cache.generation
	old := box.Children
	children := old[:0]
	shiftable := !isFlexContainer(box) && !isAbsolutePositioned(box) && !isRelativePositioned(box)
	// Positioned descendants need their containing block to be recomputed;
	// ordinary flow and flex descendants can restore a measured translation.
	box.reflow.placementReusable = !isAbsolutePositioned(box) && !isRelativePositioned(box)
	childIndex := 0
	box.hasFloats = false
	for _, childNode := range node.Children {
		children, shiftable = cache.reconcileLayoutChild(childNode, box, box.StableID, old, children, &childIndex, shiftable, styles, force)
	}
	if len(children) != len(old) {
		cache.markLayoutDirty(box)
	}
	if len(children) < len(old) {
		clear(old[len(children):])
	}
	box.Children = children
	if isFlexContainer(box) {
		slices.SortStableFunc(box.Children, compareFlexOrder)
	}
	box.reflow.shiftable = shiftable
	return box
}

func (cache *Cache) reconcileLayoutChild(node *dom.Node, parent *Box, parentID string, old, children []*Box, childIndex *int, shiftable bool, styles style.Map, force bool) ([]*Box, bool) {
	computed, ok := styles.Get(node)
	if !ok {
		return children, shiftable
	}
	if computed.Display.Kind == style.ValueKeyword && computed.Display.Keyword == "contents" {
		for _, child := range node.Children {
			children, shiftable = cache.reconcileLayoutChild(child, parent, parentID, old, children, childIndex, shiftable, styles, force)
		}
		return children, shiftable
	}
	child := cache.reconcile(node, parent, parentID, *childIndex, styles, force)
	*childIndex = *childIndex + 1
	if child == nil {
		return children, shiftable
	}
	if len(children) >= len(old) || old[len(children)] != child {
		cache.markLayoutDirty(parent)
	}
	children = append(children, child)
	parent.reflow.placementReusable = parent.reflow.placementReusable && child.reflow.placementReusable
	parent.hasFloats = parent.hasFloats || isFloated(child) || child.hasFloats && !establishesFloatContext(child)
	return children, shiftable && child.reflow.shiftable
}

func (cache *Cache) markLayoutDirty(box *Box) {
	for current := box; current != nil; current = current.Parent {
		if current.reflow.dirtyPass == cache.pass {
			break
		}
		current.reflow.dirtyPass = cache.pass
		current.reflow.dirty = true
	}
}

func layoutBlock(box *Box, containing, absolute Rect, face *font.Face, styles style.Map, ctx *layoutContext) {
	if ctx == nil || ctx.retained == nil {
		layoutBlockContents(box, containing, absolute, face, styles, ctx)
		return
	}
	cache := ctx.retained
	cache.Work.Visited++
	previous := box.reflow
	// Flex/grid sizing overrides Width/Height on a reusable style copy. Include
	// those values in the key without confusing intrinsic and final stretch
	// sizes. Other temporary styles (e.g. table sizing) have distinct identities.
	var measurement *style.ComputedStyle
	if box.Style != styles[box.Node] {
		measurement = box.Style
	}
	if ctx.uncached == 0 && !box.hasFloats && (ctx.floatContext == nil || len(ctx.floatContext.areas) == 0) && previous.valid && !previous.dirty && previous.measurement == measurement && previous.width == box.Style.Width && previous.height == box.Style.Height {
		if previous.containing == containing && previous.absolute == absolute {
			translateBox(box, -previous.placementX, -previous.placementY, ctx)
			box.reflow.placementX, box.reflow.placementY = 0, 0
			cache.Work.Reused++
			box.reflow.pass = cache.pass
			return
		}
		if (previous.shiftable || previous.pass == cache.pass && previous.placementReusable) && previous.containing.Width == containing.Width && previous.containing.Height == containing.Height {
			translateBox(box, -previous.placementX, -previous.placementY, ctx)
			box.reflow.placementX, box.reflow.placementY = 0, 0
			translateLayoutBox(box, containing.X-previous.containing.X, containing.Y-previous.containing.Y, true, ctx)
			box.reflow.containing, box.reflow.absolute, box.reflow.valid = containing, absolute, true
			cache.Work.Translated++
			box.reflow.pass = cache.pass
			return
		}
	}
	cache.Work.Reflowed++
	cache.recordChanged(box)
	// A fresh layout tree starts with no previous used size. In particular,
	// bottom-only absolute positioning must not read last frame's height here.
	// Preserve intermediate measurements only within the same layout pass.
	if box.reflow.pass != cache.pass {
		box.Content, box.FlowMarginBox = Rect{}, Rect{}
		box.reflow.pass = cache.pass
	}
	box.reflow.valid = false
	box.InvalidatePaintHash()
	layoutBlockContents(box, containing, absolute, face, styles, ctx)
	box.reflow.containing, box.reflow.absolute = containing, absolute
	box.reflow.dirty = false
	box.reflow.width, box.reflow.height = box.Style.Width, box.Style.Height
	box.reflow.measurement = measurement
	box.reflow.placementX, box.reflow.placementY = 0, 0
	box.reflow.valid = ctx.err == nil
}

func (cache *Cache) recordChanged(box *Box) {
	if box.reflow.changedPass != cache.pass {
		box.reflow.changedPass = cache.pass
		cache.Changed = append(cache.Changed, box)
	}
}

// DOM sibling indices are retained until topology changes. Use them to restore
// source order on ties; stable-sorting an already reordered list is insufficient.
func compareRetainedFlexOrder(left, right *Box) int {
	if order := compareFlexOrder(left, right); order != 0 {
		return order
	}
	return cmp.Compare(left.reflow.index, right.reflow.index)
}

func (cache *Cache) reorderFlexContainers(nodes []*dom.Node) {
	for _, node := range nodes {
		box := cache.nodes[node]
		if box == nil || box.Parent == nil || !isFlexContainer(box.Parent) {
			continue
		}
		parent := box.Parent
		if parent.reflow.orderPass == cache.pass {
			continue
		}
		parent.reflow.orderPass = cache.pass
		cache.Work.OrderContainers++
		if !slices.IsSortedFunc(parent.Children, compareRetainedFlexOrder) {
			slices.SortFunc(parent.Children, compareRetainedFlexOrder)
			parent.reflow.reorderChangedPass = cache.pass
			cache.Reordered = append(cache.Reordered, parent)
			cache.markLayoutDirty(parent)
		}
	}
}

// Coalesce nested changes, then preflight every independent context before
// laying out any of them. Ancestors stay dirty until all outer geometries agree.
func (cache *Cache) layoutLocalFlexOrder(dirty []*dom.Node, styles style.Map, face *font.Face) bool {
	if len(cache.Reordered) == 0 || !cache.Root.reflow.valid {
		return false
	}
	for _, box := range cache.Reordered {
		covered, independent := false, true
		for parent := box.Parent; parent != nil; parent = parent.Parent {
			if parent.reflow.reorderChangedPass == cache.pass {
				covered = true
				break
			}
			independent = independent && parent.Style.Position.Keyword == "static" && parent.Style.Display.Keyword == "block"
		}
		if covered {
			continue
		}
		if !independent || !box.reflow.valid || box.Style.Position.Keyword != "static" {
			return false
		}
		box.reflow.localFlexPass = cache.pass
		cache.localFlexRoots = append(cache.localFlexRoots, box)
	}
	for _, node := range dirty {
		covered := false
		for current := node; current != nil; current = current.Parent {
			if box := cache.nodes[current]; box != nil && box.reflow.localFlexPass == cache.pass {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	for _, box := range cache.localFlexRoots {
		previousFlow, previousBorder := box.FlowMarginBox, box.BorderBox()
		containing, absolute := box.reflow.containing, box.reflow.absolute
		layoutBlock(box, containing, absolute, face, styles, &cache.context)
		if cache.context.err != nil || box.FlowMarginBox != previousFlow || box.BorderBox() != previousBorder {
			return false
		}
	}
	for _, box := range cache.localFlexRoots {
		for parent := box.Parent; parent != nil && parent.reflow.dirty; parent = parent.Parent {
			parent.reflow.dirty = false
			// Shared ancestors are recorded once, for render ownership and overflow
			// dependencies, without revisiting unrelated siblings in layout.
			cache.recordChanged(parent)
		}
	}
	cache.Work.LocalFlexReflows += len(cache.localFlexRoots)
	return true
}
