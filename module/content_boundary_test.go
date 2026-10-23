package module

import (
	"errors"
	"strconv"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/ui"
)

type boundaryValue struct{ Number int }
type boundaryExtra struct{ Number int }

func boundaryContent(t testing.TB) (*Runtime, *Content, *Content) {
	t.Helper()
	plan, _ := compileFixture(t, map[string]string{
		"resources/scenes/main.json": `{"modules":[{"id":"a","mod":"box"},{"id":"b","mod":"box"}]}`,
		"box/mod.json":               `{}`,
	})
	r := newFixture(t, plan, Registry{}, Options{})
	s, err := r.MountEntry()
	must(t, err)
	return r, s.modules[0].Content(), s.modules[1].Content()
}

func TestProceduralDescendantsFollowPrivateLifetime(t *testing.T) {
	for _, disposeModule := range []bool{false, true} {
		t.Run(map[bool]string{false: "subtree", true: "module"}[disposeModule], func(t *testing.T) {
			r, a, b := boundaryContent(t)
			before := r.world.EntityCount()
			root := a.SpawnScrollArea(ui.ScrollAreaOptions{Width: 100, Height: 100})
			var children []ecs.Entity
			for e := range r.world.Entities() {
				if parent, ok := ecs.Get[scene.Parent](r.world, e); ok && parent.Entity == root {
					children = append(children, e)
					if !a.owns(e) || b.owns(e) {
						t.Fatal("procedural ownership crossed modules")
					}
				}
			}
			if len(children) != 4 {
				t.Fatalf("scroll parts = %d, want 4", len(children))
			}
			if disposeModule {
				must(t, a.context.Dispose())
			} else if !a.Despawn(root) {
				t.Fatal("subtree removal rejected")
			}
			for _, e := range append(children, root) {
				if r.world.IsAlive(e) || a.owns(e) || b.owns(e) {
					t.Fatal("procedural entity survived private lifetime")
				}
			}
			if !disposeModule && r.world.EntityCount() != before {
				t.Fatal("subtree removal leaked entities")
			}
			must(t, a.context.scene.Dispose())
			if r.world.EntityCount() != 0 {
				t.Fatal("scene removal leaked procedural entities")
			}
		})
	}
}

func TestContentQueryDefersWritesSpawnsAndDeletionAcrossNestedQueries(t *testing.T) {
	r, a, b := boundaryContent(t)
	first := ContentSpawnNode(a, ui.NodeOptions{}, boundaryValue{1})
	second := ContentSpawnNode(a, ui.NodeOptions{}, boundaryValue{2})
	ContentSpawnNode(b, ui.NodeOptions{}, boundaryValue{99})
	var spawned ecs.Entity
	visits := 0
	for e, access := range ContentQuery[boundaryValue](a) {
		visits++
		if e != first {
			continue
		}
		must(t, ContentInsert(a, e, boundaryValue{10}))
		must(t, ContentInsert(a, e, boundaryExtra{20}))
		bundle := ecs.NewBundle(1)
		ecs.Put(&bundle, boundaryValue{3})
		spawned = a.Spawn(bundle)
		must(t, ContentInsert(a, spawned, boundaryExtra{30}))
		must(t, ContentInsert(a, spawned, scene.Parent{Entity: second}))
		if r.world.IsAlive(spawned) || ContentHas[boundaryExtra](a, e) || access.Get().Number != 10 {
			t.Fatal("replacement was deferred or structural writes changed the query snapshot")
		}
		nested := 0
		for range ContentQuery2[boundaryValue, scene.Visibility](a) {
			nested++
		}
		if nested != 2 || !a.Despawn(e) || !r.world.IsAlive(e) {
			t.Fatal("nested query changed the outer snapshot or flushed deletion")
		}
	}
	if visits != 2 || r.world.IsAlive(first) || !r.world.IsAlive(spawned) {
		t.Fatal("query crossed modules or lost deferred structural commands")
	}
	extra, ok := ContentGet[boundaryExtra](a, spawned)
	parent, _ := ContentGet[scene.Parent](a, spawned)
	if !ok || extra.Number != 30 || parent.Entity != second {
		t.Fatal("reserved entity initialization was lost")
	}
	if r.busy || r.queryDepth != 0 || r.privateCommands != 0 || len(r.privateSpawns) != 0 {
		t.Fatal("query left its structural boundary open")
	}
}

func TestContentQueryEarlyBreakCommitsAndUpdateKeepsOuterBoundary(t *testing.T) {
	r, a, _ := boundaryContent(t)
	entity := ContentSpawnNode(a, ui.NodeOptions{}, boundaryValue{1})
	for e := range ContentQuery[boundaryValue](a) {
		must(t, ContentInsert(a, e, boundaryExtra{2}))
		break
	}
	if !ContentHas[boundaryExtra](a, entity) {
		t.Fatal("early break skipped structural commit")
	}
	a.context.update = func(Tick) error {
		for e := range ContentQuery2[boundaryValue, boundaryExtra](a) {
			if !a.Despawn(e) {
				return ErrOwnership
			}
		}
		if !r.world.IsAlive(entity) {
			return errors.New("query committed before the update boundary")
		}
		return nil
	}
	r.scopeDirty = true
	r.SyncScopes()
	must(t, r.Update(1.0/60))
	if r.world.IsAlive(entity) {
		t.Fatal("update failed to commit query deletion")
	}
}

func TestReparentCommandsPreserveSubtreeOrderAndRejectPendingCycles(t *testing.T) {
	r, a, _ := boundaryContent(t)
	left := ContentSpawnNode(a, ui.NodeOptions{}, boundaryValue{1})
	right := ContentSpawnNode(a, ui.NodeOptions{}, boundaryValue{2})
	child := ContentSpawnNode(a, ui.NodeOptions{Parent: left}, boundaryExtra{3})
	for e := range ContentQuery[boundaryValue](a) {
		if e != left {
			continue
		}
		must(t, ContentInsert(a, child, scene.Parent{Entity: right}))
		if err := ContentInsert(a, right, scene.Parent{Entity: child}); err == nil {
			t.Fatal("pending reparent cycle was accepted")
		}
		if !a.Despawn(left) {
			t.Fatal("subtree removal rejected")
		}
	}
	if r.world.IsAlive(left) || !r.world.IsAlive(child) || !r.world.IsAlive(right) {
		t.Fatal("subtree removal ignored an earlier reparent")
	}
	if !a.Despawn(right) || r.world.IsAlive(child) {
		t.Fatal("new parent did not own the moved subtree")
	}
}

func TestModuleQueriesIncludeProceduralDescendantsAndAllocateNothingWarm(t *testing.T) {
	_, a, b := boundaryContent(t)
	root := a.SpawnScrollArea(ui.ScrollAreaOptions{})
	visits := 0
	for _, access := range ContentQuery[scene.ClipExempt2D](a) {
		_ = access.Get()
		visits++
	}
	if visits != 4 {
		t.Fatalf("private query found %d procedural parts, want 4", visits)
	}
	for range ContentQuery[scene.ClipExempt2D](b) {
		t.Fatal("private query visited another module's descendants")
	}
	step := func() {
		for e := range ContentQuery[scene.Visibility](a) {
			if !a.owns(e) {
				panic(ErrOwnership)
			}
		}
	}
	step()
	if allocs := testing.AllocsPerRun(100, step); allocs != 0 {
		t.Fatalf("private queries allocated %g times", allocs)
	}
	if !a.Despawn(root) {
		t.Fatal("procedural root deletion rejected")
	}
}

func TestPrivateSubtreeCreationAndDeletionAllocateNothingWarm(t *testing.T) {
	_, a, _ := boundaryContent(t)
	step := func() {
		root := ContentSpawnNode(a, ui.NodeOptions{}, boundaryValue{})
		for range 8 {
			ContentSpawnNode(a, ui.NodeOptions{Parent: root}, boundaryValue{})
		}
		if !a.Despawn(root) {
			panic(ErrOwnership)
		}
	}
	for range 16 {
		step()
	}
	if allocs := testing.AllocsPerRun(100, step); allocs != 0 {
		t.Fatalf("private subtree creation/deletion allocated %g times", allocs)
	}
}

func TestQueuedNodesAllocateNothingWarmAndKeepPrivateSpawnDefaults(t *testing.T) {
	r, a, b := boundaryContent(t)
	trigger := ContentSpawnNode(a, ui.NodeOptions{}, boundaryValue{})
	step := func() {
		for range ContentQuery[boundaryValue](a) {
			parent := ContentSpawnNode(a, ui.NodeOptions{X: 7, Y: 9}, boundaryValue{1})
			child := ContentSpawnNode2(a, ui.NodeOptions{Parent: parent}, boundaryValue{2}, boundaryExtra{3})
			if r.world.IsAlive(parent) || b.owns(child) || !a.owns(child) || !a.Despawn(parent) {
				panic(ErrOwnership)
			}
		}
		if !r.world.IsAlive(trigger) || r.world.EntityCount() != 7 {
			panic("queued nodes leaked or removed the trigger")
		}
	}
	for range 16 {
		step()
	}
	if allocs := testing.AllocsPerRun(100, step); allocs != 0 {
		t.Fatalf("queued node creation/deletion allocated %g times", allocs)
	}
	bundle := ecs.NewBundle(2)
	ecs.Put(&bundle, scene.Parent{})
	ecs.Put(&bundle, scene.Activity{})
	var first, second ecs.Entity
	for range ContentQuery[boundaryValue](a) {
		first = a.Spawn(bundle)
		second = b.Spawn(bundle)
	}
	firstParent, _ := ContentGet[scene.Parent](a, first)
	secondParent, _ := ContentGet[scene.Parent](b, second)
	activity, _ := ContentGet[scene.Activity](a, first)
	if firstParent.Entity != a.Root() || secondParent.Entity != b.Root() || !activity.Active {
		t.Fatal("private spawn defaults changed or mutated a reused bundle")
	}
}

func BenchmarkContentQuery(b *testing.B) {
	for _, foreign := range []int{0, 4096} {
		b.Run("foreign="+strconv.Itoa(foreign), func(b *testing.B) {
			r, a, other := boundaryContent(b)
			for range 64 {
				ContentSpawnNode(a, ui.NodeOptions{}, boundaryValue{})
			}
			for range foreign {
				ContentSpawnNode(other, ui.NodeOptions{}, boundaryValue{})
			}
			r.SyncScopes()
			b.Run("private", func(b *testing.B) {
				b.ReportAllocs()
				visits := 0
				for b.Loop() {
					for range ContentQuery[boundaryValue](a) {
						visits++
					}
				}
				if visits != b.N*64 {
					b.Fatal("private query lost entities")
				}
			})
			b.Run("scene-filter", func(b *testing.B) {
				b.ReportAllocs()
				visits := 0
				for b.Loop() {
					for e := range ecs.Query[boundaryValue](r.world).All {
						if a.owns(e) {
							visits++
						}
					}
				}
				if visits != b.N*64 {
					b.Fatal("scene filtering lost entities")
				}
			})
		})
	}
}
