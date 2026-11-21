package html

import (
	"math"
	"slices"
	"sort"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/html/layout"
	"github.com/onix-engine/onix/html/style"
	"github.com/onix-engine/onix/scene"
	uinode "github.com/onix-engine/onix/scene/node"
	onixtext "github.com/onix-engine/onix/text"
)

// htmlPaintWindow keeps buffered CSS clip bounds in document coordinates.
// A container's own paint uses its inherited window; only its contents receive
// its clip and scroll offset. No implicit clip is added for the Surface size.
type htmlPaintWindow struct {
	left, top, right, bottom float64
	shiftX, shiftY           float64
	clipX, clipY             bool
	unbounded                bool
	noPaintList              bool
}

// Most boxes paint into an unrestricted window. Keep the snapshot only when
// culling or isolation needs one, and reuse it across subsequent scrolls.
type htmlBoxPaintClip struct {
	outer, contents htmlPaintWindow
	scrollport      layout.Rect
	clipped         bool
	owned           bool
}

var unrestrictedHTMLClip htmlBoxPaintClip

func (r *htmlBoxRendering) paintClip() *htmlBoxPaintClip {
	if r.clipWindow == nil {
		return &unrestrictedHTMLClip
	}
	return r.clipWindow
}

func (state *htmlPaintState) setPaintClip(rendered *htmlBoxRendering, clip htmlBoxPaintClip) {
	current := rendered.clipWindow
	if current != nil && current.owned {
		clip.owned = true
		*current = clip
		return
	}
	if clip == (htmlBoxPaintClip{}) {
		rendered.clipWindow = nil
		return
	}
	if clip.clipped || clip.outer != clip.contents {
		clip.owned = true
		rendered.clipWindow = new(htmlBoxPaintClip)
		*rendered.clipWindow = clip
		return
	}
	if current != nil && *current == clip {
		return
	}
	rendered.clipWindow = state.inheritedPaintClip(clip)
}

// Shared snapshots stay immutable while a row references them. The short
// cache covers repeated scroll offsets without pinning every historical window.
func (state *htmlPaintState) inheritedPaintClip(clip htmlBoxPaintClip) *htmlBoxPaintClip {
	for offset := range len(state.InheritedClips) {
		index := (int(state.NextInheritedClip) + len(state.InheritedClips) - 1 - offset) % len(state.InheritedClips)
		if candidate := state.InheritedClips[index]; candidate != nil && *candidate == clip {
			return candidate
		}
	}
	snapshot := new(htmlBoxPaintClip)
	*snapshot = clip
	state.InheritedClips[state.NextInheritedClip] = snapshot
	state.NextInheritedClip = (state.NextInheritedClip + 1) % uint8(len(state.InheritedClips))
	return snapshot
}

func htmlPaintScrollport(state *htmlRenderState, box *layout.Box) (layout.Rect, bool) {
	if htmlBoxNeedsOverflowClip(state, box) {
		return box.ScrollportBox(), true
	}
	return layout.Rect{}, false
}

func (window htmlPaintWindow) contains(rect layout.Rect) bool {
	if window.unbounded {
		return true
	}
	x, y := rect.X+window.shiftX, rect.Y+window.shiftY
	return (!window.clipX || x+rect.Width > window.left && x < window.right) &&
		(!window.clipY || y+rect.Height > window.top && y < window.bottom)
}

const htmlPaintCullMinOverscan = 256
const htmlPaintBandMinRows = 128

func htmlPaintOverscan(size float64) float64 { return max(size, htmlPaintCullMinOverscan) }

// Transform animations and external bindings can move an otherwise culled
// primitive into view without layout. Keep those subtrees mounted until their
// coordinate spaces can be mapped into the cull rect, rather than lose pixels.
func htmlPaintStyleMoves(value *style.ComputedStyle) bool {
	return value != nil && (value.TransformRotation != 0 || value.TransformScaleX != 1 || value.TransformScaleY != 1 || value.TransformSkewX != 0 ||
		value.TransformTranslateX != style.LengthPx(0) || value.TransformTranslateY != style.LengthPx(0) ||
		value.TransformTranslateCrossX != 0 || value.TransformTranslateCrossY != 0 || value.TransitionFor(0).Enabled || len(value.Animations()) != 0)
}

func htmlPaintWindowForBox(state *htmlRenderState, box *layout.Box, window htmlPaintWindow) htmlPaintWindow {
	window.noPaintList = window.noPaintList || htmlPaintListIsolated(box.Style)
	if box.Style.Position == style.Keyword("sticky") || htmlPaintStyleMoves(box.Style) || len(state.NodeOffsets) != 0 || len(state.NodeScrolls) != 0 {
		window.unbounded = true
	}
	return window
}

func htmlPaintFragmentVisible(window htmlPaintWindow, rect layout.Rect, value *style.ComputedStyle) bool {
	return htmlStyleVisible(value) && (htmlPaintStyleMoves(value) || window.contains(rect))
}

func htmlPaintWindowChanged(state *htmlRenderState, rendered *htmlBoxRendering) bool {
	clip := rendered.paintClip()
	rect, clipped := htmlPaintScrollport(state, rendered.Box)
	if clipped != clip.clipped || rect != clip.scrollport {
		return true
	}
	if state.PaintList == nil && !clip.contents.unbounded && !clip.contents.noPaintList &&
		!clipped && !clip.contents.clipX && !clip.contents.clipY && !clip.outer.clipX && !clip.outer.clipY {
		return false
	}
	window := htmlPaintWindow{}
	if parent := rendered.Box.Parent; parent != nil {
		window = state.BoxRenderings[parent.Node].paintClip().contents
	}
	window = htmlPaintWindowForBox(state, rendered.Box, window)
	return window.noPaintList != clip.contents.noPaintList || window.unbounded != clip.outer.unbounded ||
		(state.PaintCullDisabled || window.unbounded || len(rendered.Box.Style.Filters) != 0) != clip.contents.unbounded
}

// Bands share a coordinate space through nested clips. The buffer is rebuilt
// only after scrolling uses half its margin, so small wheel steps reuse paint.
func htmlPaintWindowForClip(world *ecs.World, state *htmlRenderState, box *layout.Box, window htmlPaintWindow, rect layout.Rect, clipped bool) htmlPaintWindow {
	if state.PaintCullDisabled || len(box.Style.Filters) != 0 {
		window.unbounded = true
	}
	if window.unbounded || !clipped {
		delete(state.PaintCullOffsets, box.Node)
		return window
	}
	offset := mgl32.Vec2{}
	if binding, ok := state.OverflowBindings[box.Node]; ok && htmlOverflowBindingIs(world, box.Node, binding.entity) {
		if scroll, exists := world.Get[scene.UIScroll](binding.entity); exists {
			offset = scroll.Position
		}
	}
	clipX, clipY := htmlOverflowClipsAxis(box.Style.OverflowX), htmlOverflowClipsAxis(box.Style.OverflowY)
	if clipX {
		overscan := htmlPaintOverscan(rect.Width)
		left, right := rect.X+window.shiftX-overscan, rect.X+window.shiftX+rect.Width+overscan
		if window.clipX {
			left, right = max(window.left, left), min(window.right, right)
		}
		window.left, window.right, window.clipX = left, right, true
	}
	if clipY {
		overscan := htmlPaintOverscan(rect.Height)
		top, bottom := rect.Y+window.shiftY-overscan, rect.Y+window.shiftY+rect.Height+overscan
		if window.clipY {
			top, bottom = max(window.top, top), min(window.bottom, bottom)
		}
		window.top, window.bottom, window.clipY = top, bottom, true
	}
	window.shiftX += float64(offset.X())
	window.shiftY += float64(offset.Y())
	if state.PaintCullOffsets == nil {
		state.PaintCullOffsets = make(map[*uinode.Node]mgl32.Vec2)
	}
	state.PaintCullOffsets[box.Node] = offset
	return window
}

func htmlOverflowBindingIs(world *ecs.World, node *uinode.Node, entity ecs.Entity) bool {
	child, ok := world.Get[htmlChild](entity)
	return ok && child.Node == node
}

// Gather changed scroll roots without visiting unrelated DOM branches. Nested
// roots are coalesced before emission, independently of map iteration order.
func htmlPaintCullChanged(world *ecs.World, state *htmlRenderState) bool {
	state.PaintCullRoots = state.PaintCullRoots[:0]
	for node, recorded := range state.PaintCullOffsets {
		if rendered := htmlPaintCullChangedRoot(world, state, node, recorded); rendered != nil {
			rendered.CullChanged = true
			state.PaintCullRoots = append(state.PaintCullRoots, rendered)
		}
	}
	return len(state.PaintCullRoots) != 0
}

// The idle check must not consume or collect the roots needed by reconciliation.
func htmlPaintCullPending(world *ecs.World, state *htmlRenderState) bool {
	if state.PaintCullDisabled {
		return false
	}
	for node, recorded := range state.PaintCullOffsets {
		if htmlPaintCullChangedRoot(world, state, node, recorded) != nil {
			return true
		}
	}
	return false
}

func htmlPaintCullChangedRoot(world *ecs.World, state *htmlRenderState, node *uinode.Node, recorded mgl32.Vec2) *htmlBoxRendering {
	binding, ok := state.OverflowBindings[node]
	rendered := state.BoxRenderings[node]
	if !ok || rendered == nil {
		return nil
	}
	offset, exists := scene.GetUIScrollOffset(world, binding.entity)
	rect := rendered.Box.ScrollportBox()
	if exists && (math.Abs(float64(offset.X()-recorded.X())) >= htmlPaintOverscan(rect.Width)*.5 ||
		math.Abs(float64(offset.Y()-recorded.Y())) >= htmlPaintOverscan(rect.Height)*.5) {
		return rendered
	}
	return nil
}

func htmlPaintCullDirty(rendered *htmlBoxRendering, outer, window htmlPaintWindow) bool {
	clip := rendered.paintClip()
	if rendered.CompositeRoot {
		return clip.outer.contains(rendered.Effects.Composite) != outer.contains(rendered.Effects.Composite)
	}
	if clip.outer.contains(rendered.Box.BorderBox()) != outer.contains(rendered.Box.BorderBox()) ||
		clip.outer.contains(rendered.paintBounds()) != outer.contains(rendered.paintBounds()) {
		return true
	}
	if clip.contents == window {
		return false
	}
	for _, fragment := range rendered.Box.TextFragments {
		if htmlPaintFragmentVisible(clip.contents, fragment.Rect, fragment.Style) != htmlPaintFragmentVisible(window, fragment.Rect, fragment.Style) {
			return true
		}
	}
	for _, fragment := range rendered.Box.ImageFragments {
		if htmlPaintFragmentVisible(clip.contents, fragment.Rect, fragment.Style) != htmlPaintFragmentVisible(window, fragment.Rect, fragment.Style) {
			return true
		}
	}
	return false
}

// reconcileHTMLPaintCull re-mounts the visuals a scrolled viewport moved into
// or out of its scrollport, without re-running style or layout.
func reconcileHTMLPaintCull(world *ecs.World, owner ecs.Entity, state *htmlRenderState, fontValue, boldFont *onixtext.Font, dpi float64, stackIndex int, passThrough, visible bool) {
	if state.PaintCullDisabled || len(state.PaintCullOffsets) == 0 || state.Boxes == nil {
		return
	}
	state.RenderGeneration++
	state.Reconciling, state.LocalReconcile = true, true
	state.PaintCullSync = true
	children := state.Children
	mounted, blocked := false, false
	prepareHTMLNodeBindings(world, state)
	defer func() {
		for i, root := range state.PaintCullRoots {
			root.CullChanged = false
			state.PaintCullRoots[i] = nil
		}
		state.PaintCullRoots = state.PaintCullRoots[:0]
		state.BoxFlags.clear()
	}()
	changedAncestor := func(box *layout.Box, changed bool) bool {
		return changed || state.BoxRenderings[box.Node].CullChanged
	}
	for _, root := range state.PaintCullRoots {
		if state.BoxFlags.resolve(root.Box.Parent, htmlBoxParent, changedAncestor) {
			continue
		}
		htmlPaintCullBoxes(world, owner, state, root.Box, root.paintClip().outer, fontValue, boldFont, dpi, stackIndex, passThrough, visible, &mounted, &blocked)
		if blocked {
			break
		}
	}
	state.Children = children
	state.Reconciling, state.LocalReconcile = false, false
	state.PaintCullSync = false
	state.EmittingBox = nil
	state.Textures.sweep()
	if blocked {
		// A primitive shared by two boxes cannot be retired per box, so mount the
		// whole document again and stop windowing until it is reparsed.
		state.PaintCullDisabled, state.PaintCullOffsets = true, nil
		state.Revision++
		return
	}
	if mounted {
		state.OwnerSpansValid = false
		rebuildHTMLOwnerList(state)
	}
}

func htmlPaintCullBoxes(world *ecs.World, owner ecs.Entity, state *htmlRenderState, box *layout.Box, window htmlPaintWindow, fontValue, boldFont *onixtext.Font, dpi float64, stackIndex int, passThrough, visible bool, mounted, blocked *bool) {
	if box == nil || *blocked {
		return
	}
	rendered := state.BoxRenderings[box.Node]
	if rendered == nil {
		return
	}
	state.SyncStats.PaintCullBoxes++
	if rendered.Composite && !rendered.CompositeRoot {
		return
	}
	outer := htmlPaintWindowForBox(state, box, window)
	rect, clipped := htmlPaintScrollport(state, box)
	window = htmlPaintWindowForClip(world, state, box, outer, rect, clipped)
	dirty := htmlPaintCullDirty(rendered, outer, window)
	previous := rendered.paintClip().contents
	state.setPaintClip(rendered, htmlBoxPaintClip{outer: outer, contents: window, scrollport: rect, clipped: clipped})
	if dirty {
		if rendered.SharedRendering || rendered.Box != box {
			*blocked = true
			return
		}
		reconcileHTMLPaintCullBox(world, owner, state, box, rendered, fontValue, boldFont, dpi, stackIndex, passThrough, visible)
		*mounted = true
	}
	if rendered.CompositeRoot {
		return
	}
	if len(box.Children) >= htmlPaintBandMinRows && previous.clipY && window.clipY && !previous.unbounded && !window.unbounded {
		if band := htmlScrollPaintBand(state, rendered); len(band) != 0 {
			// Compare each entering or leaving row with the last clip window.
			// Rows outside both windows do not need to store the new bounds.
			walkHTMLScrollPaintBand(band, box.Children, state, previous, window, func(child *layout.Box) bool {
				htmlPaintCullBoxes(world, owner, state, child, window, fontValue, boldFont, dpi, stackIndex, passThrough, visible, mounted, blocked)
				return !*blocked
			})
			return
		}
	}
	for _, child := range box.Children {
		htmlPaintCullBoxes(world, owner, state, child, window, fontValue, boldFont, dpi, stackIndex, passThrough, visible, mounted, blocked)
		if *blocked {
			return
		}
	}
}

type htmlPaintBand struct {
	rows  []htmlPaintRow
	epoch uint64
}

type htmlPaintRow struct {
	top, bottom float64
}

func walkHTMLScrollPaintBand(rows []htmlPaintRow, children []*layout.Box, state *htmlRenderState, before, after htmlPaintWindow, visit func(*layout.Box) bool) {
	rangeForWindow := func(window htmlPaintWindow) (int, int) {
		top, bottom := window.top-window.shiftY, window.bottom-window.shiftY
		first := sort.Search(len(rows), func(i int) bool { return rows[i].bottom > top })
		last := sort.Search(len(rows), func(i int) bool { return rows[i].top >= bottom })
		return first, max(first, last)
	}
	start, end := rangeForWindow(before)
	nextStart, nextEnd := rangeForWindow(after)
	if nextStart < start {
		start, end, nextStart, nextEnd = nextStart, nextEnd, start, end
	}
	if nextStart <= end {
		end = max(end, nextEnd)
		nextStart, nextEnd = end, end
	}
	// Indexed rows inherit this window unchanged. Use the parent's previous
	// window even if a row has not been visited since an earlier scroll.
	clip := state.inheritedPaintClip(htmlBoxPaintClip{outer: before, contents: before})
	for _, interval := range [2][2]int{{start, end}, {nextStart, nextEnd}} {
		for _, child := range children[interval[0]:interval[1]] {
			row := state.BoxRenderings[child.Node]
			row.clipWindow = clip
			if !visit(child) {
				return
			}
		}
	}
}

// Index leaf rows only when their paint bounds are monotonic and their clip is
// inherited unchanged. General CSS trees continue through the recursive walk.
func htmlScrollPaintBand(state *htmlRenderState, parent *htmlBoxRendering) []htmlPaintRow {
	band := parent.PaintBand
	if band != nil && band.epoch == state.PaintCullEpoch {
		return band.rows
	}
	if band == nil {
		band = &htmlPaintBand{}
		parent.PaintBand = band
	}
	if cap(band.rows) > 2*len(parent.Box.Children) {
		band.rows = nil
	}
	band.rows = band.rows[:0]
	band.epoch = state.PaintCullEpoch
	for _, box := range parent.Box.Children {
		row := state.BoxRenderings[box.Node]
		if row == nil || len(box.Children) != 0 || row.SharedRendering || row.Composite || row.Overflow != nil ||
			htmlPaintWindowForBox(state, box, htmlPaintWindow{}).unbounded || len(box.Style.Filters) != 0 {
			band.rows = band.rows[:0]
			return nil
		}
		// A moving inline fragment can paint outside the row regardless of its
		// stored geometry. Leave the whole scroller on the general path.
		bounds := row.paintBounds()
		for _, fragment := range box.TextFragments {
			if htmlPaintStyleMoves(fragment.Style) {
				band.rows = band.rows[:0]
				return nil
			}
			bounds = htmlUnionPaintBounds(bounds, fragment.Rect)
		}
		for _, fragment := range box.ImageFragments {
			if htmlPaintStyleMoves(fragment.Style) {
				band.rows = band.rows[:0]
				return nil
			}
			bounds = htmlUnionPaintBounds(bounds, fragment.Rect)
		}
		next := htmlPaintRow{top: bounds.Y, bottom: bounds.Y + bounds.Height}
		if n := len(band.rows); n != 0 && (band.rows[n-1].top > next.top || band.rows[n-1].bottom > next.bottom) {
			band.rows = band.rows[:0]
			return nil
		}
		if len(band.rows) == 0 {
			band.rows = slices.Grow(band.rows, len(parent.Box.Children))
		}
		band.rows = append(band.rows, next)
	}
	return band.rows
}

// Local emission retires the box's own primitives, then re-routes the freshly
// mounted ones through its clip and any app scroll binding.
func reconcileHTMLPaintCullBox(world *ecs.World, owner ecs.Entity, state *htmlRenderState, box *layout.Box, rendered *htmlBoxRendering, fontValue, boldFont *onixtext.Font, dpi float64, stackIndex int, passThrough, visible bool) {
	delete(state.EventTargets, box.Node)
	var control, focusOwner ecs.Entity
	var controlRect layout.Rect
	if parent := box.Parent; parent != nil {
		if inherited := state.BoxRenderings[parent.Node]; inherited != nil {
			context := inherited.inputContext()
			control, focusOwner, controlRect = context.Control, context.FocusOwner, context.ControlRect
		}
	}
	state.Children = rendered.Entities[:0]
	htmlSpawnBoxes(world, owner, state, box, fontValue, boldFont, dpi, stackIndex, passThrough, visible, control, focusOwner, controlRect)
	rendered.Entities = state.Children
	refreshHTMLBoxOverflow(world, owner, state, rendered, stackIndex, visible)
	finishHTMLBoxReconcile(world, state, rendered)
	applyHTMLNodeBindings(world, state, rendered.Entities)
}

// Use the same shadow bounds as texture emission, not just the border box.
func htmlBoxShadowRect(rect layout.Rect, shadow style.Shadow) (layout.Rect, float64) {
	spread := style.ResolveLength(shadow.SpreadRadius, min(rect.Width, rect.Height))
	blur := max(style.ResolveLength(shadow.BlurRadius, min(rect.Width, rect.Height)), 0)
	padding := max(blur*2+math.Abs(spread)+1, 1)
	return layout.Rect{
		X:     rect.X + style.ResolveLength(shadow.OffsetX, rect.Width) - padding,
		Y:     rect.Y + style.ResolveLength(shadow.OffsetY, rect.Height) - padding,
		Width: rect.Width + padding*2, Height: rect.Height + padding*2,
	}, padding
}

func htmlUnionPaintBounds(a, b layout.Rect) layout.Rect {
	if a.Width <= 0 || a.Height <= 0 {
		return b
	}
	if b.Width <= 0 || b.Height <= 0 {
		return a
	}
	left, top := min(a.X, b.X), min(a.Y, b.Y)
	return layout.Rect{X: left, Y: top, Width: max(a.X+a.Width, b.X+b.Width) - left, Height: max(a.Y+a.Height, b.Y+b.Height) - top}
}

func htmlBoxPaintBounds(box *layout.Box) layout.Rect {
	bounds := box.BorderBox()
	for _, shadow := range box.Style.BoxShadow {
		if shadow.Inset || shadow.Color.A == 0 {
			continue
		}
		r, _ := htmlBoxShadowRect(box.BorderBox(), shadow)
		bounds = htmlUnionPaintBounds(bounds, r)
	}
	return bounds
}
