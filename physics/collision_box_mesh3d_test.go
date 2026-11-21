package physics

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestBoxMeshEventsUseTriangleGeometry(t *testing.T) {
	mesh, err := NewTriangleMesh([]Vector3D{V3(0, 0, 0), V3(0, 0, 4), V3(4, 0, 0)}, []uint32{0, 1, 2})
	if err != nil {
		t.Fatal(err)
	}
	for _, boxFirst := range []bool{false, true} {
		w := NewWorld3D()
		w.Gravity = Vector3D{}
		d := NewBodyDef3D(BodyDynamic)
		d.Position = V3(3.5, .1, 3.5)
		box := w.CreateBody(d)
		ground := w.CreateBody(NewBodyDef3D(BodyStatic))
		c := NewColliderDef(NewBox3D(.5, .5, .5))
		c.Sensor = true
		if boxFirst {
			w.AddCollider(box, c)
			w.AddCollider(ground, NewColliderDef(mesh))
		} else {
			w.AddCollider(ground, NewColliderDef(mesh))
			w.AddCollider(box, c)
		}
		w.Step(1.0 / 60)
		if len(w.Events()) != 0 {
			t.Fatal("separated box inside mesh AABB produced a contact")
		}
		box.SetPosition(V3(1, .1, 1))
		for _, phase := range []CollisionPhase{CollisionBegin, CollisionStay} {
			w.Step(1.0 / 60)
			events := w.Events()
			wantY := 1.0
			if boxFirst {
				wantY = -1
			}
			if len(events) != 1 || events[0].Phase != phase || math.Abs(events[0].Normal.Y-wantY) > 1e-8 ||
				math.Abs(events[0].Penetration-.15) > 1e-8 || events[0].Point.Sub(V3(1, -.075, 1)).Length() > 1e-8 {
				t.Fatalf("boxFirst=%v phase=%v contact=%+v", boxFirst, phase, events)
			}
		}
		box.SetPosition(V3(3.5, .1, 3.5))
		w.Step(1.0 / 60)
		if events := w.Events(); len(events) != 1 || events[0].Phase != CollisionEnd {
			t.Fatal("missing end", events)
		}
	}
}

func TestBoxTriangleSATAgreesWithClippedIntersection(t *testing.T) {
	random := rand.New(rand.NewPCG(14, 71))
	vector := func() Vector3D { return V3(random.Float64()*4-2, random.Float64()*4-2, random.Float64()*4-2) }
	for range 1200 {
		body := bodyState3D{rotation: QuaternionFromAxisAngle(vector(), random.Float64()*math.Pi)}
		box := makeWorldBox3D(colliderState3D{half: V3(.8, .4, .6), rotation: QuaternionIdentity()}, body)
		a, b, c := vector(), vector(), vector()
		points := [16]Vector3D{a, b, c}
		count := 3
		for i, axis := range box.axis {
			points, count = clipContactPolygon3D(points, count, box.center, axis, box.half.component(i))
			points, count = clipContactPolygon3D(points, count, box.center, axis.Mul(-1), box.half.component(i))
		}
		contact, ok := boxTriangleContact(box, a, b, c)
		if ok != (count != 0) || ok && (contact.pointCount < 1 || contact.pointCount > 4 || math.Abs(contact.normal.Length()-1) > 1e-8) {
			t.Fatalf("triangle %v %v %v: clipped=%d contact=%+v hit=%v", a, b, c, count, contact, ok)
		}
	}
}

func TestRotatedBoxRestsAcrossMeshSeam(t *testing.T) {
	mesh, err := NewTriangleMesh([]Vector3D{V3(-10, 0, -10), V3(-10, 0, 10), V3(10, 0, 10), V3(10, 0, -10)}, []uint32{0, 1, 2, 0, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	w := NewWorld3D()
	ground := NewBodyDef3D(BodyStatic)
	ground.Position = V3(3, 2, -4)
	ground.Rotation = QuaternionFromAxisAngle(V3(0, 0, 1), .12)
	w.AddCollider(w.CreateBody(ground), NewColliderDef(mesh))
	up := ground.Rotation.Rotate(V3(0, 1, 0))
	d := NewBodyDef3D(BodyDynamic)
	d.Position = ground.Position.Add(up.Mul(1.2))
	d.Rotation = ground.Rotation.Mul(QuaternionFromAxisAngle(V3(0, 1, 0), .55))
	box := w.CreateBody(d)
	w.AddCollider(box, NewColliderDef(NewBox3D(1.4, 1, .8)))
	for range 360 {
		w.Step(1.0 / 120)
	}
	atRest := box.Position()
	if height := atRest.Sub(ground.Position).Dot(up); math.Abs(height-.5) > .02 {
		t.Fatalf("height above slope: %g", height)
	}
	if allocs := testing.AllocsPerRun(60, func() { w.Step(1.0 / 120) }); allocs != 0 {
		t.Fatalf("resting mesh contact allocs: %g", allocs)
	}
	if delta := box.Position().Sub(atRest).Length(); delta > .005 || box.Velocity().Length() > .02 || box.AngularVelocity().Length() > .02 {
		t.Fatalf("unstable slope contact: drift=%g v=%v spin=%v", delta, box.Velocity(), box.AngularVelocity())
	}
}

func TestBoxFallsThroughMeshHole(t *testing.T) {
	var vertices []Vector3D
	var indices []uint32
	strip := func(x0, z0, x1, z1 float64) {
		i := uint32(len(vertices))
		vertices = append(vertices, V3(x0, 0, z0), V3(x0, 0, z1), V3(x1, 0, z1), V3(x1, 0, z0))
		indices = append(indices, i, i+1, i+2, i, i+2, i+3)
	}
	strip(-5, -5, -1, 5)
	strip(1, -5, 5, 5)
	strip(-1, -5, 1, -1)
	strip(-1, 1, 1, 5)
	mesh, err := NewTriangleMesh(vertices, indices)
	if err != nil {
		t.Fatal(err)
	}
	w := NewWorld3D()
	w.AddCollider(w.CreateBody(NewBodyDef3D(BodyStatic)), NewColliderDef(mesh))
	d := NewBodyDef3D(BodyDynamic)
	d.Position = V3(0, 1, 0)
	d.Rotation = QuaternionFromAxisAngle(V3(1, 1, 1), .7)
	box := w.CreateBody(d)
	w.AddCollider(box, NewColliderDef(NewBox3D(.5, .5, .5)))
	for range 60 {
		w.Step(1.0 / 60)
		if len(w.Events()) != 0 {
			t.Fatalf("hole produced contact at %v", box.Position())
		}
	}
	if box.Position().Y >= -1 {
		t.Fatalf("box did not clear hole: %v", box.Position())
	}
}
