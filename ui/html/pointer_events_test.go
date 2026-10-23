package html

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/scene"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
)

func TestHTMLPointerEventsUpdateWithoutRebuildingControls(t *testing.T) {
	world := ecs.NewWorld()
	html := New(`<div id="panel"><button id="inherited">Inherited</button><button id="enabled" style="pointer-events:auto">Enabled</button><select id="select"><option>One</option></select><span id="callback" tabindex="0">Callback</span></div><style>#panel{pointer-events:none}#panel:hover{pointer-events:auto}</style>`)
	html.Width, html.Height, html.Font = 600, 160, notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	var enabled ecs.Entity
	for e, access := range ecs.Query[Control](world).All {
		c := access.Get()

		if c.Owner == owner && c.ID == "enabled" {
			enabled = e
		}
	}
	if enabled == 0 {
		t.Fatal("missing enabled button")
	}
	state := ecs.GetMut[htmlRenderState](world, owner)
	children := append([]ecs.Entity(nil), state.Children...)
	for _, hovered := range []bool{false, true, false} {
		if !SetControlHover(world, enabled, hovered) {
			t.Fatal("set hover")
		}
		if err := Sync(world); err != nil {
			t.Fatal(err)
		}
		targets, blockers := 0, 0
		for _, e := range children {
			if !world.IsAlive(e) {
				t.Fatal("pointer-events change rebuilt the document")
			}
			child, ok := ecs.Get[htmlChild](world, e)
			if !ok || child.Node == nil {
				continue
			}
			node := child.Node
			if node.Type == dom.TextNode && node.Parent != nil { // Text fragments inherit their element's pointer behavior.
				node = node.Parent
			}
			id, _ := node.Attr("id")
			wantPassThrough := !hovered && id != "enabled"
			if target, ok := ecs.Get[scene.InputTarget2D](world, e); ok {
				targets++
				if target.PassThrough != wantPassThrough {
					t.Errorf("hover=%v %s target pass-through=%v, want %v", hovered, id, target.PassThrough, wantPassThrough)
				}
			}
			if blocker, ok := ecs.Get[scene.InputBlocker2D](world, e); ok {
				blockers++
				if blocker.PassThrough != wantPassThrough {
					t.Errorf("hover=%v %s blocker pass-through=%v, want %v", hovered, id, blocker.PassThrough, wantPassThrough)
				}
			}
		}
		if targets == 0 || blockers == 0 {
			t.Fatal("missing input targets or blockers")
		}
	}
}
