package physics

import (
	"fmt"
	"testing"
)

func sleepPair() (*World, Body, Body) {
	w := NewWorld()
	w.Gravity = Vector{}
	w.Sleep.Enabled = true
	a := w.CreateBody(NewBodyDef(BodyDynamic))
	d := NewBodyDef(BodyDynamic)
	d.Position = V(1.99, 0)
	b := w.CreateBody(d)
	AddCollider(w, a, NewColliderDef(Circle{Radius: 1}))
	AddCollider(w, b, NewColliderDef(Circle{Radius: 1}))
	for range 120 {
		w.Step(1.0 / 60)
	}
	return w, a, b
}
func TestSleepIslandWakeAndEvents(t *testing.T) {
	w, a, b := sleepPair()
	if !a.IsSleeping() || !b.IsSleeping() {
		t.Fatal("quiet contact island did not sleep")
	}
	for _, e := range w.Events() {
		if e.Phase != CollisionStay {
			t.Fatal("sleep changed collision lifetime")
		}
	}
	a.ApplyImpulse(V(1, 0), a.Position())
	if a.IsSleeping() || b.IsSleeping() {
		t.Fatal("impulse must wake whole island immediately")
	}
	w.Step(1.0 / 60)
	if b.Velocity().X <= 0 {
		t.Fatal("woken neighbour did not receive impulse")
	}
}
func TestSleepSupportRemovalAndConfig(t *testing.T) {
	w := NewWorld()
	w.Sleep.Enabled = true
	floor := w.CreateBody(NewBodyDef(BodyStatic))
	shape := AddCollider(w, floor, NewColliderDef(Box{Center: V(0, -1), HalfExtents: V(10, 1)}))
	d := NewBodyDef(BodyDynamic)
	d.Position = V(0, 1)
	body := w.CreateBody(d)
	AddCollider(w, body, NewColliderDef(Circle{Radius: 1}))
	for range 240 {
		w.Step(1.0 / 60)
	}
	if !body.IsSleeping() {
		t.Fatalf("resting body did not sleep: %+v", *body.state())
	}
	RemoveCollider(w, shape)
	// Reusing the support collider slot must preserve the former island wake.
	replacement := NewBodyDef(BodyStatic)
	replacement.Position = V(100, 0)
	AddCollider(w, w.CreateBody(replacement), NewColliderDef(NewBox(2, 2)))
	w.Step(1.0 / 60)
	if body.IsSleeping() || body.Velocity().Y >= 0 {
		t.Fatal("removed support did not wake body")
	}
	w, a, b := sleepPair()
	w.Sleep.Enabled = false
	w.Step(1.0 / 60)
	if a.IsSleeping() || b.IsSleeping() {
		t.Fatal("disabling sleep left bodies asleep")
	}
}
func TestSleepingWarmStepHasNoAllocationsOrBroadphaseQueries(t *testing.T) {
	w, _, _ := sleepPair()
	if allocations := testing.AllocsPerRun(100, func() { w.Step(1.0 / 60) }); allocations != 0 {
		t.Fatalf("allocations=%v", allocations)
	}
	if s := w.Statistics(); s.BoundsUpdates != 0 || s.PairQueries != 0 || s.IndexUpdates != 0 {
		t.Fatalf("unchanged sleeping proxies were searched: %+v", s)
	}
}

func TestDenseStackIslandSleepsAndWakes(t *testing.T) {
	w := NewWorld()
	w.Sleep.Enabled = true
	w.Iterations = 16
	w.CellSize = 2
	floor := w.CreateBody(NewBodyDef(BodyStatic))
	AddCollider(w, floor, NewColliderDef(Box{Center: V(0, -1), HalfExtents: V(5, 1)}))
	bodies := make([]Body, 8)
	for i := range bodies {
		d := NewBodyDef(BodyDynamic)
		d.Position = V(0, float64(i)*1.99+1)
		d.FixedRotation = true
		bodies[i] = w.CreateBody(d)
		AddCollider(w, bodies[i], NewColliderDef(NewBox(2, 2)))
	}
	for range 1200 {
		w.Step(1.0 / 60)
	}
	for i, b := range bodies {
		if !b.IsSleeping() {
			t.Fatalf("stack member %d awake: %+v", i, *b.state())
		}
	}
	bodies[7].ApplyImpulse(V(1, 0), bodies[7].Position())
	for i, b := range bodies {
		if b.IsSleeping() {
			t.Fatalf("stack member %d was not woken", i)
		}
	}
}

func TestNewStaticContactWakesSleepingBody(t *testing.T) {
	w := NewWorld()
	w.Gravity = Vector{}
	w.Sleep.Enabled = true
	b := w.CreateBody(NewBodyDef(BodyDynamic))
	AddCollider(w, b, NewColliderDef(Circle{Radius: 1}))
	for range 60 {
		w.Step(1.0 / 60)
	}
	if !b.IsSleeping() {
		t.Fatal("body did not sleep")
	}
	d := NewBodyDef(BodyStatic)
	d.Position = V(1.5, 0)
	AddCollider(w, w.CreateBody(d), NewColliderDef(Circle{Radius: 1}))
	w.Step(1.0 / 60)
	if b.IsSleeping() || b.Position().X >= 0 {
		t.Fatal("new static contact did not wake/correct sleeping body")
	}
}
func TestCommonStaticSupportDoesNotJoinIndependentIslands(t *testing.T) {
	w := NewWorld()
	w.Sleep.Enabled = true
	floor := w.CreateBody(NewBodyDef(BodyStatic))
	AddCollider(w, floor, NewColliderDef(Box{Center: V(0, -1), HalfExtents: V(20, 1)}))
	d := NewBodyDef(BodyDynamic)
	d.Position = V(-5, 1)
	a := w.CreateBody(d)
	d.Position = V(5, 1)
	b := w.CreateBody(d)
	AddCollider(w, a, NewColliderDef(Circle{Radius: 1}))
	AddCollider(w, b, NewColliderDef(Circle{Radius: 1}))
	for range 240 {
		w.Step(1.0 / 60)
	}
	a.ApplyImpulse(V(1, 0), a.Position())
	if a.IsSleeping() || !b.IsSleeping() {
		t.Fatal("shared static floor joined independent sleep islands")
	}
}

func TestWakeCacheRejectsReusedBodySlots(t *testing.T) {
	w, a, b := sleepPair()
	if !w.RemoveBody(a) || b.IsSleeping() {
		t.Fatal("removing a body did not wake its former neighbour")
	}
	replacement := w.CreateBody(NewBodyDef(BodyDynamic))
	// A new body has not participated in the cached contacts. Reusing its
	// index must not reset the old neighbour's quiet timer.
	b.state().quietTime = .25
	replacement.ApplyForce(V(1, 0))
	if b.state().quietTime != .25 {
		t.Fatal("reused body slot inherited the former contact island")
	}
}

func TestWakeBatchPreservesIndependentIslandsWithoutAllocations(t *testing.T) {
	w, a, b := sleepPair()
	independent := w.CreateBody(NewBodyDef(BodyDynamic))
	for range 60 {
		w.Step(1.0 / 60)
	}
	if !a.IsSleeping() || !b.IsSleeping() || !independent.IsSleeping() {
		t.Fatal("quiet bodies did not sleep")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		// Exercise fresh propagation as well as duplicate requests on every run.
		a.state().wakeContacts, b.state().wakeContacts = false, false
		a.state().sleeping, b.state().sleeping = true, true
		a.Wake()
		b.Wake()
		a.ApplyForce(V(1, 0))
	}); allocations != 0 {
		t.Fatalf("warm island wake allocated %g times", allocations)
	}
	if a.IsSleeping() || b.IsSleeping() || !independent.IsSleeping() {
		t.Fatal("wake batch missed its island or visited an unrelated body")
	}
}

func BenchmarkWorld2DBatchSetVelocity(b *testing.B) {
	for _, count := range []int{256, 1024, 4096} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			w := NewWorld()
			w.Sleep.Enabled = true
			w.Gravity = Vector{}
			bodies := make([]Body, count)
			for i := range bodies {
				bodies[i] = w.CreateBody(NewBodyDef(BodyDynamic))
			}
			w.Step(1.0 / 60)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Reset requests to exercise one fresh batch between steps.
				for j := range w.bodies {
					w.bodies[j].wakeContacts = false
				}
				for _, body := range bodies {
					body.SetVelocity(V(float64(i%2+1), 0))
				}
			}
		})
	}
}
