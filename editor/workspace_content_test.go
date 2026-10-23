package editor

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/internal/dom"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	engineui "github.com/diiyw/onix/ui"
	htmlui "github.com/diiyw/onix/ui/html"
	"github.com/diiyw/onix/window"
)

func contentEditorFixture(t *testing.T, content project.ModComponents) *ModuleEditor {
	t.Helper()
	a := workspaceEditorFixture(t)
	if err := a.Document.EditMod("button", "Element fixture", func(mod *project.ModSpec) error {
		mod.Elements = []project.ModElementSpec{{ID: "face", Transform: project.ModTransformSpec{Position: []float32{4, 5}}, Components: content}, {ID: "other"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	a.enterModuleEdit("main.json", "start")
	a.selectElement("start", "face")
	workspaceShellTest(t, a)
	return a
}

func TestWorkspaceContentAddEditRemoveAndHistory(t *testing.T) {
	a := contentEditorFixture(t, project.ModComponents{})
	target := modDocumentID("button")
	pointer := "/elements/0/components"
	click := func(id string) {
		t.Helper()
		a.handleEvent(htmlui.Event{ID: id, Kind: htmlui.EventClick})
		workspaceShellTest(t, a)
	}
	add := func(name string) { click("content-add:" + url.PathEscape(string(target)) + "|" + pointer + "/" + name) }
	edit := func(path, axis, text string) {
		a.handleEvent(htmlui.Event{ID: workspaceFieldID(target, pointer+path, axis), Kind: htmlui.EventInput, Value: text})
	}
	click("content-picker")
	source := workspaceShellTest(t, a)
	if !a.componentPickerOpen || !strings.Contains(source, ">Add Component<") || strings.Contains(source, "Content configuration (JSON)") {
		t.Fatal("component picker did not replace the content JSON editor")
	}
	add("sprite")
	if a.componentPickerOpen || a.Document.Mods["button"].Spec.Elements[0].Components.Sprite != nil || !a.hasDrafts() {
		t.Fatal("adding a component must stage it until save and close the picker")
	}
	if _, exists := a.fields["content-add:"+url.PathEscape(string(target))+"|"+pointer+"/sprite"]; exists {
		t.Fatal("attached sprite is still available in the add list")
	}
	edit("/sprite/color", "", "#123456")
	edit("/sprite/size", "Width", "212.5")
	edit("/sprite/alpha", "", "0.5")
	// Adding another component must retain pending scalar and vector edits.
	add("input")
	a.handleEvent(htmlui.Event{ID: workspaceFieldID(target, pointer+"/input/pointer", ""), Kind: htmlui.EventChange, Checked: true})
	add("layout")
	typeID := workspaceFieldID(target, pointer+"/layout/type", "")
	if !strings.Contains(workspaceShellTest(t, a), `<select id="`+typeID+`"`) {
		t.Fatal("layout type did not use a selector")
	}
	a.handleEvent(htmlui.Event{ID: typeID, Kind: htmlui.EventChange, Value: "1"})
	a.action("save")
	workspaceShellTest(t, a)
	content := a.Document.Mods["button"].Spec.Elements[0].Components
	if a.hasDrafts() || content.Sprite == nil || content.Sprite.Color != "#123456" || content.Sprite.Size[0] != 212.5 || content.Sprite.Size[1] != 100 || content.Sprite.Alpha == nil || *content.Sprite.Alpha != .5 || content.Input == nil || !content.Input.Pointer {
		t.Fatalf("component edits were not saved: %+v (%s)", content, a.status)
	}
	if content.Layout == nil || content.Layout.Type != engineui.LayoutHorizontal {
		t.Fatal("layout selector did not save the selected enum")
	}
	mod := a.Document.Mods["button"].Spec
	if mod.Elements[0].Transform.Position[0] != 4 || mod.Elements[1].Components.Sprite != nil || mod.Components.Sprite.Color != "#3264d9" {
		t.Fatal("component edits changed unrelated content")
	}
	reloaded, err := LoadWorkspaceDocument(a.Document.ProjectDir)
	if err != nil || reloaded.Mods["button"].Spec.Elements[0].Components.Sprite.Color != "#123456" {
		t.Fatalf("saved components did not reload: %v", err)
	}
	click("remove:" + url.PathEscape(string(target)) + "|" + pointer + "/sprite")
	if a.viewMod("button").Elements[0].Components.Sprite != nil || a.Document.Mods["button"].Spec.Elements[0].Components.Sprite == nil {
		t.Fatal("removing a component did not remain a draft")
	}
	a.action("save")
	a.action("undo")
	if a.Document.Mods["button"].Spec.Elements[0].Components.Sprite == nil {
		t.Fatal("undo did not restore the removed component")
	}
	a.action("redo")
	if a.Document.Mods["button"].Spec.Elements[0].Components.Sprite != nil || a.Document.Mods["button"].Spec.Elements[0].Components.Input == nil {
		t.Fatal("redo did not remove only the selected component")
	}
}

func TestWorkspaceModuleRootComponentsEditInDetails(t *testing.T) {
	a := workspaceEditorFixture(t)
	if err := a.Document.EditMod("button", "Particle module", func(mod *project.ModSpec) error {
		mod.Elements = []project.ModElementSpec{{ID: "spark", Components: project.ModComponents{Particle: &project.TextureSpec{Color: "#ffffff"}}}}
		mod.Components.Emitter = &project.EmitterSpec{Particles: []string{"spark"}, Count: 100, Lifetime: 1, Rate: 10, SizeMin: 1, SizeMax: 5, AlphaStart: 1, Loop: true}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	a.enterModuleEdit("main.json", "start")
	workspaceShellTest(t, a)
	target, base := modDocumentID("button"), "/components"
	fieldID := workspaceFieldID(target, base+"/emitter/count", "")
	if a.controlValues[fieldID] != "100" {
		t.Fatalf("Details does not edit the module's own emitter: %q", a.controlValues[fieldID])
	}
	a.handleEvent(htmlui.Event{ID: fieldID, Kind: htmlui.EventInput, Value: "250"})
	a.action("save")
	if a.hasDrafts() || a.Document.Mods["button"].Spec.Components.Emitter.Count != 250 {
		t.Fatalf("emitter edit did not save: %s", a.status)
	}
	a.action("content-picker")
	workspaceShellTest(t, a)
	a.handleEvent(htmlui.Event{ID: "content-add:" + url.PathEscape(string(target)) + "|" + base + "/text", Kind: htmlui.EventClick})
	if !a.hasDrafts() {
		t.Fatal("adding a root component did not stage a draft")
	}
	a.action("save")
	if a.viewMod("button").Components.Text == nil {
		t.Fatal("added root component did not save")
	}
	a.handleEvent(htmlui.Event{ID: "remove:" + url.PathEscape(string(target)) + "|" + base + "/emitter", Kind: htmlui.EventClick})
	a.action("save")
	if components := a.Document.Mods["button"].Spec.Components; components.Emitter != nil || components.Text == nil {
		t.Fatalf("removing the root emitter removed its siblings: %+v", components)
	}
}

func TestWorkspaceContentNestedPropertiesAndValidation(t *testing.T) {
	a := contentEditorFixture(t, project.ModComponents{Sprite: &project.ModSpriteSpec{Color: "#ffffff", Size: []float32{40, 20}}})
	target := modDocumentID("button")
	base := "/elements/0/components/sprite"
	a.handleEvent(htmlui.Event{ID: "content-texture:" + workspaceFieldID(target, base+"/texture", "") + ":Image", Kind: htmlui.EventClick})
	workspaceShellTest(t, a)
	a.handleEvent(htmlui.Event{ID: "content-texture:" + workspaceFieldID(target, base+"/texture", "") + ":Color", Kind: htmlui.EventClick})
	workspaceShellTest(t, a)
	sprite := a.viewMod("button").Elements[0].Components.Sprite
	if sprite.Texture == nil || sprite.Color != "" {
		t.Fatal("adding a texture did not switch away from solid sprite color")
	}
	id := workspaceFieldID(target, base+"/texture/color", "")
	a.handleEvent(htmlui.Event{ID: id, Kind: htmlui.EventInput, Value: "#456789"})
	a.action("save")
	if a.hasDrafts() || a.Document.Mods["button"].Spec.Elements[0].Components.Sprite.Texture.Color != "#456789" {
		t.Fatalf("nested texture fields did not save: %s", a.status)
	}
	workspaceShellTest(t, a)
	id = workspaceFieldID(target, base+"/alpha", "")
	a.handleEvent(htmlui.Event{ID: id, Kind: htmlui.EventInput, Value: "2"})
	a.action("save")
	if !a.hasDrafts() || !strings.Contains(a.status, "alpha must be between") || a.Document.Mods["button"].Spec.Elements[0].Components.Sprite.Alpha != nil {
		t.Fatal("invalid component value was accepted or its draft was lost")
	}
	a.handleEvent(htmlui.Event{ID: id, Kind: htmlui.EventInput, Value: "0.75"})
	a.action("save")
	workspaceShellTest(t, a)
	if a.hasDrafts() || *a.Document.Mods["button"].Spec.Elements[0].Components.Sprite.Alpha != .75 {
		t.Fatalf("corrected alpha did not save: %s", a.status)
	}
	a.handleEvent(htmlui.Event{ID: "remove:" + url.PathEscape(string(target)) + "|" + base + "/texture", Kind: htmlui.EventClick})
	a.action("save")
	if a.Document.Mods["button"].Spec.Elements[0].Components.Sprite.Texture != nil {
		t.Fatal("nested texture removal did not save")
	}
}

func TestWorkspaceContentTypedVectorsAndStableSync(t *testing.T) {
	a := contentEditorFixture(t, project.ModComponents{Anchor: &scene.AnchorTopLeft})
	if err := a.Document.EditMod("button", "World physics", func(mod *project.ModSpec) error {
		mod.Space = project.CoordinateSpaceWorld
		body := project.NewRigidBody2DSpec()
		body.Velocity = [2]float64{1, 1.123456789012345}
		collider := project.NewBoxCollider2DSpec()
		collider.Width, collider.Height = 40, 20
		mod.Elements[0].Components.RigidBody2D = &body
		mod.Elements[0].Components.BoxCollider2D = &collider
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	workspaceShellTest(t, a)
	target := modDocumentID("button")
	base := "/elements/0/components"
	velocity := workspaceFieldID(target, base+"/rigidBody2D/velocity", "X")
	a.handleEvent(htmlui.Event{ID: velocity, Kind: htmlui.EventInput, Value: "2.123456789012345"})
	// Embedded collider fields use the flattened JSON path and preserve uint64.
	mask := workspaceFieldID(target, base+"/boxCollider2D/mask", "")
	if a.fields[mask].Value != "18446744073709551615" {
		t.Fatal("embedded collider fields were not flattened or lost integer precision")
	}
	a.handleEvent(htmlui.Event{ID: "content-step:" + mask + ":-1", Kind: htmlui.EventClick})
	a.handleEvent(htmlui.Event{ID: "content-choice:" + workspaceFieldID(target, base+"/rigidBody2D/type", "") + ":Static", Kind: htmlui.EventClick})
	a.action("save")
	if a.hasDrafts() {
		t.Fatalf("physics field edits failed: %s", a.status)
	}
	content := a.Document.Mods["button"].Spec.Elements[0].Components
	if content.RigidBody2D.Type != "static" || content.RigidBody2D.Velocity != [2]float64{2.123456789012345, 1.123456789012345} || content.BoxCollider2D.Mask != 18446744073709551614 {
		t.Fatal("numeric component fields lost precision")
	}
	workspaceShellTest(t, a)
	a.handleEvent(htmlui.Event{ID: "toggle:content:" + base + "/rigidBody2D", Kind: htmlui.EventClick})
	workspaceShellTest(t, a)
	context := &ecs.SystemContext{World: a.world}
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if err := a.sync(context); err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatalf("unchanged component panel allocated %g times", allocations)
	}
}

func TestWorkspaceContentLegacyPanelsAndControls(t *testing.T) {
	layout := engineui.NewLayout()
	a := contentEditorFixture(t, project.ModComponents{
		Sprite: &project.ModSpriteSpec{Color: "#ffffff", Size: []float32{40, 20}},
		Text:   &project.TextComponentSpec{Value: "First line\nSecond line", FontSize: 16, Color: "#112233"},
		Layout: &layout,
	})
	target, base := modDocumentID("button"), "/elements/0/components"
	source := workspaceShellTest(t, a)
	for _, markup := range []string{
		`src="onix-rightbar/sprite.svg"`, `src="onix-rightbar/text-file.svg"`, `src="onix-rightbar/grid.svg"`,
		`class="element-title">face</h2>`, `>Anchor X</label>`, `>Anchor Y</label>`,
		`<textarea id="` + workspaceFieldID(target, base+"/text/value", "") + `">First line`,
		`style="background:rgba(17,34,51,1);color:#ffffff"`,
		`aria-label="Choose Font"`, `aria-label="Increase Font Size"`,
	} {
		if !strings.Contains(source, markup) {
			t.Errorf("legacy component presentation missing %s", markup)
		}
	}
	root, err := dom.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	var check func(*dom.Node)
	check = func(node *dom.Node) {
		if id, ok := node.Attr("id"); ok && strings.IndexFunc(id, unicode.IsSpace) >= 0 {
			t.Errorf("HTML id contains whitespace: %q", id)
		}
		for _, child := range node.Children {
			check(child)
		}
	}
	check(root)
	fontSize := workspaceFieldID(target, base+"/text/fontSize", "")
	a.handleEvent(htmlui.Event{ID: fontSize, Kind: htmlui.EventInput, Value: "22"})
	a.handleEvent(htmlui.Event{ID: "toggle:content:" + base + "/text", Kind: htmlui.EventClick})
	workspaceShellTest(t, a)
	remove := "remove:" + url.PathEscape(string(target)) + "|" + base + "/layout"
	a.handleEvent(htmlui.Event{ID: "content-menu:" + remove, Kind: htmlui.EventClick})
	workspaceShellTest(t, a)
	if a.componentMenu != remove || a.drafts[fontSize] != "22" {
		t.Fatal("collapsing a component or opening its menu lost field drafts")
	}
	a.handleEvent(htmlui.Event{ID: remove, Kind: htmlui.EventClick})
	a.action("save")
	if a.hasDrafts() || a.componentMenu != "" || a.Document.Mods["button"].Spec.Elements[0].Components.Layout != nil || a.Document.Mods["button"].Spec.Elements[0].Components.Text.FontSize != 22 {
		t.Fatalf("component menu did not remove only the chosen component: %s", a.status)
	}
}

func TestWorkspaceContentTextureModesPreservePendingEdits(t *testing.T) {
	a := contentEditorFixture(t, project.ModComponents{})
	for _, file := range []string{"button/resources/face.png", "button/resources/sheet.atlas"} {
		if err := writeWorkspaceFile(a.Document.Absolute(file), []byte("resource")); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.Document.EditMod("button", "Texture fixture", func(mod *project.ModSpec) error {
		mod.Elements[0].Components.Sprite = &project.ModSpriteSpec{Size: []float32{40, 20}, Texture: &project.TextureSpec{Image: "resources/face.png", Slice: &project.NineSlice{Left: 2, Right: 3}}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	workspaceShellTest(t, a)
	target, base := modDocumentID("button"), "/elements/0/components/sprite"
	edit := func(path, axis, value string) {
		a.handleEvent(htmlui.Event{ID: workspaceFieldID(target, base+path, axis), Kind: htmlui.EventInput, Value: value})
	}
	click := func(id string) {
		t.Helper()
		a.handleEvent(htmlui.Event{ID: id, Kind: htmlui.EventClick})
		workspaceShellTest(t, a)
	}
	edit("/alpha", "", "0.6")
	edit("/size", "Width", "88.5")
	edit("/texture/slice/left", "", "6")
	a.handleEvent(htmlui.Event{ID: workspaceFieldID(target, base+"/texture/mipmap", ""), Kind: htmlui.EventChange, Checked: true})
	click("content-scale:" + workspaceFieldID(target, base+"/texture/slice", "") + ":9-Slice")
	click("content-texture:" + workspaceFieldID(target, base+"/texture", "") + ":Atlas")
	sprite := a.viewMod("button").Elements[0].Components.Sprite
	if sprite.Texture.Image != "" || !sprite.Texture.Mipmap || sprite.Texture.Slice.Left != 6 || *sprite.Alpha != .6 || sprite.Size[0] != 88.5 {
		t.Fatalf("switching texture source lost unrelated drafts: %+v", sprite)
	}
	if _, exists := a.fields[workspaceFieldID(target, base+"/texture/image", "")]; exists {
		t.Fatal("atlas mode still exposes the image source")
	}
	edit("/texture/atlas", "", "resources/sheet.atlas")
	edit("/texture/frame", "", "face")
	a.action("save")
	if a.hasDrafts() || len(a.textureModes) != 0 {
		t.Fatalf("texture draft did not save: %s", a.status)
	}
	a.action("undo")
	workspaceShellTest(t, a)
	if _, exists := a.fields[workspaceFieldID(target, base+"/texture/image", "")]; !exists {
		t.Fatal("undo retained the atlas selector instead of restoring image mode")
	}
	a.action("redo")
	workspaceShellTest(t, a)
	click("content-texture:" + workspaceFieldID(target, base+"/texture", "") + ":Color")
	a.action("save")
	sprite = a.Document.Mods["button"].Spec.Elements[0].Components.Sprite
	if a.hasDrafts() || sprite.Color != "" || sprite.Texture.Color != "#ffffff" || sprite.Texture.Atlas != "" || sprite.Texture.Frame != "" || sprite.Texture.Slice != nil || !sprite.Texture.Mipmap || *sprite.Alpha != .6 || sprite.Size[0] != 88.5 {
		t.Fatalf("color mode did not save one valid source: %+v (%s)", sprite, a.status)
	}
}

func TestWorkspaceContentAtlasFrameSelectsDeclaredRegions(t *testing.T) {
	a := contentEditorFixture(t, project.ModComponents{})
	metadata := `{"frames":{"face":{"frame":{"x":0,"y":0,"w":8,"h":8}},"wing":{"frame":{"x":8,"y":0,"w":8,"h":8}}},"meta":{"image":"sheet.png"}}`
	file := a.Document.Absolute("button/resources/sheet.json")
	if err := writeWorkspaceFile(file, []byte(metadata)); err != nil {
		t.Fatal(err)
	}
	if err := a.Document.EditMod("button", "Atlas fixture", func(mod *project.ModSpec) error {
		mod.Elements[0].Components.Sprite = &project.ModSpriteSpec{Texture: &project.TextureSpec{Atlas: "resources/sheet.json", Frame: "face"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	target, pointer := modDocumentID("button"), "/elements/0/components/sprite/texture"
	id := workspaceFieldID(target, pointer+"/frame", "")
	source := workspaceShellTest(t, a)
	if !strings.Contains(source, `<select id="`+id+`"`) || !strings.Contains(source, "wing") {
		t.Fatal("atlas frame did not use the atlas region selector")
	}
	a.handleEvent(htmlui.Event{ID: id, Kind: htmlui.EventChange, Value: "wing"})
	a.action("save")
	workspaceShellTest(t, a)
	sprite := a.Document.Mods["button"].Spec.Elements[0].Components.Sprite
	if a.hasDrafts() || sprite.Texture.Atlas != "resources/sheet.json" || sprite.Texture.Frame != "wing" {
		t.Fatalf("region selection did not save: %+v (%s)", sprite, a.status)
	}
	if err := writeWorkspaceFile(file, []byte("resource")); err != nil {
		t.Fatal(err)
	}
	if _, exists := a.fields[id]; !exists || strings.Contains(workspaceShellTest(t, a), `<select id="`+id+`"`) {
		t.Fatal("unreadable atlas did not fall back to the frame text field")
	}
}

func TestWorkspaceContentResourcePickerUsesModulePathsAndNames(t *testing.T) {
	a := contentEditorFixture(t, project.ModComponents{Text: &project.TextComponentSpec{Value: "Label", FontSize: 16}, Sprite: &project.ModSpriteSpec{Color: "#ffffff"}})
	for _, file := range []string{"button/resources/ui.ttf", "button/resources/icon.png"} {
		if err := writeWorkspaceFile(a.Document.Absolute(file), []byte("resource")); err != nil {
			t.Fatal(err)
		}
	}
	name, err := a.Document.AddResource("button", "font", "resources/ui.ttf")
	if err != nil {
		t.Fatal(err)
	}
	workspaceShellTest(t, a)
	target, base := modDocumentID("button"), "/elements/0/components"
	font := workspaceFieldID(target, base+"/text/font", "")
	picked := a.Document.Absolute("button/resources/ui.ttf")
	var options window.FileDialogOptions
	a.openNativeFile = func(value window.FileDialogOptions) (string, error) { options = value; return picked, nil }
	a.handleEvent(htmlui.Event{ID: "content-resource:" + font, Kind: htmlui.EventClick})
	if a.drafts[font] != name || len(options.Filters) != 1 || options.Filters[0].Name != "Font" || options.DefaultPath != a.Document.Absolute("button") {
		t.Fatalf("font picker did not author the resource declaration name: %+v", options)
	}
	a.handleEvent(htmlui.Event{ID: "content-texture:" + workspaceFieldID(target, base+"/sprite/texture", "") + ":Image", Kind: htmlui.EventClick})
	workspaceShellTest(t, a)
	image := workspaceFieldID(target, base+"/sprite/texture/image", "")
	picked = a.Document.Absolute("button/resources/icon.png")
	a.handleEvent(htmlui.Event{ID: "content-resource:" + image, Kind: htmlui.EventClick})
	if a.drafts[image] != "resources/icon.png" {
		t.Fatalf("image picker did not author a module-relative path: %q", a.drafts[image])
	}
	picked = filepath.Join(a.Document.ProjectDir, "resources", "outside.png")
	a.handleEvent(htmlui.Event{ID: "content-resource:" + image, Kind: htmlui.EventClick})
	if a.drafts[image] != "resources/icon.png" || !strings.Contains(a.status, "stay within module") {
		t.Fatal("an invalid selection replaced the previous image draft")
	}
	picked = ""
	a.handleEvent(htmlui.Event{ID: "content-resource:" + image, Kind: htmlui.EventClick})
	if a.drafts[image] != "resources/icon.png" {
		t.Fatal("canceling the picker cleared the previous image draft")
	}
	a.action("save")
	content := a.Document.Mods["button"].Spec.Elements[0].Components
	if a.hasDrafts() || content.Text.Font != name || content.Sprite.Texture.Image != "resources/icon.png" {
		t.Fatalf("resource controls did not save: %s", a.status)
	}
}

func TestWorkspaceContentFileReferenceFieldsUseNativePickers(t *testing.T) {
	a := workspaceEditorFixture(t)
	for _, file := range []string{"resources/hero.glb", "resources/walk.json", "resources/walk.atlas", "resources/walk.png", "resources/sky.hdr"} {
		if err := writeWorkspaceFile(a.Document.Absolute("button/"+file), []byte("resource")); err != nil {
			t.Fatal(err)
		}
	}
	content := project.ModComponents{
		Model:   &project.ModelSpec{ID: "hero", Path: "resources/hero.glb"},
		Spine:   &project.ModSpineSpec{Source: project.SpineSpec{ID: "walk", Skeleton: "resources/walk.json", Atlas: "resources/walk.atlas", Texture: "resources/walk.png"}},
		Tilemap: &project.TilemapSpec{ID: "ground", Width: 2, Height: 2, TileWidth: 8, TileHeight: 8, Tiles: []int{0, 0, 0, 0}},
		Camera:  &project.Camera3DSpec{Fov: 1, Near: 0.1, ClearColor: "#000000", Environment: &project.EnvironmentSpec{Path: "resources/sky.hdr"}},
	}
	if err := a.Document.EditMod("button", "Element fixture", func(mod *project.ModSpec) error {
		mod.Space = project.CoordinateSpaceWorld
		mod.Elements = []project.ModElementSpec{{ID: "face", Components: content}, {ID: "other"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	a.enterModuleEdit("main.json", "start")
	a.selectElement("start", "face")
	workspaceShellTest(t, a)
	target, base := modDocumentID("button"), "/elements/0/components"
	for _, pointer := range []string{
		"/model/path", "/model/animationPath", "/model/collisionPath",
		"/spine/source/skeleton", "/spine/source/atlas", "/spine/source/texture",
		"/tilemap/tileset", "/camera/environment/path",
	} {
		if _, exists := a.fields["content-resource:"+workspaceFieldID(target, base+pointer, "")]; !exists {
			t.Fatalf("%s is not authored through a file picker", pointer)
		}
	}
	var options window.FileDialogOptions
	pick := func(pointer, file string) string {
		t.Helper()
		absolute := a.Document.Absolute("button/" + file)
		if err := writeWorkspaceFile(absolute, []byte("resource")); err != nil {
			t.Fatal(err)
		}
		a.openNativeFile = func(value window.FileDialogOptions) (string, error) { options = value; return absolute, nil }
		id := workspaceFieldID(target, base+pointer, "")
		a.handleEvent(htmlui.Event{ID: "content-resource:" + id, Kind: htmlui.EventClick})
		return a.drafts[id]
	}
	if got := pick("/model/path", "resources/ship.glb"); got != "resources/ship.glb" || options.Filters[0].Name != "Model" || options.DefaultPath != a.Document.Absolute("button/resources") {
		t.Fatalf("model path picker authored %q with %+v", got, options)
	}
	if got := pick("/spine/source/skeleton", "resources/ship.skel"); got != "resources/ship.skel" || options.Filters[0].Name != "Spine" || options.DefaultPath != a.Document.Absolute("button/resources") {
		t.Fatalf("spine skeleton picker authored %q with %+v", got, options)
	}
	if got := pick("/tilemap/tileset", "assets/island.png"); got != "assets/island.png" || options.DefaultPath != a.Document.Absolute("button") {
		t.Fatalf("empty tileset picker did not start at the module folder: %q %+v", got, options)
	}
	if got := pick("/camera/environment/path", "resources/studio.hdr.jpg"); got != "resources/studio.hdr.jpg" || options.Filters[0].Name != "Image" || options.DefaultPath != a.Document.Absolute("button/resources") {
		t.Fatalf("environment picker authored %q with %+v", got, options)
	}
	a.action("save")
	saved := a.Document.Mods["button"].Spec.Elements[0].Components
	if a.hasDrafts() || saved.Model.Path != "resources/ship.glb" || saved.Spine.Source.Skeleton != "resources/ship.skel" || saved.Camera.Environment.Path != "resources/studio.hdr.jpg" {
		t.Fatalf("file picker drafts did not save: %+v (%s)", saved, a.status)
	}
}

func TestWorkspaceModuleRootFileFieldsUseNativePickers(t *testing.T) {
	a := workspaceEditorFixture(t)
	if err := writeWorkspaceFile(a.Document.Absolute("button/resources/hero.glb"), []byte("glb")); err != nil {
		t.Fatal(err)
	}
	if err := a.Document.EditMod("button", "Model module", func(mod *project.ModSpec) error {
		mod.Space = project.CoordinateSpaceWorld
		mod.Elements = []project.ModElementSpec{{ID: "face"}}
		mod.Components.Model = &project.ModelSpec{ID: "hero", Path: "resources/hero.glb"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	a.enterModuleEdit("main.json", "start")
	workspaceShellTest(t, a)
	if a.selectedElement != "" {
		t.Fatalf("fixture selected an element: %q", a.selectedElement)
	}
	id := workspaceFieldID(modDocumentID("button"), "/components/model/path", "")
	if _, exists := a.fields["content-resource:"+id]; !exists {
		t.Fatal("a module root file field has no picker")
	}
	picked := a.Document.Absolute("button/resources/ship.glb")
	if err := writeWorkspaceFile(picked, []byte("glb")); err != nil {
		t.Fatal(err)
	}
	a.openNativeFile = func(window.FileDialogOptions) (string, error) { return picked, nil }
	a.handleEvent(htmlui.Event{ID: "content-resource:" + id, Kind: htmlui.EventClick})
	if a.drafts[id] != "resources/ship.glb" {
		t.Fatalf("module root picker did not author a module-relative path: %q (%s)", a.drafts[id], a.status)
	}
	a.action("save")
	if path := a.viewMod("button").Components.Model.Path; path != "resources/ship.glb" {
		t.Fatalf("module root path did not save: %q (%s)", path, a.status)
	}
}

func TestWorkspaceContentSchemaMatchesSerializedComponents(t *testing.T) {
	// Metadata follows the current module format, including fields implemented
	// through custom JSON methods. Defaults must round-trip without data loss.
	for _, property := range workspaceContentSchema.Fields {
		t.Run(property.Name, func(t *testing.T) {
			data := []byte(`{"` + property.Name + `":` + property.Initial + `}`)
			var content project.ModComponents
			if err := json.Unmarshal(data, &content); err != nil {
				t.Fatalf("component default cannot decode: %v", err)
			}
			encoded, err := json.Marshal(content)
			if err != nil || !strings.Contains(string(encoded), `"`+property.Name+`"`) {
				t.Fatalf("component default was lost: %s (%v)", encoded, err)
			}
		})
	}
}
