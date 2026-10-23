package editor

import (
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	editorresources "github.com/diiyw/onix/editor/resources"
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/project"
	engineui "github.com/diiyw/onix/ui"
	"github.com/diiyw/onix/ui/html"
)

func TestWorkspaceComponentsRetainControlsAndFocusAcrossUpdates(t *testing.T) {
	a := workspaceEditorFixture(t)
	a.Selected = "start"
	world := ecs.NewWorld()
	syncWorkspaceTest(t, a, world)
	shell, _ := ecs.Get[html.Surface](world, a.shell)
	if shell.Value != editorresources.WorkspaceHTML {
		t.Fatal("workspace is not using its compiled template")
	}
	if _, ok := ecs.Get[html.Component[workspaceComponentProps, workspaceComponentState]](world, a.shellComponent); !ok {
		t.Fatal("workspace component missing")
	}
	fieldID := workspaceFieldID(modDocumentID("button"), "/name", "")
	field, ok := html.ControlByID(world, a.shell, fieldID)
	if !ok {
		t.Fatal("module name field missing")
	}
	row, _ := html.ControlByID(world, a.shell, "select:start")
	if !engineui.FocusControl(world, field) {
		t.Fatal("could not focus module name")
	}
	fieldState := ecs.GetMut[engineui.TextField](world, field)
	if fieldState == nil {
		t.Fatal("module name is not a text field")
	}
	fieldState.Caret, fieldState.Anchor = 2, 2
	a.status = "Changed status"
	a.changed()
	syncWorkspaceTest(t, a, world)
	if got, _ := html.ControlByID(world, a.shell, fieldID); got != field {
		t.Fatal("status update replaced field")
	}
	text, _ := ecs.Get[engineui.TextField](world, field)
	if !text.Focused || text.Caret != 2 {
		t.Fatalf("field lost focus or caret: %+v", text)
	}
	a.selectInstance("again") // Another instance of the same module keeps definition fields.
	syncWorkspaceTest(t, a, world)
	if got, _ := html.ControlByID(world, a.shell, fieldID); got != field {
		t.Fatal("instance selection replaced shared definition field")
	}
	if got, _ := html.ControlByID(world, a.shell, "select:start"); got != row {
		t.Fatal("selection replaced tree row")
	}
	text, _ = ecs.Get[engineui.TextField](world, field)
	if !text.Focused || text.Caret != 2 {
		t.Fatal("instance selection reset definition editing")
	}
	document := a.Document
	a.Document = nil
	a.changed()
	syncWorkspaceTest(t, a, world)
	if world.IsAlive(field) || len(a.fields) != 0 || len(a.rows) != 0 {
		t.Fatal("closed workspace retained project controls")
	}
	a.Document = document
	a.changed()
	syncWorkspaceTest(t, a, world)
	if _, ok := html.ControlByID(world, a.shell, fieldID); !ok {
		t.Fatal("reopening failed to mount inspector")
	}
}

func TestWorkspaceComponentTreeKeepsRowsWhenReordered(t *testing.T) {
	a := workspaceEditorFixture(t)
	world := ecs.NewWorld()
	syncWorkspaceTest(t, a, world)
	row, _ := html.ControlByID(world, a.shell, "select:start")
	if err := a.Document.MoveInstance("start", "", 2); err != nil {
		t.Fatal(err)
	}
	a.changed()
	syncWorkspaceTest(t, a, world)
	if got, _ := html.ControlByID(world, a.shell, "select:start"); got != row {
		t.Fatal("reorder replaced keyed row")
	}
	a.selectScene("main.json")
	a.Document.Scene().Spec.Modules = append(a.Document.Scene().Spec.Modules, project.ModInstanceSpec{ID: "extra", Mod: "empty"})
	a.changed()
	syncWorkspaceTest(t, a, world)
	if got, _ := html.ControlByID(world, a.shell, "select:start"); got != row {
		t.Fatal("insertion replaced keyed row")
	}
}

func TestTitlebarComponentReconcilesRecentProjectsByPath(t *testing.T) {
	a := workspaceEditorFixture(t)
	world := ecs.NewWorld()
	a.settings.RecentProjects = []string{"/tmp/first<&", "/tmp/second"}
	a.menuOpen, a.submenu = "file", "file.open-recent-project"
	syncWorkspaceTest(t, a, world)
	first, _ := html.ControlByID(world, a.titlebar.entity, "recent-0")
	second, _ := html.ControlByID(world, a.titlebar.entity, "recent-1")
	if first == 0 || second == 0 {
		t.Fatal("recent project controls missing")
	}
	a.settings.RecentProjects[0], a.settings.RecentProjects[1] = a.settings.RecentProjects[1], a.settings.RecentProjects[0]
	a.changed()
	syncWorkspaceTest(t, a, world)
	if got, _ := html.ControlByID(world, a.titlebar.entity, "recent-1"); got != first {
		t.Fatal("recent path reorder replaced first control")
	}
	if got, _ := html.ControlByID(world, a.titlebar.entity, "recent-0"); got != second {
		t.Fatal("recent path reorder replaced second control")
	}
	source, err := html.Serialize(world, a.titlebar.entity)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(source, "/tmp/first&lt;&amp;") {
		t.Fatal("recent path was not represented as plain text")
	}
	a.settings.RecentProjects = a.settings.RecentProjects[:1]
	a.changed()
	syncWorkspaceTest(t, a, world)
	if world.IsAlive(first) || !world.IsAlive(second) {
		t.Fatal("recent removal retained obsolete control or replaced surviving control")
	}
}

func TestEditorVNodeViewReusesStorage(t *testing.T) {
	var view editorVNodeView
	render := func() {
		view.reset()
		view.open("fieldset", "properties")
		view.input("name", "Name", "Onix")
		view.button("save", "Save")
		view.close()
	}
	render()
	if allocations := testing.AllocsPerRun(100, render); allocations != 0 {
		t.Fatalf("VNode storage allocated %g after warmup", allocations)
	}
}

func TestWorkspaceResourcesUseFlatIconTilesAndKeepSelection(t *testing.T) {
	a := workspaceEditorFixture(t)
	a.Selected = "start"
	source := workspaceShellTest(t, a)
	for _, value := range []string{`class="resource-tiles"`, `class="resource-tile"`, `src="onix-file-icons/prefab"`, `src="onix-file-icons/script"`, `src="onix-file-icons/folder"`, `>private</span>`} {
		if !strings.Contains(source, value) {
			t.Fatalf("resource tiles missing %s", value)
		}
	}
	if strings.Contains(source, "button/private/data.json") {
		t.Fatal("resource root displayed nested files")
	}
	fileID := "file:" + url.PathEscape("button/mod.go")
	var find func(*dom.Node) *dom.Node
	find = func(node *dom.Node) *dom.Node {
		if id, _ := node.Attr("id"); id == fileID {
			return node
		}
		for _, child := range node.Children {
			if found := find(child); found != nil {
				return found
			}
		}
		return nil
	}
	root, err := dom.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	file := find(root)
	if file == nil {
		t.Fatal("file tile missing")
	}
	role, _ := file.Attr("role")
	_, level := file.Attr("aria-level")
	_, style := file.Attr("style")
	if role == "treeitem" || level || style {
		t.Fatal("file tile retained tree semantics or indentation")
	}
	retainedRoot, err := html.Root(a.world, a.shell)
	if err != nil {
		t.Fatal(err)
	}
	control, exists := retainedRoot.FindByID(fileID)
	if !exists {
		t.Fatal("file tile DOM node missing")
	}
	a.handleEvent(html.Event{ID: fileID, Kind: html.EventClick})
	root, err = dom.Parse(workspaceShellTest(t, a))
	if err != nil {
		t.Fatal(err)
	}
	pressed, _ := find(root).Attr("aria-pressed")
	if pressed != "true" || a.lastFile != "button/mod.go" {
		t.Fatal("file click did not select the tile")
	}
	if got, _ := retainedRoot.FindByID(fileID); got != control {
		t.Fatal("selection replaced the file tile control")
	}
	for _, provider := range []html.ImageProvider{a, editorHTMLImages{}} {
		texture, ok := provider.ResolveHTMLImage("onix-file-icons/json")
		if !ok || texture == nil || texture != editorresources.FileIconTexture("json") {
			t.Fatal("editor image provider did not resolve the atlas frame")
		}
	}
}

func TestWorkspaceResourceCacheRefreshesWithoutIdleAllocations(t *testing.T) {
	document := workspaceFixture(t)
	var state workspaceFilesState
	state.refresh(document, "button", "button/private")
	if allocations := testing.AllocsPerRun(100, func() { state.refresh(document, "button", "button/private") }); allocations != 0 {
		t.Fatalf("unchanged file inventory allocated %g", allocations)
	}
	if err := writeWorkspaceFile(document.Absolute("button/private/new.png"), []byte("image")); err != nil {
		t.Fatal(err)
	}
	if err := document.RefreshFiles(); err != nil {
		t.Fatal(err)
	}
	state.refresh(document, "button", "button/private")
	found := false
	for _, tile := range state.Tiles {
		if tile.File.Path == "button/private/new.png" {
			found = tile.Icon == "onix-file-icons/image"
		}
	}
	if !found {
		t.Fatal("refreshed inventory did not update cached file tiles")
	}
	if err := document.CopyMod("button", "copy"); err != nil {
		t.Fatal(err)
	}
	state.refresh(document, "copy", "copy/private")
	found = false
	for _, tile := range state.Tiles {
		if tile.File.Path == "copy/private/data.json" {
			found = true
		}
	}
	if !found {
		t.Fatal("copied module lost unsaved files in cached tiles")
	}
}

func TestWorkspaceResourceFolderNavigation(t *testing.T) {
	a := workspaceEditorFixture(t)
	if err := os.MkdirAll(a.Document.Absolute("button/private/empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := a.Document.RefreshFiles(); err != nil {
		t.Fatal(err)
	}
	click := func(filename string) {
		t.Helper()
		a.time += .1
		a.handleEvent(html.Event{ID: "file:" + url.PathEscape(filename), Kind: html.EventClick})
	}
	assertFile := func(filename string, visible bool) {
		t.Helper()
		workspaceShellTest(t, a)
		root, err := html.Root(a.world, a.shell)
		if err != nil {
			t.Fatal(err)
		}
		_, exists := root.FindByID("file:" + url.PathEscape(filename))
		if exists != visible {
			t.Fatalf("file %s visible=%t, want %t", filename, exists, visible)
		}
	}
	assertFile("button", true)
	assertFile("button/mod.go", false)
	click("button")
	assertFile("button/mod.go", false)
	click("button")
	assertFile("button/mod.go", true)
	assertFile("empty/mod.json", false)
	assertFile("button/private/data.json", false)
	click("button/private")
	click("button/private")
	assertFile("button/private/data.json", true)
	assertFile("button/mod.go", false)
	if source := workspaceShellTest(t, a); !strings.Contains(source, `>data.json</span>`) || strings.Contains(source, `>private/data.json</span>`) {
		t.Fatal("nested files did not display their basename")
	}
	click("button/private/empty")
	click("button/private/empty")
	if source := workspaceShellTest(t, a); len(a.files) != 0 || !strings.Contains(source, "This folder is empty.") {
		t.Fatal("empty directory did not render correctly")
	}
	a.handleEvent(html.Event{ID: "resources-up", Kind: html.EventClick})
	assertFile("button/private/data.json", true)
	a.handleEvent(html.Event{ID: "resources-up", Kind: html.EventClick})
	assertFile("button/mod.go", true)
	a.handleEvent(html.Event{ID: "resources-up", Kind: html.EventClick})
	assertFile("button", true)
	a.handleEvent(html.Event{ID: "resources-up", Kind: html.EventClick})
	if a.resourceDirectory() != "" {
		t.Fatal("parent navigation escaped project root")
	}
	a.Selected = "start"
	assertFile("button/mod.go", true)
	click("button/private")
	click("button/private")
	assertFile("button/private/data.json", true)
	a.resourceParentDirectory()
	a.resourceParentDirectory()
	assertFile("button/mod.go", true)
	if a.resourceDirectory() != "button" {
		t.Fatal("parent navigation escaped selected module root")
	}
	a.Selected = "other"
	assertFile("empty/mod.json", true)
	assertFile("button/private/data.json", false)
	click("empty/mod.json")
	click("empty/mod.json")
	if a.Local != "empty" {
		t.Fatal("directory navigation broke module preview")
	}
	previous := a.Document
	a.Document = workspaceFixture(t)
	a.Local, a.Selected = "", ""
	assertFile("button", true)
	if a.fileBrowser.Document == previous || a.resourceDirectory() != "" {
		t.Fatal("project switch retained the previous directory")
	}
}

func TestWorkspaceFileContextMenuDeletesFilesAndCreatesFolders(t *testing.T) {
	a := workspaceEditorFixture(t)
	if err := writeWorkspaceFile(a.Document.Absolute("button/extra.txt"), []byte("extra")); err != nil {
		t.Fatal(err)
	}
	if err := a.Document.EditMod("button", "Declare root file", func(mod *project.ModSpec) error {
		mod.Resources = map[string]project.ModResourceSpec{"root": {Kind: "data", Path: "root.txt"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.Document.RefreshFiles(); err != nil {
		t.Fatal(err)
	}
	a.Selected = "start"
	world := ecs.NewWorld()
	syncWorkspaceTest(t, a, world)
	point := func(id string) (float64, float64) {
		t.Helper()
		node, ok := html.NodeByID(world, a.shell, id)
		if !ok {
			t.Fatalf("missing control %s", id)
		}
		box, ok := node.Bounds()
		if !ok {
			t.Fatalf("missing bounds for %s", id)
		}
		return float64(box.X + 8), float64(box.Y + box.Height/2)
	}
	rightClick := func(id string) {
		t.Helper()
		x, y := point(id)
		in := input.NewState()
		in.PressMouse(input.MouseRight, x, y)
		a.updateOutlinerMenu(in)
		if !a.fileMenuOpen {
			t.Fatalf("right click on %s did not open the file menu", id)
		}
	}
	rightClick("file:" + url.PathEscape("button/extra.txt"))
	if a.contextFile != "button/extra.txt" || a.lastFile != "button/extra.txt" {
		t.Fatalf("right click did not select the tile: %q %q", a.contextFile, a.lastFile)
	}
	if source := workspaceShellTest(t, a); !strings.Contains(source, `id="file-menu-delete"`) {
		t.Fatal("file menu is missing the delete item")
	}
	a.handleEvent(html.Event{ID: "file-menu-new-folder", Kind: html.EventClick})
	if a.dialog != "folder-create" || a.fileMenuOpen {
		t.Fatal("new folder did not open the dialog")
	}
	syncWorkspaceTest(t, a, world)
	workspaceDialogTestEvent(t, a, world, "dialog-field-folder-create-folder-name", "art", html.EventInput)
	workspaceDialogTestEvent(t, a, world, "folder-create-confirm", "", html.EventClick)
	if a.dialog != "" || a.status != "Created folder art" {
		t.Fatalf("folder dialog confirm failed: dialog=%q status=%q", a.dialog, a.status)
	}
	if entry, err := os.Stat(a.Document.Absolute("button/art")); err != nil || !entry.IsDir() {
		t.Fatalf("folder was not created on disk: %v", err)
	}
	if source := workspaceShellTest(t, a); !strings.Contains(source, ">art</span>") {
		t.Fatal("created folder is missing from the tiles")
	}
	rightClick("file:" + url.PathEscape("button/root.txt"))
	a.handleEvent(html.Event{ID: "file-menu-delete", Kind: html.EventClick})
	if _, err := os.Lstat(a.Document.Absolute("button/root.txt")); !os.IsNotExist(err) {
		t.Fatalf("declared file survived delete: %v", err)
	}
	if _, declared := a.Document.Mods["button"].Spec.Resources["root"]; declared {
		t.Fatal("delete left the resource declaration behind")
	}
	trash, err := os.ReadDir(a.Document.Absolute("resources/.editor-trash"))
	if err != nil || len(trash) != 1 || !strings.HasPrefix(trash[0].Name(), "root.txt-") {
		t.Fatalf("deleted file did not reach editor trash: %v (%v)", trash, err)
	}
	if source := workspaceShellTest(t, a); strings.Contains(source, ">root.txt</span>") {
		t.Fatal("deleted file is still displayed")
	}
	a.Selected = ""
	if err := a.Document.CreateMod("standalone", project.ModSpec{Name: "standalone"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Document.Save(); err != nil {
		t.Fatal(err)
	}
	if source := workspaceShellTest(t, a); !strings.Contains(source, ">standalone</span>") {
		t.Fatal("module folder is missing from the tiles")
	}
	syncWorkspaceTest(t, a, world)
	rightClick("file:standalone")
	a.handleEvent(html.Event{ID: "file-menu-delete", Kind: html.EventClick})
	if mod := a.Document.Mods["standalone"]; mod == nil || !mod.Deleted {
		t.Fatal("module folder was not deleted through the document")
	}
	if _, err := os.Lstat(a.Document.Absolute("standalone/mod.json")); err != nil {
		t.Fatalf("module delete removed disk files before save: %v", err)
	}
	if source := workspaceShellTest(t, a); strings.Contains(source, ">standalone</span>") {
		t.Fatal("deleted module folder is still displayed")
	}
}
