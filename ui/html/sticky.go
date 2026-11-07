package html

import (
	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/html/dom"
	"github.com/onix-engine/onix/html/layout"
	"github.com/onix-engine/onix/html/style"
	"github.com/onix-engine/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

// Sticky offsets stay separate from layout geometry. Each sticky box is solved
// once per scroll update; primitives only compose the portion below their ECS
// parent, including when that parent is a generated control or overflow clip.
type htmlStickyBinding struct {
	box                      *layout.Box
	containing               *layout.Box
	parent, scrollportSticky *htmlStickyBinding
	scrollport               *layout.Box
	area                     ecs.Entity
	viewport                 layout.Rect
	offset                   mgl32.Vec2
	resolved                 uint64
}

type htmlStickyRoute struct {
	sticky           *htmlStickyBinding
	scrollport       *layout.Box
	scrollportSticky *htmlStickyBinding
}

func trackHTMLStickyBox(state *htmlRenderState, box *layout.Box) {
	state.StickyDirty = true
	if box.Style.Position != style.Keyword("sticky") {
		delete(state.StickyBindings, box.Node)
		return
	}
	if state.StickyBindings == nil {
		state.StickyBindings = make(map[*dom.Node]*htmlStickyBinding)
	}
	if state.StickyBindings[box.Node] == nil {
		state.StickyBindings[box.Node] = &htmlStickyBinding{}
	}
}

func htmlStickyScrollContainer(box *layout.Box) bool {
	for _, value := range [2]style.Value{box.Style.OverflowX, box.Style.OverflowY} {
		switch value.Keyword {
		case "auto", "scroll", "hidden":
			return true
		}
	}
	return false
}

func bindHTMLStickyContent(world *ecs.World, state *htmlRenderState) {
	state.StickyDirty = false
	// Retain the route cache until the last sticky binding has been removed so
	// previously synthetic ScrollContent links can be cleared in the same pass.
	if len(state.StickyBindings) == 0 && len(state.StickyRoutes.values) == 0 {
		return
	}
	world.FrameValue[htmlStickyWorkTag, htmlStickyWork]().revision++
	for node, binding := range state.StickyBindings {
		box := state.boxForNode(node)
		if box == nil || box.Style.Position != style.Keyword("sticky") {
			delete(state.StickyBindings, node)
			continue
		}
		binding.box = box
	}
	state.StickyRoutes.clear()
	state.NodeAncestors.clear()
	defer state.NodeAncestors.clear()
	route := func(box *layout.Box) htmlStickyRoute {
		return state.StickyRoutes.resolve(box, htmlBoxParent, func(box *layout.Box, inherited htmlStickyRoute) htmlStickyRoute {
			if sticky := state.StickyBindings[box.Node]; sticky != nil {
				sticky.containing = box.Parent
				// Rows and row groups do not establish a containing block for
				// sticky table cells/rows; the table constrains their movement.
				for sticky.containing != nil && htmlStickyTableInternal(sticky.containing.Style.Display.Keyword) {
					sticky.containing = sticky.containing.Parent
				}
				sticky.parent, sticky.scrollportSticky = inherited.sticky, inherited.scrollportSticky
				sticky.scrollport = inherited.scrollport
				sticky.viewport = layout.Rect{Width: state.Viewport.Width, Height: state.Viewport.Height}
				sticky.area = 0
				if inherited.scrollport != nil {
					sticky.viewport = inherited.scrollport.Content
					sticky.area = state.OverflowBindings[inherited.scrollport.Node].entity
				}
				inherited.sticky = sticky
			}
			if htmlStickyScrollContainer(box) {
				inherited.scrollport, inherited.scrollportSticky = box, inherited.sticky
			}
			return inherited
		})
	}
	for _, entity := range state.Children {
		child := world.GetRef[htmlChild](entity)
		parent := world.GetRef[scene.Parent](entity)
		if child == nil || parent == nil {
			continue
		}
		sticky := route(htmlNearestBox(state, child.Node)).sticky
		var parentSticky *htmlStickyBinding
		if ancestor := world.GetRef[htmlChild](parent.Entity); ancestor != nil && ancestor.Owner == child.Owner {
			parentSticky = route(htmlNearestBox(state, ancestor.Node)).sticky
		}
		content, exists := world.Get[ScrollContent](entity)
		if sticky == parentSticky {
			if exists && (content.sticky != nil || content.parentSticky != nil) {
				content.sticky, content.parentSticky = nil, nil
				if content.Area == 0 {
					if transform := world.GetMut[scene.UITransform](entity); transform != nil {
						transform.Translation.X.Value, transform.Translation.Y.Value = content.X, content.Y
					}
					world.Remove[ScrollContent](entity)
				} else {
					world.SetIfChanged(entity, content)
				}
			}
			continue
		}
		if !exists {
			transform := world.GetRef[scene.UITransform](entity)
			if transform == nil {
				continue
			}
			content.X, content.Y = transform.Translation.X.Value, transform.Translation.Y.Value
		}
		content.sticky, content.parentSticky = sticky, parentSticky
		if exists {
			world.SetIfChanged(entity, content)
		} else {
			_ = world.Insert(entity, content)
		}
	}
	if len(state.StickyBindings) == 0 {
		state.StickyRoutes.clear()
	}
}

func htmlStickyTableInternal(display string) bool {
	switch display {
	case "table-row", "table-row-group", "table-header-group", "table-footer-group":
		return true
	}
	return false
}

// SyncScrollContent applies HTML scroll and sticky offsets after scroll input,
// before global transforms and hit-test bounds are synchronized.
func SyncScrollContent(world *ecs.World) {
	work := world.FrameValue[htmlStickyWorkTag, htmlStickyWork]()
	index := world.ScopeValue[htmlScrollContentTag, htmlScrollIndex[ScrollContent, htmlScrollPosition]]()
	contents, _ := world.ComponentValueRevision[ScrollContent]()
	transformWrites, transforms := world.ComponentValueRevision[scene.UITransform]()
	if index.rebuild(world, [7]uint64{contents, transforms}, work.revision) {
		for entity, access := range world.Query2[ScrollContent, scene.UITransform]().All {
			content := access.A.Get()
			entry := len(index.entries)
			index.entries = append(index.entries, htmlScrollEntry[ScrollContent]{entity: entity, value: content})
			index.add(content.Area, entry)
			index.addSticky(content.sticky, entry)
			index.addSticky(content.parentSticky, entry)
			clear(index.seen)
		}
		index.trim()
	} else {
		index.context = world.ChangesSince(index.lastRun - 1)
		index.dirtyOutput[scene.UITransform](world, transformWrites, 0, htmlScrollContentArea)
	}
	index.generation++
	work.generation++
	for area, group := range index.groups {
		snapshot := htmlScrollPosition{}
		if scroll := world.GetRef[scene.UIScroll](area); scroll != nil {
			snapshot = htmlScrollPosition{position: scroll.Position, contentSize: scroll.ContentSize, exists: true}
		}
		if !group.dirty && group.snapshot == snapshot {
			continue
		}
		group.snapshot, group.dirty = snapshot, false
		for _, entryIndex := range group.indices {
			entry := &index.entries[entryIndex]
			if entry.applied == index.generation {
				continue
			}
			entry.applied = index.generation
			content := entry.value
			offset, ok := scene.GetUIScrollOffset(world, content.Area)
			if !ok && content.sticky == nil && content.parentSticky == nil {
				continue
			}
			offset = offset.Add(content.sticky.resolve(world, work.generation)).Sub(content.parentSticky.resolve(world, work.generation))
			transform, _ := world.Get[scene.UITransform](entry.entity)
			transform.Translation.X.Value = content.X + offset.X()
			transform.Translation.Y.Value = content.Y + offset.Y()
			world.SetIfChanged(entry.entity, transform)
		}
	}
	index.outputs[0], _ = world.ComponentValueRevision[scene.UITransform]()
	index.lastRun = world.ChangeTick()
}

func (binding *htmlStickyBinding) resolve(world *ecs.World, generation uint64) mgl32.Vec2 {
	if binding == nil {
		return mgl32.Vec2{}
	}
	if binding.resolved == generation {
		return binding.offset
	}
	parent := binding.parent.resolve(world, generation)
	viewShift := parent.Sub(binding.scrollportSticky.resolve(world, generation))
	if scroll, ok := scene.GetUIScrollOffset(world, binding.area); ok {
		viewShift = viewShift.Add(scroll)
	}
	box := binding.box
	border, containing := box.BorderBox(), binding.viewport
	if binding.containing != nil {
		containing = binding.containing.Content
	}
	// The scroll container's containing block includes its scrollable content,
	// allowing a direct sticky child to remain stuck throughout the scroll.
	if binding.containing != nil && binding.containing == binding.scrollport {
		if scroll := world.GetRef[scene.UIScroll](binding.area); scroll != nil {
			containing.Width = max(containing.Width, float64(scroll.ContentSize.X())-binding.containing.Padding.Left-binding.containing.Padding.Right)
			containing.Height = max(containing.Height, float64(scroll.ContentSize.Y())-binding.containing.Padding.Top-binding.containing.Padding.Bottom)
		}
	}
	view := binding.viewport
	binding.offset = parent.Add(mgl32.Vec2{
		float32(htmlStickyAxis(border.X, border.Width, containing.X, containing.Width, box.Margin.Left, box.Margin.Right, view.X-float64(viewShift.X()), view.Width, box.Style.Left, box.Style.Right)),
		float32(htmlStickyAxis(border.Y, border.Height, containing.Y, containing.Height, box.Margin.Top, box.Margin.Bottom, view.Y-float64(viewShift.Y()), view.Height, box.Style.Top, box.Style.Bottom)),
	})
	binding.resolved = generation
	return binding.offset
}

// CSS Position 3 §3.4: reduce the end inset if the sticky view rectangle is
// smaller than the border box, then shift only as far as the position box fits
// in its containing block. Auto insets never cause a shift on their side.
func htmlStickyAxis(position, size, containingStart, containingSize, marginStart, marginEnd, viewStart, viewSize float64, start, end style.Value) float64 {
	if !style.IsLength(start) && !style.IsLength(end) {
		return 0
	}
	startInset := style.ResolveLength(start, viewSize)
	endInset := min(style.ResolveLength(end, viewSize), viewSize-startInset-size)
	target := position
	if style.IsLength(end) {
		target = min(target, viewStart+viewSize-endInset-size)
	}
	if style.IsLength(start) {
		target = max(target, viewStart+startInset)
	}
	marginStart = min(marginStart, position-containingStart)
	marginEnd = min(marginEnd, containingStart+containingSize-position-size)
	if target > position {
		return min(target-position, max(containingStart+containingSize-marginEnd-size-position, 0))
	}
	return max(target-position, min(containingStart+marginStart-position, 0))
}
