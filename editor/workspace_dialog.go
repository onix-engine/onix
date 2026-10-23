package editor

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/diiyw/onix/ecs"
	editorresources "github.com/diiyw/onix/editor/resources"
	"github.com/diiyw/onix/project"
	htmlui "github.com/diiyw/onix/ui/html"
	"github.com/diiyw/onix/window"
)

type workspaceDialogField struct{ Name, Kind, Value string }

const projectWindowIconField = "project.window.icon"

// editorDialogBrowseButtonWidth is the picker button appended to a path field.
const editorDialogBrowseButtonWidth = float32(34)

var (
	buildIconFilter  = window.FileDialogFilter{Name: "Icon", Extensions: []string{"png", "svg", "jpg", "jpeg", "gif", "webp"}}
	windowIconFilter = window.FileDialogFilter{Name: "Icon", Extensions: []string{"png", "svg"}}
)

// dialogPathPicker describes a dialog field that holds a file or folder path.
type dialogPathPicker struct {
	title   string
	folder  bool
	filters []window.FileDialogFilter
	// underResources authors the value relative to the project's resources
	// directory, which is how the packaged runtime reads the window icon.
	underResources bool
}

var dialogPathPickers = map[string]dialogPathPicker{
	buildIconField:         {title: "App Icon", filters: []window.FileDialogFilter{buildIconFilter}},
	buildAndroidSDKField:   {title: "Android SDK", folder: true},
	buildAndroidNDKField:   {title: "Android NDK", folder: true},
	projectWindowIconField: {title: "App Icon", filters: []window.FileDialogFilter{windowIconFilter}, underResources: true},
}

var workspaceDialogIDs = regexp.MustCompile(`\bid="([^"]+)"`)
var workspaceDialogButtons = regexp.MustCompile(`<button\s+id="([^"]+)"`)

var editorEngineVersionLabel = sync.OnceValue(func() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "Engine Version: (devel)"
	}
	version := info.Main.Version
	for _, dependency := range info.Deps {
		if dependency.Path == "github.com/diiyw/onix" {
			version = dependency.Version
			if dependency.Replace != nil {
				version = dependency.Replace.Version
			}
			break
		}
	}
	if version == "" || version == "(devel)" {
		version = "(devel)"
		for _, setting := range info.Settings {
			if info.Main.Path == "github.com/diiyw/onix" && setting.Key == "vcs.revision" {
				version += " " + setting.Value[:min(12, len(setting.Value))]
				break
			}
		}
	}
	return "Engine Version: " + version
})

func (a *ModuleEditor) activeWorkspaceDialog() string {
	if a.pendingAction != "" {
		return "close-confirm"
	}
	if a.buildDone != nil || a.runBuilding {
		return "progress"
	}
	if a.buildOpen {
		return "build"
	}
	return a.dialog
}

func (a *ModuleEditor) dialogPanel(width, height float32) rect {
	width, height = min(width, a.width-48), min(height, a.height-48)
	return rect{X: (a.width - width) / 2, Y: (a.height - height) / 2, W: width, H: height}
}

func (a *ModuleEditor) syncWorkspaceDialogs(world *ecs.World) error {
	u := &a.dialogUI
	u.init()
	u.font, u.rootRect = a.font, a.layout().Root
	clear(u.active)
	name := a.activeWorkspaceDialog()
	if name == "welcome" {
		a.layoutWorkspaceWelcome(world)
		u.removeInactive(world)
		return nil
	}
	if name != "" {
		panel := a.dialogPanel(600, 300)
		switch name {
		case "about":
			panel = a.dialogPanel(420, 150)
		case "project-dialog":
			panel = a.dialogPanel(650, 340)
		case "scene-create", "module-create":
			panel = a.dialogPanel(600, 340)
		case "folder-create":
			panel = a.dialogPanel(480, 260)
		case "instance-duplicate":
			panel = a.dialogPanel(480, 300)
		case "close-confirm":
			panel = a.dialogPanel(650, 200)
		case "progress":
			panel = a.dialogPanel(760, 460)
		case "build":
			panel = a.dialogPanel(800, 650)
		case "project", "preferences":
			panel = newSettingsDialogLayout[uint8](a.layout().Root, "Save").panel
		case "build-error", "run-failure":
			panel = a.dialogPanel(780, 480)
		case "color-picker":
			panel = a.workspaceColorPickerRect()
		case "nine-slice":
			panel = a.dialogPanel(860, 720)
		}
		u.htmlDialog(world, name, panel, 82)
		frame := u.htmlDialogFrames[name]
		if len(frame.Elements) == 0 {
			// Keep every authored element retained, including inactive categories.
			// Button captions and icons are controlled by their button descriptor.
			children := make(map[string]bool)
			for _, match := range workspaceDialogButtons.FindAllStringSubmatch(editorresources.DialogHTML(name), -1) {
				children[match[1]+"-icon"], children[match[1]+"-label"] = true, true
			}
			for _, match := range workspaceDialogIDs.FindAllStringSubmatch(editorresources.DialogHTML(name), -1) {
				id := match[1]
				if id != "dialog-root" && id != "overlay" && id != "panel" && id != "content" && !children[id] {
					frame.element(id)
				}
			}
			frame.reset(u.rootRect, panel)
		}
		switch name {
		case "color-picker":
			a.layoutWorkspaceColorPicker(world, panel)
		case "nine-slice":
			a.layoutWorkspaceNineSlice(world, panel)
		case "about":
			registerEditorHTMLDialogTexture(world, "about-logo", editorresources.LogoTexture(editorresources.Logo64))
			a.dialogElement(name, "about-logo", 24, 44, 64, 64).Source = "onix-dialog/about-logo"
			a.dialogLabel(name, "about-version", editorEngineVersionLabel(), 104, 44, panel.W-128, 28)
			a.dialogElement(name, "about-author", 104, 88, panel.W-128, 28)
			a.dialogClose(name)
		case "project-dialog":
			a.layoutWorkspaceProjectDialog(panel)
		case "scene-create", "module-create":
			a.layoutOutlinerCreateDialog(name, panel)
		case "folder-create":
			a.layoutFolderCreateDialog(panel)
		case "instance-duplicate":
			a.layoutInstanceDuplicateDialog(panel)
		case "preferences":
			a.layoutWorkspacePreferences()
		case "project":
			a.layoutWorkspaceProjectSettings()
		case "build":
			a.layoutWorkspaceBuild(panel)
		case "close-confirm":
			a.dialogLabel(name, "close-confirm-title", "Save changes before continuing?", 24, 22, panel.W-48, 28)
			a.dialogLabel(name, "close-confirm-body", "This project has unsaved changes.", 24, 62, panel.W-48, 28)
			w := (panel.W - 64) / 3
			a.dialogButton(name, "close-confirm-save", "Save and Continue", 24, panel.H-60, w, 34, false)
			a.dialogButton(name, "close-confirm-discard", "Don't Save", 32+w, panel.H-60, w, 34, false).Class = "danger"
			a.dialogButton(name, "close-confirm-cancel", "Cancel", 40+w*2, panel.H-60, w, 34, false)
		case "progress":
			title := "Building Project"
			if a.runBuilding {
				title = "Preparing Play"
			}
			a.dialogLabel(name, "progress-title", title, 24, 24, panel.W-48, 28)
			a.dialogLabel(name, "progress-message", a.buildState.Message, 24, 66, panel.W-48, 44).Style += ";overflow:hidden;overflow-wrap:anywhere"
			width := panel.W - 48
			bar := a.dialogElement(name, "progress-bar-bg", 24, 120, width, 8)
			bar.Style += ";background:#1b1e25"
			bar.Attrs, bar.RemoveAttrs = bar.Attrs[:0], bar.RemoveAttrs[:0]
			if a.buildState.Indeterminate {
				bar.RemoveAttrs = append(bar.RemoveAttrs, "aria-valuenow")
			} else {
				bar.Attrs = append(bar.Attrs, editorDialogAttr{Name: "aria-valuenow", Value: strconv.Itoa(int(a.buildState.Value * 100))})
			}
			a.dialogElement(name, "progress-bar", 24, 120, width*clamp(a.buildState.Value, 0, 1), 8).Style += ";background:#84ccff"
			elapsed := time.Duration(0)
			if a.buildProgress != nil {
				elapsed = time.Since(a.buildProgress.started).Truncate(time.Second)
			}
			detail := fmt.Sprintf("%d%% · Elapsed %s", int(a.buildState.Value*100), elapsed)
			if a.buildState.Indeterminate {
				detail = "Working · Elapsed " + elapsed.String()
			}
			a.dialogLabel(name, "progress-detail", detail, 24, 136, width, 24)
			e := a.dialogField(name, "log", "log", a.buildLog, 24, 172, width, max(32, panel.H-196))
			e.ReadOnly, e.ReadOnlySet = true, true
			e.FollowTail = true

		case "build-success":
			a.dialogLabel(name, "build-success-title", "Build Successful", 24, 22, panel.W-48, 28)
			a.dialogLabel(name, "build-success-body", "Project built successfully.", 24, 68, panel.W-48, 26)
			a.dialogLabel(name, "build-success-output-label", "Output", 24, 104, panel.W-48, 24)
			a.dialogLabel(name, "build-success-output", a.buildArtifact, 24, 130, panel.W-48, 60)
			a.dialogButton(name, "build-success-open-directory", "Open Directory", 24, panel.H-54, 170, 32, false)
			a.dialogButton(name, "build-success-ok", "OK", panel.W-100, panel.H-54, 76, 32, false)
			a.dialogClose(name)
		case "build-error", "run-failure":
			title, output := "Build Failed", a.buildError
			if name == "run-failure" {
				title, output = "Project Run Failed", a.runOutput
			}
			a.dialogLabel(name, name+"-title", title, 24, 22, panel.W-48, 28)
			if name == "run-failure" {
				e := a.dialogField(name, "output", "log", output, 24, 62, panel.W-48, panel.H-136)
				e.ReadOnly, e.ReadOnlySet = true, true
			} else {
				a.dialogLabel(name, "build-error-body", output, 24, 72, panel.W-48, panel.H-148).Style += ";overflow:auto;white-space:pre-wrap;pointer-events:auto"
			}
			a.dialogButton(name, name+"-copy", "Copy Output", 24, panel.H-54, 140, 32, false)
			if a.dialogCopied {
				a.dialogLabel(name, name+"-copied", "✓ Copied", 176, panel.H-48, 100, 28)
			}
			a.dialogButton(name, name+"-ok", "OK", panel.W-100, panel.H-54, 76, 32, false)
			a.dialogClose(name)
		}
		if a.dialogError != "" && name != "build-error" && name != "run-failure" && name != "progress" {
			errorY, errorHeight := panel.H-92, float32(32)
			if name == "color-picker" {
				errorY = 272
			} else if name == "nine-slice" {
				errorY, errorHeight = panel.H-68, 18
			}
			frame.dynamic(editorDialogDynamic{Key: editorDialogDynamicKey("workspace-dialog-error"), Tag: "span", Text: a.dialogError,
				Attrs: []editorDialogAttr{{Name: "id", Value: "workspace-dialog-error"}, {Name: "role", Value: "alert"}, {Name: "style", Value: fmt.Sprintf("position:absolute;left:24px;top:%gpx;width:%gpx;height:%gpx;color:#f38b9c;overflow:auto;pointer-events:auto", errorY, panel.W-48, errorHeight)}}})
		}
	}
	u.hideInactiveHTMLDialogNodes(world)
	u.removeInactive(world)
	return nil
}

func (a *ModuleEditor) dialogElement(dialog, id string, x, y, w, h float32) *editorDialogElement {
	e := a.dialogUI.htmlDialogFrames[dialog].element(id)
	e.Hidden, e.HiddenSet = false, true
	e.Style = fmt.Sprintf("position:absolute;left:%gpx;top:%gpx;width:%gpx;height:%gpx", x, y, w, h)
	return e
}

func (a *ModuleEditor) dialogLabel(dialog, id, value string, x, y, w, h float32) *editorDialogElement {
	e := a.dialogElement(dialog, id, x, y, w, h)
	e.Text, e.TextSet = value, true
	return e
}

func (a *ModuleEditor) dialogButton(dialog, id, label string, x, y, w, h float32, active bool) *editorDialogElement {
	e := a.dialogElement(dialog, id, x, y, w, h)
	if !e.Button {
		e.IconRef, e.LabelRef = id+"-icon", id+"-label"
	}
	e.Button, e.Authored, e.Text, e.TextSet = true, true, label, true
	e.Disabled, e.DisabledSet = false, true
	e.Class = ""
	if active {
		e.Class = "active"
	} else {
		e.Class = "dialog-button"
	}
	return e
}

func (a *ModuleEditor) dialogClose(dialog string) {
	panel := a.dialogUI.htmlDialogPanels[dialog]
	e := a.dialogButton(dialog, dialog+"-close", "", panel.W-40, 12, 28, 28, false)
	e.IconSource, e.Class, e.AriaLabel = "onix-titlebar/close.svg", "icon-only", "Close dialog"
}

func (a *ModuleEditor) dialogField(dialog, name, kind, value string, x, y, w, h float32) *editorDialogElement {
	id := htmlDialogFieldID(dialog, name)
	if draft, ok := a.dialogValues[id]; ok {
		value = draft
	}
	if a.dialogFields == nil {
		a.dialogFields = make(map[string]workspaceDialogField)
	}
	a.dialogFields[id] = workspaceDialogField{Name: name, Kind: kind, Value: value}
	if kind == "bool" {
		// The inline box overrides the .checkbox rule, so size it to the glyph and center it in the row.
		y += (h - editorDialogCheckboxSize) / 2
		w, h = editorDialogCheckboxSize, editorDialogCheckboxSize
	}
	e := a.dialogElement(dialog, id, x, y, w, h)
	e.Value, e.ValueSet = value, true
	if kind == "bool" {
		e.Checked, e.CheckedSet = value == "true", true
		e.ValueSet = false
	} else if kind == "number" {
		e.Type, e.Step = "number", "any"
	} else if kind == "textarea" {
		// A textarea's value is its text content, not a value attribute (see the engine's control sync).
		e.Text, e.TextSet, e.ValueSet = value, true, false
	}
	if kind == "log" {
		e.Text, e.TextSet, e.ValueSet = value, true, true
	}
	return e
}

func (a *ModuleEditor) dialogSelect(dialog, id, name, value string, options []string, x, y, w, h float32) {
	if draft, ok := a.dialogValues[id]; ok {
		value = draft
	}
	a.dialogFields[id] = workspaceDialogField{Name: name, Kind: "string", Value: value}
	a.dialogElement(dialog, id, x, y, w, h)
	a.dialogUI.htmlDialogFrames[dialog].selectElement(id, options, value)
}

// dialogPathField draws a path field with the native picker button flush against its right edge.
func (a *ModuleEditor) dialogPathField(dialog, name, kind, value string, x, y, w, h float32) {
	e := a.dialogField(dialog, name, kind, value, x, y, w-editorDialogBrowseButtonWidth, h)
	e.Class = "dialog-path-input"
	b := a.dialogButton(dialog, htmlDialogFieldID(dialog, name)+"-browse", "", x+w-editorDialogBrowseButtonWidth, y, editorDialogBrowseButtonWidth, h, false)
	b.IconSource, b.Class, b.AriaLabel = "onix-titlebar/folder.svg", "icon-only dialog-path-browse", "Choose "+dialogPathPickers[name].title
}

// pickDialogPath replaces one dialog path field with a native picker result.
func (a *ModuleEditor) pickDialogPath(id string) error {
	field, ok := a.dialogFields[id]
	if !ok {
		return nil
	}
	picker := dialogPathPickers[field.Name]
	base := ""
	if picker.underResources {
		if a.Document == nil {
			return nil
		}
		base = filepath.Join(a.Document.ProjectDir, "resources")
	}
	current := field.Value
	if base != "" && current != "" && !filepath.IsAbs(current) {
		current = filepath.Join(base, filepath.FromSlash(current))
	}
	if current != "" && !picker.folder {
		// A file reference opens beside its folder even after the file is gone.
		current = filepath.Dir(current)
	} else if current == "" {
		current = base
	}
	if current == "" && a.Document != nil {
		current = defaultFilePickerDirectory(a.settings, a.Document.ProjectDir)
	}
	value, err := a.chooseWorkspacePath(picker.folder, picker.title, current, picker.filters...)
	if err != nil || value == "" {
		return err
	}
	if base != "" {
		relative, err := filepath.Rel(base, value)
		if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("%s must be inside %s", picker.title, base)
		}
		value = filepath.ToSlash(relative)
	}
	if a.dialogValues == nil {
		a.dialogValues = make(map[string]string)
	}
	a.dialogValues[id] = value
	a.handleWorkspaceDialogValue(field.Name, value)
	a.changed()
	return nil
}

func (a *ModuleEditor) layoutWorkspaceProjectDialog(panel rect) {
	name, action, icon := "Open Project", "Open", "onix-titlebar/open.svg"
	if a.projectMode == "new" {
		name, action, icon = "New Project", "Create", "onix-titlebar/plus.svg"
	}
	a.dialogLabel("project-dialog", "project-dialog-title", name, 24, 22, panel.W-70, 28)
	if a.projectMode == "new" {
		a.dialogLabel("project-dialog", "project-dialog-body", "Choose a parent directory for the new project.", 24, 60, panel.W-48, 28)
		fields := []struct{ name, value, kind, label string }{{"name", a.projectName, "string", "Project Name"}, {"width", strconv.Itoa(a.projectWidth), "number", "Window Width"}, {"height", strconv.Itoa(a.projectHeight), "number", "Window Height"}}
		w := (panel.W - 64) / 3
		for i, field := range fields {
			x := 24 + float32(i)*(w+8)
			a.dialogLabel("project-dialog", "project-dialog-label-project-dialog."+field.name, field.label, x, 106, w, 22)
			a.dialogField("project-dialog", "project-dialog."+field.name, field.kind, field.value, x, 132, w, 32)
		}
	}
	a.dialogLabel("project-dialog", "project-dialog-path-label", "Project Directory", 24, 190, panel.W-48, 24)
	a.dialogLabel("project-dialog", "project-dialog-path", a.projectParent, 24, 222, panel.W-172, 32).Style += ";overflow:hidden"
	a.dialogButton("project-dialog", "project-dialog-path-browse", "Choose...", panel.W-140, 220, 116, 34, false)
	a.dialogButton("project-dialog", "project-dialog-confirm", action, panel.W-140, panel.H-54, 116, 32, false).IconSource = icon
	a.dialogClose("project-dialog")
}

func (a *ModuleEditor) dialogEvent(event htmlui.Event) bool {
	name := a.activeWorkspaceDialog()
	if name == "" || event.Entity != a.dialogUI.htmlDialogs[name] {
		return false
	}
	if event.Kind == htmlui.EventInput || event.Kind == htmlui.EventChange {
		if field, ok := a.dialogFields[event.ID]; ok {
			value := event.Value
			if field.Kind == "bool" {
				value = strconv.FormatBool(event.Checked)
			}
			if a.dialogValues == nil {
				a.dialogValues = make(map[string]string)
			}
			a.dialogValues[event.ID] = value
			a.handleWorkspaceDialogValue(field.Name, value)
		}
		return true
	}
	if event.Kind != htmlui.EventClick {
		return true
	}
	id := event.ID
	switch {
	case name == "close-confirm":
		switch id {
		case "close-confirm-save":
			a.action("save-pending")
		case "close-confirm-discard":
			a.action("discard-pending")
		case "close-confirm-cancel":
			a.action("cancel-pending")
		}
	case name == "progress":
	case name == "welcome":
		a.handleWorkspaceWelcome(id)
	case id == "color-picker-mode-hex" || id == "color-picker-mode-rgba":
		a.colorPicker.mode = strings.TrimPrefix(id, "color-picker-mode-")
		a.colorPicker.drag = ""
		clear(a.dialogValues)
		a.dialogError = ""
		a.changed()
	case id == "nine-slice-apply":
		a.report(a.applyWorkspaceNineSlice())
	case id == "nine-slice-cancel":
		a.closeWorkspaceDialog()
	case id == "scene-create-confirm" || id == "module-create-confirm":
		a.confirmOutlinerCreate(name)
	case id == "folder-create-confirm":
		a.confirmFolderCreate()
	case id == "instance-duplicate-confirm":
		a.confirmInstanceDuplicate()
	case strings.HasSuffix(id, "-close") || strings.HasSuffix(id, "-ok"):
		a.closeWorkspaceDialog()
	case id == "project-dialog-path-browse":
		value, err := a.chooseWorkspacePath(true, "Project Directory", a.projectParent)
		if value != "" && err == nil {
			a.projectParent = value
		}
		a.report(err)
	case strings.HasSuffix(id, "-browse"):
		a.report(a.pickDialogPath(strings.TrimSuffix(id, "-browse")))
	case id == "project-dialog-confirm":
		if a.projectMode == "new" {
			if strings.TrimSpace(a.projectName) == "" || filepath.Base(a.projectName) != a.projectName || a.projectName == "." || a.projectName == ".." || a.projectWidth <= 0 || a.projectHeight <= 0 {
				a.report(fmt.Errorf("Enter a valid project name and dimensions"))
				return true
			}
			a.pathDraft = filepath.Join(a.projectParent, a.projectName)
		} else {
			a.pathDraft = a.projectParent
		}
		mode := a.projectMode
		a.dialog = ""
		a.action(mode)
		if a.pendingAction == "" && a.Document != nil && !samePath(a.Document.ProjectDir, a.pathDraft) {
			a.dialog = "project-dialog"
		}
	case strings.HasPrefix(id, "preferences-category-"):
		a.preferencesCategory = preferencesEditing
		a.changed()
	case id == "preferences-save":
		if err := saveEditorSettings(a.preferencesDraft); err != nil {
			a.report(err)
		} else {
			a.settings = a.preferencesDraft
			a.closeWorkspaceDialog()
		}
	case strings.HasPrefix(id, "project-category-"):
		for _, item := range projectSettingsCategories {
			if id == "project-category-"+item.Key {
				a.projectCategory = item.Category
				a.changed()
				break
			}
		}
	case id == "project-save":
		a.report(a.saveWorkspaceProjectSettings())
	case id == "project-resource-packages-add":
		a.packageDirty = true
		a.packages.Packages = append(a.packages.Packages, resourcePackageRule{Name: fmt.Sprintf("package%d", len(a.packages.Packages)+1), Include: []string{"**"}})
		a.packageIndex = len(a.packages.Packages) - 1
		a.clearPackageDialogValues()
		a.changed()
	case id == "project-resource-packages-remove":
		a.packageDirty = true
		if a.packageIndex >= 0 && a.packageIndex < len(a.packages.Packages) {
			a.packages.Packages = slices.Delete(a.packages.Packages, a.packageIndex, a.packageIndex+1)
			a.packageIndex = min(a.packageIndex, len(a.packages.Packages)-1)
			a.clearPackageDialogValues()
			a.changed()
		}
	case name == "build":
		a.handleWorkspaceBuildAction(id)
	case id == "build-success-open-directory":
		a.report(openSystemDirectory(a.buildDirectory))
	case id == "build-error-copy":
		a.copyWorkspaceOutput(a.buildError)
	case id == "run-failure-copy":
		a.copyWorkspaceOutput(a.runOutput)
	}
	return true
}

func (a *ModuleEditor) handleWorkspaceDialogValue(name, value string) {
	if a.dialog == "color-picker" {
		a.report(a.setWorkspacePickerValue(name, value))
		return
	}
	if a.dialog == "nine-slice" {
		a.report(a.setWorkspaceNineSliceValue(name, value))
		return
	}
	switch name {
	case "scene-name":
		a.sceneDraft = value
		return
	case "module-name":
		a.modDraft = value
		return
	case "instance-name":
		a.instanceDraft = value
		return
	case "folder-name":
		a.folderDraft = value
		return
	}
	switch name {
	case "preferences.snapToGrid":
		a.preferencesDraft.SnapToGrid = value == "true"
	case "project-dialog.name":
		a.projectName = value
	case "project-dialog.width":
		a.projectWidth, _ = strconv.Atoi(value)
	case "project-dialog.height":
		a.projectHeight, _ = strconv.Atoi(value)
	default:
		if strings.HasPrefix(name, "project.engine.") {
			a.setWorkspaceEngineValue(name, value)
		}
		if strings.HasPrefix(name, "build.") {
			a.setWorkspaceBuildValue(name, value)
		}
		if strings.HasPrefix(name, "resource-packages.") {
			a.setWorkspacePackageValue(name, value)
		}
	}
}

func (a *ModuleEditor) clearPackageDialogValues() {
	for id, field := range a.dialogFields {
		if strings.HasPrefix(field.Name, "resource-packages.") {
			delete(a.dialogValues, id)
		}
	}
}

func (a *ModuleEditor) saveWorkspaceProjectSettings() error {
	if err := a.applyDrafts(); err != nil {
		return err
	}
	var patches []DocumentPatch
	for id, value := range a.dialogValues {
		field := a.dialogFields[id]
		if !strings.HasPrefix(field.Name, "project.") || strings.HasPrefix(field.Name, "project.engine.") {
			continue
		}
		raw := json.RawMessage(value)
		if field.Kind == "string" {
			raw, _ = json.Marshal(value)
		}
		if !json.Valid(raw) {
			return fmt.Errorf("%s: invalid value", field.Name)
		}
		patches = append(patches, DocumentPatch{Pointer: "/" + strings.ReplaceAll(strings.TrimPrefix(field.Name, "project."), ".", "/"), Value: raw})
	}
	if a.projectEngineDirty {
		raw, _ := json.Marshal(a.projectEngine)
		patches = append(patches, DocumentPatch{Pointer: "/engine", Value: raw})
	}
	data, err := applyDocumentPatches(a.Document.states["resources/project.json"].data, patches)
	if err != nil {
		return err
	}
	spec, err := decodeDocument[project.ProjectSpec](data)
	if err != nil {
		return err
	}
	if spec.Window.Width <= 0 || spec.Window.Height <= 0 || spec.Viewport.Width <= 0 || spec.Viewport.Height <= 0 {
		return fmt.Errorf("window and viewport dimensions must be positive")
	}
	command := workspaceCommand{Label: "Project settings", Edits: []documentEdit{{Target: "resources/project.json", After: data}}}
	if a.packageDirty {
		if err := validateResourcePackageConfig(a.packages); err != nil {
			return err
		}
		var raw []byte
		if len(a.packages.Packages) != 0 || len(a.packages.Keep) != 0 {
			raw, err = documentJSON(a.packages)
			if err != nil {
				return err
			}
		}
		command.Edits = append(command.Edits, documentEdit{Target: "resources/packages.json", After: raw})
	}
	if err := a.Document.commit(command); err != nil {
		return err
	}
	if err := a.Document.Save(); err != nil {
		return err
	}
	a.status = "Project settings saved"
	a.closeWorkspaceDialog()
	return nil
}
