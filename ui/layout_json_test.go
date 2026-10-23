//go:build !onix_release

package ui

import (
	"encoding/json"
	"testing"

	"github.com/diiyw/onix/ecs"
)

func TestWidgetJSONTargetAndEnabledDefaults(t *testing.T) {
	var widget Widget
	if err := json.Unmarshal([]byte(`{"alignLeft":true,"target":"panel"}`), &widget); err != nil {
		t.Fatal(err)
	}
	if !widget.Enabled || widget.TargetID != "panel" || widget.Target != 0 {
		t.Fatalf("decoded widget = %+v, want enabled target id panel without runtime entity", widget)
	}
	encoded, err := json.Marshal(Widget{Enabled: true, Target: ecs.Entity(42), TargetID: "panel"})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"target":"panel","enabled":true}` {
		t.Fatalf("encoded widget = %s", encoded)
	}
}
