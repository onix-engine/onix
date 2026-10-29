package editor

import (
	"math"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/scene/render3d"
	htmlui "github.com/diiyw/onix/ui/html"
	"github.com/go-gl/mathgl/mgl32"
)

func workspace3DEditorFixture(t *testing.T) (*ModuleEditor, *ecs.SystemContext) {
	t.Helper()
	a := workspaceEditorFixture(t)
	t.Cleanup(a.dispose)
	if err := a.Document.CreateMod("model", project.ModSpec{
		Space:     project.CoordinateSpaceWorld,
		Transform: project.ModTransformSpec{Position: []float32{2, 0, 0}},
		Components: project.ModComponents{
			Model:  &project.ModelSpec{ID: "cube", Mesh: &project.MeshSpec{Primitive: "cube", Size: 2}},
			Camera: &project.Camera3DSpec{Fov: .7, Near: .1, ClearColor: "#303030"},
		},
		Elements: []project.ModElementSpec{
			{ID: "parent", Transform: project.ModTransformSpec{Position: []float32{0, 3, 0}}},
			{ID: "child", Parent: "parent", Transform: project.ModTransformSpec{Position: []float32{0, 0, 4}}, Components: project.ModComponents{Model: &project.ModelSpec{ID: "sphere", Mesh: &project.MeshSpec{Primitive: "sphere", Radius: 1}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.Document.AddInstance("model", "model", ""); err != nil {
		t.Fatal(err)
	}
	context := &ecs.SystemContext{World: ecs.NewWorld()}
	a.action("toggle-dimension")
	graphics.BindFrame(context.World, graphics.CurrentContext())
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if a.viewport3D.err != nil {
		t.Fatal(a.viewport3D.err)
	}
	return a, context
}

func TestWorkspace3DModelsPrivateTransformsAndRetainedEntities(t *testing.T) {
	a, context := workspace3DEditorFixture(t)
	v := &a.viewport3D
	row := &v.rows[len(v.rows)-1]
	root, child := row.parts[0].entity, row.parts[2].entity
	global, _ := v.world.Get[scene.GlobalTransform](child)
	if got := global.Matrix.Col(3).Vec3(); got != (mgl32.Vec3{12, 23, 4}) {
		t.Fatalf("private parent and scene/local transforms lost: %v", got)
	}
	cameras := 0
	for range v.world.Query[scene.Camera]().All {
		cameras++
	}
	if cameras != 1 {
		t.Fatalf("authored cameras entered editor preview: %d", cameras)
	}
	if err := a.Document.EditInstance("model", "Move", func(value *project.ModInstanceSpec) error { value.Placement.Position = []float32{5, 0, 0}; return nil }); err != nil {
		t.Fatal(err)
	}
	a.changed()
	graphics.BindFrame(context.World, graphics.CurrentContext())
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	global, _ = v.world.Get[scene.GlobalTransform](child)
	if row.parts[0].entity != root || row.parts[2].entity != child || global.Matrix[12] != 17 {
		t.Fatal("placement edit recreated model geometry or lost private transforms")
	}
	if allocs := testing.AllocsPerRun(100, func() {
		if err := a.syncViewport3D(); err != nil {
			panic(err)
		}
		graphics.BindFrame(context.World, graphics.CurrentContext())
		if err := a.sync(context); err != nil {
			panic(err)
		}
	}); allocs != 0 {
		t.Fatalf("stable 3D preview allocated %g", allocs)
	}
	a.action("toggle-dimension")
	graphics.BindFrame(context.World, graphics.CurrentContext())
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	visible, _ := context.World.Get[scene.Visibility](a.canvas)
	if !visible.Visible {
		t.Fatal("2D canvas did not return after dimension switch")
	}
	a.action("toggle-dimension")
	a.sync(context)
	visible, _ = context.World.Get[scene.Visibility](a.canvas)
	if visible.Visible || row.parts[0].entity != root {
		t.Fatal("dimension switch overlaid 2D canvas or recreated 3D models")
	}
}

type workspace3DTestDriver struct {
	*viewport3DTestDriver
	draws          int
	sprites        []render.Instance2D
	spriteTextures []render.TextureHandle
	grid           render.MeshBatch3D
}

func (d *workspace3DTestDriver) DrawSprites(batch render.SpriteBatch) error {
	for _, draw := range batch.Draws {
		for _, instance := range batch.Instances[draw.FirstInstance : draw.FirstInstance+draw.InstanceCount] {
			d.sprites = append(d.sprites, instance)
			d.spriteTextures = append(d.spriteTextures, draw.TextureAt(int(instance.TextureSlot)))
		}
	}
	return nil
}

func (d *workspace3DTestDriver) DrawMeshes(batch render.MeshBatch3D) error {
	d.draws += len(batch.Draws)
	if len(batch.Instances) == 1 && batch.Instances[0].Material[3] == 8192 {
		d.grid = batch
	}
	return d.viewport3DTestDriver.DrawMeshes(batch)
}

func TestWorkspace3DRenderTargetModelsAndGuides(t *testing.T) {
	driver := &workspace3DTestDriver{viewport3DTestDriver: &viewport3DTestDriver{viewport: render.Rect{Width: 1600, Height: 960}}}
	render.Uninstall()
	if err := render.Install(driver); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(render.Uninstall)
	canvas := graphics.NewCanvas2D(render.ActiveDevice(), 1600, 960)
	t.Cleanup(graphics.SetCurrentContextForTesting(canvas))
	a, context := workspace3DEditorFixture(t)
	// Include the origin guides and the translated scene in the same frustum.
	a.viewport3D.view.target, a.viewport3D.view.distance = mgl32.Vec3{0, 10, 0}, 70
	if err := a.render(context); err != nil {
		t.Fatal(err)
	}
	if err := canvas.FinishFrame(); err != nil {
		t.Fatal(err)
	}
	if driver.draws != 7 || len(driver.clears) != 3 || !driver.clears[0].ClearDepth || driver.clears[1].ClearDepth || driver.clears[2].ClearDepth {
		t.Fatalf("models/grid/axes did not reach depth target: draws %d, clears %+v", driver.draws, driver.clears)
	}
	if len(driver.grid.Vertices) != 4 || len(driver.grid.Indices) != 6 || driver.grid.Instances[0].Custom[1][2] < 70*12 {
		t.Fatal("legacy analytic grid did not cover the camera's visible ground")
	}
	w, h := a.viewport3D.view.renderTarget.Size()
	bounds := a.canvasBounds()
	if w != int(bounds.W) || h != int(bounds.H) {
		t.Fatalf("3D target crossed panel boundaries: %dx%d, %+v", w, h, bounds)
	}
	count, index := 0, 0
	for i, texture := range driver.spriteTextures {
		if texture == a.viewport3D.view.colorTexture.RenderHandle() {
			count, index = count+1, i
		}
	}
	if count != 1 {
		t.Fatalf("3D canvas did not submit one preview sprite: %d", count)
	}
	clip := driver.sprites[index].Custom
	if clip[2] != ([4]float32{bounds.X, bounds.Y, bounds.X + bounds.W, bounds.Y + bounds.H}) || clip[3][0] != editorPanelCornerRadius-1 || clip[3][1] != 1 {
		t.Fatalf("3D preview lost rounded panel clipping: %+v", clip)
	}
	projection, _ := a.viewport3D.world.Get[scene.Projection](a.viewport3D.view.camera)
	row := a.viewport3D.rows[len(a.viewport3D.rows)-1]
	global, _ := a.viewport3D.world.Get[scene.GlobalTransform](row.parts[0].entity)
	camera := preview3DCameraTransformAround(a.viewport3D.view.target, a.viewport3D.view.yaw, a.viewport3D.view.pitch, a.viewport3D.view.distance/a.zoom).LocalMatrix()
	point := projection.Matrix(bounds.W, bounds.H).Mul4(camera.Inv()).Mul4x1(global.Matrix.Col(3))
	x, y := bounds.X+(point.X()/point.W()+1)*bounds.W*.5, bounds.Y+(1-point.Y()/point.W())*bounds.H*.5
	if id := a.pickViewport3D(x, y); id != "model" {
		t.Fatalf("3D canvas picking did not select owning module: %q", id)
	}
}

func TestWorkspace3DNavigationDoesNotEditDocument(t *testing.T) {
	a, context := workspace3DEditorFixture(t)
	in := input.NewState()
	bounds := a.canvasBounds()
	x, y := float64(bounds.X+bounds.W/2), float64(bounds.Y+bounds.H/2)
	revision, yaw := a.Document.Revision, a.viewport3D.view.yaw
	in.PressMouse(input.MouseRight, x, y)
	a.updateViewport3DInput(in)
	in.EndFrame()
	in.MoveMouse(x+30, y+10)
	a.updateViewport3DInput(in)
	if a.viewport3D.view.yaw == yaw || a.Document.Revision != revision {
		t.Fatal("orbit failed or changed authored transforms")
	}
	in.ReleaseMouse(input.MouseRight, x+30, y+10)
	a.updateViewport3DInput(in)
	graphics.BindFrame(context.World, graphics.CurrentContext())
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if err := htmlui.Sync(context.World); err != nil {
		t.Fatal(err)
	}
	if allocs := testing.AllocsPerRun(50, func() {
		a.update(context)
		a.sync(context)
		htmlui.Sync(context.World)
	}); allocs != 0 {
		t.Fatalf("idle 3D editor frame allocated %g", allocs)
	}
}

func TestWorkspace3DEmptyCanvasSkipsModuleResources(t *testing.T) {
	a := workspaceEditorFixture(t)
	context := &ecs.SystemContext{World: ecs.NewWorld()}
	a.action("toggle-dimension")
	graphics.BindFrame(context.World, graphics.CurrentContext())
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	v := &a.viewport3D
	if v.err != nil {
		t.Fatal(v.err)
	}
	if len(v.mods) != 0 {
		t.Fatal("empty 3D canvas initialized 2D module resources")
	}
	for range v.world.Query[render3d.Mesh]().All {
		t.Fatal("guides became retained scene meshes")
	}
	if v.view.target != (mgl32.Vec3{4, 4, 0}) || v.view.distance != 14 || v.view.yaw != 0 || v.view.pitch != 0 {
		t.Fatal("empty canvas lost the legacy default camera", v.view)
	}
	if allocs := testing.AllocsPerRun(50, func() {
		a.action("toggle-dimension")
		a.action("toggle-dimension")
		if err := a.syncViewport3D(); err != nil {
			panic(err)
		}
	}); allocs != 0 {
		t.Fatalf("retained dimension switch allocated %g", allocs)
	}
}

func TestWorkspace3DOrientationAndProjectionStayPrivate(t *testing.T) {
	for action := 1; action <= 6; action++ {
		a, _ := workspace3DEditorFixture(t)
		v := &a.viewport3D.view
		v.yaw, v.pitch = .7, .5
		revision, history := a.Document.Revision, len(a.Document.undo)
		target, distance := v.target, v.distance
		layout := preview3DOrientationGeometry(a.preview3DOrientationRect(), preview3DCameraTransformAround(v.target, v.yaw, v.pitch, v.distance).LocalMatrix())
		var point mgl32.Vec2
		for _, node := range layout.nodes {
			if node.action == action {
				point = node.point
			}
		}
		in := input.NewState()
		in.PressMouse(input.MouseLeft, float64(point.X()), float64(point.Y()))
		a.updateViewport3DInput(in)
		in.EndFrame()
		in.ReleaseMouse(input.MouseLeft, float64(point.X()), float64(point.Y()))
		a.updateViewport3DInput(in)
		axis := (action - 1) / 2
		sign := float32(((action-1)%2)*2 - 1)
		camera := preview3DCameraTransformAround(v.target, v.yaw, v.pitch, v.distance).LocalMatrix()
		projection := a.viewport3D.world.GetRef[scene.Projection](v.camera)
		if abs(camera.Col(2)[axis]-sign) > .0001 || projection.Kind != scene.ProjectionOrthographic {
			t.Fatal("axis did not snap to its orthographic face", action, camera.Col(2))
		}
		if v.target != target || v.distance != distance || a.Document.Revision != revision || len(a.Document.undo) != history {
			t.Fatal("orientation edited the document or changed framing")
		}
		center := a.canvasBounds()
		in.EndFrame()
		in.MoveMouse(float64(center.X+center.W/2), float64(center.Y+center.H/2))
		in.PressKey(input.KeyO)
		a.updateViewport3DInput(in)
		if v.orthographic || a.viewport3D.world.GetRef[scene.Projection](v.camera).Kind != scene.ProjectionPerspective {
			t.Fatal("O did not restore perspective")
		}
	}
}

func TestWorkspace3DOrthographicDragAndFixedSnap(t *testing.T) {
	a, context := workspace3DEditorFixture(t)
	a.selectInstance("model")
	a.viewport3D.view.orthographic = true
	a.focusViewport3D()
	index := len(a.viewport3D.rows) - 1
	x, y := workspace3DCanvasPoint(t, a, a.viewport3D.rows[index].parts[0].entity)
	a.beginViewport3DDrag("model", "", x, y)
	if a.canvasDrag.ID == "" {
		t.Fatal("orthographic drag did not start")
	}
	original := a.canvasDrag.PlanePoint
	a.updateViewport3DDrag(input.Pointer{X: x + 30, Y: y + 20, Down: true})
	projection := a.viewport3D.world.GetRef[scene.Projection](a.viewport3D.view.camera)
	units := preview3DWorldPerPixel(projection, a.viewport3D.view.distance/a.zoom, a.canvasBounds())
	if abs(a.canvasDrag.WorldDelta.X()-30*units) > .0001 || abs(a.canvasDrag.WorldDelta.Y()+20*units) > .0001 || abs(a.canvasDrag.WorldDelta.Z()) > .0001 {
		t.Fatal("orthographic drag used perspective rays", a.canvasDrag.WorldDelta, original)
	}
	a.cancelPlacementDrag()
	graphics.BindFrame(context.World, graphics.CurrentContext())
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	a.settings.SnapToGrid, a.settings.Viewport3D.SnapStep = true, .25
	a.beginViewport3DDrag("model", "", x, y)
	a.updateViewport3DDrag(input.Pointer{X: x + 30, Y: y + 20, Down: true})
	delta := a.canvasDrag.Inverse3D.Mul4x1(a.canvasDrag.WorldDelta.Vec4(0)).Vec3()
	for axis := range delta {
		value := (delta[axis] + a.canvasDrag.Position[axis]) / .25
		if abs(value-float32(math.Round(float64(value)))) > .0001 {
			t.Fatal("fixed snap did not use its selected step", value)
		}
	}
	a.cancelPlacementDrag()
}

func TestWorkspace3DStudioAndSceneAppearanceIsolation(t *testing.T) {
	a, context := workspace3DEditorFixture(t)
	v := &a.viewport3D
	if err := a.syncViewport3DEnvironment(); err != nil {
		t.Fatal(err)
	}
	environment := v.world.GetRef[render3d.Environment](v.view.camera)
	lighting := v.world.GetRef[render3d.ViewLighting](v.view.camera)
	if environment.Atlas != v.view.studioTexture || environment.Sky != &v.view.sky || !lighting.Override || !lighting.DisableShadows {
		t.Fatal("legacy studio appearance was not applied")
	}
	if err := a.Document.EditMod("model", "Camera appearance", func(mod *project.ModSpec) error {
		mod.Components.Camera.ClearColor, mod.Components.Camera.FXAA = "#123456", true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	a.changed()
	graphics.BindFrame(context.World, graphics.CurrentContext())
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	revision := a.Document.Revision
	a.settings.Viewport3D.SceneLighting, a.settings.Viewport3D.Background = true, 3
	if err := a.syncViewport3DEnvironment(); err != nil {
		t.Fatal(err)
	}
	if lighting.Override || environment.Sky != nil || v.world.GetRef[scene.Camera](v.view.camera).ClearColor != graphics.ColorFromRGB(18, 52, 86) || !v.world.GetRef[render3d.Camera](v.view.camera).FXAA {
		t.Fatal("scene camera appearance was not isolated and copied")
	}
	if a.Document.Revision != revision {
		t.Fatal("preview lighting changed the authored camera")
	}
	count := 0
	for range v.world.Query[scene.Camera]().All {
		count++
	}
	if count != 1 {
		t.Fatal("scene appearance spawned authored cameras", count)
	}
	if allocs := testing.AllocsPerRun(50, func() {
		if err := a.syncViewport3DEnvironment(); err != nil {
			panic(err)
		}
	}); allocs != 0 {
		t.Fatalf("stable scene appearance allocated %g", allocs)
	}
}

func TestWorkspace3DGuideDrawingDoesNotAllocate(t *testing.T) {
	driver := &viewport3DTestDriver{viewport: render.Rect{Width: 1600, Height: 900}}
	render.Uninstall()
	if err := render.Install(driver); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(render.Uninstall)
	canvas := graphics.NewCanvas2D(render.ActiveDevice(), 1600, 900)
	t.Cleanup(graphics.SetCurrentContextForTesting(canvas))
	a, context := workspace3DEditorFixture(t)
	a.viewport3D.view.yaw, a.viewport3D.view.pitch = -.6, .45
	frame := func() {
		driver.shapes, driver.clears = driver.shapes[:0], driver.clears[:0]
		if err := a.render(context); err != nil {
			panic(err)
		}
		if err := canvas.FinishFrame(); err != nil {
			panic(err)
		}
	}
	frame()
	if allocs := testing.AllocsPerRun(30, frame); allocs != 0 {
		t.Fatalf("stable viewport rendering allocated %g", allocs)
	}
}
