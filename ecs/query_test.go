package ecs

import "testing"

func TestFilteredQueriesDoNotAllocateAfterWarmup(t *testing.T) {
	world := NewWorld()
	spawner := NewSpawner2[testPosition, testVelocity](world)
	spawner.Spawn(testPosition{}, testVelocity{})
	context := &SystemContext{World: world, thisRun: world.changeTick}
	visit := func(Entity, *testPosition) bool { return true }
	for name, query := range map[string]func(){
		"with": func() {
			for entity, access := range Query[testPosition](world, With[testVelocity]()).All {
				if !visit(entity, access.Ref()) {
					break
				}
			}
		},
		"without": func() {
			for entity, access := range Query[testPosition](world, Without[testAccessC]()).All {
				if !visit(entity, access.Ref()) {
					break
				}
			}
		},
		"added": func() {
			for entity, access := range Query[testPosition](world, Added[testPosition](context)).All {
				if !visit(entity, access.Ref()) {
					break
				}
			}
		},
		"changed": func() {
			for entity, access := range Query[testPosition](world, Changed[testPosition](context)).All {
				if !visit(entity, access.Ref()) {
					break
				}
			}
		},
		"combined": func() {
			for entity, access := range Query[testPosition](world, With[testVelocity](), Without[testAccessC](), Added[testPosition](context), Changed[testVelocity](context)).All {
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
	positions := NewSpawner1[testPosition](world)
	positions.Spawn(testPosition{})
	moving := NewSpawner2[testPosition, testVelocity](world)
	moving.Spawn(testPosition{}, testVelocity{})
	context := &SystemContext{World: world, thisRun: world.changeTick}
	view := Query[testPosition](world, Changed[testPosition](context))
	context.lastRun = context.thisRun
	context.thisRun = world.nextChangeTick()
	for range 2 {
		count := 0
		for range view.All {
			count++
			for range Query[testPosition](world, Changed[testPosition](context), Without[testVelocity]()).All {
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
	spawner := NewSpawner1[testPosition](world)
	entity := spawner.Spawn(testPosition{})
	context := &SystemContext{World: world, thisRun: world.nextChangeTick()}
	view := Query[testPosition](world, Added[testPosition](context))
	count := 0
	visit := func(Entity, *testPosition) bool { count++; return true }
	for entity, access := range view.All {
		if !visit(entity, access.Ref()) {
			break
		}
	}
	if err := Insert(world, entity, testVelocity{}); err != nil {
		t.Fatal(err)
	}
	for range 128 {
		spawner.Spawn(testPosition{})
	}
	context.thisRun = world.nextChangeTick()
	count = 0
	for entity, access := range Query[testPosition](world, Added[testPosition](context)).All {
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
	spawner := NewSpawner1[testPosition](world)
	spawner.Spawn(testPosition{})
	context := &SystemContext{World: world, thisRun: world.changeTick}
	options := make([]QueryOption, 12)
	for index := range options {
		options[index] = Changed[testPosition](context)
	}
	view := Query[testPosition](world, options...)
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
	options[len(options)-1] = Changed[testPosition](&unmatched)
	for range Query[testPosition](world, options...).All {
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
	spawner := NewSpawner2[benchmarkPosition, benchmarkVelocity](world)
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
				option = Changed[benchmarkPosition](context)
			}
			var total float32
			visit := func(_ Entity, position *benchmarkPosition) bool { total += position.X; return true }
			for entity, access := range Query[benchmarkPosition](world, option).All {
				if !visit(entity, access.Ref()) {
					break
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				for entity, access := range Query[benchmarkPosition](world, option).All {
					if !visit(entity, access.Ref()) {
						break
					}
				}
			}
			benchmarkTotal = total
		})
	}
}
