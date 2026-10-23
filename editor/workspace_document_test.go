package editor

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/diiyw/onix/project"
)

func workspaceFixture(t testing.TB) *WorkspaceDocument {
	t.Helper()
	dir := t.TempDir()
	if _, err := project.CreateProjectDirectory(dir, project.CreateProjectOptions{Width: 750, Height: 1624}); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"button/mod.json":            `{"name":"Button","transform":{"position":[5,7]},"components":{"sprite":{"color":"#3264d9","size":[120,40]}},"slots":{"text":{"type":"string","default":"Hello"},"label":{"type":"visual","writable":false}}}`,
		"button/mod.go":              "package button\n",
		"button/private/data.json":   `{"private":true}`,
		"button/root.txt":            "root resource",
		"empty/mod.json":             `{}`,
		"resources/scenes/main.json": `{"transform":{"position":[10,20]},"modules":[{"id":"start","mod":"button","placement":{"position":[30,40]}},{"id":"again","mod":"button","placement":{"position":[200,100]}},{"id":"other","mod":"empty"}]}`,
	}
	for name, data := range files {
		if err := writeWorkspaceFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	w, err := LoadWorkspaceDocument(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Dispose() })
	return w
}

func readWorkspaceTestFile(t *testing.T, w *WorkspaceDocument, file string) []byte {
	t.Helper()
	data, err := os.ReadFile(w.Absolute(file))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestWorkspaceDocumentOwningFilesUndoAndReload(t *testing.T) {
	w := workspaceFixture(t)
	modDisk := readWorkspaceTestFile(t, w, "button/mod.json")
	settings := readWorkspaceTestFile(t, w, "resources/project.json")
	entry := readWorkspaceTestFile(t, w, "main.go")
	modRevision := w.Mods["button"].Revision
	if err := w.EditInstance("start", "Place start", func(value *project.ModInstanceSpec) error { value.Placement.Position = []float32{80, 90}; return nil }); err != nil {
		t.Fatal(err)
	}
	if !w.Scene().Dirty || w.Mods["button"].Dirty || w.Mods["button"].Revision != modRevision {
		t.Fatal("placement changed the definition document")
	}
	if len(w.undo) != 1 || w.undo[0].Edits[0].Target != "resources/scenes/main.json" {
		t.Fatal("undo lost scene ownership")
	}
	if err := w.Save(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(modDisk, readWorkspaceTestFile(t, w, "button/mod.json")) || !bytes.Equal(settings, readWorkspaceTestFile(t, w, "resources/project.json")) || !bytes.Equal(entry, readWorkspaceTestFile(t, w, "main.go")) {
		t.Fatal("scene save rewrote an unrelated file")
	}
	sceneDisk := readWorkspaceTestFile(t, w, "resources/scenes/main.json")
	sceneRevision := w.Scene().Revision
	if err := w.EditMod("button", "Local transform", func(value *project.ModSpec) error {
		value.Transform.Position = []float32{2, 3}
		value.Slots["text"] = project.SlotSpec{Type: project.StringValue, Default: json.RawMessage(`"Updated"`)}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if w.Scene().Dirty || w.Scene().Revision != sceneRevision || !w.Mods["button"].Dirty {
		t.Fatal("definition changed scene ownership")
	}
	if w.undo[len(w.undo)-1].Edits[0].Target != "button/mod.json" {
		t.Fatal("undo lost module ownership")
	}
	if err := w.Undo(); err != nil {
		t.Fatal(err)
	}
	if w.Dirty() || w.Instance("start").Placement.Position[0] != 80 || w.Mods["button"].Spec.Transform.Position[0] != 5 {
		t.Fatal("definition undo undid scene placement or dirty baseline")
	}
	if err := w.Redo(); err != nil {
		t.Fatal(err)
	}
	if err := w.Save(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sceneDisk, readWorkspaceTestFile(t, w, "resources/scenes/main.json")) {
		t.Fatal("definition save rewrote scene")
	}
	reloaded, err := LoadWorkspaceDocument(w.ProjectDir)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Active != "main.json" || reloaded.Instance("start").Placement.Position[0] != 80 || reloaded.Mods["button"].Spec.Transform.Position[0] != 2 || reloaded.Instance("again").Mod != "button" {
		t.Fatal("save/reload lost authored values or reused definitions")
	}
	if err := w.Undo(); err != nil {
		t.Fatal(err)
	}
	if !w.Mods["button"].Dirty || w.Scene().Dirty {
		t.Fatal("undo after save did not use owning baseline")
	}
}

func TestWorkspaceDocumentDeleteDocumentGuardsEntryAndTrashesScenes(t *testing.T) {
	w := workspaceFixture(t)
	if err := w.DeleteDocument("resources/scenes/main.json"); err == nil {
		t.Fatal("entry scene survived deletion")
	}
	if err := w.CreateScene("extra.json"); err != nil {
		t.Fatal(err)
	}
	if err := w.Save(); err != nil {
		t.Fatal(err)
	}
	if err := w.DeleteDocument("resources/scenes/extra.json"); err != nil {
		t.Fatal(err)
	}
	if w.Scenes["extra.json"] != nil || slices.Contains(w.Spec.Scenes, project.SceneID("extra.json")) {
		t.Fatal("deleted scene stayed registered or listed in project settings")
	}
	for _, file := range w.ModuleFiles("") {
		if file.Path == "resources/scenes/extra.json" {
			t.Fatal("deleted scene is still listed before saving")
		}
	}
	if err := w.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(w.Absolute("resources/scenes/extra.json")); !os.IsNotExist(err) {
		t.Fatalf("save did not move the scene to editor trash: %v", err)
	}
	reloaded, err := LoadWorkspaceDocument(w.ProjectDir)
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.Dispose()
	if len(reloaded.Spec.Scenes) != 1 || reloaded.Scenes["extra.json"] != nil {
		t.Fatalf("reload saw the deleted scene: %v", reloaded.Spec.Scenes)
	}
}

func TestWorkspaceHierarchyAndModuleCopies(t *testing.T) {
	w := workspaceFixture(t)
	if err := w.DuplicateInstance("start", "copy"); err != nil {
		t.Fatal(err)
	}
	if w.Instance("copy").Mod != "button" || w.Instance("start").Mod != "button" {
		t.Fatal("duplicate did not reuse definition")
	}
	if err := w.MoveInstance("copy", "start", 0); err != nil {
		t.Fatal(err)
	}
	if len(w.Instance("start").Children) != 1 || w.Instance("start").Children[0].ID != "copy" {
		t.Fatal("reparent lost child ordering")
	}
	revision := w.Revision
	if err := w.MoveInstance("start", "copy", 0); err == nil {
		t.Fatal("accepted hierarchy cycle")
	}
	if w.Revision != revision || w.Instance("copy") == nil {
		t.Fatal("failed move mutated document")
	}
	if err := w.CreateScene("game.json"); err != nil {
		t.Fatal(err)
	}
	if err := w.SelectScene("game.json"); err != nil {
		t.Fatal(err)
	}
	if err := w.AddInstance("gameButton", "button", ""); err != nil {
		t.Fatal(err)
	}
	if err := w.CopyMod("button", "buttonCopy"); err != nil {
		t.Fatal(err)
	}
	if err := w.EditMod("buttonCopy", "Independent copy", func(mod *project.ModSpec) error { mod.Name = "Independent"; return nil }); err != nil {
		t.Fatal(err)
	}
	if w.Mods["button"].Spec.Name != "Button" {
		t.Fatal("copy shares mutable definition")
	}
	if err := w.Save(); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"root.txt", "mod.go", "private/data.json"} {
		if !bytes.Equal(readWorkspaceTestFile(t, w, "button/"+file), readWorkspaceTestFile(t, w, "buttonCopy/"+file)) {
			t.Fatalf("lost private layout %s", file)
		}
	}
	copyData := readWorkspaceTestFile(t, w, "buttonCopy/root.txt")
	if err := writeWorkspaceFile(w.Absolute("buttonCopy/root.txt"), []byte("external update")); err != nil {
		t.Fatal(err)
	}
	if err := w.Save(); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(copyData, readWorkspaceTestFile(t, w, "buttonCopy/root.txt")) {
		t.Fatal("unchanged Save overwrote a copied private resource")
	}
}

func TestWorkspaceDefinitionDeletionSaveAndUndoPreservePrivateFiles(t *testing.T) {
	w := workspaceFixture(t)
	if err := w.CopyMod("button", "unreferenced"); err != nil {
		t.Fatal(err)
	}
	if err := w.Save(); err != nil {
		t.Fatal(err)
	}
	if err := w.DeleteMod("unreferenced"); err != nil {
		t.Fatal(err)
	}
	if err := w.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(w.Absolute("unreferenced")); !os.IsNotExist(err) {
		t.Fatalf("deleted module still at root: %v", err)
	}
	if err := w.Undo(); err != nil {
		t.Fatal(err)
	}
	if err := w.Save(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"mod.json", "mod.go", "root.txt", "private/data.json"} {
		_ = readWorkspaceTestFile(t, w, "unreferenced/"+name)
	}
	reloaded, err := LoadWorkspaceDocument(w.ProjectDir)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Mods["unreferenced"] == nil {
		t.Fatal("undo after saved deletion lost module")
	}
}

func TestWorkspaceValidatedAtomicPatchesAndExternalConflict(t *testing.T) {
	w := workspaceFixture(t)
	revision := w.Revision
	if err := w.PatchDocuments("Bad override", map[DocumentID][]DocumentPatch{"resources/scenes/main.json": {{Pointer: "/modules/0/slots/missing", Value: json.RawMessage(`"x"`)}}}); err == nil || !strings.Contains(err.Error(), "main.json") {
		t.Fatalf("expected located unknown-slot error, got %v", err)
	}
	if w.Revision != revision || w.Dirty() || len(w.undo) != 0 {
		t.Fatal("failed validation changed history or document")
	}
	if err := w.PatchDocuments("Update slot and override", map[DocumentID][]DocumentPatch{
		"button/mod.json":            {{Pointer: "/slots/text/type", Value: json.RawMessage(`"number"`)}, {Pointer: "/slots/text/default", Value: json.RawMessage(`1`)}},
		"resources/scenes/main.json": {{Pointer: "/modules/0/slots/text", Value: json.RawMessage(`2`)}},
	}); err != nil {
		t.Fatal(err)
	}
	if len(w.undo) != 1 || len(w.undo[0].Edits) != 2 {
		t.Fatal("multi-document edit was not atomic")
	}
	if err := w.Undo(); err != nil {
		t.Fatal(err)
	}
	if w.Dirty() {
		t.Fatal("atomic undo did not restore both baselines")
	}
	if err := w.EditInstance("start", "Move", func(value *project.ModInstanceSpec) error { value.Placement.Position = []float32{9, 9}; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := writeWorkspaceFile(w.Absolute("resources/scenes/main.json"), []byte(`{"modules":[]}`)); err != nil {
		t.Fatal(err)
	}
	if err := w.Save(); err == nil || !strings.Contains(err.Error(), "changed outside") {
		t.Fatalf("expected external-edit conflict, got %v", err)
	}
	if string(readWorkspaceTestFile(t, w, "resources/scenes/main.json")) != `{"modules":[]}` {
		t.Fatal("conflict overwrote external change")
	}
}

func TestWorkspaceFilesRootResourcesAndNoLegacyFallback(t *testing.T) {
	w := workspaceFixture(t)
	files := w.ModuleFiles("button")
	for _, name := range []string{"button/mod.json", "button/mod.go", "button/root.txt", "button/private/data.json"} {
		if !slices.ContainsFunc(files, func(file WorkspaceFile) bool { return file.Path == name }) {
			t.Fatalf("file browser lost %s", name)
		}
	}
	if err := writeWorkspaceFile(w.Absolute("resources/project.json"), []byte(`{"scenes":["main.scene.json"]}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWorkspaceDocument(w.ProjectDir); err == nil {
		t.Fatal("loaded legacy manifest")
	}
}

func TestWorkspaceCreatedSceneUndoAfterSaveAndFileRefresh(t *testing.T) {
	w := workspaceFixture(t)
	if err := w.CreateScene("game.json"); err != nil {
		t.Fatal(err)
	}
	if err := w.Save(); err != nil {
		t.Fatal(err)
	}
	if err := w.Undo(); err != nil {
		t.Fatal(err)
	}
	if err := w.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(w.Absolute("resources/scenes/game.json")); !os.IsNotExist(err) {
		t.Fatalf("undo left a created scene file: %v", err)
	}
	if err := w.Redo(); err != nil {
		t.Fatal(err)
	}
	if err := w.Save(); err != nil {
		t.Fatal(err)
	}
	scene, mod := w.Scene(), w.Mods["button"]
	if err := writeWorkspaceFile(w.Absolute("newModule/mod.json"), []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err := w.Refresh(); err != nil {
		t.Fatal(err)
	}
	if w.Scene() != scene || w.Mods["button"] != mod || w.Mods["newModule"] == nil {
		t.Fatal("refresh replaced existing documents or missed new definition")
	}
	if err := w.AddInstance("fresh", "newModule", ""); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceNestedDefinitionLifecycleAndDirectoryCopy(t *testing.T) {
	w := workspaceFixture(t)
	if err := writeWorkspaceFile(w.Absolute("button/z/mod.json"), []byte(`{"resources":{"data":{"kind":"data","path":"data.txt"}}}`)); err != nil {
		t.Fatal(err)
	}
	if err := writeWorkspaceFile(w.Absolute("button/z/data.txt"), []byte("private child")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(w.Absolute("button/empty-directory"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := w.EditScene(w.Active, "Use nested module", func(spec *project.SceneSpec) error {
		spec.Modules = append(spec.Modules, project.ModInstanceSpec{ID: "nested", Mod: "button/z"})
		return nil
	}); err == nil {
		t.Fatal("editing should request refresh for new definitions")
	}
	if err := w.Save(); err != nil {
		t.Fatal(err)
	}
	// Register the nested definition by loading an actual scene reference.
	if err := writeWorkspaceFile(w.Absolute("resources/scenes/main.json"), []byte(`{"modules":[{"id":"nested","mod":"button/z"}]}`)); err != nil {
		t.Fatal(err)
	}
	var err error
	w, err = LoadWorkspaceDocument(w.ProjectDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.DeleteMod("button"); err == nil || !strings.Contains(err.Error(), "main.json: modules[0].mod") {
		t.Fatalf("parent deletion did not report the referencing scene field: %v", err)
	}
	if err := w.DeleteInstance("nested"); err != nil {
		t.Fatal(err)
	}
	if w.Mods["button/z"].Deleted {
		t.Fatal("unreferenced nested definition was discarded")
	}
	if err := w.EditMod("button/z", "Invalid unused definition", func(spec *project.ModSpec) error { spec.Space = "invalid"; return nil }); err == nil {
		t.Fatal("unreferenced definition skipped validation")
	}
	if err := w.CopyMod("button", "independent"); err != nil {
		t.Fatal(err)
	}
	if err := w.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(w.Absolute("independent/empty-directory")); err != nil {
		t.Fatal("copy lost an empty private folder")
	}
	if err := writeWorkspaceFile(w.Absolute("button/new-after-open.txt"), []byte("added outside editor")); err != nil {
		t.Fatal(err)
	}
	if err := w.DeleteMod("button"); err != nil {
		t.Fatal(err)
	}
	if err := w.Save(); err != nil {
		t.Fatal(err)
	}
	if err := w.Undo(); err != nil {
		t.Fatal(err)
	}
	if err := w.Save(); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"button/z/data.txt", "button/new-after-open.txt"} {
		_ = readWorkspaceTestFile(t, w, file)
	}
	if _, err := os.Stat(w.Absolute("button/empty-directory")); err != nil {
		t.Fatal("saved deletion undo lost empty directory")
	}
}

func TestWorkspaceMessageModulesNeedNoRoutes(t *testing.T) {
	w := workspaceFixture(t)
	storage := t.TempDir()
	for _, id := range []project.ModID{"service", "client"} {
		name := filepath.Join(storage, string(id)+".go")
		if err := os.WriteFile(name, []byte("package "+id+"\ntype Mod struct{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		w.assets[string(id)+"/mod.go"] = name
		if err := w.CreateMod(id, project.ModSpec{Behavior: "Mod"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.AddInstance("font", "service", ""); err != nil {
		t.Fatal(err)
	}
	if err := w.AddInstance("client", "client", ""); err != nil {
		t.Fatal(err)
	}
	if err := w.DuplicateInstance("client", "client2"); err != nil {
		t.Fatal(err)
	}
	if err := w.DeleteInstance("font"); err != nil {
		t.Fatal("receiver requires no authored connections", err)
	}
	if w.Instance("font") != nil || w.Instance("client") == nil || w.Instance("client2") == nil {
		t.Fatal("instance edit changed unrelated modules")
	}
	if err := w.Undo(); err != nil {
		t.Fatal(err)
	}
	if w.Instance("font") == nil {
		t.Fatal("undo did not restore receiver")
	}
}
