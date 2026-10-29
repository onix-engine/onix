package ui

import (
	"math"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	htmlui "github.com/diiyw/onix/ui/html"
)

func TestHTMLClippedButtonFollowsAncestorScroll(t *testing.T) {
	for _, mode := range []string{"hidden", "clip", "auto", "scroll"} {
		t.Run(mode, func(t *testing.T) {
			world := ecs.NewWorld()
			RegisterRuntimeFeature[HTMLRenderer](world)
			html := htmlui.New(`<div style="width:240px;height:100px;overflow:auto"><div style="height:60px">Save file</div><button id="binding" style="width:120px;height:30px;overflow:` + mode + `">Ctrl+S</button><div style="height:200px"></div></div>`)
			html.Width, html.Height, html.Font = 300, 200, notosanssc.NotoSansSCRegular
			bundle := ecs.NewBundle(1)
			bundle.Put(html)
			owner := world.Spawn(bundle)
			syncFrame := func() {
				t.Helper()
				if err := SyncSystem("").Run(world); err != nil {
					t.Fatal(err)
				}
				if err := SyncAfterLayoutSystem("").Run(world); err != nil {
					t.Fatal(err)
				}
				if err := scene.SyncUIGlobalTransformSystem("").Run(world); err != nil {
					t.Fatal(err)
				}
			}
			syncFrame()
			button := htmlControlByID(t, world, owner, "binding")
			parent, _ := world.Get[scene.Parent](button)
			clip := parent.Entity
			parent, _ = world.Get[scene.Parent](clip)
			area := parent.Entity
			if !world.Has[scene.UIScroll](area) {
				t.Fatal("button clip has no enclosing scroll viewport")
			}
			positions := make(map[ecs.Entity]float32)
			for _, entity := range []ecs.Entity{button, clip} {
				transform, _ := world.Get[scene.UIGlobalTransform](entity)
				positions[entity] = transform.Matrix[13]
			}
			for entity, access := range world.Query[htmlui.ControlText]().All {
				link := access.Get()

				if link.Control == button {
					transform, _ := world.Get[scene.UIGlobalTransform](entity)
					positions[entity] = transform.Matrix[13]
				}
			}
			if len(positions) < 3 {
				t.Fatal("button text is missing")
			}
			for _, offset := range []float32{-20, -40, 0} {
				scroll := world.GetMut[scene.UIScroll](area)
				scroll.Position[1] = offset
				for range 2 {
					syncFrame()
					for entity, before := range positions {
						transform, _ := world.Get[scene.UIGlobalTransform](entity)
						if math.Abs(float64(transform.Matrix[13]-before-offset)) > .01 {
							t.Errorf("entity %v y = %v, want %v after scroll %v", entity, transform.Matrix[13], before+offset, offset)
						}
					}
				}
			}
		})
	}
}
