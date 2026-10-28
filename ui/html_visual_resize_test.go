package ui

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	htmlui "github.com/diiyw/onix/ui/html"
)

func TestHTMLTextVisualRolesSurviveResize(t *testing.T) {
	for _, editable := range []bool{false, true} {
		name, markup := "textarea", `<textarea id="editor">Source</textarea>`
		if editable {
			name, markup = "contenteditable", `<div id="editor" contenteditable>Source</div>`
		}
		t.Run(name, func(t *testing.T) {
			world := ecs.NewWorld()
			RegisterRuntimeFeature[HTMLRenderer](world)
			html := htmlui.New(`<style>#editor{width:300px;height:240px;margin:0;padding:0;border:0;font-size:14px;line-height:20px}</style>` + markup)
			html.Width, html.Height, html.Font = 400, 400, notosanssc.NotoSansSCRegular
			bundle := ecs.NewBundle(1)
			ecs.Put(&bundle, html)
			owner := world.Spawn(bundle)
			sync := func() {
				t.Helper()
				if err := SyncSystem("").Run(world); err != nil {
					t.Fatal(err)
				}
			}
			sync()
			var control ecs.Entity
			if editable {
				control = htmlFocusableByID(t, world, owner, "editor")
			} else {
				control = htmlControlByID(t, world, owner, "editor")
			}
			field := ecs.GetMut[TextField](world, control)
			field.Focused, field.Caret, field.Anchor = true, 3, 1
			node, _ := htmlui.NodeByID(world, owner, "editor")
			var identities [5]ecs.Entity
			for _, size := range []string{"240px", "80px", "320px", "100px", "240px"} {
				node.SetAttribute("style", "height:"+size)
				sync()
				caret := ecs.Entity(0)
				verify := func(entity ecs.Entity, kind htmlui.ControlVisualKind, selection bool) {
					t.Helper()
					isCaret := !selection && kind == htmlui.ControlVisualCaret
					if ecs.Has[Part[CaretPart]](world, entity) != isCaret || ecs.Has[Part[SelectionPart]](world, entity) != selection {
						t.Fatalf("height %s: visual kind=%d selection=%v retained another runtime role", size, kind, selection)
					}
					if selection {
						return
					}
					if previous := identities[kind]; previous != 0 && previous != entity {
						t.Fatalf("height %s: visual kind=%d changed identity", size, kind)
					}
					identities[kind] = entity
					if isCaret {
						caret = entity
					}
				}
				for entity, access := range ecs.Query[htmlui.ControlVisual](world).All {
					if visual := access.Get(); visual.Control == control {
						verify(entity, visual.Kind, visual.Selection)
					}
				}
				for entity, access := range ecs.Query[htmlui.EditableVisual](world).All {
					if visual := access.Get(); visual.Editable == control {
						verify(entity, htmlui.ControlVisualCaret, visual.Selection)
					}
				}
				visibility, _ := ecs.Get[scene.Visibility](world, caret)
				bounds, _ := ecs.Get[scene.Size2D](world, caret)
				if caret == 0 || !visibility.Visible || bounds.Width <= 0 || bounds.Height <= 0 {
					t.Fatalf("height %s: caret hidden or empty: visible=%v bounds=%+v", size, visibility.Visible, bounds)
				}
			}
		})
	}
}
