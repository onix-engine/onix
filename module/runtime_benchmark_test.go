package module

import (
	"testing"

	"github.com/diiyw/onix/project"
)

type hotState struct{ received, results, updates int64 }
type hotFixture struct {
	runtime    *Runtime
	emitter    Emitter[int64]
	requester  Requester[string, int64]
	slot       Slot
	writer     SlotWriter
	structured project.SlotValue
	state      *hotState
}

func setupHot(t testing.TB) hotFixture {
	t.Helper()
	plan, _ := compileFixture(t, map[string]string{
		"resources/project.json":     `{"entry":"main.json","scenes":["main.json"],"types":{"Style":{"fields":{"size":"int"}}}}`,
		"resources/scenes/main.json": `{"modules":[{"id":"a","mod":"hot"},{"id":"service","mod":"service"}]}`,
		"hot/mod.json":               `{"slots":{"value":{"type":"int"},"style":{"type":"Style","default":{"size":12}}}}`,
		"service/mod.json":           `{}`,
	})
	f := hotFixture{structured: project.SlotValue{Type: "Style", Fields: []project.SlotFieldValue{{Name: "size", Value: project.SlotValue{Type: project.IntValue, Integer: 18}}}}}
	registry := Registry{}
	must(t, registry.RegisterMod("hot", func() hotState { return hotState{} }, Lifecycle[hotState]{
		Install: func(c *Context, state *hotState) error {
			f.state = state
			var err error
			f.slot, err = c.Slot("value")
			if err != nil {
				return err
			}
			f.emitter, err = c.BindEmitter[int64]("ping")
			if err != nil {
				return err
			}
			f.requester, err = testRequester[string, int64](c, "ask", func(_ *Context, result Result[int64]) error {
				if result.Err != nil {
					return result.Err
				}
				state.results += result.Value
				return nil
			})
			if err != nil {
				return err
			}
			return handleValue(c, "ping", func(_ *Context, value int64) error { state.received += value; return nil })
		},
		Update: func(_ *Context, state *hotState, _ Tick) error { state.updates++; return nil },
	}))
	must(t, registry.RegisterMod("service", func() Signal { return Signal{} }, Lifecycle[Signal]{Install: func(c *Context, _ *Signal) error {
		return handleRequest(c, "ask", func(c *Context, q Request[string, int64]) error { return q.Reply(c, int64(len(q.Value))) })
	}}))
	f.runtime = newFixture(t, plan, registry, Options{EventCapacity: 8, RequestCapacity: 4})
	s, err := f.runtime.MountEntry()
	must(t, err)
	f.writer, err = s.SlotWriter("a", "style")
	must(t, err)
	return f
}

func hotDispatch(f *hotFixture) {
	if err := f.emitter.EmitTo("a", 1); err != nil {
		panic(err)
	}
	if err := f.runtime.Dispatch(); err != nil {
		panic(err)
	}
}
func hotSlotUpdate(f *hotFixture) {
	if err := f.slot.Set(project.SlotValue{Type: project.IntValue, Integer: 1}); err != nil {
		panic(err)
	}
	if err := f.runtime.Update(1.0 / 60); err != nil {
		panic(err)
	}
}
func hotSlotEvent(f *hotFixture) {
	if err := f.writer.Write(f.structured); err != nil {
		panic(err)
	}
	if err := f.runtime.Dispatch(); err != nil {
		panic(err)
	}
}
func hotRequest(f *hotFixture) {
	if _, err := f.requester.SendTo("service", "hello", RequestOptions{}); err != nil {
		panic(err)
	}
	if err := f.runtime.Dispatch(); err != nil {
		panic(err)
	}
	if err := f.runtime.Dispatch(); err != nil {
		panic(err)
	}
}

func TestHotPathsDoNotAllocateAfterWarmup(t *testing.T) {
	for _, test := range []struct {
		name string
		run  func(*hotFixture)
	}{
		{"dispatch", hotDispatch}, {"slot-update", hotSlotUpdate}, {"structured-slot-event", hotSlotEvent}, {"request-reply", hotRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := setupHot(t)
			for range 32 {
				test.run(&f)
			}
			if allocations := testing.AllocsPerRun(1000, func() { test.run(&f) }); allocations != 0 {
				t.Fatalf("hot path allocated %.2f times", allocations)
			}
		})
	}
}

func TestAlternatingStructuredSlotTypesDoNotAllocate(t *testing.T) {
	plan, _ := compileFixture(t, map[string]string{
		"resources/project.json":     `{"entry":"main.json","scenes":["main.json"],"types":{"First":{"fields":{"size":"int"}},"Second":{"fields":{"enabled":"bool","label":"string"}}}}`,
		"resources/scenes/main.json": `{"modules":[{"id":"a","mod":"box"}]}`,
		"box/mod.json":               `{"slots":{"first":{"type":"First"},"second":{"type":"Second"}}}`,
	})
	r := newFixture(t, plan, Registry{}, Options{EventCapacity: 4})
	s, err := r.MountEntry()
	must(t, err)
	first, err := s.SlotWriter("a", "first")
	must(t, err)
	second, err := s.SlotWriter("a", "second")
	must(t, err)
	writers := [2]SlotWriter{first, second}
	values := [2]project.SlotValue{
		{Type: "First", Fields: []project.SlotFieldValue{{Name: "size", Value: project.SlotValue{Type: project.IntValue, Integer: 18}}}},
		{Type: "Second", Fields: []project.SlotFieldValue{{Name: "enabled", Value: project.SlotValue{Type: project.BoolValue, Bool: true}}, {Name: "label", Value: project.SlotValue{Type: project.StringValue, String: "label"}}}},
	}
	index := 0
	run := func() {
		index ^= 1
		if err := writers[index].Write(values[index]); err != nil {
			panic(err)
		}
		if err := r.Dispatch(); err != nil {
			panic(err)
		}
	}
	for range 8 {
		run()
	}
	if allocations := testing.AllocsPerRun(1000, run); allocations != 0 {
		t.Fatalf("alternating slot shapes allocated %.2f times", allocations)
	}
}

func BenchmarkDispatch(b *testing.B) {
	f := setupHot(b)
	b.ReportAllocs()
	for b.Loop() {
		hotDispatch(&f)
	}
}
func BenchmarkSlotUpdate(b *testing.B) {
	f := setupHot(b)
	b.ReportAllocs()
	for b.Loop() {
		hotSlotUpdate(&f)
	}
}
func BenchmarkStructuredSlotEvent(b *testing.B) {
	f := setupHot(b)
	hotSlotEvent(&f)
	hotSlotEvent(&f)
	b.ReportAllocs()
	for b.Loop() {
		hotSlotEvent(&f)
	}
}
func BenchmarkRequestReply(b *testing.B) {
	f := setupHot(b)
	b.ReportAllocs()
	for b.Loop() {
		hotRequest(&f)
	}
}
