package ui

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/scene"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	htmlui "github.com/diiyw/onix/ui/html"
)

func TestHTMLSubmitDispatchesOnceToOwningForm(t *testing.T) {
	for _, test := range []struct {
		name, source string
		keyboard     bool
		want         int
	}{
		{"ancestor", `<form data-ref="form"><button id="send" type="submit">Send</button></form>`, false, 1},
		{"external", `<form id="form-owner" data-ref="form"></form><button id="send" type="submit" form="form-owner">Send</button>`, false, 1},
		{"implicit", `<form data-ref="form"><input id="field"><button id="send" type="submit">Send</button></form>`, true, 1},
		{"without owner", `<form data-ref="form"></form><button id="send" type="submit">Send</button>`, false, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			world := ecs.NewWorld()
			RegisterRuntimeFeature[HTMLRenderer](world)
			ecs.InsertResource(world, input.NewState())
			calls := 0
			definition, err := htmlui.Define(htmlui.NewTemplate(test.source), "", htmlui.Behavior[struct{}, struct{}]{
				Render: func(_ htmlui.RenderContext[struct{}, struct{}], view *htmlui.View) {
					view.Event("form", htmlui.EventSubmit)
				},
				HandleEvent: func(c *htmlui.ComponentEventContext[struct{}, struct{}]) {
					if c.Ref != "form" || c.Event.Kind != htmlui.EventSubmit || !c.Event.Bubbles || !c.Event.Cancelable {
						t.Fatalf("invalid form event: %+v", c.Event)
					}
					calls++
					c.PreventDefault()
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			page := htmlui.New(`<main></main>`)
			page.Width, page.Height = 400, 200
			page.Font = notosanssc.NotoSansSCRegular
			bundle := ecs.NewBundle(5)
			ecs.Put(&bundle, scene.NewUITransform())
			ecs.Put(&bundle, scene.UIGlobalTransform{})
			ecs.Put(&bundle, scene.Visibility{Visible: true})
			ecs.Put(&bundle, scene.AnchorTopLeft)
			ecs.Put(&bundle, page)
			owner := world.Spawn(bundle)
			if _, err := htmlui.MountComponent(world, owner, definition, struct{}{}, struct{}{}, 1); err != nil {
				t.Fatal(err)
			}
			if err := SyncSystem("").Run(world); err != nil {
				t.Fatal(err)
			}
			if test.keyboard {
				ecs.WriteMessage(world, SubmittedEvent[string]{Entity: htmlControlByID(t, world, owner, "field"), Value: "typed"})
			} else {
				ecs.WriteMessage(world, scene.InputEvent{Entity: htmlControlByID(t, world, owner, "send"), Kind: scene.InputEventTap})
			}
			if err := UpdateSystem("").Run(world); err != nil {
				t.Fatal(err)
			}
			if calls != test.want {
				t.Fatalf("submit callbacks=%d, want %d", calls, test.want)
			}
			reader := ecs.MessageReader[htmlui.Event]{}
			submits := 0
			for event := range reader.All(world) {
				if event.Kind == htmlui.EventSubmit {
					submits++
					if test.want == 1 && !event.DefaultPrevented {
						t.Fatal("form cancellation was lost")
					}
				}
			}
			if submits != 1 {
				t.Fatalf("submit messages=%d, want 1", submits)
			}
		})
	}
}
