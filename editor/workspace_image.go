package editor

import (
	"errors"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/graphics"
)

const workspaceImageDialog = "image-editor"
const workspaceImageSource = "image-editor-preview"
const workspaceImageCompactWidth = 540

type workspaceImageEditor struct {
	texture          *graphics.Texture
	original, pixels *image.NRGBA
	file             string
}

func (a *ModuleEditor) openWorkspaceImage(file string) {
	a.showWorkspaceDialog(workspaceImageDialog)
	a.imageEditor.file = file
	a.status = file
	data, err := fs.ReadFile(workspaceFS{w: a.Document}, file)
	if err == nil {
		if compressibleTextureSource(file) && !strings.EqualFold(path.Ext(file), ".svg") {
			var pixels *image.RGBA
			pixels, err = decodeTextureSource(data, file)
			if err == nil {
				a.imageEditor.original = (*image.NRGBA)(pixels)
				a.imageEditor.setPixels(a.imageEditor.original)
			}
		} else {
			a.imageEditor.texture, err = (editorImageDecoder{}).DecodeImage(data, file, graphics.TextureOptions{Usage: graphics.ResourceUsageGPU})
		}
	}
	if err != nil {
		a.report(fmt.Errorf("Could not preview %s: %w", path.Base(file), err))
	}
}

func (p *workspaceImageEditor) setPixels(pixels *image.NRGBA) {
	_ = p.texture.Dispose()
	p.pixels = pixels
	p.texture = graphics.CreateTextureBySharedImage((*image.RGBA)(pixels), false, graphics.ResourceUsageDefault)
}

func (a *ModuleEditor) layoutWorkspaceImage(world *ecs.World, panel rect) {
	const name = workspaceImageDialog
	p := &a.imageEditor
	a.dialogLabel(name, name+"-title", path.Base(p.file), 24, 15, panel.W-80, 26)
	a.dialogClose(name)
	if p.texture == nil {
		return
	}

	width, height := p.texture.LogicalWidth(), p.texture.LogicalHeight()
	label := fmt.Sprintf("%d × %d px", width, height)
	if p.pixels == nil {
		label += " · Preview only"
	} else if extension := strings.ToLower(path.Ext(p.file)); extension == ".gif" || extension == ".webp" {
		label += " · First frame"
	}
	a.dialogLabel(name, name+"-size", label, 24, 43, panel.W-48, 20)

	toolbarHeight := float32(46)
	compact := panel.W < workspaceImageCompactWidth
	if compact && p.pixels != nil {
		toolbarHeight += 36
	}
	areaWidth, areaHeight := max(0, panel.W-48), max(0, panel.H-toolbarHeight-114)
	scale := min(areaWidth/float32(width), areaHeight/float32(height))
	previewWidth, previewHeight := float32(width)*scale, float32(height)*scale
	x, y := (panel.W-previewWidth)/2, 72+(areaHeight-previewHeight)/2
	a.dialogElement(name, name+"-background", x, y, previewWidth, previewHeight)
	a.dialogElement(name, workspaceImageSource, x, y, previewWidth, previewHeight).Source = "onix-dialog/" + workspaceImageSource
	registerEditorHTMLDialogTexture(world, workspaceImageSource, p.texture)
	if p.pixels == nil {
		return
	}
	edited := p.pixels != p.original
	buttons := [...]struct{ id, label string }{
		{"rotate", "Rotate 90°"},
		{"flip-h", "Flip H"},
		{"flip-v", "Flip V"},
		{"reset", "Reset"},
		{"save", "Save PNG Copy"},
	}
	gap := float32(6)
	count := float32(len(buttons))
	if compact {
		count--
		buttons[0].label = "Rotate"
	}
	buttonWidth := (panel.W - 48 - (count-1)*gap) / count
	for i, button := range buttons {
		x, y, w := 24+float32(i)*(buttonWidth+gap), panel.H-toolbarHeight, buttonWidth
		if compact && button.id == "save" {
			x, y, w = 24, panel.H-46, panel.W-48
		}
		element := a.dialogButton(name, name+"-"+button.id, button.label, x, y, w, 30, false)
		switch button.id {
		case "rotate":
			element.AriaLabel = "Rotate 90 degrees clockwise"
		case "flip-h":
			element.AriaLabel = "Flip horizontally"
		case "flip-v":
			element.AriaLabel = "Flip vertically"
		case "reset", "save":
			element.Disabled = !edited
		}
	}
}

func (a *ModuleEditor) workspaceImageAction(id string) {
	p := &a.imageEditor
	if p.pixels == nil {
		return
	}
	switch id {
	case "image-editor-rotate", "image-editor-flip-h", "image-editor-flip-v":
		p.setPixels(transformWorkspaceImage(p.pixels, id))
	case "image-editor-reset":
		if p.pixels != p.original {
			p.setPixels(p.original)
		}
	case "image-editor-save":
		if p.pixels != p.original {
			a.report(a.saveWorkspaceImageCopy())
		}
		return
	default:
		return
	}
	a.dialogError = ""
	a.changed()
}

func transformWorkspaceImage(source *image.NRGBA, action string) *image.NRGBA {
	width, height := source.Rect.Dx(), source.Rect.Dy()
	start, column, row := 0, 4, source.Stride
	switch action {
	case "image-editor-rotate":
		start, column, row = (height-1)*source.Stride, -source.Stride, 4
		width, height = height, width
	case "image-editor-flip-h":
		start, column = (width-1)*4, -4
	case "image-editor-flip-v":
		start, row = (height-1)*source.Stride, -source.Stride
	}
	result := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		offset := start + y*row
		target := y * result.Stride
		for range width {
			copy(result.Pix[target:target+4], source.Pix[offset:offset+4])
			offset += column
			target += 4
		}
	}
	return result
}

func (a *ModuleEditor) saveWorkspaceImageCopy() error {
	doc, p := a.Document, &a.imageEditor
	directory, outputRoot := path.Dir(p.file), doc.ProjectDir
	staged, saved := false, false
	if owner := doc.assetOwner(p.file); doc.assets[p.file] != "" && owner != nil && !owner.disk.exists {
		snapshot, err := doc.newAssetSnapshot()
		if err != nil {
			return err
		}
		outputRoot, staged = snapshot, true
		defer func() {
			if !saved {
				_ = os.RemoveAll(snapshot)
			}
		}()
	}

	root, err := os.OpenRoot(outputRoot)
	if err != nil {
		return err
	}
	defer root.Close()
	stem := strings.TrimSuffix(path.Base(p.file), path.Ext(p.file)) + "-edited"
	for index := 1; ; index++ {
		base := stem + ".png"
		if index > 1 {
			base = fmt.Sprintf("%s-%d.png", stem, index)
		}
		file := path.Join(directory, base)
		if doc.assets[file] != "" || doc.assetDirs[file] {
			continue
		}
		output := file
		if staged {
			output = base
		}
		destination, err := root.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return err
		}
		encodeErr := png.Encode(destination, p.pixels)
		if err := errors.Join(encodeErr, destination.Close()); err != nil {
			_ = root.Remove(output)
			return err
		}
		saved = true
		a.closeWorkspaceDialog()
		a.lastFile = file
		if staged {
			doc.assets[file] = filepath.Join(outputRoot, output)
			doc.Revision++
			a.status = "Created " + file + ". Press Ctrl+S to save"
			return nil
		}
		a.status = "Saved " + file
		if err := doc.RefreshFiles(); err != nil {
			return fmt.Errorf("Saved %s but could not refresh the resource list: %w", file, err)
		}
		return nil
	}
}

func (a *ModuleEditor) disposeWorkspaceImage() {
	if a.imageEditor.texture != nil {
		if a.world != nil {
			registerEditorHTMLDialogTexture(a.world, workspaceImageSource, nil)
		}
		_ = a.imageEditor.texture.Dispose()
	}
	a.imageEditor = workspaceImageEditor{}
}
