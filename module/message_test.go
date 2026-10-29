package module

import (
	"errors"
	"reflect"
	"testing"

	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
)

func TestMessagesAddressAnySceneInstanceWithoutDeclarations(t *testing.T) {
	plan, _ := compileFixture(t, map[string]string{
		"resources/scenes/main.json": `{"modules":[{"id":"sender","mod":"box"},{"id":"receiver","mod":"box"},{"id":"passive","mod":"empty"}]}`,
		"box/mod.json":               `{}`, "empty/mod.json": `{}`,
	})
	type packet struct {
		Count int
		Label string
	}
	registry := Registry{}
	var got []string
	must(t, registry.RegisterMod("box", func() Signal { return Signal{} }, Lifecycle[Signal]{OnMessage: func(c *Context, _ *Signal, m Message) error {
		if m.Topic != "packet" {
			return nil
		}
		value, ok := m.Value[packet]()
		if !ok {
			return ErrType
		}
		if _, ok := m.Value[int64](); ok {
			t.Fatal("wrong payload type matched")
		}
		got = append(got, c.Name()+":"+m.From+":"+value.Label)
		return nil
	}}))
	r := newFixture(t, plan, registry, Options{})
	first, err := r.MountEntry()
	must(t, err)
	second, err := r.MountEntry()
	must(t, err)
	must(t, first.modules[0].Send("receiver", "packet", packet{Count: 7, Label: "one"}))
	must(t, first.modules[0].Send("passive", "unknown", packet{}))
	must(t, second.modules[0].Send("receiver", "packet", packet{Label: "two"}))
	if err := first.modules[0].Send("missing", "packet", packet{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatal("send recursively delivered")
	}
	must(t, r.Dispatch())
	if !reflect.DeepEqual(got, []string{"receiver:sender:one", "receiver:sender:two"}) {
		t.Fatal(got)
	}
}

func TestSceneForwardsMessagesInOrderAndBorrowExpires(t *testing.T) {
	plan, _ := compileFixture(t, map[string]string{
		"resources/scenes/main.json": `{"modules":[{"id":"source","mod":"empty"},{"id":"left","mod":"box"},{"id":"right","mod":"box"}]}`,
		"empty/mod.json":             `{}`, "box/mod.json": `{}`,
	})
	registry := Registry{}
	var borrowed Message
	var sceneContext *Context
	var got []string
	must(t, registry.RegisterScene("main.json", func() Signal { return Signal{} }, Lifecycle[Signal]{OnMessage: func(c *Context, _ *Signal, m Message) error {
		if m.Topic != "ping" {
			return nil
		}
		borrowed = m
		sceneContext = c
		if err := c.Forward("right", m); err != nil {
			return err
		}
		return c.ForwardAs("left", "renamed", m)
	}}))
	must(t, registry.RegisterMod("box", func() Signal { return Signal{} }, Lifecycle[Signal]{OnMessage: func(c *Context, _ *Signal, m Message) error {
		v, ok := m.Value[int64]()
		if !ok {
			return ErrType
		}
		if v != 42 || m.From != "source" {
			t.Fatal(m, v)
		}
		got = append(got, c.Name()+":"+m.Topic)
		return nil
	}}))
	r := newFixture(t, plan, registry, Options{})
	s, err := r.MountEntry()
	must(t, err)
	must(t, s.modules[0].Send(SceneTarget, "ping", int64(42)))
	must(t, r.Dispatch())
	if len(got) != 2 {
		t.Fatal("scene did not relay during delivery", got)
	}
	if _, ok := borrowed.Value[int64](); ok {
		t.Fatal("expired payload readable")
	}
	if err := sceneContext.Forward("left", borrowed); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	must(t, r.Dispatch())
	if !reflect.DeepEqual(got, []string{"right:ping", "left:renamed"}) {
		t.Fatal(got)
	}
}

func TestSceneChoosesInputReceivers(t *testing.T) {
	plan, _ := compileFixture(t, map[string]string{
		"resources/scenes/main.json": `{"modules":[{"id":"button","mod":"button"},{"id":"logic","mod":"empty"}]}`,
		"button/mod.json":            `{"elements":[{"id":"hit","components":{"sprite":{"color":"#ffffff","size":[20,20]},"input":{"pointer":true}}}]}`,
		"empty/mod.json":             `{}`,
	})
	registry := Registry{}
	sceneCalls, buttonCalls, logicCalls := 0, 0, 0
	must(t, registry.RegisterScene("main.json", func() Signal { return Signal{} }, Lifecycle[Signal]{OnMessage: func(c *Context, _ *Signal, m Message) error {
		if m.Topic != InputTopic {
			return nil
		}
		sceneCalls++
		event, ok := m.Value[Input]()
		if !ok || event.Element != "hit" || m.From != "button" || event.Event.Entity != 0 || event.Event.Path != nil {
			t.Fatal(m, event)
		}
		if event.Event.Kind == scene.InputEventTap {
			return c.ForwardAs("logic", "activate", m)
		}
		return nil
	}}))
	must(t, registry.RegisterMod("button", func() Signal { return Signal{} }, Lifecycle[Signal]{OnMessage: func(*Context, *Signal, Message) error { buttonCalls++; return nil }}))
	must(t, registry.RegisterMod("empty", func() Signal { return Signal{} }, Lifecycle[Signal]{OnMessage: func(c *Context, _ *Signal, m Message) error {
		logicCalls++
		v, ok := m.Value[Input]()
		if !ok || m.Topic != "activate" {
			t.Fatal(m)
		}
		if _, ok := c.Get[scene.Sprite2D](v.Target); ok {
			t.Fatal("forward exposed foreign ownership")
		}
		return nil
	}}))
	r := newFixture(t, plan, registry, Options{})
	s, err := r.MountEntry()
	must(t, err)
	entity, err := s.modules[0].Element("hit")
	must(t, err)
	must(t, r.HandleInput(scene.InputEvent{Entity: entity.entity, Kind: scene.InputEventTap}))
	must(t, r.Dispatch())
	if sceneCalls != 1 || buttonCalls != 0 || logicCalls != 1 {
		t.Fatal(sceneCalls, buttonCalls, logicCalls)
	}
	must(t, r.Dispatch())
	if logicCalls != 1 || buttonCalls != 0 {
		t.Fatal(logicCalls, buttonCalls)
	}
}

func TestSceneForwardsRequestsAndReplyUsesOriginalTopic(t *testing.T) {
	plan, _ := compileFixture(t, map[string]string{
		"resources/scenes/main.json": `{"modules":[{"id":"client","mod":"client"},{"id":"server","mod":"server"}]}`,
		"client/mod.json":            `{}`, "server/mod.json": `{}`,
	})
	registry := Registry{}
	var requester Requester[string, int64]
	var result Result[int64]
	var duplicate error
	must(t, registry.RegisterScene("main.json", func() Signal { return Signal{} }, Lifecycle[Signal]{OnMessage: func(c *Context, _ *Signal, m Message) error {
		if !m.IsRequest() {
			return nil
		}
		if err := c.ForwardAs("server", "length", m); err != nil {
			return err
		}
		duplicate = c.Forward("client", m)
		return nil
	}}))
	must(t, registry.RegisterMod("client", func() Signal { return Signal{} }, Lifecycle[Signal]{Install: func(c *Context, _ *Signal) error {
		var err error
		requester, err = c.BindRequester[string, int64]("ask")
		return err
	}, OnMessage: func(_ *Context, _ *Signal, m Message) error {
		if m.Topic != "ask" {
			t.Fatal(m.Topic)
		}
		var ok bool
		result, ok = m.ResultValue[int64]()
		if !ok {
			return ErrType
		}
		return nil
	}}))
	must(t, registry.RegisterMod("server", func() Signal { return Signal{} }, Lifecycle[Signal]{OnMessage: func(c *Context, _ *Signal, m Message) error {
		if m.Topic != "length" || m.From != "client" {
			t.Fatal(m)
		}
		q, ok := m.RequestValue[string, int64]()
		if !ok {
			return ErrType
		}
		return q.Reply(c, int64(len(q.Value)))
	}}))
	r := newFixture(t, plan, registry, Options{})
	_, err := r.MountEntry()
	must(t, err)
	id, err := requester.Send("hello", RequestOptions{})
	must(t, err)
	must(t, r.Dispatch())
	if !errors.Is(duplicate, ErrCanceled) {
		t.Fatal(duplicate)
	}
	if result.ID != 0 {
		t.Fatal("reply arrived recursively")
	}
	must(t, r.Dispatch())
	if result.ID != id || result.Value != 5 || result.Err != nil {
		t.Fatal(result)
	}
}

func TestSceneAndModuleForwardingAllocateNothingWarm(t *testing.T) {
	plan, _ := compileFixture(t, map[string]string{
		"resources/scenes/main.json": `{"modules":[{"id":"source","mod":"empty"},{"id":"relay","mod":"relay"},{"id":"sink","mod":"empty"}]}`,
		"empty/mod.json":             `{}`, "relay/mod.json": `{}`,
	})
	type packet struct {
		Position [3]float32
		Count    int64
	}
	registry := Registry{}
	var emitter Emitter[packet]
	var received int64
	must(t, registry.RegisterScene("main.json", func() Signal { return Signal{} }, Lifecycle[Signal]{OnMessage: func(c *Context, _ *Signal, m Message) error {
		return c.Forward("relay", m)
	}}))
	must(t, registry.RegisterMod("relay", func() Signal { return Signal{} }, Lifecycle[Signal]{OnMessage: func(c *Context, _ *Signal, m Message) error { return c.Forward("sink", m) }}))
	must(t, registry.RegisterMod("empty", func() Signal { return Signal{} }, Lifecycle[Signal]{Install: func(c *Context, _ *Signal) error {
		if c.Name() != "source" {
			return nil
		}
		var err error
		emitter, err = c.BindEmitter[packet]("packet")
		return err
	}, OnMessage: func(_ *Context, _ *Signal, m Message) error {
		value, ok := m.Value[packet]()
		if !ok {
			return ErrType
		}
		received += value.Count
		return nil
	}}))
	r := newFixture(t, plan, registry, Options{})
	_, err := r.MountEntry()
	must(t, err)
	step := func() {
		if err := emitter.Emit(packet{Count: 1}); err != nil {
			panic(err)
		}
		if err := r.Dispatch(); err != nil {
			panic(err)
		}
		if err := r.Dispatch(); err != nil {
			panic(err)
		}
	}
	step()
	if allocations := testing.AllocsPerRun(1000, step); allocations != 0 {
		t.Fatal("relay allocations", allocations)
	}
	if received != 1002 {
		t.Fatal("relay dropped messages", received)
	}
}

func TestSceneInputForwardingAllocatesNothing(t *testing.T) {
	plan, _ := compileFixture(t, map[string]string{
		"resources/scenes/main.json": `{"modules":[{"id":"button","mod":"button"}]}`,
		"button/mod.json":            `{"components":{"sprite":{"color":"#ffffff","size":[20,20]},"input":{"pointer":true}}}`,
	})
	registry := Registry{}
	calls := 0
	must(t, registry.RegisterScene("main.json", func() Signal { return Signal{} }, Lifecycle[Signal]{OnMessage: func(c *Context, _ *Signal, m Message) error {
		if m.Topic == InputTopic {
			return c.Forward("button", m)
		}
		return nil
	}}))
	must(t, registry.RegisterMod("button", func() Signal { return Signal{} }, Lifecycle[Signal]{OnMessage: func(_ *Context, _ *Signal, m Message) error {
		if _, ok := m.Value[Input](); !ok {
			return ErrType
		}
		calls++
		return nil
	}}))
	r := newFixture(t, plan, registry, Options{})
	s, err := r.MountEntry()
	must(t, err)
	event := scene.InputEvent{Entity: s.modules[0].root, Kind: scene.InputEventTap}
	if allocations := testing.AllocsPerRun(1000, func() {
		if err := r.HandleInput(event); err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatal("input relay allocations", allocations)
	}
	if calls != 1001 || r.Queued() != 0 {
		t.Fatal(calls, r.Queued())
	}
}

func TestMessageTypeSharingKeepsSlotDecodersIndependent(t *testing.T) {
	plan, _ := compileFixture(t, map[string]string{
		"resources/project.json":     `{"entry":"main.json","types":{"Label":{"fields":{"text":"string"}}}}`,
		"resources/scenes/main.json": `{"modules":[{"id":"box","mod":"box"}]}`,
		"box/mod.json":               `{"slots":{"plain":{"type":"string","default":"plain"},"label":{"type":"Label","default":{"text":"structured"}}}}`,
	})
	registry := Registry{}
	must(t, registry.RegisterValue[string]("Label", func(v project.SlotValue) (string, error) { return v.Fields[0].Value.String, nil }))
	r := newFixture(t, plan, registry, Options{})
	s, err := r.MountEntry()
	must(t, err)
	for _, test := range []struct{ name, want string }{{"plain", "plain"}, {"label", "structured"}} {
		slot, err := s.modules[0].Slot(test.name)
		must(t, err)
		value, err := slot.Read[string]()
		must(t, err)
		if value != test.want {
			t.Fatal(test.name, value)
		}
	}
}
