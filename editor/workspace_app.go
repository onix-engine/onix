package editor

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/diiyw/onix"
	"github.com/diiyw/onix/ecs"
	editorresources "github.com/diiyw/onix/editor/resources"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	onixtext "github.com/diiyw/onix/text"
	engineui "github.com/diiyw/onix/ui"
	htmlui "github.com/diiyw/onix/ui/html"
	"github.com/diiyw/onix/window"
	windowsvg "github.com/diiyw/onix/window/codec/svg"
	"github.com/go-gl/mathgl/mgl32"
)

// ModuleEditor edits module definitions and scene instances without running project behavior.
type ModuleEditor struct {
	workspaceChrome
	workspaceOutliner
	animation                                                   workspaceAnimation
	Document                                                    *WorkspaceDocument
	Preview                                                     DataPreview
	draftPreview                                                workspaceDraftPreview
	Selected                                                    string
	Local                                                       project.ModID
	font                                                        *onixtext.Font
	world                                                       *ecs.World
	shell, canvas                                               ecs.Entity
	shellComponent                                              ecs.Entity
	events                                                      ecs.MessageReader[htmlui.Event]
	width, height                                               float32
	dirty                                                       bool
	canvasDirty                                                 bool
	canvasImageVersion                                          uint64
	status                                                      string
	pathDraft, sceneDraft, modDraft, instanceDraft, folderDraft string
	fields                                                      map[string]workspaceField
	definitionMatrices                                          []mgl32.Mat4
	sectionsCollapsed                                           map[string]bool
	componentPickerOpen                                         bool
	componentPickerAnchor                                       rect
	componentMenu                                               string
	elementPicker                                               workspaceElementPicker
	colorPicker                                                 workspaceColorPicker
	nineSlice                                                   workspaceNineSlice
	tilemap                                                     workspaceTilemap
	textureModes                                                map[string]string
	atlasFrames                                                 map[string]workspaceAtlasFrames
	controlValues                                               map[string]string
	drafts                                                      map[string]string
	staged                                                      map[DocumentID][]byte
	viewMods                                                    map[DocumentID]stagedViewEntry[project.ModSpec]
	viewScenes                                                  map[DocumentID]stagedViewEntry[project.SceneSpec]
	additions                                                   map[string]string
	files                                                       []WorkspaceFile
	fileBrowser                                                 workspaceFileBrowser
	rows                                                        []workspaceTreeRow
	rowByID                                                     map[string]workspaceTreeRow
	zoom                                                        float32
	dimension                                                   previewDimension
	viewport2D                                                  workspaceViewport2D
	viewport3D                                                  workspaceViewport3D
	leftPanelWidth, rightPanelWidth, contentPanelHeight         float32
	panelResizeMode, panelResizeHover, menuOpen                 string
	lastFile                                                    string
	lastFileClick                                               float64
	time                                                        float64
	dragID                                                      string
	dragX, dragY                                                float32
	dragMoved                                                   bool
	canvasScale                                                 float32
	canvasDrag                                                  workspacePlacementDrag
	pendingAction                                               string
	closeConfirmed                                              bool
	requestRender                                               func()
	requestWake                                                 func(time.Time)
	requestClose                                                func()
	requestMinimize, requestToggleMaximize                      func()
	runCmd                                                      *exec.Cmd
	runDone                                                     chan runProcessResult
	runPrepared                                                 chan preparedRun
	runExecutable, runDirectory                                 string
	runResources                                                DevelopmentBuild
	runBuilding, runStopping                                    bool
	buildOpen                                                   bool
	buildOptions                                                BuildOptions
	buildDone                                                   chan buildResult
	buildProgress                                               *buildProgressFeed
	progressDirty                                               bool
}

func NewModuleEditor(directory string) (*ModuleEditor, error) {
	font, err := loadSystemUIFont()
	if err != nil {
		return nil, err
	}
	a := &ModuleEditor{font: font, dirty: true, status: "Create or open a project", zoom: 1, dimension: previewDimension2D,
		fields: make(map[string]workspaceField), sectionsCollapsed: make(map[string]bool), controlValues: make(map[string]string), drafts: make(map[string]string), staged: make(map[DocumentID][]byte), additions: make(map[string]string), rowByID: make(map[string]workspaceTreeRow)}
	a.settings, err = loadEditorSettings()
	if err != nil {
		return nil, err
	}
	a.selection = make(map[string]bool)
	if strings.TrimSpace(directory) != "" {
		if err := a.open(directory); err != nil {
			return nil, err
		}
	} else {
		a.showWorkspaceDialog("welcome")
	}
	return a, nil
}

func Run(directory string) error {
	a, err := NewModuleEditor(directory)
	if err != nil {
		return err
	}
	defer a.dispose()
	app := onix.NewApp()
	app.SetWindowOptions(editorWindowOptions())
	app.SetWindowSize(defaultViewportWidth, defaultViewportHeight)
	a.windowContext = app.Context
	app.SetWindowTitle("Onix Editor")
	icon, err := editorresources.LogoData()
	if err != nil {
		return err
	}
	app.Context.SetIconDecoder(windowsvg.Decoder[workspaceWindowIconSize]{})
	if err := app.SetWindowIcon(icon); err != nil {
		return err
	}
	app.SetTargetFPS(defaultTargetFPS)
	app.SetRenderMode(onix.RenderOnDemand)
	a.requestRender, a.requestClose = app.RequestRender, app.RequestClose
	a.requestWake = app.WakeAt
	a.requestMinimize, a.requestToggleMaximize = app.Minimize, app.ToggleMaximize
	app.SetWindowDragRegion(a.windowDragRegion)
	app.OnCloseRequested(a.closeRequested)
	app.Context.SetViewportMode(window.ViewportModeAdaptive)
	app.Context.SetViewport(1, 1)
	registerEditorUIRenderers2D(app.World)
	scene.RegisterCameraRenderer2D(app.World)
	engineui.RegisterRuntimeFeature[engineui.FullRuntime](app.World)
	ecs.InsertResource(app.World, editorUITheme(a.font))
	camera := scene.SpawnCamera2D(app.World)
	ecs.GetMut[scene.Camera](app.World, camera).Clear = false
	if err := app.AddSystems(onix.StagePreUpdate,
		scene.SyncGlobalTransformSystem("mod_editor_transform"),
		ecs.SystemConfig{Name: "mod_editor_input", After: []string{"mod_editor_transform"}, System: scene.DispatchInputEvents2DSystem("").System},
	); err != nil {
		return err
	}
	if err := app.AddSystems(onix.StageUpdate,
		engineui.UpdateSystem("mod_editor_ui_update"),
		ecs.SystemConfig{Name: "mod_editor_events", After: []string{"mod_editor_ui_update"}, System: a.update},
	); err != nil {
		return err
	}
	if err := app.AddSystems(onix.StagePostUpdate,
		ecs.SystemConfig{Name: "mod_editor_view", System: a.sync},
		ecs.SystemConfig{Name: "mod_editor_ui_sync", After: []string{"mod_editor_view"}, System: engineui.SyncSystem("").System},
		ecs.SystemConfig{Name: "mod_editor_restore_scroll", After: []string{"mod_editor_ui_sync"}, System: a.restoreOutlinerScroll},
		ecs.SystemConfig{Name: "mod_editor_restore_animation_scroll", After: []string{"mod_editor_ui_sync"}, System: a.restoreAnimationScroll},
		ecs.SystemConfig{Name: "mod_editor_scroll", After: []string{"mod_editor_restore_scroll", "mod_editor_restore_animation_scroll"}, System: scene.UIScrollSystem("").System},
		ecs.SystemConfig{Name: "mod_editor_after_layout", After: []string{"mod_editor_scroll"}, System: engineui.SyncAfterLayoutSystem("").System},
		ecs.SystemConfig{Name: "mod_editor_ui_transform", After: []string{"mod_editor_after_layout"}, System: scene.SyncUIGlobalTransformSystem("").System},
	); err != nil {
		return err
	}
	if err := app.AddSystems(onix.StageRender,
		ecs.SystemConfig{Name: "mod_editor_viewport", System: a.render},
		ecs.SystemConfig{Name: "mod_editor_cameras", After: []string{"mod_editor_viewport"}, System: scene.ExtractCamerasSystem("").System},
		ecs.SystemConfig{Name: "mod_editor_extract", After: []string{"mod_editor_cameras"}, System: scene.ExtractRender2DSystem("").System},
		ecs.SystemConfig{Name: "mod_editor_render", After: []string{"mod_editor_extract"}, System: scene.RenderCamerasSystem("").System},
		ecs.SystemConfig{Name: "mod_editor_window_border", After: []string{"mod_editor_render"}, System: a.renderWindowBorder},
	); err != nil {
		return err
	}
	return app.Run()
}

func (a *ModuleEditor) open(directory string) error {
	doc, err := LoadWorkspaceDocument(directory)
	if err != nil {
		return err
	}
	a.disposeCanvasResources()
	a.animation = workspaceAnimation{}
	a.viewport3D.dispose()
	a.disposeWorkspacePopups()
	_ = a.Document.Dispose()
	a.draftPreview = workspaceDraftPreview{}
	a.Document, a.Selected, a.Local = doc, "", ""
	clear(a.sectionsCollapsed)
	a.componentMenu = ""
	a.elementPicker.Field = ""
	clear(a.textureModes)
	clear(a.atlasFrames)
	a.workspaceOutliner = workspaceOutliner{resetScroll: true}
	clear(a.selection)
	a.dialog = ""
	a.pathDraft, a.status, a.Preview = doc.ProjectDir, "Opened "+string(doc.Active), DataPreview{}
	a.canvasDrag = workspacePlacementDrag{}
	clear(a.drafts)
	clear(a.staged)
	clear(a.additions)
	if err := a.settings.rememberRecentProject(doc.ProjectDir); err != nil {
		a.status += " · " + err.Error()
	}
	a.changed()
	return nil
}

func (a *ModuleEditor) changed() {
	a.dirty = true
	if a.requestRender != nil {
		a.requestRender()
	}
}

func (a *ModuleEditor) changedCanvas() {
	a.canvasDirty = true
	if a.requestRender != nil {
		a.requestRender()
	}
}

func (a *ModuleEditor) report(err error) {
	if err != nil {
		a.status = err.Error()
		if a.modalVisible() {
			a.dialogError = err.Error()
		}
	}
	a.changed()
}

func (a *ModuleEditor) update(context *ecs.SystemContext) error {
	a.world = context.World
	a.time = float64(time.Now().UnixMilli()) / 1000
	a.updateRun()
	a.updateModuleBuild()
	for event := range a.events.All(context.World) {
		if a.dialogEvent(*event) {
			continue
		}
		if event.Entity == a.shell || event.Entity == a.titlebar.entity {
			a.handleEvent(*event)
		}
	}
	if in, ok := ecs.GetResource[*input.State](context.World); ok && in != nil {
		a.updateOutlinerMenu(in)
		a.updateWorkspaceMenus(in)
		if a.modalVisible() {
			if a.animation.CurveOpen {
				a.updateAnimationCurveFocus(in)
				a.updateAnimationPointer(in.Pointer())
			}
			a.updateWorkspacePopupPointer(in.Pointer())
			if in.KeyPressed(input.KeyEscape) {
				a.closeWorkspaceDialog()
			}
			return nil
		}
		control := in.KeyHeld(input.KeyLeftControl) || in.KeyHeld(input.KeyRightControl)
		a.updateResourceTabKeys(in)
		if control {
			switch {
			case in.KeyPressed(input.KeyS):
				a.action("save")
			case in.KeyPressed(input.KeyZ):
				if in.KeyHeld(input.KeyLeftShift) || in.KeyHeld(input.KeyRightShift) {
					a.action("redo")
				} else {
					a.action("undo")
				}
			case in.KeyPressed(input.KeyY):
				a.action("redo")
			case in.KeyPressed(input.KeyG):
				a.action("edit.toggle-grid-snap")
			case in.KeyPressed(input.KeyA):
				if !a.workspaceControlFocused() {
					a.action("edit.select-all")
				}
			}
		}
		if !a.workspaceControlFocused() {
			switch {
			case in.KeyPressed(input.KeyF5):
				a.action("file.reload")
			case in.KeyPressed(input.KeyF6):
				a.action("top-run")
			case in.KeyPressed(input.KeyDelete):
				a.action("edit.delete")
			case in.KeyPressed(input.KeyF):
				a.action("focus")
			}
		}
		a.updateTreeDrag(in.Pointer())
		if !a.updateAnimationWheel(in) && !a.updateAnimationPointer(in.Pointer()) && !a.updatePanelResize(in) {
			if a.dimension == previewDimension3D {
				a.updateViewport3DInput(in)
			} else {
				a.updateViewport2DInput(in)
				a.updatePlacementDrag(in.Pointer())
				a.updateTilemapPointer(in.Pointer())
			}
		}
	}
	return a.updateViewport2D()
}

func (a *ModuleEditor) handleEvent(event htmlui.Event) {
	if a.dialog != "" {
		return
	}
	if a.buildDone != nil || a.runBuilding {
		return
	}
	if a.buildOpen {
		return
	}
	if a.pendingAction != "" && event.ID != "save-pending" && event.ID != "discard-pending" && event.ID != "cancel-pending" {
		return
	}
	if a.animation.CurveOpen {
		a.animationEvent(event)
		return
	}
	if event.Kind == htmlui.EventFocus && a.componentMenu != "" && event.ID != a.componentMenu {
		a.componentMenu = ""
		a.changed()
	}
	if a.animationEvent(event) {
		return
	}
	if event.Kind == htmlui.EventClick && (event.ID == "resources-tab" || event.ID == "animation-tab") {
		if err := a.stageAnimationInspectorDrafts(); err != nil {
			a.report(err)
			return
		}
		a.animation.Active = event.ID == "animation-tab"
		a.changed()
		return
	}
	if field, ok := a.fields[event.ID]; ok && field.CommitOnChange && (event.Kind == htmlui.EventChange || event.Kind == htmlui.EventSubmit || event.Kind == htmlui.EventBlur) {
		number, err := strconv.ParseFloat(event.Value, 32)
		if err != nil {
			a.drafts[event.ID] = event.Value
			a.report(err)
			return
		}
		var position [3]float32
		copy(position[:], a.Document.Scene().Spec.Transform.Position)
		if position[field.Axis] == float32(number) {
			delete(a.drafts, event.ID)
			return
		}
		position[field.Axis] = float32(number)
		err = a.Document.EditScene(a.Document.Active, "Edit scene position", func(value *project.SceneSpec) error {
			value.Transform.Position = position[:]
			return nil
		})
		if err == nil {
			delete(a.drafts, event.ID)
		} else {
			a.drafts[event.ID] = event.Value
		}
		a.report(err)
		return
	}
	if event.Kind == htmlui.EventInput || event.Kind == htmlui.EventChange {
		if field, ok := a.fields[event.ID]; ok {
			if field.Kind == "content-element-option" {
				if event.Kind == htmlui.EventChange && event.Checked != field.Selected {
					a.report(a.elementPickerAction(field))
				}
				return
			}
			value := event.Value
			if field.Kind == "bool" {
				value = fmt.Sprint(event.Checked)
			}
			a.drafts[event.ID] = value
			if field.Animation && event.Kind == htmlui.EventChange {
				previous := a.animation
				err := a.editAnimation(func(*project.AnimationClipSpec, *project.ModElementSpec) error { return nil })
				if err != nil {
					a.animation = previous
					delete(a.drafts, event.ID)
				}
				a.report(err)
				return
			}
			a.changed()
			return
		}
		switch event.ID {
		case "project-path":
			a.pathDraft = event.Value
		default:
			if strings.HasPrefix(event.ID, "add/") {
				a.additions[event.ID] = event.Value
			}
		}
	}
	if event.Kind != htmlui.EventClick {
		return
	}
	if strings.HasPrefix(event.ID, "select:") {
		if a.hasDrafts() {
			a.pendingAction = event.ID
			a.changed()
			return
		}
		id := strings.TrimPrefix(event.ID, "select:")
		// Double-clicking the already-edited root just toggles it below.
		if a.doubleClickRow(a.Document.Active, id) && a.editingModule != (workspaceModuleKey{Scene: a.Document.Active, ID: id}) {
			a.enterModuleEdit(a.Document.Active, id)
			return
		}
		a.selectWorkspaceInstance(id)
		a.toggleModule(id)
		return
	}
	if strings.HasPrefix(event.ID, "file:") {
		file, err := url.PathUnescape(strings.TrimPrefix(event.ID, "file:"))
		if err != nil {
			a.report(err)
			return
		}
		a.openFile(file)
		return
	}
	if event.ID == "resources-up" {
		a.resourceParentDirectory()
		return
	}
	if a.fileMenuAction(event.ID) {
		return
	}
	if a.outlinerAction(event.ID) {
		return
	}
	a.action(event.ID)
}

func (a *ModuleEditor) selectInstance(id string) {
	a.releaseAnimationPreview()
	a.animation.Active = false
	a.componentPickerOpen = false
	a.componentMenu = ""
	a.elementPicker.Field = ""
	clear(a.textureModes)
	clear(a.selection)
	a.cancelPlacementDrag()
	a.Selected, a.Local, a.selectedElement, a.selectedResource = id, a.editLocal(), "", ""
	a.selectedSlot, a.slotCreating = "", false
	a.instanceDraft = ""
	clear(a.drafts)
	a.changed()
}

// selectElement pins one definition element of the edited module in Details.
func (a *ModuleEditor) selectElement(instance, id string) {
	a.selectInstance(instance)
	a.selectedElement = id
}

func (a *ModuleEditor) selectScene(id string) {
	a.releaseAnimationPreview()
	a.animation.Active = false
	clear(a.selection)
	a.cancelPlacementDrag()
	if err := a.Document.SelectScene(project.SceneID(id)); err != nil {
		a.report(err)
		return
	}
	delete(a.collapsedScenes, project.SceneID(id))
	a.Selected, a.Local, a.selectedElement, a.selectedResource = "", "", "", ""
	a.selectedSlot, a.slotCreating = "", false
	clear(a.drafts)
	a.changed()
}

func (a *ModuleEditor) action(id string) {
	if a.dialog != "" && id != "save-pending" && id != "discard-pending" && id != "cancel-pending" {
		return
	}
	if a.canvasDrag.ID != "" {
		a.cancelPlacementDrag()
		a.changedCanvas()
	}
	if a.pendingAction == "" && !a.buildOpen && a.buildDone == nil && a.chromeAction(id) {
		return
	}
	switch id {
	case "window-minimize":
		if a.requestMinimize != nil {
			a.requestMinimize()
		}
		return
	case "window-maximize":
		if a.requestToggleMaximize != nil {
			a.requestToggleMaximize()
		}
		return
	case "window-close":
		if a.closeRequested() && a.requestClose != nil {
			a.closeConfirmed = true
			a.requestClose()
		}
		return
	}
	if a.buildDone != nil || a.runBuilding && id != "stop" {
		return
	}
	if a.pendingAction != "" && id != "save-pending" && id != "discard-pending" && id != "cancel-pending" {
		return
	}
	if a.buildOpen && a.pendingAction == "" {
		return
	}
	if menu, ok := strings.CutPrefix(id, "menu-"); ok {
		a.componentPickerOpen, a.componentMenu = false, ""
		a.elementPicker.Field = ""
		a.submenu = ""
		if a.menuOpen == menu {
			a.menuOpen = ""
		} else {
			a.menuOpen = menu
		}
		a.changed()
		return
	}
	a.menuOpen = ""
	if id == "build" {
		if a.Document == nil || a.runDone != nil {
			a.report(fmt.Errorf("Open a project and stop Play"))
			return
		}
		a.buildOptions = DefaultBuildOptions(a.Document.ProjectDir)
		a.buildOptions.Platform = hostBuildPlatform(runtime.GOOS)
		a.buildOptions.PackageFormat, _ = normalizedBuildPackageFormat(a.buildOptions.Platform, "")
		a.showWorkspaceDialog("")
		a.buildOpen = true
		a.changed()
		return
	}
	if id == "cancel-pending" {
		a.pendingAction = ""
		a.changed()
		return
	}
	if id == "save-pending" || id == "discard-pending" {
		action := a.pendingAction
		if id == "save-pending" {
			if err := a.applyDrafts(); err != nil {
				a.report(err)
				return
			}
			if a.Document != nil {
				if err := a.Document.Save(); err != nil {
					a.report(err)
					return
				}
			}
		} else {
			clear(a.drafts)
			clear(a.staged)
		}
		a.pendingAction = ""
		a.dialog = ""
		switch {
		case action == "close":
			a.closeConfirmed = true
			if a.requestClose != nil {
				a.requestClose()
			}
		case strings.HasPrefix(action, "select:"):
			a.selectInstance(strings.TrimPrefix(action, "select:"))
			a.toggleModule(strings.TrimPrefix(action, "select:"))
		case strings.HasPrefix(action, "scene:") || strings.HasPrefix(action, "tree-select:") || strings.HasPrefix(action, "element:") || strings.HasPrefix(action, "resource:") || strings.HasPrefix(action, "slot:"):
			a.outlinerAction(action)
		case strings.HasPrefix(action, "local:"):
			a.Local = project.ModID(strings.TrimPrefix(action, "local:"))
		case action == "open" || action == "new":
			a.report(a.openRequested(action))
		case strings.HasPrefix(action, "navigate:"):
			a.report(a.navigateWorkspace(strings.TrimPrefix(action, "navigate:")))
		default:
			a.action(action)
		}
		a.changed()
		return
	}
	if id == "open" || id == "new" {
		if a.runDone != nil {
			a.report(fmt.Errorf("Stop Play first"))
			return
		}
		if a.Document != nil && (a.Document.Dirty() || a.hasDrafts()) && a.pendingAction == "" {
			a.pendingAction = id
			a.changed()
			return
		}
		a.report(a.openRequested(id))
		return
	}
	if a.Document == nil {
		return
	}
	if a.outlinerAction(id) {
		return
	}
	if key, toggle := strings.CutPrefix(id, "toggle:"); toggle {
		a.sectionsCollapsed[key] = !a.sectionsCollapsed[key]
		a.changed()
		return
	}
	var err error
	switch id {
	case "content-picker":
		a.elementPicker.Field = ""
		a.componentPickerOpen = !a.componentPickerOpen
		if a.componentPickerOpen {
			a.componentMenu, a.contextOpen, a.resourceMenuOpen = "", false, false
			a.anchorComponentPicker()
		}
	case "save":
		err = a.applyDrafts()
		if err == nil {
			err = a.Document.Save()
		}
		if err == nil {
			a.status = "Modified scenes and module definitions saved"
		}
	case "undo", "redo":
		if a.hasDrafts() {
			a.status = "Save form changes first (Ctrl+S)"
			break
		}
		if id == "undo" {
			err = a.Document.Undo()
		} else {
			err = a.Document.Redo()
		}
	case "refresh-files":
		err = a.Document.Refresh()
		a.disposeCanvasResources()
		a.viewport3D.dispose()
	case "play":
		err = a.play()
		if err != nil {
			a.runOutput = err.Error()
			a.showWorkspaceDialog("run-failure")
		}
	case "stop":
		if a.runCmd != nil {
			err = stopRunCommand(a.runCmd)
		}
		a.runStopping = err == nil && a.runDone != nil
	case "toggle-dimension":
		a.cancelPlacementDrag()
		a.viewport2D.panning = false
		if a.dimension == previewDimension3D {
			a.dimension = previewDimension2D
		} else {
			a.dimension = previewDimension3D
		}
	case "focus":
		a.zoom = 1
		a.viewport2D.offset = [2]float32{}
		a.viewport2D.panning = false
		a.viewport2D.focusPending = true
		a.focusViewport3D()
	default:
		if handled, contentErr := a.contentAction(id); handled {
			err = contentErr
		}
		if strings.HasPrefix(id, "remove:") {
			err = a.removeField(strings.TrimPrefix(id, "remove:"))
		}
		if strings.HasPrefix(id, "add:") {
			err = a.addField(strings.TrimPrefix(id, "add:"))
		}
	}
	if a.Selected != "" && a.Document.Instance(a.Selected) == nil {
		a.Selected = ""
	}
	a.report(err)
}

func (a *ModuleEditor) openRequested(id string) error {
	if id == "new" {
		if _, err := CreateModuleProject(a.pathDraft, project.CreateProjectOptions{Title: a.projectName, Width: a.projectWidth, Height: a.projectHeight}); err != nil {
			return err
		}
	}
	return a.open(a.pathDraft)
}

func (a *ModuleEditor) selectedMod() project.ModID {
	if a.Local != "" && a.editingModule.Scene == "" {
		return a.Local
	}
	if a.Document != nil {
		if instance := a.Document.Instance(a.Selected); instance != nil {
			return instance.Mod
		}
	}
	return a.Local
}

func (a *ModuleEditor) closeRequested() bool {
	if a.buildDone != nil {
		a.status = "Building project. Wait for the build to finish"
		a.changed()
		return false
	}
	if a.closeConfirmed {
		return true
	}
	if a.runDone != nil {
		a.status = "Stop Play first"
		a.changed()
		return false
	}
	if a.hasDrafts() || a.Document != nil && a.Document.Dirty() {
		a.pendingAction = "close"
		a.changed()
		return false
	}
	return true
}

// Play compiles the authored entry with a resource-only preprocessing overlay.
func moduleProjectBuildCommand(directory, executable string, progress func(BuildProgress)) (*exec.Cmd, DevelopmentBuild, error) {
	build, err := PrepareDevelopmentBuild(directory, progress)
	if err != nil {
		return nil, DevelopmentBuild{}, err
	}
	args, env, err := cachedProjectBuildCommand()
	if err != nil {
		_ = build.Cleanup()
		return nil, DevelopmentBuild{}, err
	}
	args = append(args, "-v", "-mod=mod", "-overlay", build.OverlayPath, "-o", executable, ".")
	cmd := exec.Command("go", args...)
	cmd.Dir, cmd.Env = directory, env
	return cmd, build, nil
}

func (a *ModuleEditor) play() error {
	if a.Document == nil || a.runDone != nil || a.buildDone != nil {
		return fmt.Errorf("Open a project and wait for the current build or run to finish")
	}
	if err := a.applyDrafts(); err != nil {
		return err
	}
	if err := a.Document.Save(); err != nil {
		return err
	}
	directory, err := os.MkdirTemp("", "onix-mod-play-*")
	if err != nil {
		return err
	}
	executable := filepath.Join(directory, "game")
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	a.runDirectory, a.runExecutable, a.runBuilding = directory, executable, true
	a.runDone = make(chan runProcessResult, 1)
	a.runPrepared = make(chan preparedRun, 1)
	a.buildProgress = newBuildProgressFeed(a.requestRender)
	a.buildState = BuildProgress{Message: "Preparing Play...", Indeterminate: true}
	a.buildLog = ""
	projectDir, prepared, progress, wake := a.Document.ProjectDir, a.runPrepared, a.buildProgress.report, a.requestRender
	a.changed()
	go func() {
		cmd, build, err := moduleProjectBuildCommand(projectDir, executable, progress)
		prepared <- preparedRun{cmd: cmd, build: build, err: err}
		if wake != nil {
			wake()
		}
	}()
	return nil
}

func (a *ModuleEditor) startRun(cmd *exec.Cmd) error {
	var stdout, stderr runOutputCapture
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	var compilerOutput *commandProgressWriter
	if a.runBuilding && a.buildProgress != nil {
		reportBuildActivity(a.buildProgress.report, "Compiling project main.go and module code...", .65)
		compilerOutput = &commandProgressWriter{destination: &stderr, progress: a.buildProgress.report, stage: "Compiling", value: .65}
		cmd.Stdout, cmd.Stderr = compilerOutput, compilerOutput
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	a.runCmd, a.runDone = cmd, make(chan runProcessResult, 1)
	done, request := a.runDone, a.requestRender
	go func() {
		err := cmd.Wait()
		if compilerOutput != nil {
			compilerOutput.flush()
		}
		code := -1
		if cmd.ProcessState != nil {
			code = cmd.ProcessState.ExitCode()
		}
		done <- runProcessResult{err: err, exitCode: code, stdout: stdout.String(), stderr: stderr.String()}
		if request != nil {
			request()
		}
	}()
	return nil
}

func (a *ModuleEditor) updateRun() {
	if a.runDone == nil {
		return
	}
	if a.runBuilding {
		a.updateBuildProgress()
	}
	if a.runPrepared != nil {
		select {
		case prepared := <-a.runPrepared:
			a.runPrepared = nil
			a.runResources = prepared.build
			if prepared.err == nil && !a.runStopping {
				prepared.err = a.startRun(prepared.cmd)
				if prepared.err == nil {
					return
				}
			}
			a.runDone <- runProcessResult{err: prepared.err}
		default:
			return
		}
	}
	select {
	case result := <-a.runDone:
		a.updateBuildProgress()
		a.runCmd, a.runDone = nil, nil
		if a.runBuilding && result.err == nil && !a.runStopping {
			a.buildProgress = nil
			a.runBuilding = false
			cmd := exec.Command(a.runExecutable)
			cmd.Dir = a.Document.ProjectDir
			cmd.Env = append(os.Environ(), "ONIX_RESOURCE_DIR="+a.runResources.ResourceDir)
			if err := a.startRun(cmd); err == nil {
				a.status = "Play is running module code"
				a.changed()
				return
			} else {
				result.err = err
			}
		}
		a.buildProgress = nil
		a.status = "Play finished"
		if result.err != nil && !a.runStopping {
			a.status = fmt.Sprintf("Play: %v\n%s\n%s", result.err, result.stderr, result.stdout)
			a.runOutput = a.buildLog + "\n" + a.status
			a.showWorkspaceDialog("run-failure")
		}
		// Only the two explicitly created files/directories are removed.
		_ = os.Remove(a.runExecutable)
		_ = os.Remove(a.runDirectory)
		if err := a.runResources.Cleanup(); err != nil {
			a.status += "\n" + err.Error()
		}
		a.runResources = DevelopmentBuild{}
		a.runDirectory, a.runExecutable, a.runStopping, a.runBuilding = "", "", false, false
		a.changed()
	default:
	}
}

func (a *ModuleEditor) openFile(file string) {
	if a.Document == nil {
		return
	}
	double := a.lastFile == file && a.time-a.lastFileClick < .5
	a.lastFile, a.lastFileClick = file, a.time
	for _, entry := range a.files {
		if entry.Path == file && entry.IsDir {
			if double {
				a.fileBrowser.Directory = file
				a.lastFile = ""
			}
			a.status = file + " · Double-click to open folder"
			a.changed()
			return
		}
	}
	if strings.HasSuffix(file, "/mod.json") {
		id := project.ModID(strings.TrimSuffix(file, "/mod.json"))
		if double {
			if a.hasDrafts() {
				a.pendingAction = "local:" + string(id)
			} else {
				a.Local = id
			}
			a.changed()
		}
		a.status = file + " · Double-click to preview the module"
	} else {
		a.status = a.Document.Absolute(file)
	}
	a.changed()
}

func (a *ModuleEditor) sync(context *ecs.SystemContext) error {
	w := context.World
	a.world = w
	width, height := float32(defaultViewportWidth), float32(defaultViewportHeight)
	if c := graphics.CurrentContext(); c != nil {
		width, height = c.Size()
	}
	if width != a.width || height != a.height {
		a.width, a.height, a.dirty = width, height, true
	}
	if !a.dirty {
		if a.progressDirty {
			a.progressDirty = false
			if err := a.syncWorkspaceDialogs(w); err != nil {
				return err
			}
		}
		if a.canvasDirty {
			a.canvasDirty = false
			return a.syncCanvas(w)
		}
		return nil
	}
	a.dirty, a.canvasDirty, a.progressDirty = false, false, false
	if a.Document != nil {
		a.syncDraftPreview()
		a.Preview.Sync(a.previewDocument(), a.Local)
	}
	a.captureOutlinerScroll(w)
	if err := a.syncWorkspaceComponent(w); err != nil {
		return err
	}
	a.syncControlValues()
	if err := a.syncCanvas(w); err != nil {
		return err
	}
	if err := a.syncWorkspaceDialogs(w); err != nil {
		return err
	}
	layout := a.layout()
	return a.titlebar.syncView(w, editorTitlebarView{font: a.font, root: layout.Root, top: layout.TopBar,
		menu: a.menuOpen, submenu: a.submenu, recent: a.settings.RecentProjects,
		modal: a.modalVisible(), customControls: runtime.GOOS != "darwin" && runtime.GOOS != "android" && runtime.GOOS != "ios" && runtime.GOOS != "js",
		project: a.Document != nil, dirty: a.hasDrafts() || a.Document != nil && a.Document.Dirty(), running: a.runDone != nil, snap: a.settings.SnapToGrid, dimension: a.dimension})
}

func (a *ModuleEditor) syncControlValues() {
	for entity, access := range ecs.Query[htmlui.Control](a.world).All {
		control := access.Ref()
		if control.Owner != a.shell {
			continue
		}
		value, ok := a.controlValues[control.ID]
		if !ok {
			continue
		}
		if control.Type == "checkbox" {
			checked := value == "true"
			if control.Checked != checked {
				htmlui.SetControlChecked(a.world, entity, checked)
			}
		} else if control.Value != value {
			htmlui.SetControlValue(a.world, entity, value)
		}
	}
}

func spawnWorkspaceSurface(w *ecs.World, source, css string, font *onixtext.Font, bounds rect, stack int) ecs.Entity {
	value := htmlui.New(source)
	value.CSS, value.Font, value.FontSize = css, font, 13
	value.Width, value.Height, value.StackIndex = bounds.W, bounds.H, stack
	bundle := ecs.NewBundle(5)
	ecs.Put(&bundle, scene.UITransformFromXY(bounds.X, bounds.Y))
	ecs.Put(&bundle, scene.UIGlobalTransform{})
	ecs.Put(&bundle, scene.AnchorTopLeft)
	ecs.Put(&bundle, scene.Visibility{Visible: true})
	ecs.Put(&bundle, value)
	return w.Spawn(bundle)
}

func encodeFormJSON(value any) string { data, _ := json.Marshal(value); return string(data) }

func (a *ModuleEditor) dispose() {
	a.disposeCanvasResources()
	a.viewport3D.dispose()
	a.disposeWorkspacePopups()
	_ = a.Document.Dispose()
	a.draftPreview = workspaceDraftPreview{}
}

func registerEditorUIRenderers2D(world *ecs.World) {
	scene.RegisterSpriteRenderer2D(world)
	scene.RegisterSpriteLayersRenderer2D(world)
	scene.RegisterTextRenderer2D(world)
	scene.RegisterUIRenderer2D(world)
}

func editorWindowOptions() window.Options {
	options := window.DefaultOptions()
	options.Decorations = window.DecorationNone
	options.Border = true
	options.TitleBarHeight = float64(editorTopBarHeight + editorPanelGap*2)
	return options
}
