package editor

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	htmlui "github.com/diiyw/onix/ui/html"
)

type workspaceModuleKey struct {
	Scene project.SceneID
	ID    string
}

type workspaceElementKey struct {
	workspaceModuleKey
	Element string
}

type workspaceElementBranch struct {
	Index, First, Last, Count int // indices are one-based; zero means absent
}

type workspaceElementTree struct {
	branches map[string]workspaceElementBranch
	next     []int
}

// reset links siblings in definition order with one pass, reusing the index.
func (tree *workspaceElementTree) reset(elements []project.ModElementSpec) {
	if tree.branches == nil {
		tree.branches = make(map[string]workspaceElementBranch, len(elements)+1)
	}
	clear(tree.branches)
	if cap(tree.next) < len(elements) {
		tree.next = make([]int, len(elements))
	} else {
		tree.next = tree.next[:len(elements)]
	}
	for i, element := range elements {
		tree.next[i] = 0
		branch := tree.branches[element.ID]
		branch.Index = i + 1
		tree.branches[element.ID] = branch
		parent := tree.branches[element.Parent]
		if parent.Last == 0 {
			parent.First = i + 1
		} else {
			tree.next[parent.Last-1] = i + 1
		}
		parent.Last = i + 1
		parent.Count++
		tree.branches[element.Parent] = parent
	}
}

type workspaceOutliner struct {
	collapsedScenes    map[project.SceneID]bool
	collapsedModules   map[workspaceModuleKey]bool
	expandedElements   map[workspaceElementKey]bool
	elementTreeIndex   workspaceElementTree
	editingModule      workspaceModuleKey // non-empty scene while the double-clicked module owns the tree
	lastRow            workspaceModuleKey
	lastRowTime        float64
	contextOpen        bool
	contextX, contextY float32
	contextScene       project.SceneID
	contextParent      string
	contextElement     string
	contextResource    string
	contextSlot        string
	contextMod         project.ModID
	fileMenuOpen       bool
	contextFile        string
	resourceMenuOpen   bool
	selectedElement    string // element row the Details panel edits
	selectedResource   string
	selectedSlot       string
	slotCreating       bool
	declarationNames   []string // reused while rendering each module's declarations
	scrollPosition     [2]float32
	scrollPending      bool
	resetScroll        bool
}

func (a *ModuleEditor) outlinerScroll(world *ecs.World) *scene.UIScroll {
	if a.Document == nil || len(a.Document.Spec.Scenes) == 0 {
		return nil
	}
	control, ok := htmlui.ControlByID(world, a.shell, "scene:"+url.PathEscape(string(a.Document.Spec.Scenes[0])))
	if !ok {
		return nil
	}
	content, ok := world.Get[htmlui.ScrollContent](control)
	if !ok {
		return nil
	}
	return world.GetMut[scene.UIScroll](content.Area)
}

func (a *ModuleEditor) captureOutlinerScroll(world *ecs.World) {
	if !a.resetScroll && !a.scrollPending {
		a.scrollPosition = [2]float32{}
		if scroll := a.outlinerScroll(world); scroll != nil {
			a.scrollPosition = scroll.Position
		}
	}
	a.resetScroll, a.scrollPending = false, true
}

func (a *ModuleEditor) restoreOutlinerScroll(context *ecs.SystemContext) error {
	if !a.scrollPending {
		return nil
	}
	a.scrollPending = false
	if scroll := a.outlinerScroll(context.World); scroll != nil {
		for axis := range 2 {
			scroll.Position[axis] = clamp(a.scrollPosition[axis], -max(0, scroll.ContentSize[axis]-scroll.ViewportSize[axis]), 0)
		}
	}
	return nil
}

func (a *ModuleEditor) treeRowControlID(row workspaceTreeRow) string {
	kind, name := "element", row.Element
	if row.Resource != "" {
		kind, name = "resource", row.Resource
	} else if row.Slot != "" {
		kind, name = "slot", row.Slot
	} else if row.Group != "" {
		kind, name = "tree-group", row.Group
	}
	if name != "" {
		return kind + ":" + url.PathEscape(string(row.Scene)) + "/" + url.PathEscape(row.ID) + "/" + url.PathEscape(name)
	}
	if row.Scene == a.Document.Active {
		return "select:" + row.ID
	}
	return "tree-select:" + url.PathEscape(string(row.Scene)) + "/" + url.PathEscape(row.ID)
}

// doubleClickRow mirrors the file browser's .5s window for tree rows.
func (a *ModuleEditor) doubleClickRow(scene project.SceneID, id string) bool {
	key := workspaceModuleKey{Scene: scene, ID: id}
	double := a.lastRow == key && a.time-a.lastRowTime < .5
	a.lastRow, a.lastRowTime = key, a.time
	return double
}

// enterModuleEdit gives the double-clicked instance its own editing space:
// the tree keeps only its subtree and elements while the canvas previews the module.
func (a *ModuleEditor) enterModuleEdit(scene project.SceneID, id string) {
	a.editingModule = workspaceModuleKey{Scene: scene, ID: id}
	a.selectScene(string(scene))
	a.selectInstance(id)
	delete(a.collapsedScenes, scene)
	a.expandModule(a.editingModule)
	a.zoom = 1
	a.viewport2D.offset, a.viewport2D.panning = [2]float32{}, false
	a.viewport2D.focusPending = true
	// Consume the double-click so the very next click toggles the row.
	a.lastRow = workspaceModuleKey{}
	a.contextOpen = false
	a.status = "Editing module " + id + " · Esc to exit"
	a.changed()
}

// exitModuleEdit leaves the exclusive module space and restores the scene tree.
func (a *ModuleEditor) exitModuleEdit() {
	a.editingModule = workspaceModuleKey{}
	a.Local = ""
	a.selectedElement, a.selectedResource = "", ""
	a.selectedSlot, a.slotCreating = "", false
	// Re-frame the scene the way entering module edit framed the module.
	a.zoom = 1
	a.viewport2D.offset, a.viewport2D.panning = [2]float32{}, false
	a.viewport2D.focusPending = true
	a.status = "Exited module editing"
	a.changed()
}

// editLocal pins the canvas module preview while the exclusive space is open.
func (a *ModuleEditor) editLocal() project.ModID {
	if a.editingModule.Scene == "" {
		return ""
	}
	if instance := a.Document.Instance(a.editingModule.ID); instance != nil {
		return instance.Mod
	}
	return ""
}

func (a *ModuleEditor) toggleModule(id string) {
	instance := a.Document.Instance(id)
	if instance == nil {
		return
	}
	// In module editing, even an empty definition owns the two content groups.
	if len(instance.Children) == 0 && a.editingModule.Scene == "" {
		return
	}
	if a.collapsedModules == nil {
		a.collapsedModules = make(map[workspaceModuleKey]bool)
	}
	key := workspaceModuleKey{Scene: a.Document.Active, ID: id}
	a.collapsedModules[key] = !a.moduleCollapsed(key)
}

func (a *ModuleEditor) moduleCollapsed(key workspaceModuleKey) bool {
	if collapsed, ok := a.collapsedModules[key]; ok {
		return collapsed
	}
	return true
}

func (a *ModuleEditor) expandModule(key workspaceModuleKey) {
	if key.ID == "" {
		return
	}
	if a.collapsedModules == nil {
		a.collapsedModules = make(map[workspaceModuleKey]bool)
	}
	a.collapsedModules[key] = false
}

func (a *ModuleEditor) toggleElement(key workspaceElementKey) {
	instance := a.Document.Instance(key.ID)
	if instance == nil {
		return
	}
	a.elementTreeIndex.reset(a.modElements(instance.Mod))
	if a.elementTreeIndex.branches[key.Element].Count == 0 {
		return
	}
	if a.expandedElements == nil {
		a.expandedElements = make(map[workspaceElementKey]bool)
	}
	a.expandedElements[key] = !a.expandedElements[key]
	a.changed()
}

func (a *ModuleEditor) revealElement(owner string, mod project.ModID, id string) {
	elements := a.modElements(mod)
	a.elementTreeIndex.reset(elements)
	key := workspaceElementKey{workspaceModuleKey: workspaceModuleKey{Scene: a.Document.Active, ID: owner}}
	for index := a.elementTreeIndex.branches[id].Index; index != 0; {
		key.Element = elements[index-1].Parent
		if key.Element == "" {
			break
		}
		if a.expandedElements == nil {
			a.expandedElements = make(map[workspaceElementKey]bool)
		}
		a.expandedElements[key] = true
		index = a.elementTreeIndex.branches[key.Element].Index
	}
}

func (a *ModuleEditor) updateOutlinerMenu(in *input.State) {
	if a.Document == nil || a.modalVisible() {
		return
	}
	p := in.Pointer()
	if a.canvasDrag.ID != "" && in.KeyPressed(input.KeyEscape) {
		a.cancelPlacementDrag()
		a.changed()
		return
	}
	if a.contextOpen || a.fileMenuOpen {
		insideMenu := a.contextOpen && a.outlinerContains("outliner-menu", p.X, p.Y)
		insideResources := a.resourceMenuOpen && a.outlinerContains("outliner-resource-menu", p.X, p.Y)
		insideFiles := a.fileMenuOpen && a.outlinerContains("file-menu", p.X, p.Y)
		if in.KeyPressed(input.KeyEscape) || in.MousePressed(input.MouseLeft) && !insideMenu && !insideResources && !insideFiles {
			a.contextOpen, a.fileMenuOpen = false, false
			a.changed()
		} else if insideMenu {
			open := a.contextMod != "" && a.editingModule.Scene != "" && a.outlinerContains("outliner-add-resources", p.X, p.Y)
			if open != a.resourceMenuOpen {
				a.resourceMenuOpen = open
				a.changed()
			}
		}
	} else if a.editingModule.Scene != "" && in.KeyPressed(input.KeyEscape) && !a.componentPickerOpen && a.componentMenu == "" && a.menuOpen == "" && a.elementPicker.Field == "" {
		a.exitModuleEdit()
	}
	if !in.MousePressed(input.MouseRight) {
		return
	}
	if a.outlinerContains("resource-panel", p.X, p.Y) {
		a.contextOpen, a.resourceMenuOpen = false, false
		a.contextFile = ""
		for _, file := range a.files {
			if a.outlinerContains("file:"+url.PathEscape(file.Path), p.X, p.Y) {
				a.contextFile = file.Path
				a.lastFile = file.Path
				break
			}
		}
		a.contextX, a.contextY, a.fileMenuOpen = p.X, p.Y, true
		a.componentPickerOpen, a.componentMenu = false, ""
		a.menuOpen, a.submenu, a.dragID = "", "", ""
		a.changed()
		return
	}
	if !a.outlinerContains("scene-panel", p.X, p.Y) {
		return
	}
	a.fileMenuOpen = false
	a.contextScene, a.contextParent, a.contextElement, a.contextResource, a.contextMod = a.Document.Active, "", "", "", ""
	a.contextSlot = ""
	for _, row := range a.rows {
		if a.outlinerContains(a.treeRowControlID(row), p.X, p.Y) {
			a.contextScene, a.contextParent = row.Scene, row.ID
			a.contextElement, a.contextMod = row.Element, row.Mod
			a.contextResource = row.Resource
			a.contextSlot = row.Slot
			break
		}
	}
	if a.contextParent == "" {
		for _, id := range a.Document.Spec.Scenes {
			if a.outlinerContains("scene:"+url.PathEscape(string(id)), p.X, p.Y) {
				a.contextScene = id
				break
			}
		}
	}
	// Blank space in the exclusive tree right-clicks the edited module itself.
	if a.contextParent == "" && a.editingModule.Scene != "" && a.Document.Active == a.editingModule.Scene {
		if instance := a.Document.Instance(a.editingModule.ID); instance != nil {
			a.contextScene, a.contextParent, a.contextMod = a.editingModule.Scene, a.editingModule.ID, instance.Mod
		}
	}
	a.contextX, a.contextY, a.contextOpen = p.X, p.Y, true
	a.componentPickerOpen, a.componentMenu = false, ""
	a.resourceMenuOpen = false
	a.menuOpen, a.submenu, a.dragID = "", "", ""
	a.changed()
}

func (a *ModuleEditor) outlinerContains(id string, x, y float32) bool {
	if node, ok := htmlui.NodeByID(a.world, a.shell, id); ok {
		return workspaceNodeContains(node, x, y)
	}
	return false
}

func (a *ModuleEditor) outlinerAction(action string) bool {
	if a.Document == nil {
		return false
	}
	switch {
	case action == "outliner-exit-module":
		if a.editingModule.Scene != "" {
			a.exitModuleEdit()
		}
		return true
	case action == "outliner-duplicate-module":
		if a.contextParent == "" || a.contextElement != "" || a.contextResource != "" || a.contextSlot != "" {
			return true
		}
		if err := a.applyDrafts(); err != nil {
			a.report(err)
			return true
		}
		a.contextOpen = false
		a.instanceDraft = ""
		a.showWorkspaceDialog("instance-duplicate")
		return true
	case action == "outliner-new-slot":
		if a.contextMod == "" || a.editingModule.Scene == "" {
			return true
		}
		if err := a.applyDrafts(); err != nil {
			a.report(err)
			return true
		}
		a.contextOpen = false
		a.selectInstance(a.contextParent)
		a.slotCreating = true
		delete(a.additions, "add/slot/name")
		delete(a.additions, "add/slot/value")
		a.expandModule(workspaceModuleKey{Scene: a.contextScene, ID: a.contextParent})
		delete(a.sectionsCollapsed, a.treeRowControlID(workspaceTreeRow{Scene: a.contextScene, ID: a.contextParent, Group: "slots"}))
		a.status = "Enter the new slot's name and configuration in Details"
		a.changed()
		return true
	case strings.HasPrefix(action, "tree-group:"):
		a.sectionsCollapsed[action] = !a.sectionsCollapsed[action]
		a.changed()
		return true
	case action == "outliner-add-resources":
		if a.contextMod != "" && a.editingModule.Scene != "" {
			a.resourceMenuOpen = true
			a.changed()
		}
		return true
	case strings.HasPrefix(action, "outliner-add-resource:"):
		if a.contextMod == "" || a.editingModule.Scene == "" {
			return true
		}
		if err := a.applyDrafts(); err != nil {
			a.report(err)
			return true
		}
		a.contextOpen = false
		name, err := a.pickModuleResource(a.contextMod, strings.TrimPrefix(action, "outliner-add-resource:"))
		if err == nil && name != "" {
			a.selectInstance(a.contextParent)
			a.selectedResource = name
			a.expandModule(workspaceModuleKey{Scene: a.contextScene, ID: a.contextParent})
			delete(a.sectionsCollapsed, a.treeRowControlID(workspaceTreeRow{Scene: a.contextScene, ID: a.contextParent, Group: "resources"}))
		}
		a.report(err)
		a.changed()
		return true
	case strings.HasPrefix(action, "element:") || strings.HasPrefix(action, "resource:") || strings.HasPrefix(action, "slot:"):
		a.contextOpen = false
		kind, value, _ := strings.Cut(action, ":")
		parts := strings.SplitN(value, "/", 3)
		if len(parts) != 3 {
			a.report(fmt.Errorf("invalid %s target", kind))
			return true
		}
		for i := range parts {
			value, err := url.PathUnescape(parts[i])
			if err != nil {
				a.report(err)
				return true
			}
			parts[i] = value
		}
		if kind == "element" && a.Selected == parts[1] && a.selectedElement == parts[2] {
			if a.doubleClickRow(project.SceneID(parts[0]), action) {
				a.openAnimation()
			} else {
				a.toggleElement(workspaceElementKey{workspaceModuleKey: workspaceModuleKey{Scene: project.SceneID(parts[0]), ID: parts[1]}, Element: parts[2]})
			}
			return true
		}
		if a.hasDrafts() {
			a.pendingAction = action
			a.changed()
			return true
		}
		if kind == "slot" {
			a.selectInstance(parts[1])
			a.selectedSlot = parts[2]
			a.status = "Module slot " + parts[2]
		} else if kind == "resource" {
			a.selectInstance(parts[1])
			a.selectedResource = parts[2]
			a.status = "Module resource " + parts[2]
		} else {
			double := a.doubleClickRow(project.SceneID(parts[0]), action)
			a.selectElement(parts[1], parts[2])
			if double {
				a.openAnimation()
			} else {
				a.toggleElement(workspaceElementKey{workspaceModuleKey: workspaceModuleKey{Scene: project.SceneID(parts[0]), ID: parts[1]}, Element: parts[2]})
			}
			a.status = "Module element " + parts[2]
		}
		a.changed()
		return true
	case strings.HasPrefix(action, "scene:") || strings.HasPrefix(action, "tree-select:"):
		if a.hasDrafts() {
			a.pendingAction = action
			a.changed()
			return true
		}
		if value, ok := strings.CutPrefix(action, "tree-select:"); ok {
			sceneID, instance, _ := strings.Cut(value, "/")
			sceneID, err := url.PathUnescape(sceneID)
			if err != nil {
				a.report(err)
				return true
			}
			instance, err = url.PathUnescape(instance)
			if err != nil {
				a.report(err)
				return true
			}
			a.selectScene(sceneID)
			a.selectInstance(instance)
			// Double-clicking the already-edited root just toggles it below.
			if a.doubleClickRow(project.SceneID(sceneID), instance) && a.editingModule != (workspaceModuleKey{Scene: project.SceneID(sceneID), ID: instance}) {
				a.enterModuleEdit(project.SceneID(sceneID), instance)
				return true
			}
			a.toggleModule(instance)
		} else {
			id, err := url.PathUnescape(strings.TrimPrefix(action, "scene:"))
			if err != nil {
				a.report(err)
				return true
			}
			collapse := a.Document.Active == project.SceneID(id) && a.Selected == "" && a.Local == "" && !a.collapsedScenes[project.SceneID(id)]
			a.selectScene(id)
			if collapse {
				if a.collapsedScenes == nil {
					a.collapsedScenes = make(map[project.SceneID]bool)
				}
				a.collapsedScenes[project.SceneID(id)] = true
			}
		}
		a.contextOpen = false
		return true
	case action == "outliner-new-scene" || action == "outliner-new-module":
		if err := a.applyDrafts(); err != nil {
			a.report(err)
			return true
		}
		a.contextOpen = false
		if action == "outliner-new-scene" {
			a.sceneDraft = ""
			a.showWorkspaceDialog("scene-create")
		} else {
			a.selectScene(string(a.contextScene))
			a.modDraft, a.instanceDraft = "", ""
			a.showWorkspaceDialog("module-create")
		}
		return true
	case action == "outliner-new-element", action == "outliner-delete-element", action == "outliner-delete-module", action == "outliner-delete-resource", action == "outliner-delete-slot":
		if err := a.applyDrafts(); err != nil {
			a.report(err)
			return true
		}
		a.contextOpen = false
		if a.Document.Active != a.contextScene {
			a.selectScene(string(a.contextScene))
		}
		var err error
		switch action {
		case "outliner-delete-slot":
			err = a.Document.EditMod(a.contextMod, "Delete module slot", func(spec *project.ModSpec) error {
				if _, ok := spec.Slots[a.contextSlot]; !ok {
					return fmt.Errorf("unknown slot %q", a.contextSlot)
				}
				delete(spec.Slots, a.contextSlot)
				return nil
			})
			if err == nil {
				if a.selectedMod() == a.contextMod && a.selectedSlot == a.contextSlot {
					a.selectedSlot = ""
				}
				a.status = "Deleted slot " + a.contextSlot + ". Press Ctrl+S to save"
			}
		case "outliner-delete-resource":
			err = a.Document.EditMod(a.contextMod, "Delete module resource", func(spec *project.ModSpec) error {
				if _, ok := spec.Resources[a.contextResource]; !ok {
					return fmt.Errorf("unknown resource %q", a.contextResource)
				}
				delete(spec.Resources, a.contextResource)
				return nil
			})
			if err == nil {
				if a.selectedMod() == a.contextMod && a.selectedResource == a.contextResource {
					a.selectedResource = ""
				}
				a.status = "Deleted resource " + a.contextResource + ". Press Ctrl+S to save"
			}
		case "outliner-delete-module":
			err = a.Document.DeleteInstance(a.contextParent)
			if err == nil && a.isSelected(a.contextParent) {
				a.selectInstance("")
			}
			// Deleting the edited root closes its exclusive space.
			if err == nil && a.editingModule.Scene == a.contextScene && a.editingModule.ID == a.contextParent {
				a.editingModule = workspaceModuleKey{}
				a.Local = ""
			}
		case "outliner-new-element":
			if a.contextMod == "" {
				break
			}
			var id string
			if id, err = a.Document.AddElement(a.contextMod); err == nil {
				a.expandModule(workspaceModuleKey{Scene: a.contextScene, ID: a.contextParent})
				a.status = "Added element " + id + ". Press Ctrl+S to save"
			}
		case "outliner-delete-element":
			err = a.Document.RemoveElement(a.contextMod, a.contextElement)
			if err == nil {
				a.status = "Deleted element " + a.contextElement + ". Press Ctrl+S to save"
			}
		}
		a.report(err)
		a.changed()
		return true
	}
	return false
}

func (a *ModuleEditor) layoutOutlinerCreateDialog(name string, panel rect) {
	title, body := "Create Scene", "Scene JSON is saved to "+project.SceneDirectory
	if name == "module-create" {
		title, body = "Create Scene Module", "Scene: "+string(a.contextScene)
		if a.contextParent != "" {
			title, body = "Create Child Module", body+" · Parent module: "+a.contextParent
		}
	}
	a.dialogLabel(name, name+"-title", title, 24, 22, panel.W-48, 28)
	a.dialogLabel(name, name+"-body", body, 24, 62, panel.W-48, 28)
	if name == "scene-create" {
		a.dialogLabel(name, "scene-create-label", "Scene name (e.g. game)", 24, 104, panel.W-48, 24)
		a.dialogField(name, "scene-name", "text", a.sceneDraft, 24, 132, panel.W-48, 32)
	} else {
		a.dialogLabel(name, "module-create-definition-label", "Module directory (reuse existing or create new)", 24, 104, panel.W-48, 24)
		a.dialogField(name, "module-name", "text", a.modDraft, 24, 132, panel.W-48, 32)
		a.dialogLabel(name, "module-create-instance-label", "Instance ID", 24, 174, panel.W-48, 24)
		a.dialogField(name, "instance-name", "text", a.instanceDraft, 24, 202, panel.W-48, 32)
	}
	a.dialogButton(name, name+"-confirm", "Create", panel.W-140, panel.H-54, 116, 32, false)
	a.dialogClose(name)
}

func (a *ModuleEditor) layoutInstanceDuplicateDialog(panel rect) {
	const name = "instance-duplicate"
	a.dialogLabel(name, name+"-title", "Duplicate Module", 24, 22, panel.W-48, 28)
	a.dialogLabel(name, name+"-body", "Scene: "+string(a.contextScene)+" · Module: "+a.contextParent, 24, 62, panel.W-48, 28)
	a.dialogLabel(name, name+"-label", "Copy ID", 24, 104, panel.W-48, 24)
	a.dialogField(name, "instance-name", "text", a.instanceDraft, 24, 132, panel.W-48, 32)
	a.dialogButton(name, name+"-confirm", "Duplicate", panel.W-140, panel.H-54, 116, 32, false)
	a.dialogClose(name)
}

func (a *ModuleEditor) confirmInstanceDuplicate() {
	if a.Document.Active != a.contextScene {
		a.selectScene(string(a.contextScene))
	}
	id := strings.TrimSpace(a.instanceDraft)
	if err := a.Document.DuplicateInstance(a.contextParent, id); err != nil {
		a.report(err)
		return
	}
	if a.editingModule == (workspaceModuleKey{Scene: a.contextScene, ID: a.contextParent}) {
		a.editingModule.ID = id
	}
	delete(a.collapsedScenes, a.contextScene)
	a.selectInstance(id)
	a.closeWorkspaceDialog()
	a.status = "Duplicated module " + id + ". Press Ctrl+S to save"
}

func (a *ModuleEditor) confirmOutlinerCreate(name string) {
	var err error
	if name == "scene-create" {
		id := strings.TrimSpace(a.sceneDraft)
		if !strings.HasSuffix(id, ".json") {
			id += ".json"
		}
		if id == ".json" {
			a.report(fmt.Errorf("Enter a scene name"))
			return
		}
		err = a.Document.CreateScene(project.SceneID(id))
		if err == nil {
			a.selectScene(id)
		}
	} else {
		err = a.Document.CreateSceneModule(strings.TrimSpace(a.instanceDraft), project.ModID(strings.TrimSpace(a.modDraft)), a.contextParent)
		if err == nil {
			delete(a.collapsedScenes, a.Document.Active)
			a.expandModule(workspaceModuleKey{Scene: a.Document.Active, ID: a.contextParent})
			a.selectInstance(strings.TrimSpace(a.instanceDraft))
		}
	}
	if err != nil {
		a.report(err)
		return
	}
	a.closeWorkspaceDialog()
	a.status = "Created. Press Ctrl+S to save"
}
