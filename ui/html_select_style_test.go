package ui

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	htmlui "github.com/diiyw/onix/ui/html"
)

func TestHTMLSelectInteractionPaletteUpdatesRetainedOptions(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	surface := htmlui.New(`<select id="mode" style="width:160px;color:#123456;background:#eeeeee"><option>First</option><option>Second</option></select>`)
	surface.Width, surface.Height, surface.Font = 320, 200, notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, surface)
	owner := world.Spawn(bundle)
	sync := SyncSystem("")
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	control := htmlControlByID(t, world, owner, "mode")
	item := ecs.GetRef[htmlSelectorRuntime](world, control).Items[1]
	body := findSurfacePart[BodyPart](world, item)
	for _, active := range []graphics.ColorRGBA{graphics.ColorFromRGB(60, 80, 100), graphics.ColorFromRGB(180, 200, 220)} {
		foreground := graphics.ColorFromRGB(30, 40, 50)
		_ = ecs.Insert(world, owner, HTMLSelectStyle{
			Item: NewStateStyle(SurfaceStyle{}).WithSelected(ColorSurface(active)).WithHovered(ColorSurface(active)).WithPressed(ColorSurface(active)),
			Text: NewStateStyle(graphics.ColorRGBA{}).WithSelected(foreground).WithHovered(foreground).WithPressed(foreground),
		})
		for _, state := range []ControlState{{}, {Selected: true}, {Hovered: true}, {Pressed: true}, {Selected: true, Disabled: true}} {
			SetControlSelected(world, item, state.Selected)
			SetControlDisabled(world, item, state.Disabled)
			_ = ecs.Insert(world, item, scene.InputState2D{Hovered: state.Hovered, Pressed: state.Pressed})
			// Sync visuals directly to preserve the interaction state under test.
			syncTextContent(world, DefaultTheme())
			view, _ := controlViewFor(world, item)
			resolved := controlSurface(world, item, &view, DefaultSkin(DefaultTheme()))
			applySurface(world, body, view.Width, view.Height, resolved)
			wantBackground, wantText := graphics.ColorFromRGB(238, 238, 238), graphics.ColorFromRGB(0x12, 0x34, 0x56)
			if !state.Disabled && (state.Selected || state.Hovered || state.Pressed) {
				wantBackground, wantText = active, foreground
			}
			if got := ecs.GetRef[scene.UIBackground2D](world, body).Shape.Fill; got != wantBackground {
				t.Fatalf("state %+v: background = %v, want %v", state, got, wantBackground)
			}
			labels := 0
			for _, access := range ecs.Query2[Relation[scene.Text2D], scene.Text2D](world).All {
				if access.A.Ref().Owner == item {
					if got := access.B.Ref().Color; got != wantText {
						t.Fatalf("state %+v: text = %v, want %v", state, got, wantText)
					}
					labels++
				}
			}
			if labels != 1 {
				t.Fatalf("option has %d labels, want 1", labels)
			}
		}
	}
}
