package physics

import "testing"

func benchmarkWorld2D(count int, sensors bool) *World {
	world := NewWorld()
	world.Gravity = Vector{}
	world.Reserve(count, count)
	for index := range count {
		def := NewBodyDef(BodyDynamic)
		def.Position = V(float64(index%32)*3, float64(index/32)*3)
		body := world.CreateBody(def)
		collider := NewColliderDef(Circle{Radius: 1})
		if sensors {
			collider.Shape.Radius = 2
			collider.Sensor = true
		}
		world.AddCollider(body, collider)
	}
	for range 4 {
		world.Step(1.0 / 60)
	}
	return world
}

func BenchmarkWorld2DStep(b *testing.B) {
	for _, test := range []struct {
		name    string
		count   int
		sensors bool
	}{{"Separated256", 256, false}, {"Separated1024", 1024, false}, {"Sensors256", 256, true}} {
		b.Run(test.name, func(b *testing.B) {
			world := benchmarkWorld2D(test.count, test.sensors)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				world.Step(1.0 / 60)
			}
		})
	}
}

func BenchmarkWorld2DBroadphase(b *testing.B) {
	world := benchmarkWorld2D(256, true)
	world.CellSize = 1
	world.buildBroadphase()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		world.buildBroadphase()
	}
}
