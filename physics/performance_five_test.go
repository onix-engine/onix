package physics

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func BenchmarkReviewPhysicsSync(b *testing.B) {
	for _, mode := range []string{"unchanged", "body-value", "body-transform", "unrelated-transform"} {
		b.Run(mode, func(b *testing.B) {
			w := ecs.NewWorld()
			runtime := NewRuntime2D()
			runtime.Simulation.Gravity = Vector{}
			w.InsertResource(runtime)
			bundle := ecs.NewBundle(2)
			bundle.Put(scene.TransformFromXYZ(0, 0, 0))
			bundle.Put(NewRigidBody2D(BodyStatic))
			var target ecs.Entity
			for range 10000 {
				target = w.Spawn(bundle)
			}
			plain := ecs.NewBundle(1)
			plain.Put(scene.TransformFromXYZ(0, 0, 0))
			unrelated := w.Spawn(plain)
			system := StepSystem2D("")
			if err := system.Run(w); err != nil {
				b.Fatal(err)
			}
			update := func() {
				switch mode {
				case "body-value":
					w.GetMut[RigidBody2D](target).Mass += 1
				case "body-transform":
					w.GetMut[scene.Transform](target).Translation[0] += 1
				case "unrelated-transform":
					w.GetMut[scene.Transform](unrelated).Translation[0] += 1
				}
				if err := system.Run(w); err != nil {
					b.Fatal(err)
				}
			}
			update()
			b.ReportAllocs()
			for b.Loop() {
				update()
			}
		})
	}
}
