package scene

import (
	"math"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/go-gl/mathgl/mgl32"
)

func TestInputPickCacheRefresh(t *testing.T) {
	world := ecs.NewWorld()
	camera := spawnInputCameraForTest(t, world, 200, 100)
	parents := world.NewSpawner1[Activity]()
	parent := parents.Spawn(Activity{Active: true})
	spawner := world.NewSpawner4[InputTarget2D, UIGlobalTransform, Visibility, Parent]()
	entity := spawner.Spawn(InputTarget2D{Width: 20, Height: 20}, UIGlobalTransform{Matrix: mgl32.Ident4()}, Visibility{Visible: true}, Parent{Entity: parent})
	cache := world.ScopeValue[inputPickCache2D, inputPickCache2D]()
	if !cache.refresh(world, 0, 0) || len(cache.targets) != 1 {
		t.Fatal("initial pick snapshot missing")
	}
	generation := cache.generation
	if cache.refresh(world, entity, camera) || cache.generation != generation || !cache.lookup.targetFound {
		t.Fatal("capture lookup rebuilt unchanged geometry or missed the target")
	}
	// Access to unrelated state does not require another geometry snapshot.
	states := world.NewSpawner1[InputState2D]()
	states.Spawn(InputState2D{})
	if cache.refresh(world, entity, camera) {
		t.Fatal("unrelated component invalidated geometry")
	}
	if allocations := testing.AllocsPerRun(20, func() { cache.refresh(world, entity, camera) }); allocations != 0 {
		t.Fatalf("stable picking allocated %v times", allocations)
	}
	// All changes occur without advancing the ECS tick.
	world.GetMut[UIGlobalTransform](entity).Matrix[12] = 50
	if !cache.refresh(world, entity, camera) || inputTargetContains2D(&cache.targets[0], 0, 0) || !inputTargetContains2D(&cache.targets[0], 50, 0) {
		t.Fatal("same-tick target movement did not refresh geometry")
	}
	world.GetMut[GlobalTransform](camera).Matrix[12] = 100
	if !cache.refresh(world, entity, camera) || cache.cameras[0].transform.Matrix[12] != 100 {
		t.Fatal("same-tick camera movement did not refresh geometry")
	}
	world.GetMut[Activity](parent).Active = false
	if !cache.refresh(world, entity, camera) || len(cache.targets) != 0 || cache.lookup.targetFound {
		t.Fatal("inactive parent retained an input target")
	}
	world.GetMut[Activity](parent).Active = true
	if !cache.refresh(world, entity, camera) || len(cache.targets) != 1 {
		t.Fatal("reactivated parent did not restore its input target")
	}
	world.Remove[InputTarget2D](entity)
	if !cache.refresh(world, entity, camera) || len(cache.targets) != 0 {
		t.Fatal("removed target retained")
	}
	world.Despawn(entity)
	replacement := spawner.Spawn(InputTarget2D{Width: 40, Height: 40}, UIGlobalTransform{Matrix: mgl32.Ident4()}, Visibility{Visible: true}, Parent{Entity: parent})
	if !cache.refresh(world, replacement, camera) || len(cache.targets) != 1 || cache.targets[0].entity != replacement || cache.targets[0].width != 40 {
		t.Fatal("entity replacement retained stale geometry")
	}
}

func TestInputPickCacheScopes(t *testing.T) {
	world := ecs.NewWorld()
	camera := spawnInputCameraForTest(t, world, 200, 100)
	spawner := world.NewSpawner3[InputTarget2D, UIGlobalTransform, Visibility]()
	first := spawner.Spawn(InputTarget2D{Width: 20, Height: 20}, UIGlobalTransform{Matrix: mgl32.Ident4()}, Visibility{Visible: true})
	second := spawner.Spawn(InputTarget2D{Width: 40, Height: 40}, UIGlobalTransform{Matrix: mgl32.Ident4()}, Visibility{Visible: true})
	var scopes [2]ecs.QueryScope
	scopes[0].Reset([]ecs.Entity{camera, first})
	scopes[1].Reset([]ecs.Entity{camera, second})
	var caches [2]*inputPickCache2D
	for i := range scopes {
		world.SetQueryScope(&scopes[i])
		caches[i] = world.ScopeValue[inputPickCache2D, inputPickCache2D]()
		caches[i].refresh(world, 0, 0)
		if len(caches[i].targets) != 1 {
			t.Fatal("scope included another scope's target")
		}
	}
	world.SetQueryScope(&scopes[0])
	if caches[0] == caches[1] || caches[0].refresh(world, 0, 0) || caches[0].targets[0].entity != first {
		t.Fatal("switching scopes discarded the original snapshot")
	}
	scopes[0].Reset([]ecs.Entity{camera, second})
	if !caches[0].refresh(world, 0, 0) || caches[0].targets[0].entity != second {
		t.Fatal("scope reset retained old targets")
	}
}

func TestPreparedInputGeometryMatchesInversePicking(t *testing.T) {
	skew := mgl32.Ident4()
	skew[4], skew[1] = .6, -.3
	projective := mgl32.Ident4()
	projective[3], projective[7] = .001, -.002
	for _, matrix := range []mgl32.Mat4{
		mgl32.Ident4(), mgl32.HomogRotate3DZ(.7), skew,
		mgl32.Scale3D(-2, .5, 1), mgl32.Scale3D(0, 1, 1),
		mgl32.HomogRotate3DX(.7), projective,
	} {
		for _, ui := range []bool{false, true} {
			for _, anchor := range []Anchor{AnchorTopLeft, AnchorCenter, AnchorBottomRight} {
				target := inputPickTarget2D{width: 23, height: 17, anchor: anchor, ui: ui}
				matrix := mgl32.Translate3D(13.5, -11.25, 0).Mul4(matrix)
				target.prepare(matrix)
				model := quadBaseTransform(matrix, anchor, target.width, target.height, ui).Mul4(mgl32.Scale3D(target.width, target.height, 1))
				for _, clipped := range []bool{false, true} {
					target.clip = clipRect2D{valid: clipped, minX: -10, minY: -10, maxX: 20, maxY: 20}
					check := func(x, y float32) {
						t.Helper()
						want := false
						if !(clipped && (x < -10 || x > 20 || y < -10 || y > 20)) && math.Abs(float64(model.Det())) > 1e-8 {
							local := model.Inv().Mul4x1(mgl32.Vec4{x, y, 0, 1})
							if local.W() != 0 {
								local = local.Mul(1 / local.W())
								want = local.X() >= 0 && local.X() <= 1 && local.Y() >= 0 && local.Y() <= 1
							}
						}
						if got := inputTargetContains2D(&target, x, y); got != want {
							t.Fatalf("pick at %g,%g = %v, want %v; ui=%v, anchor=%v, matrix=%v", x, y, got, want, ui, anchor, matrix)
						}
					}
					for x := float32(-60); x <= 60; x += 2.5 {
						for y := float32(-60); y <= 60; y += 2.5 {
							check(x, y)
						}
					}
					// Corners and their neighboring floats exercise rejection bounds.
					for _, corner := range []mgl32.Vec4{{0, 0, 0, 1}, {1, 0, 0, 1}, {0, 1, 0, 1}, {1, 1, 0, 1}} {
						point := model.Mul4x1(corner)
						if point.W() != 0 {
							x, y := point.X()/point.W(), point.Y()/point.W()
							check(x, y)
							check(math.Nextafter32(x, float32(math.Inf(-1))), y)
							check(math.Nextafter32(x, float32(math.Inf(1))), y)
						}
					}
				}
			}
		}
	}
}
