package editor

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/project"
	htmlui "github.com/diiyw/onix/ui/html"
)

func slotEditorFixture(t *testing.T) *ModuleEditor {
	t.Helper()
	a := workspaceEditorFixture(t)
	if err := a.Document.EditMod("button", "Typed slot fixture", func(mod *project.ModSpec) error {
		mod.Slots = map[string]project.SlotSpec{
			"text":    {Type: project.StringValue, Default: json.RawMessage(`"Hello"`)},
			"empty":   {Type: project.StringValue},
			"count~1": {Type: project.IntValue, Default: json.RawMessage(`9007199254740993`)},
			"amount":  {Type: project.NumberValue, Default: json.RawMessage(`0.25`)},
			"enabled": {Type: project.BoolValue, Default: json.RawMessage(`true`)},
			"off":     {Type: project.BoolValue},
			"point":   {Type: project.Vec2Value, Default: json.RawMessage(`[1,2]`)},
			"offset":  {Type: project.Vec3Value, Default: json.RawMessage(`[3,4,5]`)},
			"tint":    {Type: project.ColorValue, Default: json.RawMessage(`"#123456"`)},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.Document.Save(); err != nil {
		t.Fatal(err)
	}
	a.Selected = "start"
	workspaceShellTest(t, a)
	return a
}

func compactSlotJSON(t *testing.T, value json.RawMessage) string {
	t.Helper()
	var compact bytes.Buffer
	if err := json.Compact(&compact, value); err != nil {
		t.Fatal(err)
	}
	return compact.String()
}

func TestWorkspaceSlotControlsSaveTypedOverrides(t *testing.T) {
	a := slotEditorFixture(t)
	target := sceneDocumentID(a.Document.Active)
	id := func(name, axis string) string { return workspaceFieldID(target, "/modules/0/slots/"+name, axis) }
	if err := htmlui.Sync(a.world); err != nil {
		t.Fatal(err)
	}
	for _, access := range ecs.Query[htmlui.Control](a.world).All {
		control := access.Ref()
		if control.Owner != a.shell {
			continue
		}
		switch control.ID {
		case id("count~01", ""), id("amount", ""), id("point", "X"), id("offset", "Z"):
			if control.Type != "number" {
				t.Fatalf("%s did not create a number input", control.ID)
			}
		case id("text", ""):
			if control.Type != "text" || control.Value != "Hello" {
				t.Fatalf("string did not use a decoded text input: %+v", control)
			}
		case id("enabled", ""):
			if control.Type != "checkbox" || !control.Checked {
				t.Fatal("boolean did not show its inherited value in a checkbox")
			}
		}
	}
	a.handleEvent(htmlui.Event{ID: "content-step:" + id("count~01", "") + ":1", Kind: htmlui.EventClick})
	for _, edit := range []struct{ Name, Axis, Value string }{
		{"text", "", "Line 1\n\"quoted\" \\ & <text>"},
		{"amount", "", ".125"}, {"point", "X", "8.5"}, {"offset", "Z", "9.25"}, {"tint", "", "#abcdef80"},
	} {
		a.handleEvent(htmlui.Event{ID: id(edit.Name, edit.Axis), Kind: htmlui.EventInput, Value: edit.Value})
	}
	a.handleEvent(htmlui.Event{ID: id("enabled", ""), Kind: htmlui.EventChange, Checked: false})
	a.action("save")
	if a.hasDrafts() {
		t.Fatalf("slot edits did not save: %s", a.status)
	}
	reloaded, err := LoadWorkspaceDocument(a.Document.ProjectDir)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"text": `"Line 1\n\"quoted\" \\ \u0026 \u003ctext\u003e"`, "count~1": `9007199254740994`,
		"amount": `0.125`, "enabled": `false`, "point": `[8.5,2]`, "offset": `[3,4,9.25]`, "tint": `"#abcdef80"`,
	} {
		got := compactSlotJSON(t, reloaded.Instance("start").Slots[name])
		if got != want {
			t.Errorf("%s = %s, want %s", name, got, want)
		}
	}
	if len(reloaded.Instance("again").Slots) != 0 || reloaded.Mods["button"].Spec.Slots["text"].Default[1] != 'H' {
		t.Fatal("an override changed another instance or the module default")
	}
}

func TestWorkspaceSlotEmptyValuesAndResetPreserveDrafts(t *testing.T) {
	a := slotEditorFixture(t)
	target := sceneDocumentID(a.Document.Active)
	id := func(name, axis string) string { return workspaceFieldID(target, "/modules/0/slots/"+name, axis) }
	a.handleEvent(htmlui.Event{ID: id("off", ""), Kind: htmlui.EventChange, Checked: false})
	if !a.hasDrafts() {
		t.Fatal("an explicit false matching the inherited value was discarded")
	}
	a.handleEvent(htmlui.Event{ID: id("text", ""), Kind: htmlui.EventInput, Value: ""})
	a.handleEvent(htmlui.Event{ID: id("empty", ""), Kind: htmlui.EventInput, Value: ""})
	a.handleEvent(htmlui.Event{ID: id("point", "X"), Kind: htmlui.EventInput, Value: "1"})
	a.action("save")
	for name, want := range map[string]string{"off": `false`, "text": `""`, "empty": `""`, "point": `[1,2]`} {
		if got := compactSlotJSON(t, a.Document.Instance("start").Slots[name]); got != want {
			t.Fatalf("explicit %s = %s, want %s (%s)", name, got, want, a.status)
		}
	}
	workspaceShellTest(t, a)
	a.handleEvent(htmlui.Event{ID: id("point", "Y"), Kind: htmlui.EventInput, Value: "invalid"})
	a.handleEvent(htmlui.Event{ID: id("amount", ""), Kind: htmlui.EventInput, Value: "0.75"})
	a.handleEvent(htmlui.Event{ID: "slot-reset:" + id("point", ""), Kind: htmlui.EventClick})
	workspaceShellTest(t, a)
	if _, ok := a.viewScene(a.Document.Active).Modules[0].Slots["point"]; ok || a.Document.Instance("start").Slots["point"] == nil {
		t.Fatal("reset did not remain a visible draft until saving")
	}
	if a.fields[id("point", "Y")].Value != "2" {
		t.Fatal("reset did not restore the inherited vector")
	}
	a.handleEvent(htmlui.Event{ID: "slot-reset:" + id("text", ""), Kind: htmlui.EventClick})
	workspaceShellTest(t, a)
	a.handleEvent(htmlui.Event{ID: "slot-reset:" + id("off", ""), Kind: htmlui.EventClick})
	a.action("save")
	if a.hasDrafts() || string(a.Document.Instance("start").Slots["amount"]) != "0.75" {
		t.Fatalf("reset lost unrelated edits: %s", a.status)
	}
	for _, name := range []string{"point", "text", "off"} {
		if _, ok := a.Document.Instance("start").Slots[name]; ok {
			t.Errorf("reset retained the %s override", name)
		}
	}
	a.action("undo")
	if string(a.Document.Instance("start").Slots["text"]) != `""` || string(a.Document.Instance("start").Slots["off"]) != `false` {
		t.Fatal("undo did not restore explicit empty overrides")
	}
	a.action("redo")
	workspaceShellTest(t, a)
	a.handleEvent(htmlui.Event{ID: id("amount", ""), Kind: htmlui.EventInput, Value: ""})
	a.action("save")
	if _, ok := a.Document.Instance("start").Slots["amount"]; ok || a.hasDrafts() {
		t.Fatalf("clearing a number did not restore its default: %s", a.status)
	}
}

func TestWorkspaceSlotDefaultsUseTypedControls(t *testing.T) {
	a := slotEditorFixture(t)
	a.enterModuleEdit("main.json", "start")
	a.selectedSlot = "enabled"
	workspaceShellTest(t, a)
	id := workspaceFieldID(modDocumentID("button"), "/slots/enabled/default", "")
	a.handleEvent(htmlui.Event{ID: id, Kind: htmlui.EventChange, Checked: false})
	a.action("save")
	if got := string(a.Document.Mods["button"].Spec.Slots["enabled"].Default); got != "false" {
		t.Fatalf("checkbox default = %s (%s)", got, a.status)
	}
	workspaceShellTest(t, a)
	a.handleEvent(htmlui.Event{ID: "slot-reset:" + id, Kind: htmlui.EventClick})
	a.action("save")
	if a.Document.Mods["button"].Spec.Slots["enabled"].Default != nil || a.hasDrafts() {
		t.Fatalf("default reset was not saved: %s", a.status)
	}
	a.selectedSlot = "text"
	workspaceShellTest(t, a)
	id = workspaceFieldID(modDocumentID("button"), "/slots/text/default", "")
	a.handleEvent(htmlui.Event{ID: id, Kind: htmlui.EventInput, Value: "Plain text"})
	a.action("save")
	if string(a.Document.Mods["button"].Spec.Slots["text"].Default) != `"Plain text"` {
		t.Fatalf("string default was not JSON encoded: %s", a.status)
	}
	context := &ecs.SystemContext{World: a.world}
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if err := a.sync(context); err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatalf("stable slot inspector allocated %g times", allocations)
	}
}

func TestWorkspaceSlotStructuredValueKeepsJSONEditor(t *testing.T) {
	a := workspaceEditorFixture(t)
	target := DocumentID("resources/project.json")
	if err := a.Document.PatchDocuments("Value schema fixture", map[DocumentID][]DocumentPatch{
		target:                  {{Pointer: "/types", Value: json.RawMessage(`{"Style":{"fields":{"size":"int","label":"string"}}}`)}},
		modDocumentID("button"): {{Pointer: "/slots/style", Value: json.RawMessage(`{"type":"Style","default":{"size":12,"label":"Small"}}`)}},
	}); err != nil {
		t.Fatal(err)
	}
	a.Selected = "start"
	source := workspaceShellTest(t, a)
	id := workspaceFieldID(sceneDocumentID(a.Document.Active), "/modules/0/slots/style", "")
	if !strings.Contains(source, `<textarea id="`+id+`">`) {
		t.Fatal("structured slot lost its JSON editor")
	}
	a.handleEvent(htmlui.Event{ID: id, Kind: htmlui.EventInput, Value: `{"size":24,"label":"Large"}`})
	a.action("save")
	if got := compactSlotJSON(t, a.Document.Instance("start").Slots["style"]); got != `{"size":24,"label":"Large"}` || a.hasDrafts() {
		t.Fatalf("structured override = %s (%s)", got, a.status)
	}
}
