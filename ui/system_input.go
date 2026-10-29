package ui

import (
	"math"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func isContextMenuPick(world *ecs.World, entity ecs.Entity) bool {
	if entity == 0 {
		return false
	}
	return world.Has[ContextMenuItem](entity)
}

func closeContextMenus(world *ecs.World) {
	for _, access := range world.Query[ContextMenu]().All {
		menu := access.Mut()

		menu.Open = false
	}
}

func closeOtherSelectors(world *ecs.World, picked ecs.Entity) {
	keep := ecs.Entity(0)
	if world.Has[Selector](picked) {
		keep = picked
	} else if item, ok := world.Get[SelectorItem](picked); ok {
		keep = item.Owner
	}
	for entity, access := range world.Query[Selector]().All {
		selector := access.Mut()

		if entity != keep {
			selector.Open = false
		}
	}
}

func selectorItemHeight(selector *Selector, control *controlView) float32 {
	if selector != nil && selector.ItemHeight > 0 {
		return selector.ItemHeight
	}
	if control != nil {
		return control.Height
	}
	return 0
}

func selectorListHeight(selector *Selector, count int, itemHeight float32) float32 {
	contentHeight := itemHeight * float32(count)
	if selector == nil || selector.ListHeight <= 0 {
		return contentHeight
	}
	return min(selector.ListHeight, contentHeight)
}

func selectorPopupGap(world *ecs.World, entity ecs.Entity) float32 {
	if world.Has[htmlSelectorRuntime](entity) {
		return htmlSelectorPopupGap
	}
	return 0
}

func selectorListHorizontalLayout(world *ecs.World, entity ecs.Entity, width float32) (float32, float32) {
	viewport, ok := world.GetResource[scene.ViewportSize]()
	transform := world.GetRef[scene.UIGlobalTransform](entity)
	if !ok || viewport.Width <= 0 || transform == nil {
		return width, 0
	}
	direction := transform.Matrix.Mul4x1(mgl32.Vec4{1, 0, 0, 0})
	if direction.X() <= 1e-6 || abs32(direction.Y()) > 1e-6 {
		return width, 0
	}
	origin := transform.Matrix.Mul4x1(mgl32.Vec4{0, 0, 0, 1})
	width = min(width, viewport.Width/direction.X())
	x := max(-origin.X(), min(float32(0), viewport.Width-origin.X()-width*direction.X()))
	return width, x / direction.X()
}

func selectorListLayout(world *ecs.World, entity ecs.Entity, selector *Selector, control *controlView, count int) (float32, bool) {
	itemHeight := selectorItemHeight(selector, control)
	height := selectorListHeight(selector, count, itemHeight)
	upward := selector != nil && selector.Upward
	above, below, ok := selectorViewportSpace(world, entity, control)
	if !ok {
		return height, upward
	}
	gap := selectorPopupGap(world, entity)
	above, below = max(above-gap, 0), max(below-gap, 0)
	preferred, other := below, above
	if upward {
		preferred, other = above, below
	}
	if height > preferred && (height <= other || other > preferred) {
		upward = !upward
		preferred = other
	}
	return min(height, max(preferred, 0)), upward
}

func selectorViewportSpace(world *ecs.World, entity ecs.Entity, control *controlView) (float32, float32, bool) {
	viewport, viewportOK := world.GetResource[scene.ViewportSize]()
	transform := world.GetRef[scene.UIGlobalTransform](entity)
	if (!viewportOK) || (viewport.Width <= 0) || (viewport.Height <= 0) || (transform == nil) || (control == nil) {
		return 0, 0, false
	}
	direction := transform.Matrix.Mul4x1(mgl32.Vec4{0, 1, 0, 0})
	if abs32(direction.X())+abs32(direction.Y()) <= 1e-6 {
		return 0, 0, false
	}
	topLeft := transform.Matrix.Mul4x1(mgl32.Vec4{0, 0, 0, 1})
	topRight := transform.Matrix.Mul4x1(mgl32.Vec4{control.Width, 0, 0, 1})
	bottomLeft := transform.Matrix.Mul4x1(mgl32.Vec4{0, control.Height, 0, 1})
	bottomRight := transform.Matrix.Mul4x1(mgl32.Vec4{control.Width, control.Height, 0, 1})
	below := min(
		selectorRaySpace(bottomLeft, direction.X(), direction.Y(), viewport.Width, viewport.Height),
		selectorRaySpace(bottomRight, direction.X(), direction.Y(), viewport.Width, viewport.Height),
	)
	above := min(
		selectorRaySpace(topLeft, -direction.X(), -direction.Y(), viewport.Width, viewport.Height),
		selectorRaySpace(topRight, -direction.X(), -direction.Y(), viewport.Width, viewport.Height),
	)
	return above, below, true
}

func selectorRaySpace(origin mgl32.Vec4, dx, dy, width, height float32) float32 {
	space := float32(math.Inf(1))
	for _, axis := range [][3]float32{{origin.X(), dx, width}, {origin.Y(), dy, height}} {
		position, direction, limit := axis[0], axis[1], axis[2]
		if position < 0 || position > limit {
			return 0
		}
		switch {
		case direction > 0:
			space = min(space, (limit-position)/direction)
		case direction < 0:
			space = min(space, -position/direction)
		}
	}
	if math.IsInf(float64(space), 1) {
		return 0
	}
	return max(space, 0)
}

func abs32(value float32) float32 {
	if value < 0 {
		return -value
	}
	return value
}

func hierarchyVisible(world *ecs.World, entity ecs.Entity) bool {
	for remaining := world.EntityCount() + 1; entity != 0 && remaining > 0; remaining-- {
		if visibility := world.GetRef[scene.Visibility](entity); visibility != nil && !visibility.Visible {
			return false
		}
		parent, ok := world.Get[scene.Parent](entity)
		if !ok {
			return true
		}
		entity = parent.Entity
	}
	return entity == 0
}

func setTabSubtreeVisible(world *ecs.World, parentChildren []tabParentChild, childRanges map[ecs.Entity]tabChildRange, entity ecs.Entity, visible bool) {
	setVisible(world, entity, visible)
	for _, relation := range tabChildren(parentChildren, childRanges, entity) {
		setTabSubtreeVisible(world, parentChildren, childRanges, relation.child, visible)
	}
}

func inputCoordsForUI(screenX, screenY float32) (float32, float32) {
	return screenX, screenY
}
