package ecs

import "testing"

func TestFilteredQueriesDoNotAllocateAfterWarmup(t *testing.T) {
	world := NewWorld()
	spawner := world.NewSpawner2[testPosition, testVelocity]()
	spawner.Spawn(testPosition{}, testVelocity{})
	context := &SystemContext{World: world, thisRun: world.changeTick}
	visit := func(Entity, *testPosition) bool { return true }
	for name, query := range map[string]func(){
		"with": func() {
			for entity, access := range world.Query[testPosition](With[testVelocity]()).All {
				if !visit(entity, access.Ref()) {
					break
				}
			}
		},
		"without": func() {
			for entity, access := range world.Query[testPosition](Without[testAccessC]()).All {
				if !visit(entity, access.Ref()) {
					break
				}
			}
		},
		"added": func() {
			for entity, access := range world.Query[testPosition](context.Added[testPosition]()).All {
				if !visit(entity, access.Ref()) {
					break
				}
			}
		},
		"changed": func() {
			for entity, access := range world.Query[testPosition](context.Changed[testPosition]()).All {
				if !visit(entity, access.Ref()) {
					break
				}
			}
		},
		"combined": func() {
			for entity, access := range world.Query[testPosition](With[testVelocity](), Without[testAccessC](), context.Added[testPosition](), context.Changed[testVelocity]()).All {
				if !visit(entity, access.Ref()) {
					break
				}
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			query()
			if allocations := testing.AllocsPerRun(100, query); allocations != 0 {
				t.Fatalf("allocations = %v, want 0", allocations)
			}
		})
	}
}

func TestFilteredQueryKeepsSnapshotDuringNestedQueries(t *testing.T) {
	world := NewWorld()
	positions := world.NewSpawner1[testPosition]()
	positions.Spawn(testPosition{})
	moving := world.NewSpawner2[testPosition, testVelocity]()
	moving.Spawn(testPosition{}, testVelocity{})
	context := &SystemContext{World: world, thisRun: world.changeTick}
	view := world.Query[testPosition](context.Changed[testPosition]())
	context.lastRun = context.thisRun
	context.thisRun = world.nextChangeTick()
	for range 2 {
		count := 0
		for range view.All {
			count++
			for range world.Query[testPosition](context.Changed[testPosition](), Without[testVelocity]()).All {
				t.Fatal("nested query matched an unchanged component")
				break
			}
		}
		if count != 2 {
			t.Fatalf("outer query matched %d entities, want 2", count)
		}
	}
}

func TestFilteredQueryBindsColumnsAfterStructuralChanges(t *testing.T) {
	world := NewWorld()
	spawner := world.NewSpawner1[testPosition]()
	entity := spawner.Spawn(testPosition{})
	context := &SystemContext{World: world, thisRun: world.nextChangeTick()}
	view := world.Query[testPosition](context.Added[testPosition]())
	count := 0
	visit := func(Entity, *testPosition) bool { count++; return true }
	for entity, access := range view.All {
		if !visit(entity, access.Ref()) {
			break
		}
	}
	if err := world.Insert(entity, testVelocity{}); err != nil {
		t.Fatal(err)
	}
	for range 128 {
		spawner.Spawn(testPosition{})
	}
	context.thisRun = world.nextChangeTick()
	count = 0
	for entity, access := range world.Query[testPosition](context.Added[testPosition]()).All {
		if !visit(entity, access.Ref()) {
			break
		}
	}
	if count != 129 {
		t.Fatalf("matched %d entities after column growth and migration, want 129", count)
	}
}

func TestFilteredQuerySupportsMoreThanInlineFilters(t *testing.T) {
	world := NewWorld()
	spawner := world.NewSpawner1[testPosition]()
	spawner.Spawn(testPosition{})
	context := &SystemContext{World: world, thisRun: world.changeTick}
	options := make([]QueryOption, 12)
	for index := range options {
		options[index] = context.Changed[testPosition]()
	}
	view := world.Query[testPosition](options...)
	for range 2 {
		count := 0
		for range view.All {
			count++
		}
		if count != 1 {
			t.Fatalf("matched %d entities, want 1", count)
		}
	}
	unmatched := *context
	unmatched.lastRun = unmatched.thisRun
	options[len(options)-1] = unmatched.Changed[testPosition]()
	for range world.Query[testPosition](options...).All {
		t.Fatal("last filter was ignored")
		break
	}
}

func TestQueryPlanCanonicalizesMasksAndOwnsStorage(t *testing.T) {
	world := NewWorld()
	spec := querySpec{required: componentMask{1}, without: componentMask{2}}
	plan := world.queryPlan(spec)
	spec.required[0] = 4
	spec.without[0] = 8
	if plan.spec.required[0] != 1 || plan.spec.without[0] != 2 {
		t.Fatal("plan retained mutable scratch masks")
	}
	if world.queryPlan(querySpec{required: componentMask{1, 0, 0}, without: componentMask{2, 0}}) != plan {
		t.Fatal("trailing zero words created a duplicate plan")
	}
}

func BenchmarkFilteredQueryTenThousand(b *testing.B) {
	world := NewWorld()
	spawner := world.NewSpawner2[benchmarkPosition, benchmarkVelocity]()
	spawner.Reserve(10_000)
	for range 10_000 {
		spawner.Spawn(benchmarkPosition{}, benchmarkVelocity{})
	}
	context := &SystemContext{World: world, thisRun: world.changeTick}
	for _, changed := range []bool{false, true} {
		name := "With"
		if changed {
			name = "Changed"
		}
		b.Run(name, func(b *testing.B) {
			option := With[benchmarkVelocity]()
			if changed {
				option = context.Changed[benchmarkPosition]()
			}
			var total float32
			visit := func(_ Entity, position *benchmarkPosition) bool { total += position.X; return true }
			for entity, access := range world.Query[benchmarkPosition](option).All {
				if !visit(entity, access.Ref()) {
					break
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				for entity, access := range world.Query[benchmarkPosition](option).All {
					if !visit(entity, access.Ref()) {
						break
					}
				}
			}
			benchmarkTotal = total
		})
	}
}
