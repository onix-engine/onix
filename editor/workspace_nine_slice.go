package editor

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/project"
)

type workspaceNineSlice struct {
	target         DocumentID
	pointer, file  string
	texture, owner *graphics.Texture
	slice          project.NineSlice
	drag           string
}

func (a *ModuleEditor) openWorkspaceNineSlice(field workspaceField) error {
	patches, err := a.formPatches()
	if err != nil {
		return err
	}
	state := a.Document.states[field.Target]
	if state == nil {
		return fmt.Errorf("select a sprite texture")
	}
	data, err := applyDocumentPatches(state.data, patches[field.Target])
	if err != nil {
		return err
	}
	mod, err := decodeDocument[project.ModSpec](data)
	if err != nil {
		return err
	}
	// The registered field identifies its element independently of selection.
	parts := strings.Split(field.Pointer, "/")
	if len(parts) != 7 || parts[1] != "elements" || parts[3] != "components" || parts[4] != "sprite" || parts[5] != "texture" || parts[6] != "slice" {
		return fmt.Errorf("select an element sprite texture")
	}
	index, err := strconv.Atoi(parts[2])
	if err != nil || index < 0 || index >= len(mod.Elements) {
		return fmt.Errorf("sprite element is missing")
	}
	sprite := mod.Elements[index].Components.Sprite
	if sprite == nil || sprite.Texture == nil {
		return fmt.Errorf("select an image or atlas frame first")
	}
	spec := sprite.Texture
	modID := project.ModID(strings.TrimSuffix(string(field.Target), "/mod.json"))
	texture, owner, file, err := a.loadWorkspaceSliceTexture(modID, spec)
	if err != nil {
		return err
	}
	width, height := texture.LogicalWidth(), texture.LogicalHeight()
	slice := project.NineSlice{Left: width / 3, Right: width / 3, Top: height / 3, Bottom: height / 3}
	if spec.Slice != nil {
		slice = *spec.Slice
	}
	if err := validateWorkspaceSlice(slice, width, height); err != nil {
		_ = texture.Dispose()
		_ = owner.Dispose()
		return err
	}
	a.showWorkspaceDialog("nine-slice")
	a.nineSlice = workspaceNineSlice{target: field.Target, pointer: field.Pointer, file: file, texture: texture, owner: owner, slice: slice}
	return nil
}

func (a *ModuleEditor) loadWorkspaceSliceTexture(mod project.ModID, spec *project.TextureSpec) (*graphics.Texture, *graphics.Texture, string, error) {
	if spec.Image != "" {
		texture, err := a.loadWorkspacePopupImage(mod, spec.Image)
		return texture, nil, spec.Image, err
	}
	if spec.Atlas == "" || spec.Frame == "" {
		return nil, nil, "", fmt.Errorf("select an image or atlas frame first")
	}
	file, err := project.ResolveModResourcePath(mod, spec.Atlas)
	if err != nil {
		return nil, nil, "", err
	}
	data, err := os.ReadFile(a.Document.Absolute(file))
	if err != nil {
		return nil, nil, "", err
	}
	metadata, err := graphics.ParseAtlas(data)
	if err != nil {
		return nil, nil, "", err
	}
	if metadata.Meta.Image == "" || strings.ContainsAny(metadata.Meta.Image, `\:`) || path.IsAbs(metadata.Meta.Image) {
		return nil, nil, "", fmt.Errorf("atlas meta.image must name an image within the module")
	}
	image := path.Join(path.Dir(spec.Atlas), metadata.Meta.Image)
	owner, err := a.loadWorkspacePopupImage(mod, image)
	if err != nil {
		return nil, nil, "", err
	}
	atlas, err := graphics.NewAtlas(owner, metadata)
	if err != nil {
		_ = owner.Dispose()
		return nil, nil, "", err
	}
	texture := atlas.SubTexture(spec.Frame)
	if texture == nil {
		_ = owner.Dispose()
		return nil, nil, "", fmt.Errorf("atlas frame %q was not found", spec.Frame)
	}
	return texture, owner, spec.Atlas + " · " + spec.Frame, nil
}

func (a *ModuleEditor) loadWorkspacePopupImage(mod project.ModID, file string) (*graphics.Texture, error) {
	resolved, err := project.ResolveModResourcePath(mod, file)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(a.Document.Absolute(resolved))
	if err != nil {
		return nil, err
	}
	return (editorImageDecoder{}).DecodeImage(data, file, graphics.TextureOptions{Usage: graphics.ResourceUsageGPU})
}

func (a *ModuleEditor) workspaceSliceImageRect() rect {
	panel := a.dialogPanel(860, 720)
	area := rect{X: panel.X + 32, Y: panel.Y + 76, W: panel.W - 64, H: panel.H - 208}
	texture := a.nineSlice.texture
	if texture == nil {
		return area
	}
	scale := min(area.W/float32(texture.LogicalWidth()), area.H/float32(texture.LogicalHeight()))
	w, h := float32(texture.LogicalWidth())*scale, float32(texture.LogicalHeight())*scale
	return rect{X: area.X + (area.W-w)/2, Y: area.Y + (area.H-h)/2, W: w, H: h}
}

func (a *ModuleEditor) workspaceSliceLines() [4]float32 {
	p := &a.nineSlice
	if p.texture == nil {
		return [4]float32{}
	}
	image := a.workspaceSliceImageRect()
	w, h := float32(p.texture.LogicalWidth()), float32(p.texture.LogicalHeight())
	return [4]float32{
		image.X + float32(p.slice.Left)/w*image.W, image.X + (w-float32(p.slice.Right))/w*image.W,
		image.Y + float32(p.slice.Top)/h*image.H, image.Y + (h-float32(p.slice.Bottom))/h*image.H,
	}
}

func (a *ModuleEditor) layoutWorkspaceNineSlice(world *ecs.World, panel rect) {
	const name = "nine-slice"
	p := &a.nineSlice
	a.dialogLabel(name, "nine-slice-title", "9-Slice Editor", 24, 22, 186, 28)
	a.dialogLabel(name, "nine-slice-file", p.file, 210, 25, panel.W-264, 24)
	a.dialogElement(name, "nine-slice-header-divider", 1, 64, panel.W-2, 1).Style += ";background:#3b404b"
	a.dialogClose(name)
	registerEditorHTMLDialogTexture(world, "nine-slice-image", p.texture)
	image, lines := a.workspaceSliceImageRect(), a.workspaceSliceLines()
	x, y := image.X-panel.X, image.Y-panel.Y
	a.dialogElement(name, "nine-slice-image-bg", x-1, y-1, image.W+2, image.H+2).Style += ";background:#1b1e25;border:1px solid #3b404b"
	a.dialogElement(name, "nine-slice-image", x, y, image.W, image.H).Source = "onix-dialog/nine-slice-image"
	values := [...]int{p.slice.Left, p.slice.Right, p.slice.Top, p.slice.Bottom}
	for i, side := range [...]string{"left", "right", "top", "bottom"} {
		line := lines[i]
		if i < 2 {
			a.dialogElement(name, "nine-slice-"+side, line-panel.X-1, y, 2, image.H)
			labelX := line - panel.X + 5
			if i == 1 {
				labelX -= 37
			}
			a.dialogLabel(name, "nine-slice-"+side+"-value", strconv.Itoa(values[i]), labelX, y+5, 32, 20)
		} else {
			a.dialogElement(name, "nine-slice-"+side, x, line-panel.Y-1, image.W, 2)
			labelY := line - panel.Y + 5
			if i == 3 {
				labelY -= 25
			}
			a.dialogLabel(name, "nine-slice-"+side+"-value", strconv.Itoa(values[i]), x+5, labelY, 32, 20)
		}
		width := (panel.W - 84) / 4
		fieldX := 24 + float32(i)*(width+12)
		a.dialogLabel(name, "nine-slice-label-"+side, strings.ToUpper(side[:1])+side[1:], fieldX, panel.H-122, width, 20)
		a.dialogField(name, side, "number", strconv.Itoa(values[i]), fieldX, panel.H-100, width, 30).Step = "1"
	}
	cancelW, applyW := buttonTextWidth("Cancel", true), buttonTextWidth("Apply", true)
	applyX := panel.W - 24 - applyW
	a.dialogButton(name, "nine-slice-cancel", "Cancel", applyX-8-cancelW, panel.H-48, cancelW, 28, false)
	a.dialogButton(name, "nine-slice-apply", "Apply", applyX, panel.H-48, applyW, 28, false)
}

func (a *ModuleEditor) setWorkspaceNineSliceValue(name, value string) error {
	slice := a.nineSlice.slice
	if err := setWorkspaceSliceMargin(&slice, name, value); err != nil {
		return err
	}
	p := &a.nineSlice
	if err := validateWorkspaceSlice(slice, p.texture.LogicalWidth(), p.texture.LogicalHeight()); err != nil {
		return err
	}
	p.slice = slice
	a.dialogError = ""
	return nil
}

func setWorkspaceSliceMargin(slice *project.NineSlice, name, value string) error {
	number, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("nine-slice margins must be non-negative integers")
	}
	switch name {
	case "left":
		slice.Left = number
	case "right":
		slice.Right = number
	case "top":
		slice.Top = number
	case "bottom":
		slice.Bottom = number
	default:
		return nil
	}
	return nil
}

func validateWorkspaceSlice(slice project.NineSlice, width, height int) error {
	if slice.Left >= width || slice.Right >= width || slice.Top >= height || slice.Bottom >= height {
		return fmt.Errorf("nine-slice margins must leave a stretchable center")
	}
	return slice.Validate(width, height)
}

func (a *ModuleEditor) applyWorkspaceNineSlice() error {
	p := &a.nineSlice
	if p.texture == nil {
		return fmt.Errorf("nine-slice image is not loaded")
	}
	slice := p.slice
	for id, value := range a.dialogValues {
		if err := setWorkspaceSliceMargin(&slice, a.dialogFields[id].Name, value); err != nil {
			return err
		}
	}
	if err := validateWorkspaceSlice(slice, p.texture.LogicalWidth(), p.texture.LogicalHeight()); err != nil {
		return err
	}
	patches, err := a.formPatches()
	if err != nil {
		return err
	}
	data, err := json.Marshal(slice)
	if err != nil {
		return err
	}
	patches[p.target] = append(patches[p.target], DocumentPatch{Pointer: p.pointer, Value: data})
	if err := a.stagePatches(patches); err != nil {
		return err
	}
	a.closeWorkspaceDialog()
	return nil
}

func (a *ModuleEditor) updateWorkspaceNineSlicePointer(pointer input.Pointer) {
	p := &a.nineSlice
	if p.texture == nil {
		return
	}
	if pointer.Pressed {
		image, lines := a.workspaceSliceImageRect(), a.workspaceSliceLines()
		for i, side := range [...]string{"left", "right", "top", "bottom"} {
			coordinate, along, start, length := pointer.PressX, pointer.PressY, image.Y, image.H
			if i >= 2 {
				coordinate, along, start, length = pointer.PressY, pointer.PressX, image.X, image.W
			}
			if math.Abs(float64(coordinate-lines[i])) <= 8 && along >= start && along <= start+length {
				p.drag = side
				break
			}
		}
	}
	if pointer.Pressed && p.drag != "" {
		a.dragWorkspaceSliceLine(pointer.PressX, pointer.PressY)
	} else if pointer.Down || pointer.Released {
		a.dragWorkspaceSliceLine(pointer.X, pointer.Y)
	}
	if pointer.Released {
		p.drag = ""
	}
}

func (a *ModuleEditor) dragWorkspaceSliceLine(x, y float32) {
	p := &a.nineSlice
	if p.drag == "" || p.texture == nil {
		return
	}
	image := a.workspaceSliceImageRect()
	width, height := p.texture.LogicalWidth(), p.texture.LogicalHeight()
	pixelX := int(math.Round(float64(clamp((x-image.X)/image.W, 0, 1) * float32(width))))
	pixelY := int(math.Round(float64(clamp((y-image.Y)/image.H, 0, 1) * float32(height))))
	previous := p.slice
	switch p.drag {
	case "left":
		p.slice.Left = min(pixelX, width-p.slice.Right-1)
	case "right":
		p.slice.Right = min(width-pixelX, width-p.slice.Left-1)
	case "top":
		p.slice.Top = min(pixelY, height-p.slice.Bottom-1)
	case "bottom":
		p.slice.Bottom = min(height-pixelY, height-p.slice.Top-1)
	}
	if previous != p.slice {
		clear(a.dialogValues)
		a.dialogError = ""
		a.changed()
	}
}

func (a *ModuleEditor) disposeWorkspaceNineSlice() {
	if a.nineSlice.texture == nil {
		return
	}
	if a.world != nil {
		registerEditorHTMLDialogTexture(a.world, "nine-slice-image", nil)
	}
	_ = a.nineSlice.texture.Dispose()
	_ = a.nineSlice.owner.Dispose()
	a.nineSlice = workspaceNineSlice{}
}
