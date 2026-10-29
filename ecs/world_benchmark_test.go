package ecs

import "testing"

type benchmarkPosition struct {
	X float32
	Y float32
}

type benchmarkVelocity struct {
	X float32
	Y float32
}

func BenchmarkQuery2TenThousand(b *testing.B) {
	world := NewWorld()
	for i := 0; i < 10_000; i++ {
		bundle := NewBundle(2)
		bundle.Put(benchmarkPosition{})
		bundle.Put(benchmarkVelocity{X: 1, Y: 1})
		world.Spawn(bundle)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for _, access := range world.Query2[benchmarkPosition, benchmarkVelocity]().All {
			position := access.A.Mut()
			velocity := access.B.Mut()

			position.X += velocity.X
			position.Y += velocity.Y
		}
	}
}

func BenchmarkRead2TenThousand(b *testing.B) {
	world := NewWorld()
	for i := 0; i < 10_000; i++ {
		bundle := NewBundle(2)
		bundle.Put(benchmarkPosition{X: float32(i)})
		bundle.Put(benchmarkVelocity{X: 1, Y: 1})
		world.Spawn(bundle)
	}
	var total float32
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for _, access := range world.Query2[benchmarkPosition, benchmarkVelocity]().All {
			position := access.A.Get()
			velocity := access.B.Get()

			total += position.X + velocity.X
		}
	}
	benchmarkTotal = total
}

func BenchmarkQueryAccess2TenThousand(b *testing.B) {
	world := NewWorld()
	spawner := world.NewSpawner2[benchmarkPosition, benchmarkVelocity]()
	for range 10_000 {
		spawner.Spawn(benchmarkPosition{}, benchmarkVelocity{X: 1, Y: 1})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for _, access := range world.Query2[benchmarkPosition, benchmarkVelocity]().All {
			position := access.A
			velocity := access.B

			position.Mut().X += velocity.Ref().X
		}
	}
}

var benchmarkTotal float32

func BenchmarkSpawnBundle2OneThousand(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		world := NewWorld()
		for range 1000 {
			bundle := NewBundle(2)
			bundle.Put(benchmarkPosition{})
			bundle.Put(benchmarkVelocity{X: 1, Y: 1})
			world.Spawn(bundle)
		}
	}
}

func BenchmarkSpawner2OneThousand(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		world := NewWorld()
		spawner := world.NewSpawner2[benchmarkPosition, benchmarkVelocity]()
		for range 1000 {
			spawner.Spawn(benchmarkPosition{}, benchmarkVelocity{X: 1, Y: 1})
		}
	}
}

func BenchmarkMessageReaderReadOneHundred(b *testing.B) {
	world := NewWorld()
	var reader MessageReader[int]
	b.ReportAllocs()
	for range b.N {
		for message := range 100 {
			world.WriteMessage(message)
		}
		benchmarkTotal += float32(len(reader.Read(world)))
		world.AdvanceMessages()
	}
}

func BenchmarkMessageReaderAllOneHundred(b *testing.B) {
	world := NewWorld()
	var reader MessageReader[int]
	b.ReportAllocs()
	for range b.N {
		for message := range 100 {
			world.WriteMessage(message)
		}
		for message := range reader.All(world) {
			benchmarkTotal += float32(*message)
		}
		world.AdvanceMessages()
	}
}

func BenchmarkGetMutTenThousand(b *testing.B) {
	world := NewWorld()
	entities := make([]Entity, 10_000)
	for i := range entities {
		bundle := NewBundle(1)
		bundle.Put(benchmarkPosition{})
		entities[i] = world.Spawn(bundle)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for _, entity := range entities {
			position := world.GetMut[benchmarkPosition](entity)
			position.X++
		}
	}
}
