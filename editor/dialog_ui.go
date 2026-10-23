package editor

import (
	"strings"

	"github.com/diiyw/onix/ecs"
	editorresources "github.com/diiyw/onix/editor/resources"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene"
	onixtext "github.com/diiyw/onix/text"
	engineui "github.com/diiyw/onix/ui"
	htmlui "github.com/diiyw/onix/ui/html"
)

type editorDialogUI struct {
	font                 *onixtext.Font
	rootRect             rect
	htmlDialogs          map[string]ecs.Entity
	htmlDialogComponents map[string]ecs.Entity
	htmlDialogFrames     map[string]*editorDialogFrame
	htmlDialogPanels     map[string]rect
	active               map[ecs.Entity]struct{}
}

const (
	editorButtonIconSize float32 = 16

	// Dialog checkboxes keep a fixed glyph box regardless of the row width.
	editorDialogCheckboxSize float32 = 16

	editorDialogCornerRadius = engineui.CornerRadius(10)
	// The dialog shell document is authored one offset below the stack index
	// its controls are laid out at.
	editorHTMLDialogStackOffset = 16
	// The HTML renderer multiplies the outermost CSS z-index by 1e9 when it
	// maps a box onto the flat UI stack, so a bar layer with z-index:30 tree
	// eyes (30e9) would paint over a dialog shell whose base stack is only
	// ~66. Scale dialog bases above the largest z-index used by any editor
	// HTML layer (550 in the tilebar) to keep modals on top.
	editorHTMLDialogStackStride = 1_000_000_000_000
)

func editorHTMLDialogStack(stackIndex int) int {
	return (stackIndex - editorHTMLDialogStackOffset) * editorHTMLDialogStackStride
}

// htmlDialog creates the retained HTML document used by every editor dialog.
// Markup and interactive elements live in the resource; Go only synchronizes
// data, geometry and event state on the retained DOM nodes.
func (u *editorDialogUI) htmlDialog(world *ecs.World, name string, panel rect, stackIndex int) ecs.Entity {
	entity, ok := u.htmlDialogs[name]
	if !ok || !world.IsAlive(entity) {
		html := htmlui.New(editorresources.DialogHTML(name))
		html.Font, html.FontSize = u.font, 14
		html.Width, html.Height = u.rootRect.W, u.rootRect.H
		html.StackIndex = editorHTMLDialogStack(stackIndex)
		html.PassThrough = false
		bundle := ecs.NewBundle(6)
		ecs.Put(&bundle, scene.UITransformFromXY(0, 0))
		ecs.Put(&bundle, scene.UIGlobalTransform{})
		ecs.Put(&bundle, scene.Visibility{Visible: true})
		ecs.Put(&bundle, scene.AnchorTopLeft)
		ecs.Put(&bundle, scene.UIStackIndex(editorHTMLDialogStack(stackIndex)))
		ecs.Put(&bundle, scene.Size2D{})
		ecs.Put(&bundle, html)
		entity = world.Spawn(bundle)
		_ = ecs.Insert(world, entity, engineui.Dialog{})
		shell := engineui.ColorSurface(editorTheme.panel)
		shell.Background.Radius = float32(editorDialogCornerRadius)
		background := scene.NewUIBackground2D(shell.Background)
		background.Alpha = 0
		_ = ecs.Insert(world, entity, background)
		_ = ecs.Insert(world, entity, scene.NewUIImage2D(nil))
		_ = ecs.Insert(world, entity, scene.NewUIBorder2D(scene.OutlinedRoundedRectShape2D(graphics.ColorRGBA{}, editorTheme.panelBorder, float32(editorDialogCornerRadius), 1)))
		_ = ecs.Insert(world, entity, scene.InputBlocker2D{Width: u.rootRect.W, Height: u.rootRect.H, PassThrough: false})
		if background := ecs.GetMut[scene.UIBackground2D](world, entity); background != nil {
			background.Alpha = 0
		}
		if image := ecs.GetMut[scene.UIImage2D](world, entity); image != nil {
			image.Alpha = 0
		}
		if border := ecs.GetMut[scene.UIBorder2D](world, entity); border != nil {
			border.Alpha = 0
		}
		u.htmlDialogs[name] = entity
		if name == "welcome" {
			// The welcome resource is the component template itself; hydrate it
			// so the retained page DOM remains addressable by the dialog code.
			definition, err := editorWelcomeDefinitions()
			if err != nil {
				panic(err)
			}
			component, err := htmlui.HydrateComponent(world, entity, definition, welcomeComponentProps{}, welcomeComponentState{}, 1)
			if err != nil {
				panic(err)
			}
			u.htmlDialogComponents[name] = component
		} else {
			frame := &editorDialogFrame{Elements: make(map[string]*editorDialogElement)}
			u.htmlDialogFrames[name] = frame
			definition, err := editorDialogDefinition(name)
			if err != nil {
				panic(err)
			}
			component, err := htmlui.HydrateComponent(world, entity, definition, editorDialogComponentProps{Frame: frame}, editorDialogComponentState{}, 1)
			if err != nil {
				panic(err)
			}
			u.htmlDialogComponents[name] = component
		}
		installEditorHTMLImages(world)
	}
	html := ecs.GetMut[htmlui.Surface](world, entity)
	html.Width, html.Height, html.Font = u.rootRect.W, u.rootRect.H, u.font
	html.StackIndex = editorHTMLDialogStack(stackIndex)
	if size := ecs.GetMut[scene.Size2D](world, entity); size != nil {
		// The HTML surface covers the whole editor window.  The dialog panel is
		// positioned inside that surface by CSS; using the panel dimensions here
		// makes the retained document viewport change whenever the modal is
		// rebuilt, which can clip/reflow its contents after input events.
		size.Width, size.Height = u.rootRect.W, u.rootRect.H
	}
	if index := ecs.GetMut[scene.UIStackIndex](world, entity); index != nil {
		*index = scene.UIStackIndex(editorHTMLDialogStack(stackIndex))
	}
	u.htmlDialogPanels[name] = panel
	if frame := u.htmlDialogFrames[name]; frame != nil {
		frame.reset(rect{W: u.rootRect.W, H: u.rootRect.H}, panel)
	}
	u.show(world, entity)
	return entity
}

func (u *editorDialogUI) hideInactiveHTMLDialogNodes(world *ecs.World) {
	for name, frame := range u.htmlDialogFrames {
		component, ok := u.htmlDialogComponents[name]
		if !ok || frame == nil {
			continue
		}
		if _, active := u.active[u.htmlDialogs[name]]; !active {
			continue
		}
		_ = htmlui.UpdateComponentProps[editorDialogComponentProps, editorDialogComponentState](world, component, editorDialogComponentProps{Frame: frame})
	}
}

func htmlDialogFieldID(dialog, name string) string {
	var b strings.Builder
	b.Grow(len(dialog) + len(name) + 13)
	b.WriteString("dialog-field-")
	for _, r := range dialog + "-" + name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

func (u *editorDialogUI) show(world *ecs.World, entity ecs.Entity) {
	if entity == 0 {
		return
	}
	u.active[entity] = struct{}{}
	if visibility := ecs.GetRef[scene.Visibility](world, entity); visibility != nil && visibility.Visible {
		return
	}
	if visibility := ecs.GetMut[scene.Visibility](world, entity); visibility != nil {
		visibility.Visible = true
	}
}

func (u *editorDialogUI) removeInactive(world *ecs.World) {
	// Closing a dialog only changes visibility; reopening reuses its DOM.
	for name, page := range u.htmlDialogs {
		if !world.IsAlive(page) {
			htmlui.UnmountComponent(world, u.htmlDialogComponents[name])
			delete(u.htmlDialogs, name)
			delete(u.htmlDialogComponents, name)
			delete(u.htmlDialogFrames, name)
			delete(u.htmlDialogPanels, name)
			continue
		}
		if _, active := u.active[page]; !active {
			if visibility := ecs.GetRef[scene.Visibility](world, page); visibility != nil && visibility.Visible {
				ecs.GetMut[scene.Visibility](world, page).Visible = false
			}
		}
	}
}

func (u *editorDialogUI) init() {
	if u.htmlDialogs != nil {
		return
	}
	u.htmlDialogs = make(map[string]ecs.Entity)
	u.htmlDialogComponents = make(map[string]ecs.Entity)
	u.htmlDialogFrames = make(map[string]*editorDialogFrame)
	u.htmlDialogPanels = make(map[string]rect)
	u.active = make(map[ecs.Entity]struct{})
}
