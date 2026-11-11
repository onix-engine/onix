package html

import (
	"testing"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/input"
	"github.com/onix-engine/onix/scene"
	notosanssc "github.com/onix-engine/onix/testdata/fonts/Noto_Sans_SC"
)

func TestHTMLCursorUpdatesWithoutRebuildingControls(t *testing.T) {
	world := ecs.NewWorld()
	html := New(`<div id="panel"><textarea id="editor">code</textarea><input id="input" value="name"><button id="button">Button</button><span id="label">label</span></div><style>#panel{cursor:text}#panel:hover{cursor:col-resize}#button{cursor:pointer}#editor{cursor:auto}</style>`)
	html.Width, html.Height, html.Font = 600, 160, notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	bundle.Put(html)
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	var button ecs.Entity
	for entity, access := range world.Query[Control]().All {
		if access.Get().ID == "button" {
			button = entity
		}
	}
	if button == 0 {
		t.Fatal("missing button")
	}
	children := append([]ecs.Entity(nil), world.GetRef[htmlRenderState](owner).Children...)
	for _, hovered := range []bool{false, true, false} {
		SetControlHover(world, button, hovered)
		if err := Sync(world); err != nil {
			t.Fatal(err)
		}
		checked := 0
		for _, entity := range children {
			if !world.IsAlive(entity) {
				t.Fatal("cursor change rebuilt the document")
			}
			child := world.GetRef[htmlChild](entity)
			if child == nil || child.Node == nil {
				continue
			}
			node := child.Node
			if node.Parent != nil && node.Data != "textarea" && node.Data != "input" && node.Data != "button" && node.Data != "span" {
				node = node.Parent
			}
			id, _ := node.Attr("id")
			want := input.CursorText
			if hovered {
				want = input.CursorEWResize
			}
			switch id {
			case "button":
				want = input.CursorPointer
			case "editor":
				want = input.CursorText
			case "input", "label", "panel":
			default:
				continue
			}
			if target := world.GetRef[scene.InputTarget2D](entity); target != nil {
				checked++
				if target.Cursor != want {
					t.Errorf("hover=%v id=%s cursor=%s, want %s", hovered, id, target.Cursor, want)
				}
			}
			if blocker := world.GetRef[scene.InputBlocker2D](entity); blocker != nil {
				checked++
				if blocker.Cursor != want {
					t.Errorf("hover=%v id=%s blocker cursor=%s, want %s", hovered, id, blocker.Cursor, want)
				}
			}
		}
		if checked == 0 {
			t.Fatal("no cursor hit areas checked")
		}
	}
}
