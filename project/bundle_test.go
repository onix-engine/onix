package project

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	stdpng "image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	sceneapi "github.com/diiyw/onix/scene"
	engineui "github.com/diiyw/onix/ui"
	"github.com/diiyw/onix/window"
)

func TestExplicitControlBindingComposesAuthoredParts(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{ "window": { "width": 640, "height": 480 } }`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "control": {},
      "anchor": [
        -0.5,
        0.5
      ],
      "children": [
        {
          "anchor": [
            -0.5,
            0.5
          ],
          "components": {
            "sprite": {
              "texture": {
                "color": "#2878ffff"
              }
            }
          },
          "coordinateSpace": "ui",
          "height": 40,
          "id": "run-normal",
          "width": 120
        },
        {
          "anchor": [
            -0.5,
            0.5
          ],
          "components": {
            "sprite": {
              "texture": {
                "color": "#428cffff"
              }
            }
          },
          "coordinateSpace": "ui",
          "height": 40,
          "id": "run-hover",
          "width": 120
        },
        {
          "anchor": [
            -0.5,
            0.5
          ],
          "components": {
            "sprite": {
              "texture": {
                "color": "#1c5fdcff"
              }
            }
          },
          "coordinateSpace": "ui",
          "height": 40,
          "id": "run-pressed",
          "width": 120
        },
        {
          "anchor": [
            -0.5,
            0.5
          ],
          "components": {
            "sprite": {
              "texture": {
                "color": "#00000000"
              }
            }
          },
          "coordinateSpace": "ui",
          "height": 16,
          "id": "run-icon",
          "width": 16,
          "x": 12,
          "y": 12
        },
        {
          "anchor": [
            -0.5,
            0.5
          ],
          "components": {
            "text": {
              "color": "#ffffffff",
              "fontSize": 16,
              "value": "Run"
            }
          },
          "coordinateSpace": "ui",
          "id": "run-label",
          "x": 34,
          "y": 12
        }
      ],
      "control": {
        "text": "Run"
      },
      "coordinateSpace": "ui",
      "height": 40,
      "id": "run",
      "width": 120,
      "x": 12,
      "y": 16
    },
    {
      "control": {},
      "anchor": [
        -0.5,
        0.5
      ],
      "coordinateSpace": "ui",
      "height": 40,
      "id": "name",
      "width": 240,
      "x": 12,
      "y": 64
    },
    {
      "control": {},
      "anchor": [
        -0.5,
        0.5
      ],
      "coordinateSpace": "ui",
      "height": 40,
      "id": "amount",
      "width": 120,
      "x": 260,
      "y": 64
    },
    {
      "control": {},
      "anchor": [
        -0.5,
        0.5
      ],
      "coordinateSpace": "ui",
      "height": 100,
      "id": "notes",
      "width": 320,
      "x": 12,
      "y": 112
    },
    {
      "control": {
        "text": "Enabled"
      },
      "coordinateSpace": "ui",
      "height": 32,
      "id": "enabled",
      "width": 160,
      "x": 12,
      "y": 220
    },
    {
      "control": {},
      "coordinateSpace": "ui",
      "height": 40,
      "id": "mode",
      "width": 220,
      "x": 12,
      "y": 260
    },
    {
      "control": {},
      "coordinateSpace": "ui",
      "height": 12,
      "id": "health",
      "width": 220,
      "x": 12,
      "y": 316
    },
    {
      "control": {},
      "children": [
        {
          "coordinateSpace": "ui",
          "id": "listContent"
        }
      ],
      "coordinateSpace": "ui",
      "height": 300,
      "id": "list",
      "width": 240,
      "x": 360,
      "y": 16
    }
  ]
}`)

	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatalf("load bundle: %v", err)
	}
	if !bundle.HasUI() {
		t.Fatal("bundle should report declarative UI components")
	}
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatalf("instantiate ecs: %v", err)
	}

	buttonEntity := scene.MustEntity("run")
	if ecs.Has[engineui.Control](scene.World, buttonEntity) || ecs.Has[sceneapi.InputTarget2D](scene.World, buttonEntity) {
		t.Fatal("static scene unexpectedly installed button behavior")
	}
	if err := BindControl(scene, "name", engineui.TextControl[engineui.Input]{
		Value: "Onix", Placeholder: "Name", MaxRunes: 80,
	}); err != nil {
		t.Fatal(err)
	}
	if err := BindControl(scene, "amount", engineui.TextControl[engineui.Input]{Value: "2.5"}); err != nil {
		t.Fatal(err)
	}
	if err := BindControl(scene, "notes", engineui.TextControl[engineui.Textarea]{Placeholder: "Notes"}); err != nil {
		t.Fatal(err)
	}
	if err := ecs.Insert(scene.World, scene.MustEntity("amount"), engineui.NumberInput{Step: .5}); err != nil {
		t.Fatal(err)
	}
	if err := BindControl(scene, "run", engineui.Button{}); err != nil {
		t.Fatal(err)
	}
	if err := BindControl(scene, "enabled", engineui.Toggle[bool]{Value: true, On: true}); err != nil {
		t.Fatal(err)
	}
	if err := BindControl(scene, "mode", engineui.Selector{Selected: 1, ItemHeight: 36, ListHeight: 72}); err != nil {
		t.Fatal(err)
	}
	if err := BindControl(scene, "health", engineui.Progress{Min: 0, Max: 100, Value: 75}); err != nil {
		t.Fatal(err)
	}
	if err := BindControl(scene, "list", engineui.ScrollArea{ContentWidth: 240, ContentHeight: 600, BarSize: 7}); err != nil {
		t.Fatal(err)
	}
	if err := ecs.Insert(scene.World, scene.MustEntity("listContent"), engineui.ScrollContent{Area: scene.MustEntity("list")}); err != nil {
		t.Fatal(err)
	}
	button, ok := ecs.Get[engineui.Button](scene.World, buttonEntity)
	buttonContent, contentOK := ecs.Get[engineui.Content[string]](scene.World, scene.MustEntity("run"))
	if !ok || !contentOK || buttonContent.Value != "Run" {
		t.Fatalf("unexpected button behavior/content: behavior=%+v content=%+v", button, buttonContent)
	}
	if ecs.Has[sceneapi.UIBackground2D](scene.World, buttonEntity) || ecs.Has[sceneapi.UIImage2D](scene.World, buttonEntity) || ecs.Has[sceneapi.Text2D](scene.World, buttonEntity) {
		t.Fatal("declarative button root still owns implicit visuals")
	}
	hover := scene.MustEntity("run-hover")
	variants, ok := ecs.Get[engineui.Variants[engineui.ControlVariant]](scene.World, buttonEntity)
	if !ok || !hasControlVariant(variants, hover, engineui.VariantHovered) {
		t.Fatalf("unexpected hover variants: %#v", variants)
	}
	labelElement, ok := ecs.Get[engineui.Relation[sceneapi.Text2D]](scene.World, scene.MustEntity("run-label"))
	if !ok || labelElement.Owner != buttonEntity || !ecs.Has[engineui.Part[engineui.LabelPart]](scene.World, scene.MustEntity("run-label")) {
		t.Fatalf("unexpected authored label element: %#v", labelElement)
	}
	if !hasControlVariant(variants, scene.MustEntity("run-label"), engineui.VariantAlways) {
		t.Fatalf("authored label is not bound to button variants: %#v", variants)
	}
	for _, id := range []string{"run", "name", "amount", "notes", "enabled", "mode", "health", "list"} {
		root := scene.MustEntity(id)
		if ecs.Has[sceneapi.UIBackground2D](scene.World, root) ||
			ecs.Has[sceneapi.UIImage2D](scene.World, root) ||
			ecs.Has[sceneapi.UIBorder2D](scene.World, root) ||
			ecs.Has[sceneapi.Sprite2D](scene.World, root) ||
			ecs.Has[sceneapi.Text2D](scene.World, root) {
			t.Fatalf("declarative control %q root still owns render components", id)
		}
	}
	field, ok := ecs.Get[engineui.TextField](scene.World, scene.MustEntity("name"))
	if !ok || field.Value != "Onix" || field.Placeholder != "Name" || field.MaxRunes != 80 {
		t.Fatalf("unexpected input: %+v ok=%v", field, ok)
	}
	number, ok := ecs.Get[engineui.NumberInput](scene.World, scene.MustEntity("amount"))
	if !ok || number.Step != .5 {
		t.Fatalf("unexpected number input: %+v ok=%v", number, ok)
	}
	if _, ok := ecs.Get[engineui.Textarea](scene.World, scene.MustEntity("notes")); !ok {
		t.Fatal("expected textarea component")
	}
	checkbox, ok := ecs.Get[engineui.Toggle[bool]](scene.World, scene.MustEntity("enabled"))
	checkboxContent, contentOK := ecs.Get[engineui.Content[string]](scene.World, scene.MustEntity("enabled"))
	if !ok || !checkbox.Value || !contentOK || checkboxContent.Value != "Enabled" {
		t.Fatalf("unexpected checkbox behavior/content: behavior=%+v content=%+v", checkbox, checkboxContent)
	}
	selector, ok := ecs.Get[engineui.Selector](scene.World, scene.MustEntity("mode"))
	if !ok || selector.Selected != 1 || selector.ListHeight != 72 {
		t.Fatalf("unexpected selector: %+v ok=%v", selector, ok)
	}
	progress, ok := ecs.Get[engineui.Progress](scene.World, scene.MustEntity("health"))
	if !ok || progress.Min != 0 || progress.Max != 100 || progress.Value != 75 || progress.Slider {
		t.Fatalf("unexpected program-controlled progress: %+v ok=%v", progress, ok)
	}
	area, ok := ecs.Get[engineui.ScrollArea](scene.World, scene.MustEntity("list"))
	if !ok || area.ContentHeight != 600 || area.BarSize != 7 {
		t.Fatalf("unexpected scroll area: %+v ok=%v", area, ok)
	}
	content, ok := ecs.Get[engineui.ScrollContent](scene.World, scene.MustEntity("listContent"))
	if !ok || content.Area != scene.MustEntity("list") {
		t.Fatalf("unexpected scroll content: %+v ok=%v", content, ok)
	}
	anchor, _ := ecs.Get[sceneapi.Anchor](scene.World, scene.MustEntity("run"))
	if anchor != sceneapi.AnchorTopLeft {
		t.Fatalf("explicit control anchor = %+v, want top-left", anchor)
	}
	buttonSpec := SceneNodeSpec{
		ID: "run", CoordinateSpace: CoordinateSpaceUI, X: 30, Y: 36, Width: 120, Height: 40,
		ScaleX: 1, ScaleY: 1, Order: 12, Anchor: sceneapi.AnchorTopLeft,
	}
	buttonSpec.Control = &ControlSpec{Text: "Run again"}
	if err := scene.ApplyNode(buttonSpec); err != nil {
		t.Fatalf("apply button spec: %v", err)
	}
	buttonContent, _ = ecs.Get[engineui.Content[string]](scene.World, scene.MustEntity("run"))
	width, height, _ := engineui.ControlBounds(scene.World, scene.MustEntity("run"))
	if buttonContent.Value != "Run again" || width != 120 || height != 40 {
		t.Fatalf("button spec was not applied: content=%+v bounds=(%v,%v)", buttonContent, width, height)
	}
	variants, ok = ecs.Get[engineui.Variants[engineui.ControlVariant]](scene.World, buttonEntity)
	if !ok || !hasControlVariant(variants, hover, engineui.VariantHovered) {
		t.Fatalf("button composition was lost after ApplyNode: %#v", variants)
	}
}

func hasControlVariant(variants engineui.Variants[engineui.ControlVariant], entity ecs.Entity, state engineui.ControlVariant) bool {
	for _, variant := range variants.Items {
		if variant.Entity == entity && variant.State == state && variant.Enabled {
			return true
		}
	}
	return false
}

func TestTextureMipmapIsExplicitAndPartOfCacheKey(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "components": {
        "sprite": {
          "texture": {
            "image": "resources/image.png"
          }
        }
      },
      "id": "plain"
    },
    {
      "components": {
        "sprite": {
          "texture": {
            "image": "resources/image.png",
            "mipmap": true
          }
        }
      },
      "id": "mipmapped"
    }
  ]
}`)
	writeTestBinaryFile(t, dir, "resources/image.png", tinyPNG(t))

	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := ecs.Get[sceneapi.Sprite2D](scene.World, scene.MustEntity("plain"))
	mipmapped, _ := ecs.Get[sceneapi.Sprite2D](scene.World, scene.MustEntity("mipmapped"))
	if plain.Texture.Mipmapped() {
		t.Fatal("texture enabled mipmaps by default")
	}
	if !mipmapped.Texture.Mipmapped() {
		t.Fatal("texture ignored explicit mipmap option")
	}
	if plain.Texture == mipmapped.Texture || bundle.resources.textures.len() != 2 {
		t.Fatal("textures with different mipmap options shared a cache entry")
	}
}

func TestLayerDoesNotSelectUICoordinateSpace(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "components": {
        "sprite": {
          "texture": {
            "color": "#ffffff"
          }
        }
      },
      "height": 10,
      "id": "world-overlay",
      "width": 10
    }
  ]
}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	entity := scene.MustEntity("world-overlay")
	if !ecs.Has[sceneapi.Transform](scene.World, entity) || ecs.Has[sceneapi.UITransform](scene.World, entity) {
		t.Fatal("draw order selected UI coordinate space")
	}
}

func TestLoadInstantiatesSceneFromJSON(t *testing.T) {
	dir := t.TempDir()

	writeTestFile(t, dir, "resources/project.json", `{
	  "window": { "width": 320, "height": 180, "title": "Tree Demo", "transparent": true, "decorations": "none", "border": false, "shadow": false, "resizable": false, "showInTaskbar": false, "alwaysOnTop": true, "vsync": true, "fps": 24, "tps": 60, "renderScale": 0.75, "maxDevicePixelRatio": 1.5, "antialias": false, "drmDevice": "/dev/dri/card1", "drmConnectorId": 43 },
  "viewport": { "width": 640, "height": 360, "mode": "fixed-width" },
  "clearColor": "#112233"
}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "children": [
        {
          "components": {
            "text": {
              "value": "Hero"
            }
          },
          "coordinateSpace": "ui",
          "id": "hero"
        },
        {
          "components": {
            "text": {
              "value": "Hello"
            }
          },
          "coordinateSpace": "ui",
          "id": "title"
        }
      ],
      "coordinateSpace": "ui",
      "id": "root",
      "x": 12,
      "y": 18
    },
    {
      "components": {
        "particle": {
          "resource": "spark"
        }
      },
      "id": "spark"
    }
  ]
}`)
	writeTestFile(t, dir, "resources/resources.json", `{
  "effects": [
    { "id": "spark" }
  ]
}`)

	project, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatalf("load bundle: %v", err)
	}
	scene, err := project.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatalf("instantiate ecs: %v", err)
	}

	clr, ok := project.ClearColor()
	if !ok {
		t.Fatalf("expected clear color from project.json")
	}
	goColor := clr.GoColor()
	if goColor.R != 0x11 || goColor.G != 0x22 || goColor.B != 0x33 || goColor.A != 0xff {
		t.Fatalf("unexpected clear color: %+v", clr)
	}

	root := scene.MustEntity("root")
	rootTransform, ok := ecs.Get[sceneapi.UITransform](scene.World, root)
	if !ok || rootTransform.Translation != sceneapi.Px2(12, 18) {
		t.Fatalf("unexpected root transform: %+v ok=%v", rootTransform, ok)
	}

	hero := scene.MustEntity("hero")
	parent, ok := ecs.Get[sceneapi.Parent](scene.World, hero)
	if !ok || parent.Entity != root {
		t.Fatalf("expected hero parent %d, got %+v ok=%v", root, parent, ok)
	}
	if path := scene.Path("hero"); path != "root/hero" {
		t.Fatalf("unexpected hero path: %q", path)
	}
	if _, ok := ecs.Get[sceneapi.Text2D](scene.World, hero); !ok {
		t.Fatalf("expected hero text game object")
	}

	title := scene.MustEntity("title")
	if _, ok := ecs.Get[sceneapi.Text2D](scene.World, title); !ok {
		t.Fatalf("expected title text entity")
	}
	stackIndex, ok := ecs.Get[sceneapi.UIStackIndex](scene.World, title)
	if !ok || stackIndex != 0 {
		t.Fatalf("expected title UI stack index 0, got %d", stackIndex)
	}

	if len(scene.Roots) != 2 {
		t.Fatalf("expected 2 root entities, got %d", len(scene.Roots))
	}
	if _, ok := ecs.Get[sceneapi.ParticleEmitter2D](scene.World, scene.MustEntity("spark")); !ok {
		t.Fatalf("expected spark particle emitter")
	}

	appConfig := project.AppConfig()
	if appConfig.Window.Width != 320 || appConfig.Window.Height != 180 {
		t.Fatalf("unexpected window size: %+v", appConfig.Window)
	}
	if appConfig.Window.FPS == nil || *appConfig.Window.FPS != 24 {
		t.Fatalf("unexpected fps config: %+v", appConfig.Window.FPS)
	}
	if !appConfig.Window.Transparent || appConfig.Window.Decorations != window.DecorationNone {
		t.Fatalf("unexpected transparent window config: %+v", appConfig.Window)
	}
	if appConfig.Window.Shadow == nil || *appConfig.Window.Shadow {
		t.Fatalf("unexpected window shadow config: %+v", appConfig.Window.Shadow)
	}
	if appConfig.Window.Border == nil || *appConfig.Window.Border {
		t.Fatalf("unexpected window border config: %+v", appConfig.Window.Border)
	}
	if appConfig.Window.Resizable == nil || *appConfig.Window.Resizable {
		t.Fatalf("unexpected resizable config: %+v", appConfig.Window.Resizable)
	}
	if appConfig.Window.ShowInTaskbar == nil || *appConfig.Window.ShowInTaskbar {
		t.Fatalf("unexpected taskbar config: %+v", appConfig.Window.ShowInTaskbar)
	}
	if appConfig.Window.AlwaysOnTop == nil || !*appConfig.Window.AlwaysOnTop {
		t.Fatalf("unexpected always-on-top config: %+v", appConfig.Window.AlwaysOnTop)
	}
	if appConfig.Window.TPS == nil || *appConfig.Window.TPS != 60 {
		t.Fatalf("unexpected tps config: %+v", appConfig.Window.TPS)
	}
	if appConfig.Window.RenderScale != 0.75 || appConfig.Window.MaxDevicePixelRatio != 1.5 {
		t.Fatalf("unexpected web render config: %+v", appConfig.Window)
	}
	if appConfig.Window.Antialias == nil || *appConfig.Window.Antialias {
		t.Fatalf("unexpected antialias config: %+v", appConfig.Window.Antialias)
	}
	if appConfig.Window.DRMDevice != "/dev/dri/card1" || appConfig.Window.DRMConnectorID != 43 {
		t.Fatalf("unexpected DRM config: %+v", appConfig.Window)
	}
	if appConfig.Viewport.Width != 640 || appConfig.Viewport.Height != 360 {
		t.Fatalf("unexpected viewport: %+v", appConfig.Viewport)
	}
	if appConfig.Viewport.Mode != window.ViewportModeFixedWidth {
		t.Fatalf("unexpected viewport mode: %q", appConfig.Viewport.Mode)
	}
}

func TestLoadIgnoresMissingResourcesManifest(t *testing.T) {
	dir := t.TempDir()

	writeTestFile(t, dir, "resources/project.json", `{
  "window": { "width": 320, "height": 180, "title": "No Resources" },
  "clearColor": "#112233"
}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "components": {
        "text": {
          "value": "Hero"
        }
      },
      "id": "hero"
    }
  ]
}`)

	project, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatalf("load bundle without resources.json: %v", err)
	}
	config := project.AppConfig().Window
	if config.Decorations != "" || config.Border != nil || config.Shadow != nil || config.Resizable != nil || config.ShowInTaskbar != nil || config.AlwaysOnTop != nil {
		t.Fatalf("unspecified window flags should preserve defaults: %+v", config)
	}
	scene, err := project.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatalf("instantiate ecs without resources.json: %v", err)
	}
	if _, ok := ecs.Get[sceneapi.Text2D](scene.World, scene.MustEntity("hero")); !ok {
		t.Fatalf("expected hero text to load without resources.json")
	}
	if _, err := project.Resources().ParseColor("missing", 1); err == nil {
		t.Fatalf("expected non-hex color to return an error")
	}
}

func TestResourcesParseFunctionalCSSColor(t *testing.T) {
	clr, err := parseColorValue("rgba(255, 182, 62, 0.5)", .5)
	if err != nil {
		t.Fatal(err)
	}
	if clr.R != 1 || clr.G < .713 || clr.G > .715 || clr.B < .242 || clr.B > .244 || clr.A != .25 {
		t.Fatalf("unexpected color: %+v", clr)
	}
}

func TestGameObjectAtlasLoadsFromRelativeJSONPath(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{ "window": { "width": 64, "height": 64 } }`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "components": {
        "sprite": {
          "texture": {
            "atlas": "resources/atlases/hero.json",
            "frame": "idle"
          }
        }
      },
      "height": 19,
      "id": "hero",
      "width": 37
    },
    {
      "components": {
        "sprite": {
          "texture": {
            "atlas": "resources/atlases/hero.json",
            "frame": "idle"
          }
        }
      },
      "id": "native"
    }
  ]
}`)
	writeTestFile(t, dir, "resources/atlases/hero.json", `{
	  "frames": [{ "filename": "idle", "frame": { "x": 0, "y": 0, "w": 7, "h": 5 } }],
	  "meta": { "image": "hero.png", "size": { "w": 8, "h": 8 } }
	}`)
	writeTestBinaryFile(t, dir, "resources/atlases/hero.png", tinyPNG(t))

	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatalf("load bundle: %v", err)
	}
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatalf("instantiate atlas sprite: %v", err)
	}
	if sprite, ok := ecs.Get[sceneapi.Sprite2D](scene.World, scene.MustEntity("hero")); !ok || sprite.Width != 37 || sprite.Height != 19 {
		t.Fatal("expected explicit atlas sprite size")
	}
	if sprite, ok := ecs.Get[sceneapi.Sprite2D](scene.World, scene.MustEntity("native")); !ok || sprite.Width != 7 || sprite.Height != 5 || sprite.Slice != nil {
		t.Fatal("expected atlas sprite size from metadata")
	}
}

func TestGameObjectAtlasLoadsFromAtlasPathWithoutMetadataSize(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "components": {
        "sprite": {
          "texture": {
            "atlas": "resources/hero.atlas",
            "frame": "idle.png"
          }
        }
      },
      "id": "hero"
    }
  ]
}`)
	writeTestFile(t, dir, "resources/hero.atlas", `{
  "meta":{"image":"hero.png","prefix":"resources/hero/"},
  "frames":{"idle.png":{
    "frame":{"idx":0,"x":0,"y":0,"w":1,"h":1},
    "sourceSize":{"w":1,"h":1},
    "spriteSourceSize":{"x":0,"y":0}
  }}
}`)
	writeTestBinaryFile(t, dir, "resources/hero.png", tinyPNG(t))

	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatalf("load bundle: %v", err)
	}
	defer bundle.Dispose()
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatalf("instantiate .atlas sprite: %v", err)
	}
	sprite, ok := ecs.Get[sceneapi.Sprite2D](scene.World, scene.MustEntity("hero"))
	if !ok || sprite.Width != 1 || sprite.Height != 1 || sprite.Texture == nil || !sprite.Texture.Valid() {
		t.Fatalf("unexpected .atlas sprite: %+v", sprite)
	}
}

func TestLoadRejectsUnknownManifestFields(t *testing.T) {
	dir := t.TempDir()

	writeTestFile(t, dir, "resources/project.json", `{
  "resources": "resources.json"
}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": []
}`)

	if _, err := loadTestProject(os.DirFS(dir), "."); err == nil {
		t.Fatalf("expected unknown manifest fields to be rejected")
	}
}

func TestLoadSupportsManifestFilesInsideResourcesDir(t *testing.T) {
	dir := t.TempDir()

	writeTestFile(t, dir, "resources/project.json", `{
  "window": { "width": 640, "height": 360, "title": "Resources Layout" },
  "clearColor": "#334455"
}`)
	writeTestFile(t, dir, "resources/resources.json", `{
	  "effects": [
	    { "id": "spark" }
	  ]
	}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "components": {
        "sprite": {
          "texture": {
            "image": "resources/hero.png"
          }
        }
      },
      "id": "hero"
    },
    {
      "components": {
        "particle": {
          "resource": "spark"
        }
      },
      "id": "spark"
    },
    {
      "components": {
        "text": {
          "value": "Hello"
        }
      },
      "coordinateSpace": "ui",
      "id": "title"
    }
  ]
}`)
	writeTestBinaryFile(t, dir, "resources/hero.png", tinyPNG(t))

	project, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatalf("load bundle from resources dir manifests: %v", err)
	}
	scene, err := project.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatalf("instantiate ecs from resources dir manifests: %v", err)
	}
	if _, ok := ecs.Get[sceneapi.Sprite2D](scene.World, scene.MustEntity("hero")); !ok {
		t.Fatalf("expected hero sprite from resources dir manifest")
	}
	if _, ok := ecs.Get[sceneapi.Text2D](scene.World, scene.MustEntity("title")); !ok {
		t.Fatalf("expected title text from resources dir manifest")
	}
	if _, ok := ecs.Get[sceneapi.ParticleEmitter2D](scene.World, scene.MustEntity("spark")); !ok {
		t.Fatalf("expected spark particle emitter from resources dir manifest")
	}

	if width, height, ok := project.WindowSize(); !ok || width != 640 || height != 360 {
		t.Fatalf("unexpected window size from resources/project.json: %d %d %v", width, height, ok)
	}
}

func TestLoadDoesNotFallBackToRootManifests(t *testing.T) {
	dir := t.TempDir()
	writeTestBinaryFile(t, dir, "project.json", []byte(`{}`))
	writeTestBinaryFile(t, dir, "scenes.json", []byte(`{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`))
	writeTestBinaryFile(t, dir, "scenes/main.scene.json", []byte(`{"nodes":[]}`))

	if _, err := loadTestProject(os.DirFS(dir), "."); err == nil || !strings.Contains(filepath.ToSlash(err.Error()), "resources/scenes.json") {
		t.Fatalf("root manifest load error = %v", err)
	}
}

func TestLoadReadsWindowIconRelativeToGameManifest(t *testing.T) {
	dir := t.TempDir()

	writeTestFile(t, dir, "resources/project.json", `{
  "window": { "width": 640, "height": 360, "icon": "icons/app.png" }
}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": []
}`)
	writeTestBinaryFile(t, dir, "resources/icons/app.png", tinyPNG(t))

	project, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatalf("load bundle: %v", err)
	}

	want, err := os.ReadFile(filepath.Join(dir, "resources", "icons", "app.png"))
	if err != nil {
		t.Fatal(err)
	}
	if got := project.WindowIcon(); !bytes.Equal(got, want) {
		t.Fatal("unexpected window icon data")
	}
}

func TestLoadDoesNotInferWindowIcon(t *testing.T) {
	dir := t.TempDir()

	writeTestFile(t, dir, "resources/project.json", `{
  "window": { "width": 640, "height": 360 }
}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": []
}`)
	writeTestBinaryFile(t, dir, "logo.png", tinyPNG(t))
	writeTestBinaryFile(t, dir, "resources/logo.png", tinyPNG(t))

	project, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatalf("load bundle: %v", err)
	}

	if got := project.WindowIcon(); len(got) != 0 {
		t.Fatal("window icon was inferred without window.icon")
	}
}

func TestLoadRejectsInvalidWindowIconPath(t *testing.T) {
	for name, icon := range map[string]string{
		"parent":    "../logo.png",
		"absolute":  "/tmp/logo.png",
		"drive":     "C:/game/logo.png",
		"backslash": "icons\\logo.png",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeTestFile(t, dir, "resources/project.json", fmt.Sprintf("{\"window\":{\"icon\":%q}}", icon))
			writeTestFile(t, dir, "resources/resources.json", "{}")
			writeTestFile(t, dir, "resources/scenes/main.scene.json", "{\"nodes\":[]}")
			if _, err := loadTestProject(os.DirFS(dir), "."); err == nil || !strings.Contains(err.Error(), "window.icon") {
				t.Fatalf("invalid icon path error = %v", err)
			}
		})
	}
}

func TestLoadSupportsInlineGameObjectAndParticleTextures(t *testing.T) {
	dir := t.TempDir()

	writeTestFile(t, dir, "resources/project.json", `{
  "clearColor": "#112233"
}`)
	writeTestFile(t, dir, "resources/resources.json", `{
	  "effects": [
	    { "id": "spark", "texture": {"color":"#445566cc"}, "colors": ["#445566cc", "#ffffff"] }
	  ]
	}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "components": {
        "sprite": {
          "texture": {
            "color": "#445566cc"
          }
        }
      },
      "height": 48,
      "id": "hero",
      "width": 120
    },
    {
      "components": {
        "sprite": {
          "texture": {
            "image": "resources/hero.png"
          }
        }
      },
      "id": "badge"
    },
    {
      "components": {
        "particle": {
          "resource": "spark"
        }
      },
      "id": "spark"
    },
    {
      "components": {
        "text": {
          "color": "#ffffff",
          "value": "Hello"
        }
      },
      "coordinateSpace": "ui",
      "id": "title"
    }
  ]
}`)
	writeTestBinaryFile(t, dir, "resources/hero.png", tinyPNG(t))

	project, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatalf("load bundle with inline textures: %v", err)
	}
	scene, err := project.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatalf("instantiate ecs with inline textures: %v", err)
	}

	clr, ok := project.ClearColor()
	if !ok {
		t.Fatalf("expected clear color")
	}
	goColor := clr.GoColor()
	if goColor.R != 0x11 || goColor.G != 0x22 || goColor.B != 0x33 {
		t.Fatalf("unexpected clear color: %+v", clr)
	}

	hero, ok := ecs.Get[sceneapi.Sprite2D](scene.World, scene.MustEntity("hero"))
	if !ok || hero.Texture == nil {
		t.Fatal("expected hero color texture")
	}
	colorTexture := hero.Texture
	badge, ok := ecs.Get[sceneapi.Sprite2D](scene.World, scene.MustEntity("badge"))
	if !ok || badge.Texture == nil {
		t.Fatal("expected badge texture")
	}
	ownedTexture := badge.Texture
	if err := project.Dispose(); err != nil {
		t.Fatalf("dispose project resources: %v", err)
	}
	if !colorTexture.Valid() {
		t.Fatal("disposing project resources should not invalidate shared color textures")
	}
	if ownedTexture.Valid() {
		t.Fatal("disposing project resources retained an owned image texture")
	}
	if _, ok := ecs.Get[sceneapi.Sprite2D](scene.World, scene.MustEntity("hero")); !ok {
		t.Fatalf("expected hero sprite")
	}
	if _, ok := ecs.Get[sceneapi.Sprite2D](scene.World, scene.MustEntity("badge")); !ok {
		t.Fatalf("expected badge sprite")
	}
	if _, ok := ecs.Get[sceneapi.Text2D](scene.World, scene.MustEntity("title")); !ok {
		t.Fatalf("expected title text")
	}
	if _, ok := ecs.Get[sceneapi.ParticleEmitter2D](scene.World, scene.MustEntity("spark")); !ok {
		t.Fatalf("expected spark particle emitter")
	}
}

func TestParticleTexturesSupportImageAtlasAndColor(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/resources.json", `{
	  "effects": [
	    {"id":"image","texture":{"image":"resources/particle.png"}},
	    {"id":"atlas","texture":{"atlas":"resources/particles.json","frame":"spark"}},
	    {"id":"color","texture":{"color":"#ffffff"}}
	  ]
	}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "components": {
        "particle": {
          "resource": "image"
        }
      },
      "id": "image"
    },
    {
      "components": {
        "particle": {
          "resource": "atlas"
        }
      },
      "id": "atlas"
    },
    {
      "components": {
        "particle": {
          "resource": "color"
        }
      },
      "id": "color"
    }
  ]
}`)
	writeTestBinaryFile(t, dir, "resources/particle.png", tinyPNG(t))
	writeTestFile(t, dir, "resources/particles.json", `{
	  "frames":[{"filename":"spark","frame":{"x":0,"y":0,"w":1,"h":1}}],
	  "meta":{"image":"particle.png","size":{"w":1,"h":1}}
	}`)

	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"image", "atlas", "color"} {
		emitter, ok := ecs.Get[sceneapi.ParticleEmitter2D](scene.World, scene.MustEntity(id))
		if !ok || emitter.Texture == nil || !emitter.Texture.Valid() {
			t.Fatalf("particle %q did not load its texture: %+v", id, emitter)
		}
	}
}

func TestLoadRejectsTexturesManifestField(t *testing.T) {
	dir := t.TempDir()

	writeTestFile(t, dir, "resources/project.json", `{
  "clearColor": "#ffffff"
}`)
	writeTestFile(t, dir, "resources/resources.json", `{
  "textures": [
    { "id": "heroTexture", "image": "resources/hero.png" }
  ]
}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": []
}`)

	if _, err := loadTestProject(os.DirFS(dir), "."); err == nil || !strings.Contains(err.Error(), "unknown field \"textures\"") {
		t.Fatalf("expected textures manifest field to be rejected, got %v", err)
	}
}

func TestLoadRejectsMultipleSpriteTextureKinds(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{ "window": { "width": 64, "height": 64 } }`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "components": {
        "sprite": {
          "texture": {
            "color": "#ffffff",
            "image": "resources/hero.png"
          }
        }
      },
      "id": "hero"
    }
  ]
}`)
	if _, err := loadTestProject(os.DirFS(dir), "."); err == nil || !strings.Contains(err.Error(), "exactly one of image, atlas, or color") {
		t.Fatalf("expected mutually exclusive texture error, got %v", err)
	}
}

func TestLoadRejectsInvalidParticleTexture(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/resources.json", `{
	  "effects":[{"id":"spark","texture":{"image":"resources/spark.png","color":"#ffffff"}}]
	}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": []
}`)
	if _, err := loadTestProject(os.DirFS(dir), "."); err == nil || !strings.Contains(err.Error(), "exactly one of image, atlas, or color") {
		t.Fatalf("expected mutually exclusive particle texture error, got %v", err)
	}
}

func TestLoadRejectsMixedWorldAndUICoordinateHierarchy(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{ "window": { "width": 64, "height": 64 } }`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "children": [
        {
          "components": {
            "text": {
              "value": "mixed"
            }
          },
          "coordinateSpace": "ui",
          "id": "label"
        }
      ],
      "id": "world"
    }
  ]
}`)

	if _, err := loadTestProject(os.DirFS(dir), "."); err == nil || !strings.Contains(err.Error(), "cannot use UI coordinates under world parent") {
		t.Fatalf("expected mixed coordinate hierarchy error, got %v", err)
	}
}

func TestLoadRejectsParticleGroups(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/resources.json", `{"effects":[{"id":"spark"}],"groups":{"default":["spark"]}}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": []
}`)
	if _, err := loadTestProject(os.DirFS(dir), "."); err == nil || !strings.Contains(err.Error(), "unknown field \"groups\"") {
		t.Fatalf("expected particle groups to be rejected, got %v", err)
	}
}

func TestAudioResourcesDecodeLazily(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": []
}`)
	writeTestFile(t, dir, "resources/resources.json", `{
  "audio": [{ "id": "broken", "path": "broken.wav" }]
}`)
	writeTestFile(t, dir, "broken.wav", "not a wav")

	bundle, err := LoadConfiguredWith[testAudioFeature](os.DirFS(dir), ".", testProjectEngineConfig())
	if err != nil {
		t.Fatalf("load bundle: %v", err)
	}
	cache := bundle.resources.named[testAudioResourceKind].(*namedResourceTable[string]).cache
	if cache.ready("broken") {
		t.Fatal("audio resource loaded before first access")
	}
	_, firstErr := LoadNamedResource[string](bundle.Resources(), testAudioResourceKind, "broken")
	if firstErr == nil {
		t.Fatal("loading broken audio succeeded")
	}
	if !cache.ready("broken") {
		t.Fatal("audio decode result was not cached")
	}
	if _, err := LoadNamedResource[string](bundle.Resources(), testAudioResourceKind, "broken"); err == nil || err.Error() != firstErr.Error() {
		t.Fatalf("cached broken audio result changed: first=%v second=%v", firstErr, err)
	}
}

const testAudioResourceKind = "test-audio"

type testAudioFeature struct{}

func (testAudioFeature) ConfigureResources(context ResourceLoadContext, entries ResourceEntries) error {
	for _, entry := range entries.Audio {
		entry := entry
		if err := RegisterNamedResource(context, testAudioResourceKind, entry.ID, ResourceRegistration[string]{
			Load: func() (string, error) {
				data, err := context.ReadFile(entry.Path)
				if err != nil {
					return "", err
				}
				if string(data) == "not a wav" {
					return "", errors.New("invalid audio")
				}
				return string(data), nil
			},
		}); err != nil {
			return err
		}
	}
	return nil
}

func TestFileResourcesReadOnFirstAccess(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": []
}`)
	writeTestFile(t, dir, "resources/resources.json", `{
  "fonts": [{"id":"body","path":"missing.ttf"}],
  "videos": [{"id":"intro","path":"missing.mp4"}],
  "spines": [{"id":"hero","skeleton":"missing.json","atlas":"missing.atlas","texture":"missing.png"}]
}`)

	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatalf("load bundle with unused file resources: %v", err)
	}
	defer bundle.Dispose()

	tests := []struct {
		name  string
		ready func() bool
		load  func() error
	}{
		{name: "font", ready: func() bool { return bundle.resources.fonts.ready("body") }, load: func() error { _, err := bundle.Resources().Font("body"); return err }},
		{name: "video", ready: func() bool { return bundle.resources.videos.ready("intro") }, load: func() error { _, err := bundle.Resources().Video("intro"); return err }},
		{name: "spine", ready: func() bool { return bundle.resources.spines.ready("hero") }, load: func() error { _, err := bundle.Resources().Spine("hero"); return err }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.ready() {
				t.Fatal("resource loaded while opening the bundle")
			}
			if err := test.load(); err == nil {
				t.Fatal("loading missing resource succeeded")
			}
			if !test.ready() {
				t.Fatal("resource result was not cached after first access")
			}
		})
	}
}

func TestShaderResourcesLoadLazilyAndCanBeReferenced(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{
  "window": {"width": 320, "height": 180, "title": "Shader"},
  "viewport": {"width": 320, "height": 180}
}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": []
}`)
	writeTestFile(t, dir, "resources/resources.json", `{
  "shaders": [{
    "id": "effect",
    "metal": "shaders/effect.metal",
    "webGL2Fragment": "shaders/effect.frag.glsl"
  }]
}`)

	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	defer bundle.Dispose()
	if bundle.resources.shaders.ready("effect") {
		t.Fatal("shader source loaded eagerly")
	}
	shaders := filepath.Join(dir, "shaders")
	if err := os.MkdirAll(shaders, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shaders, "effect.metal"), []byte("metal source"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shaders, "effect.frag.glsl"), []byte("glsl source"), 0o644); err != nil {
		t.Fatal(err)
	}
	descriptor, err := bundle.Resources().Shader("effect")
	if err != nil {
		t.Fatalf("load shader resource: %v", err)
	}
	if descriptor.MetalSource != "metal source" || descriptor.WebGL2FragmentSource != "glsl source" {
		t.Fatalf("unexpected shader descriptor: %+v", descriptor)
	}
	shader, err := bundle.Resources().Material2DShader("effect")
	if err != nil {
		t.Fatalf("create Material2D shader resource: %v", err)
	}
	again, err := bundle.Resources().Material2DShader("effect")
	if err != nil || shader != again {
		t.Fatalf("Material2D shader resource was not cached: first=%p second=%p err=%v", shader, again, err)
	}
}

func TestLoadRejectsUnsupportedAudioFormat(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": []
}`)
	writeTestFile(t, dir, "resources/resources.json", `{
  "audio": [{ "id": "music", "path": "music.aac" }]
}`)

	if _, err := loadTestProject(os.DirFS(dir), "."); err == nil || !strings.Contains(err.Error(), "unsupported format") {
		t.Fatalf("Load unsupported audio error = %v", err)
	}
}

func TestLoadInstantiatesAnimatedSpriteFromInlineClip(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "components": {
        "animation": {
          "clip": {
            "duration": 0.25,
            "sampleRate": 8,
            "loop": true,
            "textureTracks": [{
              "binding": {"target": "sprite.texture"},
              "keyframes": [
                {"time":0,"value":{"image":"frame0.png"},"tween":"step"},
                {"time":0.125,"value":{"image":"frame1.png"},"tween":"step"}
              ]
            }]
          }
        }
      },
      "id": "hero"
    }
  ]
}`)
	writeTestBinaryFile(t, dir, "frame0.png", tinyPNG(t))
	writeTestBinaryFile(t, dir, "frame1.png", tinyPNG(t))

	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	entity := scene.MustEntity("hero")
	animator, ok := ecs.Get[sceneapi.Animator](scene.World, entity)
	if !ok || animator.Clip == nil || len(animator.Clip.TextureTracks) != 1 || !animator.Loop || !animator.Playing {
		t.Fatalf("animator component = %+v", animator)
	}
	sprite, ok := ecs.Get[sceneapi.Sprite2D](scene.World, entity)
	if !ok || sprite.Texture != animator.CurrentTexture() {
		t.Fatalf("sprite did not start on animation keyframe 0: %+v", sprite)
	}
}

func TestLoadBindsAnimationTracksToRealChildNodes(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "components": {
        "animation": {
          "clip": {
            "duration": 1, "sampleRate": 10, "loop": true,
            "floatTracks": [{
              "binding":{"path":"blade","target":"transform.position.x"},
              "keyframes":[{"time":0,"value":10},{"time":0.5,"value":30}]
            }]
          }
        }
      },
      "height": 20,
      "id": "hero",
      "width": 40,
      "children": [{
        "id":"blade", "order":1, "width":20, "height":10,
        "scaleX":1, "scaleY":1, "scaleZ":1, "alpha":1, "visible":true,
        "components":{"sprite":{"texture":{"color":"#ffffff"}}}
      }]
    }
  ]
}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	entity := scene.MustEntity("hero")
	if !ecs.Has[sceneapi.Animator](scene.World, entity) || ecs.Has[sceneapi.Sprite2D](scene.World, entity) {
		t.Fatal("child-bound animation should attach only an animator to its owner")
	}
	blade := scene.MustEntity("blade")
	transform, ok := ecs.Get[sceneapi.Transform](scene.World, blade)
	if !ok || transform.Translation.X() != 10 || !ecs.Has[sceneapi.Sprite2D](scene.World, blade) {
		t.Fatalf("animated child = transform:%+v sprite:%v", transform, ecs.Has[sceneapi.Sprite2D](scene.World, blade))
	}
	if items := scene.RenderItems(); len(items) != 1 || items[0].Entity != blade || items[0].SortIndex != 1 {
		t.Fatalf("child render items = %+v", items)
	}
}

func TestLoadAllowsSpriteAndAnimationComponentsTogether(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "components": {
        "sprite": {
          "texture": {
            "color": "#fff"
          }
        },
		"animation": {
		  "clip": {
		    "duration": 1,
		    "floatTracks": [{"binding":{"target":"transform.position.x"},"keyframes":[{"time":0,"value":0}]}]
		  }
		}
      },
      "id": "hero"
    }
  ]
}`)
	if _, err := loadTestProject(os.DirFS(dir), "."); err != nil {
		t.Fatalf("sprite and animation composition was rejected: %v", err)
	}
}

func TestLoadRejectsInvalidAnimation(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/resources.json", `{
  "animations": [{"id":"idle","duration":1,"sampleRate":12}]
}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": []
}`)
	if _, err := loadTestProject(os.DirFS(dir), "."); err == nil || !strings.Contains(err.Error(), "unknown field \"animations\"") {
		t.Fatalf("expected invalid animation error, got %v", err)
	}
}

func TestNormalizeSceneCatalogRejectsInvalidRegistrations(t *testing.T) {
	tests := []struct {
		name    string
		catalog sceneCatalog
		want    string
	}{
		{name: "missing entry", catalog: sceneCatalog{Scenes: []SceneCatalogEntry{{ID: "main", Path: "scenes/main.scene.json"}}}, want: "entry scene is required"},
		{name: "unknown entry", catalog: sceneCatalog{Entry: "missing", Scenes: []SceneCatalogEntry{{ID: "main", Path: "scenes/main.scene.json"}}}, want: "is not registered"},
		{name: "duplicate id", catalog: sceneCatalog{Entry: "main", Scenes: []SceneCatalogEntry{{ID: "main", Path: "scenes/main.scene.json"}, {ID: "main", Path: "scenes/other.scene.json"}}}, want: "duplicate scene id"},
		{name: "duplicate path", catalog: sceneCatalog{Entry: "main", Scenes: []SceneCatalogEntry{{ID: "main", Path: "scenes/main.scene.json"}, {ID: "other", Path: "scenes/main.scene.json"}}}, want: "duplicate scene path"},
		{name: "escaping path", catalog: sceneCatalog{Entry: "main", Scenes: []SceneCatalogEntry{{ID: "main", Path: "../main.scene.json"}}}, want: "relative *.scene.json"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := normalizeSceneCatalog(&test.catalog); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("catalog error = %v, want %q", err, test.want)
			}
		})
	}
}

func writeTestFile(t *testing.T, dir string, name string, contents string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	registerTestScene(t, dir, filepath.ToSlash(name))
}

func registerTestScene(t *testing.T, dir, name string) {
	t.Helper()
	marker := "scenes/"
	index := strings.Index(name, marker)
	if index < 0 || !strings.HasSuffix(name, ".scene.json") {
		return
	}
	prefix, scenePath := name[:index], name[index:]
	catalogPath := filepath.Join(dir, filepath.FromSlash(prefix+"scenes.json"))
	catalog := sceneCatalog{Entry: "main"}
	if data, err := os.ReadFile(catalogPath); err == nil {
		if err := json.Unmarshal(data, &catalog); err != nil {
			t.Fatalf("decode test scene catalog: %v", err)
		}
	} else if !os.IsNotExist(err) {
		t.Fatalf("read test scene catalog: %v", err)
	}
	id := SceneID(strings.TrimSuffix(filepath.Base(scenePath), ".scene.json"))
	if !slices.ContainsFunc(catalog.Scenes, func(entry SceneCatalogEntry) bool { return entry.ID == id }) {
		catalog.Scenes = append(catalog.Scenes, SceneCatalogEntry{ID: id, Path: scenePath})
	}
	data, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		t.Fatalf("encode test scene catalog: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(catalogPath), 0o755); err != nil {
		t.Fatalf("mkdir test scene catalog: %v", err)
	}
	if err := os.WriteFile(catalogPath, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write test scene catalog: %v", err)
	}
}

func writeTestBinaryFile(t *testing.T, dir string, name string, contents []byte) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	im := image.NewRGBA(image.Rect(0, 0, 1, 1))
	im.Pix = []byte{0xff, 0x80, 0x40, 0xff}
	if err := stdpng.Encode(&buf, im); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func sizedPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var buf bytes.Buffer
	im := image.NewRGBA(image.Rect(0, 0, width, height))
	if err := stdpng.Encode(&buf, im); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestNineSliceSpriteLoadsInlineImageSlice(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "components": {
        "sprite": {
          "texture": {
            "image": "resources/window.png",
            "slice": {
              "bottom": 3,
              "left": 6,
              "right": 5,
              "top": 4
            }
          }
        }
      },
      "height": 90,
      "id": "window",
      "width": 160
    }
  ]
}`)
	writeTestBinaryFile(t, dir, "resources/window.png", sizedPNG(t, 30, 24))

	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	sprite, ok := ecs.Get[sceneapi.Sprite2D](scene.World, scene.MustEntity("window"))
	if !ok || sprite.Slice == nil {
		t.Fatal("expected nine-slice sprite")
	}
	if *sprite.Slice != (sceneapi.NineSlice2D{Left: 6, Right: 5, Top: 4, Bottom: 3}) {
		t.Fatalf("unexpected slice: %+v", sprite.Slice)
	}
}

func TestNineSliceAtlasFrameUsesInlineSlice(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "components": {
        "sprite": {
          "texture": {
            "atlas": "resources/sheet.json",
            "frame": "window",
            "slice": {
              "bottom": 3,
              "left": 6,
              "right": 5,
              "top": 4
            }
          }
        }
      },
      "height": 90,
      "id": "window",
      "width": 160
    }
  ]
}`)
	writeTestFile(t, dir, "resources/sheet.json", `{"frames":[{"filename":"window","frame":{"x":0,"y":0,"w":30,"h":24}}],"meta":{"image":"ui.png","size":{"w":30,"h":24}}}`)
	writeTestBinaryFile(t, dir, "resources/ui.png", sizedPNG(t, 30, 24))

	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	sprite, ok := ecs.Get[sceneapi.Sprite2D](scene.World, scene.MustEntity("window"))
	if !ok || sprite.Slice == nil || *sprite.Slice != (sceneapi.NineSlice2D{Left: 6, Right: 5, Top: 4, Bottom: 3}) {
		t.Fatalf("unexpected atlas slice: %+v", sprite.Slice)
	}
}

func TestNineSliceAtlasFrameValidatesAgainstLogicalSize(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "components": {
        "sprite": {
          "texture": {
            "atlas": "resources/sheet.json",
            "frame": "window",
            "slice": {
              "bottom": 9,
              "left": 16,
              "right": 15,
              "top": 10
            }
          }
        }
      },
      "height": 90,
      "id": "window",
      "width": 160
    }
  ]
}`)
	writeTestFile(t, dir, "resources/sheet.json", `{"frames":{"window":{"frame":{"x":0,"y":0,"w":30,"h":24},"sourceSize":{"w":40,"h":32},"spriteSourceSize":{"x":5,"y":4,"w":30,"h":24}}},"meta":{"image":"ui.png","size":{"w":30,"h":24}}}`)
	writeTestBinaryFile(t, dir, "resources/ui.png", sizedPNG(t, 30, 24))

	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	sprite, ok := ecs.Get[sceneapi.Sprite2D](scene.World, scene.MustEntity("window"))
	if !ok || sprite.Texture == nil || sprite.Texture.LogicalWidth() != 40 || sprite.Texture.LogicalHeight() != 32 {
		t.Fatalf("unexpected trimmed nine-slice sprite: %#v", sprite)
	}
}
