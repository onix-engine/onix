package ui

import (
	"strings"
	"testing"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/graphics"
	"github.com/onix-engine/onix/scene"
	notosanssc "github.com/onix-engine/onix/testdata/fonts/Noto_Sans_SC"
	htmlui "github.com/onix-engine/onix/ui/html"
)

func TestHTMLSelectPopupTextFollowsCSSColor(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	surface := htmlui.New(`<select id="mode" style="width:180px;height:28px"><option>Alpha</option><option selected>Beta</option></select>`)
	surface.Width, surface.Height, surface.Font = 320, 200, notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	bundle.Put(surface)
	owner := world.Spawn(bundle)
	sync := SyncSystem("")
	for _, fixture := range []struct {
		name, css string
		color     graphics.ColorRGBA
	}{
		{"dark", `select{background:#303030;color:#eeeeee}`, graphics.ColorFromRGB(0xee, 0xee, 0xee)},
		{"light", `select{background:#ffffff;color:#333333}`, graphics.ColorFromRGB(0x33, 0x33, 0x33)},
		{"foreground only", `select{background:#ffffff;color:#123456}`, graphics.ColorFromRGB(0x12, 0x34, 0x56)},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			world.GetMut[htmlui.Surface](owner).CSS = fixture.css
			if err := sync.Run(world); err != nil {
				t.Fatal(err)
			}
			control := htmlControlByID(t, world, owner, "mode")
			SetHTMLSelectOpen(world, control, true)
			if err := sync.Run(world); err != nil {
				t.Fatal(err)
			}
			runtime := world.GetRef[htmlSelectorRuntime](control)
			var labels [2]ecs.Entity
			for entity, access := range world.Query2[Relation[scene.Text2D], scene.Text2D]().All {
				for index, item := range runtime.Items {
					if access.A.Ref().Owner == item {
						labels[index] = entity
					}
				}
			}
			for index, label := range labels {
				text := world.GetRef[scene.Text2D](label)
				want := fixture.color
				if index == 1 {
					want = graphics.White
				}
				if text == nil || text.Color != want {
					t.Fatalf("option %d text = %+v, want color %v", index, text, want)
				}
			}
			world.GetMut[scene.InputState2D](runtime.Items[0]).Hovered = true
			if err := sync.Run(world); err != nil {
				t.Fatal(err)
			}
			if text := world.GetRef[scene.Text2D](labels[0]); text == nil || text.Color != graphics.White {
				t.Fatalf("highlighted option text = %+v, want white", text)
			}
			world.GetMut[scene.InputState2D](runtime.Items[0]).Hovered = false
			if err := sync.Run(world); err != nil {
				t.Fatal(err)
			}
			if text := world.GetRef[scene.Text2D](labels[0]); text == nil || text.Color != fixture.color {
				t.Fatalf("unhighlighted option text = %+v, want color %v", text, fixture.color)
			}
		})
	}
}

func TestHTMLSelectRefreshRemovesAllPopupElements(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	surface := htmlui.New(`<select id="mode" style="width:180px;height:28px"><option id="first">Alpha</option><option>Beta</option></select>`)
	surface.Width, surface.Height, surface.Font = 320, 200, notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	bundle.Put(surface)
	owner := world.Spawn(bundle)
	sync := SyncSystem("")
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	control := htmlControlByID(t, world, owner, "mode")
	node, _ := htmlui.NodeByID(world, owner, "mode")
	option, _ := htmlui.NodeByID(world, owner, "first")
	for _, step := range []string{"resize width", "resize height", "option text"} {
		t.Run(step, func(t *testing.T) {
			SetHTMLSelectOpen(world, control, true)
			runtime := world.GetRef[htmlSelectorRuntime](control)
			state := world.GetMut[scene.InputState2D](runtime.Items[0])
			state.Hovered, state.Pressed = true, true
			if err := sync.Run(world); err != nil {
				t.Fatal(err)
			}
			// Include every generated entity, including the highlighted option's
			// background, independently of the runtime's teardown list.
			var previous []ecs.Entity
			for entity, access := range world.Query[htmlSelectorElement]().All {
				if access.Ref().Owner == control {
					previous = append(previous, entity)
				}
			}
			switch step {
			case "resize width":
				node.SetAttribute("style", "width:220px;height:28px")
			case "resize height":
				node.SetAttribute("style", "width:220px;height:32px")
			case "option text":
				option.SetText("Changed")
			}
			if err := sync.Run(world); err != nil {
				t.Fatal(err)
			}
			if htmlControlByID(t, world, owner, "mode") != control {
				t.Fatal("refresh replaced the select control")
			}
			for _, entity := range previous {
				if world.IsAlive(entity) {
					t.Errorf("popup refresh left generated entity %d alive (option background: %v)", entity, world.Has[Part[BodyPart]](entity))
				}
			}
			count := 0
			for _, access := range world.Query[htmlSelectorElement]().All {
				if access.Ref().Owner == control {
					count++
				}
			}
			if count != len(previous) {
				t.Errorf("popup entity count grew from %d to %d", len(previous), count)
			}
		})
	}
}

func TestHTMLSelectPopupRetainsViewportPositionOnRefresh(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	world.InsertResource(scene.ViewportSize{Width: 320, Height: 320})
	surface := htmlui.New(`<select id="mode" style="width:180px;height:28px"><option id="first">First</option>` +
		strings.Repeat(`<option>Other</option>`, 30) + `</select>`)
	surface.Width, surface.Height, surface.Font = 320, 320, notosanssc.NotoSansSCRegular
	surfaces := world.NewSpawner1[htmlui.Surface]()
	owner := surfaces.Spawn(surface)
	sync := SyncSystem("")
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	control := htmlControlByID(t, world, owner, "mode")
	if err := scene.SyncUIGlobalTransformSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	SetHTMLSelectOpen(world, control, true)
	previous := world.GetRef[htmlSelectorRuntime](control).List
	world.GetMut[scene.UIScroll](previous).Position[1] = -80

	option, _ := htmlui.NodeByID(world, owner, "first")
	option.SetText("Changed")
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	list := world.GetRef[htmlSelectorRuntime](control).List
	scroll := world.GetRef[scene.UIScroll](list)
	if list == previous || world.IsAlive(previous) || scroll == nil || scroll.Position[1] != -80 {
		t.Fatalf("popup refresh lost its scroll position: list=%v scroll=%+v", list, scroll)
	}
}
