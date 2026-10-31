package editor

import (
	"fmt"
	"image"
	"math"
	"strconv"
	"strings"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/project"
	htmlui "github.com/diiyw/onix/ui/html"
)

type workspaceColorPicker struct {
	field, mode, drag                  string
	anchor                             rect
	rgba                               [4]uint8
	hue, saturation, value, textureHue float64
	sv, hues                           *graphics.Texture
	pixels                             []byte
}

func (a *ModuleEditor) openWorkspaceColorPicker(id string) error {
	field, ok := a.fields[id]
	if !ok || field.Kind != "color" {
		return fmt.Errorf("select a color field")
	}
	value := field.Value
	if draft, ok := a.drafts[id]; ok {
		value = draft
	}
	rgba, err := workspacePickerRGBA(value)
	if err != nil {
		rgba = [4]uint8{255, 255, 255, 255}
	}
	anchor := rect{X: a.width - a.rightPanelWidth, Y: a.height / 2}
	if node, ok := htmlui.NodeByID(a.world, a.shell, "content-color:"+id); ok {
		if bounds, ok := node.Bounds(); ok {
			anchor = rect{X: bounds.X, Y: bounds.Y, W: bounds.Width, H: bounds.Height}
		}
	}
	a.showWorkspaceDialog("color-picker")
	p := &a.colorPicker
	p.field, p.mode, p.anchor, p.rgba = id, "hex", anchor, rgba
	p.hue, p.saturation, p.value = rgbToHSV(rgba[0], rgba[1], rgba[2])
	p.rebuildTextures()
	return nil
}

func workspacePickerRGBA(value string) ([4]uint8, error) {
	color, err := project.ParseColor(value, 1)
	if err != nil {
		return [4]uint8{}, err
	}
	return [4]uint8{
		uint8(math.Round(float64(color.R) * 255)), uint8(math.Round(float64(color.G) * 255)),
		uint8(math.Round(float64(color.B) * 255)), uint8(math.Round(float64(color.A) * 255)),
	}, nil
}

func (p *workspaceColorPicker) hex() string {
	if p.rgba[3] == 255 {
		return fmt.Sprintf("#%02x%02x%02x", p.rgba[0], p.rgba[1], p.rgba[2])
	}
	return fmt.Sprintf("#%02x%02x%02x%02x", p.rgba[0], p.rgba[1], p.rgba[2], p.rgba[3])
}

func (a *ModuleEditor) workspaceColorPickerRect() rect {
	const gap = float32(8)
	height := float32(278)
	if a.dialogError != "" {
		height += 36
	}
	p := rect{W: min(328, a.width-2*gap), H: min(height, a.height-2*gap)}
	anchor := a.colorPicker.anchor
	p.X = anchor.X - p.W - gap
	if p.X < gap {
		p.X = anchor.X + anchor.W + gap
	}
	p.X = clamp(p.X, gap, a.width-p.W-gap)
	p.Y = clamp(anchor.Y+(anchor.H-p.H)/2, gap, a.height-p.H-gap)
	return p
}

func (a *ModuleEditor) workspaceColorPickerAreas() (sv, hue rect) {
	panel := a.workspaceColorPickerRect()
	sv = rect{X: panel.X + 16, Y: panel.Y + 84, W: panel.W - 64, H: 128}
	hue = rect{X: sv.X + sv.W + 10, Y: sv.Y, W: 22, H: sv.H}
	return
}

func (a *ModuleEditor) layoutWorkspaceColorPicker(world *ecs.World, panel rect) {
	const name = "color-picker"
	p := &a.colorPicker
	a.dialogLabel(name, "color-picker-title", "Color", 16, 12, panel.W-64, 24)
	a.dialogButton(name, "color-picker-mode-hex", "HEX", 16, 42, 86, 28, p.mode == "hex")
	a.dialogButton(name, "color-picker-mode-rgba", "RGBA", 106, 42, 86, 28, p.mode == "rgba")
	a.dialogClose(name)
	if p.mode == "rgba" {
		for i, channel := range [...]string{"r", "g", "b", "a"} {
			width := (panel.W - 44) / 2
			x, y := 16+float32(i%2)*(width+12), 104+float32(i/2)*60
			a.dialogLabel(name, "color-picker-label-color-picker."+channel, strings.ToUpper(channel), x, y-22, width, 20)
			a.dialogField(name, "color-picker."+channel, "number", strconv.Itoa(int(p.rgba[i])), x, y, width, 32).Step = "1"
		}
		a.dialogElement(name, "color-picker-preview", 16, 220, panel.W-32, 32).Style += ";background:" + p.hex()
		return
	}
	registerEditorHTMLDialogTexture(world, "color-picker-sv", p.sv)
	registerEditorHTMLDialogTexture(world, "color-picker-hue", p.hues)
	sv, hue := a.workspaceColorPickerAreas()
	a.dialogElement(name, "color-picker-sv", sv.X-panel.X, sv.Y-panel.Y, sv.W, sv.H).Source = "onix-dialog/color-picker-sv"
	a.dialogElement(name, "color-picker-hue", hue.X-panel.X, hue.Y-panel.Y, hue.W, hue.H).Source = "onix-dialog/color-picker-hue"
	x, y := sv.X-panel.X+float32(p.saturation)*sv.W-5, sv.Y-panel.Y+float32(1-p.value)*sv.H-5
	a.dialogElement(name, "color-picker-sv-indicator-shadow", x-1, y-1, 12, 12).Style += ";border:1px solid #111;border-radius:50%"
	a.dialogElement(name, "color-picker-sv-indicator", x, y, 10, 10).Style += ";border:1px solid #fff;border-radius:50%"
	y = hue.Y - panel.Y + float32(p.hue/360)*hue.H - 2
	a.dialogElement(name, "color-picker-hue-indicator-shadow", hue.X-panel.X-4, y-1, hue.W+8, 6).Style += ";background:#111"
	a.dialogElement(name, "color-picker-hue-indicator", hue.X-panel.X-3, y, hue.W+6, 4).Style += ";background:#fff"
	a.dialogField(name, "color-picker.hex", "string", p.hex(), 16, 230, panel.W-32, 32)
}

func (a *ModuleEditor) setWorkspacePickerValue(name, value string) error {
	p := &a.colorPicker
	rgba := p.rgba
	if name == "color-picker.hex" {
		var err error
		rgba, err = workspacePickerRGBA(value)
		if err != nil {
			return err
		}
	} else {
		index := -1
		for i, channel := range [...]string{"color-picker.r", "color-picker.g", "color-picker.b", "color-picker.a"} {
			if name == channel {
				index = i
				break
			}
		}
		if index < 0 {
			return nil
		}
		number, err := strconv.ParseUint(value, 10, 8)
		if err != nil {
			return fmt.Errorf("color channel must be an integer from 0 to 255")
		}
		rgba[index] = uint8(number)
	}
	if rgba[0] != p.rgba[0] || rgba[1] != p.rgba[1] || rgba[2] != p.rgba[2] {
		h, s, v := rgbToHSV(rgba[0], rgba[1], rgba[2])
		if s != 0 {
			p.hue = h
		}
		p.saturation, p.value = s, v
	}
	p.rgba = rgba
	p.rebuildTextures()
	a.stageWorkspacePickerColor()
	// Preserve text while it is being typed: expanding #rgb here would change
	// the input and caret before the user can finish a six/eight-digit color.
	a.dialogValues[htmlDialogFieldID("color-picker", name)] = value
	return nil
}

func (a *ModuleEditor) stageWorkspacePickerColor() {
	p := &a.colorPicker
	a.drafts[p.field] = p.hex()
	clear(a.dialogValues)
	a.dialogError = ""
	a.status = "Color modified. Press Ctrl+S to save"
	a.changed()
}

func (a *ModuleEditor) dragWorkspacePicker(x, y float32) {
	p := &a.colorPicker
	sv, hue := a.workspaceColorPickerAreas()
	h, s, v := p.hue, p.saturation, p.value
	switch p.drag {
	case "hue":
		h = float64(clamp((y-hue.Y)/hue.H, 0, 1)) * 360
	case "sv":
		s = float64(clamp((x-sv.X)/sv.W, 0, 1))
		v = 1 - float64(clamp((y-sv.Y)/sv.H, 0, 1))
	default:
		return
	}
	if h == p.hue && s == p.saturation && v == p.value {
		return
	}
	p.hue, p.saturation, p.value = h, s, v
	p.rgba[0], p.rgba[1], p.rgba[2] = hsvToRGB(h, s, v)
	p.rebuildTextures()
	a.stageWorkspacePickerColor()
}

func (p *workspaceColorPicker) rebuildTextures() {
	const width, height = 192, 128
	hue := math.Mod(p.hue, 360)
	if p.sv == nil || p.textureHue != hue {
		if p.pixels == nil {
			p.pixels = make([]byte, width*height*4)
		}
		for y := range height {
			for x := range width {
				r, g, b := hsvToRGB(hue, float64(x)/(width-1), 1-float64(y)/(height-1))
				pixel := p.pixels[(y*width+x)*4:][:4]
				pixel[0], pixel[1], pixel[2], pixel[3] = r, g, b, 255
			}
		}
		if p.sv == nil {
			pixels := image.RGBA{Pix: p.pixels, Stride: width * 4, Rect: image.Rect(0, 0, width, height)}
			p.sv = graphics.CreateTextureByImageNoMipmap(&pixels)
		} else {
			p.sv.UpdateRegions([]graphics.TextureRegionUpdate{{Width: width, Height: height, Pixels: p.pixels}})
		}
		p.textureHue = hue
	}
	if p.hues == nil {
		pixels := image.NewRGBA(image.Rect(0, 0, 16, height))
		for y := range height {
			r, g, b := hsvToRGB(float64(y)*360/(height-1), 1, 1)
			for x := range 16 {
				pixel := pixels.Pix[(y*16+x)*4:][:4]
				pixel[0], pixel[1], pixel[2], pixel[3] = r, g, b, 255
			}
		}
		p.hues = graphics.CreateTextureBySharedImage(pixels, false, graphics.ResourceUsageDefault)
	}
}

func (a *ModuleEditor) updateWorkspacePopupPointer(pointer input.Pointer) {
	switch a.dialog {
	case "color-picker":
		p := &a.colorPicker
		if pointer.Pressed {
			if p.mode == "hex" {
				sv, hue := a.workspaceColorPickerAreas()
				switch {
				case workspacePopupContains(sv, pointer.PressX, pointer.PressY):
					p.drag = "sv"
				case workspacePopupContains(hue, pointer.PressX, pointer.PressY):
					p.drag = "hue"
				}
			}
			if !workspacePopupContains(a.workspaceColorPickerRect(), pointer.PressX, pointer.PressY) {
				a.closeWorkspaceDialog()
				return
			}
		}
		if pointer.Pressed && p.drag != "" {
			a.dragWorkspacePicker(pointer.PressX, pointer.PressY)
		} else if pointer.Down || pointer.Released {
			a.dragWorkspacePicker(pointer.X, pointer.Y)
		}
		if pointer.Released {
			p.drag = ""
		}
	case "nine-slice":
		a.updateWorkspaceNineSlicePointer(pointer)
	}
}

func workspacePopupContains(r rect, x, y float32) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

func (a *ModuleEditor) closeWorkspacePopup() {
	a.colorPicker.field, a.colorPicker.drag = "", ""
	a.disposeWorkspaceNineSlice()
}

func (a *ModuleEditor) disposeWorkspacePopups() {
	a.closeWorkspacePopup()
	if a.world != nil {
		registerEditorHTMLDialogTexture(a.world, "color-picker-sv", nil)
		registerEditorHTMLDialogTexture(a.world, "color-picker-hue", nil)
	}
	_ = a.colorPicker.sv.Dispose()
	_ = a.colorPicker.hues.Dispose()
	a.colorPicker = workspaceColorPicker{}
}

func hsvToRGB(h, s, v float64) (uint8, uint8, uint8) {
	h = math.Mod(h, 360) / 60
	c, x := v*s, v*s*(1-math.Abs(math.Mod(h, 2)-1))
	m := v - c
	var r, g, b float64
	switch int(h) {
	case 0:
		r, g = c, x
	case 1:
		r, g = x, c
	case 2:
		g, b = c, x
	case 3:
		g, b = x, c
	case 4:
		r, b = x, c
	default:
		r, b = c, x
	}
	return uint8(math.Round((r + m) * 255)), uint8(math.Round((g + m) * 255)), uint8(math.Round((b + m) * 255))
}

func rgbToHSV(r, g, b uint8) (h, s, v float64) {
	rf, gf, bf := float64(r)/255, float64(g)/255, float64(b)/255
	maxValue, minValue := max(rf, gf, bf), min(rf, gf, bf)
	delta := maxValue - minValue
	v = maxValue
	if maxValue != 0 {
		s = delta / maxValue
	}
	if delta == 0 {
		return
	}
	switch maxValue {
	case rf:
		h = 60 * math.Mod((gf-bf)/delta, 6)
	case gf:
		h = 60 * ((bf-rf)/delta + 2)
	default:
		h = 60 * ((rf-gf)/delta + 4)
	}
	if h < 0 {
		h += 360
	}
	return
}
