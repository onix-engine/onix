package module

import (
	"errors"
	"reflect"
	"testing"

	"github.com/diiyw/onix/project"
)

func TestPreparationUsesIndependentFullCapacityPayloadBanks(t *testing.T) {
	for _, slots := range []bool{false, true} {
		for _, fail := range []bool{false, true} {
			name := map[bool]string{false: "messages", true: "slots"}[slots] + "/" + map[bool]string{false: "commit", true: "rollback"}[fail]
			t.Run(name, func(t *testing.T) {
				plan, _ := compileFixture(t, map[string]string{
					"resources/project.json":     `{"entry":"main.json","scenes":["main.json","next.json"]}`,
					"resources/scenes/main.json": `{"modules":[{"id":"a","mod":"box"}]}`,
					"resources/scenes/next.json": `{"modules":[{"id":"a","mod":"box"}]}`,
					"box/mod.json":               `{"slots":{"value":{"type":"int"}}}`,
				})
				registry := Registry{}
				var target *Scene
				var frontSum, preparedSum int64
				must(t, registry.RegisterScene("main.json", func() Signal { return Signal{} }, Lifecycle[Signal]{
					OnMessage: func(_ *Context, _ *Signal, message Message) error {
						if message.Topic == "number" {
							value, _ := message.Value[int64]()
							frontSum += value
						}
						return nil
					},
				}))
				must(t, registry.RegisterScene("next.json", func() Signal { return Signal{} }, Lifecycle[Signal]{
					Enter: func(c *Context, _ *Signal, _ SceneStartup) error {
						if slots {
							writer, err := target.SlotWriter("a", "value")
							if err != nil {
								return err
							}
							for _, value := range [...]int64{100, 200} {
								if err = writer.Write(project.SlotValue{Type: project.IntValue, Integer: value}); err != nil {
									return err
								}
							}
						} else {
							for _, value := range [...]int64{100, 200} {
								if err := c.Send(SceneTarget, "number", value); err != nil {
									return err
								}
							}
						}
						if fail {
							return ErrInvalid
						}
						return nil
					},
					OnMessage: func(_ *Context, _ *Signal, message Message) error {
						if message.Topic == "number" {
							value, _ := message.Value[int64]()
							preparedSum += value
						}
						return nil
					},
				}))
				r := newFixture(t, plan, registry, Options{EventCapacity: 2})
				front, err := r.Manager().StartEntry()
				must(t, err)
				target, err = r.Manager().Preload(ScenePreloadRequest{Scene: "next.json"})
				must(t, err)
				writer, err := front.SlotWriter("a", "value")
				must(t, err)
				emitter, err := front.Emitter[int64]("number")
				must(t, err)
				for _, value := range [...]int64{10, 20} {
					if slots {
						must(t, writer.Write(project.SlotValue{Type: project.IntValue, Integer: value}))
					} else {
						must(t, emitter.Emit(value))
					}
				}
				tick, write := r.tick, r.write
				step := func() {
					_, prepareErr := r.Manager().prepareScene(target, target.ID(), SceneStartup{})
					if fail {
						if !errors.Is(prepareErr, ErrInvalid) {
							t.Fatalf("rollback error = %v", prepareErr)
						}
					} else {
						must(t, prepareErr)
					}
				}
				step()
				if r.Queued() != 2 || r.tick != tick || r.write != write || r.Manager().Active() != front || target.State() != SceneReady || frontSum != 0 {
					t.Fatal("preparation changed parked traffic or foreground state")
				}
				if !slots && ((!fail && preparedSum != 300) || (fail && preparedSum != 0)) {
					t.Fatal("startup messages used the foreground payload bank")
				}
				if slots && !fail {
					slot, _ := target.modules[0].Slot("value")
					value, err := slot.Read[int64]()
					must(t, err)
					if value != 200 {
						t.Fatal("startup slot traffic was overwritten")
					}
				}
				if !fail {
					step()
					if allocs := testing.AllocsPerRun(100, step); allocs != 0 {
						t.Fatalf("preparation with full parked buffers allocated %g times", allocs)
					}
				}
				must(t, r.Dispatch())
				if slots {
					slot, _ := front.modules[0].Slot("value")
					value, err := slot.Read[int64]()
					must(t, err)
					if value != 20 {
						t.Fatal("preparation corrupted parked slot data")
					}
				} else if frontSum != 30 {
					t.Fatal("preparation corrupted parked messages")
				}
			})
		}
	}
}

func TestSlotDecodersDoNotAllocateMessageStorageAndAliasesSharePayloads(t *testing.T) {
	plan, _ := compileFixture(t, map[string]string{
		"resources/project.json":     `{"entry":"main.json","scenes":["main.json"],"types":{"Label":{"fields":{"text":"string"}}}}`,
		"resources/scenes/main.json": `{"modules":[{"id":"a","mod":"box"}]}`,
		"box/mod.json":               `{"slots":{"label":{"type":"Label","default":{"text":"label"}},"text":{"type":"string","default":"plain"}}}`,
	})
	registry := Registry{}
	must(t, registry.RegisterValue("Label", func(value project.SlotValue) (string, error) {
		return value.Fields[0].Value.String, nil
	}))
	r := newFixture(t, plan, registry, Options{})
	s, err := r.MountEntry()
	must(t, err)
	c := &s.modules[0]
	label, _ := c.Slot("label")
	plain, _ := c.Slot("text")
	for _, slot := range [...]Slot{label, plain} {
		_, err = slot.Read[string]()
		must(t, err)
	}
	if len(r.typedStores) != 0 {
		t.Fatal("slot decoding eagerly allocated message payload buffers")
	}
	first, err := c.BindEmitter[string]("first")
	must(t, err)
	second, err := c.BindEmitter[string]("second")
	must(t, err)
	if first.store != second.store || len(r.typedStores) != 1 {
		t.Fatal("the same Go type allocated multiple payload stores")
	}
	must(t, first.EmitTo("a", "payload"))
	if len(r.usedStores[r.buffer()]) != 1 {
		t.Fatal("used message storage was not recorded")
	}
	must(t, r.Dispatch())
	if len(first.store.values[0]) != 0 || first.store.values[0][:1][0] != "" || len(r.usedStores[0]) != 0 {
		t.Fatal("dispatch retained message payload references")
	}
	// Registration changes after creation cannot affect existing slot decoders.
	registry.values["Label"] = func(project.SlotValue) (int64, error) { return 0, nil }
	value, err := label.Read[string]()
	must(t, err)
	if value != "label" || r.typedStores[reflect.TypeFor[string]()] != first.store {
		t.Fatal("runtime did not preserve independent decoding and typed storage")
	}
}
