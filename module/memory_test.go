package module

import (
	"errors"
	"testing"
	"unsafe"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/project"
)

func TestScopeUpdatesReleaseRemovedContexts(t *testing.T) {
	plan, _ := compileFixture(t, map[string]string{
		"resources/scenes/main.json": `{"modules":[{"id":"a","mod":"box"},{"id":"b","mod":"box"}]}`,
		"box/mod.json":               `{}`,
	})
	registry := Registry{}
	must(t, RegisterMod(&registry, "box", func() Signal { return Signal{} }, Lifecycle[Signal]{
		Update:      func(*Context, *Signal, Tick) error { return nil },
		FrameUpdate: func(*Context, *Signal, Tick) error { return nil },
	}))
	r := newFixture(t, plan, registry, Options{})
	s, err := r.MountEntry()
	must(t, err)
	for _, c := range []*Context{&s.modules[1], &s.modules[0]} {
		must(t, c.Dispose())
		for _, list := range [][]*Context{r.runningUpdates, r.runningFrameUpdates} {
			for _, retained := range list[:cap(list)] {
				if retained == c {
					t.Fatal("execution list retains a disposed context")
				}
			}
		}
	}
}

func TestSlotBuffersRetainOnlyShape(t *testing.T) {
	r := &Runtime{options: Options{EventCapacity: 8, PayloadCapacity: 2}, slotBuffers: make(map[project.ValueType]*slotBuffer)}
	shape := project.SlotValue{Type: "Label", Fields: []project.SlotFieldValue{{Name: "text", Value: project.SlotValue{Type: project.StringValue, String: "instance data"}}}}
	buffer := r.slotBuffer(shape)
	if buffer.shape.Fields[0].Value.String != "" {
		t.Fatal("slot shape retains instance data")
	}
	for _, bank := range buffer.values {
		for _, value := range bank {
			if value.Fields[0].Value.String != "" {
				t.Fatal("unused slot entry retains instance data")
			}
		}
	}
	if shape.Fields[0].Value.String != "instance data" {
		t.Fatal("preparing the shape changed the source")
	}
}

func TestPayloadTrackingPreservesQueuedTypesDuringRegistration(t *testing.T) {
	plan, _ := compileFixture(t, map[string]string{"resources/scenes/main.json": `{"modules":[]}`})
	registry := Registry{}
	var numbers int64
	var labels string
	var flags int
	must(t, RegisterScene(&registry, "main.json", func() Signal { return Signal{} }, Lifecycle[Signal]{
		OnMessage: func(_ *Context, _ *Signal, message Message) error {
			if value, ok := Value[int64](message); ok {
				numbers += value
			}
			if value, ok := Value[string](message); ok {
				labels += value
			}
			if value, ok := Value[bool](message); ok && value {
				flags++
			}
			return nil
		},
	}))
	r := newFixture(t, plan, registry, Options{EventCapacity: 2})
	if r.prepareWorkspace.pending != nil || r.prepareWorkspace.queues[0] != nil {
		t.Fatal("runtime reserved unused preparation storage")
	}
	s, err := r.MountEntry()
	must(t, err)
	number, err := SceneEmitter[int64](s, "number")
	must(t, err)
	must(t, number.Emit(12))
	label, err := SceneEmitter[string](s, "label")
	must(t, err)
	must(t, label.Emit("hello"))
	// Register a third type while the queue and its tracking list are full.
	flag, err := SceneEmitter[bool](s, "flag")
	must(t, err)
	must(t, r.Dispatch())
	must(t, flag.Emit(true))
	must(t, r.Dispatch())
	if numbers != 12 || labels != "hello" || flags != 1 {
		t.Fatal("registration disturbed queued payloads")
	}
	for i := range 4 {
		if len(r.usedStores[i]) != 0 {
			t.Fatal("dispatch did not reset tracked payloads")
		}
	}
}

func TestPreparationRollbackReleasesPendingReferences(t *testing.T) {
	plan, _ := compileFixture(t, map[string]string{"resources/scenes/main.json": `{"modules":[]}`})
	registry := Registry{}
	must(t, RegisterScene(&registry, "main.json", func() Signal { return Signal{} }, Lifecycle[Signal]{
		Enter: func(c *Context, _ *Signal, _ SceneStartup) error {
			request, err := BindRequester[Signal, Signal](c, "pending")
			if err != nil {
				return err
			}
			if _, err = request.Send(Signal{}, RequestOptions{}); err != nil {
				return err
			}
			return ErrInvalid
		},
	}))
	r := newFixture(t, plan, registry, Options{})
	s, err := r.Manager().Preload(ScenePreloadRequest{Scene: "main.json"})
	must(t, err)
	if _, err = r.Manager().prepareScene(s, s.ID(), SceneStartup{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("preparation error = %v", err)
	}
	for _, pending := range r.prepareWorkspace.pending {
		if pending.source != nil || pending.target != nil || pending.response != nil {
			t.Fatal("preparation rollback retains pending request references")
		}
	}
}

func BenchmarkRuntimeMemory(b *testing.B) {
	plan, _ := compileFixture(b, map[string]string{"resources/scenes/main.json": `{"modules":[]}`})
	for _, mode := range []string{"new", "prepared"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			var reserved uintptr
			for b.Loop() {
				r, err := New(ecs.NewWorld(), plan, Registry{}, Options{})
				must(b, err)
				if mode == "prepared" {
					_, err = r.Manager().StartEntry()
					must(b, err)
				}
				reserved = 0
				for i := range 4 {
					reserved += uintptr(cap(r.usedStores[i]))*unsafe.Sizeof(valueStore(nil)) + uintptr(cap(r.usedSlots[i]))*unsafe.Sizeof((*slotBuffer)(nil))
				}
				p := &r.prepareWorkspace
				reserved += uintptr(cap(p.pending))*unsafe.Sizeof(pendingRequest{}) + uintptr(cap(p.free))*unsafe.Sizeof(int(0))
				for _, queue := range p.queues {
					reserved += uintptr(cap(queue)) * unsafe.Sizeof(envelope{})
				}
				must(b, r.Close())
			}
			b.ReportMetric(float64(reserved), "aux-reserved-B")
		})
	}
}
