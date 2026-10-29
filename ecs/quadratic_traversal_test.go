package ecs

import (
	"fmt"
	"slices"
	"testing"
)

func TestScheduleNewlyReadySystemsKeepLexicalOrder(t *testing.T) {
	schedule := NewSchedule("ready_order")
	log := make([]string, 0, 9)
	configs := []SystemConfig{
		{Name: "z"},
		{Name: "f", Before: []string{"c"}},
		{Name: "e", After: []string{"c"}},
		{Name: "d"},
		{Name: "c", After: []string{"a", "f", "f"}},
		{Name: "b"},
		{Name: "aa", After: []string{"a"}},
		{Name: "a", After: []string{"b"}},
	}
	for _, config := range configs {
		config.System = func(*SystemContext) error {
			log = append(log, config.Name)
			return nil
		}
		if err := schedule.Add(config); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"b", "a", "aa", "d", "f", "c", "e", "z"}
	assertOrder := func(want []string) {
		t.Helper()
		order, err := schedule.Order()
		if err != nil || !slices.Equal(order, want) {
			t.Fatalf("order = %v, error = %v, want %v", order, err, want)
		}
	}
	assertOrder(want)
	if err := schedule.Add(SystemConfig{Name: "0", After: []string{"d"}, System: func(*SystemContext) error { return nil }}); err != nil {
		t.Fatal(err)
	}
	assertOrder([]string{"b", "a", "aa", "d", "0", "f", "c", "e", "z"})
	schedule.Remove("0")
	assertOrder(want)
	world := NewWorld()
	run := func() {
		log = log[:0]
		if err := schedule.Run(world); err != nil {
			panic(err)
		}
	}
	if allocations := testing.AllocsPerRun(100, run); allocations != 0 {
		t.Fatalf("compiled schedule allocated %v times", allocations)
	}
	if !slices.Equal(log, want) {
		t.Fatalf("execution = %v, want %v", log, want)
	}
}

func TestScheduleRejectsCyclesAndMissingReferences(t *testing.T) {
	for _, test := range []struct {
		name    string
		configs []SystemConfig
	}{
		{"missing_after", []SystemConfig{{Name: "a", After: []string{"missing"}}}},
		{"missing_before", []SystemConfig{{Name: "a", Before: []string{"missing"}}}},
		{"cycle", []SystemConfig{{Name: "a", After: []string{"b"}}, {Name: "b", After: []string{"a"}}, {Name: "ready"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			schedule := NewSchedule(test.name)
			ran := false
			for _, config := range test.configs {
				config.System = func(*SystemContext) error { ran = true; return nil }
				if err := schedule.Add(config); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := schedule.Order(); err == nil {
				t.Fatal("invalid schedule compiled")
			}
			if err := schedule.Run(NewWorld()); err == nil || ran {
				t.Fatal("invalid schedule executed a system")
			}
		})
	}
}

// Synthetic IDs isolate bundle preparation from type registration and spawning.
type traversalBundleEntry struct {
	id      componentID
	payload int
}

func (entry traversalBundleEntry) componentID(*World) componentID { return entry.id }

func reverseTraversalBundle(count int) Bundle {
	bundle := NewBundle(count)
	for index := count - 1; index >= 0; index-- {
		bundle.entries = append(bundle.entries, traversalBundleEntry{id: componentID(index), payload: index * 3})
	}
	return bundle
}

func TestBundlePreparationKeepsEntryAssociations(t *testing.T) {
	for _, count := range []int{0, 1, 8, 32, 33, 128} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			world := NewWorld()
			bundle := reverseTraversalBundle(count)
			storage := make([]preparedBundleEntry, count)
			entries := world.prepareBundle(bundle, storage)
			for index, entry := range entries {
				value := entry.entry.(traversalBundleEntry)
				if entry.id != componentID(index) || value.id != entry.id || value.payload != index*3 {
					t.Fatalf("component %d lost its entry: %+v %+v", index, entry, value)
				}
			}
			if allocations := testing.AllocsPerRun(100, func() { world.prepareBundle(bundle, storage) }); allocations != 0 {
				t.Fatalf("preparation with storage allocated %v times", allocations)
			}
		})
	}
	world := NewWorld()
	bundle := reverseTraversalBundle(64)
	duplicate := world.registerComponent[testPosition]()
	bundle.entries = append(bundle.entries, traversalBundleEntry{id: duplicate})
	assertRemovalPanic(t, func() { world.PrepareBundle(bundle) })
}

func TestComponentTypeMaskKeepsRegistrationOrder(t *testing.T) {
	world := NewWorld()
	for _, ids := range [][]componentID{{}, {255, 0, 63, 64, 127, 128, 255}, {256, 0, 63, 64, 127, 128, 255, 256, 0}} {
		registered := make([]componentID, 0, len(ids))
		types := make([]ComponentType, len(ids))
		for index, id := range ids {
			types[index] = ComponentType{register: func(w *World) componentID {
				if w != world {
					t.Fatal("descriptor registered in another world")
				}
				registered = append(registered, id)
				return id
			}}
		}
		mask := componentTypeMask(world, types)
		ordered := slices.Clone(ids)
		slices.Sort(ordered)
		if !slices.Equal(registered, ids) || !slices.Equal(mask, maskFor(slices.Compact(ordered))) {
			t.Fatalf("IDs %v produced registration %v and mask %v", ids, registered, mask)
		}
	}
	assertRemovalPanic(t, func() { componentTypeMask(world, []ComponentType{{}}) })
}

func TestMaskGrowthClearsReusedWords(t *testing.T) {
	storage := componentMask{1, ^uint64(0), ^uint64(0), ^uint64(0)}
	mask := maskWith(storage[:1], 193)
	if !slices.Equal(mask, componentMask{1, 0, 0, 2}) {
		t.Fatalf("reused words retained old bits: %v", mask)
	}
	mask = maskWith(mask, 65)
	mask = maskWith(mask, 193)
	if !slices.Equal(mask, componentMask{1, 2, 0, 2}) {
		t.Fatalf("bits lost or duplicated: %v", mask)
	}
	if allocations := testing.AllocsPerRun(100, func() { maskWith(storage[:1], 193) }); allocations != 0 {
		t.Fatalf("growth within capacity allocated %v times", allocations)
	}
	mask = maskWith(storage[:1], 257)
	if !slices.Equal(mask, componentMask{1, 0, 0, 0, 2}) {
		t.Fatalf("new storage retained hidden bits: %v", mask)
	}
}

func TestComponentMaskEqualIgnoresTrailingZeroWords(t *testing.T) {
	for _, test := range []struct {
		left, right componentMask
		want        bool
	}{
		{nil, nil, true},
		{nil, componentMask{0, 0}, true},
		{componentMask{1, 2}, componentMask{1, 2, 0, 0}, true},
		{componentMask{1, 2, 0, 0}, componentMask{1, 2}, true},
		{componentMask{1, 2}, componentMask{1, 3, 0}, false},
		{componentMask{1}, componentMask{1, 0, 2}, false},
		{componentMask{1, 0, 2}, componentMask{1}, false},
	} {
		if got := test.left.equal(test.right); got != test.want {
			t.Fatalf("%v equal %v = %v, want %v", test.left, test.right, got, test.want)
		}
	}
}

func TestPatchInterleavesWritesAndRetainedIDs(t *testing.T) {
	world := NewWorld()
	// Register writes below, between, at, and above source IDs.
	name := world.registerComponent[testName]()
	position := world.registerComponent[testPosition]()
	visibility := world.registerComponent[testVisibility]()
	velocity := world.registerComponent[testVelocity]()
	world.registerComponent[testSize]()
	tag := world.registerComponent[testLocalTag]()
	anchor := world.registerComponent[testAnchor]()
	spawn := world.NewSpawner4[testPosition, testVelocity, testSize, testLocalTag]()
	entity := spawn.Spawn(testPosition{X: 10}, testVelocity{X: 20}, testSize{Width: 30}, testLocalTag{})
	other := spawn.Spawn(testPosition{X: 11}, testVelocity{X: 21}, testSize{Width: 31}, testLocalTag{})
	patch := world.NewPatch4[testAnchor, testVelocity, testName, testVisibility](Type[testSize](), Type[testVelocity](), Type[testName]())
	if err := patch.Apply(entity, testAnchor{9, 8}, testVelocity{X: 200}, testName{Value: "inserted"}, testVisibility{Visible: true}); err != nil {
		t.Fatal(err)
	}
	_, meta, _ := world.entityMeta(entity)
	want := []componentID{name, position, visibility, velocity, tag, anchor}
	if got := world.archetypes[meta.archetypeID].ids; !slices.Equal(got, want) {
		t.Fatalf("target components = %v, want %v", got, want)
	}
	if world.GetRef[testPosition](entity).X != 10 || world.GetRef[testVelocity](entity).X != 200 || world.GetRef[testName](entity).Value != "inserted" || !world.GetRef[testVisibility](entity).Visible || *world.GetRef[testAnchor](entity) != (testAnchor{9, 8}) || world.Has[testSize](entity) {
		t.Fatal("patch lost a retained value or wrote values in the wrong order")
	}
	if world.GetRef[testPosition](other).X != 11 || world.GetRef[testVelocity](other).X != 21 || world.GetRef[testSize](other).Width != 31 {
		t.Fatal("patch corrupted the swapped source row")
	}
}

func BenchmarkScheduleCompileTraversal(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			schedule := NewSchedule("independent")
			for index := range count {
				if err := schedule.Add(SystemConfig{Name: fmt.Sprintf("system_%06d", index), System: func(*SystemContext) error { return nil }}); err != nil {
					b.Fatal(err)
				}
			}
			if err := schedule.compile(); err != nil {
				b.Fatal(err)
			}
			order, err := schedule.Order()
			if err != nil || !slices.IsSorted(order) {
				b.Fatal("independent systems were not sorted")
			}
			b.ReportAllocs()
			for b.Loop() {
				schedule.dirty = true
				if err := schedule.compile(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkBundlePreparationTraversal(b *testing.B) {
	for _, count := range []int{8, 100, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world := NewWorld()
			bundle := reverseTraversalBundle(count)
			storage := make([]preparedBundleEntry, count)
			b.ReportAllocs()
			for b.Loop() {
				world.prepareBundle(bundle, storage)
			}
		})
	}
}

func BenchmarkMaskGrowthTraversal(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			b.ReportAllocs()
			var mask componentMask
			for b.Loop() {
				mask = nil
				for word := range count {
					mask = maskWith(mask, componentID(word*64))
				}
			}
			if len(mask) != count || mask[count-1] != 1 {
				b.Fatal("mask lost a component")
			}
		})
	}
}

func BenchmarkComponentTypeMaskTraversal(b *testing.B) {
	for _, count := range []int{8, 100, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world := NewWorld()
			types := make([]ComponentType, count)
			for index := range types {
				id := componentID(count - index - 1)
				types[index] = ComponentType{register: func(*World) componentID { return id }}
			}
			b.ReportAllocs()
			var mask componentMask
			for b.Loop() {
				mask = componentTypeMask(world, types)
			}
			if len(mask) != (count+63)/64 {
				b.Fatal("incorrect mask length")
			}
		})
	}
}
