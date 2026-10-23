package editor

import (
	"github.com/diiyw/onix/graphics"
	onixtext "github.com/diiyw/onix/text"
	engineui "github.com/diiyw/onix/ui"
)

type theme struct {
	windowBG     graphics.ColorRGBA
	panel        graphics.ColorRGBA
	panelSoft    graphics.ColorRGBA
	panelBorder  graphics.ColorRGBA
	text         graphics.ColorRGBA
	textMuted    graphics.ColorRGBA
	accent       graphics.ColorRGBA
	hover        graphics.ColorRGBA
	danger       graphics.ColorRGBA
	viewportBG   graphics.ColorRGBA
	viewport3DBG graphics.ColorRGBA
	gridMajor    graphics.ColorRGBA
	gridMinor    graphics.ColorRGBA
	axisX        graphics.ColorRGBA
	axisY        graphics.ColorRGBA
	axisZ        graphics.ColorRGBA
	axisOrigin   graphics.ColorRGBA
	inputBG      graphics.ColorRGBA
	inputFocus   graphics.ColorRGBA
	selection    graphics.ColorRGBA
}

var editorTheme = theme{
	windowBG:     rgba(24, 27, 34, 255),
	panel:        rgba(33, 36, 44, 255),
	panelSoft:    rgba(40, 44, 53, 255),
	panelBorder:  rgba(61, 67, 79, 255),
	text:         rgba(231, 235, 243, 255),
	textMuted:    rgba(170, 180, 196, 255),
	accent:       rgba(132, 204, 255, 255),
	hover:        rgba(52, 56, 66, 255),
	danger:       rgba(255, 122, 122, 255),
	viewportBG:   rgba(38, 42, 50, 255),
	viewport3DBG: rgba(48, 48, 48, 255),
	gridMajor:    rgba(152, 164, 184, 210),
	gridMinor:    rgba(100, 112, 132, 160),
	axisX:        rgba(192, 72, 72, 255),
	axisY:        rgba(72, 168, 88, 255),
	axisZ:        rgba(72, 112, 192, 255),
	axisOrigin:   rgba(255, 235, 160, 235),
	inputBG:      rgba(27, 30, 37, 255),
	inputFocus:   rgba(59, 101, 137, 255),
	selection:    rgba(132, 204, 255, 128),
}

func rgba(r, g, b, a uint8) graphics.ColorRGBA {
	return graphics.ColorFromRGBA(r, g, b, a)
}

func editorUITheme(font *onixtext.Font) engineui.Theme {
	return engineui.Theme{
		Font:            font,
		FontSize:        14,
		DPI:             72,
		StackIndex:      0,
		Background:      editorTheme.panel,
		Surface:         editorTheme.inputBG,
		SurfaceHover:    editorTheme.hover,
		SurfaceActive:   editorTheme.inputFocus,
		SurfaceDisabled: editorTheme.panelSoft,
		Primary:         editorTheme.panelSoft,
		PrimaryHover:    editorTheme.hover,
		PrimaryActive:   editorTheme.inputFocus,
		Text:            editorTheme.text,
		Muted:           editorTheme.textMuted,
		Accent:          editorTheme.accent,
		Danger:          editorTheme.danger,
		Selection:       editorTheme.selection,
	}
}
