package html

import (
	"math"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/layout"
	"github.com/diiyw/onix/html/style"
	"github.com/diiyw/onix/scene"
	onixtext "github.com/diiyw/onix/text"
	"github.com/go-gl/mathgl/mgl32"
)

// htmlPaintWindow keeps buffered CSS clip bounds in document coordinates.
// A container's own paint uses its inherited window; only its contents receive
// its clip and scroll offset. No implicit clip is added for the Surface size.
type htmlPaintWindow struct {
	left, top, right, bottom float64
	shiftX, shiftY           float64
	clipX, clipY             bool
	unbounded                bool
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
	if box.Style.Position == style.Keyword("sticky") || htmlPaintStyleMoves(box.Style) || len(state.NodeOffsets) != 0 || len(state.NodeScrolls) != 0 {
		window.unbounded = true
	}
	return window
}

func htmlPaintFragmentVisible(window htmlPaintWindow, rect layout.Rect, value *style.ComputedStyle) bool {
	return htmlStyleVisible(value) && (htmlPaintStyleMoves(value) || window.contains(rect))
}

func htmlPaintWindowChanged(state *htmlRenderState, rendered *htmlBoxRendering) bool {
	clipped := htmlBoxNeedsOverflowClip(state, rendered.Box)
	if clipped && rendered.CullRect != rendered.Box.ScrollportBox() {
		return true
	}
	if !clipped && !rendered.Window.clipX && !rendered.Window.clipY && !rendered.OuterWindow.clipX && !rendered.OuterWindow.clipY {
		return false
	}
	window := htmlPaintWindow{}
	if parent := rendered.Box.Parent; parent != nil {
		window = state.BoxRenderings[parent.Node].Window
	}
	window = htmlPaintWindowForBox(state, rendered.Box, window)
	return window.unbounded != rendered.OuterWindow.unbounded ||
		(window.unbounded || len(rendered.Box.Style.Filters) != 0) != rendered.Window.unbounded
}

// Bands share a coordinate space through nested clips. The buffer is rebuilt
// only after scrolling uses half its margin, so small wheel steps reuse paint.
func htmlPaintWindowForClip(world *ecs.World, state *htmlRenderState, box *layout.Box, window htmlPaintWindow) htmlPaintWindow {
	if state.PaintCullDisabled || len(box.Style.Filters) != 0 {
		window.unbounded = true
	}
	if window.unbounded || !htmlBoxNeedsOverflowClip(state, box) {
		delete(state.PaintCullOffsets, box.Node)
		return window
	}
	rect := box.ScrollportBox()
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
		state.PaintCullOffsets = make(map[*dom.Node]mgl32.Vec2)
	}
	state.PaintCullOffsets[box.Node] = offset
	return window
}

func htmlOverflowBindingIs(world *ecs.World, node *dom.Node, entity ecs.Entity) bool {
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

func htmlPaintCullChangedRoot(world *ecs.World, state *htmlRenderState, node *dom.Node, recorded mgl32.Vec2) *htmlBoxRendering {
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
	if rendered.CompositeRoot {
		return rendered.OuterWindow.contains(rendered.EffectBounds) != outer.contains(rendered.EffectBounds)
	}
	if rendered.OuterWindow.contains(rendered.Box.BorderBox()) != outer.contains(rendered.Box.BorderBox()) ||
		rendered.OuterWindow.contains(rendered.PaintBounds) != outer.contains(rendered.PaintBounds) {
		return true
	}
	if rendered.Window == window {
		return false
	}
	for _, fragment := range rendered.Box.TextFragments {
		if htmlPaintFragmentVisible(rendered.Window, fragment.Rect, fragment.Style) != htmlPaintFragmentVisible(window, fragment.Rect, fragment.Style) {
			return true
		}
	}
	for _, fragment := range rendered.Box.ImageFragments {
		if htmlPaintFragmentVisible(rendered.Window, fragment.Rect, fragment.Style) != htmlPaintFragmentVisible(window, fragment.Rect, fragment.Style) {
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
		htmlPaintCullBoxes(world, owner, state, root.Box, root.OuterWindow, fontValue, boldFont, dpi, stackIndex, passThrough, visible, &mounted, &blocked)
		if blocked {
			break
		}
	}
	state.Children = children
	state.Reconciling, state.LocalReconcile = false, false
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
	window = htmlPaintWindowForClip(world, state, box, outer)
	dirty := htmlPaintCullDirty(rendered, outer, window)
	rendered.OuterWindow, rendered.Window = outer, window
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
	for _, child := range box.Children {
		htmlPaintCullBoxes(world, owner, state, child, window, fontValue, boldFont, dpi, stackIndex, passThrough, visible, mounted, blocked)
		if *blocked {
			return
		}
	}
}

// Local emission retires the box's own primitives, then re-routes the freshly
// mounted ones through its clip and any app scroll binding.
func reconcileHTMLPaintCullBox(world *ecs.World, owner ecs.Entity, state *htmlRenderState, box *layout.Box, rendered *htmlBoxRendering, fontValue, boldFont *onixtext.Font, dpi float64, stackIndex int, passThrough, visible bool) {
	updateHTMLControlSnapshots(world, state, rendered.Entities)
	delete(state.EventTargets, box.Node)
	var control, focusOwner ecs.Entity
	var controlRect layout.Rect
	if parent := box.Parent; parent != nil {
		if inherited := state.BoxRenderings[parent.Node]; inherited != nil {
			control, focusOwner, controlRect = inherited.Control, inherited.FocusOwner, inherited.ControlRect
		}
	}
	state.Children = rendered.Entities[:0]
	htmlSpawnBoxes(world, owner, state, box, fontValue, boldFont, dpi, stackIndex, passThrough, visible, control, focusOwner, controlRect)
	rendered.Entities = state.Children
	state.ControlStylesChanged = restoreHTMLControlEntities(world, state, rendered.Entities) || state.ControlStylesChanged
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
