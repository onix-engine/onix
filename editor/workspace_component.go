package editor

import (
	"fmt"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/diiyw/onix/ecs"
	editorresources "github.com/diiyw/onix/editor/resources"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/ui/html"
)

type workspaceComponentProps struct {
	Editor *ModuleEditor
	Fields []html.VNode
}
type workspaceComponentState struct{ Inspector, Picker, Animation, AnimationCurve editorVNodeView }
type workspaceListState struct{ View editorVNodeView }

type workspaceFileTile struct {
	File            WorkspaceFile
	Label, ID, Icon string
}

type workspaceFilesState struct {
	View      editorVNodeView
	Document  *WorkspaceDocument
	Mod       project.ModID
	Revision  uint64
	Inventory []WorkspaceFile
	AllFiles  []WorkspaceFile
	Directory string
	Files     []WorkspaceFile
	Tiles     []workspaceFileTile
}

func (state *workspaceFilesState) refresh(document *WorkspaceDocument, mod project.ModID, directory string) {
	inventory := document.Files
	sameInventory := len(inventory) == len(state.Inventory) && (len(inventory) == 0 || &inventory[0] == &state.Inventory[0])
	sameFiles := state.Document == document && state.Mod == mod && state.Revision == document.Revision && sameInventory
	if sameFiles {
		if state.Directory == directory {
			return
		}
	} else {
		state.AllFiles = document.ModuleFiles(mod)
	}
	state.Document, state.Mod, state.Revision, state.Inventory = document, mod, document.Revision, inventory
	state.Directory = directory
	parent := directory
	if parent == "" {
		parent = "."
	}
	clear(state.Files)
	state.Files = state.Files[:0]
	for _, file := range state.AllFiles {
		if path.Dir(file.Path) != parent {
			continue
		}
		index := len(state.Files)
		state.Files = append(state.Files, file)
		if index == len(state.Tiles) {
			state.Tiles = append(state.Tiles, workspaceFileTile{})
		}
		tile := &state.Tiles[index]
		tile.File = file
		tile.Label = path.Base(file.Path)
		tile.ID = "file:" + url.PathEscape(file.Path)
		tile.Icon = "onix-file-icons/" + editorresources.FileIconName(file.Path, file.IsDir)
	}
	clear(state.Tiles[len(state.Files):])
	state.Tiles = state.Tiles[:len(state.Files)]
}

var workspaceDefinitions struct {
	once              sync.Once
	shell             *html.Definition[workspaceComponentProps, workspaceComponentState]
	scenes, inspector *html.Definition[workspaceComponentProps, workspaceListState]
	files             *html.Definition[workspaceComponentProps, workspaceFilesState]
	err               error
}

func editorWorkspaceDefinition() (*html.Definition[workspaceComponentProps, workspaceComponentState], error) {
	d := &workspaceDefinitions
	d.once.Do(func() {
		d.scenes, d.err = html.Define(html.NewTemplate(editorresources.WorkspaceScenesHTML), "", html.Behavior[workspaceComponentProps, workspaceListState]{Render: renderWorkspaceScenes})
		if d.err != nil {
			return
		}
		d.files, d.err = html.Define(html.NewTemplate(editorresources.WorkspaceFilesHTML), "", html.Behavior[workspaceComponentProps, workspaceFilesState]{Render: renderWorkspaceFiles})
		if d.err != nil {
			return
		}
		d.inspector, d.err = html.Define(html.NewTemplate(editorresources.WorkspaceInspectorHTML), "", html.Behavior[workspaceComponentProps, workspaceListState]{Render: renderWorkspaceInspector})
		if d.err != nil {
			return
		}
		d.shell, d.err = html.Define(html.NewTemplate(editorresources.WorkspaceHTML), "", html.Behavior[workspaceComponentProps, workspaceComponentState]{Render: renderWorkspaceShell})
	})
	return d.shell, d.err
}

func componentHidden(view *html.View, ref string, hidden bool) {
	if hidden {
		view.Attr(ref, "hidden", "")
	} else {
		view.RemoveAttr(ref, "hidden")
	}
}

func renderWorkspaceShell(context html.RenderContext[workspaceComponentProps, workspaceComponentState], view *html.View) {
	a := context.Props.Editor
	state := context.State
	state.Inspector.reset()
	state.Picker.reset()
	state.Animation.reset()
	state.AnimationCurve.reset()
	layout := a.layout()
	view.Attr("scenes", "style", workspaceRectStyle(layout.Files))
	view.Attr("resources", "style", workspaceRectStyle(layout.Content))
	view.Attr("inspector", "style", workspaceRectStyle(layout.Inspector))
	view.Attr("viewport", "style", workspaceRectStyle(layout.Viewport))
	view.Attr("welcome", "style", workspaceRectStyle(layout.Viewport))
	edge := a.panelResizeHover
	if a.panelResizeMode != "" {
		edge = a.panelResizeMode
	}
	resizeEdgeClass := func(class, id, edgeClass string) string {
		if edge == id {
			return class + " " + edgeClass
		}
		return class
	}
	view.Attr("scenes", "class", resizeEdgeClass("outliner", panelResizeLeftWidth, "edge-resize-right"))
	view.Attr("inspector", "class", resizeEdgeClass("panel inspector", panelResizeRightWidth, "edge-resize-left"))
	view.Attr("resources", "class", resizeEdgeClass("content-browser", panelResizeContentHeight, "edge-resize-top"))
	open := a.Document != nil
	animationAvailable := false
	viewportClass := "viewport-panel"
	if !open {
		viewportClass = "viewport-panel empty"
	}
	view.Attr("viewport", "class", viewportClass)
	for _, ref := range [...]string{"scene-empty", "file-empty", "detail-empty", "welcome"} {
		componentHidden(view, ref, open)
	}
	if open {
		a.resourceDirectory()
		a.inspector(&state.Inspector, &state.Picker)
		a.renderElementPicker(&state.Picker)
		animationAvailable = a.renderAnimation(&state.Animation)
		a.renderAnimationCurvePopup(&state.AnimationCurve)
		props := workspaceComponentProps{Editor: a, Fields: state.Inspector.nodes()}
		scenes := [...]html.ComponentVNode{workspaceDefinitions.scenes.ComponentChild(1, props, workspaceListState{})}
		files := [...]html.ComponentVNode{workspaceDefinitions.files.ComponentChild(2, props, workspaceFilesState{})}
		inspector := [...]html.ComponentVNode{workspaceDefinitions.inspector.ComponentChild(3, props, workspaceListState{})}
		view.Components("scene-content", scenes[:])
		view.Components("file-content", files[:])
		view.Components("detail-content", inspector[:])
	}
	animationActive := animationAvailable && a.animation.Active
	componentHidden(view, "animation-tab", !animationAvailable)
	componentHidden(view, "animation-panel", !animationActive)
	componentHidden(view, "resource-panel", animationActive)
	view.Attr("resources-tab", "aria-selected", strconv.FormatBool(!animationActive))
	view.Attr("animation-tab", "aria-selected", strconv.FormatBool(animationActive))
	view.Attr("resources-tab", "tabindex", "0")
	view.Attr("animation-tab", "tabindex", "-1")
	if animationActive {
		view.Attr("resources-tab", "tabindex", "-1")
		view.Attr("animation-tab", "tabindex", "0")
	}
	view.Keyed("animation-content", state.Animation.nodes())
	view.Keyed("animation-curve-popup", state.AnimationCurve.nodes())
	view.Keyed("component-picker", state.Picker.nodes())
	componentHidden(view, "menu", !open || !a.contextOpen)
	moduleActions := a.contextParent != "" && a.contextElement == "" && a.contextResource == "" && a.contextSlot == ""
	editDefinition := a.contextMod != "" && a.editingModule.Scene != ""
	menuHeight := float32(38) // Create Module plus the menu border and padding.
	for _, visible := range [...]bool{a.editingModule.Scene == "", moduleActions, moduleActions, a.contextMod != "", a.contextElement != "", editDefinition, a.contextSlot != "", editDefinition, a.contextResource != ""} {
		if visible {
			menuHeight += 28
		}
	}
	x, y := clamp(a.contextX, 0, max(0, a.width-204)), clamp(a.contextY, editorTopBarHeight, max(editorTopBarHeight, a.height-menuHeight))
	view.Attr("menu", "style", fmt.Sprintf("position:absolute;left:%gpx;top:%gpx;width:200px;z-index:60", x, y))
	resourceX := x + 200
	if resourceX+200 > a.width {
		resourceX = max(0, x-200)
	}
	resourceTop := y + 4 + 28 // Create Module precedes Add Resources.
	if moduleActions {
		resourceTop += 56 // Duplicate Module and Delete Module.
	}
	if a.contextMod != "" {
		resourceTop += 28
	}
	if a.contextElement != "" {
		resourceTop += 28
	}
	if editDefinition {
		resourceTop += 28
	}
	if a.contextSlot != "" {
		resourceTop += 28
	}
	resourceY := clamp(resourceTop, editorTopBarHeight, max(editorTopBarHeight, a.height-float32(len(workspaceResourceTypes)*28+10)))
	view.Attr("resource-menu", "style", fmt.Sprintf("position:absolute;left:%gpx;top:%gpx;width:200px;z-index:61", resourceX, resourceY))
	componentHidden(view, "resource-menu", !open || !a.contextOpen || !a.resourceMenuOpen || a.contextMod == "" || a.editingModule.Scene == "")
	view.Attr("add-resources", "aria-expanded", strconv.FormatBool(a.resourceMenuOpen))
	fileMenuHeight := float32(66) // New Folder plus the menu border and padding.
	if a.contextFile != "" {
		fileMenuHeight += 28 // Delete.
	}
	fileX, fileY := clamp(a.contextX, 0, max(0, a.width-204)), clamp(a.contextY, editorTopBarHeight, max(editorTopBarHeight, a.height-fileMenuHeight))
	view.Attr("file-menu", "style", fmt.Sprintf("position:absolute;left:%gpx;top:%gpx;width:200px;z-index:60", fileX, fileY))
	componentHidden(view, "file-menu", !open || !a.fileMenuOpen)
	componentHidden(view, "file-delete", a.contextFile == "")
	label := "Create Scene Module"
	if a.contextParent != "" {
		label = "Create Child Module"
	}
	view.Text("new-module-label", label)
	if a.contextElement != "" {
		view.Text("new-element-label", "Add Sibling Element")
	} else {
		view.Text("new-element-label", "Add Element")
	}
	componentHidden(view, "new-scene", a.editingModule.Scene != "")
	componentHidden(view, "exit-module", a.editingModule.Scene == "")
	componentHidden(view, "duplicate-module", !moduleActions)
	componentHidden(view, "delete-module", !moduleActions)
	componentHidden(view, "new-element", a.contextMod == "")
	componentHidden(view, "delete-element", a.contextElement == "")
	componentHidden(view, "new-slot", !editDefinition)
	componentHidden(view, "delete-slot", a.contextSlot == "")
	componentHidden(view, "add-resources", !editDefinition)
	componentHidden(view, "delete-resource", a.contextResource == "")
}

func renderWorkspaceScenes(context html.RenderContext[workspaceComponentProps, workspaceListState], view *html.View) {
	a, b := context.Props.Editor, &context.State.View
	b.reset()
	a.rows = a.rows[:0]
	clear(a.rowByID)
	// The edited module owns the tree: render only its subtree, no scene headers.
	if a.editingModule.Scene != "" {
		if instance := a.Document.Instance(a.editingModule.ID); instance != nil {
			a.sceneTree(b, a.editingModule.Scene, []project.ModInstanceSpec{*instance}, "", 0, true)
		}
		view.Keyed("rows", b.nodes())
		return
	}
	for index, id := range a.Document.Spec.Scenes {
		doc := a.Document.Scenes[id]
		dirty := ""
		if doc.Dirty {
			dirty = " *"
		}
		expanded := !a.collapsedScenes[id]
		arrow := "onix-leftbar/arrow.svg"
		if !expanded {
			arrow += "#right"
		}
		control := "scene:" + url.PathEscape(string(id))
		b.open("li", control, "role", "none")
		b.open("button", control, "id", control, "type", "button", "class", "scene-tree-heading", "role", "treeitem", "aria-label", string(id), "aria-selected", strconv.FormatBool(id == a.Document.Active && a.Selected == ""), "aria-level", "1", "aria-posinset", strconv.Itoa(index+1), "aria-setsize", strconv.Itoa(len(a.Document.Spec.Scenes)), "aria-expanded", strconv.FormatBool(expanded))
		b.text("img", "icon", "", "src", arrow, "alt", "")
		b.text("span", "label", strings.TrimSuffix(string(id), ".json")+dirty)
		b.close()
		b.close()
		if expanded {
			a.sceneTree(b, id, doc.Spec.Modules, "", 1, false)
		}
	}
	view.Keyed("rows", b.nodes())
}

func (a *ModuleEditor) sceneTree(b *editorVNodeView, sceneID project.SceneID, values []project.ModInstanceSpec, parent string, depth int, edit bool) {
	for i, instance := range values {
		row := workspaceTreeRow{Scene: sceneID, ID: instance.ID, Parent: parent, Index: i, Depth: depth, Mod: instance.Mod}
		a.rows = append(a.rows, row)
		if sceneID == a.Document.Active {
			a.rowByID[instance.ID] = row
		}
		collapsed := a.moduleCollapsed(workspaceModuleKey{Scene: sceneID, ID: instance.ID})
		icon := "onix-leftbar/component.svg"
		control := a.treeRowControlID(row)
		b.open("li", string(sceneID)+"/"+instance.ID, "role", "none")
		b.open("button", "row", "id", control, "type", "button", "class", "module-tree-row", "role", "treeitem", "aria-label", instance.ID, "aria-selected", strconv.FormatBool(sceneID == a.Document.Active && a.isSelected(instance.ID) && a.selectedElement == "" && a.selectedResource == "" && a.selectedSlot == "" && !a.slotCreating), "aria-level", strconv.Itoa(depth+1), "aria-posinset", strconv.Itoa(i+1), "aria-setsize", strconv.Itoa(len(values)), "style", "padding-left:"+strconv.Itoa(depth*14+5)+"px")
		if len(instance.Children) != 0 || edit {
			icon = "onix-leftbar/arrow.svg"
			if collapsed {
				icon += "#right"
			}
			b.attr("aria-expanded", strconv.FormatBool(!collapsed))
		}
		b.text("img", "icon", "", "src", icon, "alt", "")
		b.text("span", "label", instance.ID)
		b.close()
		b.close()
		if !collapsed {
			a.sceneTree(b, sceneID, instance.Children, instance.ID, depth+1, edit)
			if edit {
				if document := a.Document.Mods[instance.Mod]; document != nil && !document.Deleted {
					mod := a.viewMod(instance.Mod)
					if a.moduleTreeGroup(b, sceneID, instance.ID, instance.Mod, "elements", "Elements", depth+1) {
						a.elementTreeIndex.reset(mod.Elements)
						a.elementTree(b, sceneID, instance.ID, instance.Mod, "", mod.Elements, depth+2)
					}
					if a.moduleTreeGroup(b, sceneID, instance.ID, instance.Mod, "resources", "Resources", depth+1) {
						a.moduleDeclarationTree(b, workspaceTreeRow{Scene: sceneID, ID: instance.ID, Mod: instance.Mod, Depth: depth + 2}, mod.Resources, "resource", a.selectedResource, workspaceResourceTreeInfo)
					}
					if a.moduleTreeGroup(b, sceneID, instance.ID, instance.Mod, "slots", "Slots", depth+1) {
						a.moduleDeclarationTree(b, workspaceTreeRow{Scene: sceneID, ID: instance.ID, Mod: instance.Mod, Depth: depth + 2}, mod.Slots, "slot", a.selectedSlot, workspaceSlotTreeInfo)
					}
				}
			}
		}
	}
}

func (a *ModuleEditor) moduleTreeGroup(b *editorVNodeView, sceneID project.SceneID, owner string, mod project.ModID, group, label string, depth int) bool {
	row := workspaceTreeRow{Scene: sceneID, ID: owner, Mod: mod, Depth: depth, Group: group}
	a.rows = append(a.rows, row)
	control := a.treeRowControlID(row)
	expanded := !a.sectionsCollapsed[control]
	arrow := "onix-leftbar/arrow.svg"
	if !expanded {
		arrow += "#right"
	}
	b.open("li", control, "role", "none")
	b.open("button", "row", "id", control, "type", "button", "class", "module-tree-row", "role", "treeitem", "aria-label", label, "aria-level", strconv.Itoa(depth+1), "aria-expanded", strconv.FormatBool(expanded), "style", "padding-left:"+strconv.Itoa(depth*14+5)+"px")
	b.text("img", "icon", "", "src", arrow, "alt", "")
	b.text("span", "label", label)
	b.close()
	b.close()
	return expanded
}

func (a *ModuleEditor) moduleDeclarationTree[V any](b *editorVNodeView, row workspaceTreeRow, values map[string]V, kind, selected string, describe func(V) (label, icon, title string)) {
	a.declarationNames = a.declarationNames[:0]
	for name := range values {
		a.declarationNames = append(a.declarationNames, name)
	}
	slices.Sort(a.declarationNames)
	for i, name := range a.declarationNames {
		label, icon, title := describe(values[name])
		row.Index = i
		if kind == "slot" {
			row.Slot = name
		} else {
			row.Resource = name
		}
		a.rows = append(a.rows, row)
		control := a.treeRowControlID(row)
		b.open("li", control, "role", "none")
		b.open("button", "row", "id", control, "type", "button", "class", "module-tree-row", "role", "treeitem", "aria-label", name+" ("+label+")", "aria-selected", strconv.FormatBool(row.Scene == a.Document.Active && a.Selected == row.ID && selected == name), "aria-level", strconv.Itoa(row.Depth+1), "aria-posinset", strconv.Itoa(i+1), "aria-setsize", strconv.Itoa(len(values)), "title", title, "style", "padding-left:"+strconv.Itoa(row.Depth*14+5)+"px")
		b.text("img", "icon", "", "src", icon, "alt", "")
		b.text("span", "label", name)
		b.close()
		b.close()
	}
}

func workspaceResourceTreeInfo(resource project.ModResourceSpec) (label, icon, title string) {
	return resource.Kind, "onix-file-icons/" + editorresources.FileIconName(resource.Path, false), resource.Path
}

func workspaceSlotTreeInfo(slot project.SlotSpec) (label, icon, title string) {
	return string(slot.Type), "onix-leftbar/component.svg", string(slot.Type)
}

// modElements reads one definition's elements, including staged edits.
func (a *ModuleEditor) modElements(mod project.ModID) []project.ModElementSpec {
	if value := a.Document.Mods[mod]; value == nil || value.Deleted {
		return nil
	}
	return a.viewMod(mod).Elements
}

// elementTree lists an instance's definition elements inside the module's
// exclusive editing space; add/remove edits land on the shared mod.json.
func (a *ModuleEditor) elementTree(b *editorVNodeView, sceneID project.SceneID, owner string, mod project.ModID, parent string, elements []project.ModElementSpec, depth int) {
	siblings := a.elementTreeIndex.branches[parent]
	for index, position := siblings.First, 1; index != 0; index, position = a.elementTreeIndex.next[index-1], position+1 {
		i := index - 1
		element := elements[i]
		branch := a.elementTreeIndex.branches[element.ID]
		expanded := a.expandedElements[workspaceElementKey{workspaceModuleKey: workspaceModuleKey{Scene: sceneID, ID: owner}, Element: element.ID}]
		row := workspaceTreeRow{Scene: sceneID, ID: owner, Parent: parent, Index: i, Depth: depth, Mod: mod, Element: element.ID}
		a.rows = append(a.rows, row)
		control := a.treeRowControlID(row)
		b.open("li", string(sceneID)+"/"+owner+"/"+element.ID, "role", "none")
		b.open("button", "row", "id", control, "type", "button", "class", "module-tree-row", "role", "treeitem", "aria-label", element.ID, "aria-selected", strconv.FormatBool(sceneID == a.Document.Active && a.Selected == owner && a.selectedElement == element.ID), "aria-level", strconv.Itoa(depth+1), "aria-posinset", strconv.Itoa(position), "aria-setsize", strconv.Itoa(siblings.Count), "style", "padding-left:"+strconv.Itoa(depth*14+5)+"px")
		icon := "onix-leftbar/component.svg"
		if branch.Count != 0 {
			icon = "onix-leftbar/arrow.svg"
			if !expanded {
				icon += "#right"
			}
			b.attr("aria-expanded", strconv.FormatBool(expanded))
		}
		b.text("img", "icon", "", "src", icon, "alt", "")
		b.text("span", "label", element.ID)
		b.close()
		b.close()
		if branch.Count != 0 && expanded {
			a.elementTree(b, sceneID, owner, mod, element.ID, elements, depth+1)
		}
	}
}

func renderWorkspaceFiles(context html.RenderContext[workspaceComponentProps, workspaceFilesState], view *html.View) {
	a, b := context.Props.Editor, &context.State.View
	b.reset()
	directory := a.resourceDirectory()
	context.State.refresh(a.Document, a.selectedMod(), directory)
	a.files = context.State.Files
	label := directory
	if label == "" {
		label = "Project files"
	}
	view.Text("directory", label)
	if directory == string(a.selectedMod()) {
		view.Attr("up", "disabled", "")
	} else {
		view.RemoveAttr("up", "disabled")
	}
	componentHidden(view, "empty", len(a.files) != 0)
	for _, tile := range context.State.Tiles {
		file := tile.File
		b.open("li", file.Path)
		b.open("button", "file", "id", tile.ID, "type", "button", "class", "resource-tile", "aria-pressed", strconv.FormatBool(file.Path == a.lastFile), "aria-label", file.Path, "title", file.Path)
		b.text("img", "icon", "", "src", tile.Icon, "alt", "")
		b.text("span", "label", tile.Label)
		b.close()
		b.close()
	}
	view.Keyed("rows", b.nodes())
}

func renderWorkspaceInspector(context html.RenderContext[workspaceComponentProps, workspaceListState], view *html.View) {
	view.Keyed("fields", context.Props.Fields)
}

func (a *ModuleEditor) syncWorkspaceComponent(world *ecs.World) error {
	a.captureAnimationScroll(world)
	definition, err := editorWorkspaceDefinition()
	if err != nil {
		return err
	}
	clear(a.fields)
	clear(a.controlValues)
	// One frame's staged decodes are enough; dropping them keeps the cache bounded.
	clear(a.viewMods)
	clear(a.viewScenes)
	a.controlValues["project-path"] = a.pathDraft
	if a.Document == nil {
		a.rows = a.rows[:0]
		a.files = a.files[:0]
		a.fileBrowser = workspaceFileBrowser{}
		a.lastFile = ""
		clear(a.rowByID)
	}
	props := workspaceComponentProps{Editor: a}
	if a.shell == 0 {
		a.shell = spawnWorkspaceSurface(world, editorresources.WorkspaceHTML, editorresources.WorkspaceCSS, a.font, rect{W: a.width, H: a.height}, 0)
		a.shellComponent, err = definition.HydrateComponent(world, a.shell, props, workspaceComponentState{}, 1)
		if err != nil {
			return err
		}
	} else {
		surface := world.GetMut[html.Surface](a.shell)
		surface.Width, surface.Height = a.width, a.height
		html.UpdateComponentProps[workspaceComponentProps, workspaceComponentState](world, a.shellComponent, props)
	}
	if err := html.SyncComponentsSystem("").Run(world); err != nil {
		return err
	}
	root, err := html.Root(world, a.shell)
	if err != nil {
		return err
	}
	a.bindTree(root)
	a.bindAnimation(root)
	a.bindElementPicker(root)
	return nil
}
