package scene

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/go-gl/mathgl/mgl32"
)

type cacheOptimizationMarker struct{}

func TestPickAndRenderSnapshotsIgnoreMigration(t *testing.T) {
	world, _, _, entities := incrementalRenderFixture(t, 1000, false)
	if err := world.Insert(entities[0], InputTarget2D{Width: 10, Height: 5}); err != nil {
		t.Fatal(err)
	}
	cache := world.ScopeValue[inputPickCache2D, inputPickCache2D]()
	cache.refresh(world, 0, 0)
	context := &ecs.SystemContext{World: world}
	if err := extractRender2D(context); err != nil {
		t.Fatal(err)
	}
	retained := RenderWorldFor(world).Items2D[0].sprite
	for _, insert := range []bool{true, false} {
		if insert {
			if err := world.Insert(entities[0], cacheOptimizationMarker{}); err != nil {
				t.Fatal(err)
			}
		} else {
			world.Remove[cacheOptimizationMarker](entities[0])
		}
		if cache.refresh(world, 0, 0) {
			t.Fatal("unrelated migration rebuilt pick geometry")
		}
		if err := extractRender2D(context); err != nil {
			t.Fatal(err)
		}
		state := RenderWorldFor(world)
		if state.Preparation2D.Extracted != 0 || state.Preparation2D.Reused != 1000 || state.Preparation2D.Sorted || state.Items2D[0].sprite != retained {
			t.Fatalf("unrelated migration rebuilt render snapshots: %+v", state.Preparation2D)
		}
	}
	// Rows have moved twice; subsequent value writes must still be observed.
	world.GetMut[UIGlobalTransform](entities[0]).Matrix[12] = 40
	if !cache.refresh(world, 0, 0) || !inputTargetContains2D(&cache.targets[0], 45, 2) {
		t.Fatal("migrated target stopped tracking transform writes")
	}
	world.GetMut[Sprite2D](entities[0]).Alpha = .5
	if err := extractRender2D(context); err != nil {
		t.Fatal(err)
	}
	if state := RenderWorldFor(world); state.Preparation2D.Extracted != 1 || state.Items2D[0].sprite.Alpha != .5 {
		t.Fatalf("migrated sprite stopped tracking value writes: %+v", state.Preparation2D)
	}
}

func TestRenderHierarchySharedAcrossScopes(t *testing.T) {
	world, entities := hierarchyFixture(7)
	if err := world.Insert(entities[0], PainterGroup2D(7)); err != nil {
		t.Fatal(err)
	}
	var scopes [2]ecs.QueryScope
	var previous *renderHierarchy2DCache
	for i := range scopes {
		scopes[i].Reset(entities[3+i : 4+i])
		world.SetQueryScope(&scopes[i])
		cache := frameRenderHierarchy2DCachePtr(world)
		if previous != nil && previous != cache {
			t.Fatal("scope duplicated world-owned inherited state")
		}
		if info := cache.resolve(world, entities[3+i]); info.group != 7 {
			t.Fatal("scope lost inherited properties from an excluded ancestor")
		}
		previous = cache
	}
	*world.GetMut[PainterGroup2D](entities[0]) = 9
	if info := frameRenderHierarchy2DCachePtr(world).resolve(world, entities[4]); info.group != 9 {
		t.Fatal("shared cache retained stale inherited values")
	}
	world.GetMut[Parent](entities[0]).Entity = entities[4]
	frameRenderHierarchy2DCachePtr(world).resolve(world, entities[4])
	world.Remove[Parent](entities[0])
	if info := frameRenderHierarchy2DCachePtr(world).resolve(world, entities[4]); info.group != 9 {
		t.Fatal("repairing a cycle left the cache unresolved")
	}
}

func TestPickTransformUpdatesMatchCollection(t *testing.T) {
	world := ecs.NewWorld()
	camera := spawnInputCameraForTest(t, world, 200, 100)
	spawn := world.NewSpawner5[InputTarget2D, InputBlocker2D, GlobalTransform, UIGlobalTransform, Visibility]()
	var entities [3]ecs.Entity
	for i := range entities {
		entities[i] = spawn.Spawn(InputTarget2D{Width: 20, Height: 20}, InputBlocker2D{Width: 10, Height: 10},
			GlobalTransform{Matrix: mgl32.Translate3D(float32(i*10), 0, float32(i))},
			UIGlobalTransform{Matrix: mgl32.Translate3D(float32(i*10), 0, float32(i))}, Visibility{Visible: true})
	}
	cache := world.ScopeValue[inputPickCache2D, inputPickCache2D]()
	cache.refresh(world, entities[0], camera)
	check := func() {
		t.Helper()
		cache.refresh(world, entities[0], camera)
		want := collectInputTargets2D(world, nil)
		if len(cache.targets) != len(want) {
			t.Fatalf("target count=%d, want %d", len(cache.targets), len(want))
		}
		for i := range want {
			// Collection sequence can change when rows migrate; entity order and
			// the relative order of an entity's target/blocker records must agree.
			want[i].sequence = cache.targets[i].sequence
			if !reflect.DeepEqual(cache.targets[i], want[i]) {
				t.Fatalf("target %d differs from full collection: got %+v, want %+v", i, cache.targets[i], want[i])
			}
		}
		lookup := inputCaptureLookup2D{entity: entities[0], cameraEntity: camera}
		collectInputCameras2D(world, &lookup)
		for i := range want {
			lookup.consider(&want[i])
		}
		lookup.target.sequence = cache.lookup.target.sequence
		if !reflect.DeepEqual(cache.lookup, lookup) {
			t.Fatal("capture lookup differs from full collection")
		}
	}
	// Repeated writes in one system share a tick. Exercise both directions of
	// depth reordering, clipping, affine transforms and a singular matrix.
	system := ecs.SystemConfig{Name: "pick-transform-updates", System: func(*ecs.SystemContext) error {
		for _, depth := range []float32{0, 8, -8, 0} {
			world.GetMut[GlobalTransform](entities[0]).Matrix = mgl32.Translate3D(12, 9, depth).Mul4(mgl32.HomogRotate3DZ(.5))
			ui := world.GetMut[UIGlobalTransform](entities[0])
			ui.Matrix = mgl32.Translate3D(14, 6, depth).Mul4(mgl32.Scale3D(2, .5, 1))
			ui.clip = clipRect2D{valid: true, minX: 0, minY: 0, maxX: 30, maxY: 30}
			check()
		}
		world.GetMut[UIGlobalTransform](entities[1]).Matrix = mgl32.Mat4{}
		check()
		return nil
	}}
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	world.Remove[UIGlobalTransform](entities[0])
	check()
	if err := world.Insert(entities[0], UIGlobalTransform{Matrix: mgl32.Ident4()}); err != nil {
		t.Fatal(err)
	}
	check()
	world.GetMut[Camera](camera).Order++
	check()
	for range 2 {
		world.GetMut[UIGlobalTransform](entities[0]).Matrix[12]++
		cache.refresh(world, entities[0], camera)
	}
	if allocations := testing.AllocsPerRun(20, func() {
		world.GetMut[UIGlobalTransform](entities[0]).Matrix[12]++
		cache.refresh(world, entities[0], camera)
	}); allocations != 0 {
		t.Fatalf("warm transform updates allocated %v times", allocations)
	}
}

func TestInheritedRenderChangesShareTraversal(t *testing.T) {
	world, _, _, entities := incrementalRenderFixture(t, 100, false)
	parents := world.NewSpawner3[Parent, RenderLayers, UITargetCamera]()
	parent := parents.Spawn(Parent{}, RenderLayers{}, UITargetCamera{})
	if err := world.Insert(entities[0], Parent{Entity: parent}); err != nil {
		t.Fatal(err)
	}
	if err := world.Insert(entities[1], Parent{Entity: entities[0]}); err != nil {
		t.Fatal(err)
	}
	context := &ecs.SystemContext{World: world}
	if err := extractRender2D(context); err != nil {
		t.Fatal(err)
	}
	cache := world.ScopeValue[retainedRender2D, retainedRender2D]()
	generation := cache.hierarchyGeneration
	*world.GetMut[RenderLayers](parent) = RenderLayer(1)
	world.GetMut[UITargetCamera](parent).Entity = entities[99]
	if err := extractRender2D(context); err != nil {
		t.Fatal(err)
	}
	if cache.hierarchyGeneration != generation+1 || len(cache.hierarchyQueue) != 3 {
		t.Fatalf("inherited changes repeated traversal: generation=%d, nodes=%d", cache.hierarchyGeneration-generation, len(cache.hierarchyQueue))
	}
	if cache.stats.Extracted != 2 || cache.stats.Reused != 98 {
		t.Fatalf("inherited changes rebuilt unrelated content: %+v", cache.stats)
	}
	for i := range 2 {
		if item := cache.items[i]; item.target != entities[99] || !item.layers.Contains(1) {
			t.Fatal("merged traversal missed inherited properties")
		}
	}
}

func TestRetainedRenderSkipsIntermediateComponentCopies(t *testing.T) {
	world, canvas, _, entities := incrementalRenderFixture(t, 100, false)
	context := &ecs.SystemContext{World: world}
	if err := extractRender2D(context); err != nil {
		t.Fatal(err)
	}
	intermediate := world.FrameValue[renderSnapshots2DScratch, renderSnapshots2D]()
	if cap(intermediate.sprites) != 0 {
		t.Fatal("retained extraction allocated intermediate component copies")
	}
	scratch := world.FrameScratch[renderItems2DScratch, renderItem2D]()
	for _, item := range scratch.Values[:cap(scratch.Values)] {
		if item.sprite != nil || item.text != nil || item.emitter != nil || item.tilemap != nil || item.spine != nil {
			t.Fatal("temporary render items retained borrowed component references")
		}
	}
	retained := RenderWorldFor(world).Items2D[0].sprite
	direct := collectRenderItems2D(canvas, world, false)[0].sprite
	source := world.GetMut[Sprite2D](entities[0])
	source.Alpha = .25
	if retained == source || direct == source || retained.Alpha != 1 || direct.Alpha != 1 {
		t.Fatal("render snapshots alias mutable component storage")
	}
}

func TestRetainedTextPreservesCollectorState(t *testing.T) {
	world, canvas, _, entities := incrementalRenderFixture(t, 1, false)
	RegisterTextRenderer2D(world)
	text := Text2D{Value: "World and UI", Font: outputTestFont(t), FontSize: 18, DPI: 72, Color: graphics.White, Alpha: 1,
		Spans: []TextSpan2D{{Start: 0, End: 5, Color: graphics.White}}}
	if err := world.Insert(entities[0], text); err != nil {
		t.Fatal(err)
	}
	if err := world.Insert(entities[0], GlobalTransform{Matrix: mgl32.Ident4()}); err != nil {
		t.Fatal(err)
	}
	items := collectRenderSnapshots2D(canvas, world, false, true)
	source := world.GetRef[Text2D](entities[0])
	var worldText, uiText *textPaint2D
	for _, item := range items {
		if item.text == nil {
			continue
		}
		if item.ui {
			uiText = item.text
		} else {
			worldText = item.text
		}
	}
	if worldText == nil || uiText == nil || worldText == uiText {
		t.Fatal("world/UI text collection must freeze each occurrence's layout state")
	}
	intermediate := world.FrameValue[renderSnapshots2DScratch, renderSnapshots2D]()
	if len(intermediate.texts) != 2 || worldText.cache != source.cache {
		t.Fatal("intermediate text collection copied nested inputs")
	}
	clear(items)
	if err := extractRender2D(&ecs.SystemContext{World: world}); err != nil {
		t.Fatal(err)
	}
	source.Spans[0].Color = graphics.Black
	source.Alpha = .25
	for _, item := range RenderWorldFor(world).Items2D {
		if item.text != nil && (item.color != graphics.White || item.text.Alpha != 1) {
			t.Fatal("retained text aliases mutable component spans")
		}
	}
	if err := world.Insert(entities[0], cacheOptimizationMarker{}); err != nil {
		t.Fatal(err)
	}
	if err := extractRender2D(&ecs.SystemContext{World: world}); err != nil {
		t.Fatal(err)
	}
	cache := world.ScopeValue[retainedRender2D, retainedRender2D]()
	if cache.stats.Extracted != 0 || cache.nodes[entities[0]].sourceText != nil {
		t.Fatal("text migration rebuilt snapshots or retained an ECS column pointer")
	}
}

func TestRenderOrderChangesStayLocal(t *testing.T) {
	world, _, _, entities := incrementalRenderFixture(t, 100, false)
	parents := world.NewSpawner1[RenderOrder2D]()
	parent := parents.Spawn(1)
	if err := world.Insert(entities[0], Parent{Entity: parent}); err != nil {
		t.Fatal(err)
	}
	if err := world.Insert(entities[0], GlobalTransform{Matrix: mgl32.Ident4()}); err != nil {
		t.Fatal(err)
	}
	if err := world.Insert(entities[0], RenderOrder2D(2)); err != nil {
		t.Fatal(err)
	}
	context := &ecs.SystemContext{World: world}
	if err := extractRender2D(context); err != nil {
		t.Fatal(err)
	}
	*world.GetMut[RenderOrder2D](parent) = 9
	if err := extractRender2D(context); err != nil {
		t.Fatal(err)
	}
	state := RenderWorldFor(world)
	if state.Preparation2D.Extracted != 0 || state.Preparation2D.Sorted {
		t.Fatalf("ancestor's local order rebuilt descendants: %+v", state.Preparation2D)
	}
	*world.GetMut[RenderOrder2D](entities[0]) = 3
	if err := extractRender2D(context); err != nil {
		t.Fatal(err)
	}
	if state.Items2D[0].sortIndex != 3 {
		t.Fatal("local order change was not reflected in world rendering")
	}
}

func BenchmarkPickSnapshotRefresh(b *testing.B) {
	for _, count := range []int{1000, 10000} {
		for _, mode := range []string{"stable", "move-one", "migrate-one"} {
			b.Run(fmt.Sprintf("%d/%s", count, mode), func(b *testing.B) {
				world := ecs.NewWorld()
				spawn := world.NewSpawner3[InputTarget2D, UIGlobalTransform, Visibility]()
				var first ecs.Entity
				for i := range count {
					entity := spawn.Spawn(InputTarget2D{Width: 20, Height: 20}, UIGlobalTransform{Matrix: mgl32.Translate3D(float32(i), 0, 0)}, Visibility{Visible: true})
					if i == 0 {
						first = entity
					}
				}
				cache := world.ScopeValue[inputPickCache2D, inputPickCache2D]()
				cache.refresh(world, 0, 0)
				if err := world.Insert(first, cacheOptimizationMarker{}); err != nil {
					b.Fatal(err)
				}
				cache.refresh(world, 0, 0)
				world.Remove[cacheOptimizationMarker](first)
				cache.refresh(world, 0, 0)
				present := false
				b.ReportAllocs()
				for b.Loop() {
					switch mode {
					case "move-one":
						world.GetMut[UIGlobalTransform](first).Matrix[12]++
					case "migrate-one":
						if present {
							world.Remove[cacheOptimizationMarker](first)
						} else if err := world.Insert(first, cacheOptimizationMarker{}); err != nil {
							b.Fatal(err)
						}
						present = !present
					}
					cache.refresh(world, 0, 0)
				}
			})
		}
	}
}
