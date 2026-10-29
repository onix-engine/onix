package ui

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/scene"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	htmlui "github.com/diiyw/onix/ui/html"
)

func TestHTMLHoverKeepsSiblingInputEntityAndValue(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	world.InsertResource(input.NewState())

	html := htmlui.New(`<input id="name" value="Onix"><button id="send">Send</button><style>#send:hover{background:#445566}</style>`)
	html.Width, html.Height = 320, 80
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	bundle.Put(html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	field := htmlControlByID(t, world, owner, "name")
	button := htmlControlByID(t, world, owner, "send")
	if control := world.GetMut[htmlui.Control](field); control != nil {
		control.Value = "edited"
	}
	if text := world.GetMut[TextField](field); text != nil {
		text.Value = "edited"
	}

	world.WriteMessage(scene.InputEvent{Entity: button, Kind: scene.InputEventEnter})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if !world.IsAlive(field) {
		t.Fatal("hover rebuilt the HTML tree and despawned the input control")
	}
	control, ok := world.Get[htmlui.Control](field)
	if !ok || control.Value != "edited" {
		t.Fatalf("hover changed input control value: %#v, ok=%v", control, ok)
	}
	text, ok := world.Get[TextField](field)
	if !ok || text.Value != "edited" {
		t.Fatalf("hover changed text field value: %#v, ok=%v", text, ok)
	}
}

func TestHTMLHoverKeepsInputPlaceholderColor(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	uiInputState := input.NewState()
	world.InsertResource(uiInputState)

	html := htmlui.New(`<input id="search" placeholder="搜索资源"><button id="item">世界观.md</button><style>input{color:#dbe5ee;placeholder-color:#757575}#item:hover{background:#445566}</style>`)
	html.Width, html.Height = 320, 80
	html.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	bundle.Put(html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	search := htmlControlByID(t, world, owner, "search")
	item := htmlControlByID(t, world, owner, "item")
	placeholderColor := func() graphics.ColorRGBA {
		var color graphics.ColorRGBA
		found := false
		for _, access := range world.Query2[htmlui.ControlText, scene.Text2D]().All {
			link := access.A.Get()
			text := access.B.Get()

			if link.Control == search {
				color, found = text.Color, true
				break
			}
		}
		if !found {
			t.Fatal("search input placeholder text was not generated")
		}
		return color
	}
	want := graphics.ColorFromRGBA(117, 117, 117, 255)
	if got := placeholderColor(); got != want {
		t.Fatalf("initial placeholder color = %+v, want %+v", got, want)
	}

	world.WriteMessage(scene.InputEvent{Entity: item, Kind: scene.InputEventEnter})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if got := placeholderColor(); got != want {
		t.Fatalf("hovered placeholder color = %+v, want %+v", got, want)
	}

	world.WriteMessage(scene.InputEvent{Entity: item, Kind: scene.InputEventLeave})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if got := placeholderColor(); got != want {
		t.Fatalf("unhovered placeholder color = %+v, want %+v", got, want)
	}
}
