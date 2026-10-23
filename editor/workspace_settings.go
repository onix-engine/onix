package editor

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/diiyw/onix/project"
)

func layoutWorkspaceSettings[C ~uint8](a *ModuleEditor, dialog, title, save string, categories []settingsCategoryItem[C], category C) settingsDialogLayout[C] {
	layout := newSettingsDialogLayout[C](a.layout().Root, save)
	p := layout.panel
	a.dialogLabel(dialog, dialog+"-title", title, 24, 22, p.W-70, 28)
	a.dialogElement(dialog, dialog+"-sidebar", 1, 64, layout.sidebar.W, p.H-65).Style += ";background:#20232b"
	a.dialogElement(dialog, dialog+"-header-divider", 1, 64, p.W-2, 1).Style += ";background:#3b404b"
	a.dialogElement(dialog, dialog+"-sidebar-divider", layout.sidebar.W+1, 64, 1, p.H-65).Style += ";background:#3b404b"
	for _, item := range categories {
		r := layout.categoryRect(item.Category)
		a.dialogButton(dialog, dialog+"-category-"+item.Key, item.Label, r.X-p.X, r.Y-p.Y, r.W, r.H, category == item.Category)
		if category == item.Category {
			a.dialogLabel(dialog, dialog+"-category-title", item.Label, layout.content.X-p.X, layout.content.Y-p.Y+14, layout.content.W, 26)
		}
	}
	r := layout.action
	a.dialogButton(dialog, dialog+"-save", save, r.X-p.X, r.Y-p.Y, r.W, r.H, false)
	a.dialogClose(dialog)
	return layout
}

func (a *ModuleEditor) layoutWorkspacePreferences() {
	layout := layoutWorkspaceSettings(a, "preferences", "Preferences", "Save Preferences", preferencesCategories[:], a.preferencesCategory)
	r := layout.fieldRect(0)
	x, y := r.X-layout.panel.X, r.Y-layout.panel.Y
	a.dialogLabel("preferences", "preferences-label-preferences.snapToGrid", "Snap to Grid", layout.content.X-layout.panel.X, y+5, x-(layout.content.X-layout.panel.X)-16, 24)
	a.dialogField("preferences", "preferences.snapToGrid", "bool", strconv.FormatBool(a.preferencesDraft.SnapToGrid), x, y, r.W, r.H)
}

type workspaceSetting struct {
	name, label, kind, fallback string
	options                     []string
}

func (a *ModuleEditor) layoutWorkspaceProjectSettings() {
	layout := layoutWorkspaceSettings(a, "project", "Project Settings", "Save Project", projectSettingsCategories[:], a.projectCategory)
	var fields []workspaceSetting
	switch a.projectCategory {
	case projectSettingsWindow:
		fields = []workspaceSetting{
			{name: "window.title", label: "Window Title", kind: "string"}, {name: "window.icon", label: "App Icon", kind: "string"},
			{name: "window.width", label: "Window Width", kind: "number"}, {name: "window.height", label: "Window Height", kind: "number"},
			{name: "window.decorations", label: "Window Decorations", kind: "string", fallback: "system", options: windowDecorationOptions()},
			{name: "window.fps", label: "Target FPS", kind: "number", fallback: "60"}, {name: "window.tps", label: "Target TPS", kind: "number", fallback: "60"},
			{name: "window.transparent", label: "Transparent", kind: "bool", fallback: "false"}, {name: "window.border", label: "Window Border", kind: "bool", fallback: "true"},
			{name: "window.shadow", label: "Window Shadow", kind: "bool", fallback: "true"}, {name: "window.resizable", label: "Resizable", kind: "bool", fallback: "true"},
			{name: "window.showInTaskbar", label: "Show In Taskbar", kind: "bool", fallback: "true"}, {name: "window.alwaysOnTop", label: "Always On Top", kind: "bool", fallback: "false"},
		}
	case projectSettingsRendering:
		fields = []workspaceSetting{
			{name: "viewport.width", label: "Viewport Width", kind: "number"}, {name: "viewport.height", label: "Viewport Height", kind: "number"},
			{name: "viewport.mode", label: "Viewport Mode", kind: "string", fallback: "fit", options: viewportModeOptions()}, {name: "clearColor", label: "Clear Color", kind: "string"},
			{name: "window.renderScale", label: "Render Scale", kind: "number", fallback: "1"}, {name: "window.maxDevicePixelRatio", label: "Max Device Pixel Ratio", kind: "number", fallback: "2"},
			{name: "window.vsync", label: "VSync", kind: "bool", fallback: "true"}, {name: "window.antialias", label: "Antialias", kind: "bool", fallback: "false"},
		}
	case projectSettingsFeatures:
		for _, descriptor := range projectRuntimeFeatureDescriptors {
			fields = append(fields, workspaceSetting{name: "engine.features." + string(descriptor.ID), label: descriptor.Label, kind: "bool", fallback: strconv.FormatBool(a.projectEngine.HasFeature(descriptor.ID))})
		}
	case projectSettingsResources:
		for _, descriptor := range projectResourceDescriptors {
			fields = append(fields, workspaceSetting{name: strings.TrimPrefix(descriptor.FieldName, "project."), label: descriptor.Label, kind: "bool", fallback: strconv.FormatBool(a.projectEngine.HasResource(descriptor.ID))})
		}
		for _, descriptor := range projectImageDecoderDescriptors {
			fields = append(fields, workspaceSetting{name: strings.TrimPrefix(descriptor.FieldName, "project."), label: descriptor.Label, kind: "bool", fallback: strconv.FormatBool(projectImageDecoderEnabled(a.projectEngine.ImageDecoders, descriptor.ID))})
		}
		fields = append(fields, workspaceSetting{name: "engine.textureCompression", label: "Compress Textures", kind: "bool", fallback: strconv.FormatBool(a.projectEngine.TextureCompression)})
	case projectSettingsResourcePackages:
		a.layoutWorkspacePackages(layout)
		return
	}
	raw, _ := json.Marshal(a.Document.Spec)
	var values map[string]json.RawMessage
	_ = json.Unmarshal(raw, &values)
	spacing := min(float32(34), (layout.content.H-46)/float32(max(1, len(fields))))
	for i, field := range fields {
		value := field.fallback
		data := values
		if strings.HasPrefix(field.name, "engine.") {
			data = nil
		}
		parts := strings.Split(field.name, ".")
		for index, part := range parts {
			item := data[part]
			if len(item) == 0 || string(item) == "null" {
				break
			}
			if index == len(parts)-1 {
				if field.kind == "string" {
					_ = json.Unmarshal(item, &value)
				} else {
					value = string(item)
				}
				break
			}
			var child map[string]json.RawMessage
			if json.Unmarshal(item, &child) != nil {
				break
			}
			data = child
		}
		r := layout.fieldRect(i)
		r.Y = layout.content.Y + 46 + float32(i)*spacing
		r.H = min(28, spacing-2)
		x, y := r.X-layout.panel.X, r.Y-layout.panel.Y
		name := "project." + field.name
		a.dialogLabel("project", "project-label-"+name, field.label, layout.content.X-layout.panel.X, y+3, x-(layout.content.X-layout.panel.X)-12, r.H)
		if field.options != nil {
			a.dialogSelect("project", "project-input-"+name, name, value, field.options, x, y, r.W, r.H)
		} else if _, ok := dialogPathPickers[name]; ok {
			a.dialogPathField("project", name, field.kind, value, x, y, r.W, r.H)
		} else {
			a.dialogField("project", name, field.kind, value, x, y, r.W, r.H)
		}
	}
}

func (a *ModuleEditor) layoutWorkspacePackages(layout settingsDialogLayout[projectSettingsCategory]) {
	p, c := layout.panel, layout.content
	x, y, w := c.X-p.X, c.Y-p.Y+48, c.W
	options := make([]string, len(a.packages.Packages))
	for i, item := range a.packages.Packages {
		options[i] = item.Name
	}
	value := ""
	if a.packageIndex >= 0 && a.packageIndex < len(options) {
		value = options[a.packageIndex]
	}
	a.dialogSelect("project", "project-input-resource-packages.package", "resource-packages.package", value, options, x, y, w, 28)
	a.dialogButton("project", "project-resource-packages-add", "Add Package", x, y+38, 150, 28, false)
	remove := a.dialogButton("project", "project-resource-packages-remove", "Remove", x+160, y+38, 100, 28, false)
	remove.Disabled, remove.Class = len(options) == 0, "danger"
	fields := []workspaceDialogField{{Name: resourcePackagesKeepField, Kind: "textarea", Value: strings.Join(a.packages.Keep, "\n")}}
	labels := []string{"Keep"}
	if a.packageIndex >= 0 && a.packageIndex < len(a.packages.Packages) {
		rule := a.packages.Packages[a.packageIndex]
		fields = append(fields, workspaceDialogField{Name: resourcePackagesNameField, Kind: "string", Value: rule.Name}, workspaceDialogField{Name: resourcePackagesIncludeField, Kind: "textarea", Value: strings.Join(rule.Include, "\n")}, workspaceDialogField{Name: resourcePackagesExcludeField, Kind: "textarea", Value: strings.Join(rule.Exclude, "\n")}, workspaceDialogField{Name: resourcePackagesDependenciesField, Kind: "textarea", Value: strings.Join(rule.Dependencies, "\n")}, workspaceDialogField{Name: resourcePackagesRequiredField, Kind: "bool", Value: strconv.FormatBool(rule.Required)})
		labels = append(labels, "Name", "Include", "Exclude", "Dependencies", "Required")
	}
	rowY := y + 80
	for i, field := range fields {
		height := float32(28)
		if field.Kind == "textarea" {
			height = 60
		}
		a.dialogLabel("project", "project-label-"+field.Name, labels[i], x, rowY+4, w*.3, 28)
		a.dialogField("project", field.Name, field.Kind, field.Value, x+w*.32, rowY, w*.68, height)
		rowY += height + 12
	}
}

func (a *ModuleEditor) setWorkspacePackageValue(name, value string) {
	if name != "resource-packages.package" {
		a.packageDirty = true
	}
	patterns := func(value string) []string {
		values := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' })
		for i := range values {
			values[i] = strings.TrimSpace(values[i])
		}
		return values
	}
	if name == "resource-packages.package" {
		for i, rule := range a.packages.Packages {
			if rule.Name == value {
				a.packageIndex = i
				a.clearPackageDialogValues()
				a.changed()
				return
			}
		}
		return
	}
	if name == resourcePackagesKeepField {
		a.packages.Keep = patterns(value)
		return
	}
	if a.packageIndex < 0 || a.packageIndex >= len(a.packages.Packages) {
		return
	}
	rule := &a.packages.Packages[a.packageIndex]
	switch name {
	case resourcePackagesNameField:
		rule.Name = value
	case resourcePackagesIncludeField:
		rule.Include = patterns(value)
	case resourcePackagesExcludeField:
		rule.Exclude = patterns(value)
	case resourcePackagesDependenciesField:
		rule.Dependencies = patterns(value)
	case resourcePackagesRequiredField:
		rule.Required = value == "true"
	}
}

func (a *ModuleEditor) layoutWorkspaceBuild(p rect) {
	const dialog = "build"
	a.dialogLabel(dialog, "build-title", "Build Project", 24, 22, p.W-70, 28)
	a.dialogLabel(dialog, "build-platform-label", "Platform", 24, 70, p.W-48, 24)
	platforms := buildPlatforms()
	w := (p.W - 48) / float32(len(platforms))
	for i, platform := range platforms {
		e := a.dialogButton(dialog, "build-platform-"+string(platform), buildPlatformLabel(platform), 24+float32(i)*w, 100, w, 32, a.buildOptions.Platform == platform)
		e.IconSource = "onix-titlebar/" + string(buildPlatformIcon(platform)) + ".svg"
	}
	a.dialogLabel(dialog, "build-mode-label", "Configuration", 24, 146, p.W-48, 24)
	for i, mode := range []BuildMode{BuildModeDebug, BuildModeRelease} {
		a.dialogButton(dialog, "build-mode-"+string(mode), buildModeLabel(mode), 24+float32(i)*(p.W-48)/2, 176, (p.W-48)/2, 32, a.buildOptions.Mode == mode)
	}
	y := float32(228)
	architectures := buildArchitectures(a.buildOptions.Platform)
	if len(architectures) != 0 {
		a.dialogLabel(dialog, "build-architecture-label", "Architecture", 24, y-6, p.W-48, 24)
		w := (p.W - 48) / float32(len(architectures))
		for i, value := range architectures {
			a.dialogButton(dialog, "build-architecture-"+string(value), buildArchitectureLabel(value), 24+float32(i)*w, y+24, w, 30, a.buildOptions.Architecture == value)
		}
		y += 72
	}
	formats := buildPackageFormats(a.buildOptions.Platform)
	if len(formats) != 0 {
		a.dialogLabel(dialog, "build-package-format-label", "Package", 24, y-6, p.W-48, 24)
		w := (p.W - 48) / float32(len(formats))
		for i, value := range formats {
			a.dialogButton(dialog, "build-package-format-"+string(value), buildPackageFormatLabel(value), 24+float32(i)*w, y+24, w, 30, a.buildOptions.PackageFormat == value)
		}
		y += 72
	}
	fields := []workspaceDialogField{{Name: buildAppNameField, Kind: "string", Value: a.buildOptions.AppName}, {Name: buildIconField, Kind: "string", Value: a.buildOptions.Icon}}
	labels := []string{"App Name", "Icon"}
	switch a.buildOptions.Platform {
	case BuildPlatformAndroid:
		fields = append(fields, workspaceDialogField{Name: buildJavaPackageField, Kind: "string", Value: a.buildOptions.JavaPackage}, workspaceDialogField{Name: buildAndroidMinSDKField, Kind: "number", Value: strconv.Itoa(a.buildOptions.AndroidMinSDK)}, workspaceDialogField{Name: buildAndroidSDKField, Kind: "string", Value: a.buildOptions.AndroidSDK}, workspaceDialogField{Name: buildAndroidNDKField, Kind: "string", Value: a.buildOptions.AndroidNDK})
		labels = append(labels, "Java Package", "Minimum SDK", "Android SDK", "Android NDK")
	case BuildPlatformWeb:
		fields = append(fields, workspaceDialogField{Name: buildWebTitleField, Kind: "string", Value: a.buildOptions.WebTitle})
		labels = append(labels, "Page Title")
	case BuildPlatformWeChat:
		fields = append(fields, workspaceDialogField{Name: buildWeChatAppIDField, Kind: "string", Value: a.buildOptions.WeChatAppID})
		labels = append(labels, "App ID")
	}
	for i, field := range fields {
		row := float32(i / 2)
		col := float32(i % 2)
		w := (p.W - 60) / 2
		x := 24 + col*(w+12)
		rowY := y + row*62
		a.dialogLabel(dialog, "build-label-"+field.Name, labels[i], x, rowY, w, 22)
		if _, ok := dialogPathPickers[field.Name]; ok {
			a.dialogPathField(dialog, field.Name, field.Kind, field.Value, x, rowY+24, w, 30)
			continue
		}
		a.dialogField(dialog, field.Name, field.Kind, field.Value, x, rowY+24, w, 30)
	}
	a.dialogLabel(dialog, "build-output-label", "Output", 24, p.H-88, p.W-48, 20)
	a.dialogLabel(dialog, "build-output", buildPlatformOutputDir(a.Document.ProjectDir, a.buildOptions.Platform), 24, p.H-64, p.W-230, 40)
	a.dialogButton(dialog, "build-submit", "Build Project", p.W-190, p.H-54, 166, 32, false)
	a.dialogClose(dialog)
}

func (a *ModuleEditor) handleWorkspaceBuildAction(id string) {
	switch {
	case strings.HasPrefix(id, "build-platform-"):
		platform := BuildPlatform(strings.TrimPrefix(id, "build-platform-"))
		for _, value := range buildPlatforms() {
			if value == platform {
				a.buildOptions.Platform = platform
				a.buildOptions.PackageFormat, _ = normalizedBuildPackageFormat(platform, "")
				a.buildOptions.Architecture = defaultBuildArchitecture()
				a.changed()
				break
			}
		}
	case strings.HasPrefix(id, "build-mode-"):
		a.buildOptions.Mode = BuildMode(strings.TrimPrefix(id, "build-mode-"))
		a.changed()
	case strings.HasPrefix(id, "build-architecture-"):
		a.buildOptions.Architecture = BuildArchitecture(strings.TrimPrefix(id, "build-architecture-"))
		a.changed()
	case strings.HasPrefix(id, "build-package-format-"):
		a.buildOptions.PackageFormat = BuildPackageFormat(strings.TrimPrefix(id, "build-package-format-"))
		a.changed()
	case id == "build-submit":
		if a.buildOptions.AndroidMinSDK < 1 {
			a.report(fmt.Errorf("Minimum SDK must be positive"))
			return
		}
		if err := a.startModuleBuild(); err != nil {
			a.buildError = err.Error()
			a.buildOpen = false
			a.showWorkspaceDialog("build-error")
		}
	}
}

func (a *ModuleEditor) setWorkspaceBuildValue(name, value string) {
	switch name {
	case buildAppNameField:
		a.buildOptions.AppName = value
	case buildIconField:
		a.buildOptions.Icon = value
	case buildJavaPackageField:
		a.buildOptions.JavaPackage = value
	case buildAndroidMinSDKField:
		a.buildOptions.AndroidMinSDK, _ = strconv.Atoi(value)
	case buildAndroidSDKField:
		a.buildOptions.AndroidSDK = value
	case buildAndroidNDKField:
		a.buildOptions.AndroidNDK = value
	case buildWebTitleField:
		a.buildOptions.WebTitle = value
	case buildWeChatAppIDField:
		a.buildOptions.WeChatAppID = value
	}
}

func (a *ModuleEditor) setWorkspaceEngineValue(name, value string) {
	config := a.projectEngine
	switch {
	case strings.HasPrefix(name, "project.engine.features."):
		config, _ = setProjectRuntimeFeature(config, project.RuntimeFeatureID(strings.TrimPrefix(name, "project.engine.features.")), value == "true")
	case strings.HasPrefix(name, "project.engine.resources."):
		config, _ = setProjectResource(config, project.ResourceID(strings.TrimPrefix(name, "project.engine.resources.")), value == "true")
	case strings.HasPrefix(name, "project.engine.imageDecoders."):
		config.ImageDecoders = setProjectImageDecoder(config.ImageDecoders, project.ImageDecoderID(strings.TrimPrefix(name, "project.engine.imageDecoders.")), value == "true")
	case name == "project.engine.textureCompression":
		config.TextureCompression = value == "true"
	default:
		return
	}
	a.projectEngine, a.projectEngineDirty = config, true
	for id, field := range a.dialogFields {
		if strings.HasPrefix(field.Name, "project.engine.") {
			delete(a.dialogValues, id)
		}
	}
	a.changed()
}
