package editor

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/onix-engine/onix/ecs"
	htmlui "github.com/onix-engine/onix/ui/html"
)

func TestWorkspaceImageEditorOpensAndSavesFromResourceTile(t *testing.T) {
	a := workspaceEditorFixture(t)
	const file = "Face 1.PNG"
	source := image.NewNRGBA(image.Rect(0, 0, 2, 3))
	pixel := color.NRGBA{R: 190, B: 79, A: 43}
	source.SetNRGBA(0, 0, pixel)
	var original bytes.Buffer
	if err := png.Encode(&original, source); err != nil {
		t.Fatal(err)
	}
	if err := writeWorkspaceFile(a.Document.Absolute(file), original.Bytes()); err != nil {
		t.Fatal(err)
	}
	const existing = "Face 1-edited.png"
	workspacePopupPNG(t, a, existing, 1, 1)
	if err := a.Document.RefreshFiles(); err != nil {
		t.Fatal(err)
	}

	world := ecs.NewWorld()
	syncWorkspaceTest(t, a, world)
	tile := "file:" + url.PathEscape(file)
	if _, ok := htmlui.NodeByID(world, a.shell, tile); !ok {
		t.Fatal("image tile is missing")
	}
	world.WriteMessage(htmlui.Event{Entity: a.shell, ID: tile, Kind: htmlui.EventClick})
	if err := a.update(&ecs.SystemContext{World: world}); err != nil {
		t.Fatal(err)
	}
	syncWorkspaceTest(t, a, world)
	if a.dialog != workspaceImageDialog || a.imageEditor.pixels == nil || a.lastFile != file {
		t.Fatalf("first click did not open the selected image: %s", a.status)
	}
	dialog := a.dialogUI.htmlDialogs[workspaceImageDialog]
	view, err := htmlui.Serialize(world, dialog)
	if err != nil || !strings.Contains(view, "2 × 3 px") || !strings.Contains(view, `src="onix-dialog/image-editor-preview"`) {
		t.Fatalf("image preview or dimensions are missing: %v", err)
	}

	click := func(id string) {
		t.Helper()
		workspaceDialogTestEvent(t, a, world, "image-editor-"+id, "", htmlui.EventClick)
	}
	click("rotate")
	if got := a.imageEditor.pixels; got.Bounds().Size() != image.Pt(3, 2) || got.NRGBAAt(2, 0) != pixel {
		t.Fatal("rotation did not update the image dimensions and pixels")
	}
	click("flip-h")
	click("flip-v")
	if a.imageEditor.pixels.NRGBAAt(0, 1) != pixel {
		t.Fatal("flipping did not update the image pixels")
	}
	click("reset")
	if a.imageEditor.pixels != a.imageEditor.original {
		t.Fatal("reset did not restore the original preview")
	}
	click("rotate")
	click("save")
	const saved = "Face 1-edited-2.png"
	if a.dialog != "" || a.imageEditor.texture != nil || a.lastFile != saved {
		t.Fatalf("saving did not close the editor and select the new copy: %s", a.status)
	}
	if _, ok := htmlui.NodeByID(world, a.shell, "file:"+url.PathEscape(saved)); !ok {
		t.Fatal("the saved image is missing from the resource browser")
	}
	data, err := os.ReadFile(a.Document.Absolute(saved))
	if err != nil {
		t.Fatal(err)
	}
	edited, err := png.Decode(bytes.NewReader(data))
	if err != nil || edited.Bounds().Size() != image.Pt(3, 2) || color.NRGBAModel.Convert(edited.At(2, 0)) != pixel {
		t.Fatalf("PNG copy did not preserve the edited pixels: %v", err)
	}
	data, err = os.ReadFile(a.Document.Absolute(file))
	if err != nil || !bytes.Equal(data, original.Bytes()) {
		t.Fatalf("saving a copy changed the source image: %v", err)
	}
}

func TestWorkspaceImageEditorPreviewsSVGAndReportsDecodeErrors(t *testing.T) {
	a := workspaceEditorFixture(t)
	const file = "preview.svg"
	if err := writeWorkspaceFile(a.Document.Absolute(file), []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="16"><rect width="24" height="16" fill="red"/></svg>`)); err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	a.openFile(file)
	syncWorkspaceTest(t, a, world)
	dialog := a.dialogUI.htmlDialogs[workspaceImageDialog]
	view, err := htmlui.Serialize(world, dialog)
	if err != nil || !strings.Contains(view, "24 × 16 px · Preview only") || a.imageEditor.pixels != nil {
		t.Fatalf("SVG did not open in preview mode: %v, %s", err, a.status)
	}
	workspaceDialogTestEvent(t, a, world, "image-editor-close", "", htmlui.EventClick)
	provider, _ := world.GetResource[htmlui.ImageProvider]()
	if _, ok := provider.ResolveHTMLImage("onix-dialog/"+workspaceImageSource, 24, 16); ok {
		t.Fatal("closing the dialog retained the preview texture")
	}
	a.openFile("missing.png")
	syncWorkspaceTest(t, a, world)
	if a.dialog != workspaceImageDialog || a.imageEditor.texture != nil || !strings.Contains(a.dialogError, "Could not preview missing.png") {
		t.Fatalf("missing image did not surface a clear error: %s", a.status)
	}
}

func TestWorkspaceImageEditorSavesCopyInsideStagedModule(t *testing.T) {
	a := workspaceEditorFixture(t)
	workspacePopupPNG(t, a, "button/face.png", 2, 3)
	if err := a.Document.RefreshFiles(); err != nil {
		t.Fatal(err)
	}
	if err := a.Document.CopyMod("button", "clone"); err != nil {
		t.Fatal(err)
	}
	a.openFile("clone/face.png")
	a.workspaceImageAction("image-editor-rotate")
	a.workspaceImageAction("image-editor-save")
	const saved = "clone/face-edited.png"
	if a.dialog != "" || a.lastFile != saved || a.Document.assets[saved] == "" {
		t.Fatalf("edited copy did not enter the module's staged assets: %s", a.status)
	}
	if err := a.Document.Save(); err != nil {
		t.Fatal(err)
	}
	data := readWorkspaceTestFile(t, a.Document, saved)
	copyImage, err := png.Decode(bytes.NewReader(data))
	if err != nil || copyImage.Bounds().Size() != image.Pt(3, 2) {
		t.Fatalf("saving the module did not persist the edited PNG: %v", err)
	}
}
