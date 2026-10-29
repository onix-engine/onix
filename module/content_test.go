package module

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	graphicspng "github.com/diiyw/onix/graphics/codec/png"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/scene/render3d"
	htmlui "github.com/diiyw/onix/ui/html"
)

type contentPNGDecoder = graphics.ImageDecoderSet[graphicspng.Decoder, graphics.NoImageDecoder, graphics.NoImageDecoder, graphics.NoImageDecoder, graphics.NoImageDecoder]

func TestModuleHTMLDocumentOwnershipAndChildInput(t *testing.T) {
	plan, source := compileFixture(t, map[string]string{
		"resources/scenes/main.json": `{"modules":[{"id":"a","mod":"page"},{"id":"b","mod":"page"}]}`,
		"page/mod.json":              `{"components":{"size":[200,100],"html":{"resource":"page"}},"resources":{"page":{"kind":"html","path":"index.html"}}}`,
		"page/index.html":            `<button id="click">Click</button><span id="label">Waiting</span>`,
	})
	calls := 0
	registry := Registry{}
	must(t, registry.RegisterMod("page", func() Signal { return Signal{} }, Lifecycle[Signal]{OnMessage: func(c *Context, _ *Signal, message Message) error {
		event, ok := message.Value[Input]()
		if !ok {
			return nil
		}
		if event.HTMLID != "click" || event.Event.Entity != 0 || len(event.Event.Path) != 0 {
			t.Fatal("child input escaped module ownership", event)
		}
		doc, err := HTML(c, c.Root())
		if err != nil {
			return err
		}
		label, ok := doc.FindByID("label")
		if !ok || !label.SetText("Clicked") {
			t.Fatal("owned document cannot be edited")
		}
		calls++
		return nil
	}}))
	must(t, registry.RegisterScene("main.json", func() Signal { return Signal{} }, Lifecycle[Signal]{OnMessage: func(c *Context, _ *Signal, m Message) error {
		if m.Topic != InputTopic {
			return nil
		}
		target := m.From
		if target == "" {
			if err := c.Forward("disabled", m); err != nil {
				return err
			}
			target = "live"
		}
		return c.Forward(target, m)
	}}))
	r := newFixture(t, plan, registry, Options{Source: source})
	s, err := r.Manager().StartEntry()
	must(t, err)
	a, b := &s.modules[0], &s.modules[1]
	docA, err := HTML(a, a.Root())
	must(t, err)
	docB, err := HTML(b, b.Root())
	must(t, err)
	if _, err := HTML(a, b.Root()); err != ErrOwnership {
		t.Fatal("foreign surface was accepted", err)
	}
	labelA, _ := docA.FindByID("label")
	labelB, _ := docB.FindByID("label")
	if docA.Append(labelB.Clone()) {
		t.Fatal("foreign DOM was appended")
	}
	must(t, htmlui.Sync(r.world))
	var child ecs.Entity
	for entity, access := range r.world.Query[htmlui.Control]().All {
		if value := access.Get(); value.Owner == a.root && value.ID == "click" {
			child = entity
			break
		}
	}
	if child == 0 {
		t.Fatal("HTML did not create a control")
	}
	must(t, r.HandleInput(scene.InputEvent{Entity: child, Kind: scene.InputEventTap}))
	must(t, r.Dispatch())
	must(t, r.Dispatch())
	if calls != 1 {
		t.Fatal("generated child did not reach the owning behavior")
	}
	if allocs := testing.AllocsPerRun(100, func() {
		if !labelA.SetText("Clicked") {
			panic("invalid owned node")
		}
	}); allocs != 0 {
		t.Fatalf("unchanged private DOM edit allocated %g", allocs)
	}
	surface, _ := a.Get[htmlui.Surface](a.Root())
	surface.Value = `<span id="label">New</span>`
	must(t, a.Set(a.Root(), surface))
	if labelA.SetText("stale") {
		t.Fatal("old document handle remained valid")
	}
	must(t, a.Dispose())
	if docA.SetText("disposed") {
		t.Fatal("disposed HTML handle remained valid")
	}
	if !labelB.SetText("Still independent") {
		t.Fatal("disposing one instance invalidated another document")
	}
}

func TestModuleImportedModelSharesRigAndIsolatesPoseAndMaterials(t *testing.T) {
	data, err := os.ReadFile("../project/resources/fixtures/gltf/embedded_cubic.gltf")
	must(t, err)
	plan, source := compileFixture(t, map[string]string{
		"resources/scenes/main.json": `{"modules":[{"id":"a","mod":"model"},{"id":"b","mod":"model"}]}`,
		"model/mod.json":             `{"space":"world","components":{"model":{"id":"model","path":"model.gltf"}}}`,
		"model/model.gltf":           string(data),
	})
	r := newFixture(t, plan, Registry{}, Options{Source: source, Resources: project.ImageFeature[contentPNGDecoder]{}})
	s, err := r.Manager().StartEntry()
	must(t, err)
	a, b := &s.modules[0], &s.modules[1]
	rigA, okA := a.Get[*project.ModelRig](a.Root())
	rigB, okB := b.Get[*project.ModelRig](b.Root())
	poseA, _ := a.Get[*project.ModelPose](a.Root())
	poseB, _ := b.Get[*project.ModelPose](b.Root())
	if !okA || !okB || rigA != rigB || poseA == nil || poseB == nil || poseA == poseB {
		t.Fatal("model rig or pose sharing is incorrect")
	}
	materialA, _ := a.Get[render3d.Material[graphics.StandardMaterial]](a.Root())
	materialB, _ := b.Get[render3d.Material[graphics.StandardMaterial]](b.Root())
	if materialA.Material == nil || materialB.Material == nil || materialA.Material == materialB.Material {
		t.Fatal("model material crossed instances")
	}
	materialA.Material.PerceptualRoughness = .123
	if materialB.Material.PerceptualRoughness == .123 {
		t.Fatal("mutable material crossed instances")
	}
	meshA, _ := a.Get[render3d.Mesh](a.Root())
	meshB, _ := b.Get[render3d.Mesh](b.Root())
	if meshA.Mesh == nil || meshA.Mesh != meshB.Mesh {
		t.Fatal("immutable mesh was not shared")
	}
	clip, err := rigA.Animation("cubic-move")
	must(t, err)
	poseA.Sample(clip, .5, false)
	if poseA.NodeMatrix(0) == poseB.NodeMatrix(0) {
		t.Fatal("animation pose crossed instances")
	}
	if allocs := testing.AllocsPerRun(100, func() { poseA.Sample(clip, .5, false) }); allocs != 0 {
		t.Fatalf("warm model sampling allocated %g", allocs)
	}
	must(t, a.Dispose())
	poseB.Sample(clip, .5, false)
	if len(r.contentResources) != 1 {
		t.Fatal("first instance released shared model")
	}
	must(t, b.Dispose())
	if len(r.contentResources) != 0 {
		t.Fatal("model content store leaked")
	}
}

func TestModulePrivateContentSharesAssetsAndIsolatesMutableState(t *testing.T) {
	plan, source := compileFixture(t, map[string]string{
		"resources/scenes/main.json": `{"modules":[{"id":"a","mod":"panel"},{"id":"b","mod":"panel","placement":{"position":[100,0]}}]}`,
		"panel/mod.json":             `{"elements":[{"id":"image","components":{"sprite":{"texture":{"image":"pixel.png"},"size":[20,20]},"animation":{"clip":{"duration":1,"sampleRate":60,"floatTracks":[{"binding":{"target":"transform.position.x"},"keyframes":[{"time":0,"value":0},{"time":1,"value":10}]}]}}}},{"id":"child","parent":"image","transform":{"position":[5,0]},"components":{"sprite":{"color":"#ffffff","size":[2,2]}}},{"id":"map","components":{"tilemap":{"id":"tiles","tileset":"pixel.png","width":1,"height":1,"tileWidth":1,"tileHeight":1,"tiles":[0]}}}]}`,
		"panel/pixel.png":            "placeholder",
	})
	var data bytes.Buffer
	bitmap := image.NewRGBA(image.Rect(0, 0, 1, 1))
	bitmap.SetRGBA(0, 0, color.RGBA{255, 0, 0, 255})
	must(t, png.Encode(&data, bitmap))
	source["panel/pixel.png"].Data = data.Bytes()
	r := newFixture(t, plan, Registry{}, Options{Source: source, Resources: project.ImageFeature[contentPNGDecoder]{}})
	s, err := r.MountEntry()
	must(t, err)
	a, b := &s.modules[0], &s.modules[1]
	imageA, err := a.Element("image")
	must(t, err)
	imageB, err := b.Element("image")
	must(t, err)
	spriteA, _ := a.Get[scene.Sprite2D](imageA)
	spriteB, _ := b.Get[scene.Sprite2D](imageB)
	if spriteA.Texture != spriteB.Texture || a.resources != b.resources {
		t.Fatal("immutable assets were not shared")
	}
	animatorA, _ := a.Get[scene.Animator](imageA)
	animatorB, _ := b.Get[scene.Animator](imageB)
	if animatorA.Clip != animatorB.Clip {
		t.Fatal("immutable clip was copied")
	}
	animatorA.Seek(.5)
	must(t, a.Set(imageA, animatorA))
	animatorB, _ = b.Get[scene.Animator](imageB)
	if animatorB.Time() != 0 {
		t.Fatal("animator state crossed instances")
	}
	mapA, err := a.Element("map")
	must(t, err)
	mapB, err := b.Element("map")
	must(t, err)
	tileA, _ := a.Get[scene.Tilemap2D](mapA)
	tileB, _ := b.Get[scene.Tilemap2D](mapB)
	tileA.Tiles[0] = 7
	if tileB.Tiles[0] != 0 {
		t.Fatal("tilemap state crossed instances")
	}
	if _, ok := a.Get[scene.Sprite2D](imageB); ok {
		t.Fatal("private element escaped ownership")
	}
	must(t, scene.SyncUIGlobalTransformSystem("").Run(r.world))
	childA, err := a.Element("child")
	must(t, err)
	childB, err := b.Element("child")
	must(t, err)
	globalA, _ := a.Get[scene.UIGlobalTransform](childA)
	globalB, _ := b.Get[scene.UIGlobalTransform](childB)
	if globalA.Matrix[12] != 5 || globalB.Matrix[12] != 105 {
		t.Fatal("private hierarchy ignored scene placement")
	}
	must(t, a.Dispose())
	if len(r.contentResources) != 1 || b.resources.refs != 1 || !r.world.IsAlive(imageB.entity) {
		t.Fatal("first instance released shared content")
	}
	must(t, b.Dispose())
	if len(r.contentResources) != 0 {
		t.Fatal("last content store leaked")
	}
}

func TestModuleHTMLPrivateURLsPauseResumeAndUnload(t *testing.T) {
	plan, source := compileFixture(t, map[string]string{
		"resources/project.json":     `{"entry":"main.json","scenes":["main.json","next.json"]}`,
		"resources/scenes/main.json": `{"modules":[{"id":"page","mod":"page"}]}`,
		"resources/scenes/next.json": `{"modules":[]}`,
		"page/mod.json":              `{"components":{"size":[100,80],"html":{"resource":"page"}},"resources":{"page":{"kind":"html","path":"pages/index.html"}}}`,
		"page/pages/index.html":      `<div style="width:40px;height:30px;background:red"></div><img src="pixel.png" style="width:2px;height:2px">`,
		"page/pages/pixel.png":       "placeholder",
	})
	var data bytes.Buffer
	must(t, png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 1, 1))))
	source["page/pages/pixel.png"].Data = data.Bytes()
	r := newFixture(t, plan, Registry{}, Options{Source: source, Resources: project.ImageFeature[contentPNGDecoder]{}})
	s, err := r.Manager().StartEntry()
	must(t, err)
	c := &s.modules[0]
	surface, _ := c.Get[htmlui.Surface](c.Root())
	for _, name := range []string{"pixel.png", "./pixel.png", "p%69xel.png?version=1#icon"} {
		if texture, ok := surface.Images.Provider.ResolveHTMLImage(name); !ok || texture == nil {
			t.Fatal("relative private HTML image was not loaded", name)
		}
	}
	for _, name := range []string{"../../outside.png", "%2e%2e/%2e%2e/outside.png", "/page/pages/pixel.png", "%2fpixel.png", "https://example.com/image.png", "//example.com/image.png", `%2e%2e%5cother%5cimage.png`, `..\other\image.png`, "%zz"} {
		if _, ok := surface.Images.Provider.ResolveHTMLImage(name); ok {
			t.Fatal("HTML resource escaped", name)
		}
	}
	must(t, htmlui.Sync(r.world))
	baseline := r.world.EntityCount()
	if baseline <= len(c.entities)+3 {
		t.Fatal("HTML did not create retained content")
	}
	must(t, r.Manager().Switch(SceneSwitchRequest{Scene: "next.json", Retain: true}))
	must(t, r.Dispatch())
	must(t, htmlui.Sync(r.world))
	for entity, access := range r.world.Query[scene.Sprite2D]().All {
		_ = access
		if scene.IsActive(r.world, entity) {
			t.Fatal("suspended HTML child remained active")
		}
		if visibility, ok := r.world.Get[scene.Visibility](entity); ok && visibility.Visible {
			t.Fatal("suspended HTML child remained visible")
		}
	}
	must(t, r.Manager().Switch(SceneSwitchRequest{Scene: "main.json"}))
	must(t, r.Dispatch())
	must(t, htmlui.Sync(r.world))
	if r.Manager().Active() != s || r.world.EntityCount() != baseline {
		t.Fatal("resume replaced or leaked retained HTML")
	}
	must(t, r.Close())
	if r.world.EntityCount() != 0 {
		t.Fatal("unload left HTML descendants", r.world.EntityCount())
	}
}

func TestModuleGlobalInputAndAnimationHonorSceneActivity(t *testing.T) {
	plan, source := compileFixture(t, map[string]string{
		"resources/project.json":     `{"entry":"main.json","scenes":["main.json","next.json"]}`,
		"resources/scenes/main.json": `{"modules":[{"id":"live","mod":"box"},{"id":"disabled","mod":"box","enabled":false}]}`,
		"resources/scenes/next.json": `{"modules":[]}`,
		"box/mod.json":               `{"components":{"input":{"keyboard":true},"animation":{"clip":{"duration":1,"sampleRate":60,"loop":true,"floatTracks":[{"binding":{"target":"transform.position.x"},"keyframes":[{"time":0,"value":0},{"time":1,"value":10}]}]}}}}`,
	})
	var inputs int
	registry := Registry{}
	must(t, registry.RegisterMod("box", func() Signal { return Signal{} }, Lifecycle[Signal]{OnMessage: func(_ *Context, _ *Signal, message Message) error {
		event, ok := message.Value[Input]()
		if !ok {
			return nil
		}
		if event.Event.Entity != 0 || event.Event.Path != nil {
			t.Fatal("global input leaked ECS identities")
		}
		inputs++
		return nil
	}}))
	must(t, registry.RegisterScene("main.json", func() Signal { return Signal{} }, Lifecycle[Signal]{OnMessage: func(c *Context, _ *Signal, m Message) error {
		if m.Topic != InputTopic {
			return nil
		}
		target := m.From
		if target == "" {
			if err := c.Forward("disabled", m); err != nil {
				return err
			}
			target = "live"
		}
		return c.Forward(target, m)
	}}))
	r := newFixture(t, plan, registry, Options{Source: source})
	s, err := r.Manager().StartEntry()
	must(t, err)
	event := scene.InputEvent{Kind: scene.InputEventKeyDown, Path: []ecs.Entity{s.modules[0].root}}
	must(t, r.HandleInput(event))
	must(t, r.Dispatch())
	must(t, r.Dispatch())
	if inputs != 1 {
		t.Fatal("global input reached disabled module")
	}
	r.world.InsertResource(&scene.Time{Delta: time.Second / 4})
	must(t, scene.UpdateUIAnimatorSystem("").Run(r.world))
	a, _ := s.modules[0].Get[scene.Animator](s.modules[0].Root())
	disabled, _ := s.modules[1].Get[scene.Animator](s.modules[1].Root())
	if a.Time() != .25 || disabled.Time() != 0 {
		t.Fatal("animation ignored enabled state")
	}
	must(t, r.Manager().Switch(SceneSwitchRequest{Scene: "next.json", Retain: true}))
	must(t, r.Dispatch())
	must(t, r.HandleInput(event))
	must(t, r.Dispatch())
	must(t, r.Dispatch())
	must(t, scene.UpdateUIAnimatorSystem("").Run(r.world))
	paused, _ := s.modules[0].Get[scene.Animator](s.modules[0].Root())
	if inputs != 1 || paused.Time() != a.Time() {
		t.Fatal("suspended content still consumed input or advanced")
	}
	must(t, r.Manager().Switch(SceneSwitchRequest{Scene: "main.json"}))
	must(t, r.Dispatch())
	must(t, r.HandleInput(event))
	must(t, r.Dispatch())
	must(t, r.Dispatch())
	if inputs != 2 {
		t.Fatal("resumed module lost global input")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if err := r.HandleInput(event); err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatalf("warm global input allocated %g", allocations)
	}
}
