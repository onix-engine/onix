package editor

import (
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"unicode"

	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	engineui "github.com/diiyw/onix/ui"
)

// Cache authoring metadata once. Rendering reuses the editor's VNodes and field
// maps; an unchanged frame never walks types or serializes component objects.
type workspaceContentProperty struct {
	Name, Label string
	Index       int
	Type        reflect.Type
	Seed        reflect.Value
	Initial     string
	Fields      []workspaceContentProperty
	Axes        []string
	Options     []string
	Values      []string
	Kind        string
	Resource    string
	Segmented   bool
	Optional    bool
	Embedded    bool
}

func contentDefault[T any](defaults map[reflect.Type]reflect.Value, value T) {
	defaults[reflect.TypeFor[T]()] = reflect.ValueOf(value)
}

func contentSchema[T any]() workspaceContentProperty {
	defaults := make(map[reflect.Type]reflect.Value)
	contentDefault(defaults, scene.AnchorTopLeft)
	contentDefault(defaults, engineui.NewLayout())
	contentDefault(defaults, project.ModSpriteSpec{Color: "#ffffff", Size: []float32{100, 100}})
	trail := scene.NewSpriteTrail2D()
	contentDefault(defaults, project.SpriteTrailSpec{Lifetime: trail.Lifetime, Interval: trail.Interval, MinDistance: trail.MinDistance, AlphaStart: trail.AlphaStart, AlphaEnd: trail.AlphaEnd, MaxSamples: trail.MaxSamples})
	contentDefault(defaults, project.TextureSpec{Color: "#ffffff"})
	contentDefault(defaults, project.TextComponentSpec{Value: "Text", FontSize: 16, Color: "#ffffff"})
	contentDefault(defaults, project.AnimationComponentSpec{Speed: 1})
	contentDefault(defaults, project.ModCamera2DSpec{Scale: 1})
	contentDefault(defaults, project.NewCamera3DSpec())
	contentDefault(defaults, project.NewAmbientLightSpec())
	contentDefault(defaults, project.NewDirectionalLightSpec())
	contentDefault(defaults, project.NewPointLightSpec())
	contentDefault(defaults, project.NewSpotLightSpec())
	contentDefault(defaults, project.NewAreaLightSpec())
	contentDefault(defaults, *project.NewShadowSpec())
	contentDefault(defaults, *project.NewPunctualShadowSpec())
	contentDefault(defaults, *project.NewFogSpec())
	contentDefault(defaults, *project.NewSSAOSpec())
	contentDefault(defaults, *project.NewSSRSpec())
	contentDefault(defaults, project.NewRigidBody2DSpec())
	contentDefault(defaults, project.NewRigidBody3DSpec())
	contentDefault(defaults, project.NewCircleCollider2DSpec())
	contentDefault(defaults, project.NewBoxCollider2DSpec())
	contentDefault(defaults, project.NewSegmentCollider2DSpec())
	contentDefault(defaults, project.NewTriangleMeshCollider3DSpec())
	contentDefault(defaults, project.NewConvexHullCollider3DSpec())
	contentDefault(defaults, project.NewCompoundCollider3DSpec())
	contentDefault(defaults, project.ModelSpec{ID: "model", Mesh: &project.MeshSpec{Primitive: "cube", Size: 1}})
	contentDefault(defaults, project.MeshSpec{Primitive: "cube", Size: 1})
	contentDefault(defaults, project.ModSpineSpec{Speed: 1, Loop: true})
	contentDefault(defaults, project.EmitterSpec{Count: 100, Rate: 10, Lifetime: 1, SizeMin: 1, SizeMax: 5, AlphaStart: 1, Loop: true})
	contentDefault(defaults, project.TilemapSpec{ID: "tilemap", Width: 1, Height: 1, TileWidth: 32, TileHeight: 32, Tiles: []int{0}})
	return buildContentProperty(reflect.TypeFor[T](), "", "", defaults)
}

var workspaceContentSchema = contentSchema[project.ModComponents]()

var workspaceElementSpace = workspaceContentProperty{
	Label: "Coordinate Space", Options: []string{"Inherit", "World", "UI"},
	Values: []string{"", "world", "ui"}, Segmented: true,
}

var workspaceModuleSpace = workspaceContentProperty{
	Label: "Coordinate Space", Options: []string{"UI", "World"},
	Values: []string{"ui", "world"}, Segmented: true,
}

func buildContentProperty(t reflect.Type, name, label string, defaults map[reflect.Type]reflect.Value) workspaceContentProperty {
	p := workspaceContentProperty{Name: name, Label: label, Optional: t.Kind() == reflect.Pointer}
	if p.Optional {
		t = t.Elem()
	}
	p.Type = t
	p.Seed = defaults[t]
	if !p.Seed.IsValid() {
		p.Seed = reflect.Zero(t)
	}
	if p.Optional && name == "alpha" {
		p.Seed = reflect.ValueOf(float32(1))
	}
	if p.Optional && name == "clear" {
		p.Seed = reflect.ValueOf(true)
	}
	if name == "size" && t.Kind() == reflect.Slice {
		p.Seed = reflect.ValueOf([]float32{100, 100})
		p.Axes = axesWH
	} else if t.Kind() == reflect.Array && t.Len() >= 2 && t.Len() <= 4 && (t.Elem().Kind() == reflect.Float32 || t.Elem().Kind() == reflect.Float64) {
		p.Axes = []string{"X", "Y", "Z", "W"}[:t.Len()]
	}
	if t.Kind() == reflect.Struct {
		for i := range t.NumField() {
			f := t.Field(i)
			key, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if !f.IsExported() || key == "-" {
				continue
			}
			if t == reflect.TypeFor[project.ModelMaterialSpec]() && f.Name == "Shader" {
				continue
			}
			embedded := f.Anonymous && key == ""
			if key == "" && !embedded {
				key = f.Name
			}
			child := buildContentProperty(f.Type, key, contentLabel(f.Name), defaults)
			child.Index, child.Embedded = i, embedded
			configureContentProperty(t, &child)
			p.Fields = append(p.Fields, child)
		}
	}
	data, err := json.Marshal(p.Seed.Interface())
	if err != nil {
		panic(err)
	}
	p.Initial = string(data)
	switch t {
	case reflect.TypeFor[engineui.LayoutType]():
		p.Options = []string{"None", "Horizontal", "Vertical", "Grid"}
	case reflect.TypeFor[engineui.LayoutResizeMode]():
		p.Options = []string{"None", "Children", "Container"}
	case reflect.TypeFor[engineui.LayoutHorizontalDirection]():
		p.Options = []string{"Left to right", "Right to left"}
	case reflect.TypeFor[engineui.LayoutVerticalDirection]():
		p.Options = []string{"Top to bottom", "Bottom to top"}
	case reflect.TypeFor[engineui.LayoutStartAxis]():
		p.Options = []string{"Horizontal", "Vertical"}
	case reflect.TypeFor[engineui.LayoutConstraint]():
		p.Options = []string{"None", "Fixed row", "Fixed column"}
	}
	if t == reflect.TypeFor[scene.Anchor]() {
		p.Axes = []string{"Anchor X", "Anchor Y"}
	}
	if t.Kind() == reflect.String && strings.HasSuffix(strings.ToLower(name), "color") {
		p.Kind = "color"
	}
	return p
}

func configureContentProperty(parent reflect.Type, p *workspaceContentProperty) {
	if parent == reflect.TypeFor[project.ModComponents]() && p.Name == "text" {
		p.Label = "Label"
	}
	switch p.Name {
	case "font", "boldFont":
		p.Resource = "font"
	case "image", "atlas":
		p.Resource = p.Name
	}
	// Every file reference is authored through the native picker, keyed by the
	// spec that declares it because the same JSON name means different files.
	switch parent {
	case reflect.TypeFor[project.ShaderComponentSpec]():
		if p.Type.Kind() == reflect.String {
			p.Resource = "shader"
		}
	case reflect.TypeFor[project.ModelSpec]():
		if p.Name == "path" || p.Name == "animationPath" || p.Name == "collisionPath" {
			p.Resource = "model"
		}
	case reflect.TypeFor[project.SpineSpec]():
		switch p.Name {
		case "skeleton":
			p.Resource = "spine"
		case "texture":
			p.Resource = "image"
		}
	case reflect.TypeFor[project.TilemapSpec]():
		if p.Name == "tileset" {
			p.Resource = "image"
		}
	case reflect.TypeFor[project.EnvironmentSpec]():
		if p.Name == "path" {
			p.Resource = "image"
		}
	}
	if parent == reflect.TypeFor[project.ModHTMLSpec]() && p.Name == "resource" {
		p.Resource, p.Label = "html", "HTML"
	}
	if parent == reflect.TypeFor[project.TextComponentSpec]() && p.Name == "value" {
		p.Kind, p.Label = "text", "Text"
	}
	if (parent == reflect.TypeFor[project.RigidBody2DSpec]() || parent == reflect.TypeFor[project.RigidBody3DSpec]()) && p.Name == "type" {
		p.Options = []string{"Dynamic", "Static", "Kinematic"}
		p.Values = []string{"dynamic", "static", "kinematic"}
		p.Segmented = true
	}
	if parent == reflect.TypeFor[project.EmitterSpec]() && p.Name == "particles" {
		p.Kind = "elements"
	}
	if parent == reflect.TypeFor[engineui.Layout]() && p.Name == "cellSize" {
		p.Axes = []string{"Cell Width", "Cell Height"}
	}
	if parent == reflect.TypeFor[project.TilemapSpec]() && p.Name == "tiles" {
		p.Kind = "tilemap-tiles"
	}
}

func contentLabel(name string) string {
	runes := []rune(name)
	var label strings.Builder
	for i, r := range runes {
		if i > 0 && (unicode.IsDigit(r) && unicode.IsLetter(runes[i-1]) || unicode.IsUpper(r) && (unicode.IsLower(runes[i-1]) || i+1 < len(runes) && unicode.IsLower(runes[i+1]))) {
			label.WriteByte(' ')
		}
		label.WriteRune(r)
	}
	return label.String()
}

// visibilityHeader draws the name with an eye button that toggles a bool field.
func (a *ModuleEditor) visibilityHeader(b *editorVNodeView, target DocumentID, pointer, name string, visible bool) {
	id, value := a.contentFieldValue(target, pointer, "bool", strconv.FormatBool(visible))
	icon, label := "onix-rightbar/eye-off.svg", "Show"
	if value == "true" {
		icon, label = "onix-rightbar/eye.svg", "Hide"
	}
	control := "content-toggle:" + id
	b.open("header", "detail-header", "class", "detail-header")
	b.text("h2", "element-title", name, "class", "element-title")
	b.open("button", control, "id", control, "type", "button", "class", "detail-visibility", "aria-label", label, "title", label)
	b.text("img", "icon", "", "src", icon, "alt", "")
	b.close()
	b.close()
}

// elementHeader shows the element name with an eye button that toggles visibility.
func (a *ModuleEditor) elementHeader(b *editorVNodeView, target DocumentID, base string, element project.ModElementSpec) {
	a.visibilityHeader(b, target, base+"/visible", element.ID, element.Visible == nil || *element.Visible)
}

// elementInspector groups identity, size and anchor before transform and components.
func (a *ModuleEditor) elementInspector(b, picker *editorVNodeView, target DocumentID, index int, element project.ModElementSpec, elements []project.ModElementSpec) {
	base := "/elements/" + strconv.Itoa(index)
	// Texture tracks can provide an implicit sprite without an authored component.
	var implicitSprite project.ModSpriteSpec
	implicit := a.animation.Detail.ValuePointer != "" && a.animation.Detail.Pointer == base+"/components/sprite/texture" && element.Components.Sprite == nil
	if implicit {
		element.Components.Sprite = &implicitSprite
	}
	a.elementHeader(b, target, base, element)
	a.section(b, "element-identity", "Element")
	b.open("div", "identity", "class", "content-fields")
	b.open("div", "name", "class", "content-property")
	a.contentField(b, target, base+"/id", "Name", "string", element.ID, "")
	b.close()
	b.open("div", "parent", "class", "content-property")
	a.contentParentField(b, target, base+"/parent", element.ID, element.Parent, elements)
	b.close()
	pointer := base + "/components"
	value := reflect.ValueOf(element.Components)
	var available, attached [32]*workspaceContentProperty
	var animationProperty *workspaceContentProperty
	count, attachedCount := 0, 0
	for i := range workspaceContentSchema.Fields {
		p := &workspaceContentSchema.Fields[i]
		if p.Name == "animation" {
			animationProperty = p
		}
		v := value.Field(p.Index)
		path := pointer + "/" + p.Name
		if p.Name == "size" || p.Name == "anchor" {
			if p.Name == "size" {
				if sprite := element.Components.Sprite; sprite != nil && len(sprite.Size) == 2 {
					path, v = pointer+"/sprite/size", reflect.ValueOf(sprite.Size)
				} else if v.IsNil() {
					v = reflect.ValueOf([2]float32{})
				}
			} else if v.IsNil() {
				anchor := scene.AnchorTopLeft
				space := element.Space
				if space == "" {
					space = a.Document.Mods[a.selectedMod()].Spec.Space
				}
				if space == project.CoordinateSpaceWorld {
					anchor = scene.AnchorCenter
				}
				v = reflect.ValueOf(anchor)
			}
			b.open("div", path, "class", "content-property")
			a.contentProperty(b, target, path, p, v)
			b.close()
			continue
		}
		if v.IsNil() {
			available[count] = p
			count++
			continue
		}
		attached[attachedCount] = p
		attachedCount++
	}
	b.open("div", "space", "class", "content-property")
	a.contentOptions(b, target, base+"/space", &workspaceElementSpace, "string", string(element.Space))
	b.close()
	b.close()
	a.sectionEnd(b)
	a.section(b, "element-transform", "Transform")
	a.transform(b, target, base+"/transform", element.Transform)
	a.sectionEnd(b)
	for _, p := range attached[:attachedCount] {
		path := pointer + "/" + p.Name
		if implicit && p.Name == "sprite" {
			a.section(b, "content:"+path, p.Label)
		} else {
			a.sectionWithRemove(b, "content:"+path, p.Label, "remove:"+url.PathEscape(string(target))+"|"+path)
		}
		a.contentProperty(b, target, path, p, value.Field(p.Index))
		a.sectionEnd(b)
	}
	if detail := &a.animation.Detail; detail.Animation != nil && index != a.animation.ElementIndex {
		path := detail.AnimationPointer
		a.sectionWithRemove(b, "content:"+path, animationProperty.Label, "remove:"+url.PathEscape(string(target))+"|"+path)
		a.contentProperty(b, target, path, animationProperty, reflect.ValueOf(detail.Animation))
		a.sectionEnd(b)
	}
	b.open("div", "component-actions", "class", "content-actions")
	b.open("button", "content-picker", "id", "content-picker", "type", "button", "class", "content-add", "aria-haspopup", "menu", "aria-expanded", strconv.FormatBool(a.componentPickerOpen), "aria-controls", "content-picker-list")
	b.text("img", "icon", "", "src", "onix-rightbar/plus.svg", "alt", "")
	b.text("span", "label", "Add Component")
	b.close()
	b.close()
	picker.open("div", "content-picker-list", "id", "content-picker-list", "class", "content-picker", "role", "menu", "aria-labelledby", "content-picker", "style", workspaceRectStyle(a.contentPickerBounds(a.componentPickerAnchor, count)))
	if !a.componentPickerOpen {
		picker.attr("hidden", "")
	}
	if count == 0 {
		picker.text("p", "empty", "All components are already added.", "class", "muted")
	}
	for _, p := range available[:count] {
		a.contentAddButton(picker, target, pointer+"/"+p.Name, p, p.Label, "role", "menuitem")
	}
	picker.close()
}

// definitionComponents edits the components authored on a module's own root.
// Sprite authoring belongs to elements.
func (a *ModuleEditor) definitionComponents(b, picker *editorVNodeView, target DocumentID, content project.ModComponents) {
	value := reflect.ValueOf(content)
	var available, attached [32]*workspaceContentProperty
	count, attachedCount := 0, 0
	for i := range workspaceContentSchema.Fields {
		p := &workspaceContentSchema.Fields[i]
		switch p.Name {
		case "size", "anchor", "animation", "sprite", "shader":
			continue
		}
		v := value.Field(p.Index)
		if v.IsNil() {
			available[count] = p
			count++
			continue
		}
		attached[attachedCount] = p
		attachedCount++
	}
	for _, p := range attached[:attachedCount] {
		path := "/components/" + p.Name
		a.sectionWithRemove(b, "content:"+path, p.Label, "remove:"+url.PathEscape(string(target))+"|"+path)
		a.contentProperty(b, target, path, p, value.Field(p.Index))
		a.sectionEnd(b)
	}
	b.open("div", "component-actions", "class", "content-actions")
	b.open("button", "content-picker", "id", "content-picker", "type", "button", "class", "content-add", "aria-haspopup", "menu", "aria-expanded", strconv.FormatBool(a.componentPickerOpen), "aria-controls", "content-picker-list")
	b.text("img", "icon", "", "src", "onix-rightbar/plus.svg", "alt", "")
	b.text("span", "label", "Add Component")
	b.close()
	b.close()
	picker.open("div", "content-picker-list", "id", "content-picker-list", "class", "content-picker", "role", "menu", "aria-labelledby", "content-picker", "style", workspaceRectStyle(a.contentPickerBounds(a.componentPickerAnchor, count)))
	if !a.componentPickerOpen {
		picker.attr("hidden", "")
	}
	if count == 0 {
		picker.text("p", "empty", "All components are already added.", "class", "muted")
	}
	for _, p := range available[:count] {
		a.contentAddButton(picker, target, "/components/"+p.Name, p, p.Label, "role", "menuitem")
	}
	picker.close()
}

func (a *ModuleEditor) contentAddButton(b *editorVNodeView, target DocumentID, pointer string, p *workspaceContentProperty, label string, attrs ...string) {
	id := "content-add:" + url.PathEscape(string(target)) + "|" + pointer
	a.fields[id] = workspaceField{Target: target, Pointer: pointer, Kind: "content-add", Value: p.Initial, Axis: -1}
	b.open("button", id, "id", id, "type", "button")
	for i := 0; i < len(attrs); i += 2 {
		b.attr(attrs[i], attrs[i+1])
	}
	b.text("img", "icon", "", "src", workspaceSectionIcon("content:/components/"+p.Name), "alt", "")
	b.text("span", "label", label)
	b.close()
}

func (a *ModuleEditor) contentVectorInputs(b *editorVNodeView, target DocumentID, pointer string, p *workspaceContentProperty, value reflect.Value) {
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	var seed [4]float64
	for i := range p.Axes {
		if i < value.Len() {
			seed[i] = value.Index(i).Float()
		}
	}
	a.numericVectorInputs(b, target, pointer, seed, p.Axes, p.Type.Elem().Bits(), false)
}

// contentFieldCompact reports whether a leaf field is a bare number that can
// share a row with its neighbour, keeping the Details panel dense.
func contentFieldCompact(p *workspaceContentProperty) bool {
	if len(p.Axes) != 0 || len(p.Options) != 0 || p.Kind == "color" {
		return false
	}
	switch p.Type.Kind() {
	case reflect.Float32, reflect.Float64,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	}
	return false
}

func (a *ModuleEditor) contentProperty(b *editorVNodeView, target DocumentID, pointer string, p *workspaceContentProperty, value reflect.Value) {
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			value = p.Seed
		} else {
			value = value.Elem()
		}
	}
	if p.Kind == "tilemap-tiles" {
		a.tilemapSummaryField(b, target, pointer, p, value)
		return
	}
	if len(p.Axes) != 0 {
		if p.Name != "size" && p.Name != "anchor" && p.Name != "cellSize" {
			b.text("div", pointer+"-label", p.Label, "class", "content-field-label")
		}
		a.contentVectorInputs(b, target, pointer, p, value)
		return
	}
	if p.Type.Kind() == reflect.Struct {
		if p.Type == reflect.TypeFor[project.AnimationComponentSpec]() {
			animation := value.Interface().(project.AnimationComponentSpec)
			b.open("div", pointer+"-fields", "class", "content-fields")
			b.open("div", "properties", "class", "content-animation-fields")
			if detail := &a.animation.Detail; detail.ValuePointer != "" && target == detail.Target && pointer == detail.AnimationPointer {
				a.animationControl(b, "key-time", "Keyframe time (s)", "number", animationNumber(detail.Time))
			}
			b.open("div", "duration", "class", "content-property")
			a.field(b, target, pointer+"/clip/duration", "Duration (s)", "number", animationNumber(animation.Clip.Duration))
			b.close()
			b.open("div", "sample-rate", "class", "content-property")
			a.field(b, target, pointer+"/clip/sampleRate", "Sample Rate", "number", animationNumber(animation.Clip.SampleRate))
			b.close()
			b.open("div", "speed", "class", "content-property")
			a.field(b, target, pointer+"/speed", "Speed", "number", animationNumber(animation.Speed))
			b.close()
			b.open("div", "loop", "class", "content-property")
			a.field(b, target, pointer+"/clip/loop", "Loop", "bool", strconv.FormatBool(animation.Clip.Loop))
			b.close()
			b.close()
			b.open("div", "actions", "class", "content-animation-actions")
			b.text("p", "tracks", fmt.Sprintf("%d tracks · %d events", len(animation.Clip.FloatTracks)+len(animation.Clip.TextureTracks)+len(animation.Clip.BoolTracks), len(animation.Clip.Events)), "class", "muted")
			b.button("animation:open", "Edit Animation")
			b.close()
			b.close()
			return
		}
		if p.Type == reflect.TypeFor[project.ModSpriteSpec]() {
			a.contentSprite(b, target, pointer, p, value.Interface().(project.ModSpriteSpec))
			return
		}
		if p.Type == reflect.TypeFor[project.TextureSpec]() {
			a.contentTexture(b, target, pointer, value.Interface().(project.TextureSpec), "")
			return
		}
		class := "content-fields content-component-fields"
		if p.Type == reflect.TypeFor[project.ModInputSpec]() {
			class = "content-fields content-input-fields"
		}
		b.open("div", pointer+"-fields", "class", class)
		for i := range p.Fields {
			child := &p.Fields[i]
			v := value.Field(child.Index)
			if child.Embedded {
				a.contentProperty(b, target, pointer, child, v)
				continue
			}
			path := pointer + "/" + child.Name
			if child.Type.Kind() == reflect.Struct {
				a.contentGroup(b, target, path, child, v)
			} else {
				property := "content-property"
				if contentFieldCompact(child) {
					property += " content-compact"
				}
				b.open("div", path, "class", property)
				a.contentProperty(b, target, path, child, v)
				b.close()
			}
		}
		b.close()
		return
	}
	kind, text := "string", ""
	switch p.Type.Kind() {
	case reflect.String:
		text = value.String()
	case reflect.Bool:
		kind, text = "bool", strconv.FormatBool(value.Bool())
	case reflect.Float32, reflect.Float64:
		kind, text = "number", strconv.FormatFloat(value.Float(), 'f', -1, p.Type.Bits())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		kind, text = "number", strconv.FormatInt(value.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		kind, text = "number", strconv.FormatUint(value.Uint(), 10)
	default:
		kind, text = "json", encodeFormJSON(value.Interface())
	}
	if p.Kind != "" {
		kind = p.Kind
	}
	if kind == "elements" {
		a.contentElementsField(b, target, pointer, p.Label, text)
		return
	}
	if len(p.Options) != 0 {
		a.contentOptions(b, target, pointer, p, kind, text)
	} else {
		a.contentField(b, target, pointer, p.Label, kind, text, p.Resource)
		if kind == "number" && (p.Type.Kind() == reflect.Float32 || p.Type.Kind() == reflect.Float64) {
			id := workspaceFieldID(target, pointer, "")
			field := a.fields[id]
			field.NumberStep = .1
			a.fields[id] = field
		}
	}
}

func (a *ModuleEditor) tilemapSummaryField(b *editorVNodeView, target DocumentID, pointer string, p *workspaceContentProperty, value reflect.Value) {
	painted, total := 0, value.Len()
	for i := 0; i < total; i++ {
		if value.Index(i).Int() > 0 {
			painted++
		}
	}
	b.text("div", pointer+"-label", p.Label, "class", "content-field-label")
	b.text("p", pointer+"-summary", fmt.Sprintf("%d of %d tiles painted · edit in the viewport", painted, total), "class", "muted")
}

func (a *ModuleEditor) contentGroup(b *editorVNodeView, target DocumentID, pointer string, p *workspaceContentProperty, value reflect.Value) {
	b.open("fieldset", pointer, "class", "content-properties")
	b.open("legend", "label", "class", "content-properties-title")
	b.text("span", "label", p.Label)
	if p.Optional && !value.IsNil() {
		detailRemoveButton(b, "remove:"+url.PathEscape(string(target))+"|"+pointer, p.Label)
	}
	b.close()
	if p.Optional && value.IsNil() {
		a.contentAddButton(b, target, pointer, p, "Add "+p.Label, "class", "content-add")
	} else {
		a.contentProperty(b, target, pointer, p, value)
	}
	b.close()
}

func (a *ModuleEditor) addContentField(field workspaceField) error {
	patches, err := a.formPatches()
	if err != nil {
		return err
	}
	if field.Target != modDocumentID(a.selectedMod()) {
		return fmt.Errorf("Select a module before adding components")
	}
	if a.selectedElement != "" {
		if strings.HasSuffix(field.Pointer, "/components/animation") {
			spec := a.viewMod(a.selectedMod())
			index, ok := a.selectedElementIndex(spec.Elements)
			if !ok {
				return fmt.Errorf("Select a module element before adding animation")
			}
			var x float32
			if position := spec.Elements[index].Transform.Position; len(position) > 0 {
				x = position[0]
			}
			clip := project.DefaultAnimationClip(project.SceneNodeSpec{X: x})
			field.Value = encodeFormJSON(project.AnimationComponentSpec{Speed: 1, Clip: clip})
		}
	} else if !strings.HasPrefix(field.Pointer, "/components/") {
		return fmt.Errorf("Select a module element before adding components")
	}
	patches[field.Target] = append(patches[field.Target], DocumentPatch{Pointer: field.Pointer, Value: json.RawMessage(field.Value)})
	if err := a.stagePatches(patches); err != nil {
		return err
	}
	a.componentPickerOpen = false
	return nil
}
