package editor

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/project"
	engineui "github.com/diiyw/onix/ui"
	htmlui "github.com/diiyw/onix/ui/html"
	"github.com/diiyw/onix/window"
)

type workspaceChrome struct {
	titlebar                                                          editorTitlebar
	dialogUI                                                          editorDialogUI
	settings                                                          editorSettings
	dialog, submenu, projectMode, projectName, projectParent          string
	projectWidth, projectHeight                                       int
	preferencesCategory                                               preferencesCategory
	projectCategory                                                   projectSettingsCategory
	preferencesDraft                                                  editorSettings
	projectEngine                                                     project.EngineConfig
	projectEngineDirty, packageDirty                                  bool
	dialogValues                                                      map[string]string
	dialogFields                                                      map[string]workspaceDialogField
	selection                                                         map[string]bool
	packages                                                          resourcePackageConfig
	packageIndex                                                      int
	buildState                                                        BuildProgress
	buildLog                                                          string
	buildArtifact, buildDirectory, buildError, runOutput, dialogError string
	dialogCopied                                                      bool
	windowContext                                                     *window.Context
	openNativeFolder                                                  func(window.FileDialogOptions) (string, error)
	openNativeFile                                                    func(window.FileDialogOptions) (string, error)
}

func (a *ModuleEditor) modalVisible() bool {
	return a.dialog != "" || a.pendingAction != "" || a.buildOpen || a.buildDone != nil || a.runBuilding || a.animation.CurveOpen
}

func (a *ModuleEditor) workspaceControlFocused() bool {
	if a.world == nil {
		return false
	}
	for _, access := range a.world.Query[engineui.TextField]().All {
		if access.Get().Focused {
			return true
		}
	}
	return false
}

func (a *ModuleEditor) showWorkspaceDialog(name string) {
	a.closeWorkspacePopup()
	a.dialog, a.menuOpen, a.submenu = name, "", ""
	a.contextOpen, a.fileMenuOpen = false, false
	a.dialogCopied = false
	a.dialogError = ""
	if a.world != nil {
		engineui.FocusControl(a.world, 0)
	}
	a.dialogValues = make(map[string]string)
	a.dialogFields = make(map[string]workspaceDialogField)
	a.cancelPlacementDrag()
	a.dragID, a.panelResizeMode, a.panelResizeHover = "", "", ""
	a.changed()
}

func (a *ModuleEditor) closeWorkspaceDialog() {
	if a.animation.CurveOpen {
		a.closeAnimationCurve()
		return
	}
	if a.buildDone != nil || a.runBuilding {
		return
	}
	a.closeWorkspacePopup()
	a.dialog, a.pendingAction, a.menuOpen, a.submenu = "", "", "", ""
	a.buildOpen = false
	if a.world != nil {
		engineui.FocusControl(a.world, 0)
	}
	a.changed()
}

func (a *ModuleEditor) chromeAction(id string) bool {
	switch id {
	case "file.open-project":
		a.menuOpen, a.submenu, a.contextOpen = "", "", false
		a.changed()
		if a.runDone != nil {
			a.report(fmt.Errorf("Stop Play first"))
			return true
		}
		value, err := a.chooseWorkspacePath(true, "Open Project", defaultFilePickerDirectory(a.settings, a.pathDraft))
		if err != nil {
			a.report(err)
		} else if value != "" {
			a.dialog, a.pathDraft = "", value
			a.action("open")
		}
	case "file.new-project":
		a.projectMode = "new"
		a.projectName, a.projectWidth, a.projectHeight = "NewProject", 750, 1624
		a.projectParent = defaultFilePickerDirectory(a.settings, a.pathDraft)
		a.showWorkspaceDialog("project-dialog")
	case "file.open-recent-project":
		a.menuOpen, a.submenu = "file", id
		a.changed()
	case "file.close-project":
		a.requestWorkspaceNavigation("close-project")
	case "file.reload", "top-reload":
		a.requestWorkspaceNavigation("reload")
	case "file.save", "top-save":
		a.action("save")
	case "file.preferences":
		a.preferencesCategory, a.preferencesDraft = preferencesEditing, a.settings
		a.showWorkspaceDialog("preferences")
	case "edit.project-settings":
		if a.Document == nil {
			a.report(fmt.Errorf("Open a project first"))
			return true
		}
		var packages resourcePackageConfig
		var err error
		if state := a.Document.states["resources/packages.json"]; state != nil {
			packages, err = resourcePackageConfig{}, nil
			if state.data != nil {
				packages, err = decodeDocument[resourcePackageConfig](state.data)
			}
		} else {
			packages, err = loadResourcePackageConfig(a.Document.ProjectDir)
		}
		if err != nil {
			a.report(err)
			return true
		}
		a.packages, a.packageIndex, a.projectCategory = packages, 0, projectSettingsWindow
		a.projectEngine = a.Document.Spec.Engine
		a.projectEngine.Features = append([]project.RuntimeFeatureID(nil), a.projectEngine.Features...)
		a.projectEngine.Resources = append([]project.ResourceID(nil), a.projectEngine.Resources...)
		a.projectEngine.ImageDecoders = append([]project.ImageDecoderID(nil), a.projectEngine.ImageDecoders...)
		a.projectEngineDirty, a.packageDirty = false, false
		a.showWorkspaceDialog("project")
	case "edit.undo":
		a.action("undo")
	case "edit.redo":
		a.action("redo")
	case "edit.toggle-grid-snap":
		next := a.settings
		next.SnapToGrid = !next.SnapToGrid
		if err := saveEditorSettings(next); err != nil {
			a.report(err)
			return true
		}
		a.settings = next
		a.status = fmt.Sprintf("Snap to Grid: %t", next.SnapToGrid)
		a.changed()
	case "edit.select-all":
		if a.Document == nil {
			return true
		}
		if a.hasDrafts() {
			a.report(fmt.Errorf("Apply form changes first"))
			return true
		}
		clear(a.selection)
		var selectInstances func([]project.ModInstanceSpec)
		selectInstances = func(values []project.ModInstanceSpec) {
			for _, value := range values {
				a.selection[value.ID] = true
				selectInstances(value.Children)
			}
		}
		selectInstances(a.Document.Scene().Spec.Modules)
		a.Selected, a.Local, a.selectedElement, a.selectedResource = "", "", "", ""
		a.selectedSlot, a.slotCreating = "", false
		a.status = fmt.Sprintf("Selected %d module instances", len(a.selection))
		a.changed()
	case "edit.delete":
		if a.Document == nil {
			return true
		}
		if a.selectedSlot != "" && a.editingModule.Scene != "" {
			a.contextScene, a.contextParent, a.contextMod, a.contextSlot = a.Document.Active, a.Selected, a.selectedMod(), a.selectedSlot
			a.outlinerAction("outliner-delete-slot")
			return true
		}
		if a.selectedResource != "" && a.editingModule.Scene != "" {
			a.contextScene, a.contextParent, a.contextMod, a.contextResource = a.Document.Active, a.Selected, a.selectedMod(), a.selectedResource
			a.outlinerAction("outliner-delete-resource")
			return true
		}
		if err := a.applyDrafts(); err != nil {
			a.report(err)
			return true
		}
		if len(a.selection) == 0 {
			if a.Selected == "" {
				return true
			}
			a.selection[a.Selected] = true
		}
		err := a.Document.DeleteInstances(a.selection)
		if err == nil {
			a.selectInstance("")
		}
		a.report(err)
	case "debug.run", "top-run":
		if a.runDone != nil {
			a.action("stop")
		} else {
			a.action("play")
		}
	case "debug.build":
		a.action("build")
	case "debug.rebuild-preview":
		a.disposeCanvasResources()
		a.viewport3D.dispose()
		a.Preview = DataPreview{}
		a.status = "Preview rebuilt"
		a.changed()
	case "debug.focus", "top-frame":
		a.action("focus")
	case "top-dimension":
		a.action("toggle-dimension")
	case "about.show":
		a.showWorkspaceDialog("about")
	default:
		if index, ok := strings.CutPrefix(id, "recent-"); ok {
			i, err := strconv.Atoi(index)
			if err == nil && i >= 0 && i < len(a.titlebar.recentPaths) {
				a.requestWorkspaceNavigation("recent:" + a.titlebar.recentPaths[i])
			}
			return true
		}
		return false
	}
	return true
}

func (a *ModuleEditor) requestWorkspaceNavigation(action string) {
	a.menuOpen, a.submenu = "", ""
	if a.runDone != nil {
		a.report(fmt.Errorf("Stop Play first"))
		return
	}
	if a.Document != nil && (a.Document.Dirty() || a.hasDrafts()) {
		a.pendingAction = "navigate:" + action
		a.changed()
		return
	}
	a.report(a.navigateWorkspace(action))
}

func (a *ModuleEditor) navigateWorkspace(action string) error {
	switch {
	case action == "close-project":
		a.disposeCanvasResources()
		a.viewport3D.dispose()
		_ = a.Document.Dispose()
		a.draftPreview = workspaceDraftPreview{}
		a.Document, a.Preview, a.Selected, a.Local = nil, DataPreview{}, "", ""
		clear(a.selection)
		clear(a.drafts)
		clear(a.staged)
		a.status = "Project closed"
		a.showWorkspaceDialog("welcome")
		return nil
	case action == "reload":
		if a.Document == nil {
			return nil
		}
		return a.reloadWorkspace()
	case strings.HasPrefix(action, "recent:"):
		return a.open(strings.TrimPrefix(action, "recent:"))
	}
	return fmt.Errorf("unknown project action %q", action)
}

func (a *ModuleEditor) reloadWorkspace() error {
	doc, err := LoadWorkspaceDocument(a.Document.ProjectDir)
	if err != nil {
		return err
	}
	if doc.Scenes[a.Document.Active] != nil {
		doc.Active = a.Document.Active
	}
	offset, focusPending := a.viewport2D.offset, a.viewport2D.focusPending
	a.disposeCanvasResources()
	a.viewport3D.dispose()
	a.disposeWorkspacePopups()
	_ = a.Document.Dispose()
	a.draftPreview = workspaceDraftPreview{}
	a.Document, a.Preview = doc, DataPreview{}
	clear(a.drafts)
	clear(a.staged)
	clear(a.additions)
	clear(a.atlasFrames)
	a.componentPickerOpen, a.contextOpen, a.componentMenu = false, false, ""
	a.elementPicker.Field = ""
	if a.editingModule.Scene != "" && (a.editingModule.Scene != doc.Active || doc.Instance(a.editingModule.ID) == nil) {
		a.editingModule = workspaceModuleKey{}
		a.Local = ""
	}
	if doc.Instance(a.Selected) == nil {
		a.Selected = ""
	}
	if a.editingModule.Scene != "" {
		a.Local = a.editLocal()
	} else if mod := doc.Mods[a.Local]; mod == nil || mod.Deleted {
		a.Local = ""
	}
	for id := range a.selection {
		if doc.Instance(id) == nil {
			delete(a.selection, id)
		}
	}
	if mod := doc.Mods[a.selectedMod()]; mod != nil && a.editingModule.Scene != "" {
		if _, ok := a.selectedElementIndex(mod.Spec.Elements); !ok {
			a.selectedElement = ""
		}
		if _, ok := mod.Spec.Resources[a.selectedResource]; !ok {
			a.selectedResource = ""
		}
		if _, ok := mod.Spec.Slots[a.selectedSlot]; !ok {
			a.selectedSlot = ""
		}
	} else {
		a.selectedElement, a.selectedResource, a.selectedSlot = "", "", ""
	}
	a.slotCreating = false
	a.viewport2D.offset, a.viewport2D.focusPending, a.viewport2D.local = offset, focusPending, a.Local
	a.status = "Reloaded from disk"
	a.changed()
	return nil
}

func (a *ModuleEditor) workspaceMenuOpen() bool {
	return a.menuOpen != "" || a.contextOpen || a.componentPickerOpen || a.componentMenu != "" || a.elementPicker.Field != ""
}

func (a *ModuleEditor) updateWorkspaceMenus(in *input.State) {
	if a.modalVisible() {
		return
	}
	if in.KeyPressed(input.KeyEscape) {
		a.menuOpen, a.submenu = "", ""
		a.componentMenu = ""
		a.componentPickerOpen = false
		a.elementPicker.Field = ""
		a.changed()
		return
	}
	x, y := in.Pointer().X, in.Pointer().Y
	if p := &a.elementPicker; p.Field != "" {
		if anchor, ok := a.elementPickerAnchor(p.Field); ok && anchor != p.Anchor {
			p.Anchor = anchor
			a.changed()
		}
		if in.MousePressed(input.MouseLeft) && !viewportContains(p.Anchor, x, y) && !a.outlinerContains("content-elements-list", x, y) {
			p.Field = ""
			a.changed()
		}
	}
	if a.componentPickerOpen && in.MousePressed(input.MouseLeft) && !a.outlinerContains("content-picker", x, y) && !a.outlinerContains("content-picker-list", x, y) {
		a.componentPickerOpen = false
		a.changed()
	}
	top := a.layout().TopBar
	x, y = x-top.X, y-top.Y
	t := &a.titlebar
	if t.hoverKnown && x == t.hoverX && y == t.hoverY && !in.MousePressed(input.MouseLeft) {
		return
	}
	t.hoverKnown, t.hoverX, t.hoverY = true, x, y
	menu, submenu := a.menuOpen, a.submenu
	for _, item := range t.menus {
		if workspaceNodeContains(item.trigger, x, y) {
			if menu != "" {
				a.menuOpen, a.submenu = item.key, ""
			}
			if menu != a.menuOpen || submenu != a.submenu {
				a.changed()
			}
			return
		}
		if menu == item.key && workspaceNodeContains(item.popup, x, y) {
			if workspaceNodeContains(t.nodes["file.open-recent-project"], x, y) {
				a.submenu = "file.open-recent-project"
			} else {
				a.submenu = ""
			}
			if submenu != a.submenu {
				a.changed()
			}
			return
		}
	}
	if submenu != "" && workspaceNodeContains(t.nodes["popup-recent"], x, y) {
		return
	}
	if menu != "" && in.MousePressed(input.MouseLeft) {
		a.menuOpen, a.submenu = "", ""
		a.changed()
	}
}

func workspaceNodeContains(node htmlui.Node, x, y float32) bool {
	box, ok := node.Bounds()
	return ok && x >= box.X && x < box.X+box.Width && y >= box.Y && y < box.Y+box.Height
}

func (a *ModuleEditor) isSelected(id string) bool { return id == a.Selected || a.selection[id] }

func (a *ModuleEditor) selectWorkspaceInstance(id string) {
	if a.world != nil {
		if in, ok := a.world.GetResource[*input.State](); ok && in != nil && (in.KeyHeld(input.KeyLeftControl) || in.KeyHeld(input.KeyRightControl)) {
			if a.selection == nil {
				a.selection = make(map[string]bool)
			}
			if a.Selected != "" {
				a.selection[a.Selected] = true
				a.Selected = ""
			}
			if a.selection[id] {
				delete(a.selection, id)
			} else {
				a.selection[id] = true
			}
			a.Local = a.editLocal()
			a.changed()
			return
		}
	}
	a.selectInstance(id)
}

func (a *ModuleEditor) chooseWorkspacePath(folder bool, title, current string, filters ...window.FileDialogFilter) (string, error) {
	options := window.FileDialogOptions{Title: title, DefaultPath: fileDialogDirectory(current), Filters: filters}
	var value string
	var err error
	if folder {
		if a.openNativeFolder != nil {
			value, err = a.openNativeFolder(options)
		} else if a.windowContext != nil {
			value, err = a.windowContext.OpenFolder(options)
		} else {
			value, err = window.OpenFolder(options)
		}
	} else {
		if a.openNativeFile != nil {
			value, err = a.openNativeFile(options)
		} else if a.windowContext != nil {
			value, err = a.windowContext.OpenFile(options)
		} else {
			value, err = window.OpenFile(options)
		}
	}
	if err == nil && value != "" {
		directory := value
		if !folder {
			directory = filepath.Dir(value)
		}
		err = a.settings.rememberFilePickerDirectory(directory)
	}
	return value, err
}

func (a *ModuleEditor) copyWorkspaceOutput(value string) {
	if a.windowContext != nil && a.windowContext.SetClipboardText(value) {
		a.dialogCopied = true
		a.changed()
	}
}
