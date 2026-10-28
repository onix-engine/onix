package physics

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	"math/rand/v2"
	"slices"
	"testing"
)

func assertBroadphase(t *testing.T, w *World) {
	t.Helper()
	w.buildBroadphase()
	got := slices.Clone(w.pairs)
	w.pairs = w.pairs[:0]
	for i, a := range w.colliders {
		if !a.alive {
			continue
		}
		for j := i + 1; j < len(w.colliders); j++ {
			if w.colliders[j].alive {
				w.appendPair(i, j)
			}
		}
	}
	if !slices.Equal(got, w.pairs) {
		t.Fatalf("incremental pairs differ from brute force: got %v want %v", got, w.pairs)
	}
}
func TestBroadphaseIncrementalMutationsMatchBruteForce(t *testing.T) {
	w := NewWorld()
	w.CellSize = 3
	r := rand.New(rand.NewPCG(53, 17))
	bodies := make([]Body, 80)
	colliders := make([]ColliderHandle[Box], 80)
	for i := range bodies {
		d := NewBodyDef(BodyType(i % 3))
		d.Position = V(r.Float64()*20, r.Float64()*20)
		bodies[i] = w.CreateBody(d)
		colliders[i] = AddCollider(w, bodies[i], NewColliderDef(NewBox(3, 3)))
	}
	assertBroadphase(t, w)
	for i := range 160 {
		j := i % len(bodies)
		switch i % 7 {
		case 0:
			bodies[j].SetPosition(V(r.Float64()*20, r.Float64()*20))
		case 1:
			bodies[j].SetType(BodyType(i % 3))
		case 2:
			colliders[j].SetSensor(i%2 == 0)
		case 3:
			colliders[j].SetFilter(NewFilter(0, 1, uint64(i%2)))
		case 4:
			colliders[j].SetShape(NewBox(float64(i+1), 2))
		case 5:
			RemoveCollider(w, colliders[j])
			colliders[j] = AddCollider(w, bodies[j], NewColliderDef(NewBox(2, 2)))
		case 6:
			w.CellSize = float64(i%8 + 1)
		}
		assertBroadphase(t, w)
	}
}
func TestBroadphaseOnlyUpdatesMovedProxy(t *testing.T) {
	w := NewWorld()
	w.CellSize = 2
	for i := range 4096 {
		d := NewBodyDef(BodyStatic)
		d.Position = V(float64(i)*4, 0)
		AddCollider(w, w.CreateBody(d), NewColliderDef(Circle{Radius: 1}))
	}
	d := NewBodyDef(BodyDynamic)
	d.Position = V(0, 3)
	b := w.CreateBody(d)
	AddCollider(w, b, NewColliderDef(Circle{Radius: 1}))
	w.buildBroadphase()
	w.stats = StepStatistics{}
	b.SetPosition(V(4, 3))
	w.buildBroadphase()
	w.buildBroadphase()
	if s := w.Statistics(); s.BoundsUpdates != 1 || s.IndexUpdates != 1 || s.PairQueries != 1 {
		t.Fatalf("unchanged proxies were searched: %+v", s)
	}
	if n := testing.AllocsPerRun(100, func() { b.SetPosition(V(8-b.Position().X, 3)); w.buildBroadphase() }); n != 0 {
		t.Fatalf("moving index allocations=%v", n)
	}
}
func TestSleepingTaskCountConsumesExternalECSInput(t *testing.T) {
	w := ecs.NewWorld()
	r := NewRuntime2D()
	r.Simulation.Sleep.Enabled = true
	r.Simulation.Gravity = Vector{}
	ecs.InsertResource(w, r)
	bundle := ecs.NewBundle(2)
	ecs.Put(&bundle, NewRigidBody2D(BodyDynamic))
	ecs.Put(&bundle, scene.NewTransform())
	e := w.Spawn(bundle)
	step := StepSystem2D("")
	for range 60 {
		if err := step.Run(w); err != nil {
			t.Fatal(err)
		}
	}
	if TaskCount2D(w) != 0 {
		t.Fatal("sleeping ECS body keeps idle loop ticking")
	}
	ecs.GetMut[RigidBody2D](w, e).Force = V(1, 0)
	if TaskCount2D(w) != 1 {
		t.Fatal("queued force did not request physics tick")
	}
	if err := step.Run(w); err != nil {
		t.Fatal(err)
	}
	if b := ecs.GetRef[RigidBody2D](w, e); b.Handle().IsSleeping() || b.Velocity.X <= 0 {
		t.Fatal("queued force was not consumed")
	}
}
func BenchmarkWorld2DStaticIndex(b *testing.B) {
	w := NewWorld()
	w.CellSize = 2
	for i := range 4096 {
		d := NewBodyDef(BodyStatic)
		d.Position = V(float64(i%64)*4, float64(i/64)*4)
		AddCollider(w, w.CreateBody(d), NewColliderDef(Circle{Radius: 1}))
	}
	body := w.CreateBody(NewBodyDef(BodyDynamic))
	AddCollider(w, body, NewColliderDef(Circle{Radius: 1}))
	w.buildBroadphase()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		body.SetPosition(V(4-body.Position().X, 2))
		w.buildBroadphase()
	}
}
func BenchmarkWorld2DSleepingStack(b *testing.B) {
	w := NewWorld()
	w.Sleep.Enabled = true
	d := NewBodyDef(BodyStatic)
	d.Position = V(0, -1)
	AddCollider(w, w.CreateBody(d), NewColliderDef(NewBox(100, 2)))
	for i := range 64 {
		d := NewBodyDef(BodyDynamic)
		d.FixedRotation = true
		d.Position = V(float64(i%8)*2, float64(i/8)*2+1)
		AddCollider(w, w.CreateBody(d), NewColliderDef(NewBox(2, 2)))
	}
	for range 1200 {
		w.Step(1.0 / 60)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		w.Step(1.0 / 60)
	}
	b.ReportMetric(float64(w.Statistics().SleepingBodies), "sleeping")
}
