package physics

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"
)

// Derive every AABB from current shape and pose, independently of cached
// bounds, grid membership and dirty flags. Iteration order also checks that
// pairs are unique and ordered for warm starting and event generation.
func checkBroadphase3D(t *testing.T, w *World3D) {
	t.Helper()
	w.buildBroadphase()
	var want []colliderPair
	for i, a := range w.colliders {
		if !a.alive {
			continue
		}
		ba := w.bodies[a.bodyIndex]
		bounds := colliderBounds3D(a, ba)
		for j := i + 1; j < len(w.colliders); j++ {
			b := w.colliders[j]
			if !b.alive || b.bodyIndex == a.bodyIndex || a.filter.Reject(b.filter) {
				continue
			}
			bb := w.bodies[b.bodyIndex]
			if !a.sensor && !b.sensor && ba.bodyType != BodyDynamic && bb.bodyType != BodyDynamic {
				continue
			}
			if overlapAABB3D(bounds, colliderBounds3D(b, bb)) {
				want = append(want, colliderPair{i, j})
			}
		}
	}
	if !slices.Equal(w.pairs, want) {
		t.Fatalf("pairs %v, want %v", w.pairs, want)
	}
}

func TestBroadphase3DIncrementalMutations(t *testing.T) {
	w := NewWorld3D()
	r := rand.New(rand.NewPCG(13, 51))
	var bodies [28]Body3D
	var colliders [28]ColliderHandle3D[Box3D]
	position := func() Vector3D { return V3(r.Float64()*8, r.Float64()*8, r.Float64()*8) }
	for i := range bodies {
		d := NewBodyDef3D(BodyType(i % 3))
		d.Position = position()
		bodies[i] = w.CreateBody(d)
		colliders[i] = w.AddCollider(bodies[i], NewColliderDef(NewBox3D(2, 2, 2)))
	}
	checkBroadphase3D(t, w)
	w.Reserve(90, 120) // Grow the hash tables with live chains.
	for range 120 {
		for range 3 {
			j := r.IntN(len(bodies))
			switch r.IntN(10) {
			case 0:
				bodies[j].SetPosition(position())
			case 1:
				bodies[j].SetRotation(QuaternionFromAxisAngle(V3(0, 1, 0), r.Float64()*math.Pi))
			case 2:
				bodies[j].SetType(BodyType(r.IntN(3)))
			case 3:
				colliders[j].SetSensor(!colliders[j].Sensor())
			case 4:
				colliders[j].SetFilter(NewFilter(0, 1, uint64(r.IntN(2))))
			case 5:
				colliders[j].SetShape(NewBox3D(float64(r.IntN(30)+1), 2, 3))
			case 6:
				w.RemoveCollider(colliders[j])
				colliders[j] = w.AddCollider(bodies[j], NewColliderDef(NewBox3D(2, 3, 1)))
			case 7:
				w.CellSize = float64(r.IntN(5) + 1)
			case 8:
				w.RemoveBody(bodies[j])
				bodies[j] = w.CreateBody(NewBodyDef3D(BodyStatic))
				colliders[j] = w.AddCollider(bodies[j], NewColliderDef(NewBox3D(1, 1, 1)))
			case 9:
				w.AddCollider(bodies[j], NewColliderDef(Sphere{Radius: .25, Center: V3(.5, 1, 0)}))
			}
		}
		checkBroadphase3D(t, w)
	}
}

func TestBroadphase3DCellRange(t *testing.T) {
	span := cellSpan3D{cell3D{math.MinInt32, math.MinInt32, math.MinInt32}, cell3D{math.MaxInt32, math.MaxInt32, math.MaxInt32}}
	if !span.large() {
		t.Fatal("wide span must use large proxy path")
	}
	single := cellSpan3D{span.max, span.max}
	count := 0
	for c := range single.cells {
		if c != single.min || count != 0 {
			t.Fatal("unexpected grid cell", c)
		}
		count++
	}
	if count != 1 {
		t.Fatal("missing boundary cell")
	}
}

func TestBroadphase3DCorrectionAddsContact(t *testing.T) {
	w := NewWorld3D()
	w.Iterations = 1
	w.Gravity = Vector3D{}
	sphere := NewColliderDef(Sphere{Radius: 1})
	w.AddCollider(w.CreateBody(NewBodyDef3D(BodyStatic)), sphere)
	d := NewBodyDef3D(BodyDynamic)
	d.Position = V3(1, 0, 0)
	mover := w.AddCollider(w.CreateBody(d), sphere)
	d.Type, d.Position = BodyStatic, V3(3.5, 0, 0)
	sphere.Sensor = true
	sensor := w.AddCollider(w.CreateBody(d), sphere)
	w.Step(1.0 / 60)
	for _, e := range w.Events() {
		if e.Phase == CollisionBegin && e.ColliderA == mover.ID() && e.ColliderB == sensor.ID() {
			return
		}
	}
	t.Fatal("contact introduced by the last position correction was missed")
}

func staticWorld3D() (*World3D, Body3D) {
	w := NewWorld3D()
	w.Gravity = Vector3D{}
	w.CellSize = 2
	d := NewBodyDef3D(BodyStatic)
	for i := range 4096 {
		d.Position = V3(float64(i%64)*4, 0, float64(i/64)*4)
		w.AddCollider(w.CreateBody(d), NewColliderDef(Sphere{Radius: .5}))
	}
	d = NewBodyDef3D(BodyDynamic)
	d.Position = V3(0, 2, 0)
	body := w.CreateBody(d)
	w.AddCollider(body, NewColliderDef(Sphere{Radius: .5}))
	w.Step(1.0 / 60)
	return w, body
}

func TestBroadphase3DUpdatesOnlyMovingCollider(t *testing.T) {
	w, body := staticWorld3D()
	allocs := testing.AllocsPerRun(20, func() {
		body.SetPosition(V3(4-body.Position().X, 2, 0))
		w.Step(1.0 / 60)
	})
	s := w.Statistics()
	if allocs != 0 || s.BoundsUpdates != 1 || s.IndexUpdates != 1 || s.PairQueries != 1 || s.ContactBuilds != 2 || s.CandidatePairs != 0 {
		t.Fatalf("sparse step: %+v, %v allocations", s, allocs)
	}
	w.Step(1.0 / 60)
	if s := w.Statistics(); s.BoundsUpdates != 0 || s.IndexUpdates != 0 || s.PairQueries != 0 {
		t.Fatalf("unchanged step: %+v", s)
	}
}

func BenchmarkWorld3DStaticIndexStep(b *testing.B) {
	w, body := staticWorld3D()
	b.ReportAllocs()
	for b.Loop() {
		body.SetPosition(V3(4-body.Position().X, 2, 0))
		w.Step(1.0 / 60)
	}
}
