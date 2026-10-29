package editor

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	htmlui "github.com/diiyw/onix/ui/html"
	"github.com/go-gl/mathgl/mgl32"
)

type workspaceField struct {
	Target               DocumentID
	Pointer, Kind, Value string
	Axis                 int
	Vector               [4]float64
	VectorBits           int
	VectorSize           int
	CommitOnChange       bool
	Selected             bool
	NumberStep           float64
	SlotValue            bool
	Unset                bool
	Animation            bool
}

func (f workspaceField) modified(value string) bool { return f.Unset || value != f.Value }

type workspaceTreeRow struct {
	Scene        project.SceneID
	ID, Parent   string
	Index, Depth int
	Mod          project.ModID
	Element      string // non-empty for private module element rows
	Resource     string
	Slot         string
	Group        string
}

func workspaceRectStyle(bounds rect) string {
	return fmt.Sprintf("position:absolute;left:%gpx;top:%gpx;width:%gpx;height:%gpx", bounds.X, bounds.Y, bounds.W, bounds.H)
}

func detailHeader(b *editorVNodeView, name string) {
	b.open("header", "detail-header", "class", "detail-header")
	b.text("h2", "element-title", name, "class", "element-title")
	b.close()
}

// section opens one collapsible Details panel; sectionEnd closes it. Collapsed
// bodies render with the hidden attribute so field drafts survive a collapse.
func (a *ModuleEditor) section(b *editorVNodeView, key, title string) {
	a.sectionWithRemove(b, key, title, "")
}

func (a *ModuleEditor) sectionWithRemove(b *editorVNodeView, key, title, removeID string) {
	collapsed := a.sectionsCollapsed[key]
	titleClass := "detail-section-title"
	if collapsed {
		titleClass = "detail-section-title collapsed"
	}
	b.open("section", "section:"+key, "class", "detail-section", "aria-labelledby", key+"-title")
	b.open("h3", "title", "class", titleClass)
	b.open("button", "toggle:"+key, "id", "toggle:"+key, "type", "button", "class", "detail-section-toggle", "aria-expanded", strconv.FormatBool(!collapsed), "aria-controls", "body:"+key)
	arrow := "onix-rightbar/arrow.svg"
	if collapsed {
		arrow = "onix-rightbar/arrow.svg#right"
	}
	b.text("img", "arrow", "", "src", arrow, "alt", "")
	b.text("img", "icon", "", "src", workspaceSectionIcon(key), "alt", "")
	b.text("span", "label", title, "id", key+"-title")
	b.close()
	if removeID != "" {
		menuID := "content-menu:" + removeID
		b.open("button", menuID, "id", menuID, "type", "button", "class", "detail-component-menu", "aria-label", title+" actions", "aria-haspopup", "menu", "aria-expanded", strconv.FormatBool(a.componentMenu == removeID), "aria-controls", "menu:"+key)
		b.text("img", "icon", "", "src", "onix-rightbar/more-horizontal.svg", "alt", "")
		b.close()
	}
	b.close()
	if removeID != "" {
		b.open("div", "menu", "id", "menu:"+key, "class", "content-component-menu", "role", "menu", "aria-label", title+" actions")
		if a.componentMenu != removeID {
			b.attr("hidden", "")
		}
		b.open("button", removeID, "id", removeID, "type", "button", "role", "menuitem", "class", "danger")
		b.text("img", "icon", "", "src", "onix-rightbar/trash-2.svg#danger", "alt", "")
		b.text("span", "label", "Remove Component")
		b.close()
		b.close()
	}
	b.open("div", "body:"+key, "id", "body:"+key, "class", "detail-section-body")
	if collapsed {
		b.attr("hidden", "")
	}
}

func detailRemoveButton(b *editorVNodeView, id, label string) {
	b.open("button", id, "id", id, "type", "button", "class", "detail-remove", "aria-label", "Remove "+label, "title", "Remove "+label)
	b.text("img", "icon", "", "src", "onix-rightbar/trash-2.svg#danger", "alt", "")
	b.close()
}

func (a *ModuleEditor) sectionEnd(b *editorVNodeView) {
	b.close()
	b.close()
}

func (a *ModuleEditor) field(b *editorVNodeView, target DocumentID, pointer, label, kind, value string) {
	a.contentField(b, target, pointer, label, kind, value, "")
}

// axes name the inputs of a numeric vector panel. Transform vectors use X/Y/Z,
// sizes use Width/Height.
var (
	axesXYZ = []string{"X", "Y", "Z"}
	axesWH  = []string{"Width", "Height"}
)

var sceneExitOptions = workspaceContentProperty{
	Label:   "When leaving scene",
	Options: []string{"Automatic (destroy by default)", "Keep (pause and restore)", "Destroy (recreate on return)"},
	Values:  []string{"", string(project.SceneExitRetain), string(project.SceneExitDestroy)},
}

func (a *ModuleEditor) vector(b *editorVNodeView, target DocumentID, pointer, label string, values []float32, axes []string, defaultValue float32) {
	var seed [3]float32
	for i := range axes {
		seed[i] = defaultValue
	}
	copy(seed[:len(axes)], values)
	b.open("div", pointer+"-group", "class", "transform-property", "role", "group", "aria-labelledby", pointer+"-label")
	b.text("div", pointer+"-label", label, "id", pointer+"-label", "class", "content-field-label")
	a.vectorInputs(b, target, pointer, seed, axes, false)
	b.close()
}

func (a *ModuleEditor) vectorInputs(b *editorVNodeView, target DocumentID, pointer string, seed [3]float32, axes []string, commitOnChange bool) {
	a.numericVectorInputs(b, target, pointer, [4]float64{float64(seed[0]), float64(seed[1]), float64(seed[2])}, axes, 32, commitOnChange)
}

func (a *ModuleEditor) numericVectorInputs(b *editorVNodeView, target DocumentID, pointer string, seed [4]float64, axes []string, bits int, commitOnChange bool) {
	a.numericVectorFields(b, target, pointer, seed, axes, bits, workspaceField{CommitOnChange: commitOnChange})
}

func (a *ModuleEditor) numericVectorFields(b *editorVNodeView, target DocumentID, pointer string, seed [4]float64, axes []string, bits int, field workspaceField) {
	field.Target, field.Pointer, field.Kind = target, pointer, "number"
	field.Vector, field.VectorBits, field.VectorSize = seed, bits, len(axes)
	b.open("div", pointer, "class", "row detail-vector")
	for i, axis := range axes {
		id := workspaceFieldID(target, pointer, axis)
		value := strconv.FormatFloat(seed[i], 'f', -1, bits)
		axisField := field
		axisField.Value, axisField.Axis = value, i
		axisField = a.animationDetailField(axisField)
		value = axisField.Value
		a.fields[id] = axisField
		if draft, ok := a.drafts[id]; ok {
			value = draft
		}
		a.controlValues[id] = value
		b.open("div", axis)
		b.text("label", id, axis, "for", id)
		b.open("div", "control", "class", "content-control content-number")
		b.text("input", id, "", "id", id, "type", "number", "step", "any", "value", value)
		contentStepButtons(b, id, axis)
		b.close()
		b.close()
	}
	b.close()
}

func (a *ModuleEditor) transform(b *editorVNodeView, target DocumentID, pointer string, value project.ModTransformSpec) {
	b.open("div", pointer, "class", "transform-properties")
	a.vector(b, target, pointer+"/position", "Position", value.Position, axesXYZ, 0)
	rotation := value.Rotation
	var planarRotation [3]float32
	if len(rotation) == 1 {
		planarRotation[2] = rotation[0]
		rotation = planarRotation[:]
	}
	a.vector(b, target, pointer+"/rotation", "Rotation (radians)", rotation, axesXYZ, 0)
	a.vector(b, target, pointer+"/scale", "Scale", value.Scale, axesXYZ, 1)
	b.close()
}

// Resolve the instance and patch path in one traversal. Build
// path strings only for the matching branch, rather than every visited node.
func locateWorkspaceInstance(values []project.ModInstanceSpec, id string) (*project.ModInstanceSpec, string) {
	for i := range values {
		instance := &values[i]
		if instance.ID == id {
			return instance, "/" + strconv.Itoa(i)
		}
		if found, pointer := locateWorkspaceInstance(instance.Children, id); found != nil {
			return found, "/" + strconv.Itoa(i) + "/children" + pointer
		}
	}
	return nil, ""
}

func (a *ModuleEditor) inspector(b, picker *editorVNodeView) {
	a.animation.Detail = workspaceAnimationDetail{}
	var instance *project.ModInstanceSpec
	var pointer string
	if a.Selected != "" {
		instance, pointer = locateWorkspaceInstance(a.viewScene(a.Document.Active).Modules, a.Selected)
		if instance != nil {
			pointer = "/modules" + pointer
		}
	}
	modID := a.Local
	if instance != nil && (a.Local == "" || a.editingModule.Scene != "") {
		modID = instance.Mod
	}
	if modID == "" {
		target, value := sceneDocumentID(a.Document.Active), a.viewScene(a.Document.Active)
		detailHeader(b, strings.TrimSuffix(string(a.Document.Active), ".json"))
		a.section(b, "scene-lifecycle", "Scene Lifecycle")
		b.open("div", "settings", "class", "content-fields")
		a.contentOptions(b, target, "/exitPolicy", &sceneExitOptions, "string", string(value.ExitPolicy))
		b.close()
		a.sectionEnd(b)
		var position [3]float32
		copy(position[:], value.Transform.Position)
		a.section(b, "scene-position", "Position")
		a.vectorInputs(b, target, "/transform/position", position, axesXYZ, true)
		a.sectionEnd(b)
		return
	}
	if instance != nil && a.Local == "" {
		a.visibilityHeader(b, sceneDocumentID(a.Document.Active), pointer+"/visible", instance.ID, instance.Visible == nil || *instance.Visible)
	}
	if document := a.Document.Mods[modID]; document != nil && !document.Deleted {
		view := *document
		view.Spec = a.viewMod(modID)
		target := modDocumentID(modID)
		if a.slotCreating && a.editingModule.Scene != "" {
			a.section(b, "slot-create", "New Slot")
			a.addDeclaration(b, "slot", "Slot", `{"type":"string","default":"","writable":true}`)
			a.sectionEnd(b)
		} else if slot, ok := view.Spec.Slots[a.selectedSlot]; ok && a.selectedSlot != "" && a.editingModule.Scene != "" {
			a.slotInspector(b, target, a.selectedSlot, slot)
		} else if resource, ok := view.Spec.Resources[a.selectedResource]; ok && a.selectedResource != "" && a.editingModule.Scene != "" {
			detailHeader(b, a.selectedResource)
			a.section(b, "resource-details", "Resource")
			b.text("p", "kind", "Type: "+resource.Kind)
			b.text("p", "path", "Path within module: "+resource.Path)
			a.sectionEnd(b)
		} else if index, ok := a.selectedElementIndex(view.Spec.Elements); ok {
			index = a.animationInspectorIndex(view.Spec, index)
			a.elementInspector(b, picker, target, index, view.Spec.Elements[index], view.Spec.Elements)
		} else {
			if a.Local != "" {
				name := view.Spec.Name
				if name == "" {
					name = string(modID)
				}
				detailHeader(b, name)
			}
			a.section(b, "definition-identity", "Module Definition")
			if instance != nil && a.Local == "" {
				sceneTarget := sceneDocumentID(a.Document.Active)
				b.open("div", "state", "class", "detail-checks")
				a.field(b, sceneTarget, pointer+"/enabled", "Enabled at runtime", "bool", strconv.FormatBool(instance.Enabled == nil || *instance.Enabled))
				b.close()
			}
			a.definitionFields(b, &view, target)
			a.sectionEnd(b)
			if a.Local != "" && a.editingModule.Scene != "" {
				a.definitionComponents(b, picker, target, view.Spec.Components)
			}
		}
		if instance != nil && a.Local == "" {
			sceneTarget := sceneDocumentID(a.Document.Active)
			a.section(b, "instance-transform", "Transform")
			a.transform(b, sceneTarget, pointer+"/placement", instance.Placement)
			a.sectionEnd(b)
			a.instanceOverrides(b, sceneTarget, pointer, instance, view.Spec.Slots)
		}
	}
}

func (a *ModuleEditor) instanceOverrides(b *editorVNodeView, target DocumentID, pointer string, instance *project.ModInstanceSpec, slots map[string]project.SlotSpec) {
	if len(slots) == 0 {
		return
	}
	a.section(b, "instance-slots", "Slot")
	b.open("div", "slots", "class", "content-fields")
	for _, name := range sortedWorkspaceKeys(slots) {
		slot := slots[name]
		path := pointer + "/slots/" + strings.ReplaceAll(name, "~", "~0")
		a.slotValueField(b, target, path, name+" ("+string(slot.Type)+")", slot.Type, instance.Slots[name], slot.Default, "Use default")
	}
	b.close()
	a.sectionEnd(b)
}

// definitionFields edits the selected module's size and identity.
func (a *ModuleEditor) definitionFields(b *editorVNodeView, mod *ModDocument, target DocumentID) {
	b.open("div", "identity", "class", "content-fields")
	b.open("div", "name", "class", "content-property")
	a.field(b, target, "/name", "Name", "string", mod.Spec.Name)
	b.close()
	size := a.definitionSize(mod)
	a.vectorInputs(b, target, "/components/size", size, axesWH, false)
	b.open("div", "space", "class", "content-property")
	space := mod.Spec.Space
	if space == "" {
		space = project.CoordinateSpaceUI
	}
	a.contentOptions(b, target, "/space", &workspaceModuleSpace, "string", string(space))
	b.close()
	b.open("div", "behavior", "class", "content-property")
	a.field(b, target, "/behavior", "Behavior type", "string", mod.Spec.Behavior)
	b.close()
	b.close()
}

// An explicit module size defines its logical extent. Otherwise use content
// bounds in module coordinates, independent of scene placement and zoom.
func (a *ModuleEditor) definitionSize(mod *ModDocument) [3]float32 {
	if size := mod.Spec.Components.Size; len(size) == 2 {
		return [3]float32{size[0], size[1]}
	}
	var lowX, lowY, highX, highY float32
	found := false
	include := func(content project.ModComponents, matrix mgl32.Mat4, space project.CoordinateSpace) {
		size := content.Size
		if sprite := content.Sprite; sprite != nil && len(sprite.Size) == 2 {
			size = sprite.Size
		}
		if len(size) != 2 {
			return
		}
		anchor := scene.AnchorCenter
		if space != project.CoordinateSpaceWorld {
			anchor = scene.AnchorTopLeft
		}
		if content.Anchor != nil {
			anchor = *content.Anchor
		}
		x0, y0, x1, y1, ok := scene.BoundsInSpace2D(matrix, anchor, size[0], size[1], space != project.CoordinateSpaceWorld)
		if !ok {
			return
		}
		if !found {
			lowX, lowY, highX, highY, found = x0, y0, x1, y1, true
			return
		}
		lowX, lowY, highX, highY = min(lowX, x0), min(lowY, y0), max(highX, x1), max(highY, y1)
	}
	include(mod.Spec.Components, mgl32.Ident4(), mod.Spec.Space)
	a.definitionMatrices = slices.Grow(a.definitionMatrices[:0], len(mod.Plan.Elements))[:len(mod.Plan.Elements)]
	for i, element := range mod.Plan.Elements {
		space := mod.Spec.Space
		if element.Spec.Space != "" {
			space = element.Spec.Space
		}
		matrix := previewTransformMatrix(element.Transform, space)
		if element.Parent >= 0 {
			matrix = a.definitionMatrices[element.Parent].Mul4(matrix)
		}
		a.definitionMatrices[i] = matrix
		include(element.Spec.Components, matrix, space)
	}
	return [3]float32{highX - lowX, highY - lowY}
}

func (a *ModuleEditor) slotInspector(b *editorVNodeView, target DocumentID, name string, slot project.SlotSpec) {
	pointer := "/slots/" + strings.ReplaceAll(name, "~", "~0")
	detailHeader(b, name)
	a.section(b, "slot-details", "Slot")
	b.open("div", "slot", "class", "content-fields")
	b.open("div", "type", "class", "content-property")
	a.field(b, target, pointer+"/type", "Type", "string", string(slot.Type))
	b.close()
	a.slotValueField(b, target, pointer+"/default", "Default value", slot.Type, slot.Default, nil, "Reset")
	a.field(b, target, pointer+"/writable", "Allow external writes at runtime", "bool", fmt.Sprint(slot.Writable == nil || *slot.Writable))
	b.close()
	a.sectionEnd(b)
	a.section(b, "slot-transform", "Placement")
	a.transform(b, target, pointer+"/placement", slot.Placement)
	a.sectionEnd(b)
}

// selectedElementIndex resolves the tree selection against the viewed
// definition, so a deleted or renamed element falls back to the module panel.
func (a *ModuleEditor) selectedElementIndex(elements []project.ModElementSpec) (int, bool) {
	if a.selectedElement == "" || a.editingModule.Scene == "" {
		return 0, false
	}
	index := slices.IndexFunc(elements, func(element project.ModElementSpec) bool { return element.ID == a.selectedElement })
	return index, index >= 0
}

func sortedWorkspaceKeys[K ~string, V any](values map[K]V) []K {
	keys := make([]K, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func (a *ModuleEditor) addInput(b *editorVNodeView, id, label, initial string) {
	value, ok := a.additions["add/"+id]
	if !ok {
		value = initial
	}
	a.controlValues["add/"+id] = value
	b.input("add/"+id, label, value)
}

func (a *ModuleEditor) addDeclaration(b *editorVNodeView, id, label, initial string) {
	a.addInput(b, id+"/name", "New "+label+" name", "")
	value, ok := a.additions["add/"+id+"/value"]
	if !ok {
		value = initial
	}
	a.controlValues["add/"+id+"/value"] = value
	control := "add/" + id + "/value"
	b.text("label", control, "Declaration JSON", "for", control)
	b.text("textarea", control, value, "id", control)
	b.button("add:"+id, "Add "+label)
}

func (a *ModuleEditor) hasDrafts() bool {
	if len(a.staged) != 0 {
		return true
	}
	for id, value := range a.drafts {
		if field, ok := a.fields[id]; ok && field.modified(value) {
			return true
		}
	}
	return false
}

func (a *ModuleEditor) formPatches() (map[DocumentID][]DocumentPatch, error) {
	patches := make(map[DocumentID][]DocumentPatch)
	for target, data := range a.staged {
		patches[target] = []DocumentPatch{{Value: json.RawMessage(data)}}
	}
	vectors := make(map[string][]float64)
	vectorFields := make(map[string]workspaceField)
	ids := sortedWorkspaceKeys(a.drafts)
	for _, id := range ids {
		value, field := a.drafts[id], a.fields[id]
		if field.Target == "" || !field.modified(value) {
			continue
		}
		if field.Axis >= 0 {
			key := string(field.Target) + "|" + field.Pointer
			vector, ok := vectors[key]
			if !ok {
				vector = slices.Clone(field.Vector[:field.VectorSize])
				vectors[key], vectorFields[key] = vector, field
			}
			number, err := strconv.ParseFloat(value, field.VectorBits)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", id, err)
			}
			vector[field.Axis] = number
			continue
		}
		patch := DocumentPatch{Pointer: field.Pointer}
		switch field.Kind {
		case "json":
			patch.Value = json.RawMessage(value)
			patch.Remove = strings.TrimSpace(value) == ""
		case "bool", "number", "int":
			patch.Value = json.RawMessage(value)
			if field.Animation && field.Kind == "number" {
				var err error
				patch.Value, err = slotNumberJSON(field.Kind, value)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", id, err)
				}
			}
		default:
			patch.Value = json.RawMessage(encodeFormJSON(value))
		}
		if field.SlotValue && (field.Kind == "number" || field.Kind == "int") {
			value = strings.TrimSpace(value)
			patch.Remove = value == ""
			if !patch.Remove {
				var err error
				patch.Value, err = slotNumberJSON(field.Kind, value)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", id, err)
				}
			}
		}
		patches[field.Target] = append(patches[field.Target], patch)
	}
	for _, key := range sortedWorkspaceKeys(vectors) {
		field := vectorFields[key]
		patches[field.Target] = append(patches[field.Target], DocumentPatch{Pointer: field.Pointer, Value: json.RawMessage(encodeFormJSON(vectors[key]))})
	}
	return patches, nil
}

func (a *ModuleEditor) applyDrafts() error {
	if a.Document == nil {
		return nil
	}
	if err := a.flushAnimationDrafts(); err != nil {
		return err
	}
	patches, err := a.formPatches()
	if err != nil {
		return err
	}
	if err := a.Document.PatchDocuments("Edit scene / module form", patches); err != nil {
		return err
	}
	clear(a.drafts)
	clear(a.staged)
	clear(a.textureModes)
	a.componentMenu = ""
	a.elementPicker.Field = ""
	return nil
}

func (a *ModuleEditor) removeField(value string) error {
	target, pointer, ok := strings.Cut(value, "|")
	if !ok {
		return fmt.Errorf("invalid removal target")
	}
	var err error
	target, err = url.PathUnescape(target)
	if err != nil {
		return err
	}
	patches, err := a.formPatches()
	if err != nil {
		return err
	}
	patches[DocumentID(target)] = append(patches[DocumentID(target)], DocumentPatch{Pointer: pointer, Remove: true})
	if err := a.stagePatches(patches); err != nil {
		return err
	}
	a.componentMenu = ""
	prefix := workspaceFieldID(DocumentID(target), pointer, "")
	for id := range a.textureModes {
		if strings.HasPrefix(id, prefix) {
			delete(a.textureModes, id)
		}
	}
	return nil
}

func workspaceFieldID(target DocumentID, pointer, axis string) string {
	id := "field:" + url.PathEscape(string(target)) + ":" + pointer
	if axis != "" {
		id += ":" + url.PathEscape(axis)
	}
	return id
}

func (a *ModuleEditor) addField(group string) error {
	patches, err := a.formPatches()
	if err != nil {
		return err
	}
	target, pointer := sceneDocumentID(a.Document.Active), ""
	if mod := a.selectedMod(); mod != "" {
		target = modDocumentID(mod)
	}
	name := a.additions["add/"+group+"/name"]
	if name == "" {
		return fmt.Errorf("Name cannot be empty")
	}
	if mod := a.selectedMod(); mod != "" {
		spec := a.viewMod(mod)
		exists := false
		if group == "slot" {
			_, exists = spec.Slots[name]
		}
		if exists {
			return fmt.Errorf("%s already exists; edit the existing declaration", name)
		}
	}
	value := json.RawMessage(a.additions["add/"+group+"/value"])
	if len(value) == 0 {
		switch group {
		case "slot":
			value = json.RawMessage(`{"type":"string","default":"","writable":true}`)
		}
	}
	switch group {
	case "slot":
		pointer = "/slots/" + strings.ReplaceAll(name, "~", "~0")
	default:
		return fmt.Errorf("unknown declaration %q", group)
	}
	patches[target] = append(patches[target], DocumentPatch{Pointer: pointer, Value: value})
	if err := a.stagePatches(patches); err != nil {
		return err
	}
	if group == "slot" {
		a.selectedSlot, a.slotCreating = name, false
	}
	return nil
}

// Structural form changes remain drafts until the document is applied.
func (a *ModuleEditor) stagePatches(patches map[DocumentID][]DocumentPatch) error {
	staged := make(map[DocumentID][]byte, len(patches))
	for target, values := range patches {
		state := a.Document.states[target]
		if state == nil || state.data == nil {
			return fmt.Errorf("unknown document %s", target)
		}
		data, err := applyDocumentPatches(state.data, values)
		if err != nil {
			return err
		}
		canonical, err := canonicalWorkspaceJSON(target, data)
		if err != nil {
			return err
		}
		staged[target] = canonical
	}
	a.staged = staged
	clear(a.drafts)
	a.status = "Form modified. Press Ctrl+S to save"
	return nil
}

// stagedViewEntry caches one decoded staged document, keyed by source identity.
// Staged JSON is replaced wholesale, never edited in place, so this cannot go stale.
type stagedViewEntry[T any] struct {
	// source is retained so a freed buffer's address cannot be reused by a newer one.
	source []byte
	spec   T
}

func stagedView[T any](cache map[DocumentID]stagedViewEntry[T], target DocumentID, data []byte) (T, bool) {
	if entry, ok := cache[target]; ok && len(entry.source) == len(data) && (len(data) == 0 || &entry.source[0] == &data[0]) {
		return entry.spec, true
	}
	spec, err := decodeDocument[T](data)
	if err != nil {
		return spec, false
	}
	cache[target] = stagedViewEntry[T]{source: data, spec: spec}
	return spec, true
}

func (a *ModuleEditor) viewMod(id project.ModID) project.ModSpec {
	target := modDocumentID(id)
	data := a.staged[target]
	if data == nil {
		return a.Document.Mods[id].Spec
	}
	if a.viewMods == nil {
		a.viewMods = make(map[DocumentID]stagedViewEntry[project.ModSpec], 1)
	}
	if spec, ok := stagedView(a.viewMods, target, data); ok {
		return spec
	}
	return a.Document.Mods[id].Spec
}

func (a *ModuleEditor) viewScene(id project.SceneID) project.SceneSpec {
	target := sceneDocumentID(id)
	data := a.staged[target]
	if data == nil {
		return a.Document.Scenes[id].Spec
	}
	if a.viewScenes == nil {
		a.viewScenes = make(map[DocumentID]stagedViewEntry[project.SceneSpec], 1)
	}
	if spec, ok := stagedView(a.viewScenes, target, data); ok {
		return spec
	}
	return a.Document.Scenes[id].Spec
}

func (a *ModuleEditor) bindTree(root htmlui.Node) {
	for _, row := range a.rows {
		if row.Element != "" || row.Resource != "" || row.Slot != "" || row.Group != "" {
			continue
		}
		row := row
		if node, ok := root.FindByID(a.treeRowControlID(row)); ok {
			node.OnPointer(func(event *htmlui.PointerEvent) {
				if a.modalVisible() {
					event.Consume()
					return
				}
				if event.Kind == scene.InputEventPress && row.Scene == a.Document.Active {
					a.dragID, a.dragX, a.dragY, a.dragMoved = row.ID, event.X, event.Y, false
				}
			})
		}
	}
}

func (a *ModuleEditor) updateTreeDrag(pointer input.Pointer) {
	if a.dragID == "" {
		return
	}
	if pointer.Down && (abs(pointer.X-a.dragX) > 5 || abs(pointer.Y-a.dragY) > 5) {
		a.dragMoved = true
	}
	if !pointer.Released {
		return
	}
	id, moved := a.dragID, a.dragMoved
	a.dragID = ""
	if !moved || a.hasDrafts() || a.Local != "" {
		return
	}
	for entity, access := range a.world.Query2[htmlui.Control, scene.UIGlobalTransform]().All {
		control, global := access.A.Ref(), access.B.Ref()
		if control.Owner != a.shell {
			continue
		}
		row, ok := a.rowByID[strings.TrimPrefix(control.ID, "select:")]
		if !ok {
			continue
		}
		target, ok := a.world.Get[scene.InputTarget2D](entity)
		if !ok {
			continue
		}
		x, y, width, height, ok := scene.BoundsInSpace2D(global.Matrix, scene.AnchorTopLeft, target.Width, target.Height, true)
		if !ok || pointer.X < x || pointer.X > x+width || pointer.Y < y || pointer.Y > y+height {
			continue
		}
		if clip, ok := global.ClipRect(); ok && (pointer.X < clip.X || pointer.Y < clip.Y || pointer.X > clip.X+clip.Width || pointer.Y > clip.Y+clip.Height) {
			continue
		}
		parent, index := row.Parent, row.Index
		if pointer.X > x+float32(row.Depth*14)+80 {
			parent, index = row.ID, len(a.Document.Instance(row.ID).Children)
		} else if pointer.Y > y+height*.5 {
			index++
		}
		if source := a.rowByID[id]; source.Parent == parent && source.Index < index {
			index--
		}
		a.report(a.Document.MoveInstance(id, parent, index))
		return
	}
}
