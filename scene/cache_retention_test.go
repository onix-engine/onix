package scene

import (
	"reflect"
	"testing"
	"unsafe"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/internal/entitycache"
	"github.com/diiyw/onix/internal/reuse"
)

func entityStoreBytes[T any](store *entitycache.Store[T]) uintptr {
	value := reflect.ValueOf(store).Elem()
	var total uintptr
	for _, name := range []string{"entries", "indices"} {
		field := value.FieldByName(name)
		total += uintptr(field.Cap()) * field.Type().Elem().Size()
	}
	return total
}

func TestTransformHierarchyAndPickReclaimPeaks(t *testing.T) {
	w := ecs.NewWorld()
	spawn := w.NewSpawner5[Transform, UITransform, InputTarget2D, Visibility, Size2D]()
	entities := make([]ecs.Entity, 10000)
	for i := range entities {
		entities[i] = spawn.Spawn(TransformFromXYZ(float32(i), 0, 0), NewUITransform(), InputTarget2D{Width: 20, Height: 20}, Visibility{Visible: true}, Size2D{Width: 20, Height: 20})
	}
	pick := w.ScopeValue[inputPickCache2D, inputPickCache2D]()
	frame := func() {
		if err := SyncGlobalTransformSystem("").Run(w); err != nil {
			t.Fatal(err)
		}
		if err := SyncUIGlobalTransformSystem("").Run(w); err != nil {
			t.Fatal(err)
		}
		pick.refresh(w, 0, 0)
		w.Maintain()
	}
	frame()
	spatial := w.ScopeValue[transformWorkspaceTag, transformWorkspace]()
	ui := w.ScopeValue[uiTransformWorkspaceScratch, uiTransformWorkspace]()
	h := HierarchyFor(w)
	peakTransform, peakPick := entityStoreBytes(&spatial.nodes), uintptr(cap(pick.targets))*unsafe.Sizeof(inputPickTarget2D{})
	for _, e := range entities[100:] {
		w.Despawn(e)
	}
	for range reuse.RetentionFrames*2 + 4 {
		frame()
	}
	if spatial.nodes.Len() != 100 || ui.nodes.Len() != 100 || cap(spatial.queue) > 200 || cap(ui.queue) > 200 || cap(h.nodes) > 200 || cap(h.entities) > 200 || cap(pick.targets) > 400 {
		t.Fatal("transform/hierarchy/pick retained historical peak arrays")
	}
	for i, e := range entities[:100] {
		if w.GetRef[GlobalTransform](e).Matrix[12] != float32(i) || spatial.nodes.Get(e) == nil || ui.nodes.Get(e) == nil {
			t.Fatal("shrinking dense caches lost live transforms")
		}
	}
	if n := testing.AllocsPerRun(100, frame); n != 0 {
		t.Fatalf("stable reclaimed scene allocated %g", n)
	}
	t.Logf("transform store: %d -> %d B; pick targets: %d -> %d B", peakTransform, entityStoreBytes(&spatial.nodes), peakPick, uintptr(cap(pick.targets))*unsafe.Sizeof(inputPickTarget2D{}))
	t.Logf("record sizes transform/UI/renderItem/paint/pick = %d/%d/%d/%d/%d B", unsafe.Sizeof(transformNode{}), unsafe.Sizeof(uiTransformNode{}), unsafe.Sizeof(renderItem2D{}), unsafe.Sizeof(paintItem2D{}), unsafe.Sizeof(inputPickTarget2D{}))
}

func TestRenderHierarchyReclaimsPeakThroughWorldMaintenance(t *testing.T) {
	world, entities := hierarchyFixture(4096)
	if err := world.Insert(entities[0], PainterGroup2D(7)); err != nil {
		t.Fatal(err)
	}
	cache := frameRenderHierarchy2DCachePtr(world)
	for _, entity := range entities {
		if cache.resolve(world, entity).group != 7 {
			t.Fatal("hierarchy fixture lost inherited group")
		}
	}
	world.Maintain()
	old := cache.values
	for _, entity := range entities[4:] {
		world.Despawn(entity)
	}
	frame := func() {
		cache = frameRenderHierarchy2DCachePtr(world)
		for _, entity := range entities[:4] {
			if cache.resolve(world, entity).group != 7 {
				t.Fatal("shrinking hierarchy lost inherited group")
			}
		}
		world.Maintain()
	}
	for range reuse.RetentionFrames + 2 {
		frame()
	}
	probe := ^ecs.Entity(0)
	old[probe] = renderHierarchy2DInfo{}
	if _, ok := cache.values[probe]; ok || len(cache.values) != 4 {
		t.Fatal("world maintenance retained the historical render hierarchy map")
	}
	if n := testing.AllocsPerRun(100, frame); n != 0 {
		t.Fatalf("reclaimed hierarchy allocated %g", n)
	}
}
