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
	engineui "github.com/diiyw/onix/ui"
	htmlui "github.com/diiyw/onix/ui/html"
)

func workspaceElementDragFixture(t *testing.T) (*ModuleEditor, *ecs.SystemContext) {
	t.Helper()
	a := workspaceEditorFixture(t)
	if err := a.Document.EditMod("button", "Nested elements", func(mod *project.ModSpec) error {
		mod.Elements = []project.ModElementSpec{
			{ID: "group", Transform: project.ModTransformSpec{Position: []float32{100, 100}, Rotation: []float32{0, 0, math.Pi / 2}, Scale: []float32{2, 3}}},
			{ID: "child", Parent: "group", Transform: project.ModTransformSpec{Position: []float32{10, 20}}, Components: project.ModComponents{Sprite: &project.ModSpriteSpec{Color: "#ff0000", Size: []float32{20, 10}}}},
			{ID: "other", Components: project.ModComponents{Sprite: &project.ModSpriteSpec{Color: "#00ff00", Size: []float32{5, 5}}}},
			{ID: "leaf", Parent: "child", Transform: project.ModTransformSpec{Position: []float32{20, 10}}, Components: project.ModComponents{Sprite: &project.ModSpriteSpec{Color: "#ffffff", Size: []float32{5, 5}}}},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	a.enterModuleEdit(a.Document.Active, "start")
	context := &ecs.SystemContext{World: ecs.NewWorld()}
	graphics.BindFrame(context.World, graphics.CurrentContext())
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	return a, context
}

func TestWorkspaceCanvasTreeSelectionHighlightsModulesAndElements(t *testing.T) {
	driver := &viewport3DTestDriver{viewport: render.Rect{Width: 1600, Height: 900}}
	render.Uninstall()
	if err := render.Install(driver); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(render.Uninstall)
	canvas := graphics.NewCanvas2D(render.ActiveDevice(), 1600, 900)
	t.Cleanup(graphics.SetCurrentContextForTesting(canvas))
	a, context := workspaceElementDragFixture(t)
	for _, action := range []string{"select:start", "element:main.json/start/group", "element:main.json/start/child"} {
		a.handleEvent(htmlui.Event{ID: action, Kind: htmlui.EventClick})
		graphics.BindFrame(context.World, graphics.CurrentContext())
		if err := a.sync(context); err != nil {
			t.Fatal(err)
		}
		bounds, ok := a.viewport2DSelectionBounds(0, a.viewport2DView())
		if !ok || bounds.W <= 0 || bounds.H <= 0 {
			t.Fatal("tree selection has no live bounds", action, bounds)
		}
		driver.shapes = driver.shapes[:0]
		if err := a.render(context); err != nil {
			t.Fatal(err)
		}
		if err := canvas.FinishFrame(); err != nil {
			t.Fatal(err)
		}
		found := false
		color := editorTheme.accent
		for _, shape := range driver.shapes {
			if shape.Border == ([4]float32{color.R, color.G, color.B, color.A}) {
				found = true
				if abs(shape.Model[2]-(bounds.X-2)) > .001 || abs(shape.Model[5]-(bounds.Y-2)) > .001 {
					t.Fatal("highlight did not follow selected live geometry", shape.Model, bounds)
				}
			}
		}
		if !found {
			t.Fatal("tree selection was not drawn onto the canvas", action)
		}
	}
	// A parent includes its descendants even when sibling definitions intervene.
	a.selectElement("start", "child")
	childBounds, _ := a.viewport2DSelectionBounds(0, a.viewport2DView())
	leafBounds, _ := a.viewport2DContentBounds(a.viewport2D.rows[0].targets["leaf"], a.viewport2DView(), false)
	if !viewportContains(childBounds, leafBounds.X, leafBounds.Y) || !viewportContains(childBounds, leafBounds.X+leafBounds.W, leafBounds.Y+leafBounds.H) {
		t.Fatal("selected group excluded a descendant", childBounds, leafBounds)
	}
	frame := func() {
		driver.shapes, driver.clears = driver.shapes[:0], driver.clears[:0]
		if err := a.render(context); err != nil {
			panic(err)
		}
		if err := canvas.FinishFrame(); err != nil {
			panic(err)
		}
	}
	if allocs := testing.AllocsPerRun(100, frame); allocs != 0 {
		t.Fatalf("selected canvas render allocated %g", allocs)
	}
}

func TestWorkspaceCanvasElementDragPreviewCommitUndoAndCancel(t *testing.T) {
	a, context := workspaceElementDragFixture(t)
	a.selectElement("start", "child")
	graphics.BindFrame(context.World, graphics.CurrentContext())
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	row, world := &a.viewport2D.rows[0], a.viewport2D.world
	child, leaf := row.targets["child"], row.targets["leaf"]
	original := ecs.GetRef[scene.UIGlobalTransform](world, child).Matrix
	originalLeaf := ecs.GetRef[scene.UIGlobalTransform](world, leaf).Matrix
	box, _ := a.viewport2DContentBounds(child, a.viewport2DView(), false)
	bounds := a.canvasBounds()
	x, y := bounds.X+box.X+box.W/2, bounds.Y+box.Y+box.H/2
	revision, history := a.Document.Revision, len(a.Document.undo)
	sceneRevision := a.Document.Scene().Revision
	a.pressViewport2D(x, y)
	if a.canvasDrag.Element != "child" || !a.canvasDrag.Definition {
		t.Fatal("tree-selected element did not start a definition drag", a.canvasDrag)
	}
	graphics.BindFrame(context.World, graphics.CurrentContext())
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	move := func(dx, dy float32) {
		a.updatePlacementDrag(input.Pointer{X: x + dx, Y: y + dy, Down: true})
		graphics.BindFrame(context.World, graphics.CurrentContext())
		if err := a.sync(context); err != nil {
			panic(err)
		}
	}
	move(30, 20)
	if a.Document.Revision != revision || len(a.Document.undo) != history {
		t.Fatal("drag preview changed authored data before release")
	}
	global := ecs.GetRef[scene.UIGlobalTransform](world, child).Matrix
	globalLeaf := ecs.GetRef[scene.UIGlobalTransform](world, leaf).Matrix
	if abs((global[12]-original[12])*a.canvasScale-30) > .001 || abs((global[13]-original[13])*a.canvasScale-20) > .001 || abs(globalLeaf[12]-originalLeaf[12]-global[12]+original[12]) > .001 {
		t.Fatal("drag did not invert the parent rotation/scale or move descendants", original, global)
	}
	if allocs := testing.AllocsPerRun(100, func() { move(40, 30); move(30, 20) }); allocs != 0 {
		t.Fatalf("element drag and preview sync allocated %g", allocs)
	}
	want := a.canvasDrag.Position
	want[0], want[1] = want[0]+a.canvasDrag.Delta[0], want[1]+a.canvasDrag.Delta[1]
	a.updatePlacementDrag(input.Pointer{X: x + 30, Y: y + 20, Released: true})
	position := a.Document.Mods["button"].Spec.Elements[1].Transform.Position
	if position[0] != want[0] || position[1] != want[1] || a.Document.Scene().Revision != sceneRevision || len(a.Document.undo) != history+1 {
		t.Fatal("element drag did not commit one isolated definition edit", position, want)
	}
	if err := a.Document.Undo(); err != nil {
		t.Fatal(err)
	}
	a.changed()
	graphics.BindFrame(context.World, graphics.CurrentContext())
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if got := a.Document.Mods["button"].Spec.Elements[1].Transform.Position; got[0] != 10 || got[1] != 20 {
		t.Fatal("undo did not restore element position", got)
	}
	a.beginCanvasDrag("start", "child", x, y)
	move(40, 30)
	history = len(a.Document.undo)
	in := input.NewState()
	in.PressKey(input.KeyEscape)
	a.updateOutlinerMenu(in)
	graphics.BindFrame(context.World, graphics.CurrentContext())
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if a.canvasDrag.ID != "" || a.Local != "button" || len(a.Document.undo) != history {
		t.Fatal("Escape did not cancel the drag while retaining module editing")
	}
	child = a.viewport2D.rows[0].targets["child"]
	if got := ecs.GetRef[scene.UIGlobalTransform](a.viewport2D.world, child).Matrix; got != original {
		t.Fatal("cancel did not restore live element transform", got, original)
	}
}

func TestWorkspaceCanvasModuleDefinitionDragAndClick(t *testing.T) {
	a, context := workspaceElementDragFixture(t)
	revision, history := a.Document.Scene().Revision, len(a.Document.undo)
	original := a.Document.Mods["button"].Plan.Transform.Position
	a.beginPlacementDrag("start", 100, 100)
	a.updatePlacementDrag(input.Pointer{X: 130, Y: 120, Down: true})
	graphics.BindFrame(context.World, graphics.CurrentContext())
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	a.updatePlacementDrag(input.Pointer{X: 130, Y: 120, Released: true})
	position := a.Document.Mods["button"].Spec.Transform.Position
	if abs(position[0]-original[0]-30/a.canvasScale) > .001 || abs(position[1]-original[1]-20/a.canvasScale) > .001 || a.Document.Scene().Revision != revision || len(a.Document.undo) != history+1 {
		t.Fatal("local module drag changed instance placement or lost preview scale", position)
	}
	graphics.BindFrame(context.World, graphics.CurrentContext())
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	history = len(a.Document.undo)
	a.beginPlacementDrag("start", 100, 100)
	a.updatePlacementDrag(input.Pointer{X: 102, Y: 102, Released: true})
	if len(a.Document.undo) != history {
		t.Fatal("click created a move command")
	}
}

func TestWorkspaceCanvasMouseCaptureAndElementPicking(t *testing.T) {
	a, context := workspaceElementDragFixture(t)
	world := context.World
	engineui.RegisterRuntimeFeature[engineui.FullRuntime](world)
	scene.RegisterCameraRenderer2D(world)
	camera := scene.SpawnCamera2D(world)
	ecs.GetMut[scene.Camera](world, camera).UpdateViewport(int(a.width), int(a.height))
	ecs.InsertResource(world, scene.ViewportSize{Width: a.width, Height: a.height, ScaleFactor: 1})
	if err := scene.SyncGlobalTransformSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	in := input.NewState()
	ecs.InsertResource(world, in)
	a.selectInstance("start")
	syncWorkspaceTest(t, a, world)
	if err := scene.SyncUIGlobalTransformSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	child := a.viewport2D.rows[0].targets["child"]
	box, ok := a.viewport2DContentBounds(child, a.viewport2DView(), false)
	if !ok {
		t.Fatal("missing element geometry")
	}
	canvas := a.canvasBounds()
	x, y := float64(canvas.X+box.X+box.W*.5), float64(canvas.Y+box.Y+box.H*.5)
	in.PressMouse(input.MouseLeft, x, y)
	for _, system := range []ecs.SystemConfig{scene.DispatchInputEvents2DSystem(""), engineui.UpdateSystem("")} {
		if err := system.Run(world); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.update(context); err != nil {
		t.Fatal(err)
	}
	if a.canvasDrag.Element != "child" || a.selectedElement != "child" || a.Selected != "start" {
		t.Fatal("mouse press through canvas HTML did not pick the module element", a.canvasDrag, a.Selected, a.selectedElement)
	}
	history := len(a.Document.undo)
	in.EndFrame()
	// Capture remains active after crossing into another editor panel.
	in.MoveMouse(float64(canvas.X+canvas.W+20), y+10)
	if err := a.update(context); err != nil {
		t.Fatal(err)
	}
	if !a.canvasDrag.Moved || len(a.Document.undo) != history {
		t.Fatal("drag lost capture at the canvas boundary")
	}
	in.EndFrame()
	in.ReleaseMouse(input.MouseLeft, float64(canvas.X+canvas.W+20), y+10)
	if err := a.update(context); err != nil {
		t.Fatal(err)
	}
	if a.canvasDrag.ID != "" || len(a.Document.undo) != history+1 {
		t.Fatal("release outside the canvas did not commit the captured drag")
	}
}

func TestWorkspace3DCanvasInstanceDragThroughSceneTransform(t *testing.T) {
	a, context := workspace3DEditorFixture(t)
	if err := a.Document.EditScene(a.Document.Active, "Transform scene", func(spec *project.SceneSpec) error {
		spec.Transform.Rotation, spec.Transform.Scale = []float32{.3, .4, .5}, []float32{2, 3, 4}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	a.selectInstance("model")
	graphics.BindFrame(context.World, graphics.CurrentContext())
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	a.focusViewport3D()
	index := len(a.viewport3D.rows) - 1
	x, y := workspace3DCanvasPoint(t, a, a.viewport3D.rows[index].parts[0].entity)
	in := input.NewState()
	ecs.InsertResource(context.World, in)
	history, revision, modRevision := len(a.Document.undo), a.Document.Revision, a.Document.Mods["model"].Revision
	child := a.viewport3D.rows[index].parts[2].entity
	original := ecs.GetRef[scene.GlobalTransform](a.viewport3D.world, child).Matrix
	in.PressMouse(input.MouseLeft, float64(x), float64(y))
	if err := a.update(context); err != nil {
		t.Fatal(err)
	}
	if a.canvasDrag.ID != "model" || a.canvasDrag.Definition {
		t.Fatal("scene canvas mouse press did not start an instance drag")
	}
	in.EndFrame()
	in.MoveMouse(float64(x+25), float64(y+20))
	if err := a.update(context); err != nil {
		t.Fatal(err)
	}
	graphics.BindFrame(context.World, graphics.CurrentContext())
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	drag := a.canvasDrag
	global := ecs.GetRef[scene.GlobalTransform](a.viewport3D.world, child).Matrix
	if global == original || global.Col(3).Vec3().Sub(original.Col(3).Vec3()).Sub(drag.WorldDelta).Len() > .001 || a.Document.Revision != revision {
		t.Fatal("instance preview lost scene rotation/scale or authored during movement")
	}
	in.EndFrame()
	in.ReleaseMouse(input.MouseLeft, float64(x+25), float64(y+20))
	if err := a.update(context); err != nil {
		t.Fatal(err)
	}
	position := a.Document.Instance("model").Placement.Position
	if len(a.Document.undo) != history+1 || a.Document.Mods["model"].Revision != modRevision || abs(position[0]-drag.Position[0]-drag.Delta[0]) > .001 || abs(position[2]-drag.Position[2]-drag.DeltaZ) > .001 {
		t.Fatal("3D instance drag failed to isolate and commit its local placement", position)
	}
	if err := a.Document.Undo(); err != nil {
		t.Fatal(err)
	}
	a.changed()
	graphics.BindFrame(context.World, graphics.CurrentContext())
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if got := ecs.GetRef[scene.GlobalTransform](a.viewport3D.world, child).Matrix; got != original {
		t.Fatal("3D instance undo did not restore retained geometry")
	}
}

func TestWorkspace3DCanvasModuleAndElementDrag(t *testing.T) {
	a, context := workspace3DEditorFixture(t)
	a.enterModuleEdit(a.Document.Active, "model")
	graphics.BindFrame(context.World, graphics.CurrentContext())
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	for _, element := range []string{"", "parent", "child"} {
		if element == "" {
			a.selectInstance("model")
		} else {
			a.selectElement("model", element)
		}
		graphics.BindFrame(context.World, graphics.CurrentContext())
		if err := a.sync(context); err != nil {
			t.Fatal(err)
		}
		bounds, ok := a.viewport3DSelectionBounds(0)
		if !ok {
			t.Fatal("3D selection has no projected bounds", element)
		}
		x, y := bounds.X+bounds.W/2, bounds.Y+bounds.H/2
		revision, history, sceneRevision := a.Document.Revision, len(a.Document.undo), a.Document.Scene().Revision
		a.beginCanvasDrag("model", element, x, y)
		if a.canvasDrag.ID != "model" || a.canvasDrag.Element != element || !a.canvasDrag.ThreeD {
			t.Fatal("3D selection did not start matching drag", a.canvasDrag)
		}
		graphics.BindFrame(context.World, graphics.CurrentContext())
		if err := a.sync(context); err != nil {
			t.Fatal(err)
		}
		child := a.viewport3D.rows[0].parts[2].entity
		original := ecs.GetRef[scene.GlobalTransform](a.viewport3D.world, child).Matrix
		move := func(dx float32) {
			a.updatePlacementDrag(input.Pointer{X: x + dx, Y: y + 15, Down: true})
			graphics.BindFrame(context.World, graphics.CurrentContext())
			if err := a.sync(context); err != nil {
				panic(err)
			}
		}
		move(25)
		global := ecs.GetRef[scene.GlobalTransform](a.viewport3D.world, child).Matrix
		if global == original || global.Col(3).Vec3().Sub(original.Col(3).Vec3()).Sub(a.canvasDrag.WorldDelta).Len() > .001 || a.Document.Revision != revision || len(a.Document.undo) != history {
			t.Fatal("3D drag did not preview descendants without authoring", element)
		}
		if allocs := testing.AllocsPerRun(100, func() { move(35); move(25); a.viewport3DSelectionBounds(0) }); allocs != 0 {
			t.Fatalf("3D drag and selection allocated %g", allocs)
		}
		a.updatePlacementDrag(input.Pointer{X: x + 25, Y: y + 15, Released: true})
		if len(a.Document.undo) != history+1 || a.Document.Scene().Revision != sceneRevision {
			t.Fatal("3D drag did not commit one definition edit", element)
		}
		if err := a.Document.Undo(); err != nil {
			t.Fatal(err)
		}
		a.changed()
		graphics.BindFrame(context.World, graphics.CurrentContext())
		if err := a.sync(context); err != nil {
			t.Fatal(err)
		}
		child = a.viewport3D.rows[0].parts[2].entity
		if got := ecs.GetRef[scene.GlobalTransform](a.viewport3D.world, child).Matrix; got != original {
			t.Fatal("3D undo lost original geometry", element, got, original)
		}
	}
}
