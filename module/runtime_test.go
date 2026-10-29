package module

import (
	"errors"
	"io"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func compileFixture(t testing.TB, files map[string]string) (*project.ProjectPlan, fstest.MapFS) {
	t.Helper()
	if _, ok := files["resources/project.json"]; !ok {
		files["resources/project.json"] = `{"entry":"main.json","scenes":["main.json"]}`
	}
	source := make(fstest.MapFS, len(files))
	for name, data := range files {
		source[name] = &fstest.MapFile{Data: []byte(data)}
	}
	plan, err := project.CompileProject(source, ".")
	if err != nil {
		t.Fatal(err)
	}
	return plan, source
}

func newFixture(t testing.TB, plan *project.ProjectPlan, registry Registry, options Options) *Runtime {
	t.Helper()
	return newFixtureWith[FullContent](t, plan, registry, options)
}

func newFixtureWith[P ContentProfile](t testing.TB, plan *project.ProjectPlan, registry Registry, options Options) *Runtime {
	t.Helper()
	r, err := New[P](ecs.NewWorld(), plan, registry, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := r.Close(); err != nil {
			t.Error(err)
		}
	})
	return r
}

func must(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestIndependentStatePlacementAndOwnedEntities(t *testing.T) {
	for _, space := range []string{"ui", "world"} {
		t.Run(space, func(t *testing.T) {
			plan, source := compileFixture(t, map[string]string{
				"resources/scenes/main.json": `{"transform":{"position":[100,0]},"modules":[{"id":"a","mod":"box","placement":{"position":[10,0]},"children":[{"id":"child","mod":"box","placement":{"position":[20,0]}}]},{"id":"b","mod":"box","placement":{"position":[30,0]}}]}`,
				"box/mod.json":               `{"space":"` + space + `","transform":{"position":[2,0]},"slots":{"text":{"type":"string","default":"original"},"style":{"type":"Style","default":{"size":12}}},"resources":{"data":{"kind":"data","path":"private.json"}}}`,
				"box/private.json":           `{"value":1}`,
				"resources/project.json":     `{"entry":"main.json","scenes":["main.json"],"types":{"Style":{"fields":{"size":"int"}}}}`,
			})
			type state struct{ count int }
			var contexts []*Context
			var states []*state
			registry := Registry{}
			must(t, registry.RegisterMod("box", func() state { return state{} }, Lifecycle[state]{
				Install: func(c *Context, s *state) error {
					contexts = append(contexts, c)
					states = append(states, s)
					return nil
				},
				Update: func(_ *Context, s *state, _ Tick) error { s.count++; return nil },
			}))
			r := newFixture(t, plan, registry, Options{Source: source})
			s, err := r.MountEntry()
			must(t, err)
			must(t, r.Update(1.0/60))
			if states[0] == states[1] || states[0].count != 1 || states[2].count != 1 {
				t.Fatal("behavior state was shared or skipped")
			}
			a, child, b := contexts[0], contexts[1], contexts[2]
			if a.definition != b.definition {
				t.Fatal("definition is not shared")
			}
			text, _ := a.Slot("text")
			must(t, text.Set(project.SlotValue{Type: project.StringValue, String: "changed"}))
			other, _ := b.Slot("text")
			got, _ := other.Read[string]()
			if got != "original" {
				t.Fatal("slot state crossed instances")
			}
			style, _ := a.Slot("style")
			v, _ := style.Value()
			v.Fields[0].Value.Integer = 42
			otherStyle, _ := b.Slot("style")
			otherValue, _ := otherStyle.Value()
			if otherValue.Fields[0].Value.Integer != 12 || a.definition.Slots[0].Default.Fields[0].Value.Integer != 12 {
				t.Fatal("structured state changed another instance/definition")
			}
			if space == "ui" {
				local, _ := a.Get[scene.UITransform](a.Root())
				local.Translation = scene.Px2(50, 0)
				must(t, a.Set(a.Root(), local))
				must(t, scene.SyncUIGlobalTransformSystem("").System(&ecs.SystemContext{World: r.world}))
				for c, want := range map[*Context]float32{a: 160, child: 132, b: 132} {
					g, _ := c.Get[scene.UIGlobalTransform](c.Root())
					if g.Matrix[12] != want {
						t.Fatalf("position %s: got %v want %v", c.name, g.Matrix[12], want)
					}
				}
			} else {
				local, _ := a.Get[scene.Transform](a.Root())
				local.Translation = mgl32.Vec3{50, 0, 0}
				must(t, a.Set(a.Root(), local))
				must(t, scene.SyncGlobalTransformSystem("").System(&ecs.SystemContext{World: r.world}))
				for c, want := range map[*Context]float32{a: 160, child: 132, b: 132} {
					g, _ := c.Get[scene.GlobalTransform](c.Root())
					if g.Matrix[12] != want {
						t.Fatalf("position %s: got %v want %v", c.name, g.Matrix[12], want)
					}
				}
			}
			if a.definition.Transform.Position[0] != 2 || s.plan.Instances[0].Placement.Position[0] != 10 {
				t.Fatal("runtime rewrote authored coordinates")
			}
			if _, ok := a.Get[scene.Visibility](b.Root()); ok {
				t.Fatal("foreign entity readable")
			}
			if err := a.Set(b.Root(), scene.Visibility{Visible: false}); !errors.Is(err, ErrOwnership) {
				t.Fatal("foreign entity writable")
			}
			if _, ok := a.Get[scene.Parent](a.Root()); ok {
				t.Fatal("placement proxy was exposed")
			}
			if err := a.Set(a.Root(), scene.Parent{}); !errors.Is(err, ErrOwnership) {
				t.Fatal("raw cross-module parenting accepted")
			}
			private, err := a.Spawn(Entity{})
			must(t, err)
			must(t, a.Set(private, int64(7)))
			count := 0
			b.Each[int64](func(Entity, int64) bool { count++; return true })
			if count != 0 {
				t.Fatal("query crossed ownership")
			}
			file, err := a.OpenResource("data")
			must(t, err)
			data, err := io.ReadAll(file)
			must(t, err)
			must(t, file.Close())
			if string(data) != `{"value":1}` {
				t.Fatal("private resource not resolved")
			}
			if _, err := a.OpenResource("../box/private.json"); err == nil {
				t.Fatal("resource escape accepted")
			}
			if _, err := a.OpenFile("../box/private.json"); err == nil {
				t.Fatal("private file escape accepted")
			}
			file, err = a.OpenFile("private.json")
			must(t, err)
			must(t, file.Close())
			must(t, a.Dispose())
			if a.live || child.live || !b.live {
				t.Fatal("subtree disposal did not match placement hierarchy")
			}
			must(t, s.Dispose())
			if r.world.EntityCount() != 0 || len(r.runningUpdates) != 0 {
				t.Fatal("scene ownership leaked")
			}
		})
	}
}

type requestFixtureState struct {
	runtime  *Runtime
	scene    *Scene
	clients  []Requester[string, int64]
	contexts []*Context
	server   *Context
	requests []Request[string, int64]
	results  []Result[int64]
}

func setupRequests(t testing.TB, options Options, echo bool) *requestFixtureState {
	t.Helper()
	plan, _ := compileFixture(t, map[string]string{
		"resources/scenes/main.json": `{"modules":[{"id":"server","mod":"service"},{"id":"a","mod":"client"},{"id":"b","mod":"client"}]}`,
		"service/mod.json":           `{}`,
		"client/mod.json":            `{"slots":{"text":{"type":"string","default":"text"}}}`,
	})
	f := &requestFixtureState{requests: make([]Request[string, int64], 0, 64), results: make([]Result[int64], 0, 64)}
	registry := Registry{}
	must(t, registry.RegisterMod("service", func() Signal { return Signal{} }, Lifecycle[Signal]{Install: func(c *Context, _ *Signal) error {
		f.server = c
		return handleRequest(c, "ask", func(c *Context, q Request[string, int64]) error {
			f.requests = append(f.requests, q)
			if echo {
				return q.Reply(c, int64(len(q.Value)))
			}
			return nil
		})
	}}))
	must(t, registry.RegisterMod("client", func() Signal { return Signal{} }, Lifecycle[Signal]{Install: func(c *Context, _ *Signal) error {
		p, err := testRequester[string, int64](c, "ask", func(_ *Context, result Result[int64]) error { f.results = append(f.results, result); return nil })
		if err != nil {
			return err
		}
		f.clients = append(f.clients, p)
		f.contexts = append(f.contexts, c)
		return nil
	}}))
	f.runtime = newFixture(t, plan, registry, options)
	var err error
	f.scene, err = f.runtime.MountEntry()
	must(t, err)
	return f
}

func TestRequestsDeferredReplyCancellationAndTimeout(t *testing.T) {
	t.Run("reply", func(t *testing.T) {
		f := setupRequests(t, Options{RequestCapacity: 1}, true)
		id, err := f.clients[0].SendTo("server", "hello", RequestOptions{})
		must(t, err)
		must(t, f.runtime.Dispatch())
		if len(f.requests) != 1 || len(f.results) != 0 {
			t.Fatal("request/reply batch was not frozen")
		}
		if err := f.requests[0].Reply(f.server, 5); !errors.Is(err, ErrAlreadyReplied) {
			t.Fatal("duplicate reply accepted")
		}
		must(t, f.runtime.Dispatch())
		if !reflect.DeepEqual(f.results, []Result[int64]{{ID: id, Value: 5}}) || f.runtime.Pending() != 0 {
			t.Fatal("reply address/result incorrect")
		}
		if err := f.requests[0].Reply(f.server, 99); !errors.Is(err, ErrCanceled) {
			t.Fatal("completed reply stayed valid")
		}
	})
	t.Run("cancel", func(t *testing.T) {
		f := setupRequests(t, Options{}, false)
		id, err := f.clients[0].SendTo("server", "cancel", RequestOptions{})
		must(t, err)
		if err := f.clients[1].Cancel(id); !errors.Is(err, ErrOwnership) {
			t.Fatal("foreign requester could cancel")
		}
		must(t, f.clients[0].Cancel(id))
		must(t, f.runtime.Dispatch())
		if len(f.requests) != 0 || len(f.results) != 1 || f.results[0].Err != ErrCanceled || f.runtime.Pending() != 0 {
			t.Fatal("canceled request delivered or failure missing")
		}
	})
	t.Run("timeout", func(t *testing.T) {
		f := setupRequests(t, Options{TimeoutTicks: 3}, false)
		_, err := f.clients[0].SendTo("server", "timeout", RequestOptions{})
		must(t, err)
		must(t, f.runtime.Dispatch())
		must(t, f.runtime.Dispatch())
		if len(f.results) != 0 {
			t.Fatal("timeout fired early")
		}
		must(t, f.runtime.Dispatch())
		if len(f.results) != 1 || f.results[0].Err != ErrTimeout || f.runtime.Pending() != 0 {
			t.Fatal("timeout did not complete request")
		}
		if err := f.requests[0].Reply(f.server, 1); !errors.Is(err, ErrCanceled) {
			t.Fatal("late timeout reply accepted")
		}
	})
	t.Run("revision", func(t *testing.T) {
		f := setupRequests(t, Options{}, false)
		slot, _ := f.contexts[0].Slot("text")
		_, err := f.clients[0].SendTo("server", "old", RequestOptions{Revision: slot.Revision()})
		must(t, err)
		must(t, f.runtime.Dispatch())
		must(t, slot.Set(project.SlotValue{Type: project.StringValue, String: "new"}))
		must(t, f.requests[0].Reply(f.server, 3))
		must(t, f.runtime.Dispatch())
		if len(f.results) != 1 || f.results[0].Err != ErrStaleRevision {
			t.Fatal("stale reply was accepted")
		}
	})
}

func TestRequestCapacityBackpressureAndIdentityReuse(t *testing.T) {
	f := setupRequests(t, Options{EventCapacity: 2, RequestCapacity: 1}, false)
	oldID, err := f.clients[0].SendTo("server", "old", RequestOptions{})
	must(t, err)
	if _, err := f.clients[1].SendTo("server", "second", RequestOptions{}); !errors.Is(err, ErrPendingFull) {
		t.Fatal("pending overflow was not reported")
	}
	must(t, f.runtime.Dispatch())
	old := f.requests[0]
	emit, err := f.contexts[0].BindEmitter[int64]("tick")
	must(t, err)
	must(t, emit.Emit(1))
	must(t, emit.Emit(2))
	if err := emit.Emit(3); !errors.Is(err, ErrQueueFull) {
		t.Fatal("event overflow was not reported")
	}
	if err := old.Reply(f.server, 3); !errors.Is(err, ErrQueueFull) {
		t.Fatal("reply overflow was not reported")
	}
	must(t, f.runtime.Dispatch())
	must(t, f.clients[0].Cancel(oldID))
	must(t, f.runtime.Dispatch())
	newID, err := f.clients[1].SendTo("server", "new", RequestOptions{})
	must(t, err)
	if newID <= oldID {
		t.Fatal("request identities are not monotonic")
	}
	if err := old.Reply(f.server, 9); !errors.Is(err, ErrCanceled) {
		t.Fatal("old token replied into reused table slot")
	}
	must(t, f.runtime.Dispatch())
	must(t, f.requests[1].Reply(f.server, 3))
	must(t, f.runtime.Dispatch())
	if len(f.results) != 2 || f.results[1].ID != newID || f.results[1].Value != 3 {
		t.Fatal("reused pending slot received an old result")
	}
}

func TestDestroyCancelsRequestsAndNewSceneRejectsOldAddresses(t *testing.T) {
	f := setupRequests(t, Options{RequestCapacity: 1}, false)
	_, err := f.clients[0].SendTo("server", "old", RequestOptions{})
	must(t, err)
	must(t, f.runtime.Dispatch())
	old := f.requests[0]
	generation := f.scene.Generation()
	must(t, f.server.Dispose())
	must(t, f.runtime.Dispatch())
	if len(f.results) != 1 || f.results[0].Err != ErrCanceled {
		t.Fatal("destroyed receiver did not cancel its request")
	}
	if err := old.Reply(f.server, 1); !errors.Is(err, ErrCanceled) {
		t.Fatal("disposed receiver replied")
	}
	must(t, f.scene.Dispose())
	fresh, err := f.runtime.MountEntry()
	must(t, err)
	if fresh.Generation() == generation {
		t.Fatal("scene identity reused")
	}
	_, err = f.clients[2].SendTo("server", "new", RequestOptions{})
	must(t, err)
	must(t, f.runtime.Dispatch())
	if err := old.Reply(f.server, 100); !errors.Is(err, ErrCanceled) {
		t.Fatal("old request targeted remounted scene")
	}
	must(t, f.requests[1].Reply(f.server, 3))
	must(t, f.runtime.Dispatch())
	if len(f.results) != 2 || f.results[1].Value != 3 {
		t.Fatal("new generation received old response")
	}
	_, err = f.clients[2].SendTo("server", "dispose source", RequestOptions{})
	must(t, err)
	must(t, f.contexts[2].Dispose())
	if f.runtime.Pending() != 0 {
		t.Fatal("disposed requester leaked pending state")
	}
	must(t, f.runtime.Dispatch())
}

func TestDisposalAndPrivateStructuralChangesWaitForBoundary(t *testing.T) {
	plan, _ := compileFixture(t, map[string]string{
		"resources/scenes/main.json": `{"modules":[{"id":"a","mod":"test"},{"id":"b","mod":"test"}]}`,
		"test/mod.json":              `{}`,
	})
	var r *Runtime
	var b *Context
	var log []string
	registry := Registry{}
	must(t, registry.RegisterMod("test", func() Signal { return Signal{} }, Lifecycle[Signal]{
		Install: func(c *Context, _ *Signal) error {
			if c.name == "b" {
				b = c
			}
			return handleValue(c, "go", func(c *Context, _ Signal) error {
				log = append(log, c.name)
				if c.name == "a" {
					must(t, b.Dispose())
					if !r.world.IsAlive(b.root) {
						t.Fatal("destruction happened inside frozen batch")
					}
					private, err := c.Spawn(Entity{})
					if err != nil {
						return err
					}
					must(t, c.Set(private, int64(8)))
					if _, ok := c.Get[int64](private); ok {
						t.Fatal("new entity became visible before boundary")
					}
				}
				return nil
			})
		},
		Destroy: func(c *Context, _ *Signal) error { log = append(log, c.name+".destroy"); return nil },
	}))
	r = newFixture(t, plan, registry, Options{})
	s, err := r.MountEntry()
	must(t, err)
	emit, err := s.modules[0].BindEmitter[Signal]("go")
	must(t, err)
	must(t, emit.EmitTo("a", Signal{}))
	must(t, emit.EmitTo("b", Signal{}))
	must(t, r.Dispatch())
	if !reflect.DeepEqual(log, []string{"a", "b", "b.destroy"}) {
		t.Fatalf("unexpected structural boundary: %v", log)
	}
	count := 0
	s.modules[0].Each[int64](func(_ Entity, v int64) bool {
		count++
		if v != 8 {
			t.Fatal("private command value lost")
		}
		return true
	})
	if count != 1 || r.world.IsAlive(b.root) {
		t.Fatal("structural commands were not applied")
	}
}

func TestSlotsCopyQueuedStructuresAndRejectInvalidWrites(t *testing.T) {
	plan, _ := compileFixture(t, map[string]string{
		"resources/project.json":     `{"entry":"main.json","scenes":["main.json"],"types":{"Style":{"fields":{"size":"int"}}}}`,
		"resources/scenes/main.json": `{"modules":[{"id":"a","mod":"box"},{"id":"b","mod":"box"}]}`,
		"box/mod.json":               `{"slots":{"style":{"type":"Style","default":{"size":12}},"locked":{"type":"int","writable":false}}}`,
	})
	r := newFixture(t, plan, Registry{}, Options{})
	s, err := r.MountEntry()
	must(t, err)
	w, err := s.SlotWriter("a", "style")
	must(t, err)
	value := project.SlotValue{Type: "Style", Fields: []project.SlotFieldValue{{Name: "size", Value: project.SlotValue{Type: project.IntValue, Integer: 24}}}}
	must(t, w.Write(value))
	value.Fields[0].Value.Integer = 99
	slot, _ := s.modules[0].Slot("style")
	v, _ := slot.Value()
	if v.Fields[0].Value.Integer != 12 {
		t.Fatal("slot event was immediate")
	}
	must(t, r.Dispatch())
	v, _ = slot.Value()
	if v.Fields[0].Value.Integer != 24 {
		t.Fatal("queued structure borrowed mutable caller storage")
	}
	other, _ := s.modules[1].Slot("style")
	v, _ = other.Value()
	if v.Fields[0].Value.Integer != 12 {
		t.Fatal("external write crossed instance scope")
	}
	if _, err := s.SlotWriter("a", "locked"); !errors.Is(err, ErrReadOnly) {
		t.Fatal("read-only slot was writable")
	}
	if err := w.Write(project.SlotValue{Type: project.IntValue}); !errors.Is(err, ErrType) {
		t.Fatal("wrong slot type accepted")
	}
	must(t, s.Dispose())
	if err := w.Write(value); !errors.Is(err, ErrDisposed) {
		t.Fatal("stale slot writer accepted")
	}
}

func TestMountFailureRollsBackOwnershipAndQueuedStartup(t *testing.T) {
	plan, _ := compileFixture(t, map[string]string{
		"resources/scenes/main.json": `{"modules":[{"id":"a","mod":"box"}]}`,
		"box/mod.json":               `{}`,
	})
	fail := true
	destroyed := 0
	registry := Registry{}
	must(t, registry.RegisterMod("box", func() Signal { return Signal{} }, Lifecycle[Signal]{
		Install: func(c *Context, _ *Signal) error {
			e, err := c.BindEmitter[Signal]("go")
			if err != nil {
				return err
			}
			if err := e.Emit(Signal{}); err != nil {
				return err
			}
			if fail {
				return ErrInvalid
			}
			return nil
		}, Destroy: func(*Context, *Signal) error { destroyed++; return nil },
	}))
	r := newFixture(t, plan, registry, Options{EventCapacity: 1})
	if _, err := r.MountEntry(); !errors.Is(err, ErrInvalid) {
		t.Fatal("installation failure was ignored")
	}
	if r.world.EntityCount() != 0 || r.Queued() != 0 || r.Pending() != 0 || destroyed != 1 {
		t.Fatal("failed mount leaked state")
	}
	fail = false
	_, err := r.MountEntry()
	must(t, err)
	must(t, r.Dispatch())
	if r.Queued() != 0 {
		t.Fatal("startup rollback blocked reuse")
	}
}

func TestOwnedQueryDefersPrivateSubtreeDeletionAndDestroyIsIdempotent(t *testing.T) {
	plan, _ := compileFixture(t, map[string]string{"resources/scenes/main.json": `{"modules":[{"id":"a","mod":"box"}]}`, "box/mod.json": `{}`})
	registry := Registry{}
	var c *Context
	destroyed := 0
	must(t, registry.RegisterMod("box", func() Signal { return Signal{} }, Lifecycle[Signal]{
		Install: func(context *Context, _ *Signal) error { c = context; return nil },
		Destroy: func(c *Context, _ *Signal) error { destroyed++; return c.Dispose() },
	}))
	r := newFixture(t, plan, registry, Options{})
	_, err := r.MountEntry()
	must(t, err)
	parent, err := c.Spawn(Entity{})
	must(t, err)
	child, err := c.Spawn(parent)
	must(t, err)
	must(t, c.Set(parent, int64(1)))
	must(t, c.Set(child, int64(2)))
	seen := 0
	must(t, c.Each[int64](func(entity Entity, _ int64) bool {
		seen++
		if entity == parent {
			must(t, c.Despawn(parent))
			if !r.world.IsAlive(child.entity) {
				t.Fatal("owned traversal was mutated before boundary")
			}
		}
		return true
	}))
	if seen != 2 || r.world.IsAlive(parent.entity) || r.world.IsAlive(child.entity) {
		t.Fatal("private subtree cleanup did not preserve traversal")
	}
	if err := c.Set(parent, int64(3)); !errors.Is(err, ErrOwnership) {
		t.Fatal("despawned owned handle stayed valid")
	}
	must(t, r.Close())
	if destroyed != 1 {
		t.Fatal("Destroy recursively disposed itself")
	}
}

func TestPainterOrderSurvivesEntityReuseAndPrivateContent(t *testing.T) {
	for _, space := range []string{"ui", "world"} {
		t.Run(space, func(t *testing.T) {
			plan, _ := compileFixture(t, map[string]string{
				"resources/scenes/main.json": `{"modules":[{"id":"a","mod":"box"},{"id":"b","mod":"box"}]}`,
				"box/mod.json":               `{"space":"` + space + `","components":{"sprite":{"color":"#ff0000","size":[10,10]}}}`,
			})
			registry := Registry{}
			var private Entity
			must(t, registry.RegisterMod("box", func() Signal { return Signal{} }, Lifecycle[Signal]{Install: func(c *Context, _ *Signal) error {
				if c.name == "a" {
					var err error
					private, err = c.Spawn(Entity{})
					return err
				}
				return nil
			}}))
			r := newFixture(t, plan, registry, Options{})
			dummies := make([]ecs.Entity, 32)
			for i := range dummies {
				dummies[i] = r.world.Spawn(ecs.NewBundle(0))
			}
			old, err := r.MountEntry()
			must(t, err)
			must(t, old.Dispose())
			for _, entity := range dummies {
				r.world.Despawn(entity)
			}
			s, err := r.MountEntry()
			must(t, err)
			if s.modules[0].root.PickingID() < s.modules[1].root.PickingID() {
				t.Fatal("fixture did not exercise reversed ECS slot reuse")
			}
			order := func(entity ecs.Entity) int {
				if space == "ui" {
					v, _ := r.world.Get[scene.UIStackIndex](entity)
					return int(v)
				}
				v, _ := r.world.Get[scene.RenderOrder2D](entity)
				return int(v)
			}
			firstGroup, firstOK := r.world.Get[scene.PainterGroup2D](s.modules[0].root)
			secondGroup, secondOK := r.world.Get[scene.PainterGroup2D](s.modules[1].root)
			if !firstOK || !secondOK || firstGroup >= secondGroup || order(s.modules[0].root) >= order(private.entity) {
				t.Fatal("private content escaped module painter order")
			}
		})
	}
}

func TestStructuralCommandCapacityReportsOverflow(t *testing.T) {
	plan, _ := compileFixture(t, map[string]string{
		"resources/scenes/main.json": `{"modules":[{"id":"a","mod":"box"}]}`,
		"box/mod.json":               `{}`,
	})
	registry := Registry{}
	var emit Emitter[Signal]
	var overflow error
	must(t, registry.RegisterMod("box", func() Signal { return Signal{} }, Lifecycle[Signal]{Install: func(c *Context, _ *Signal) error {
		var err error
		emit, err = c.BindEmitter[Signal]("go")
		if err != nil {
			return err
		}
		return handleValue(c, "go", func(c *Context, _ Signal) error {
			e, err := c.Spawn(Entity{})
			if err != nil {
				return err
			}
			overflow = c.Set(e, int64(1))
			return nil
		})
	}}))
	r := newFixture(t, plan, registry, Options{CommandCapacity: 1})
	_, err := r.MountEntry()
	must(t, err)
	must(t, emit.EmitTo("a", Signal{}))
	must(t, r.Dispatch())
	if !errors.Is(overflow, ErrCommandFull) {
		t.Fatal("private ECS command overflow was not reported")
	}
}
