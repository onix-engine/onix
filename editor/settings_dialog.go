package editor

import (
	"github.com/diiyw/onix/window"
)

type projectSettingsCategory uint8

type preferencesCategory uint8

const (
	preferencesEditing preferencesCategory = iota
	preferencesCategoryCount
)

const (
	projectSettingsWindow projectSettingsCategory = iota
	projectSettingsRendering
	projectSettingsFeatures
	projectSettingsResources
	projectSettingsResourcePackages
	projectSettingsCategoryCount
)

const (
	resourcePackagesKeepField         = "resource-packages.keep"
	resourcePackagesNameField         = "resource-packages.name"
	resourcePackagesIncludeField      = "resource-packages.include"
	resourcePackagesExcludeField      = "resource-packages.exclude"
	resourcePackagesDependenciesField = "resource-packages.dependencies"
	resourcePackagesRequiredField     = "resource-packages.required"
)

type settingsCategoryItem[C ~uint8] struct {
	Category C
	Key      string
	Label    string
}

var preferencesCategories = [preferencesCategoryCount]settingsCategoryItem[preferencesCategory]{
	{Category: preferencesEditing, Key: "editing", Label: "Editing"},
}

var projectSettingsCategories = [projectSettingsCategoryCount]settingsCategoryItem[projectSettingsCategory]{
	{Category: projectSettingsWindow, Key: "window", Label: "Window"},
	{Category: projectSettingsRendering, Key: "rendering", Label: "Rendering & Viewport"},
	{Category: projectSettingsFeatures, Key: "features", Label: "Runtime Features"},
	{Category: projectSettingsResources, Key: "resources", Label: "Resources & Decoders"},
	{Category: projectSettingsResourcePackages, Key: "resource-packages", Label: "Resource Packages"},
}

type settingsDialogLayout[C ~uint8] struct {
	panel   rect
	sidebar rect
	content rect
	action  rect
}

func newSettingsDialogLayout[C ~uint8](root rect, actionLabel string) settingsDialogLayout[C] {
	width := min(root.W-48, float32(900))
	height := min(root.H-48, float32(620))
	panel := rect{X: (root.W - width) * 0.5, Y: (root.H - height) * 0.5, W: width, H: height}
	actionWidth := buttonTextWidth(actionLabel, true)
	action := rect{X: panel.X + panel.W - actionWidth - 24, Y: panel.Y + panel.H - 48, W: actionWidth, H: editorCompactControlHeight}
	sidebar := rect{X: panel.X + 1, Y: panel.Y + 64, W: 240, H: panel.H - 65}
	content := rect{
		X: sidebar.X + sidebar.W + 24,
		Y: panel.Y + 64,
		W: panel.X + panel.W - 24 - (sidebar.X + sidebar.W + 24),
		H: action.Y - 12 - (panel.Y + 64),
	}
	return settingsDialogLayout[C]{panel: panel, sidebar: sidebar, content: content, action: action}
}

func (layout settingsDialogLayout[C]) categoryRect(category C) rect {
	return rect{
		X: layout.sidebar.X + 12,
		Y: layout.sidebar.Y + 16 + float32(category)*40,
		W: layout.sidebar.W - 24,
		H: 34,
	}
}

func (layout settingsDialogLayout[C]) fieldRect(index int) rect {
	labelWidth := min(float32(220), layout.content.W*0.38)
	return rect{
		X: layout.content.X + labelWidth + 20,
		Y: layout.content.Y + 48 + float32(index)*34,
		W: layout.content.W - labelWidth - 20,
		H: editorFieldHeight,
	}
}

func panelCloseRect(panel rect) rect {
	return rect{X: panel.X + panel.W - 38, Y: panel.Y + 12, W: editorCompactControlHeight, H: editorCompactControlHeight}
}

func viewportModeOptions() []string {
	return []string{
		string(window.ViewportModeContain), string(window.ViewportModeCover), string(window.ViewportModeStretch),
		string(window.ViewportModeFixedWidth), string(window.ViewportModeFixedHeight), string(window.ViewportModeExpand),
		string(window.ViewportModeAdaptive), string(window.ViewportModeInteger),
	}
}

func windowDecorationOptions() []string {
	return []string{string(window.DecorationSystem), string(window.DecorationNone)}
}
