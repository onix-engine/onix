package ecs

import (
	"slices"
	"testing"
	"unsafe"

	"github.com/onix-engine/onix/internal/reuse"
)

func TestScopeCacheRetirementAndReactivation(t *testing.T) {
	w := NewWorld()
	spawn := w.NewSpawner1[scopeNumber]()
	e := spawn.Spawn(7)
	var idle, active QueryScope
	idle.Reset([]Entity{e})
	active.Reset([]Entity{e})
	w.SetQueryScope(&idle)
	*w.ScopeValue[scopeCacheTag, int]() = 42
	for range w.Query[scopeNumber]().All {
	}
	for range reuse.RetentionFrames {
		w.SetQueryScope(&active)
		*w.ScopeValue[scopeCacheTag, int]() = 9
		for range w.Query[scopeNumber]().All {
		}
		w.Maintain()
	}
	if idle.world != nil || idle.values != nil || idle.tables != nil || idle.directory != nil || len(idle.entities) != 1 {
		t.Fatal("idle scope retained caches or lost its borrowed membership")
	}
	w.SetQueryScope(&active)
	if *w.ScopeValue[scopeCacheTag, int]() != 9 {
		t.Fatal("active scope lost its cache")
	}
	w.SetQueryScope(&idle)
	if *w.ScopeValue[scopeCacheTag, int]() != 0 || !w.InQueryScope(e) {
		t.Fatal("retired scope did not rebuild")
	}
	idle.ReleaseCache()
	if !w.InQueryScope(e) {
		t.Fatal("explicit release lost membership")
	}
	other := NewWorld()
	otherSpawn := other.NewSpawner1[scopeNumber]()
	otherSpawn.Spawn(8)
	w.SetQueryScope(nil)
	other.SetQueryScope(&idle)
	if *other.ScopeValue[scopeCacheTag, int]() != 0 || idle.world != other || w.scopes != &active {
		t.Fatal("cross-world rebinding retained the previous cache or registration")
	}
	idle.Reset(nil)
	if idle.values != nil || idle.tables != nil || other.scopes != nil {
		t.Fatal("empty reset did not immediately release caches")
	}
}

func TestCompactScopeDirectoryAndSparseMembership(t *testing.T) {
	w := NewWorld()
	bundle := memoryBundle(12, true)
	w.PrepareBundle(bundle)
	for mask := 1; mask < 1<<12; mask++ {
		var ids []componentID
		for bit := range 12 {
			if mask&(1<<bit) != 0 {
				ids = append(ids, componentID(bit))
			}
		}
		w.archetypeForOwned(ids)
	}
	e := w.Spawn(bundle)
	var scope QueryScope
	scope.Reset([]Entity{e})
	w.SetQueryScope(&scope)
	if !w.InQueryScope(e) || len(scope.tables) != 1 {
		t.Fatal("directory allocated records for unrelated historical tables")
	}
	bytes := uintptr(cap(scope.directory))*4 + uintptr(cap(scope.tables))*unsafe.Sizeof(scopeTable{})
	t.Logf("4097 historical tables, one member: directory+table capacity = %d B", bytes)

	w.SetQueryScope(nil)
	spawn := w.NewSpawner1[scopeNumber]()
	var last Entity
	for i := range 65536 {
		last = spawn.Spawn(scopeNumber(i))
	}
	scope.Reset([]Entity{last, last})
	w.SetQueryScope(&scope)
	var got []scopeNumber
	for _, access := range w.Query[scopeNumber]().All {
		got = append(got, access.Get())
	}
	_, meta, _ := w.entityMeta(last)
	table := scope.table(meta.archetypeID)
	if !slices.Equal(got, []scopeNumber{65535}) || len(table.dense) != 0 || len(table.sparse) != 1 || cap(table.sparse) > 4 {
		t.Fatal("late sparse membership retained a dense row bitmap or duplicates")
	}
	t.Logf("one member in row 65535: row capacity = %d B", cap(table.sparse)*4)
	step := func() {
		scope.Reset(scope.entities)
		for range w.Query[scopeNumber]().All {
		}
		w.Maintain()
	}
	for range reuse.RetentionFrames + 1 {
		step()
	}
	if len(scope.tables) != 1 || !w.InQueryScope(last) {
		t.Fatal("retiring unused table records corrupted the directory")
	}
	if n := testing.AllocsPerRun(100, step); n != 0 {
		t.Fatalf("warm sparse rebuild/maintenance allocated %g", n)
	}
}

func TestTableCompactionPreservesPreparedPlansAndTicks(t *testing.T) {
	w := NewWorld()
	spawn := w.NewSpawner1[scopeNumber]()
	patch := w.NewPatch1[bool]()
	remove := w.PrepareRemoval(Type[bool]())
	entities := make([]Entity, 2048)
	for i := range entities {
		entities[i] = spawn.Spawn(scopeNumber(i))
	}
	source := spawn.plan.archetype
	column := source.column(w.registerComponent[scopeNumber]()).(*typedColumn[scopeNumber])
	since := w.ChangeTick()
	*w.GetMut[scopeNumber](entities[0]) = 9999
	added, changed := slices.Clone(column.added[:32]), slices.Clone(column.changed[:32])
	for _, e := range entities[32:] {
		if err := patch.Apply(e, true); err != nil {
			t.Fatal(err)
		}
	}
	before := cap(column.values)
	row := w.Row(entities[0])
	for range reuse.RetentionFrames + 1 {
		w.Maintain()
	}
	if cap(source.entities) > 64 || cap(column.values) > 64 || len(column.values) != 32 || !slices.Equal(column.added, added) || !slices.Equal(column.changed, changed) {
		t.Fatal("table compaction lost values/ticks or retained peak storage")
	}
	window := w.ChangesSince(since)
	changedCount := 0
	for _, access := range w.Query[scopeNumber](window.Changed[scopeNumber]()).All {
		if access.Get() != 9999 {
			t.Fatal("compacted tick ranges included an unchanged component")
		}
		changedCount++
	}
	if changedCount != 1 {
		t.Fatal("compacted tick ranges lost a changed component")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("compaction left a borrowed row valid")
			}
		}()
		w.BindComponent[scopeNumber]().Get(row)
	}()
	for _, e := range entities[:32] {
		if err := patch.Apply(e, true); err != nil {
			t.Fatal(err)
		}
	}
	for range reuse.RetentionFrames + 1 {
		w.Maintain()
	}
	if source.entities != nil || column.values != nil || column.added != nil || column.changed != nil || column.addedRanges != nil || column.changedRanges != nil {
		t.Fatal("empty table retained backing arrays")
	}
	if !remove.Apply(entities[0]) {
		t.Fatal("prepared removal failed after compaction")
	}
	spawn.Spawn(99)
	if source != spawn.plan.archetype || source.column(column.id) != column || *w.GetRef[scopeNumber](entities[0]) != 9999 {
		t.Fatal("compaction invalidated prepared table/column identities")
	}
	changes := w.ChangesSince(0)
	count := 0
	for range w.Query[scopeNumber](changes.Added[scopeNumber](), Without[bool]()).All {
		count++
	}
	if count != 2 {
		t.Fatal("compacted tick ranges did not recover after prepared reuse")
	}
	t.Logf("component capacity: peak %d rows, compacted 64, empty 0", before)
}

func TestMaintenanceHonorsTransientUseAndIterationGuard(t *testing.T) {
	w := NewWorld()
	spawn := w.NewSpawner1[int]()
	entities := make([]Entity, 1024)
	for i := range entities {
		entities[i] = spawn.Spawn(i)
	}
	for _, e := range entities[1:] {
		w.Despawn(e)
	}
	for range reuse.RetentionFrames - 1 {
		w.Maintain()
	}
	peak := cap(spawn.plan.archetype.entities)
	for i := range entities[1:] {
		entities[i+1] = spawn.Spawn(i)
	}
	for _, e := range entities[1:] {
		w.Despawn(e)
	}
	w.Maintain()
	if cap(spawn.plan.archetype.entities) != peak {
		t.Fatal("maintenance ignored the frame's temporary peak")
	}
	for range w.Query[int]().All {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatal("maintenance moved storage during iteration")
				}
			}()
			w.Maintain()
		}()
	}
}

func TestSmallEmptyTableReleasesStorage(t *testing.T) {
	w := NewWorld()
	spawn := w.NewSpawner1[scopeNumber]()
	e := spawn.Spawn(1)
	w.Despawn(e)
	for range reuse.RetentionFrames {
		w.Maintain()
	}
	if cap(spawn.plan.archetype.entities) != 0 || cap(spawn.a.values) != 0 {
		t.Fatal("small empty table retained its component arrays")
	}
}

func TestScopeRepresentationChangesPreserveMembership(t *testing.T) {
	w := NewWorld()
	spawn := w.NewSpawner1[scopeNumber]()
	entities := make([]Entity, 4096)
	for i := range entities {
		entities[i] = spawn.Spawn(scopeNumber(i))
	}
	var scope QueryScope
	w.SetQueryScope(&scope)
	for _, members := range [][]Entity{entities, entities[:1], entities, {entities[63], entities[64], entities[4095], entities[4095]}} {
		scope.Reset(members)
		want := make(map[Entity]bool)
		for _, e := range members {
			want[e] = true
		}
		count := 0
		for e := range w.Query[scopeNumber]().All {
			if !want[e] {
				t.Fatal("representation change included an outsider")
			}
			count++
		}
		if count != len(want) {
			t.Fatal("representation change lost or duplicated members")
		}
		step := func() {
			scope.Reset(members)
			for range w.Query[scopeNumber]().All {
			}
		}
		if n := testing.AllocsPerRun(100, step); n != 0 {
			t.Fatalf("stable representation rebuilt with %g allocations", n)
		}
	}
}

func TestEmptyLargeComponentTableReleasesCapacity(t *testing.T) {
	w := NewWorld()
	spawn := w.NewSpawner1[[256]byte]()
	spawn.Reserve(10000)
	entities := make([]Entity, 10000)
	for i := range entities {
		entities[i] = spawn.Spawn([256]byte{})
	}
	a, c := spawn.plan.archetype, spawn.a
	bytes := func() uintptr {
		return uintptr(cap(a.entities))*unsafe.Sizeof(Entity(0)) + uintptr(cap(c.values))*256 + uintptr(cap(c.added)+cap(c.changed))*unsafe.Sizeof(Tick(0)) + uintptr(cap(c.addedRanges)+cap(c.changedRanges))*unsafe.Sizeof(tickRange{})
	}
	before := bytes()
	for _, e := range entities {
		if err := w.Insert(e, true); err != nil {
			t.Fatal(err)
		}
	}
	for range reuse.RetentionFrames {
		w.Maintain()
	}
	if after := bytes(); after != 0 {
		t.Fatalf("empty table retains %d B", after)
	}
	t.Logf("10000 migrated 256 B components: empty source capacity %d -> 0 B", before)
}
