package window

import (
	"testing"

	"github.com/diiyw/onix/input"
)

func TestTextInputSupportGatesAndDispatchesEvents(t *testing.T) {
	var support textInputSupport
	var committed string
	var composition input.TextComposition
	var command input.TextCommand
	support.OnTextInput(func(text string) { committed = text })
	support.OnTextComposition(func(event input.TextComposition) { composition = event })
	support.OnTextCommand(func(value input.TextCommand) { command = value })

	support.emitTextInput("ignored")
	support.emitTextComposition(input.TextComposition{Text: "ignored", Active: true})
	support.emitTextCommand(input.TextCommandEnter)
	if committed != "" || composition.Active || command != 0 {
		t.Fatal("inactive text input accepted events")
	}

	config := input.TextInputConfig{Revision: 1, Active: true}
	support.SetTextInputConfig(config)
	support.emitTextInput("你")
	support.emitTextComposition(input.TextComposition{Text: "ni", Active: true})
	support.emitTextCommand(input.TextCommandBackspace)
	if committed != "你" || composition.Text != "ni" || !composition.Active || command != input.TextCommandBackspace {
		t.Fatalf("unexpected dispatched values: %q %#v %d", committed, composition, command)
	}

	support.SetTextInputConfig(input.TextInputConfig{Revision: 2})
	support.emitTextComposition(input.TextComposition{})
	if composition.Active || composition.Text != "" {
		t.Fatalf("expected composition end while inactive: %#v", composition)
	}
}

func TestTextInputSupportAppliesConfigChangesOnce(t *testing.T) {
	var support textInputSupport
	var calls int
	var previous, current input.TextInputConfig
	support.applyConfig = func(before, after input.TextInputConfig) {
		calls++
		previous, current = before, after
	}

	config := input.TextInputConfig{Revision: 1, Kind: input.TextInputInteger, Active: true}
	support.SetTextInputConfig(config)
	support.SetTextInputConfig(config)
	if calls != 1 || previous != (input.TextInputConfig{}) || current != config {
		t.Fatalf("unexpected config application: calls=%d previous=%#v current=%#v", calls, previous, current)
	}
}

func TestTextInputSupportRejectsStaleSessions(t *testing.T) {
	var support textInputSupport
	first := input.TextInputConfig{Revision: 1, Active: true}
	second := input.TextInputConfig{Revision: 2, Active: true}
	support.SetTextInputConfig(first)
	if !support.acceptsTextInputSession(first.Value()) {
		t.Fatal("active text input session was rejected")
	}

	support.SetTextInputConfig(second)
	if support.acceptsTextInputSession(first.Value()) || !support.acceptsTextInputSession(second.Value()) {
		t.Fatal("stale text input session was accepted")
	}
}
