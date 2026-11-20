package html

import (
	"strings"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/html/layout"
	"github.com/onix-engine/onix/html/style"
	"github.com/onix-engine/onix/scene"
	uinode "github.com/onix-engine/onix/scene/node"
	onixtext "github.com/onix-engine/onix/text"
)

func htmlPassivePaint(computed *style.ComputedStyle, passThrough bool) bool {
	return computed != nil && (passThrough || computed.PointerEvents == style.Keyword("none"))
}

func htmlPaintListIsolated(computed *style.ComputedStyle) bool {
	return computed.Opacity != style.Number(1) || computed.AnimatedOpacity || len(computed.Filters) != 0
}

// A leaf can use the surface transform when HTML has no independent coordinate
// space for its visual. Inline descendants and editable targets keep their own
// paint and input geometry.
func emitHTMLPaintListBox(world *ecs.World, owner ecs.Entity, state *htmlRenderState, rendered *htmlBoxRendering, window htmlPaintWindow, regular, bold *onixtext.Font, dpi float64, stack int, passThrough bool) bool {
	box := rendered.Box
	computed := box.Style
	if !htmlPassivePaint(computed, passThrough) || window.clipX || window.clipY || window.unbounded || window.noPaintList || htmlOverflowClipsStyle(computed) ||
		box.Node == nil || box.Node.Type != uinode.ElementNode || box.Node.Namespace != uinode.HTML || box.Node.Data == "img" ||
		len(box.Children) != 0 || len(box.ImageFragments) != 0 || len(box.ControlFragments) != 0 || len(box.InlineBackgrounds) != 0 ||
		len(box.TextFragments) > 1 || htmlPropagatesCanvasBackground(box) ||
		computed.AnimatedPaint != 0 || len(computed.BoxShadow) != 0 ||
		htmlNeedsRasterPaint(computed) || computed.Background.Image != "" || len(computed.ExtraBackgroundLayers) != 0 {
		return false
	}
	if _, focusable := htmlFocusableFromNode(owner, box.Node); focusable || isHTMLControlNode(box.Node) ||
		state.NodeEvents[box.Node].handlesPointer() {
		return false
	}
	rect := box.BorderBox()
	if rect.Width <= 0 || rect.Height <= 0 {
		return false
	}
	shape := htmlShape(computed, rect.Width, rect.Height)
	if _, _, uniform := htmlUniformBorder(computed, rect.Width); !uniform && htmlHasVisibleBorder(computed, rect.Width) {
		return false
	}
	if shape.Fill.A <= 0 && shape.Border.A <= 0 {
		shape = scene.Shape2D{}
	}
	paint := scene.UIBoxPaint2D{
		Stack: stack, Order: box.Node.Order, Shape: shape, X: float32(rect.X), Y: float32(rect.Y),
		Width: float32(rect.Width), Height: float32(rect.Height),
	}
	if len(box.TextFragments) == 1 {
		fragment := &box.TextFragments[0]
		if fragment.Node == nil || fragment.Node.Type != uinode.TextNode || fragment.Node.Parent != box.Node ||
			!htmlPassivePaint(fragment.Style, passThrough) || strings.TrimSpace(fragment.Text) == "" ||
			len(fragment.Glyphs) == 0 || fragment.Style.TextDecorationLine == style.Keyword("line-through") ||
			fragment.Style.AnimatedPaint != 0 || state.NodeEvents[fragment.Node].handlesPointer() {
			return false
		}
		font := htmlFontForControl(world, 0, regular, bold, fragment.Style)
		if font == nil {
			return false
		}
		lcd, alpha := htmlTextPaint(state.Styles, fragment.Node)
		if alpha != 1 {
			return false
		}
		paint.Text = scene.UIStaticText2D{
			Font: font, FontSize: fragmentFontSize(fragment.Style), DPI: dpi, Color: htmlColor(fragment.Color()), LCD: lcd,
			X: float32(fragment.Rect.X), Y: float32(fragment.Rect.Y),
			Layout: scene.TextPrelayout2D{
				Glyphs: fragment.Glyphs, OriginX: fragment.Rect.X, OriginY: fragment.Rect.Y,
				Width: float32(fragment.Rect.Width), Height: float32(fragment.Rect.Height), Revision: rendered.GeometryHash,
			},
		}
	}
	if paint.Shape.Kind == 0 && paint.Text.Font == nil {
		return false
	}
	if state.PaintList == nil {
		state.PaintList = new(scene.UIPaintList2D)
	}
	rendered.PaintSlot = state.PaintList.SetBox(rendered.PaintSlot, paint)
	state.PaintListDirty = true
	return true
}

func removeHTMLPaintListBox(state *htmlRenderState, rendered *htmlBoxRendering) {
	if rendered.PaintSlot != 0 {
		state.PaintList.RemoveBox(rendered.PaintSlot)
		rendered.PaintSlot = 0
		state.PaintListDirty = true
	}
}

func publishHTMLPaintList(world *ecs.World, owner ecs.Entity, state *htmlRenderState) {
	if !state.PaintListDirty {
		return
	}
	state.PaintListDirty = false
	if state.PaintList.Len() == 0 {
		world.Remove[*scene.UIPaintList2D](owner)
		state = world.GetMut[htmlRenderState](owner)
		state.PaintList = nil
		return
	}
	if relocated := state.PaintList.Compact(); relocated != nil {
		for _, box := range state.BoxRenderings {
			box.PaintSlot = relocated[box.PaintSlot]
		}
	}
	if binding := world.GetMut[*scene.UIPaintList2D](owner); binding == nil {
		_ = world.Insert(owner, state.PaintList)
	}
}

func htmlPaintListStyleChanged(state *htmlRenderState, node *uinode.Node, box *layout.Box, passThrough bool) bool {
	rendered := state.BoxRenderings[box.Node]
	if rendered.PaintSlot != 0 {
		return true
	}
	computed := state.Styles[node]
	if box.Node != node || computed == nil {
		return false
	}
	return len(box.Children) == 0 && (htmlPassivePaint(box.Style, passThrough) || htmlPassivePaint(computed, passThrough)) ||
		(state.PaintList != nil || rendered.paintClip().contents.unbounded || rendered.paintClip().contents.noPaintList) && (htmlPaintStyleMoves(box.Style) != htmlPaintStyleMoves(computed) ||
			htmlPaintListIsolated(box.Style) != htmlPaintListIsolated(computed))
}

// A binding needs a generated transform or clip immediately. Reconcile once
// when the surface first acquires one, then use the existing local binding path.
func syncHTMLPaintListBinding(world *ecs.World, owner ecs.Entity, state *htmlRenderState) bool {
	surface := world.GetRef[Surface](owner)
	if surface == nil {
		return false
	}
	state.requestVisual()
	if _, err := renderHTML(world, owner, *surface); err != nil {
		return false
	}
	state = world.GetMut[htmlRenderState](owner)
	bindHTMLStickyContent(world, state)
	publishHTMLPaintList(world, owner, state)
	return true
}
