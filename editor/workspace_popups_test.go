package editor

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"maps"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/project"
	htmlui "github.com/diiyw/onix/ui/html"
)

func TestWorkspaceColorPickerDraftModesDragAndHistory(t *testing.T) {
	a := contentEditorFixture(t, project.ModComponents{Sprite: &project.ModSpriteSpec{Color: "#12345678"}})
	world := a.world
	target := modDocumentID("button")
	id := workspaceFieldID(target, "/elements/0/components/sprite/color", "")
	// Opening must include pending input, without committing it to the document.
	a.handleEvent(htmlui.Event{ID: id, Kind: htmlui.EventInput, Value: "#2468"})
	a.handleEvent(htmlui.Event{ID: "content-color:" + id, Kind: htmlui.EventClick})
	syncWorkspaceTest(t, a, world)
	if a.dialog != "color-picker" || a.colorPicker.rgba != ([4]uint8{34, 68, 102, 136}) {
		t.Fatalf("picker ignored current draft: %+v (%s)", a.colorPicker.rgba, a.status)
	}
	hexID := htmlDialogFieldID("color-picker", "color-picker.hex")
	workspaceDialogTestEvent(t, a, world, hexID, "#abc", htmlui.EventInput)
	if a.dialogValues[hexID] != "#abc" || a.drafts[id] != "#aabbcc" {
		t.Fatal("valid shorthand was expanded in the input before typing finished")
	}
	workspaceDialogTestEvent(t, a, world, hexID, "#abcdef80", htmlui.EventInput)
	if a.drafts[id] != "#abcdef80" || a.Document.Mods["button"].Spec.Elements[0].Components.Sprite.Color != "#12345678" {
		t.Fatal("valid picker input must remain an element draft")
	}
	workspaceDialogTestEvent(t, a, world, hexID, "#bad-value", htmlui.EventInput)
	if a.drafts[id] != "#abcdef80" || a.dialogError == "" {
		t.Fatal("invalid hex replaced the valid draft or failed to report an error")
	}
	workspaceDialogTestEvent(t, a, world, "color-picker-mode-rgba", "", htmlui.EventClick)
	for channel, value := range map[string]string{"r": "250", "g": "0", "b": "0", "a": "64"} {
		workspaceDialogTestEvent(t, a, world, htmlDialogFieldID("color-picker", "color-picker."+channel), value, htmlui.EventInput)
	}
	workspaceDialogTestEvent(t, a, world, htmlDialogFieldID("color-picker", "color-picker.a"), "256", htmlui.EventInput)
	if a.colorPicker.rgba != ([4]uint8{250, 0, 0, 64}) || a.drafts[id] != "#fa000040" || a.dialogError == "" {
		t.Fatal("RGBA editing did not preserve valid channels")
	}
	workspaceDialogTestEvent(t, a, world, "color-picker-mode-hex", "", htmlui.EventClick)
	svTexture := a.colorPicker.sv
	_, hue := a.workspaceColorPickerAreas()
	a.updateWorkspacePopupPointer(input.Pointer{Pressed: true, Down: true, PressX: hue.X + 10, PressY: hue.Y + hue.H/3})
	a.updateWorkspacePopupPointer(input.Pointer{Released: true, X: hue.X + 10, Y: hue.Y + hue.H/3})
	if a.colorPicker.rgba[1] != 250 || a.colorPicker.rgba[3] != 64 || a.colorPicker.sv != svTexture {
		t.Fatal("hue drag lost alpha or replaced the cached texture")
	}
	sv, _ := a.workspaceColorPickerAreas()
	a.updateWorkspacePopupPointer(input.Pointer{Pressed: true, PressX: sv.X + sv.W*.5, PressY: sv.Y + sv.H*.5})
	a.updateWorkspacePopupPointer(input.Pointer{Released: true, X: sv.X + sv.W*.5, Y: sv.Y + sv.H*.5})
	if a.colorPicker.rgba != ([4]uint8{64, 128, 64, 64}) || a.colorPicker.drag != "" {
		t.Fatalf("SV drag used incorrect color coordinates: %+v", a.colorPicker.rgba)
	}
	a.changed()
	syncWorkspaceTest(t, a, world)
	context := &ecs.SystemContext{World: world}
	if allocations := testing.AllocsPerRun(100, func() {
		a.updateWorkspacePopupPointer(input.Pointer{})
		if err := a.sync(context); err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatalf("idle color picker allocated %g", allocations)
	}
	// The modal must block underlying component and viewport actions.
	a.handleEvent(htmlui.Event{ID: "content-picker", Kind: htmlui.EventClick})
	if a.componentPickerOpen {
		t.Fatal("color popup allowed an underlying click")
	}
	workspaceDialogTestEvent(t, a, world, "color-picker-close", "", htmlui.EventClick)
	if a.dialog != "" || !a.hasDrafts() || a.colorPicker.field != "" {
		t.Fatal("closing the picker lost its draft")
	}
	a.action("save")
	if a.hasDrafts() || a.Document.Mods["button"].Spec.Elements[0].Components.Sprite.Color != "#40804040" {
		t.Fatalf("picker did not save: %s", a.status)
	}
	a.action("undo")
	if a.Document.Mods["button"].Spec.Elements[0].Components.Sprite.Color != "#12345678" {
		t.Fatal("undo did not restore the original color")
	}
	a.action("redo")
	if a.Document.Mods["button"].Spec.Elements[0].Components.Sprite.Color != "#40804040" || a.Document.Mods["button"].Spec.Elements[1].Components.Sprite != nil {
		t.Fatal("redo modified the wrong element")
	}
}

func workspacePopupPNG(t *testing.T, a *ModuleEditor, file string, width, height int) {
	t.Helper()
	pixels := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			pixels.SetRGBA(x, y, color.RGBA{R: 160, G: 200, B: 240, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, pixels); err != nil {
		t.Fatal(err)
	}
	if err := writeWorkspaceFile(a.Document.Absolute(file), encoded.Bytes()); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceNineSliceDraftApplyCancelAndLimits(t *testing.T) {
	a := contentEditorFixture(t, project.ModComponents{})
	workspacePopupPNG(t, a, "button/resources/face.png", 90, 60)
	workspacePopupPNG(t, a, "button/resources/next.png", 120, 80)
	initial := project.NineSlice{Left: 10, Right: 12, Top: 5, Bottom: 7}
	if err := a.Document.EditMod("button", "Image fixture", func(mod *project.ModSpec) error {
		mod.Elements[0].Components.Sprite = &project.ModSpriteSpec{Size: []float32{40, 20}, Texture: &project.TextureSpec{Image: "resources/face.png", Slice: &initial}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	workspaceShellTest(t, a)
	world, target := a.world, modDocumentID("button")
	base := "/elements/0/components/sprite"
	alpha := workspaceFieldID(target, base+"/alpha", "")
	margin := workspaceFieldID(target, base+"/texture/slice/left", "")
	a.handleEvent(htmlui.Event{ID: alpha, Kind: htmlui.EventInput, Value: "0.7"})
	a.handleEvent(htmlui.Event{ID: margin, Kind: htmlui.EventInput, Value: "15"})
	a.handleEvent(htmlui.Event{ID: workspaceFieldID(target, base+"/texture/image", ""), Kind: htmlui.EventInput, Value: "resources/next.png"})
	originalDrafts := maps.Clone(a.drafts)
	openID := "content-nine-slice:" + workspaceFieldID(target, base+"/texture/slice", "")
	open := func() {
		t.Helper()
		a.handleEvent(htmlui.Event{ID: openID, Kind: htmlui.EventClick})
		syncWorkspaceTest(t, a, world)
		if a.dialog != "nine-slice" {
			t.Fatalf("nine-slice did not open: %s", a.status)
		}
	}
	open()
	if a.nineSlice.texture.LogicalWidth() != 120 || a.nineSlice.slice.Left != 15 {
		t.Fatal("nine-slice ignored pending resource or margin input")
	}
	texture := a.nineSlice.texture
	workspaceDialogTestEvent(t, a, world, htmlDialogFieldID("nine-slice", "left"), "22", htmlui.EventInput)
	workspaceDialogTestEvent(t, a, world, "nine-slice-cancel", "", htmlui.EventClick)
	if !maps.Equal(a.drafts, originalDrafts) || texture.Width != 0 || a.nineSlice.texture != nil {
		t.Fatal("cancel changed element drafts or retained the image")
	}
	open()
	workspaceDialogTestEvent(t, a, world, htmlDialogFieldID("nine-slice", "left"), "120", htmlui.EventInput)
	workspaceDialogTestEvent(t, a, world, htmlDialogFieldID("nine-slice", "top"), "8", htmlui.EventInput)
	workspaceDialogTestEvent(t, a, world, "nine-slice-apply", "", htmlui.EventClick)
	if a.dialog != "nine-slice" || a.dialogError == "" || len(a.staged) != 0 {
		t.Fatal("invalid margin was silently applied after another valid field changed")
	}
	workspaceDialogTestEvent(t, a, world, htmlDialogFieldID("nine-slice", "left"), "22", htmlui.EventInput)
	image := a.workspaceSliceImageRect()
	lines := a.workspaceSliceLines()
	a.updateWorkspacePopupPointer(input.Pointer{Pressed: true, Down: true, PressX: lines[0], PressY: image.Y + image.H/2})
	a.updateWorkspacePopupPointer(input.Pointer{Down: true, X: image.X + image.W*2, Y: image.Y + image.H/2})
	a.updateWorkspacePopupPointer(input.Pointer{Released: true, X: image.X + image.W*2, Y: image.Y + image.H/2})
	if a.nineSlice.slice.Left != 107 || a.nineSlice.drag != "" {
		t.Fatal("drag did not keep a one-pixel stretchable center")
	}
	workspaceDialogTestEvent(t, a, world, htmlDialogFieldID("nine-slice", "left"), "25", htmlui.EventInput)
	texture = a.nineSlice.texture
	context := &ecs.SystemContext{World: world}
	if allocations := testing.AllocsPerRun(100, func() {
		a.updateWorkspacePopupPointer(input.Pointer{})
		if err := a.sync(context); err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatalf("idle nine-slice dialog allocated %g", allocations)
	}
	workspaceDialogTestEvent(t, a, world, "nine-slice-apply", "", htmlui.EventClick)
	sprite := a.viewMod("button").Elements[0].Components.Sprite
	if a.dialog != "" || texture.Width != 0 || sprite.Texture.Slice.Left != 25 || sprite.Texture.Slice.Top != 8 || sprite.Texture.Image != "resources/next.png" || *sprite.Alpha != .7 || !a.hasDrafts() {
		t.Fatalf("apply did not stage the complete draft: %+v (%s)", sprite, a.status)
	}
	if a.Document.Mods["button"].Spec.Elements[0].Components.Sprite.Texture.Image != "resources/face.png" {
		t.Fatal("apply saved the document before Ctrl+S")
	}
	a.action("save")
	if a.hasDrafts() {
		t.Fatalf("nine-slice did not save: %s", a.status)
	}
	a.action("undo")
	if *a.Document.Mods["button"].Spec.Elements[0].Components.Sprite.Texture.Slice != initial {
		t.Fatal("undo did not restore original margins")
	}
	a.action("redo")
	if a.Document.Mods["button"].Spec.Elements[0].Components.Sprite.Texture.Slice.Left != 25 {
		t.Fatal("redo did not restore applied margins")
	}
	workspaceShellTest(t, a)
	open()
	in := input.NewState()
	ecs.InsertResource(world, in)
	in.PressKey(input.KeyEscape)
	texture = a.nineSlice.texture
	if err := a.update(context); err != nil {
		t.Fatal(err)
	}
	if a.dialog != "" || texture.Width != 0 {
		t.Fatal("Escape did not close and release the preview")
	}
}

func TestWorkspaceNineSliceAtlasLogicalFrameAndResourceScope(t *testing.T) {
	a := contentEditorFixture(t, project.ModComponents{})
	workspacePopupPNG(t, a, "button/resources/sheet.png", 90, 60)
	metadata := `{"frames":{"face":{"frame":{"x":2,"y":3,"w":30,"h":24},"sourceSize":{"w":40,"h":32},"spriteSourceSize":{"x":5,"y":4,"w":30,"h":24}}},"meta":{"image":"sheet.png","size":{"w":90,"h":60}}}`
	file := a.Document.Absolute("button/resources/sheet.atlas")
	if err := writeWorkspaceFile(file, []byte(metadata)); err != nil {
		t.Fatal(err)
	}
	if err := a.Document.EditMod("button", "Atlas fixture", func(mod *project.ModSpec) error {
		mod.Elements[0].Components.Sprite = &project.ModSpriteSpec{Texture: &project.TextureSpec{Atlas: "resources/sheet.atlas", Frame: "face", Slice: &project.NineSlice{Left: 16, Right: 15, Top: 10, Bottom: 9}}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	workspaceShellTest(t, a)
	field := a.fields["content-nine-slice:"+workspaceFieldID(modDocumentID("button"), "/elements/0/components/sprite/texture/slice", "")]
	if err := a.openWorkspaceNineSlice(field); err != nil {
		t.Fatal(err)
	}
	p := &a.nineSlice
	if p.texture.LogicalWidth() != 40 || p.texture.LogicalHeight() != 32 || p.owner == nil || p.texture.U == 0 {
		t.Fatal("atlas did not use the selected frame and its logical size")
	}
	syncWorkspaceTest(t, a, a.world)
	owner, frame := p.owner, p.texture
	a.closeWorkspaceDialog()
	if owner.Width != 0 || frame.Width != 0 {
		t.Fatal("closing the atlas editor leaked its owner")
	}
	if err := writeWorkspaceFile(file, []byte(strings.Replace(metadata, `"sheet.png"`, `"../../../outside.png"`, 1))); err != nil {
		t.Fatal(err)
	}
	if err := a.openWorkspaceNineSlice(field); err == nil || a.nineSlice.texture != nil {
		t.Fatal("atlas image escaped module scope")
	}
	if err := writeWorkspaceFile(file, []byte(strings.Replace(metadata, `"face":`, `"missing":`, 1))); err != nil {
		t.Fatal(err)
	}
	if err := a.openWorkspaceNineSlice(field); err == nil || a.nineSlice.texture != nil {
		t.Fatal("missing atlas frame did not fail cleanly")
	}
	if err := writeWorkspaceFile(file, []byte(metadata)); err != nil {
		t.Fatal(err)
	}
	if err := writeWorkspaceFile(a.Document.Absolute("button/resources/sheet.png"), []byte("invalid image")); err != nil {
		t.Fatal(err)
	}
	if err := a.openWorkspaceNineSlice(field); err == nil || a.nineSlice.texture != nil {
		t.Fatal("invalid image did not fail cleanly")
	}
}

func TestWorkspaceNineSliceDragPixelMapping(t *testing.T) {
	a := workspaceEditorFixture(t)
	a.nineSlice.texture = &graphics.Texture{Width: 120, Height: 80}
	image := a.workspaceSliceImageRect()
	initial := project.NineSlice{Left: 10, Right: 20, Top: 5, Bottom: 15}
	for _, test := range []struct {
		side string
		x, y float32
		want project.NineSlice
	}{
		{"left", .25, .5, project.NineSlice{Left: 30, Right: 20, Top: 5, Bottom: 15}},
		{"right", .75, .5, project.NineSlice{Left: 10, Right: 30, Top: 5, Bottom: 15}},
		{"top", .5, .25, project.NineSlice{Left: 10, Right: 20, Top: 20, Bottom: 15}},
		{"bottom", .5, .75, project.NineSlice{Left: 10, Right: 20, Top: 5, Bottom: 20}},
		{"left", -1, .5, project.NineSlice{Left: 0, Right: 20, Top: 5, Bottom: 15}},
		{"right", -1, .5, project.NineSlice{Left: 10, Right: 109, Top: 5, Bottom: 15}},
		{"top", .5, 2, project.NineSlice{Left: 10, Right: 20, Top: 64, Bottom: 15}},
		{"bottom", .5, 2, project.NineSlice{Left: 10, Right: 20, Top: 5, Bottom: 0}},
	} {
		a.nineSlice.slice, a.nineSlice.drag = initial, test.side
		a.dragWorkspaceSliceLine(image.X+test.x*image.W, image.Y+test.y*image.H)
		if a.nineSlice.slice != test.want {
			t.Errorf("%s at (%g, %g): got %+v, want %+v", test.side, test.x, test.y, a.nineSlice.slice, test.want)
		}
	}
}

func TestWorkspaceNineSliceFromStretchDefaultsAndCancel(t *testing.T) {
	a := contentEditorFixture(t, project.ModComponents{})
	workspacePopupPNG(t, a, "button/resources/face.png", 90, 60)
	if err := a.Document.EditMod("button", "Stretch fixture", func(mod *project.ModSpec) error {
		mod.Elements[0].Components.Sprite = &project.ModSpriteSpec{Texture: &project.TextureSpec{Image: "resources/face.png"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	workspaceShellTest(t, a)
	id := "content-scale:" + workspaceFieldID(modDocumentID("button"), "/elements/0/components/sprite/texture/slice", "") + ":9-Slice"
	a.handleEvent(htmlui.Event{ID: id, Kind: htmlui.EventClick})
	if a.dialog != "nine-slice" || a.nineSlice.slice != (project.NineSlice{Left: 30, Right: 30, Top: 20, Bottom: 20}) {
		t.Fatalf("new nine-slice did not use original one-third defaults: %+v (%s)", a.nineSlice.slice, a.status)
	}
	a.closeWorkspaceDialog()
	if a.hasDrafts() || a.viewMod("button").Elements[0].Components.Sprite.Texture.Slice != nil {
		t.Fatal("cancel switched the texture out of stretch mode")
	}
	a.handleEvent(htmlui.Event{ID: id, Kind: htmlui.EventClick})
	if err := a.applyWorkspaceNineSlice(); err != nil {
		t.Fatal(err)
	}
	if slice := a.viewMod("button").Elements[0].Components.Sprite.Texture.Slice; slice == nil || slice.Left != 30 || !a.hasDrafts() {
		t.Fatal("apply did not stage the first nine-slice")
	}
}
