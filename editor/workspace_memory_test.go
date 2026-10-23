package editor

import (
	"bytes"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/project"
)

func TestWorkspaceHistoryReleasesDiscardedCommands(t *testing.T) {
	w := workspaceFixture(t)
	edit := func(name string) {
		t.Helper()
		if err := w.EditMod("button", name, func(mod *project.ModSpec) error { mod.Name = name; return nil }); err != nil {
			t.Fatal(err)
		}
	}
	edit("one")
	edit("two")
	for range 2 {
		if err := w.Undo(); err != nil {
			t.Fatal(err)
		}
	}
	for _, command := range w.undo[:cap(w.undo)] {
		if command.Label != "" || command.Edits != nil {
			t.Fatal("popped undo command retained its snapshots")
		}
	}
	if err := w.Redo(); err != nil {
		t.Fatal(err)
	}
	for _, command := range w.redo[len(w.redo):cap(w.redo)] {
		if command.Label != "" || command.Edits != nil {
			t.Fatal("popped redo command retained its snapshots")
		}
	}
	edit("new branch")
	for _, command := range w.redo[:cap(w.redo)] {
		if command.Label != "" || command.Edits != nil {
			t.Fatal("discarded redo branch retained its snapshots")
		}
	}
}

func TestWorkspaceImmutableSavedBaseline(t *testing.T) {
	w := workspaceFixture(t)
	state := w.states["button/mod.json"]
	if &state.data[0] != &state.saved[0] {
		t.Fatal("initial baseline duplicated immutable JSON")
	}
	before := bytes.Clone(state.saved)
	if err := w.EditMod("button", "Rename", func(mod *project.ModSpec) error { mod.Name = "Renamed"; return nil }); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(state.saved, before) || !w.Dirty() {
		t.Fatal("editing mutated the saved baseline")
	}
	if err := w.Save(); err != nil {
		t.Fatal(err)
	}
	if &state.data[0] != &state.saved[0] || w.Dirty() {
		t.Fatal("save duplicated the new baseline or kept it dirty")
	}
	saved := state.saved
	if err := w.Undo(); err != nil {
		t.Fatal(err)
	}
	if !w.Dirty() || !bytes.Equal(state.saved, saved) || w.Mods["button"].Spec.Name != "Button" {
		t.Fatal("undo did not preserve the saved version")
	}
}

func TestWorkspaceDraftSharesUnchangedModulePlansFromLoad(t *testing.T) {
	w := workspaceFixture(t)
	button := w.Mods["button"].Plan
	a := ModuleEditor{
		Document: w,
		fields:   map[string]workspaceField{"name": {Target: "empty/mod.json", Pointer: "/name", Kind: "string", Axis: -1}},
		drafts:   map[string]string{"name": "Draft"},
	}
	a.syncDraftPreview()
	if a.draftPreview.document == nil || &a.previewDocument().Mods["button"].Plan.Slots[0] != &button.Slots[0] {
		t.Fatal("first draft duplicated unchanged module plans")
	}
	clear(a.drafts)
	a.syncDraftPreview()
	plan, err := w.compile(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, mod := range plan.Mods {
		if mod.ID == "button" && &mod.Slots[0] != &button.Slots[0] {
			t.Fatal("cancelled draft left a second copy in the compilation cache")
		}
	}
}

func TestWorkspacePrivateFilesStayOnDiskAcrossCopyAndDelete(t *testing.T) {
	w := workspaceFixture(t)
	original := []byte("private copy snapshot")
	if err := writeWorkspaceFile(w.Absolute("button/private/binary.bin"), original); err != nil {
		t.Fatal(err)
	}
	if err := w.Refresh(); err != nil {
		t.Fatal(err)
	}
	if err := w.CopyMod("button", "independent"); err != nil {
		t.Fatal(err)
	}
	staging := w.stagingDir
	if err := writeWorkspaceFile(w.Absolute("button/private/binary.bin"), []byte("source changed")); err != nil {
		t.Fatal(err)
	}
	assertSnapshot := func() {
		t.Helper()
		data, err := fs.ReadFile(workspaceFS{w: w}, "independent/private/binary.bin")
		if err != nil || !bytes.Equal(data, original) {
			t.Fatalf("private snapshot changed: %q (%v)", data, err)
		}
	}
	assertSnapshot()
	if err := w.Save(); err != nil {
		t.Fatal(err)
	}
	if len(w.assets) != 0 {
		t.Fatal("saved copy kept unnecessary overlay entries")
	}
	if err := w.DeleteMod("independent"); err != nil {
		t.Fatal(err)
	}
	if mod := w.Mods["independent"]; mod.Plan != nil || mod.Spec.Resources != nil || mod.Spec.Components.Sprite != nil {
		t.Fatal("deleted document kept an old compiled plan")
	}
	if err := w.Save(); err != nil {
		t.Fatal(err)
	}
	if file := w.assets["independent/private/binary.bin"]; !strings.Contains(file, ".editor-trash") {
		t.Fatalf("deleted private file was not backed by editor trash: %s", file)
	}
	if err := w.Undo(); err != nil {
		t.Fatal(err)
	}
	assertSnapshot()
	if err := w.Save(); err != nil {
		t.Fatal(err)
	}
	for _, file := range w.Files {
		if strings.HasPrefix(file.Path, workspaceStagingDirectory) {
			t.Fatal("temporary snapshots appeared in the resource inventory")
		}
	}
	if err := w.Dispose(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(staging); !os.IsNotExist(err) {
		t.Fatalf("document disposal kept temporary copies: %v", err)
	}
	assertSnapshot()
}

func TestWorkspaceVNodeViewReleasesUnusedGroups(t *testing.T) {
	var view editorVNodeView
	view.reset()
	view.text("p", "one", "one")
	view.text("p", "two", strings.Repeat("x", 1<<20))
	view.nodes()
	view.reset()
	view.text("p", "one", "one")
	view.nodes()
	for _, group := range view.groups[len(view.groups):cap(view.groups)] {
		if group.node.Text != "" || group.children != nil || group.attrs != nil {
			t.Fatal("smaller view retained discarded groups")
		}
	}
	if allocations := testing.AllocsPerRun(100, func() {
		view.reset()
		view.text("p", "one", "one")
		view.nodes()
	}); allocations != 0 {
		t.Fatalf("unchanged view allocated %g", allocations)
	}
}

func TestWorkspace2DResourceCacheFollowsPreview(t *testing.T) {
	a := workspaceEditorFixture(t)
	context := &ecs.SystemContext{World: ecs.NewWorld()}
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	button := a.viewport2D.mods["button"].resources
	if button == nil || a.viewport2D.mods["empty"].resources == nil {
		t.Fatal("scene resources were not initialized")
	}
	a.Local = "button"
	a.changed()
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if len(a.viewport2D.mods) != 1 || a.viewport2D.mods["button"].resources != button {
		t.Fatal("module preview retained unused scene resources or discarded active resources")
	}
	a.Local = "empty"
	a.changed()
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if len(a.viewport2D.mods) != 1 || a.viewport2D.mods["empty"].resources == nil {
		t.Fatal("switching modules retained the previous resources")
	}
}

func TestWorkspace3DResourceCacheRetainsCameraOnlyModules(t *testing.T) {
	a, context := workspace3DEditorFixture(t)
	if err := a.Document.CreateMod("camera", project.ModSpec{
		Space:      project.CoordinateSpaceWorld,
		Components: project.ModComponents{Camera: &project.Camera3DSpec{Fov: .7, Near: .1, ClearColor: "#303030"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.Document.AddInstance("camera", "camera", ""); err != nil {
		t.Fatal(err)
	}
	a.settings.Viewport3D.EnvironmentCamera = "camera"
	a.changed()
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if _, id, err := a.viewport3DEnvironmentSource(); err != nil || id != "camera" {
		t.Fatalf("camera-only environment was not selected: %s (%v)", id, err)
	}
	camera := a.viewport3D.mods["camera"].resources
	if err := a.Document.EditInstance("camera", "Move camera", func(instance *project.ModInstanceSpec) error {
		instance.Placement.Position = []float32{1, 2, 3}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	a.changed()
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if a.viewport3D.mods["camera"].resources != camera {
		t.Fatal("camera-only module resources were evicted while still in the preview")
	}
	if err := a.Document.DeleteInstance("camera"); err != nil {
		t.Fatal(err)
	}
	a.changed()
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if _, exists := a.viewport3D.mods["camera"]; exists {
		t.Fatal("removed camera retained its environment resources")
	}
	a.Local = "empty"
	a.changed()
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if len(a.viewport3D.mods) != 0 {
		t.Fatal("empty local preview retained scene geometry resources")
	}
}

func TestWorkspaceInactiveViewportReleasesPreviousScene(t *testing.T) {
	a, context := workspace3DEditorFixture(t)
	a.action("toggle-dimension")
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if a.viewport2D.world == nil || a.viewport3D.world == nil {
		t.Fatal("unchanged dimension switch discarded a retained viewport")
	}
	a.Local = "button"
	a.changed()
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if a.viewport3D.world != nil || len(a.viewport3D.mods) != 0 {
		t.Fatal("inactive 3D viewport retained the previous scene")
	}
	a.action("toggle-dimension")
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	a.Local = "empty"
	a.changed()
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if a.viewport2D.world != nil || len(a.viewport2D.mods) != 0 {
		t.Fatal("inactive 2D viewport retained the previous scene")
	}
}
