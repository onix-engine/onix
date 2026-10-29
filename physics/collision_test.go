package physics

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestCircleInsideBoxUsesNearestExit(t *testing.T) {
	for _, rotation := range []float64{0, .7, -1.2} {
		for _, position := range []Vector{V(.8, 0), V(-.8, 0), V(0, .8), V(0, -.8), V(1, 0), V(0, -1)} {
			world := NewWorld()
			world.Gravity, world.CollisionSlop = Vector{}, 0
			floorDef := NewBodyDef(BodyStatic)
			floorDef.Rotation = rotation
			floor := world.CreateBody(floorDef)
			world.AddCollider(floor, NewColliderDef(NewBox(2, 2)))
			def := NewBodyDef(BodyDynamic)
			def.Position = rotate(position, rotation)
			body := world.CreateBody(def)
			world.AddCollider(body, NewColliderDef(Circle{Radius: .25}))
			world.Step(1.0 / 60)
			want := rotate(position.Normalize().Mul(1.25), rotation)
			if body.Position().Sub(want).Length() > 1e-5 {
				t.Fatalf("rotation=%v local=%v: position=%v, want=%v", rotation, position, body.Position(), want)
			}
		}
	}
}

func TestBoxContactPointClipsToOverlappingFaces(t *testing.T) {
	a := worldBox{half: V(10, 1), xAxis: V(1, 0), yAxis: V(0, 1)}
	b := worldBox{center: V(3, 1.9), half: V(1, 1), xAxis: V(1, 0), yAxis: V(0, 1)}
	for _, rotation := range []float64{0, .4, -1.5} {
		first, second := a, b
		first.xAxis, first.yAxis = rotate(a.xAxis, rotation), rotate(a.yAxis, rotation)
		second.xAxis, second.yAxis = first.xAxis, first.yAxis
		second.center = rotate(b.center, rotation)
		contact, ok := collideBoxBox(first, second)
		if !ok || contact.point.Sub(rotate(V(3, .95), rotation)).Length() > 1e-9 || math.Abs(contact.penetration-.1) > 1e-9 || contact.pointCount != 2 {
			t.Fatalf("rotation=%v: contact=%+v hit=%v", rotation, contact, ok)
		}
	}
}

func TestBoxSegmentRoundedCorner(t *testing.T) {
	box := worldBox{half: V(1, 1), xAxis: V(1, 0), yAxis: V(0, 1)}
	segment := worldSegment{start: V(1.4, 1.4), end: V(2, 1.4), radius: .5}
	if _, _, _, ok := collideBoxSegment(box, segment); ok {
		t.Fatal("rounded segment corner produced a false collision")
	}
	segment.radius = .6
	normal, _, depth, ok := collideBoxSegment(box, segment)
	if !ok || normal.Sub(V(1, 1).Normalize()).Length() > 1e-9 || math.Abs(depth-(.6-math.Sqrt(.32))) > 1e-9 {
		t.Fatalf("rounded corner contact = normal:%v depth:%v hit:%v", normal, depth, ok)
	}
}

func TestTiltedBoxContactStaysNearPenetratingCorner(t *testing.T) {
	angle := .3
	floor := worldBox{half: V(10, 1), xAxis: V(1, 0), yAxis: V(0, 1)}
	box := worldBox{half: V(1, 1), xAxis: rotate(V(1, 0), angle), yAxis: rotate(V(0, 1), angle)}
	box.center = V(0, 1+math.Sin(angle)+math.Cos(angle)-.01)
	corner := box.center.Sub(box.xAxis).Sub(box.yAxis)
	contact, ok := collideBoxBox(floor, box)
	if !ok || contact.point.Sub(corner).Length() > .03 {
		t.Fatalf("contact=%v, want near penetrating corner=%v", contact.point, corner)
	}
}

func TestBroadphaseMatchesBruteForce(t *testing.T) {
	random := rand.New(rand.NewPCG(31, 79))
	world := NewWorld()
	for index := range 120 {
		def := NewBodyDef(BodyType(index % 3))
		def.Position = V(random.Float64()*50-25, random.Float64()*50-25)
		def.Rotation = random.Float64() * math.Pi
		body := world.CreateBody(def)
		collider := NewColliderDef(NewBox(random.Float64()*15, random.Float64()*15))
		collider.Sensor = index%7 == 0
		collider.Filter = NewFilter(int32(index%5), uint64(1)<<uint(index%3), uint64(3)<<uint(index%2))
		world.AddCollider(body, collider)
	}
	for _, size := range []float64{.1, 1, 3, 16, 64, 1e6, math.NaN(), math.Inf(1)} {
		world.CellSize = size
		world.buildBroadphase()
		pairs := make(map[colliderPair]bool)
		for index, pair := range world.pairs {
			if pairs[pair] {
				t.Fatalf("cellSize=%v: duplicate pair %+v", size, pair)
			}
			if index > 0 && pairKey(pair.a, pair.b) <= pairKey(world.pairs[index-1].a, world.pairs[index-1].b) {
				t.Fatal("pairs are not sorted")
			}
			pairs[pair] = true
		}
		for a, first := range world.colliders {
			for b := a + 1; b < len(world.colliders); b++ {
				second := world.colliders[b]
				want := first.bodyIndex != second.bodyIndex && !first.filter.Reject(second.filter) &&
					(first.sensor || second.sensor || world.bodies[first.bodyIndex].bodyType == BodyDynamic || world.bodies[second.bodyIndex].bodyType == BodyDynamic) &&
					overlapAABB(world.aabbs[a], world.aabbs[b])
				if pairs[colliderPair{a: a, b: b}] != want {
					t.Fatalf("cellSize=%v: pair=(%v,%v) present=%v want=%v", size, a, b, !want, want)
				}
			}
		}
	}
}

func TestBroadphaseExtremeCoordinates(t *testing.T) {
	world := NewWorld()
	world.CellSize = 1
	for _, position := range []float64{math.MaxInt32, math.MinInt32, 1e20, -1e20} {
		for range 2 {
			def := NewBodyDef(BodyDynamic)
			def.Position = V(position, position)
			world.AddCollider(world.CreateBody(def), NewColliderDef(Circle{Radius: 1}))
		}
	}
	world.AddCollider(world.CreateBody(NewBodyDef(BodyStatic)), NewColliderDef(NewBox(1e21, 1e21)))
	world.buildBroadphase()
	if len(world.pairs) != 12 {
		t.Fatalf("extreme-coordinate pair count = %v, want 12", len(world.pairs))
	}
}
