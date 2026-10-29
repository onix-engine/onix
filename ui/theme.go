package ui

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	onixtext "github.com/diiyw/onix/text"
)

func alphaColor(clr graphics.ColorRGBA, alpha float32) graphics.ColorRGBA {
	clr.A *= alpha
	return clr
}

type Theme struct {
	Font       *onixtext.Font
	FontSize   float64
	DPI        float64
	StackIndex int

	Background      graphics.ColorRGBA
	Surface         graphics.ColorRGBA
	SurfaceHover    graphics.ColorRGBA
	SurfaceActive   graphics.ColorRGBA
	SurfaceDisabled graphics.ColorRGBA
	Primary         graphics.ColorRGBA
	PrimaryHover    graphics.ColorRGBA
	PrimaryActive   graphics.ColorRGBA
	Text            graphics.ColorRGBA
	Muted           graphics.ColorRGBA
	Accent          graphics.ColorRGBA
	Danger          graphics.ColorRGBA
	Selection       graphics.ColorRGBA
}

func DefaultTheme() Theme {
	return Theme{
		Font:            onixtext.DefaultFont(),
		FontSize:        18,
		DPI:             72,
		StackIndex:      0,
		Background:      graphics.ColorFromRGBA(20, 24, 31, 255),
		Surface:         graphics.ColorFromRGBA(42, 48, 59, 255),
		SurfaceHover:    graphics.ColorFromRGBA(55, 63, 77, 255),
		SurfaceActive:   graphics.ColorFromRGBA(68, 78, 96, 255),
		SurfaceDisabled: graphics.ColorFromRGBA(60, 64, 72, 160),
		Primary:         graphics.ColorFromRGBA(40, 120, 255, 255),
		PrimaryHover:    graphics.ColorFromRGBA(66, 140, 255, 255),
		PrimaryActive:   graphics.ColorFromRGBA(28, 95, 220, 255),
		Text:            graphics.ColorFromRGBA(242, 245, 250, 255),
		Muted:           graphics.ColorFromRGBA(160, 169, 184, 255),
		Accent:          graphics.ColorFromRGBA(74, 213, 162, 255),
		Danger:          graphics.ColorFromRGBA(244, 96, 109, 255),
	}
}

func themeWithDefaults(theme Theme) Theme {
	return themeWithFallback(theme, DefaultTheme())
}

// ThemeWithDefaults fills omitted values with the standard UI theme.
func ThemeWithDefaults(theme Theme) Theme { return themeWithDefaults(theme) }

func themeWithFallback(theme, fallback Theme) Theme {
	if theme.Font == nil {
		theme.Font = fallback.Font
	}
	if theme.FontSize == 0 {
		theme.FontSize = fallback.FontSize
	}
	if theme.DPI == 0 {
		theme.DPI = fallback.DPI
	}
	theme.Background = colorOr(theme.Background, fallback.Background)
	theme.Surface = colorOr(theme.Surface, fallback.Surface)
	theme.SurfaceHover = colorOr(theme.SurfaceHover, fallback.SurfaceHover)
	theme.SurfaceActive = colorOr(theme.SurfaceActive, fallback.SurfaceActive)
	theme.SurfaceDisabled = colorOr(theme.SurfaceDisabled, fallback.SurfaceDisabled)
	theme.Primary = colorOr(theme.Primary, fallback.Primary)
	theme.PrimaryHover = colorOr(theme.PrimaryHover, fallback.PrimaryHover)
	theme.PrimaryActive = colorOr(theme.PrimaryActive, fallback.PrimaryActive)
	theme.Text = colorOr(theme.Text, fallback.Text)
	theme.Muted = colorOr(theme.Muted, fallback.Muted)
	theme.Accent = colorOr(theme.Accent, fallback.Accent)
	theme.Danger = colorOr(theme.Danger, fallback.Danger)
	theme.Selection = colorOr(theme.Selection, alphaColor(theme.Primary, 0.55))
	return theme
}

func themeForWorld(world *ecs.World, theme Theme) Theme {
	fallback := DefaultTheme()
	if configured, ok := world.GetResource[Theme](); ok {
		fallback = themeWithFallback(configured, fallback)
	}
	return themeWithFallback(theme, fallback)
}

// ThemeForWorld resolves a theme against the world's configured theme.
func ThemeForWorld(world *ecs.World, theme Theme) Theme { return themeForWorld(world, theme) }

func colorOr(value, fallback graphics.ColorRGBA) graphics.ColorRGBA {
	if value.R == 0 && value.G == 0 && value.B == 0 && value.A == 0 {
		return fallback
	}
	return value
}
