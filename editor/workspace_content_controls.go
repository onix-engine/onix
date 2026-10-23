package editor

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/project"
	engineui "github.com/diiyw/onix/ui"
	htmlui "github.com/diiyw/onix/ui/html"
)

// These are the legacy inspector's presentation choices, applied to module
// document fields. The controls never depend on runtime scene entities.
func workspaceSectionIcon(key string) string {
	_, name, content := strings.Cut(key, "/components/")
	if !content {
		name = key
	}
	icon := "component"
	switch name {
	case "element-transform", "slot-transform", "scene-position", "instance-transform":
		icon = "moveup"
	case "sprite", "text", "animation", "model", "spine":
		icon = name
		if name == "text" {
			icon = "text-file"
		}
	case "html":
		icon = "code-file"
	case "particle", "emitter":
		icon = "particles"
	case "tilemap":
		icon = "tile-map"
	case "camera", "camera2D":
		icon = "camera"
	case "ambientLight", "directionalLight", "pointLight", "spotLight", "areaLight":
		icon = "light"
	case "layout":
		icon = "grid"
	case "input":
		icon = "input"
	}
	return "onix-rightbar/" + icon + ".svg"
}

func (a *ModuleEditor) contentFieldValue(target DocumentID, pointer, kind, value string) (string, string) {
	id := workspaceFieldID(target, pointer, "")
	field := a.animationDetailField(workspaceField{Target: target, Pointer: pointer, Kind: kind, Value: value, Axis: -1})
	a.fields[id] = field
	value = field.Value
	if draft, ok := a.drafts[id]; ok {
		value = draft
	}
	a.controlValues[id] = value
	return id, value
}

func (a *ModuleEditor) contentField(b *editorVNodeView, target DocumentID, pointer, label, kind, value, resource string) {
	id, value := a.contentFieldValue(target, pointer, kind, value)
	if kind == "bool" {
		b.open("label", id+"-label", "for", id, "class", "content-check")
		a.contentFieldControl(b, id, label, kind, value, resource)
		b.text("span", "label", label)
		b.close()
		return
	}
	labelFor := id
	if kind == "color" {
		labelFor = "content-color:" + id
	}
	b.text("label", id, label, "for", labelFor)
	a.contentFieldControl(b, id, label, kind, value, resource)
}

func (a *ModuleEditor) contentFieldControl(b *editorVNodeView, id, label, kind, value, resource string) {
	switch kind {
	case "bool":
		b.open("input", id, "id", id, "type", "checkbox")
		if value == "true" {
			b.attr("checked", "")
		}
		b.close()
	case "json", "text":
		b.text("textarea", id, value, "id", id)
	case "color":
		color, err := project.ParseColor(value, 1)
		style := ""
		if err == nil {
			textColor := "#ffffff"
			if color.R*.299+color.G*.587+color.B*.114 > 160.0/255 {
				textColor = "#18202c"
			}
			style = "background:" + color.String() + ";color:" + textColor
		}
		b.button("content-color:"+id, value, "class", "content-color", "style", style, "aria-label", "Edit "+label, "aria-haspopup", "dialog")
	default:
		class, inputType := "content-control", "text"
		if kind == "number" || kind == "int" {
			class, inputType = "content-control content-number", "number"
		}
		b.open("div", id+"-control", "class", class)
		b.open("input", id, "id", id, "type", inputType, "value", value)
		if kind == "int" {
			b.attr("step", "1")
		} else if kind == "number" {
			b.attr("step", "any")
		}
		b.close()
		if kind == "number" || kind == "int" {
			contentStepButtons(b, id, label)
		} else if resource != "" {
			button := "content-resource:" + id
			field := a.fields[id]
			a.fields[button] = workspaceField{Target: field.Target, Pointer: field.Pointer, Kind: "content-resource", Value: resource, Axis: -1}
			b.open("button", button, "id", button, "type", "button", "class", "content-file-button", "aria-label", "Choose "+label)
			b.text("img", "icon", "", "src", "onix-rightbar/folder.svg", "alt", "")
			b.close()
		}
		b.close()
	}
}

// contentParentField lists the module's elements as parent options, hiding the
// edited element and its descendants so the tree cannot be cycled.
func (a *ModuleEditor) contentParentField(b *editorVNodeView, target DocumentID, pointer, elementID, parent string, elements []project.ModElementSpec) {
	id, value := a.contentFieldValue(target, pointer, "string", parent)
	b.text("label", id, "Parent", "for", id)
	b.open("select", id, "id", id)
	b.open("option", "default", "value", "")
	if value == "" {
		b.attr("selected", "")
	}
	b.content("None")
	b.close()
	for _, candidate := range elements {
		if elementIsDescendant(elements, candidate.ID, elementID) {
			continue
		}
		b.open("option", candidate.ID, "value", candidate.ID)
		if value == candidate.ID {
			b.attr("selected", "")
		}
		b.content(candidate.ID)
		b.close()
	}
	b.close()
}

// elementIsDescendant reports whether node equals ancestor or nests beneath it.
func elementIsDescendant(elements []project.ModElementSpec, node, ancestor string) bool {
	for current := node; current != ""; {
		if current == ancestor {
			return true
		}
		index := slices.IndexFunc(elements, func(element project.ModElementSpec) bool { return element.ID == current })
		if index < 0 {
			return false
		}
		current = elements[index].Parent
	}
	return false
}

func contentStepButtons(b *editorVNodeView, id, label string) {
	for _, direction := range [...]string{"1", "-1"} {
		arrow, name := "onix-rightbar/arrow.svg#up", "Increase "
		if direction == "-1" {
			arrow, name = "onix-rightbar/arrow.svg", "Decrease "
		}
		button := "content-step:" + id + ":" + direction
		b.open("button", button, "id", button, "type", "button", "class", "content-step", "aria-label", name+label)
		b.text("img", "icon", "", "src", arrow, "alt", "")
		b.close()
	}
}

func (a *ModuleEditor) anchorComponentPicker() {
	if node, ok := htmlui.NodeByID(a.world, a.shell, "content-picker"); ok {
		if box, ok := node.Bounds(); ok {
			a.componentPickerAnchor = rect{X: box.X, Y: box.Y, W: box.Width, H: box.Height}
			return
		}
	}
	panel := a.layout().Inspector
	a.componentPickerAnchor = rect{X: panel.X + 14, Y: panel.Y + panel.H - 44, W: panel.W - 28, H: 28}
}

func (a *ModuleEditor) contentPickerBounds(anchor rect, count int) rect {
	width := min(max(anchor.W, 200), 280, max(0, a.width-16))
	top, bottom := float32(editorTopBarHeight+8), a.height-8
	height := min(float32(max(count, 1)*26+10), 360)
	below, above := max(0, bottom-anchor.Y-anchor.H-4), max(0, anchor.Y-top-4)
	y := anchor.Y + anchor.H + 4
	if below < height && above > below {
		height = min(height, above)
		y = anchor.Y - height - 4
	} else {
		height = min(height, below)
	}
	return rect{X: clamp(anchor.X+anchor.W-width, 8, max(8, a.width-width-8)), Y: clamp(y, top, max(top, bottom-height)), W: width, H: height}
}

func (a *ModuleEditor) contentOptions(b *editorVNodeView, target DocumentID, pointer string, p *workspaceContentProperty, kind, value string) {
	id, value := a.contentFieldValue(target, pointer, kind, value)
	if p.Segmented {
		b.text("div", id+"-label", p.Label, "id", id+"-label", "class", "content-field-label")
		b.open("div", id, "class", "content-segments", "role", "group", "aria-labelledby", id+"-label")
	} else {
		b.text("label", id, p.Label, "for", id)
		b.open("select", id, "id", id)
	}
	for i, label := range p.Options {
		option := strconv.Itoa(i)
		if len(p.Values) != 0 {
			option = p.Values[i]
		}
		if p.Segmented {
			a.contentChoice(b, target, pointer, label, option, kind, value == option)
		} else {
			b.open("option", option, "value", option)
			if value == option {
				b.attr("selected", "")
			}
			b.content(label)
			b.close()
		}
	}
	b.close()
}

func (a *ModuleEditor) contentChoice(b *editorVNodeView, target DocumentID, pointer, label, value, kind string, selected bool) {
	id := "content-choice:" + workspaceFieldID(target, pointer, "") + ":" + label
	if kind != "number" {
		value = encodeFormJSON(value)
	}
	a.fields[id] = workspaceField{Target: target, Pointer: pointer, Kind: "content-set", Value: value, Axis: -1, Selected: selected}
	b.button(id, label, "aria-pressed", strconv.FormatBool(selected))
}

func (a *ModuleEditor) contentSprite(b *editorVNodeView, target DocumentID, pointer string, p *workspaceContentProperty, sprite project.ModSpriteSpec) {
	texture := project.TextureSpec{Color: sprite.Color}
	if sprite.Texture != nil {
		texture = *sprite.Texture
	}
	colorPointer := ""
	if sprite.Texture == nil {
		colorPointer = pointer + "/color"
	}
	a.contentTexture(b, target, pointer+"/texture", texture, colorPointer)
	b.open("div", pointer+"-fields", "class", "content-fields")
	value := reflect.ValueOf(sprite)
	for i := range p.Fields {
		child := &p.Fields[i]
		if child.Name == "texture" || child.Name == "color" || child.Name == "size" {
			continue
		}
		// Optional authoring groups keep their own add/remove controls.
		path, v := pointer+"/"+child.Name, value.Field(child.Index)
		if child.Type.Kind() == reflect.Struct {
			a.contentGroup(b, target, path, child, v)
		} else {
			b.open("div", path, "class", "content-property")
			a.contentProperty(b, target, path, child, v)
			b.close()
		}
	}
	b.close()
}

func (a *ModuleEditor) contentTexture(b *editorVNodeView, target DocumentID, pointer string, texture project.TextureSpec, colorPointer string) {
	if detail := &a.animation.Detail; detail.ValuePointer != "" && detail.Target == target && detail.Pointer == pointer {
		pointer, texture, colorPointer = detail.ValuePointer, detail.Texture, ""
	}
	mode := "Color"
	if texture.Image != "" {
		mode = "Image"
	} else if texture.Atlas != "" {
		mode = "Atlas"
	}
	b.open("div", pointer+"-fields", "class", "content-fields")
	b.open("div", "source", "class", "content-property")
	b.text("div", "label", "Texture", "id", pointer+"-source-label", "class", "content-field-label")
	b.open("div", "modes", "class", "content-segments", "role", "group", "aria-labelledby", pointer+"-source-label")
	for _, option := range [...]string{"Image", "Atlas", "Color"} {
		id := "content-texture:" + workspaceFieldID(target, pointer, "") + ":" + option
		selected := a.contentTextureMode(target, pointer, mode) == option
		a.fields[id] = workspaceField{Target: target, Pointer: pointer, Kind: "content-texture", Value: option, Axis: -1, Selected: selected}
		// The empty image/atlas is an editable draft, so the chosen source is
		// recorded separately until its resource field has a value.
		b.button(id, option, "aria-pressed", strconv.FormatBool(selected))
	}
	b.close()
	b.close()
	mode = a.contentTextureMode(target, pointer, mode)
	switch mode {
	case "Image":
		a.contentTextureField(b, target, pointer+"/image", "Image", "string", texture.Image, "image")
	case "Atlas":
		a.contentTextureField(b, target, pointer+"/atlas", "Atlas", "string", texture.Atlas, "atlas")
		a.contentAtlasFrame(b, target, pointer, texture.Atlas, texture.Frame)
	case "Color":
		if colorPointer == "" {
			colorPointer = pointer + "/color"
		}
		a.contentTextureField(b, target, colorPointer, "Sprite Color", "color", texture.Color, "")
	}
	if mode != "Color" {
		a.contentTextureField(b, target, pointer+"/mipmap", "Mipmap", "bool", strconv.FormatBool(texture.Mipmap), "")
	}
	if mode != "Color" && strings.HasSuffix(pointer, "/sprite/texture") {
		b.open("div", "scale", "class", "content-property")
		b.text("div", "label", "Scale Mode", "id", pointer+"-scale-label", "class", "content-field-label")
		b.open("div", "modes", "class", "content-segments", "role", "group", "aria-labelledby", pointer+"-scale-label")
		for _, option := range [...]string{"Stretch", "9-Slice"} {
			id := "content-scale:" + workspaceFieldID(target, pointer+"/slice", "") + ":" + option
			kind, value := "content-remove", ""
			if option == "9-Slice" {
				kind = "content-nine-slice"
			}
			selected := (texture.Slice != nil) == (option == "9-Slice")
			a.fields[id] = workspaceField{Target: target, Pointer: pointer + "/slice", Kind: kind, Value: value, Axis: -1, Selected: selected}
			b.button(id, option, "aria-pressed", strconv.FormatBool(selected))
		}
		b.close()
		b.close()
		if texture.Slice != nil {
			id := "content-nine-slice:" + workspaceFieldID(target, pointer+"/slice", "")
			a.fields[id] = workspaceField{Target: target, Pointer: pointer + "/slice", Kind: "content-nine-slice", Axis: -1}
			b.button(id, "Edit 9-Slice...", "aria-haspopup", "dialog")
			b.open("div", "margins", "class", "content-margins")
			for _, field := range [...]struct {
				Name, Label string
				Value       int
			}{{"left", "Left", texture.Slice.Left}, {"right", "Right", texture.Slice.Right}, {"top", "Top", texture.Slice.Top}, {"bottom", "Bottom", texture.Slice.Bottom}} {
				a.contentTextureField(b, target, pointer+"/slice/"+field.Name, field.Label, "number", strconv.Itoa(field.Value), "")
			}
			b.close()
		}
	}
	b.close()
}

func (a *ModuleEditor) contentTextureField(b *editorVNodeView, target DocumentID, pointer, label, kind, value, resource string) {
	b.open("div", pointer, "class", "content-property")
	a.contentField(b, target, pointer, label, kind, value, resource)
	b.close()
}

// contentAtlasFrame offers the atlas's own regions instead of free text. An
// unreadable atlas or a frame it no longer declares keeps the editable field.
func (a *ModuleEditor) contentAtlasFrame(b *editorVNodeView, target DocumentID, pointer, atlas, frame string) {
	// contentOptions overlays drafts; keep the authored value as its baseline.
	originalFrame := frame
	if draft, ok := a.drafts[workspaceFieldID(target, pointer+"/atlas", "")]; ok {
		atlas = draft
	}
	if draft, ok := a.drafts[workspaceFieldID(target, pointer+"/frame", "")]; ok {
		frame = draft
	}
	options := a.atlasFrameNames(target, atlas)
	if len(options) == 0 || frame != "" && !slices.Contains(options, frame) {
		a.contentTextureField(b, target, pointer+"/frame", "Frame", "string", originalFrame, "")
		return
	}
	p := workspaceContentProperty{Label: "Frame", Options: options, Values: options}
	if frame == "" {
		p.Options = append([]string{"Choose a frame"}, options...)
		p.Values = append([]string{""}, options...)
	}
	b.open("div", pointer+"/frame", "class", "content-property")
	a.contentOptions(b, target, pointer+"/frame", &p, "string", originalFrame)
	b.close()
}

// workspaceAtlasFrames caches one atlas file's region names by its disk identity.
type workspaceAtlasFrames struct {
	size     int64
	modified time.Time
	names    []string
}

func (a *ModuleEditor) atlasFrameNames(target DocumentID, atlas string) []string {
	mod, ok := strings.CutSuffix(string(target), "/mod.json")
	if !ok || atlas == "" {
		return nil
	}
	file, err := project.ResolveModResourcePath(project.ModID(mod), atlas)
	if err != nil {
		return nil
	}
	name := a.Document.Absolute(file)
	info, err := os.Stat(name)
	if err != nil || info.IsDir() {
		return nil
	}
	if cached, ok := a.atlasFrames[name]; ok && cached.size == info.Size() && cached.modified.Equal(info.ModTime()) {
		return cached.names
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return nil
	}
	metadata, err := graphics.ParseAtlas(data)
	if err != nil || len(metadata.Frames) == 0 {
		return nil
	}
	names := make([]string, 0, len(metadata.Frames))
	for _, frame := range metadata.Frames {
		names = append(names, frame.Filename)
	}
	slices.Sort(names)
	if a.atlasFrames == nil {
		a.atlasFrames = make(map[string]workspaceAtlasFrames, 1)
	}
	a.atlasFrames[name] = workspaceAtlasFrames{size: info.Size(), modified: info.ModTime(), names: names}
	return names
}

func (a *ModuleEditor) contentTextureMode(target DocumentID, pointer, mode string) string {
	if selected := a.textureModes[workspaceFieldID(target, pointer, "")]; selected != "" {
		return selected
	}
	return mode
}

func (a *ModuleEditor) contentAction(id string) (bool, error) {
	if remove, ok := strings.CutPrefix(id, "content-menu:"); ok {
		a.componentPickerOpen = false
		a.elementPicker.Field = ""
		if a.componentMenu == remove {
			a.componentMenu = ""
		} else {
			a.componentMenu = remove
		}
		return true, nil
	}
	if field, ok := strings.CutPrefix(id, "content-color:"); ok {
		return true, a.openWorkspaceColorPicker(field)
	}
	if field, ok := strings.CutPrefix(id, "slot-reset:"); ok {
		return true, a.resetSlotField(field)
	}
	if fieldID, ok := strings.CutPrefix(id, "content-toggle:"); ok {
		field, exists := a.fields[fieldID]
		if !exists {
			return true, nil
		}
		value := field.Value
		if draft, exists := a.drafts[fieldID]; exists {
			value = draft
		}
		a.drafts[fieldID] = strconv.FormatBool(value != "true")
		return true, nil
	}
	if step, ok := strings.CutPrefix(id, "content-step:"); ok {
		index := strings.LastIndexByte(step, ':')
		if index < 0 {
			return true, nil
		}
		fieldID := step[:index]
		field, exists := a.fields[fieldID]
		if !exists {
			return true, nil
		}
		value := field.Value
		if draft, exists := a.drafts[fieldID]; exists {
			value = draft
		}
		direction, _ := strconv.Atoi(step[index+1:])
		// Integer masks and categories can exceed float64's exact range.
		if integer, ok := new(big.Int).SetString(value, 10); ok && field.NumberStep == 0 {
			a.drafts[fieldID] = integer.Add(integer, big.NewInt(int64(direction))).String()
		} else if next, ok := engineui.StepNumberInputValue(value, engineui.NumberInput{Step: field.NumberStep}, direction); ok {
			a.drafts[fieldID] = next
		}
		if field.CommitOnChange {
			a.handleEvent(htmlui.Event{ID: fieldID, Kind: htmlui.EventChange, Value: a.drafts[fieldID]})
		}
		return true, nil
	}
	field, exists := a.fields[id]
	if !exists {
		return false, nil
	}
	switch field.Kind {
	case "content-elements-open", "content-element-remove":
		return true, a.elementPickerAction(field)
	case "content-add":
		return true, a.addContentField(field)
	case "content-resource":
		return true, a.pickContentResource(field)
	case "content-nine-slice":
		if field.Selected {
			return true, nil
		}
		return true, a.openWorkspaceNineSlice(field)
	case "content-texture":
		if field.Selected {
			return true, nil
		}
		return true, a.setContentTextureSource(field)
	case "content-set", "content-remove":
		if field.Selected {
			return true, nil
		}
		patches, err := a.formPatches()
		if err != nil {
			return true, err
		}
		patches[field.Target] = append(patches[field.Target], DocumentPatch{Pointer: field.Pointer, Value: json.RawMessage(field.Value), Remove: field.Kind == "content-remove"})
		if err := a.stagePatches(patches); err != nil {
			return true, err
		}
		return true, nil
	}
	return false, nil
}

func (a *ModuleEditor) setContentTextureSource(field workspaceField) error {
	source := strings.ToLower(field.Value)
	// Change only the mutually exclusive sources. Pending mipmap, margins and
	// unrelated component edits remain in the same draft transaction.
	patches, err := a.formPatches()
	if err != nil {
		return err
	}
	for _, name := range [...]string{"image", "atlas", "frame", "color"} {
		patches[field.Target] = append(patches[field.Target], DocumentPatch{Pointer: field.Pointer + "/" + name, Remove: true})
	}
	if parent, ok := strings.CutSuffix(field.Pointer, "/sprite/texture"); ok {
		patches[field.Target] = append(patches[field.Target], DocumentPatch{Pointer: parent + "/sprite/color", Remove: true})
	}
	value := `""`
	if source == "color" {
		value = `"#ffffff"`
		patches[field.Target] = append(patches[field.Target], DocumentPatch{Pointer: field.Pointer + "/slice", Remove: true})
	}
	patches[field.Target] = append(patches[field.Target], DocumentPatch{Pointer: field.Pointer + "/" + source, Value: json.RawMessage(value)})
	if err := a.stagePatches(patches); err != nil {
		return err
	}
	if a.textureModes == nil {
		a.textureModes = make(map[string]string)
	}
	a.textureModes[workspaceFieldID(field.Target, field.Pointer, "")] = field.Value
	return nil
}

func (a *ModuleEditor) pickContentResource(field workspaceField) error {
	index := slices.IndexFunc(workspaceResourceTypes[:], func(t workspaceResourceType) bool { return t.Kind == field.Value })
	if index < 0 {
		return fmt.Errorf("unknown resource kind %q", field.Value)
	}
	mod := a.selectedMod()
	if mod == "" || field.Target != modDocumentID(mod) {
		return fmt.Errorf("Select the module you are editing before choosing a resource")
	}
	filter := workspaceResourceTypes[index].Filter
	base := a.Document.Absolute(string(mod))
	// Open the picker in the folder of the file already referenced so swapping to
	// a sibling needs no navigation.
	id := workspaceFieldID(field.Target, field.Pointer, "")
	current, drafted := a.drafts[id]
	if !drafted {
		current = a.controlValues[id]
	}
	if current != "" && (field.Value == "font" || field.Value == "html") {
		current = a.viewMod(mod).Resources[current].Path
	}
	directory := base
	if current != "" {
		directory = filepath.Dir(filepath.Join(base, filepath.FromSlash(current)))
	}
	file, err := a.chooseWorkspacePath(false, "Choose "+filter.Name, directory, filter)
	if err != nil || file == "" {
		return err
	}
	path, err := filepath.Rel(base, file)
	if err != nil {
		return err
	}
	path = filepath.ToSlash(path)
	if _, err := project.ResolveModResourcePath(mod, path); err != nil {
		return err
	}
	if !slices.Contains(filter.Extensions, strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")) {
		return fmt.Errorf("resource %s is not a %s file", path, filter.Name)
	}
	// References such as text.font and html.resource name declarations. Texture
	// sources use module-relative paths. Keep both forms in the current format.
	if field.Value == "font" || field.Value == "html" {
		name := ""
		for key, resource := range a.viewMod(mod).Resources {
			if resource.Kind == field.Value && resource.Path == path {
				name = key
				break
			}
		}
		if name == "" {
			return fmt.Errorf("Declare %s in Add Resources before selecting it", path)
		}
		path = name
	}
	a.drafts[id] = path
	return nil
}
