package editor

import (
	"strings"
	"testing"

	editorresources "github.com/diiyw/onix/editor/resources"
)

func TestEditorDialogResourcesAreComponents(t *testing.T) {
	for _, name := range append(editorDialogNames[:], "welcome") {
		source := editorresources.DialogHTML(name)
		if strings.Contains(source, "<!doctype") || strings.Contains(source, "<html") || strings.Contains(source, "<body") {
			t.Fatalf("dialog %q is still a complete HTML document", name)
		}
		if strings.Contains(source, "-template") {
			t.Fatalf("dialog %q still relies on a template clone node", name)
		}
	}
	for _, name := range editorDialogNames {
		definition, err := editorDialogDefinition(name)
		if err != nil {
			t.Fatalf("define dialog %q: %v", name, err)
		}
		if !definition.ScopedCSS || definition.Behavior.Render == nil {
			t.Fatalf("dialog %q is missing scoped component rendering", name)
		}
	}
	if _, err := editorWelcomeDefinitions(); err != nil {
		t.Fatalf("define welcome dialog: %v", err)
	}
}

func TestEditorDialogStaticTextIsAuthored(t *testing.T) {
	fragments := map[string][]string{
		"about": {
			`data-ref="about-version"`,
			`data-ref="about-author"`, `>Author: diiyw<`,
		},
		"close-confirm": {
			`data-ref="close-confirm-body"`, `>This project has unsaved changes.<`,
			`data-ref="close-confirm-discard"`, `>Don't Save<`, `data-ref="close-confirm-cancel"`, `>Cancel<`,
		},
		"build": {
			`data-ref="build-title"`, `>Build Project<`, `data-ref="build-mode-debug"`, `>Debug<`,
			`data-ref="build-label-build.app-name"`, `>App Name<`,
			`data-ref="build-platform-windows"`, `>Windows<`,
		},
		"build-success": {
			`data-ref="build-success-title"`, `>Build Successful<`, `data-ref="build-success-open-directory"`, `>Open Directory<`,
		},
		"build-error": {
			`data-ref="build-error-title"`, `>Build Failed<`, `data-ref="build-error-copy"`, `>Copy Error<`,
		},
		"preferences": {
			`data-ref="preferences-title"`, `>Preferences<`, `data-ref="preferences-save"`, `>Save Preferences<`,
			`data-ref="preferences-label-preferences.snapToGrid"`, `>Snap to Grid<`,
		},
		"project": {
			`data-ref="project-title"`, `>Project Settings<`, `data-ref="project-save"`, `>Save Project<`,
			`data-ref="project-label-project.window.title"`, `>Window Title<`,
			`data-ref="project-label-project.engine.features.render2d"`, `>Render 2D<`,
		},
		"project-dialog": {
			`data-ref="project-dialog-title"`, `>New Project<`, `data-ref="project-dialog-confirm"`, `>Create<`,
			`data-ref="project-dialog-label-project-dialog.name"`, `>Project Name<`,
		},
		"run-failure": {
			`data-ref="run-failure-copy"`, `>Copy Output<`, `data-ref="run-failure-ok"`, `>OK<`,
		},
	}
	for dialog, expected := range fragments {
		source := editorresources.DialogHTML(dialog)
		for _, fragment := range expected {
			if !strings.Contains(source, fragment) {
				t.Fatalf("dialog %q is missing authored static text fragment %q", dialog, fragment)
			}
		}
	}
}

func TestEditorChromeStaticTextIsAuthored(t *testing.T) {
	for _, fragment := range []string{
		`id="menu-file"`, `>File<`,
		`id="file.open-project"`, `>Open Project<`,
		`id="debug.build"`, `>Build<`,
	} {
		if !strings.Contains(editorresources.TitlebarHTML, fragment) {
			t.Fatalf("titlebar is missing authored static text fragment %q", fragment)
		}
	}
}

func TestEditorDialogFieldsAreAuthoredHTMLControls(t *testing.T) {
	fields := map[string][]string{
		"preferences": {
			"preferences.snapToGrid",
		},
		"project": {
			"project.window.title", "project.window.icon", "project.window.width", "project.window.height", "project.window.decorations", "project.window.fps", "project.window.tps",
			"project.window.transparent", "project.window.border", "project.window.shadow", "project.window.resizable", "project.window.showInTaskbar", "project.window.alwaysOnTop",
			"project.viewport.width", "project.viewport.height", "project.viewport.mode", "project.clearColor", "project.window.renderScale", "project.window.maxDevicePixelRatio", "project.window.vsync", "project.window.antialias",
			"project.engine.textureCompression", "project.engine.features.render2d", "project.engine.features.render3d", "project.engine.features.input2d", "project.engine.features.animations2d", "project.engine.features.sprite-trails2d", "project.engine.features.particles2d", "project.engine.features.tilemaps2d", "project.engine.features.spine2d", "project.engine.features.physics2d", "project.engine.features.physics3d", "project.engine.features.ui-controls", "project.engine.features.ui-widgets", "project.engine.features.ui-text-input", "project.engine.features.ui-markdown", "project.engine.features.ui-html", "project.engine.features.webview", "project.engine.features.file-dialogs",
			"project.engine.resources.images", "project.engine.resources.fonts", "project.engine.resources.audio", "project.engine.resources.videos", "project.engine.resources.particles", "project.engine.resources.tilemaps", "project.engine.resources.models", "project.engine.resources.spine", "project.engine.resources.shaders", "project.engine.resources.html",
			"project.engine.imageDecoders.gif", "project.engine.imageDecoders.jpeg", "project.engine.imageDecoders.png", "project.engine.imageDecoders.webp",
			"resource-packages.keep", "resource-packages.name", "resource-packages.include", "resource-packages.exclude", "resource-packages.dependencies", "resource-packages.required",
		},
		"project-dialog":     {"project-dialog.name", "project-dialog.width", "project-dialog.height"},
		"build":              {buildAppNameField, buildIconField, buildJavaPackageField, buildAndroidMinSDKField, buildAndroidSDKField, buildAndroidNDKField, buildWebTitleField, buildWeChatAppIDField},
		"run-failure":        {"output"},
		"instance-duplicate": {"instance-name"},
		"color-picker":       {"color-picker.hex", "color-picker.r", "color-picker.g", "color-picker.b", "color-picker.a"},
		"nine-slice":         {"left", "right", "top", "bottom"},
	}
	for dialog, names := range fields {
		source := editorresources.DialogHTML(dialog)
		for _, name := range names {
			refID := htmlDialogFieldID(dialog, name)
			if dialog == "project" && (name == "project.window.decorations" || name == "project.viewport.mode") {
				refID = "project-input-" + name
			}
			ref := `data-ref="` + refID + `"`
			if !strings.Contains(source, ref) {
				t.Fatalf("dialog %q field %q is not authored in static HTML", dialog, name)
			}
		}
	}
}
