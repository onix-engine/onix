package ui

import (
	"math"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/scene"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	htmlui "github.com/diiyw/onix/ui/html"
)

func TestHTMLStickyButtonFollowsWheelAndPicking(t *testing.T) {
	world := newUIWorld(t)
	html := htmlui.New(`<div id="scroll" style="width:200px;height:100px;overflow:auto"><div style="height:40px"></div><button id="target" style="position:sticky;top:10px;width:100px;height:30px;margin:0;overflow:hidden">Sticky</button><div style="height:1000px"></div></div>`)
	html.Width, html.Height, html.Font = 300, 200, notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	bundle.Put(html)
	owner := world.Spawn(bundle)
	if err := runUIFrame(world); err != nil {
		t.Fatal(err)
	}
	button := htmlControlByID(t, world, owner, "target")
	scrollNode, _ := htmlui.NodeByID(world, owner, "scroll")
	area, _ := scrollNode.ScrollContainer()
	state, _ := world.GetResource[*input.State]()
	state.MoveMouse(150, 80)
	state.ScrollMouse(0, -2)
	if err := runUIFrame(world); err != nil {
		t.Fatal(err)
	}
	if scroll := world.GetRef[scene.UIScroll](area); scroll.Position.Y() != -96 {
		t.Fatalf("wheel offset=%v, want -96", scroll.Position.Y())
	}
	transform := world.GetRef[scene.UIGlobalTransform](button)
	if math.Abs(float64(transform.Matrix[13]-10)) > .001 {
		t.Fatalf("sticky button y=%v, want 10", transform.Matrix[13])
	}
	state.EndFrame()
	state.PressMouse(input.MouseLeft, 20, 20)
	if err := runUIUpdate(world); err != nil {
		t.Fatal(err)
	}
	capture, ok := world.GetResource[*scene.InputCapture2D]()
	if !ok || capture.Receiver != button {
		t.Fatalf("sticky button did not receive input at its painted location: %+v", capture)
	}
}
