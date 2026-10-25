package html

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/html/layout"
	"github.com/diiyw/onix/html/style"
	"github.com/diiyw/onix/scene"
)

func htmlStyleVisible(computed *style.ComputedStyle) bool {
	return computed != nil && computed.Visibility != style.Keyword("hidden") && computed.Visibility != style.Keyword("collapse")
}

// Clips and composited groups carry descendants whose visibility can override
// the ancestor's. Only the element's own primitives inherit its hidden state.
func syncHTMLCSSVisibility(world *ecs.World, entity ecs.Entity, computed *style.ComputedStyle) {
	visible := htmlStyleVisible(computed)
	if scroll := ecs.GetMut[scene.UIScroll](world, entity); scroll != nil {
		scroll.DisableInput = !visible
	}
	if visible || ecs.Has[htmlOverflowClip](world, entity) || ecs.Has[htmlOpacitySurface](world, entity) {
		return
	}
	if visibility := ecs.GetMut[scene.Visibility](world, entity); visibility != nil {
		visibility.Visible = false
	}
}

func htmlOwnVisiblePaintBounds(box *layout.Box) layout.Rect {
	var bounds layout.Rect
	if htmlStyleVisible(box.Style) {
		bounds = box.BorderBox()
	}
	for _, fragment := range box.TextFragments {
		if htmlStyleVisible(fragment.Style) {
			bounds = htmlUnionPaintBounds(bounds, fragment.Rect)
		}
	}
	return bounds
}

// Adopt style instances during emission and composite eligibility checks. The
// generation avoids a separate tree walk and repeated fragment restyling when
// a compositor visits descendants before their individual entities are emitted.
func restyleHTMLVisualBox(state *htmlRenderState, box *layout.Box) {
	if rendered := state.BoxRenderings[box.Node]; rendered != nil {
		if rendered.StyleGeneration == state.RenderGeneration {
			return
		}
		rendered.StyleGeneration = state.RenderGeneration
	}
	if state.PendingVisual {
		box.Restyle(state.Styles)
	}
	box.UpdateInlineBackgrounds(state.Styles)
}
