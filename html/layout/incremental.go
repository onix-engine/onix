package layout

import (
	"cmp"
	"fmt"
	"github.com/onix-engine/onix/html/element"
	"slices"

	"github.com/onix-engine/onix/font"
	"github.com/onix-engine/onix/html/style"
	"github.com/onix-engine/onix/internal/reuse"
	uinode "github.com/onix-engine/onix/scene/node"
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
	nodes          map[*uinode.Node]*Box
	document       *uinode.Node
	face, boldFace *font.Face
	// Changed contains boxes needing render synchronization: reflowed or
	// translated boxes, plus ancestors whose descendant bounds may have changed.
	// Each box appears once per pass; callers can sort by current tree depth.
	Changed []*Box
	// Removed contains boxes retired by this topology pass.
	Removed        []*Box
	localFlexRoots []*Box
	// Reordered contains flex containers whose child order actually changed.
	Reordered     []*Box
	context       layoutContext
	retention     [4]reuse.Retention
	nodeRetention reuse.MapRetention
	Work          Work
	viewport      Rect
	pass          uint64
	generation    uint64
	restylePass   uint64
	nextID        uint64
	// ReserveScrollbarSpace selects classic scrollbars for this document.
	ReserveScrollbarSpace bool
}

// MaintainCache releases historical workspace peaks after sustained underuse.
// It is also safe between updates, after consumers finish using Changed/Removed.
func (cache *Cache) MaintainCache() {
	cache.nodes = cache.nodeRetention.Trim(cache.nodes, 64)
	cache.Changed = cache.retention[0].Trim(cache.Changed, 64)
	cache.Removed = cache.retention[1].Trim(cache.Removed, 64)
	cache.Reordered = cache.retention[2].Trim(cache.Reordered, 16)
	cache.localFlexRoots = cache.retention[3].Trim(cache.localFlexRoots, 16)
	cache.context.trim()
}

type boxLayoutCache struct {
	measurement          *style.ComputedStyle
	width, height        layoutDimension
	containing, absolute Rect
	changedPass          uint64
	placementX           float64
	pass                 uint64
	index                int
	generation           uint64
	placementY           float64
	restylePass          uint64
	dirtyPass            uint64
	dirtyFirst           int
	dirtyLast            int
	placementReusable    bool
	shiftable            bool
	dirty                bool
	dirtySelf            bool
	plainFlow            bool
	valid                bool
}

// Block layout resolves dimensions from their kind and two length terms.
type layoutDimension struct {
	pixels, percent float64
	kind            style.ValueKind
}

func dimensionKey(v style.Value) layoutDimension {
	return layoutDimension{pixels: v.LengthPx, percent: v.Percent, kind: v.Kind}
}

// BoxForNode returns the node's box from the latest layout update. Nodes with
// display:none or display:contents and inline nodes without a box return nil.
func (cache *Cache) BoxForNode(node *uinode.Node) *Box {
	return cache.nodes[node]
}

// Update reconciles topology only when needed, then propagates changed layout
// inputs through the existing formatting algorithm. Force invalidates all
// geometry for environment/resource changes. Reordered nodes only require
// sorting their retained flex parent; topology handles DOM/display changes.
func (cache *Cache) Update(root *uinode.Node, styles style.Map, face, boldFace *font.Face, width, height float64, dirty, reordered []*uinode.Node, topology, force bool) (*Box, error) {
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
		cache.retention[3].Record(len(cache.localFlexRoots))
		clear(cache.localFlexRoots)
		cache.localFlexRoots = cache.localFlexRoots[:0]
		clear(cache.context.inlineSegments[:cap(cache.context.inlineSegments)])
		clear(cache.context.controlTokens[:cap(cache.context.controlTokens)])
		cache.context.shapeGlyphs = cache.context.shapeGlyphs[:0]
		for index := range cache.context.textScratch {
			scratch := &cache.context.textScratch[index]
			clear(scratch.lineItems[:cap(scratch.lineItems)])
			clear(scratch.inlineSegments[:cap(scratch.inlineSegments)])
			clear(scratch.controlTokens[:cap(scratch.controlTokens)])
			scratch.shapeGlyphs = scratch.shapeGlyphs[:0]
		}
		cache.MaintainCache()
	}()
	viewport := Rect{Width: width, Height: height}
	if cache.document != root {
		cache.Root = nil
		cache.nodeRetention.Record(len(cache.nodes))
		clear(cache.nodes)
		cache.document = root
		cache.context = layoutContext{}
		topology, force = true, true
	}
	if cache.face != face || cache.boldFace != boldFace || cache.context.reserveScrollbarSpace != cache.ReserveScrollbarSpace {
		force = true
	}
	cache.face, cache.boldFace, cache.viewport = face, boldFace, viewport
	cache.context.boldFace, cache.context.err, cache.context.retained = boldFace, nil, cache
	cache.context.reserveScrollbarSpace = cache.ReserveScrollbarSpace
	// Resource dimensions may have changed in an environment-wide update.
	if force {
		clear(cache.context.imageSizes)
		topology = true
	}
	if topology || cache.Root == nil {
		cache.nodeRetention.Record(len(cache.nodes))
		if cache.nodes == nil {
			cache.nodes = make(map[*uinode.Node]*Box)
		}
		cache.generation++
		cache.Root = cache.reconcile(root, nil, 0, styles, force)
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
	cache.Root.setViewport(viewport)
	return cache.Root, nil
}

// Restyle repoints retained boxes and fragments at the instances the cascade
// chose for changed nodes. Interning can move a node to another pointer.
func (cache *Cache) Restyle(nodes []*uinode.Node, styles style.Map) {
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

func styleInstance(styles style.Map, node *uinode.Node, current *style.ComputedStyle) *style.ComputedStyle {
	if computed := styles[node]; computed != nil {
		return computed
	}
	return current
}

func (cache *Cache) reconcile(node *uinode.Node, parent *Box, index int, styles style.Map, force bool) *Box {
	cache.Work.TopologyVisited++
	if !nodeCreatesBox(node, styles) {
		return nil
	}
	box := cache.nodes[node]
	if box == nil {
		cache.nextID++
		box = &Box{Node: node, StableID: cache.nextID}
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
	box.reflow.index = index
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
		children, shiftable = cache.reconcileLayoutChild(childNode, box, old, children, &childIndex, shiftable, styles, force)
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
	box.reflow.plainFlow = len(children) >= 8 && len(children) == len(node.Children) && (node.Type == uinode.DocumentNode || box.Style.Display == style.Keyword("block") && !element.IsWidgetControl(node))
	if box.reflow.plainFlow {
		for i, child := range children {
			if child.Node != node.Children[i] || child.Style.Display != style.Keyword("block") || isAbsolutePositioned(child) {
				box.reflow.plainFlow = false
				break
			}
		}
	}
	box.reflow.shiftable = shiftable
	return box
}

func (cache *Cache) reconcileLayoutChild(node *uinode.Node, parent *Box, old, children []*Box, childIndex *int, shiftable bool, styles style.Map, force bool) ([]*Box, bool) {
	computed, ok := styles.Get(node)
	if !ok {
		return children, shiftable
	}
	if computed.Display.Kind == style.ValueKeyword && computed.Display.Keyword == "contents" {
		for _, child := range node.Children {
			children, shiftable = cache.reconcileLayoutChild(child, parent, old, children, childIndex, shiftable, styles, force)
		}
		return children, shiftable
	}
	child := cache.reconcile(node, parent, *childIndex, styles, force)
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
	if box.reflow.dirtyPass != cache.pass {
		box.reflow.dirtyFirst, box.reflow.dirtyLast = 0, 0
	}
	box.reflow.dirtySelf = true
	var child *Box
	for current := box; current != nil; current = current.Parent {
		reflow := &current.reflow
		alreadyDirty := reflow.dirtyPass == cache.pass
		if !alreadyDirty {
			reflow.dirtyFirst, reflow.dirtyLast = 0, 0
			reflow.dirtySelf = child == nil
		}
		if child != nil {
			index := child.reflow.index + 1
			if reflow.dirtyFirst == 0 {
				reflow.dirtyFirst = index
			}
			reflow.dirtyFirst, reflow.dirtyLast = min(reflow.dirtyFirst, index), max(reflow.dirtyLast, index)
		}
		if alreadyDirty {
			break
		}
		reflow.dirtyPass = cache.pass
		reflow.dirty = true
		child = current
	}
}

func layoutBlock(box *Box, containing, absolute Rect, face *font.Face, styles style.Map, ctx *layoutContext) {
	layoutBlockSized(box, box.Style.Width, box.Style.Height, containing, absolute, face, styles, ctx)
}

func layoutBlockSized(box *Box, width, height style.Value, containing, absolute Rect, face *font.Face, styles style.Map, ctx *layoutContext) {
	if ctx == nil || ctx.retained == nil {
		layoutBlockScrollbars(box, width, height, containing, absolute, face, styles, ctx)
		return
	}
	cache := ctx.retained
	cache.Work.Visited++
	previous := box.reflow
	widthKey, heightKey := dimensionKey(width), dimensionKey(height)
	// Include used dimensions in the key, since intrinsic and final layout can
	// size the same box differently. Table measurement can also override style.
	var measurement *style.ComputedStyle
	if box.Style != styles[box.Node] {
		measurement = box.Style
	}
	if ctx.uncached == 0 && !box.hasFloats && (ctx.floatContext == nil || len(ctx.floatContext.areas) == 0) && previous.valid && !previous.dirty && previous.measurement == measurement && previous.width == widthKey && previous.height == heightKey {
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
	if previous.plainFlow && previous.valid && !previous.dirtySelf && previous.dirtyFirst != 0 && previous.dirtyLast <= len(box.Children) &&
		previous.containing == containing && previous.absolute == absolute && previous.width == widthKey && previous.height == heightKey &&
		previous.placementX == 0 && previous.placementY == 0 && previous.measurement == measurement &&
		!ctx.reserveScrollbarSpace && ctx.uncached == 0 && !box.hasFloats && (ctx.floatContext == nil || len(ctx.floatContext.areas) == 0) &&
		reflowUnchangedBlockFlow(box, face, styles, ctx, previous.dirtyFirst-1, previous.dirtyLast) {
		box.reflow.pass, box.reflow.dirty = cache.pass, false
		return
	}
	// A fresh layout tree starts with no previous used size. In particular,
	// bottom-only absolute positioning must not read last frame's height here.
	// Preserve intermediate measurements only within the same layout pass.
	if box.reflow.pass != cache.pass {
		box.Content, box.FlowMarginBox = Rect{}, Rect{}
		box.reflow.pass = cache.pass
	}
	box.reflow.valid = false
	box.InvalidatePaintHash()
	layoutBlockScrollbars(box, width, height, containing, absolute, face, styles, ctx)
	box.reflow.containing, box.reflow.absolute = containing, absolute
	box.reflow.dirty = false
	box.reflow.width, box.reflow.height = widthKey, heightKey
	box.reflow.measurement = measurement
	box.reflow.placementX, box.reflow.placementY = 0, 0
	box.reflow.valid = ctx.err == nil
}

// Direct block children with unchanged flow positions leave the containing
// box's flow geometry and the layout inputs of every later sibling intact.
func reflowUnchangedBlockFlow(box *Box, face *font.Face, styles style.Map, ctx *layoutContext, first, end int) bool {
	for _, child := range box.Children[first:end] {
		if !child.reflow.dirty {
			continue
		}
		if !child.reflow.valid || child.Style.Display != style.Keyword("block") || isAbsolutePositioned(child) || isFloated(child) || child.hasFloats {
			return false
		}
		previous := child.FlowMarginBox
		marginBottom := child.Margin.Bottom
		// A changed top margin can change the collapse at this boundary.
		if child.Margin.Top != resolveLength(child.Style.Margin.Top, child.reflow.containing.Width) {
			return false
		}
		layoutBlock(child, child.reflow.containing, child.reflow.absolute, face, styles, ctx)
		if ctx.err != nil || previous.Y != child.FlowMarginBox.Y || previous.Height != child.FlowMarginBox.Height || marginBottom != child.Margin.Bottom {
			return false
		}
	}
	return true
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

func (cache *Cache) reorderFlexContainers(nodes []*uinode.Node) {
	for _, node := range nodes {
		box := cache.nodes[node]
		if box == nil || box.Parent == nil || !isFlexContainer(box.Parent) {
			continue
		}
		parent := box.Parent
		scratch := parent.flexScratch()
		if scratch.orderPass == cache.pass {
			continue
		}
		scratch.orderPass = cache.pass
		cache.Work.OrderContainers++
		if !slices.IsSortedFunc(parent.Children, compareRetainedFlexOrder) {
			slices.SortFunc(parent.Children, compareRetainedFlexOrder)
			scratch.reorderChangedPass = cache.pass
			cache.Reordered = append(cache.Reordered, parent)
			cache.markLayoutDirty(parent)
		}
	}
}

// Coalesce nested changes, then preflight every independent context before
// laying out any of them. Ancestors stay dirty until all outer geometries agree.
func (cache *Cache) layoutLocalFlexOrder(dirty []*uinode.Node, styles style.Map, face *font.Face) bool {
	if cache.ReserveScrollbarSpace || len(cache.Reordered) == 0 || !cache.Root.reflow.valid {
		return false
	}
	for _, box := range cache.Reordered {
		covered, independent := false, true
		for parent := box.Parent; parent != nil; parent = parent.Parent {
			if parent.flex != nil && parent.flex.reorderChangedPass == cache.pass {
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
		box.flex.localFlexPass = cache.pass
		cache.localFlexRoots = append(cache.localFlexRoots, box)
	}
	for _, node := range dirty {
		covered := false
		for current := node; current != nil; current = current.Parent {
			if box := cache.nodes[current]; box != nil && box.flex != nil && box.flex.localFlexPass == cache.pass {
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
