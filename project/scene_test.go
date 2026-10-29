package project

import (
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	sceneapi "github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/scene/render3d"
	onixtext "github.com/diiyw/onix/text"
	"github.com/go-gl/mathgl/mgl32"
)

func TestInstantiateSceneSpawnsSpriteTrailFromJSON(t *testing.T) {
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
          },
          "trail": {
            "alphaEnd": 0.1,
            "alphaStart": 0.7,
            "interval": 0.03,
            "lifetime": 0.8,
            "maxSamples": 18,
            "minDistance": 6
          }
        }
      },
      "height": 24,
      "id": "hero",
      "width": 32
    }
  ]
}`)

	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatalf("instantiate ecs: %v", err)
	}
	trail, ok := scene.World.Get[sceneapi.SpriteTrail2D](scene.MustEntity("hero"))
	if !ok {
		t.Fatal("sprite trail component was not instantiated")
	}
	if trail.Lifetime != 0.8 || trail.Interval != 0.03 || trail.MinDistance != 6 || trail.AlphaStart != 0.7 || trail.AlphaEnd != 0.1 || trail.MaxSamples != 18 {
		t.Fatalf("unexpected sprite trail: %+v", trail)
	}
}

func TestCatalogScenesAreInstantiatedOnDemand(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "id": "world"
    }
  ]
}`)
	writeTestFile(t, dir, "resources/scenes/ui/menu.scene.json", `{
  "nodes": [
    {
      "children": [
        {
          "coordinateSpace": "ui",
          "height": 40,
          "id": "play",
          "width": 120
        }
      ],
      "coordinateSpace": "ui",
      "id": "menu"
    }
  ]
}`)
	writeTestFile(t, dir, "resources/scenes/ui/dialog.scene.json", `{
  "nodes": [
    {
      "components": {
        "sprite": {
          "texture": {
            "color": "#ffffff"
          }
        }
      },
      "coordinateSpace": "ui",
      "height": 100,
      "id": "dialog",
      "width": 200
    }
  ]
}`)

	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	if got, want := bundle.SceneIDs(), []SceneID{"main", "menu", "dialog"}; !slices.Equal(got, want) {
		t.Fatalf("unexpected scene ids: got %v want %v", got, want)
	}
	world := ecs.NewWorld()
	main, err := bundle.InstantiateEntryScene(world)
	if err != nil {
		t.Fatalf("instantiate main scene: %v", err)
	}
	if _, err := main.Entity("menu"); err == nil {
		t.Fatal("referenced menu should remain dormant")
	}
	if world.EntityCount() != 1 {
		t.Fatalf("expected only the main scene entity, got %d", world.EntityCount())
	}

	page, err := bundle.InstantiateScene(world, "menu")
	if err != nil {
		t.Fatalf("instantiate menu: %v", err)
	}
	if page.SceneID() != "menu" || page.MustEntity("play") == 0 {
		t.Fatalf("unexpected instantiated page: scene=%q entities=%v", page.SceneID(), page.Entities)
	}
	menu := page.MustEntity("menu")
	if err := page.Switch("dialog"); err != nil {
		t.Fatalf("switch page: %v", err)
	}
	if world.IsAlive(menu) {
		t.Fatal("switch should despawn the previous page")
	}
	if page.SceneID() != "dialog" || page.MustEntity("dialog") == 0 {
		t.Fatalf("unexpected switched page: scene=%q entities=%v", page.SceneID(), page.Entities)
	}
	page.Dispose()
	if world.EntityCount() != 1 {
		t.Fatalf("disposing the page should leave the main scene, got %d entities", world.EntityCount())
	}
}

func TestSceneLifecycleRefreshesOnlyAtCommitPoints(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes.json", `{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"},{"id":"other","path":"scenes/other.scene.json"}]}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[{"id":"main","coordinateSpace":"ui"}]}`)
	writeTestFile(t, dir, "resources/scenes/other.scene.json", `{"nodes":[{"id":"other","coordinateSpace":"ui"}]}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()

	refreshes := 0
	bundle.uiSync.beforeSync = func(*ecs.World) error {
		refreshes++
		return nil
	}
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	refreshes = 0
	if err := scene.Switch("other"); err != nil {
		t.Fatal(err)
	}
	if refreshes != 1 {
		t.Fatalf("switch refreshes = %d, want 1", refreshes)
	}

	refreshes = 0
	if err := scene.ReplaceNodes([]SceneNodeSpec{{ID: "replacement", CoordinateSpace: CoordinateSpaceUI}}); err != nil {
		t.Fatal(err)
	}
	if refreshes != 1 {
		t.Fatalf("replace refreshes = %d, want 1", refreshes)
	}

	refreshes = 0
	scene.Dispose()
	if refreshes != 1 {
		t.Fatalf("public dispose refreshes = %d, want 1", refreshes)
	}

	refreshes = 0
	if err := bundle.PreloadScene("other"); err != nil {
		t.Fatal(err)
	}
	if refreshes != 1 {
		t.Fatalf("preload refreshes = %d, want 1", refreshes)
	}
}

func TestPreloadSceneReusesInlineAnimationsAndCachesGlyphs(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	fontData, err := os.ReadFile(filepath.Join("..", "testdata", "fonts", "Noto_Sans_SC", "static", "NotoSansSC-Regular.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	writeTestBinaryFile(t, dir, "font.otf", fontData)
	writeTestFile(t, dir, "resources/resources.json", `{
	"fonts": [{"id":"ui","path":"font.otf"}]
}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": []
}`)
	writeTestFile(t, dir, "resources/scenes/pages/menu.scene.json", `{
  "nodes": [
    {
      "components": {
        "animation": {
		  "clip": {
		    "duration": 1, "sampleRate": 12,
		    "textureTracks": [{
		      "binding": {"target": "sprite.texture"},
		      "keyframes": [{"time": 0, "value": {"image": "resources/pulse.png"}}]
		    }]
		  }
        }
      },
      "id": "pulse"
    },
    {
      "components": {
        "text": {
          "font": "ui",
          "fontSize": 37,
          "value": "Preloaded"
        }
      },
      "coordinateSpace": "ui",
      "id": "title"
    }
  ]
}`)
	writeTestBinaryFile(t, dir, "resources/pulse.png", tinyPNG(t))

	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	defer bundle.Dispose()
	if bundle.inlineAnimationClips.ready("scene/menu/pulse") || bundle.resources.textures.len() != 0 {
		t.Fatal("inline animation textures were loaded while opening the bundle")
	}
	projectFont, err := bundle.Resources().Font("ui")
	if err != nil {
		t.Fatal(err)
	}
	cache := projectFont.GlyphCache(37, 72)
	beforeAtlases := cache.AtlasCount()

	if err := bundle.PreloadScene("menu"); err != nil {
		t.Fatalf("preload scene: %v", err)
	}
	clip, loaded := bundle.inlineAnimationClips.loaded("scene/menu/pulse")
	if !loaded || clip == nil || bundle.resources.textures.len() != 1 {
		t.Fatal("inline animation clip was not prepared during scene preload")
	}
	if cache.AtlasCount() <= beforeAtlases {
		t.Fatal("preload did not rasterize the scene's static text")
	}
	scene, err := bundle.InstantiateScene(ecs.NewWorld(), "menu")
	if err != nil {
		t.Fatalf("instantiate preloaded scene: %v", err)
	}
	animator, ok := scene.World.Get[sceneapi.Animator](scene.MustEntity("pulse"))
	if !ok || animator.Clip != clip {
		t.Fatal("scene did not use the precompiled animation clip")
	}
}

func TestSceneInstanceResolvesNamedLogicReferences(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "children": [
        {
          "components": {
            "sprite": {
              "texture": {
                "color": "#ffffffff"
              }
            }
          },
          "coordinateSpace": "ui",
          "height": 40,
          "id": "normal",
          "width": 120
        },
        {
          "components": {
            "sprite": {
              "texture": {
                "color": "#aaaaaaff"
              }
            }
          },
          "coordinateSpace": "ui",
          "height": 40,
          "id": "hover",
          "width": 120
        }
      ],
      "coordinateSpace": "ui",
      "id": "Start"
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
	root := scene.MustEntity("Start")
	if got := scene.MustChild(root, "hover"); got != scene.MustEntityAt("Start/hover") {
		t.Fatalf("child and path references differ: child=%d path=%d", got, scene.MustEntityAt("Start/hover"))
	}
}

func TestInstantiateSceneSpawnsHierarchyAndRenderComponentsFromJSON(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{
  "window": { "width": 320, "height": 180, "title": "ECS Demo" },
  "clearColor": "#112233"
}`)
	writeTestFile(t, dir, "resources/resources.json", `{
	  "effects": [
    { "id": "spark", "count": 5, "rate": 2, "loop": true, "colors": ["#ff8844"] }
  ]
}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "children": [
        {
          "components": {
            "sprite": {
              "texture": {
                "color": "#ff8844"
              }
            }
          },
          "coordinateSpace": "ui",
          "height": 24,
          "id": "button",
          "width": 80
        },
        {
          "components": {
            "text": {
              "color": "#ffffff",
              "maxWidth": 42,
              "outlineColor": "#123456",
              "outlineWidth": 2.5,
              "letterSpacing": 3,
              "lineHeight": 28,
              "textAlignment": "right",
              "wrapMode": "newlines",
              "wrapWidth": 96,
              "value": "Play"
            }
          },
          "coordinateSpace": "ui",
          "id": "label"
        }
      ],
      "coordinateSpace": "ui",
      "height": 50,
      "id": "root",
      "width": 100,
      "x": 12,
      "y": 18
    },
    {
      "components": {
        "particle": {
          "resource": "spark"
        }
      },
      "id": "spark",
      "x": 20,
      "y": 30
    }
  ]
}`)
	project, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatalf("load project: %v", err)
	}

	world := ecs.NewWorld()
	scene, err := project.InstantiateEntryScene(world)
	if err != nil {
		t.Fatalf("instantiate ecs: %v", err)
	}

	if len(scene.Roots) != 2 {
		t.Fatalf("expected 2 root entities, got %d", len(scene.Roots))
	}
	root, ok := scene.Entities["root"]
	if !ok {
		t.Fatalf("expected root to be indexed")
	}
	button := scene.Entities["button"]
	parent, ok := world.Get[sceneapi.Parent](button)
	if !ok || parent.Entity != root {
		t.Fatalf("expected button parent %d, got %+v ok=%v", root, parent, ok)
	}
	transform, ok := world.Get[sceneapi.UITransform](root)
	if !ok || transform.Translation != sceneapi.Px2(12, 18) {
		t.Fatalf("unexpected root transform: %+v ok=%v", transform, ok)
	}
	if stackIndex, ok := world.Get[sceneapi.UIStackIndex](root); !ok || stackIndex != 0 {
		t.Fatalf("unexpected root UI stack index: %d ok=%v", stackIndex, ok)
	}
	x0, y0, x1, y1, ok := scene.BoundsByID("root")
	if !ok || x0 != -38 || y0 != -7 || x1 != 62 || y1 != 43 {
		t.Fatalf("unexpected authored node bounds: (%v, %v)-(%v, %v) ok=%v", x0, y0, x1, y1, ok)
	}
	sprite, ok := world.Get[sceneapi.Sprite2D](button)
	if !ok || sprite.Width != 80 || sprite.Height != 24 {
		t.Fatalf("unexpected button sprite: %+v ok=%v", sprite, ok)
	}
	label := scene.Entities["label"]
	text, ok := world.Get[sceneapi.Text2D](label)
	if !ok || text.Value != "Play" || text.MaxWidth != 42 || text.WrapMode != sceneapi.TextWrapNewlines || text.WrapWidth != 96 || text.LetterSpacing != 3 || text.LineHeight != 28 || text.TextAlignment != sceneapi.TextAlignRight || text.OutlineWidth != 2.5 || text.OutlineColor.GoColor() != (color.RGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff}) {
		t.Fatalf("unexpected label text: %+v ok=%v", text, ok)
	}
	settings, ok := world.GetResource[sceneapi.RenderSettings2D]()
	if !ok || settings.ClearColor.GoColor().R != 0x11 {
		t.Fatalf("unexpected render settings: %+v ok=%v", settings, ok)
	}
	particle := scene.MustEntity("spark")
	emitter, ok := world.Get[sceneapi.ParticleEmitter2D](particle)
	if !ok || emitter.Count != 5 {
		t.Fatalf("unexpected particle emitter: %+v ok=%v", emitter, ok)
	}
	if order, ok := world.Get[sceneapi.RenderOrder2D](particle); !ok || order != sceneapi.DefaultParticleRenderOrder {
		t.Fatalf("unexpected particle render order: %d ok=%v", order, ok)
	}
}

func TestSceneInstancePreviewQueriesAndApplyNode(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{
  "window": { "width": 320, "height": 180, "title": "Preview Demo" }
}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "children": [
        {
          "components": {
            "sprite": {
              "texture": {
                "color": "#44cc88"
              }
            }
          },
          "height": 10,
          "id": "hero",
          "width": 20,
          "x": 5,
          "y": 7
        }
      ],
      "id": "root",
      "x": 10,
      "y": 20
    }
  ]
}`)
	project, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatalf("load project: %v", err)
	}

	scene, err := project.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatalf("instantiate ecs: %v", err)
	}

	if got := scene.Path("hero"); got != "root/hero" {
		t.Fatalf("unexpected path: got %q want %q", got, "root/hero")
	}
	items := scene.RenderItems()
	if len(items) != 1 || scene.ID(items[0].Entity) != "hero" {
		t.Fatalf("unexpected render items: %+v", items)
	}
	x0, y0, x1, y1, ok := scene.BoundsByID("hero")
	if !ok || x0 != 5 || y0 != 22 || x1 != 25 || y1 != 32 {
		t.Fatalf("unexpected bounds: (%v, %v)-(%v, %v) ok=%v", x0, y0, x1, y1, ok)
	}
	id, ok := scene.Pick(mgl32.Vec2{16, 28}, mgl32.Vec2{16, 28})
	if !ok || id != "hero" {
		t.Fatalf("unexpected pick result: id=%q ok=%v", id, ok)
	}

	spriteSpec := SceneNodeSpec{
		ID:         "hero",
		Components: Components(SceneComponentSprite),
		X:          25,
		Y:          9,
		Width:      30,
		Height:     12,
		ScaleX:     1,
		ScaleY:     1,
		Alpha:      1,
	}
	spriteSpec.Components.Sprite.Texture = &TextureSpec{Color: "#44cc88"}
	err = scene.ApplyNode(spriteSpec)
	if err != nil {
		t.Fatalf("apply spec: %v", err)
	}
	x0, y0, x1, y1, ok = scene.BoundsByID("hero")
	if !ok || x0 != 20 || y0 != 23 || x1 != 50 || y1 != 35 {
		t.Fatalf("unexpected updated bounds: (%v, %v)-(%v, %v) ok=%v", x0, y0, x1, y1, ok)
	}
	sprite, ok := scene.World.Get[sceneapi.Sprite2D](scene.MustEntity("hero"))
	wantTint := graphics.ColorFromRGBA(0x44, 0xcc, 0x88, 0xff)
	if !ok || sprite.Width != 30 || sprite.Height != 12 || sprite.Tint != wantTint || sprite.Texture.BatchKey() != graphics.CreateTextureByColor(graphics.White).BatchKey() {
		t.Fatalf("expected hero sprite size update, got %+v ok=%v", sprite, ok)
	}
}

func TestRenderItemsPreserveStableSceneOrderWithinOrder(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "components": {
        "sprite": {
          "texture": {
            "color": "#ff0000"
          }
        }
      },
      "height": 8,
      "id": "stable",
      "order": 5,
      "width": 8
    },
    {
      "components": {
        "sprite": {
          "texture": {
            "color": "#0000ff"
          }
        }
      },
      "height": 8,
      "id": "second",
      "order": 5,
      "width": 8
    },
    {
      "components": {
        "sprite": {
          "texture": {
            "color": "#00ff00"
          }
        }
      },
      "height": 8,
      "id": "first",
      "order": 5,
      "width": 8
    }
  ]
}`)
	project, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	scene, err := project.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatalf("instantiate ecs: %v", err)
	}

	items := scene.RenderItems()
	if len(items) != 3 {
		t.Fatalf("unexpected render items: %+v", items)
	}
	for _, item := range items {
		order, ok := scene.World.Get[sceneapi.RenderOrder2D](item.Entity)
		if !ok || order != 5 || item.SortIndex != 5 {
			t.Fatalf("project order was not applied to entity %d: component=%d item=%+v", item.Entity, order, item)
		}
	}
	got := []string{scene.ID(items[0].Entity), scene.ID(items[1].Entity), scene.ID(items[2].Entity)}
	want := []string{"stable", "second", "first"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected render order at %d: got %v want %v", i, got, want)
		}
	}
}

func TestReplaceNodesPreservesStableSceneOrder(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {"components":{"sprite":{"texture":{"color":"#ff0000"}}},"height":8,"id":"first","order":5,"width":8},
    {"components":{"sprite":{"texture":{"color":"#00ff00"}}},"height":8,"id":"second","order":5,"width":8},
    {"components":{"sprite":{"texture":{"color":"#0000ff"}}},"height":8,"id":"third","order":5,"width":8}
  ]
}`)

	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	defer bundle.Dispose()
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatalf("instantiate scene: %v", err)
	}
	defer scene.Dispose()

	specs := []SceneNodeSpec{
		stableRenderSprite("first", "#ff0000"),
		stableRenderSprite("second", "#00ff00"),
		stableRenderSprite("third", "#0000ff"),
	}
	want := []string{"first", "second", "third"}
	for range 32 {
		if err := scene.ReplaceNodes(specs); err != nil {
			t.Fatalf("replace nodes: %v", err)
		}
		items := scene.RenderItems()
		got := make([]string, len(items))
		for i, item := range items {
			got[i] = scene.ID(item.Entity)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("render order after replace = %v, want %v", got, want)
		}
	}
}

func stableRenderSprite(id, color string) SceneNodeSpec {
	spec := SceneNodeSpec{
		ID: id, Width: 8, Height: 8, Order: 5,
		ScaleX: 1, ScaleY: 1, Alpha: 1,
		Components: Components(SceneComponentSprite),
	}
	spec.Components.Sprite.Texture = &TextureSpec{Color: color}
	return spec
}

func TestRenderItemsAndPickIncludeMountedParticleEffects(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{
  "effects": [
    { "id": "spark", "width": 16, "height": 12, "count": 4, "rate": 4, "loop": true }
  ]
}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "components": {
        "particle": {
          "resource": "spark"
        }
      },
      "id": "spark",
      "x": 20,
      "y": 30
    }
  ]
}`)

	project, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	scene, err := project.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatalf("instantiate ecs: %v", err)
	}

	items := scene.RenderItems()
	if len(items) != 1 || scene.ID(items[0].Entity) != "spark" {
		t.Fatalf("expected particle render item, got %+v", items)
	}
	if id, ok := scene.Pick(mgl32.Vec2{24, 34}, mgl32.Vec2{24, 34}); !ok || id != "spark" {
		t.Fatalf("expected particle effect to be picked, got %q ok=%v", id, ok)
	}

	particle := scene.MustEntity("spark")
	x0, y0, x1, y1, ok := scene.Bounds(particle)
	if !ok || x0 != 12 || y0 != 24 || x1 != 28 || y1 != 36 {
		t.Fatalf("unexpected particle bounds: (%v, %v)-(%v, %v) ok=%v", x0, y0, x1, y1, ok)
	}
}

func TestUnreferencedResourcesDoNotCreateRenderEntities(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{
	  "effects":[{"id":"spark","count":4,"loop":true}],
	  "tilemaps":[{"id":"level","width":1,"height":1,"tileWidth":1,"tileHeight":1,"tiles":[1]}]
	}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": []
}`)

	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatalf("instantiate project: %v", err)
	}
	if scene.World.EntityCount() != 0 || len(scene.RenderItems()) != 0 {
		t.Fatalf("unreferenced resources created entities: count=%d render=%+v", scene.World.EntityCount(), scene.RenderItems())
	}
}

func TestMountedTilemapCreatesRenderableGameObject(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{
	  "tilemaps":[{"id":"level","width":2,"height":1,"tileWidth":1,"tileHeight":1,"tileset":"tiles.png","tiles":[1,0]}]
	}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "components": {
        "tilemap": {
          "resource": "level"
        }
      },
      "id": "map",
      "x": 4,
      "y": 5
    }
  ]
}`)
	writeTestBinaryFile(t, dir, "tiles.png", tinyPNG(t))

	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatalf("instantiate project: %v", err)
	}
	entity := scene.MustEntity("map")
	tilemap, ok := scene.World.Get[sceneapi.Tilemap2D](entity)
	if !ok || !tilemap.Valid() || tilemap.Width != 2 || tilemap.Tiles[0] != 1 {
		t.Fatalf("unexpected tilemap component: %+v ok=%v", tilemap, ok)
	}
	if items := scene.RenderItems(); len(items) != 1 || items[0].Entity != entity {
		t.Fatalf("expected mounted tilemap render item, got %+v", items)
	}
	if x0, y0, x1, y1, ok := scene.Bounds(entity); !ok || x1-x0 != 2 || y1-y0 != 1 {
		t.Fatalf("unexpected tilemap bounds: (%v,%v)-(%v,%v) ok=%v", x0, y0, x1, y1, ok)
	}
}

func TestRenderItemsMatchRuntimeParticleBeforeTextOrder(t *testing.T) {
	world := ecs.NewWorld()
	scene := &SceneInstance{World: world}
	particleBundle := ecs.NewBundle(4)
	particleBundle.Put(sceneapi.NewTransform())
	particleBundle.Put(sceneapi.GlobalTransform{})
	particleBundle.Put(sceneapi.Visibility{Visible: true})
	particleBundle.Put(sceneapi.ParticleEmitter2D{})
	particleBundle.Put(sceneapi.RenderOrder2D(500))
	particle := world.Spawn(particleBundle)
	labelBundle := ecs.NewBundle(4)
	labelBundle.Put(sceneapi.NewTransform())
	labelBundle.Put(sceneapi.GlobalTransform{})
	labelBundle.Put(sceneapi.Visibility{Visible: true})
	labelBundle.Put(sceneapi.Text2D{Value: "label", Font: &onixtext.Font{}})
	labelBundle.Put(sceneapi.RenderOrder2D(500))
	label := world.Spawn(labelBundle)

	items := scene.RenderItems()
	if len(items) != 2 {
		t.Fatalf("expected particle and text render items, got %+v", items)
	}
	if items[0].Entity != particle || items[1].Entity != label {
		t.Fatalf("expected particle before text to match runtime order, got %+v", items)
	}
}

func TestSceneTransformMatrix2DSupportsWorldAndUIEntities(t *testing.T) {
	world := ecs.NewWorld()
	scene := &SceneInstance{World: world}

	worldBundle := ecs.NewBundle(1)
	worldMatrix := mgl32.Translate3D(-300, -188, 0)
	worldBundle.Put(sceneapi.GlobalTransform{Matrix: worldMatrix})
	worldEntity := world.Spawn(worldBundle)

	uiBundle := ecs.NewBundle(1)
	uiMatrix := mgl32.Translate3D(300, 20, 0)
	uiBundle.Put(sceneapi.UIGlobalTransform{Matrix: uiMatrix})
	uiEntity := world.Spawn(uiBundle)

	for name, test := range map[string]struct {
		entity ecs.Entity
		want   mgl32.Mat4
	}{
		"world": {entity: worldEntity, want: worldMatrix},
		"ui":    {entity: uiEntity, want: uiMatrix},
	} {
		t.Run(name, func(t *testing.T) {
			got, ok := scene.TransformMatrix2D(test.entity)
			if !ok || got != test.want {
				t.Fatalf("transform = %v, %v; want %v, true", got, ok, test.want)
			}
		})
	}
}

func TestApplyParticleSpecUpdatesEmitterInPreviewScene(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{
	  "effects": [
	    { "id": "spark", "count": 3, "rate": 6, "texture": {"color":"#ffffff"} }
	  ]
}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "components": {
        "particle": {
          "resource": "spark"
        }
      },
      "id": "spark",
      "x": 10,
      "y": 12
    }
  ]
}`)

	project, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	scene, err := project.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatalf("instantiate ecs: %v", err)
	}

	err = scene.SetParticleResource(ParticleSpec{
		ID:      "spark",
		Count:   8,
		Rate:    14,
		Texture: &TextureSpec{Color: "#ffffff"},
	})
	if err != nil {
		t.Fatalf("apply particle spec: %v", err)
	}
	particleSpec := SceneNodeSpec{ID: "spark", Components: Components(SceneComponentParticle), X: 40, Y: 50, ScaleX: 1, ScaleY: 1, Alpha: 1}
	particleSpec.Components.Particle.Resource = "spark"
	if err := scene.ApplyNode(particleSpec); err != nil {
		t.Fatalf("apply particle game object: %v", err)
	}

	entity := scene.MustEntity("spark")
	transform, ok := scene.World.Get[sceneapi.Transform](entity)
	if !ok || transform.Translation.X() != 40 || transform.Translation.Y() != 50 {
		t.Fatalf("unexpected particle transform: %+v ok=%v", transform, ok)
	}
	emitter, ok := scene.World.Get[sceneapi.ParticleEmitter2D](entity)
	if !ok || emitter.Count != 8 || emitter.Rate != 14 {
		t.Fatalf("unexpected particle emitter: %+v ok=%v", emitter, ok)
	}
}

func TestApplyNodesAddNewPreviewEntities(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{"effects":[]}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": []
}`)

	project, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	scene, err := project.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatalf("instantiate ecs: %v", err)
	}

	createdSpec := SceneNodeSpec{
		ID: "created", Components: Components(SceneComponentSprite), Width: 32, Height: 24, ScaleX: 1, ScaleY: 1, Alpha: 1,
	}
	createdSpec.Components.Sprite.Texture = &TextureSpec{Color: "#ffffff"}
	if err := scene.ApplyNode(createdSpec); err != nil {
		t.Fatalf("add game object spec: %v", err)
	}
	if _, err := scene.Entity("created"); err != nil {
		t.Fatalf("expected added game object entity: %v", err)
	}

	if err := scene.SetParticleResource(ParticleSpec{
		ID: "created-particle", Count: 12, Rate: 12, Lifetime: 1, Loop: true,
		Texture: &TextureSpec{Color: "#ffffff"},
	}); err != nil {
		t.Fatalf("add particle resource: %v", err)
	}
	createdParticleSpec := SceneNodeSpec{ID: "created-particle", Components: Components(SceneComponentParticle), ScaleX: 1, ScaleY: 1, Alpha: 1}
	createdParticleSpec.Components.Particle.Resource = "created-particle"
	if err := scene.ApplyNode(createdParticleSpec); err != nil {
		t.Fatalf("add particle object: %v", err)
	}
	if _, err := scene.Entity("created-particle"); err != nil {
		t.Fatalf("expected added particle entity: %v", err)
	}
}

func TestSceneEntitySlotAvoidsRepeatedStringLookup(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[{"id":"hero"}]}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatalf("instantiate scene: %v", err)
	}
	slot, err := scene.Slot("hero")
	if err != nil {
		t.Fatalf("resolve hero slot: %v", err)
	}
	want := scene.MustEntity("hero")
	if got, ok := scene.EntityBySlot(slot); !ok || got != want {
		t.Fatalf("slot resolved to %d/%v, want %d/true", got, ok, want)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if got, ok := scene.EntityBySlot(slot); !ok || got != want {
			t.Fatalf("slot resolved to %d/%v, want %d/true", got, ok, want)
		}
	}); allocations != 0 {
		t.Fatalf("scene slot lookup allocated %.0f objects, want 0", allocations)
	}
	if removed := scene.RemoveNodes("hero"); removed != 1 {
		t.Fatalf("removed %d entities, want 1", removed)
	}
	if got, ok := scene.EntityBySlot(slot); ok || got != 0 {
		t.Fatalf("stale scene slot resolved to %d/%v", got, ok)
	}
}

func TestApplyNodeReplacesAuthoredComponentsAcrossDimensions(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "blockInput": true,
      "components": {
        "sprite": {
          "texture": {
            "color": "#ffffff"
          }
        }
      },
      "coordinateSpace": "ui",
      "height": 16,
      "id": "node",
      "width": 24
    }
  ]
}`)

	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatalf("instantiate ecs: %v", err)
	}
	entity := scene.MustEntity("node")
	if err := scene.ApplyNode(SceneNodeSpec{
		ID: "node", Components: Components(SceneComponentCamera), ScaleX: 1, ScaleY: 1, ScaleZ: 1,
	}); err != nil {
		t.Fatalf("replace sprite with camera: %v", err)
	}
	if scene.MustEntity("node") != entity {
		t.Fatal("replacing authored components changed entity identity")
	}
	if !scene.World.Has[render3d.Camera](entity) || !scene.World.Has[sceneapi.Transform](entity) {
		t.Fatal("camera components were not installed")
	}
	if scene.World.Has[sceneapi.Sprite2D](entity) || scene.World.Has[sceneapi.UITransform](entity) || scene.World.Has[sceneapi.InputBlocker2D](entity) {
		t.Fatal("2D components remained after changing node to camera")
	}

	textSpec := SceneNodeSpec{
		ID: "node", Components: Components(SceneComponentText), CoordinateSpace: CoordinateSpaceUI,
		Width: 80, Height: 20, ScaleX: 1, ScaleY: 1, Alpha: 1, BlockInput: true,
	}
	textSpec.Components.Text.Value = "ready"
	if err := scene.ApplyNode(textSpec); err != nil {
		t.Fatalf("replace camera with text: %v", err)
	}
	if !scene.World.Has[sceneapi.Text2D](entity) || !scene.World.Has[sceneapi.UITransform](entity) || !scene.World.Has[sceneapi.InputBlocker2D](entity) {
		t.Fatal("UI text components were not installed")
	}
	if scene.World.Has[sceneapi.Camera](entity) || scene.World.Has[render3d.Camera](entity) || scene.World.Has[sceneapi.Projection](entity) || scene.World.Has[sceneapi.Transform](entity) {
		t.Fatal("3D components remained after changing node to UI text")
	}
}

func TestApplyNodeInstallsMultipleComponentsOnOneEntity(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": []
}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	err = scene.ApplyNode(SceneNodeSpec{
		ID: "invalid", Components: SceneComponents{Sprite: &SpriteComponentSpec{}, Text: &TextComponentSpec{}},
	})
	if err != nil {
		t.Fatalf("multiple component apply error = %v", err)
	}
	entity, err := scene.Entity("invalid")
	if err != nil || !scene.World.Has[sceneapi.Sprite2D](entity) || !scene.World.Has[sceneapi.Text2D](entity) {
		t.Fatalf("composed components were not installed: entity=%v err=%v", entity, err)
	}
}

func TestRemoveNodesDespawnsSubtrees(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "children": [
        {
          "id": "child"
        }
      ],
      "id": "group"
    },
    {
      "id": "sibling"
    }
  ]
}`)

	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatalf("instantiate ecs: %v", err)
	}
	if removed := scene.RemoveNodes("group"); removed != 2 {
		t.Fatalf("removed %d game objects, want 2", removed)
	}
	for _, id := range []string{"group", "child"} {
		if _, err := scene.Entity(id); err == nil {
			t.Fatalf("removed game object %q is still indexed", id)
		}
	}
	if _, err := scene.Entity("sibling"); err != nil {
		t.Fatalf("sibling was removed with subtree: %v", err)
	}
	if len(scene.Roots) != 1 || scene.ID(scene.Roots[0]) != "sibling" {
		t.Fatalf("unexpected remaining roots: %v", scene.Roots)
	}
}
