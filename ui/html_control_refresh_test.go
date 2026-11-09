package ui

import (
	"strconv"
	"testing"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/scene"
	htmlui "github.com/onix-engine/onix/ui/html"
)

func TestHTMLControlTextRefreshOnlyMarksChangedText(t *testing.T) {
	world := ecs.NewWorld()
	controls := world.NewSpawner1[htmlui.Control]()
	texts := world.NewSpawner2[htmlui.ControlText, scene.Text2D]()
	control := htmlui.Control{Tag: "input", Type: "checkbox", Checked: true}
	owner := controls.Spawn(control)
	text := texts.Spawn(htmlui.ControlText{Control: owner}, scene.NewText2D(""))
	other := controls.Spawn(control)
	texts.Spawn(htmlui.ControlText{Control: other}, scene.NewText2D("other"))
	baseline := world.ChangeTick()
	links, _ := world.ComponentValueRevision[htmlui.ControlText]()

	refreshHTMLControlText(world, owner, control)
	if current, _ := world.ComponentValueRevision[htmlui.ControlText](); current != links {
		t.Fatal("text refresh marked immutable control links changed")
	}
	changes := world.ChangesSince(baseline)
	count := 0
	for entity, access := range world.Query[scene.Text2D](changes.Changed[scene.Text2D]()).All {
		if entity != text || access.Ref().Value != "✓" {
			t.Fatal("text refresh changed an unrelated text entity")
		}
		count++
	}
	if count != 1 {
		t.Fatalf("changed %d text entities, want 1", count)
	}
	version, _ := world.ComponentValueRevision[scene.Text2D]()
	if allocs := testing.AllocsPerRun(100, func() {
		refreshHTMLControlText(world, owner, control)
	}); allocs != 0 {
		t.Fatalf("unchanged text refresh allocated %v times", allocs)
	}
	if current, _ := world.ComponentValueRevision[scene.Text2D](); current != version {
		t.Fatal("unchanged text refresh marked text changed")
	}
}

func BenchmarkHTMLResetControlTexts(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			world := ecs.NewWorld()
			owner := world.Spawn(ecs.NewBundle(0))
			controls := world.NewSpawner1[htmlui.Control]()
			texts := world.NewSpawner2[htmlui.ControlText, scene.Text2D]()
			for range count {
				entity := controls.Spawn(htmlui.Control{Owner: owner, Tag: "input", Type: "checkbox", Checked: true})
				texts.Spawn(htmlui.ControlText{Control: entity}, scene.NewText2D(""))
			}
			syncHTMLControlsAfterReset(world, owner)
			b.ReportAllocs()
			for b.Loop() {
				syncHTMLControlsAfterReset(world, owner)
			}
		})
	}
}
