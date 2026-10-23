package project

import (
	"os"
	"testing"

	"github.com/diiyw/onix/ecs"
	sceneapi "github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/scene/render3d"
	engineui "github.com/diiyw/onix/ui"
)

func TestEmptyNodeIsNeutralAtRuntime(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[{"id":"root","width":40,"height":20,"blockInput":true}]}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()
	entity := scene.MustEntity("root")
	if items := scene.RenderItems(); len(items) != 0 {
		t.Fatalf("empty node produced render items: %+v", items)
	}
	if ecs.Has[sceneapi.InputTarget2D](scene.World, entity) || ecs.Has[sceneapi.InputBlocker2D](scene.World, entity) {
		t.Fatal("empty node should not participate in pointer input")
	}
	entityBounds := scene.MustEntity("root")
	if _, _, _, _, ok := scene.Bounds(entityBounds); !ok {
		t.Fatal("authored empty-node bounds should remain available for editor/layout geometry")
	}
}

func TestNodeComponentsComposeWithoutKind(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[{"id":"mixed","width":16,"height":12,"components":{"sprite":{"texture":{"color":"#ffffff"}},"text":{"value":"hello"}}}]}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()
	entity := scene.MustEntity("mixed")
	if !ecs.Has[sceneapi.Sprite2D](scene.World, entity) || !ecs.Has[sceneapi.Text2D](scene.World, entity) {
		t.Fatal("all authored visual components must be attached to one node")
	}
}

func TestAnimationIsAnIndependentNodeComponent(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[
		{"id":"bound","components":{"animation":{"clip":{"duration":1,"floatTracks":[{"binding":{"path":"part","target":"transform.position.x"},"keyframes":[{"time":0,"value":4}]}]}}},"children":[{"id":"part","width":20,"height":10,"components":{"sprite":{"texture":{"color":"#ffffff"}}}}]},
		{"id":"root","components":{"animation":{"clip":{"duration":1,"textureTracks":[{"binding":{"target":"sprite.texture"},"keyframes":[{"time":0,"value":{"color":"#ffffff"}}]}]}}}},
		{"id":"mixed","components":{"sprite":{"texture":{"color":"#000000"}},"animation":{"clip":{"duration":1,"textureTracks":[{"binding":{"target":"sprite.texture"},"keyframes":[{"time":0,"value":{"color":"#ffffff"}}]}]}}}}
	]}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()
	bound := scene.MustEntity("bound")
	if ecs.Has[sceneapi.Sprite2D](scene.World, bound) || !ecs.Has[sceneapi.Animator](scene.World, bound) {
		t.Fatal("child-bound animation should be independent of a root Sprite2D")
	}
	part := scene.MustEntity("part")
	if transform, ok := ecs.Get[sceneapi.Transform](scene.World, part); !ok || transform.Translation.X() != 4 {
		t.Fatalf("bound child transform = %+v", transform)
	}
	if _, _, _, _, ok := scene.Bounds(part); !ok {
		t.Fatal("animated child bounds were not reported")
	}
	root := scene.MustEntity("root")
	if !ecs.Has[sceneapi.Sprite2D](scene.World, root) || !ecs.Has[sceneapi.Animator](scene.World, root) {
		t.Fatal("root texture animation should materialize a Sprite2D")
	}
	mixed := scene.MustEntity("mixed")
	animator, _ := ecs.Get[sceneapi.Animator](scene.World, mixed)
	sprite, _ := ecs.Get[sceneapi.Sprite2D](scene.World, mixed)
	if animator.CurrentTexture() == nil || sprite.Texture != animator.CurrentTexture() {
		t.Fatal("root animation did not initialize a composed Sprite2D on its first frame")
	}
	if err := scene.ApplyNode(SceneNodeSpec{ID: "bound"}); err != nil {
		t.Fatal(err)
	}
	if ecs.Has[sceneapi.Animator](scene.World, bound) || ecs.Has[sceneapi.Sprite2D](scene.World, bound) {
		t.Fatal("removing animation did not restore an empty node")
	}
}

func TestEmptyNodeCanOwnScripts(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[{"id":"logic","scripts":[{"name":"Logic"}]}]}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()
	var bound ecs.Entity
	err = scene.SetScriptRegistry(NewScriptRegistry(ScriptBinding{
		Name: "Logic",
		Bind: func(context *ScriptContext, _ ScriptComponentSpec) error {
			bound = context.Entity
			return nil
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if bound != scene.MustEntity("logic") {
		t.Fatalf("script bound to %v, want logic entity", bound)
	}
}

func TestNodeComponentsComposeAcrossDimensions(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[{"id":"mixed","components":{"sprite":{"texture":{"color":"#ffffff"}},"camera":{"fov":1.1,"near":0.1}}}]}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()
	entity := scene.MustEntity("mixed")
	if !ecs.Has[sceneapi.Sprite2D](scene.World, entity) || !ecs.Has[sceneapi.Camera](scene.World, entity) || !ecs.Has[render3d.Camera](scene.World, entity) {
		t.Fatal("2D and 3D components must coexist on one world node")
	}
}

func TestNodeComponentsComposeResourceAndCameraParts(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[]}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()
	if err := scene.SetParticleResource(ParticleSpec{
		ID: "spark", Texture: &TextureSpec{Color: "#ffffff"}, Count: 1, Rate: 1, Lifetime: 1,
		SizeMin: 1, SizeMax: 1, AlphaStart: 1, AlphaEnd: 1, Loop: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := scene.SetTilemapResource(TilemapSpec{ID: "tiles", Width: 1, Height: 1, TileWidth: 1, TileHeight: 1, Tiles: []int{0}}); err != nil {
		t.Fatal(err)
	}
	if err := scene.SetModelResource(ModelSpec{ID: "cube", Mesh: &MeshSpec{Primitive: "cube", Size: 1}}); err != nil {
		t.Fatal(err)
	}
	components := Components(SceneComponentParticle, SceneComponentTilemap, SceneComponentModel, SceneComponentCamera)
	components.Particle.Resource = "spark"
	components.Tilemap.Resource = "tiles"
	components.Model.Resource = "cube"
	spec := SceneNodeSpec{ID: "composed", Components: components, ScaleX: 1, ScaleY: 1, ScaleZ: 1, Alpha: 1}
	if err := scene.ApplyNode(spec); err != nil {
		t.Fatal(err)
	}
	entity := scene.MustEntity("composed")
	if !ecs.Has[sceneapi.ParticleEmitter2D](scene.World, entity) || !ecs.Has[sceneapi.Tilemap2D](scene.World, entity) ||
		!ecs.Has[render3d.Mesh](scene.World, entity) || !ecs.Has[sceneapi.Camera](scene.World, entity) || !ecs.Has[render3d.Camera](scene.World, entity) {
		t.Fatal("resource and camera components were not composed on one entity")
	}
	if err := scene.ApplyNode(SceneNodeSpec{ID: "composed"}); err != nil {
		t.Fatal(err)
	}
	if ecs.Has[sceneapi.ParticleEmitter2D](scene.World, entity) || ecs.Has[sceneapi.Tilemap2D](scene.World, entity) ||
		ecs.Has[render3d.Mesh](scene.World, entity) || ecs.Has[sceneapi.Camera](scene.World, entity) || ecs.Has[render3d.Camera](scene.World, entity) {
		t.Fatal("empty composition retained resource or camera components")
	}
}

func TestApplyNodePreservesCompiledCameraVisibility(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[{"id":"node"}]}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()
	visible := false
	spec := SceneNodeSpec{ID: "node", Components: Components(SceneComponentCamera), Visible: &visible}
	if err := scene.ApplyNode(spec); err != nil {
		t.Fatal(err)
	}
	entity := scene.MustEntity("node")
	camera, ok := ecs.Get[sceneapi.Camera](scene.World, entity)
	if !ok || camera.Active {
		t.Fatalf("hidden camera was activated during composition: %+v ok=%v", camera, ok)
	}
}

func TestApplyNodeCanReturnToNeutralEmptyComposition(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[{"id":"node"}]}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()
	entity := scene.MustEntity("node")
	if err := scene.ApplyNode(SceneNodeSpec{ID: "node", Components: Components(SceneComponentSprite)}); err != nil {
		t.Fatal(err)
	}
	if !ecs.Has[sceneapi.Sprite2D](scene.World, entity) {
		t.Fatal("sprite component was not attached")
	}
	if err := scene.ApplyNode(SceneNodeSpec{ID: "node", Width: 20, Height: 10, BlockInput: true}); err != nil {
		t.Fatal(err)
	}
	if ecs.Has[sceneapi.Sprite2D](scene.World, entity) || ecs.Has[sceneapi.InputBlocker2D](scene.World, entity) || len(scene.RenderItems()) != 0 {
		t.Fatal("empty composition retained render or input state")
	}
}

func TestApplyNodeFillsDirectConstructionDefaults(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[{"id":"node"}]}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()
	spec := SceneNodeSpec{ID: "node", Components: Components(SceneComponentSprite), Width: 12, Height: 8}
	spec.Components.Sprite.Texture = &TextureSpec{Color: "#ffffff"}
	if err := scene.ApplyNode(spec); err != nil {
		t.Fatal(err)
	}
	entity := scene.MustEntity("node")
	transform, ok := ecs.Get[sceneapi.Transform](scene.World, entity)
	if !ok || transform.Scale.X() != 1 || transform.Scale.Y() != 1 {
		t.Fatalf("direct node defaults produced transform %+v ok=%v", transform, ok)
	}
	sprite, ok := ecs.Get[sceneapi.Sprite2D](scene.World, entity)
	if !ok || sprite.Alpha != 1 || len(scene.RenderItems()) != 1 {
		t.Fatalf("direct node defaults produced sprite=%+v renderItems=%d ok=%v", sprite, len(scene.RenderItems()), ok)
	}
}

func TestApplyNodeUnbindsControlWhenCompositionBecomesEmpty(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[{"id":"button","coordinateSpace":"ui","width":80,"height":24,"control":{}}]}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()
	entity := scene.MustEntity("button")
	if err := BindControl(scene, "button", engineui.Button{}); err != nil {
		t.Fatal(err)
	}
	if !ecs.Has[engineui.Control](scene.World, entity) || !ecs.Has[sceneapi.InputTarget2D](scene.World, entity) {
		t.Fatal("control binding did not install input state")
	}
	if err := scene.ApplyNode(SceneNodeSpec{ID: "button", CoordinateSpace: CoordinateSpaceUI, Width: 80, Height: 24}); err != nil {
		t.Fatal(err)
	}
	if ecs.Has[engineui.Control](scene.World, entity) || ecs.Has[sceneapi.InputTarget2D](scene.World, entity) || ecs.Has[sceneapi.InputState2D](scene.World, entity) {
		t.Fatal("empty node retained control input state")
	}
}

func TestControlCompositionOnlyIncludesItsAuthoredDescendants(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[
		{"id":"first","coordinateSpace":"ui","width":80,"height":24,"control":{},"children":[
			{"id":"first-label","coordinateSpace":"ui","components":{"text":{"value":"First"}}}
		]},
		{"id":"second","coordinateSpace":"ui","width":80,"height":24,"control":{},"children":[
			{"id":"second-label","coordinateSpace":"ui","components":{"text":{"value":"Second"}}}
		]}
	]}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()
	if err := BindControl(scene, "first", engineui.Button{}); err != nil {
		t.Fatal(err)
	}
	firstLabel := scene.MustEntity("first-label")
	secondLabel := scene.MustEntity("second-label")
	if relation, ok := ecs.Get[engineui.Relation[engineui.Control]](scene.World, firstLabel); !ok || relation.Owner != scene.MustEntity("first") {
		t.Fatalf("first control composition relation = %+v ok=%v", relation, ok)
	}
	if ecs.Has[engineui.Relation[engineui.Control]](scene.World, secondLabel) {
		t.Fatal("second control received a relation while first control was bound")
	}
}
