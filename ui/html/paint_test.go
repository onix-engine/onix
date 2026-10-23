package html

import (
	"fmt"
	"image"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/internal/style"
	"github.com/diiyw/onix/scene"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
)

func TestHTMLAsymmetricRoundedBackgroundAntialiasesCorners(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<button id="close" style="width:46px;height:36px;border:0;border-radius:0 7px 7px 0;background:#c42b34"></button>`)
	defer Dispose(world)
	node := incrementalHTMLNode(t, world, owner, "close")
	state := ecs.GetRef[htmlRenderState](world, owner)
	computed := state.Styles[node.node]
	var raster image.RGBA
	rasterHTMLBoxPaintPrepared(&raster, computed, 46, 36, 1, nil)
	partial := 0
	for y := range 7 {
		for x := 39; x < 46; x++ {
			alpha := raster.RGBAAt(x, y).A
			if alpha > 0 && alpha < 255 {
				partial++
			}
			if alpha != raster.RGBAAt(x, 35-y).A {
				t.Fatalf("right corner coverage is asymmetric at %d,%d", x, y)
			}
		}
	}
	if partial == 0 {
		t.Fatal("right corner arcs have no antialiasing")
	}
	if raster.RGBAAt(45, 0).A != 0 || raster.RGBAAt(45, 35).A != 0 || raster.RGBAAt(0, 0).A != 255 || raster.RGBAAt(0, 35).A != 255 || raster.RGBAAt(45, 18).A != 255 {
		t.Fatal("rounded right corners changed the square left corners or straight edge")
	}
	computed.BackgroundColor = style.ColorValue(style.Color{R: 196, G: 43, B: 52, A: 128})
	rasterHTMLBoxPaintPrepared(&raster, computed, 46, 36, 1, nil)
	if raster.RGBAAt(44, 2).A == 0 || raster.RGBAAt(44, 2).A >= 128 || raster.RGBAAt(0, 0).A != 128 {
		t.Fatal("corner coverage did not preserve background opacity")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		rasterHTMLBoxPaintPrepared(&raster, computed, 46, 36, 1, nil)
	}); allocations != 0 {
		t.Fatalf("retained rounded paint allocated %g times", allocations)
	}
}

func TestHTMLZIndexKeepsDescendantsAboveContainerBackground(t *testing.T) {
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	html := New(`<div id="overlay" style="position:absolute;z-index:90000;background:black;width:400px;height:300px"><div id="panel" style="background:white;width:200px;height:160px"><button id="button">Action</button></div></div>`)
	html.Width, html.Height, html.Font = 400, 300, notosanssc.NotoSansSCRegular
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	for iteration := range 3 {
		root, _ := Root(world, owner)
		overlay, _ := root.FindByID("overlay")
		overlay.SetAttribute("data-revision", fmt.Sprint(iteration))
		if err := Sync(world); err != nil {
			t.Fatal(err)
		}
		stacks := map[string]scene.UIStackIndex{}
		for entity, access := range ecs.Query2[htmlChild, scene.UIStackIndex](world).All {
			child := access.A.Get()
			stack := access.B.Get()

			if child.Owner == owner && child.Node != nil && (ecs.Has[scene.Sprite2D](world, entity) || ecs.Has[Control](world, entity)) {
				id, _ := child.Node.Attr("id")
				stacks[id] = stack
			}
		}
		if stacks["overlay"] < 90000 || stacks["panel"] <= stacks["overlay"] || stacks["button"] <= stacks["panel"] {
			t.Fatalf("background can obscure descendant controls after rebuild %d: %v", iteration, stacks)
		}
	}
}

func TestHTMLRasterizesLayeredBackgroundsIndependentCornersAndShadows(t *testing.T) {
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	html := New(`<div id="box" style="display:block;width:80px;height:40px;background-color:#112233;background-image:linear-gradient(to right,red,blue),linear-gradient(to bottom,white,black);border-radius:18px 4px 12px 2px;box-shadow:0 3px 6px rgba(0,0,0,.5)"></div>`)
	html.Width, html.Height = 120, 80
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		t.Fatalf("render layered background: %v", err)
	}
	state, _ := ecs.Get[htmlRenderState](world, owner)
	node, ok := NodeByID(world, owner, "box")
	if !ok {
		t.Fatal("missing box")
	}
	if state.Textures.paint[htmlTextureKey{node: node.node, variant: "background"}] == nil {
		t.Fatal("layered background did not create a retained box raster")
	}
	shadow := state.EntityGroups[htmlRenderIdentity{Node: node.node, Role: htmlRoleShadow, Variant: "shadow-0"}]
	if shadow == nil || len(shadow.Bindings) != 1 {
		t.Fatal("box shadow sprite was not emitted")
	}
	sprite, exists := ecs.Get[scene.Sprite2D](world, shadow.Bindings[0].Entity)
	if !exists || sprite.Texture == nil || !sprite.Texture.Valid() || sprite.Shape.Kind != 0 {
		t.Fatal("box shadow was not rasterized with its blur")
	}
	computed := state.Styles[node.node]
	if htmlRoundedPixel(computed, 1, 1, 80, 40) || !htmlRoundedPixel(computed, 12, 2, 80, 40) {
		t.Fatal("individual top-left corner radius was not applied")
	}
}

func TestHTMLPaintTexturesResizeAndReleaseDetachedNodes(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="box" style="width:40px;height:20px;background:linear-gradient(to right,red,blue);box-shadow:0 2px 3px black"></div><div id="group" style="width:30px;height:20px;opacity:.5"><div id="child" style="width:10px;height:10px;background:blue"></div></div>`)
	defer Dispose(world)
	box := incrementalHTMLNode(t, world, owner, "box")
	group := incrementalHTMLNode(t, world, owner, "group")
	child := incrementalHTMLNode(t, world, owner, "child")
	state, _ := ecs.Get[htmlRenderState](world, owner)
	key := htmlTextureKey{node: box.node, variant: "background"}
	old := state.Textures.paint[key].texture
	box.SetAttribute("style", "width:70px;height:25px;background:linear-gradient(to right,red,blue);box-shadow:0 2px 3px black")
	incrementalSync(t, world)
	state, _ = ecs.Get[htmlRenderState](world, owner)
	replacement := state.Textures.paint[key].texture
	if old == replacement || replacement.Width != 70 || replacement.Height != 25 || old.Valid() {
		t.Fatalf("texture did not resize/release: old valid=%v new=%dx%d", old.Valid(), replacement.Width, replacement.Height)
	}
	box.Remove()
	group.Remove()
	incrementalSync(t, world)
	state, _ = ecs.Get[htmlRenderState](world, owner)
	if replacement.Valid() || state.Textures.paint[key] != nil || state.Textures.rasters[group.node] != nil || state.Textures.rasters[child.node] != nil || len(state.Textures.entries) != 0 || len(state.Textures.entities) != 0 {
		t.Fatalf("detached paint retained resources: entries=%d entities=%d rasters=%d paint=%d", len(state.Textures.entries), len(state.Textures.entities), len(state.Textures.rasters), len(state.Textures.paint))
	}
}

func TestHTMLOverflowCreatesHierarchyClipForDescendants(t *testing.T) {
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	html := New(`<div id="viewport" style="width:100px;height:40px;overflow:hidden"><div id="content" style="width:240px;height:40px;background:#fff">content</div></div>`)
	html.Width, html.Height = 300, 100
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	viewport, ok := NodeByID(world, owner, "viewport")
	if !ok {
		t.Fatal("missing overflow element")
	}
	var clip ecs.Entity
	var clipRect scene.ClipRect2D
	var contentParent ecs.Entity
	for entity, access := range ecs.Query3[htmlChild, htmlOverflowClip, scene.ClipRect2D](world).All {
		child := access.A.Get()
		rect := access.C.Get()

		if child.Owner == owner && child.Node == viewport.node {
			clip, clipRect = entity, rect
			break
		}
	}
	if clip == 0 || clipRect != (scene.ClipRect2D{Width: 100, Height: 40}) {
		t.Fatalf("overflow clip = %d %+v", clip, clipRect)
	}
	content, _ := NodeByID(world, owner, "content")
	for _, access := range ecs.Query2[htmlChild, scene.Parent](world).All {
		child := access.A.Get()
		parent := access.B.Get()

		if child.Owner == owner && child.Node == content.node {
			contentParent = parent.Entity
			break
		}
	}
	if contentParent != clip {
		t.Fatalf("content parent = %d, want overflow clip %d", contentParent, clip)
	}
	if err := scene.SyncUIGlobalTransformSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	global, ok := ecs.Get[scene.UIGlobalTransform](world, contentParent)
	bounds, clipped := global.ClipRect()
	if !ok || !clipped || bounds.Width != 100 || bounds.Height != 40 {
		t.Fatalf("descendant clip = %+v, clipped=%t", bounds, clipped)
	}
}

func TestHTMLOverflowAutoCreatesScrollState(t *testing.T) {
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	html := New(`<div id="viewport" style="width:100px;height:40px;overflow:auto"><div style="width:240px;height:80px;background:#fff"></div></div>`)
	html.Width, html.Height = 300, 100
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	var clip ecs.Entity
	for entity, access := range ecs.Query2[htmlChild, scene.UIScroll](world).All {
		child := access.A.Get()
		scroll := access.B.Get()

		if child.Owner == owner && child.Node != nil {
			if id, _ := child.Node.Attr("id"); id == "viewport" {
				clip = entity
				if !scroll.Horizontal || !scroll.Vertical || scroll.ContentSize.X() < 240 || scroll.ContentSize.Y() < 80 {
					t.Fatalf("overflow scroll state = %+v", scroll)
				}
				break
			}
		}
	}
	if clip == 0 {
		t.Fatal("overflow:auto did not create a scroll container")
	}
}

func TestHTMLZIndexRespectsNegativeContextsAndDocumentOrder(t *testing.T) {
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	html := New(`<div id="negative" style="position:absolute;z-index:-1;width:80px;height:20px"><div id="deep" style="width:80px;height:20px;background:#fff"></div></div><div id="later" style="position:absolute;width:80px;height:20px;background:#fff"></div>`)
	html.Width, html.Height = 100, 40
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	stacks := map[string]scene.UIStackIndex{}
	for entity, access := range ecs.Query2[htmlChild, scene.UIStackIndex](world).All {
		child := access.A.Get()
		stack := access.B.Get()

		if child.Owner != owner || child.Node == nil || !ecs.Has[scene.Sprite2D](world, entity) {
			continue
		}
		id, _ := child.Node.Attr("id")
		if id != "" {
			stacks[id] = stack
		}
	}
	if stacks["negative"] >= stacks["later"] {
		t.Fatalf("negative stacking context escaped behind later sibling: %+v", stacks)
	}
	if stacks["deep"] >= stacks["later"] {
		t.Fatalf("deep auto-stacked descendant covered later sibling: %+v", stacks)
	}
}

func TestHTMLZIndexKeepsNestedContextInsideParent(t *testing.T) {
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	html := New(`<div id="low" style="position:absolute;z-index:1;width:80px;height:20px;background:#111"><div id="low-child" style="position:absolute;z-index:999999;width:80px;height:20px;background:#222"></div></div><div id="high" style="position:absolute;z-index:2;width:80px;height:20px;background:#333"><div id="high-child" style="position:absolute;z-index:-999999;width:80px;height:20px;background:#444"></div></div>`)
	html.Width, html.Height = 100, 40
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	stacks := map[string]scene.UIStackIndex{}
	for entity, access := range ecs.Query2[htmlChild, scene.UIStackIndex](world).All {
		child := access.A.Get()
		stack := access.B.Get()

		if child.Owner != owner || child.Node == nil || !ecs.Has[scene.Sprite2D](world, entity) {
			continue
		}
		id, _ := child.Node.Attr("id")
		if id != "" {
			stacks[id] = stack
		}
	}
	if stacks["low-child"] >= stacks["high"] {
		t.Fatalf("nested low context escaped above high context: %+v", stacks)
	}
	if stacks["high-child"] <= stacks["low"] {
		t.Fatalf("nested high context did not remain above low context: %+v", stacks)
	}
}

func TestHTMLObjectFitPreservesLayoutAndClipsImage(t *testing.T) {
	texture := graphics.CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 100, 50)))
	t.Cleanup(func() { _ = texture.Dispose() })
	for _, test := range []struct {
		fit, position       string
		x, y, width, height float32
	}{
		{"fill", "center", 0, 0, 200, 200},
		{"contain", "center", 0, 50, 200, 100},
		{"cover", "center", -100, 0, 400, 200},
		{"none", "center", 50, 75, 100, 50},
		{"scale-down", "center", 50, 75, 100, 50},
		{"none", "right 10px bottom 20px", 90, 130, 100, 50},
	} {
		t.Run(test.fit+"/"+test.position, func(t *testing.T) {
			world := ecs.NewWorld()
			ecs.InsertResource[ImageProvider](world, testHTMLImageProvider{texture: texture})
			bundle := ecs.NewBundle(1)
			html := New(fmt.Sprintf(`<img id="image" src="test.png" style="display:block;width:200px;height:200px;object-fit:%s;object-position:%s">`, test.fit, test.position))
			html.Width, html.Height = 300, 300
			ecs.Put(&bundle, html)
			owner := world.Spawn(bundle)
			if err := Sync(world); err != nil {
				t.Fatal(err)
			}
			node, _ := NodeByID(world, owner, "image")
			var entity ecs.Entity
			for candidate, access := range ecs.Query[htmlChild](world).All {
				child := access.Get()

				if child.Owner == owner && child.Node == node.node && (ecs.Has[scene.Sprite2D](world, candidate) || ecs.Has[scene.SpriteLayers2D](world, candidate)) {
					entity = candidate
					break
				}
			}
			sprite, _ := ecs.Get[scene.Sprite2D](world, entity)
			x, y := float32(0), float32(0)
			if test.fit != "fill" {
				layers, ok := ecs.Get[scene.SpriteLayers2D](world, entity)
				if !ok || len(layers.Layers) != 1 {
					t.Fatal("fitted image layer missing")
				}
				layer := layers.Layers[0]
				sprite, x, y = layer.Sprite, layer.Transform.X, layer.Transform.Y
				clip, _ := ecs.Get[scene.ClipRect2D](world, entity)
				if clip != (scene.ClipRect2D{Width: 200, Height: 200}) {
					t.Fatalf("image escaped content box: %+v", clip)
				}
			}
			if x != test.x || y != test.y || sprite.Width != test.width || sprite.Height != test.height {
				t.Fatalf("image rect = %v,%v %vx%v", x, y, sprite.Width, sprite.Height)
			}
			size, _ := ecs.Get[scene.Size2D](world, entity)
			target, _ := ecs.Get[scene.InputTarget2D](world, entity)
			if size != (scene.Size2D{Width: 200, Height: 200}) || target.Width != 200 || target.Height != 200 {
				t.Fatal("object-fit changed layout or hit bounds")
			}
		})
	}
}
