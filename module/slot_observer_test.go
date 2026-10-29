package module

import (
	"errors"
	"testing"

	"github.com/diiyw/onix/project"
)

func TestSlotChangeCoalescesAndIgnoresUnchangedWrites(t *testing.T) {
	plan, _ := compileFixture(t, map[string]string{
		"resources/scenes/main.json": `{"modules":[{"id":"box","mod":"box"}]}`,
		"box/mod.json":               `{"slots":{"text":{"type":"string"},"size":{"type":"number"}}}`,
	})
	registry := Registry{}
	var text, size Slot
	callbacks := 0
	must(t, registry.RegisterMod("box", func() Signal { return Signal{} }, Lifecycle[Signal]{Install: func(c *Context, _ *Signal) error {
		text, _ = c.Slot("text")
		size, _ = c.Slot("size")
		return OnSlotsChange(c, []string{"text", "size"}, func(*Context) error { callbacks++; return nil })
	}}))
	r := newFixture(t, plan, registry, Options{EventCapacity: 1})
	_, err := r.MountEntry()
	must(t, err)
	revision := text.Revision()
	must(t, text.Set(project.SlotValue{Type: project.StringValue}))
	if r.Queued() != 0 || revision.stale() {
		t.Fatal("unchanged write requested new text")
	}
	must(t, text.Set(project.SlotValue{Type: project.StringValue, String: "one"}))
	must(t, text.Set(project.SlotValue{Type: project.StringValue, String: "two"}))
	must(t, size.Set(project.SlotValue{Type: project.NumberValue, Number: 24}))
	if r.Queued() != 1 || callbacks != 0 {
		t.Fatal("slot change recursed or failed to coalesce")
	}
	must(t, r.Dispatch())
	if callbacks != 1 {
		t.Fatal("coalesced observer did not run")
	}
	// A writer event occupies the queue; internal Set must reject pressure
	// before modifying state/revision or silently dropping its notification.
	writer, err := text.context.scene.SlotWriter("box", "text")
	must(t, err)
	must(t, writer.Write(project.SlotValue{Type: project.StringValue, String: "three"}))
	revision = text.Revision()
	if err = text.Set(project.SlotValue{Type: project.StringValue, String: "rejected"}); !errors.Is(err, ErrQueueFull) || revision.stale() {
		t.Fatal("queue overflow partially applied slot change")
	}
	step := func() { must(t, r.Dispatch()) }
	step()
}
