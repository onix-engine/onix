package module

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
)

func TestTransformSpawnersReuseWarmStorage(t *testing.T) {
	for _, ui := range []bool{false, true} {
		r := &Runtime{world: ecs.NewWorld()}
		pose := project.ModTransform{Scale: [3]float32{1, 1, 1}}
		root := r.spawnTransform(pose, ui, 0, 7, true)
		c := &Context{runtime: r, ui: ui, visible: false, enabled: true}
		spawn := func() {
			child := r.spawnTransform(pose, ui, root, 3, false)
			owned := r.spawnOwnedTransform(c, root, 5)
			parent, _ := r.world.Get[scene.Parent](owned)
			owner, _ := r.world.Get[entityOwner](owned)
			visibility, _ := r.world.Get[scene.Visibility](owned)
			activity, _ := r.world.Get[scene.Activity](owned)
			if parent.Entity != root || owner.context != c || visibility.Visible || !activity.Active {
				t.Fatal("typed spawn lost parent, ownership or lifecycle flags")
			}
			r.world.Despawn(child)
			r.world.Despawn(owned)
		}
		spawn()
		if n := testing.AllocsPerRun(100, spawn); n != 0 {
			t.Fatalf("UI=%t warm spawn/despawn allocated %g objects", ui, n)
		}
	}
}

func BenchmarkModuleTransformSpawning(b *testing.B) {
	for _, ui := range []bool{false, true} {
		name := "World"
		if ui {
			name = "UI"
		}
		b.Run(name, func(b *testing.B) {
			pose := project.ModTransform{Scale: [3]float32{1, 1, 1}}
			b.ReportAllocs()
			for b.Loop() {
				r := &Runtime{world: ecs.NewWorld()}
				for range 1000 {
					r.spawnTransform(pose, ui, 0, 0, true)
				}
			}
		})
	}
}
