package editor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	editorresources "github.com/diiyw/onix/editor/resources"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	htmlui "github.com/diiyw/onix/ui/html"
	"github.com/diiyw/onix/window"
)

// titlebarButtonTag returns the serialized markup of one titlebar button so
// tests can assert on dynamically bound text and attributes.
func titlebarButtonTag(t *testing.T, world *ecs.World, entity ecs.Entity, id string) string {
	t.Helper()
	source, err := htmlui.Serialize(world, entity)
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(source, `id="`+id+`"`)
	if start < 0 {
		t.Fatalf("missing %q in serialized titlebar", id)
	}
	start = strings.LastIndex(source[:start], "<button")
	if start < 0 {
		t.Fatalf("missing button opening tag for %q", id)
	}
	end := strings.Index(source[start:], "</button>")
	if end < 0 {
		t.Fatalf("malformed button markup for %q", id)
	}
	return source[start : start+end+len("</button>")]
}

func syncWorkspaceTest(t *testing.T, a *ModuleEditor, world *ecs.World) {
	t.Helper()
	if err := a.sync(&ecs.SystemContext{World: world}); err != nil {
		t.Fatal(err)
	}
	if err := htmlui.Sync(world); err != nil {
		t.Fatal(err)
	}
	if err := a.restoreOutlinerScroll(&ecs.SystemContext{World: world}); err != nil {
		t.Fatal(err)
	}
}

func workspaceDialogTestEvent(t *testing.T, a *ModuleEditor, world *ecs.World, id, value string, kind htmlui.EventKind) {
	t.Helper()
	name := a.activeWorkspaceDialog()
	world.WriteMessage(htmlui.Event{Entity: a.dialogUI.htmlDialogs[name], ID: id, Value: value, Checked: value == "true", Kind: kind})
	if err := a.update(&ecs.SystemContext{World: world}); err != nil {
		t.Fatal(err)
	}
	syncWorkspaceTest(t, a, world)
}

func TestWorkspaceTitlebarMenusKeepTheirWidthsWhenOpened(t *testing.T) {
	a := workspaceEditorFixture(t)
	world := ecs.NewWorld()
	var widths [len(titlebarMenuDefinitions)]float32
	for _, open := range []string{"", "file", "edit", "debug", "about", ""} {
		a.menuOpen = open
		a.changed()
		syncWorkspaceTest(t, a, world)
		previousRight := float32(0)
		for i, menu := range titlebarMenuDefinitions {
			host, ok := htmlui.NodeByID(world, a.titlebar.entity, "menu-host-"+menu.key)
			if !ok {
				t.Fatal("missing menu host", menu.key)
			}
			button, ok := htmlui.NodeByID(world, a.titlebar.entity, "menu-"+menu.key)
			if !ok {
				t.Fatal("missing menu button", menu.key)
			}
			hostBounds, _ := host.Bounds()
			buttonBounds, _ := button.Bounds()
			if hostBounds.Width <= 16 || abs(hostBounds.Width-buttonBounds.Width) > .001 || abs(hostBounds.X-buttonBounds.X) > .001 {
				t.Fatalf("menu %s compressed with %q open: host=%+v button=%+v", menu.key, open, hostBounds, buttonBounds)
			}
			if i > 0 && abs(hostBounds.X-previousRight-4) > .001 {
				t.Fatalf("menu %s lost its gap with %q open", menu.key, open)
			}
			if widths[i] != 0 && abs(hostBounds.Width-widths[i]) > .001 {
				t.Fatalf("menu %s width changed with %q open", menu.key, open)
			}
			widths[i] = hostBounds.Width
			previousRight = hostBounds.X + hostBounds.Width
		}
	}
}

func TestWorkspaceTitlebarRestoresAllAuthoredMenusAndRecentProjects(t *testing.T) {
	a := workspaceEditorFixture(t)
	world := ecs.NewWorld()
	syncWorkspaceTest(t, a, world)
	surface, _ := world.Get[htmlui.Surface](a.titlebar.entity)
	if surface.Value != editorresources.TitlebarHTML {
		t.Fatal("titlebar did not reuse the original template")
	}
	for _, menu := range titlebarMenuDefinitions {
		if _, ok := htmlui.NodeByID(world, a.titlebar.entity, "menu-"+menu.key); !ok {
			t.Fatal("missing menu", menu.key)
		}
		for _, id := range menu.actions {
			if _, ok := htmlui.NodeByID(world, a.titlebar.entity, id); !ok {
				t.Fatal("missing action", id)
			}
		}
	}
	for _, id := range []string{"top-run", "top-save", "top-reload", "top-frame", "top-dimension", "recent-0"} {
		if _, ok := htmlui.NodeByID(world, a.titlebar.entity, id); !ok {
			t.Fatal("missing toolbar/recent action", id)
		}
	}
	if len(a.settings.RecentProjects) != 1 || a.settings.RecentProjects[0] != a.Document.ProjectDir {
		t.Fatal("opened project not remembered")
	}
	if tag := titlebarButtonTag(t, world, a.titlebar.entity, "top-dimension"); strings.Contains(tag, "disabled") || !strings.Contains(tag, ">2D</button>") || !strings.Contains(tag, "Switch to 3D view") {
		t.Fatalf("dimension toggle did not render the 2D state: %s", tag)
	}
	world.WriteMessage(htmlui.Event{Entity: a.titlebar.entity, ID: "top-dimension", Kind: htmlui.EventClick})
	if err := a.update(&ecs.SystemContext{World: world}); err != nil {
		t.Fatal(err)
	}
	syncWorkspaceTest(t, a, world)
	if a.dimension != previewDimension3D {
		t.Fatal("dimension toggle did not switch the canvas to 3D")
	}
	if tag := titlebarButtonTag(t, world, a.titlebar.entity, "top-dimension"); strings.Contains(tag, "disabled") || !strings.Contains(tag, ">3D</button>") || !strings.Contains(tag, "Switch to 2D view") {
		t.Fatalf("dimension toggle did not render the 3D state: %s", tag)
	}
	world.WriteMessage(htmlui.Event{Entity: a.titlebar.entity, ID: "top-dimension", Kind: htmlui.EventClick})
	if err := a.update(&ecs.SystemContext{World: world}); err != nil {
		t.Fatal(err)
	}
	syncWorkspaceTest(t, a, world)
	if a.dimension != previewDimension2D {
		t.Fatal("dimension toggle did not switch the canvas back to 2D")
	}
	world.WriteMessage(htmlui.Event{Entity: a.titlebar.entity, ID: "about.show", Kind: htmlui.EventClick})
	if err := a.update(&ecs.SystemContext{World: world}); err != nil {
		t.Fatal(err)
	}
	syncWorkspaceTest(t, a, world)
	if a.dialog != "about" {
		t.Fatal("titlebar events were ignored")
	}
	dialog, _ := world.Get[htmlui.Surface](a.dialogUI.htmlDialogs["about"])
	if dialog.Value != editorresources.DialogHTML("about") {
		t.Fatal("about did not use dialog resources")
	}
	about, err := htmlui.Serialize(world, a.dialogUI.htmlDialogs["about"])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(about, editorEngineVersionLabel()) || !strings.Contains(about, "Author: diiyw") {
		t.Fatal("about is missing engine version or author")
	}
	for _, obsolete := range []string{"about-title", "about-body", "about-menus", "about-shortcuts"} {
		if strings.Contains(about, obsolete) {
			t.Fatalf("about still contains %q", obsolete)
		}
	}
	if visible, _ := world.Get[scene.Visibility](a.titlebar.entity); visible.Visible {
		t.Fatal("modal left titlebar interactive")
	}
	workspaceDialogTestEvent(t, a, world, "about-close", "", htmlui.EventClick)
	if a.modalVisible() {
		t.Fatal("dialog close event was ignored")
	}
}

func TestWorkspacePreferencesDialogPersistsValuesAndKeepsRetainedControls(t *testing.T) {
	a := workspaceEditorFixture(t)
	world := ecs.NewWorld()
	a.action("file.preferences")
	syncWorkspaceTest(t, a, world)
	owner := a.dialogUI.htmlDialogs["preferences"]
	workspaceDialogTestEvent(t, a, world, htmlDialogFieldID("preferences", "preferences.snapToGrid"), "true", htmlui.EventChange)
	workspaceDialogTestEvent(t, a, world, "preferences-save", "", htmlui.EventClick)
	settings, err := loadEditorSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !settings.SnapToGrid {
		t.Fatal("preference control values were lost")
	}
	a.action("file.preferences")
	syncWorkspaceTest(t, a, world)
	if a.dialogUI.htmlDialogs["preferences"] != owner {
		t.Fatal("reopening dialog replaced retained surface")
	}
	if allocations := testing.AllocsPerRun(50, func() {
		if err := a.update(&ecs.SystemContext{World: world}); err != nil {
			panic(err)
		}
		if err := a.sync(&ecs.SystemContext{World: world}); err != nil {
			panic(err)
		}
		if err := htmlui.Sync(world); err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatalf("idle dialog allocated %g", allocations)
	}
}

func TestWorkspaceProjectSettingsCategoriesSaveAsOneUndoableTransaction(t *testing.T) {
	a := workspaceEditorFixture(t)
	world := ecs.NewWorld()
	a.action("edit.project-settings")
	syncWorkspaceTest(t, a, world)
	workspaceDialogTestEvent(t, a, world, htmlDialogFieldID("project", "project.window.title"), "Edited title", htmlui.EventInput)
	workspaceDialogTestEvent(t, a, world, "project-category-rendering", "", htmlui.EventClick)
	workspaceDialogTestEvent(t, a, world, htmlDialogFieldID("project", "project.viewport.width"), "900", htmlui.EventInput)
	workspaceDialogTestEvent(t, a, world, "project-category-features", "", htmlui.EventClick)
	workspaceDialogTestEvent(t, a, world, htmlDialogFieldID("project", "project.engine.features.render2d"), "false", htmlui.EventChange)
	workspaceDialogTestEvent(t, a, world, htmlDialogFieldID("project", "project.engine.features.ui-html"), "true", htmlui.EventChange)
	if !a.projectEngine.HasFeature(project.FeatureRender2D) || !a.projectEngine.HasFeature(project.FeatureUIHTML) {
		t.Fatal("feature dependencies not restored")
	}
	workspaceDialogTestEvent(t, a, world, "project-category-resource-packages", "", htmlui.EventClick)
	workspaceDialogTestEvent(t, a, world, "project-resource-packages-add", "", htmlui.EventClick)
	workspaceDialogTestEvent(t, a, world, htmlDialogFieldID("project", resourcePackagesNameField), "pictures", htmlui.EventInput)
	workspaceDialogTestEvent(t, a, world, htmlDialogFieldID("project", resourcePackagesIncludeField), "button/resources/**", htmlui.EventInput)
	workspaceDialogTestEvent(t, a, world, "project-save", "", htmlui.EventClick)
	if a.modalVisible() || a.Document.Dirty() || a.Document.Spec.Window.Title != "Edited title" || a.Document.Spec.Viewport.Width != 900 {
		t.Fatal("project settings were not applied", a.status)
	}
	if len(a.Document.undo) != 1 || len(a.Document.undo[0].Edits) != 2 {
		t.Fatal("settings were not one transaction")
	}
	config, err := loadResourcePackageConfig(a.Document.ProjectDir)
	if err != nil || len(config.Packages) != 1 || config.Packages[0].Name != "pictures" {
		t.Fatal("package settings were not saved", err)
	}
	a.action("edit.undo")
	if a.Document.Spec.Window.Title == "Edited title" {
		t.Fatal("undo did not restore project manifest")
	}
	a.action("file.save")
	if _, err := os.Stat(a.Document.Absolute("resources/packages.json")); !os.IsNotExist(err) {
		t.Fatal("undo did not remove newly created package config", err)
	}
}

func TestWorkspaceMenuReloadAndCloseUseResourceConfirmationWithoutLosingEdits(t *testing.T) {
	a := workspaceEditorFixture(t)
	world := ecs.NewWorld()
	original := a.Document.Instance("start").Placement.Position[0]
	if err := a.Document.EditInstance("start", "Move", func(instance *project.ModInstanceSpec) error { instance.Placement.Position[0] = 999; return nil }); err != nil {
		t.Fatal(err)
	}
	a.action("file.reload")
	syncWorkspaceTest(t, a, world)
	owner := a.dialogUI.htmlDialogs["close-confirm"]
	surface, _ := world.Get[htmlui.Surface](owner)
	if surface.Value != editorresources.DialogHTML("close-confirm") {
		t.Fatal("confirmation template not used")
	}
	for id, icon := range map[string]string{"close-confirm-save": "save.svg", "close-confirm-discard": "trash-2.svg#danger", "close-confirm-cancel": "close.svg"} {
		button := titlebarButtonTag(t, world, owner, id)
		if !strings.Contains(button, `src="onix-titlebar/`+icon+`"`) || strings.Contains(button, `style="display:none"`) {
			t.Fatalf("confirmation button %q lost its authored icon: %s", id, button)
		}
		if id == "close-confirm-discard" && !strings.Contains(button, "danger") {
			t.Fatal("discard is missing danger styling")
		}
	}
	workspaceDialogTestEvent(t, a, world, "close-confirm-cancel", "", htmlui.EventClick)
	if a.Document.Instance("start").Placement.Position[0] != 999 {
		t.Fatal("cancel discarded edits")
	}
	a.action("file.reload")
	syncWorkspaceTest(t, a, world)
	workspaceDialogTestEvent(t, a, world, "close-confirm-discard", "", htmlui.EventClick)
	if a.Document.Instance("start").Placement.Position[0] != original || a.pendingAction != "" {
		t.Fatal("discard failed to reload")
	}
	a.action("file.close-project")
	if a.Document != nil {
		t.Fatal("close project action was ignored")
	}
	syncWorkspaceTest(t, a, world)
	workspaceDialogTestEvent(t, a, world, "welcome-recent-0-button", "", htmlui.EventClick)
	if a.Document == nil {
		t.Fatal("recent project action did not reopen project")
	}
}

func TestWorkspaceMenuSelectAllDeleteAndUndoRestoreAllInstances(t *testing.T) {
	a := workspaceEditorFixture(t)
	a.action("edit.select-all")
	if len(a.selection) != 3 {
		t.Fatal("select all did not include scene instances")
	}
	a.action("edit.delete")
	if len(a.Document.Scene().Spec.Modules) != 0 || len(a.Document.undo) != 1 {
		t.Fatal("selection deletion not a single edit")
	}
	a.action("edit.undo")
	if len(a.Document.Scene().Spec.Modules) != 3 {
		t.Fatal("undo lost selected instances")
	}
}

func TestWorkspaceBuildOptionsAndResultDialogsUseResources(t *testing.T) {
	a := workspaceEditorFixture(t)
	world := ecs.NewWorld()
	a.action("debug.build")
	syncWorkspaceTest(t, a, world)
	workspaceDialogTestEvent(t, a, world, "build-platform-android", "", htmlui.EventClick)
	workspaceDialogTestEvent(t, a, world, htmlDialogFieldID("build", buildJavaPackageField), "com.example.game", htmlui.EventInput)
	workspaceDialogTestEvent(t, a, world, htmlDialogFieldID("build", buildAndroidMinSDKField), "28", htmlui.EventInput)
	if a.buildOptions.JavaPackage != "com.example.game" || a.buildOptions.AndroidMinSDK != 28 {
		t.Fatal("platform-specific build fields ignored")
	}
	workspaceDialogTestEvent(t, a, world, "build-platform-macos", "", htmlui.EventClick)
	workspaceDialogTestEvent(t, a, world, "build-architecture-arm64", "", htmlui.EventClick)
	workspaceDialogTestEvent(t, a, world, "build-package-format-dmg", "", htmlui.EventClick)
	if a.buildOptions.Architecture != BuildArchitectureARM64 || a.buildOptions.PackageFormat != BuildPackageDMG {
		t.Fatal("architecture/package choices ignored")
	}
	workspaceDialogTestEvent(t, a, world, "build-close", "", htmlui.EventClick)
	for _, name := range []string{"progress", "build-success", "build-error", "run-failure"} {
		a.buildState = BuildProgress{Message: "Compiling", Value: .5}
		a.buildError, a.runOutput = "failure", "stderr output"
		a.showWorkspaceDialog(name)
		syncWorkspaceTest(t, a, world)
		surface, _ := world.Get[htmlui.Surface](a.dialogUI.htmlDialogs[name])
		if surface.Value != editorresources.DialogHTML(name) {
			t.Fatal("template not used", name)
		}
		a.closeWorkspaceDialog()
	}
}

func TestWorkspaceDialogPathFieldsUseNativePickers(t *testing.T) {
	a := workspaceEditorFixture(t)
	world := ecs.NewWorld()
	icon := filepath.Join(a.Document.ProjectDir, "resources", "app-icon.png")
	if err := writeWorkspaceFile(icon, []byte("png")); err != nil {
		t.Fatal(err)
	}
	var options window.FileDialogOptions
	a.openNativeFile = func(value window.FileDialogOptions) (string, error) { options = value; return icon, nil }
	a.action("debug.build")
	syncWorkspaceTest(t, a, world)
	workspaceDialogTestEvent(t, a, world, htmlDialogFieldID("build", buildIconField)+"-browse", "", htmlui.EventClick)
	if a.buildOptions.Icon != icon || len(options.Filters) != 1 || options.Filters[0].Name != "Icon" {
		t.Fatalf("build icon field did not use the native picker: %q %+v", a.buildOptions.Icon, options)
	}
	workspaceDialogTestEvent(t, a, world, "build-platform-android", "", htmlui.EventClick)
	sdk := filepath.Join(a.Document.ProjectDir, "resources")
	a.openNativeFolder = func(value window.FileDialogOptions) (string, error) { options = value; return sdk, nil }
	workspaceDialogTestEvent(t, a, world, htmlDialogFieldID("build", buildAndroidSDKField)+"-browse", "", htmlui.EventClick)
	if a.buildOptions.AndroidSDK != sdk || options.Title != "Android SDK" {
		t.Fatalf("Android SDK field did not use the native folder picker: %q %+v", a.buildOptions.AndroidSDK, options)
	}
	workspaceDialogTestEvent(t, a, world, "build-close", "", htmlui.EventClick)
	a.action("edit.project-settings")
	syncWorkspaceTest(t, a, world)
	input := htmlDialogFieldID("project", projectWindowIconField)
	workspaceDialogTestEvent(t, a, world, input+"-browse", "", htmlui.EventClick)
	if a.dialogValues[input] != "app-icon.png" || options.DefaultPath != sdk {
		t.Fatalf("window icon did not author a resources-relative path: %q %+v", a.dialogValues[input], options)
	}
	a.openNativeFile = func(window.FileDialogOptions) (string, error) {
		return filepath.Join(a.Document.ProjectDir, "outside.png"), nil
	}
	workspaceDialogTestEvent(t, a, world, input+"-browse", "", htmlui.EventClick)
	if a.dialogValues[input] != "app-icon.png" || !strings.Contains(a.status, "must be inside") {
		t.Fatalf("an icon outside resources was accepted: %q %s", a.dialogValues[input], a.status)
	}
	workspaceDialogTestEvent(t, a, world, "project-save", "", htmlui.EventClick)
	if a.Document.Spec.Window.Icon != "app-icon.png" {
		t.Fatalf("picked icon was not saved: %q %s", a.Document.Spec.Window.Icon, a.status)
	}
}

func TestWorkspaceProjectDialogCreatesWithAuthoredNameAndDimensions(t *testing.T) {
	a := workspaceEditorFixture(t)
	world := ecs.NewWorld()
	a.action("file.new-project")
	a.projectParent = t.TempDir()
	syncWorkspaceTest(t, a, world)
	for name, value := range map[string]string{"name": "Created", "width": "1024", "height": "768"} {
		workspaceDialogTestEvent(t, a, world, htmlDialogFieldID("project-dialog", "project-dialog."+name), value, htmlui.EventInput)
	}
	workspaceDialogTestEvent(t, a, world, "project-dialog-confirm", "", htmlui.EventClick)
	if a.Document == nil || a.Document.Spec.Window.Width != 1024 || a.Document.Spec.Window.Height != 768 || filepath.Base(a.Document.ProjectDir) != "Created" {
		t.Fatal(fmt.Sprintf("new project dialog failed: %s", a.status))
	}
}

func TestWorkspaceWelcomeOpenUsesNativeDirectorySelection(t *testing.T) {
	a := workspaceEditorFixture(t)
	directory := a.Document.ProjectDir
	a.Document = nil
	a.settings.RecentProjects = nil
	a.showWorkspaceDialog("welcome")
	world := ecs.NewWorld()
	syncWorkspaceTest(t, a, world)
	surface, _ := world.Get[htmlui.Surface](a.dialogUI.htmlDialogs["welcome"])
	if surface.Value != editorresources.DialogHTML("welcome") {
		t.Fatal("welcome template not used")
	}
	selections := 0
	a.openNativeFolder = func(options window.FileDialogOptions) (string, error) {
		selections++
		if options.Title != "Open Project" {
			t.Error("unexpected native picker title", options.Title)
		}
		return directory, nil
	}
	workspaceDialogTestEvent(t, a, world, "welcome-open", "", htmlui.EventClick)
	if selections != 1 || a.dialog != "" || a.Document == nil || a.Document.ProjectDir != directory || a.settings.FilePickerDirectory != directory {
		t.Fatal("directory selection did not open and remember project")
	}
}

func TestWorkspaceNativeOpenCancellationAndFailurePreserveState(t *testing.T) {
	for _, welcome := range []bool{false, true} {
		for _, pickerErr := range []error{nil, fmt.Errorf("native picker failed")} {
			t.Run(fmt.Sprintf("welcome=%t/error=%v", welcome, pickerErr), func(t *testing.T) {
				a := workspaceEditorFixture(t)
				if welcome {
					a.Document = nil
					a.showWorkspaceDialog("welcome")
				}
				document, dialog, path := a.Document, a.dialog, a.pathDraft
				a.openNativeFolder = func(window.FileDialogOptions) (string, error) { return "", pickerErr }
				if welcome {
					a.handleWorkspaceWelcome("welcome-open")
				} else {
					a.menuOpen = "file"
					a.action("file.open-project")
				}
				if a.Document != document || a.dialog != dialog || a.pathDraft != path || a.pendingAction != "" || a.menuOpen != "" {
					t.Fatal("cancelled or failed native picker changed workspace state")
				}
				if pickerErr != nil && a.status != pickerErr.Error() {
					t.Fatal("native picker error was not reported")
				}
			})
		}
	}
}

func TestWorkspaceMenuNativeOpenPreservesUnsavedConfirmation(t *testing.T) {
	a := workspaceEditorFixture(t)
	document := a.Document
	directory := workspaceFixture(t).ProjectDir
	if err := document.EditInstance("start", "Move", func(instance *project.ModInstanceSpec) error { instance.Placement.Position[0] = 999; return nil }); err != nil {
		t.Fatal(err)
	}
	selections := 0
	a.openNativeFolder = func(window.FileDialogOptions) (string, error) {
		selections++
		return directory, nil
	}
	a.action("file.open-project")
	if selections != 1 || a.pendingAction != "open" || a.activeWorkspaceDialog() != "close-confirm" || a.Document != document {
		t.Fatal("native open skipped unsaved confirmation")
	}
	a.action("cancel-pending")
	if a.Document != document || !document.Dirty() {
		t.Fatal("cancelling project switch lost edits")
	}
	a.action("file.open-project")
	a.action("discard-pending")
	if selections != 2 || a.pendingAction != "" || a.dialog != "" || a.Document == document || a.Document.ProjectDir != directory {
		t.Fatal("confirmed native selection did not open project", a.status)
	}
}

func TestWorkspaceInvalidProjectSettingsKeepDialogAndDocumentIntact(t *testing.T) {
	a := workspaceEditorFixture(t)
	world := ecs.NewWorld()
	a.action("edit.project-settings")
	syncWorkspaceTest(t, a, world)
	revision := a.Document.Revision
	workspaceDialogTestEvent(t, a, world, htmlDialogFieldID("project", "project.window.width"), "-10", htmlui.EventInput)
	workspaceDialogTestEvent(t, a, world, "project-save", "", htmlui.EventClick)
	if a.dialog != "project" || a.Document.Revision != revision || a.Document.Dirty() {
		t.Fatal("invalid settings changed project")
	}
	if _, ok := htmlui.NodeByID(world, a.dialogUI.htmlDialogs["project"], "workspace-dialog-error"); !ok {
		t.Fatal("validation error was not shown in dialog")
	}
}

func TestWorkspaceNativeCloseFromBuildDialogCanSavePendingChanges(t *testing.T) {
	a := workspaceEditorFixture(t)
	if err := a.Document.EditInstance("start", "Move", func(instance *project.ModInstanceSpec) error { instance.Placement.Position[0] = 99; return nil }); err != nil {
		t.Fatal(err)
	}
	a.action("debug.build")
	if a.closeRequested() {
		t.Fatal("native close skipped unsaved confirmation")
	}
	closed := false
	a.requestClose = func() { closed = true }
	world := ecs.NewWorld()
	syncWorkspaceTest(t, a, world)
	workspaceDialogTestEvent(t, a, world, "close-confirm-save", "", htmlui.EventClick)
	if !closed || a.Document.Dirty() {
		t.Fatal("build dialog blocked saving before close")
	}
}
