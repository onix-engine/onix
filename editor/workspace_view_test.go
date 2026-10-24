package editor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	engineui "github.com/diiyw/onix/ui"
	htmlui "github.com/diiyw/onix/ui/html"
	"github.com/diiyw/onix/window"
)

// Render the real component tree; serialization is only used by DOM assertions.
func workspaceShellTest(t *testing.T, a *ModuleEditor) string {
	t.Helper()
	if a.world == nil {
		a.world = ecs.NewWorld()
	}
	if err := a.syncWorkspaceComponent(a.world); err != nil {
		t.Fatal(err)
	}
	source, err := htmlui.Serialize(a.world, a.shell)
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func workspaceEditorFixture(t *testing.T) *ModuleEditor {
	t.Helper()
	directory := t.TempDir()
	t.Setenv("USERPROFILE", directory)
	t.Setenv("HOME", directory)
	w := workspaceFixture(t)
	a, err := NewModuleEditor(w.ProjectDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.dispose)
	a.width, a.height = 1600, 900
	return a
}

func TestWorkspaceEditorPreviewsPrivatePartsWithRuntimeComponents(t *testing.T) {
	a := workspaceEditorFixture(t)
	center := scene.AnchorCenter
	if err := a.Document.EditMod("button", "Add private parts", func(mod *project.ModSpec) error {
		mod.Elements = []project.ModElementSpec{
			{ID: "parent", Transform: project.ModTransformSpec{Position: []float32{4, 5}}, Components: project.ModComponents{Anchor: &center, Sprite: &project.ModSpriteSpec{Color: "#ff0000", Size: []float32{30, 10}}}},
			{ID: "child", Parent: "parent", Transform: project.ModTransformSpec{Position: []float32{2, 3}}, Components: project.ModComponents{Sprite: &project.ModSpriteSpec{Color: "#ffffff", Size: []float32{2, 2}}}},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	a.changed()
	context := &ecs.SystemContext{World: ecs.NewWorld()}
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	v := &a.viewport2D
	row := &v.rows[0]
	child, _ := ecs.Get[scene.UIGlobalTransform](v.world, row.parts[1])
	if len(row.parts) != 2 || child.Matrix[12] != 51 || child.Matrix[13] != 75 {
		t.Fatal("private parent transform was lost", child.Matrix)
	}
	anchor, _ := ecs.Get[scene.Anchor](v.world, row.parts[0])
	sprite, _ := ecs.Get[scene.Sprite2D](v.world, row.parts[0])
	if anchor != center || sprite.Width != 30 || sprite.Height != 10 {
		t.Fatal("private sprite did not use runtime anchor and size")
	}
	root, childEntity, other := row.root, row.parts[1], v.rows[2].root
	if err := a.Document.EditInstance("start", "Move placement", func(instance *project.ModInstanceSpec) error {
		instance.Placement.Position = []float32{34, 44}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	a.changed()
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	child, _ = ecs.Get[scene.UIGlobalTransform](v.world, row.parts[1])
	if row.root != root || row.parts[1] != childEntity || child.Matrix[12] != 55 || v.rows[2].root != other {
		t.Fatal("placement edit replaced content or lost transforms")
	}
	if allocs := testing.AllocsPerRun(100, func() {
		if err := a.sync(context); err != nil {
			panic(err)
		}
		if err := a.syncViewport2D(); err != nil {
			panic(err)
		}
	}); allocs != 0 {
		t.Fatalf("stable private preview allocated %g", allocs)
	}
}

func TestWorkspaceEditorScenesFilesInspectorAndFormTargets(t *testing.T) {
	a := workspaceEditorFixture(t)
	if a.Document.Active != "main.json" {
		t.Fatal("did not open entry scene")
	}
	workspaceShellTest(t, a)
	a.handleEvent(htmlui.Event{ID: "select:start", Kind: htmlui.EventClick})
	source := workspaceShellTest(t, a)
	if a.selectedMod() != "button" || !strings.Contains(source, `>private</span>`) || strings.Contains(source, "button/private/data.json") || !strings.Contains(source, `id="definition-identity-title"`) || !strings.Contains(source, `class="element-title">start</h2>`) || strings.Contains(source, `class="detail-kind"`) || strings.Contains(source, `class="detail-source"`) || strings.Contains(source, ">Nodes<") {
		t.Fatal("selection did not switch Scenes/file/Mod views")
	}
	modRevision := a.Document.Mods["button"].Revision
	id := "field:resources%2Fscenes%2Fmain.json:/modules/0/placement/position:X"
	a.handleEvent(htmlui.Event{ID: id, Kind: htmlui.EventInput, Value: "123"})
	a.action("save")
	if a.Document.Instance("start").Placement.Position[0] != 123 || a.Document.Mods["button"].Revision != modRevision {
		t.Fatalf("form placement wrote wrong target: %s", a.status)
	}
	workspaceShellTest(t, a)
	id = workspaceFieldID("button/mod.json", "/components/size", "Height")
	a.handleEvent(htmlui.Event{ID: id, Kind: htmlui.EventInput, Value: "77"})
	a.action("save")
	if size := a.Document.Mods["button"].Spec.Components.Size; size[0] != 120 || size[1] != 77 {
		t.Fatalf("definition form failed: %s", a.status)
	}
	a.time = 10
	a.openFile("button/mod.json")
	a.time = 10.1
	a.openFile("button/mod.json")
	if a.Local != "button" {
		t.Fatal("double-click did not open local preview")
	}
	workspaceShellTest(t, a)
	if _, exists := a.fields["field:resources%2Fscenes%2Fmain.json:/modules/0/placement/position:X"]; exists {
		t.Fatal("local preview exposed scene placement as module local")
	}
	if err := a.Document.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadWorkspaceDocument(a.Document.ProjectDir)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Instance("start").Placement.Position[0] != 123 || reloaded.Mods["button"].Spec.Components.Size[1] != 77 {
		t.Fatal("UI changes did not survive saving")
	}
}

func TestWorkspaceModuleInspectorUsesContentSize(t *testing.T) {
	a := workspaceEditorFixture(t)
	a.enterModuleEdit("main.json", "start")
	source := workspaceShellTest(t, a)
	for _, removed := range []string{`id="definition-size-title"`, `>Identity<`, `mod.go behavior type`, `>W</label>`, `>H</label>`} {
		if strings.Contains(source, removed) {
			t.Fatalf("module Details still contains %q", removed)
		}
	}
	if a.sectionsCollapsed["definition-identity"] || !strings.Contains(source, `id="definition-identity-title">Module Definition</span>`) {
		t.Fatal("module properties should open with a descriptive title")
	}
	target := modDocumentID("button")
	assertSize := func(width, height string) {
		t.Helper()
		workspaceShellTest(t, a)
		for axis, want := range map[string]string{"Width": width, "Height": height} {
			if got := a.fields[workspaceFieldID(target, "/components/size", axis)].Value; got != want {
				t.Fatalf("%s = %s, want %s", axis, got, want)
			}
		}
	}
	assertSize("120", "40") // Root sprite size without components.size.
	if err := a.Document.EditMod("button", "Background element", func(mod *project.ModSpec) error {
		mod.Components = project.ModComponents{}
		mod.Space = project.CoordinateSpaceUI
		mod.Transform.Scale = []float32{3, 3}
		mod.Elements = []project.ModElementSpec{{ID: "background", Components: project.ModComponents{Size: []float32{720, 1280}}}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	assertSize("720", "1280") // Element bounds exclude module/scene placement and scale.
	if err := a.Document.EditMod("button", "Nested sprites", func(mod *project.ModSpec) error {
		mod.Elements = []project.ModElementSpec{
			{ID: "parent", Transform: project.ModTransformSpec{Position: []float32{50, 30}, Scale: []float32{2, 3}}},
			{ID: "child", Parent: "parent", Components: project.ModComponents{Sprite: &project.ModSpriteSpec{Size: []float32{10, 20}}}},
			{ID: "sibling", Transform: project.ModTransformSpec{Position: []float32{80, 40}}, Components: project.ModComponents{Size: []float32{10, 20}}},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	assertSize("40", "60")
	mod := a.Document.Mods["button"]
	if allocations := testing.AllocsPerRun(100, func() { a.definitionSize(mod) }); allocations != 0 {
		t.Fatalf("module size calculation allocated %g after warmup", allocations)
	}
	a.handleEvent(htmlui.Event{ID: workspaceFieldID(target, "/components/size", "Width"), Kind: htmlui.EventInput, Value: "100"})
	a.action("save")
	assertSize("100", "60") // Preserve the derived height when editing only width.
	a.action("undo")
	assertSize("40", "60")
}

func TestWorkspaceInspectorShowsNestedInstanceProperties(t *testing.T) {
	a := workspaceEditorFixture(t)
	for index, id := range []string{"other", "again"} {
		if err := a.Document.MoveInstance(id, "start", index); err != nil {
			t.Fatal(err)
		}
	}
	a.expandModule(workspaceModuleKey{Scene: a.Document.Active, ID: "start"})
	a.selectInstance("again")
	source := workspaceShellTest(t, a)
	id := workspaceFieldID(sceneDocumentID(a.Document.Active), "/modules/0/children/1/placement/position", "X")
	if _, ok := a.fields[id]; !ok {
		t.Fatal("nested instance fields lost their scene patch path")
	}
	if row := a.rowByID["again"]; row.Parent != "start" || row.Index != 1 {
		t.Fatal("the scene tree lost the instance's parent or sibling order")
	}
	for _, removed := range []string{`id="instance-hierarchy-title"`, `id="add/parent"`, `id="add/index"`, `id="add:move"`, `id="instance-actions-title"`, `id="new-instance"`, `id="duplicate"`, `id="delete-instance"`} {
		if strings.Contains(source, removed) {
			t.Fatalf("Details still contains %s", removed)
		}
	}
}

func TestWorkspaceInspectorShowsSelectedTreeElement(t *testing.T) {
	a := workspaceEditorFixture(t)
	if err := a.Document.EditMod("button", "Add elements", func(mod *project.ModSpec) error {
		mod.Elements = []project.ModElementSpec{{ID: "face", Transform: project.ModTransformSpec{Position: []float32{4, 5}}}, {ID: "glow", Parent: "face"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	a.enterModuleEdit("main.json", "start")
	source := workspaceShellTest(t, a)
	if strings.Contains(source, "definition-transform") || strings.Contains(source, "definition-elements") || strings.Contains(source, "definition-content") || !strings.Contains(source, `id="element:main.json/start/face"`) {
		t.Fatal("module definition still carries its own transform, content or element list")
	}
	a.handleEvent(htmlui.Event{ID: "element:main.json/start/glow", Kind: htmlui.EventClick})
	source = workspaceShellTest(t, a)
	if a.selectedElement != "glow" || !strings.Contains(source, `id="element-transform-title"`) || strings.Contains(source, `id="definition-identity-title"`) {
		t.Fatalf("element row did not switch Details: %s", a.status)
	}
	id := workspaceFieldID("button/mod.json", "/elements/1/transform/position", "Y")
	a.handleEvent(htmlui.Event{ID: id, Kind: htmlui.EventInput, Value: "77"})
	a.action("save")
	if a.Document.Mods["button"].Spec.Elements[1].Transform.Position[1] != 77 || a.selectedElement != "glow" {
		t.Fatalf("element form failed: %s", a.status)
	}
	a.handleEvent(htmlui.Event{ID: "select:start", Kind: htmlui.EventClick})
	if source = workspaceShellTest(t, a); a.selectedElement != "" || !strings.Contains(source, `id="definition-identity-title"`) {
		t.Fatal("reselecting the module root did not restore the definition panel")
	}
}

func TestWorkspaceInspectorShowsSelectedTreeSlot(t *testing.T) {
	a := workspaceEditorFixture(t)
	a.enterModuleEdit("main.json", "start")
	source := workspaceShellTest(t, a)
	for _, removed := range []string{"definition-slots-title", "definition-actions-title", "copy-mod", "copy-definition", "delete-definition"} {
		if strings.Contains(source, `id="`+removed+`"`) {
			t.Fatalf("Details still contains %s", removed)
		}
	}
	if !strings.Contains(source, `id="slot:main.json/start/text"`) || !strings.Contains(source, `id="slot:main.json/start/label"`) {
		t.Fatal("the module tree does not expose its slots")
	}
	a.handleEvent(htmlui.Event{ID: "slot:main.json/start/text", Kind: htmlui.EventClick})
	source = workspaceShellTest(t, a)
	if a.selectedSlot != "text" || !strings.Contains(source, `id="slot-details-title"`) || !strings.Contains(source, `id="slot-transform-title"`) || strings.Contains(source, `id="definition-identity-title"`) {
		t.Fatal("selecting a slot did not show its own Details")
	}
	target := modDocumentID("button")
	if _, ok := a.fields[workspaceFieldID(target, "/slots/label/type", "")]; ok {
		t.Fatal("slot Details includes another slot's fields")
	}
	a.handleEvent(htmlui.Event{ID: workspaceFieldID(target, "/slots/text/default", ""), Kind: htmlui.EventInput, Value: "Changed"})
	a.handleEvent(htmlui.Event{ID: workspaceFieldID(target, "/slots/text/writable", ""), Kind: htmlui.EventChange, Checked: false})
	a.handleEvent(htmlui.Event{ID: workspaceFieldID(target, "/slots/text/placement/position", "Y"), Kind: htmlui.EventInput, Value: "42"})
	a.handleEvent(htmlui.Event{ID: "slot:main.json/start/label", Kind: htmlui.EventClick})
	if a.pendingAction != "slot:main.json/start/label" || a.selectedSlot != "text" {
		t.Fatal("switching slots discarded pending changes")
	}
	a.action("save-pending")
	slot := a.Document.Mods["button"].Spec.Slots["text"]
	if a.selectedSlot != "label" || string(slot.Default) != `"Changed"` || slot.Writable == nil || *slot.Writable || slot.Placement.Position[1] != 42 {
		t.Fatalf("slot changes were not applied before selection: %+v (%s)", slot, a.status)
	}
	a.action("undo")
	if string(a.Document.Mods["button"].Spec.Slots["text"].Default) != `"Hello"` {
		t.Fatal("undo did not restore the slot's configuration")
	}
	a.handleEvent(htmlui.Event{ID: "tree-group:main.json/start/slots", Kind: htmlui.EventClick})
	if source = workspaceShellTest(t, a); strings.Contains(source, `id="slot:main.json/start/text"`) || !strings.Contains(source, `id="tree-group:main.json/start/resources"`) {
		t.Fatal("collapsing Slots affected Resources or left slot rows visible")
	}
	a.handleEvent(htmlui.Event{ID: "select:start", Kind: htmlui.EventClick})
	if source = workspaceShellTest(t, a); a.selectedSlot != "" || !strings.Contains(source, `id="definition-identity-title"`) {
		t.Fatal("module selection did not restore its Details")
	}
}

func TestWorkspaceSceneInspectorCommitsOnlyPosition(t *testing.T) {
	a := workspaceEditorFixture(t)
	a.selectInstance("start")
	workspaceShellTest(t, a)
	a.selectScene("main.json")
	source := workspaceShellTest(t, a)
	if len(a.fields) != 3 || !strings.Contains(source, `class="detail-section"`) {
		t.Fatal("scene detail did not replace module fields with the Position panel")
	}
	for _, text := range []string{"Scene Go behavior", "Rotation (radians)", ">Scale<", "Module definitions", "Module definition:", `id="apply-form"`, `id="clear-drafts"`} {
		if strings.Contains(source, text) {
			t.Errorf("scene detail still contains %q", text)
		}
	}
	target := sceneDocumentID(a.Document.Active)
	for i, axis := range [...]string{"X", "Y", "Z"} {
		field := a.fields[workspaceFieldID(target, "/transform/position", axis)]
		if field.Axis != i || field.Target != target || !field.CommitOnChange || field.Vector != [4]float64{10, 20, 0} {
			t.Fatalf("incorrect scene position field: %+v", field)
		}
	}
	modRevision, history := a.Document.Mods["button"].Revision, len(a.Document.undo)
	x := workspaceFieldID(target, "/transform/position", "X")
	y := workspaceFieldID(target, "/transform/position", "Y")
	a.handleEvent(htmlui.Event{ID: x, Kind: htmlui.EventInput, Value: "42.5"})
	if a.Document.Scene().Spec.Transform.Position[0] != 10 {
		t.Fatal("typing committed a partial value")
	}
	for _, kind := range []htmlui.EventKind{htmlui.EventChange, htmlui.EventSubmit, htmlui.EventBlur} {
		a.handleEvent(htmlui.Event{ID: x, Kind: kind, Value: "42.5"})
	}
	a.handleEvent(htmlui.Event{ID: y, Kind: htmlui.EventChange, Value: "-3"})
	position := a.Document.Scene().Spec.Transform.Position
	if len(position) != 3 || position[0] != 42.5 || position[1] != -3 || position[2] != 0 || a.hasDrafts() || len(a.Document.undo) != history+2 || a.Document.Mods["button"].Revision != modRevision {
		t.Fatalf("scene position commit lost values or duplicated history: %v", position)
	}
	a.handleEvent(htmlui.Event{ID: x, Kind: htmlui.EventChange, Value: "invalid"})
	if a.Document.Scene().Spec.Transform.Position[0] != 42.5 || !a.hasDrafts() {
		t.Fatal("invalid position changed the document or lost its draft")
	}
	a.handleEvent(htmlui.Event{ID: x, Kind: htmlui.EventChange, Value: "10"})
	if a.Document.Scene().Spec.Transform.Position[0] != 10 || a.hasDrafts() {
		t.Fatal("committing the original value did not restore X")
	}
	a.action("undo")
	if a.Document.Scene().Spec.Transform.Position[0] != 42.5 {
		t.Fatal("position edit could not be undone")
	}
	a.action("save")
	reloaded, err := LoadWorkspaceDocument(a.Document.ProjectDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Scene().Spec.Transform.Position; got[0] != 42.5 || got[1] != -3 {
		t.Fatalf("position edits were not saved: %v", got)
	}
}

func TestWorkspaceEditorHTMLSemanticsAndEscaping(t *testing.T) {
	a := workspaceEditorFixture(t)
	if err := a.Document.EditMod("button", "Unsafe title", func(mod *project.ModSpec) error { mod.Name = `<script>"&`; return nil }); err != nil {
		t.Fatal(err)
	}
	a.Selected = "start"
	root, err := dom.Parse(workspaceShellTest(t, a))
	if err != nil {
		t.Fatal(err)
	}
	ids := make(map[string]*dom.Node)
	var labels []string
	var visit func(*dom.Node)
	visit = func(node *dom.Node) {
		if id, ok := node.Attr("id"); ok {
			if ids[id] != nil {
				t.Errorf("duplicate HTML id %s", id)
			}
			ids[id] = node
		}
		if node.Type == dom.ElementNode && node.Data == "script" {
			t.Error("unescaped user value became HTML")
		}
		if node.Type == dom.ElementNode && node.Data == "label" {
			label, ok := node.Attr("for")
			if !ok {
				t.Error("label has no associated control")
			}
			labels = append(labels, label)
		}
		if role, _ := node.Attr("role"); role == "treeitem" {
			if _, ok := node.Attr("aria-level"); !ok {
				t.Error("treeitem lacks level")
			}
			if _, ok := node.Attr("aria-selected"); !ok {
				t.Error("treeitem lacks selection state")
			}
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	visit(root)
	for _, label := range labels {
		if ids[label] == nil {
			t.Errorf("label references missing control %s", label)
		}
	}
}

func TestWorkspaceEditorInvalidDraftCloseAndDiscardNavigation(t *testing.T) {
	a := workspaceEditorFixture(t)
	if err := a.Document.EditMod("button", "Integer slot fixture", func(mod *project.ModSpec) error {
		mod.Slots["text"] = project.SlotSpec{Type: project.IntValue, Default: json.RawMessage(`0`)}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.Document.Save(); err != nil {
		t.Fatal(err)
	}
	a.Selected = "start"
	workspaceShellTest(t, a)
	id := "field:resources%2Fscenes%2Fmain.json:/modules/0/slots/text"
	a.handleEvent(htmlui.Event{ID: id, Kind: htmlui.EventInput, Value: "1.5"}) // not an integer
	a.action("save")
	if !strings.Contains(a.status, id) || !a.hasDrafts() || a.Document.Dirty() {
		t.Fatalf("invalid draft changed document or lost diagnosis: %s", a.status)
	}
	if a.closeRequested() || a.pendingAction != "close" {
		t.Fatal("close discarded unapplied form")
	}
	a.action("cancel-pending")
	a.handleEvent(htmlui.Event{ID: id, Kind: htmlui.EventInput, Value: "7"})
	if err := a.applyDrafts(); err != nil {
		t.Fatal(err)
	}
	if !a.Document.Dirty() {
		t.Fatalf("valid form was not applied: %s", a.status)
	}
	next := workspaceFixture(t)
	a.pathDraft = next.ProjectDir
	a.action("open")
	if a.pendingAction != "open" {
		t.Fatal("open ignored unsaved documents")
	}
	a.action("discard-pending")
	if a.pendingAction != "" || a.Document.ProjectDir != next.ProjectDir {
		t.Fatal("discard-and-open prompted again or did not navigate")
	}
}

func TestWorkspaceEditorRetainedPreviewAndStableFrameAllocations(t *testing.T) {
	a := workspaceEditorFixture(t)
	world := ecs.NewWorld()
	context := &ecs.SystemContext{World: world}
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if err := htmlui.Sync(world); err != nil {
		t.Fatal(err)
	}
	if len(a.viewport2D.rows) != 3 {
		t.Fatal("preview rows missing")
	}
	start, again, other := a.viewport2D.rows[0].root, a.viewport2D.rows[1].root, a.viewport2D.rows[2].root
	otherRevision := a.viewport2D.rows[2].revision
	if err := a.Document.EditMod("button", "Preview edit", func(mod *project.ModSpec) error { mod.Transform.Position = []float32{40, 50}; return nil }); err != nil {
		t.Fatal(err)
	}
	a.changed()
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if a.viewport2D.rows[0].root != start || a.viewport2D.rows[1].root != again || a.viewport2D.rows[2].root != other || a.viewport2D.rows[2].revision != otherRevision {
		t.Fatal("definition transform edit rebuilt retained entities or updated unrelated instances")
	}
	startTransform, _ := ecs.Get[scene.UIGlobalTransform](a.viewport2D.world, start)
	againTransform, _ := ecs.Get[scene.UIGlobalTransform](a.viewport2D.world, again)
	if startTransform.Matrix[12] != 80 || againTransform.Matrix[12] != 250 {
		t.Fatal("definition transform edit did not update every reused instance")
	}
	if err := htmlui.Sync(world); err != nil {
		t.Fatal(err)
	}
	allocations := testing.AllocsPerRun(100, func() {
		if err := a.update(context); err != nil {
			panic(err)
		}
		if err := a.sync(context); err != nil {
			panic(err)
		}
		a.Preview.Sync(a.Document, "")
		if err := htmlui.Sync(world); err != nil {
			panic(err)
		}
	})
	if allocations != 0 {
		t.Fatalf("stable editor frame allocated %g", allocations)
	}
}

func TestWorkspaceEditorSlotFormPreservesJSONValue(t *testing.T) {
	a := workspaceEditorFixture(t)
	a.enterModuleEdit("main.json", "start")
	a.handleEvent(htmlui.Event{ID: "slot:main.json/start/text", Kind: htmlui.EventClick})
	workspaceShellTest(t, a)
	id := workspaceFieldID("button/mod.json", "/slots/text/default", "")
	a.handleEvent(htmlui.Event{ID: id, Kind: htmlui.EventInput, Value: "Changed"})
	patches, err := a.formPatches()
	if err != nil {
		t.Fatal(err)
	}
	if string(patches["button/mod.json"][0].Value) != `"Changed"` || !json.Valid(patches["button/mod.json"][0].Value) {
		t.Fatal("slot form lost JSON value")
	}
}

func TestWorkspaceEditorPointerSelectionAndPlacementDragSingleUndo(t *testing.T) {
	a := workspaceEditorFixture(t)
	if err := a.Document.MoveInstance("again", "start", 0); err != nil {
		t.Fatal(err)
	}
	if err := a.Document.EditScene(a.Document.Active, "Scale scene", func(spec *project.SceneSpec) error { spec.Transform.Scale = []float32{2, 2}; return nil }); err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	engineui.RegisterRuntimeFeature[engineui.FullRuntime](world)
	ecs.InsertResource(world, input.NewState())
	context := &ecs.SystemContext{World: world}
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if err := htmlui.Sync(world); err != nil {
		t.Fatal(err)
	}
	// A real generated button event reaches the editor's retained message reader.
	var selectEntity ecs.Entity
	for entity, access := range ecs.Query[htmlui.Control](world).All {
		if access.Ref().Owner == a.shell && access.Ref().ID == "select:start" {
			selectEntity = entity
			break
		}
	}
	if selectEntity == 0 {
		t.Fatal("missing generated instance selector")
	}
	scene.DispatchInputEvent(world, scene.InputEvent{Entity: selectEntity, Kind: scene.InputEventTap})
	if err := engineui.UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if err := a.update(context); err != nil {
		t.Fatal(err)
	}
	if a.Selected != "start" {
		t.Fatal("generated HTML event did not select module")
	}
	modRevision, history := a.Document.Mods["button"].Revision, len(a.Document.undo)
	a.canvasScale = .5
	a.beginPlacementDrag("start", 100, 100)
	a.updatePlacementDrag(input.Pointer{X: 120, Y: 130, Down: true})
	if len(a.Document.undo) != history || a.Document.Instance("start").Placement.Position[0] != 30 {
		t.Fatal("drag movement wrote undo history before release")
	}
	a.updatePlacementDrag(input.Pointer{X: 120, Y: 130, Released: true})
	position := a.Document.Instance("start").Placement.Position
	if position[0] != 50 || position[1] != 70 || a.Document.Mods["button"].Revision != modRevision || len(a.Document.undo) != history+1 {
		t.Fatalf("drag did not invert scene scale or isolate placement: %v", position)
	}
	if err := a.Document.Undo(); err != nil {
		t.Fatal(err)
	}
	if a.Document.Instance("start").Placement.Position[0] != 30 {
		t.Fatal("one undo did not restore entire drag")
	}
}

func TestWorkspaceEditorOpenInvalidatesPriorPreviewAndDrafts(t *testing.T) {
	a := workspaceEditorFixture(t)
	world := ecs.NewWorld()
	context := &ecs.SystemContext{World: world}
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	original := a.viewport2D.world
	other := workspaceFixture(t)
	if err := other.EditMod("button", "Color", func(mod *project.ModSpec) error { mod.Components.Sprite.Color = "#ff0000"; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := other.Save(); err != nil {
		t.Fatal(err)
	}
	if err := a.open(other.ProjectDir); err != nil {
		t.Fatal(err)
	}
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if a.viewport2D.world == original {
		t.Fatal("new project reused prior project's preview cache")
	}
	a.Selected = "start"
	workspaceShellTest(t, a)
	a.handleEvent(htmlui.Event{ID: "field:resources%2Fscenes%2Fmain.json:/modules/0/placement/position:X", Kind: htmlui.EventInput, Value: "88"})
	a.time = 20
	a.openFile("button/mod.json")
	a.time = 20.1
	a.openFile("button/mod.json")
	if a.pendingAction != "local:button" || a.Local != "" {
		t.Fatal("local preview silently discarded scene form draft")
	}
	a.action("discard-pending")
	if a.Local != "button" || a.hasDrafts() {
		t.Fatal("draft discard did not enter local preview")
	}
}

func TestWorkspaceEditorWhitespaceFileNamesHaveValidControlIDs(t *testing.T) {
	a := workspaceEditorFixture(t)
	if err := a.Document.CopyMod("button", "custom module"); err != nil {
		t.Fatal(err)
	}
	if err := a.Document.CreateScene("game ui.json"); err != nil {
		t.Fatal(err)
	}
	if err := a.Document.SelectScene("game ui.json"); err != nil {
		t.Fatal(err)
	}
	if err := a.Document.AddInstance("start", "custom module", ""); err != nil {
		t.Fatal(err)
	}
	a.Selected = "start"
	root, err := dom.Parse(workspaceShellTest(t, a))
	if err != nil {
		t.Fatal(err)
	}
	var visit func(*dom.Node)
	visit = func(node *dom.Node) {
		if id, ok := node.Attr("id"); ok && strings.IndexFunc(id, unicode.IsSpace) >= 0 {
			t.Errorf("HTML id contains whitespace: %q", id)
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	visit(root)
	id := workspaceFieldID("resources/scenes/game ui.json", "/modules/0/placement/position", "X")
	a.handleEvent(htmlui.Event{ID: id, Kind: htmlui.EventInput, Value: "99"})
	if err := a.applyDrafts(); err != nil {
		t.Fatal(err)
	}
	if a.Document.Instance("start").Placement.Position[0] != 99 {
		t.Fatal("encoded control ID lost document ownership")
	}
}

func TestWorkspaceEditorDiscardAndUndoRestoreLiveHTMLControlValues(t *testing.T) {
	a := workspaceEditorFixture(t)
	a.Selected = "start"
	world := ecs.NewWorld()
	context := &ecs.SystemContext{World: world}
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if err := htmlui.Sync(world); err != nil {
		t.Fatal(err)
	}
	id := workspaceFieldID("resources/scenes/main.json", "/modules/0/placement/position", "X")
	find := func() (ecs.Entity, htmlui.Control) {
		for entity, access := range ecs.Query[htmlui.Control](world).All {
			if access.Ref().Owner == a.shell && access.Ref().ID == id {
				return entity, access.Get()
			}
		}
		t.Fatal("live placement field missing")
		return 0, htmlui.Control{}
	}
	entity, _ := find()
	htmlui.SetControlValue(world, entity, "bad draft")
	a.handleEvent(htmlui.Event{ID: id, Kind: htmlui.EventInput, Value: "bad draft"})
	a.handleEvent(htmlui.Event{ID: "select:start", Kind: htmlui.EventClick})
	a.action("discard-pending")
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if err := htmlui.Sync(world); err != nil {
		t.Fatal(err)
	}
	_, control := find()
	if control.Value != "30" {
		t.Fatalf("discard kept dirty native value %q", control.Value)
	}
	entity, _ = find()
	htmlui.SetControlValue(world, entity, "90")
	a.handleEvent(htmlui.Event{ID: id, Kind: htmlui.EventInput, Value: "90"})
	a.action("save")
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if err := htmlui.Sync(world); err != nil {
		t.Fatal(err)
	}
	a.action("undo")
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if err := htmlui.Sync(world); err != nil {
		t.Fatal(err)
	}
	_, control = find()
	if control.Value != "30" {
		t.Fatalf("undo left stale native value %q", control.Value)
	}
}

func TestWorkspaceModuleResourcePickerAddsSelectedKind(t *testing.T) {
	a := workspaceEditorFixture(t)
	directory := filepath.Join(a.Document.ProjectDir, "button")
	for _, file := range []string{filepath.Join(directory, "fonts", "ui.ttf"), filepath.Join(directory, "icon.png"), filepath.Join(directory, "resources", "rig.json")} {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := writeWorkspaceFile(file, []byte("bytes")); err != nil {
			t.Fatal(err)
		}
	}
	a.enterModuleEdit("main.json", "start")
	a.contextScene, a.contextParent, a.contextMod = "main.json", "start", "button"
	source := workspaceShellTest(t, a)
	if strings.Contains(source, `id="definition-resources-title"`) || strings.Contains(source, `id="add-resource"`) || !strings.Contains(source, `id="tree-group:main.json/start/resources"`) {
		t.Fatal("resources were not moved from Details to the module tree")
	}
	picked := filepath.Join(directory, "fonts", "ui.ttf")
	var options window.FileDialogOptions
	a.openNativeFile = func(value window.FileDialogOptions) (string, error) { options = value; return picked, nil }
	a.action("outliner-add-resource:font")
	resource, ok := a.Document.Mods["button"].Spec.Resources["ui"]
	if !ok || resource.Kind != "font" || resource.Path != "fonts/ui.ttf" || a.status != "Added resource ui" {
		t.Fatalf("added %+v with status %q", resource, a.status)
	}
	if len(options.Filters) != 1 || options.Filters[0].Name != "Font" || options.DefaultPath != directory || a.selectedResource != "ui" {
		t.Fatalf("font picker options = %+v, selection = %q", options, a.selectedResource)
	}
	picked = filepath.Join(directory, "icon.png")
	a.action("outliner-add-resource:font")
	if !strings.Contains(a.status, "not a Font file") {
		t.Fatalf("wrong file type was accepted: %s", a.status)
	}
	a.action("outliner-add-resource:image")
	if resource := a.Document.Mods["button"].Spec.Resources["icon"]; resource.Kind != "image" || resource.Path != "icon.png" {
		t.Fatalf("picked icon as %+v", resource)
	}
	picked = filepath.Join(directory, "resources", "rig.json")
	a.action("outliner-add-resource:spine")
	if resource := a.Document.Mods["button"].Spec.Resources["rig"]; resource.Kind != "spine" || resource.Path != "resources/rig.json" {
		t.Fatalf("JSON resource lost its explicitly selected type: %+v", resource)
	}
	picked = filepath.Join(a.Document.ProjectDir, "resources", "project.json")
	a.action("outliner-add-resource:data")
	if !strings.Contains(a.status, "outside module") {
		t.Fatalf("outside module status = %q", a.status)
	}
	picked = ""
	revision := a.Document.Revision
	a.action("outliner-add-resource:data")
	if a.Document.Revision != revision {
		t.Fatal("cancelling the picker edited the module")
	}
}
