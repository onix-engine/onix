package editor

import (
	"math"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	htmlui "github.com/diiyw/onix/ui/html"
)

func workspace3DCanvasPoint(t *testing.T, a *ModuleEditor, entity ecs.Entity) (float32, float32) {
	t.Helper()
	v, bounds := &a.viewport3D, a.canvasBounds()
	projection, _ := v.world.Get[scene.Projection](v.view.camera)
	camera := preview3DCameraTransformAround(v.view.target, v.view.yaw, v.view.pitch, v.view.distance/a.zoom).LocalMatrix()
	global, _ := v.world.Get[scene.GlobalTransform](entity)
	point := projection.Matrix(bounds.W, bounds.H).Mul4(camera.Inv()).Mul4x1(global.Matrix.Col(3))
	x, y := bounds.X+(point[0]/point[3]+1)*bounds.W*.5, bounds.Y+(1-point[1]/point[3])*bounds.H*.5
	if point[3] <= 0 || !viewportContains(bounds, x, y) {
		t.Fatal("test node is outside the canvas", point)
	}
	return x, y
}

func TestWorkspaceCanvasSelectsFrontModuleAndRevealsTree(t *testing.T) {
	a := workspaceEditorFixture(t)
	if err := a.Document.MoveInstance("again", "start", 0); err != nil {
		t.Fatal(err)
	}
	if err := a.Document.EditInstance("again", "Overlap modules", func(value *project.ModInstanceSpec) error {
		value.Placement.Position = []float32{0, 0}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	a.selectInstance("start")
	a.collapsedScenes = map[project.SceneID]bool{a.Document.Active: true}
	a.collapsedModules = map[workspaceModuleKey]bool{{Scene: a.Document.Active, ID: "start"}: true}
	context := &ecs.SystemContext{World: ecs.NewWorld()}
	syncWorkspaceTest(t, a, context.World)
	revision, history := a.Document.Revision, len(a.Document.undo)
	bounds, frame := a.canvasBounds(), a.viewport2D.frame
	x, y := bounds.X+frame.X+60*a.canvasScale, bounds.Y+frame.Y+80*a.canvasScale
	a.pressViewport2D(x, y)
	if a.Selected != "again" || a.canvasDrag.ID != "again" || a.collapsedScenes[a.Document.Active] || a.collapsedModules[workspaceModuleKey{Scene: a.Document.Active, ID: "start"}] {
		t.Fatal("current selection blocked the front module or left its tree collapsed", a.Selected)
	}
	a.updatePlacementDrag(input.Pointer{X: x, Y: y, Released: true})
	syncWorkspaceTest(t, a, context.World)
	if _, ok := htmlui.ControlByID(context.World, a.shell, "select:again"); !ok {
		t.Fatal("canvas-selected node is not visible in the tree")
	}
	if a.Document.Revision != revision || len(a.Document.undo) != history {
		t.Fatal("canvas selection authored a movement")
	}
}

func TestWorkspaceCanvasSwitchesElementModuleAndBlankSelection(t *testing.T) {
	a, context := workspaceElementDragFixture(t)
	group := a.treeRowControlID(workspaceTreeRow{Scene: a.Document.Active, ID: "start", Group: "elements"})
	a.collapsedModules = map[workspaceModuleKey]bool{{Scene: a.Document.Active, ID: "start"}: true}
	a.sectionsCollapsed[group] = true
	syncWorkspaceTest(t, a, context.World)
	revision, history := a.Document.Revision, len(a.Document.undo)
	click := func(x, y float32, want string) {
		t.Helper()
		a.pressViewport2D(x, y)
		a.updatePlacementDrag(input.Pointer{X: x, Y: y, Released: true})
		syncWorkspaceTest(t, a, context.World)
		if a.selectedElement != want || a.canvasDrag.ID != "" {
			t.Fatal("canvas did not select the actual hit element", a.selectedElement, want)
		}
	}
	for _, id := range []string{"child", "leaf"} {
		entity := a.viewport2D.rows[0].targets[id]
		box, _ := a.viewport2DContentBounds(entity, a.viewport2DView(), false)
		canvas := a.canvasBounds()
		click(canvas.X+box.X+box.W/2, canvas.Y+box.Y+box.H/2, id)
		control := a.treeRowControlID(workspaceTreeRow{Scene: a.Document.Active, ID: "start", Element: id})
		if _, ok := htmlui.ControlByID(context.World, a.shell, control); !ok || a.sectionsCollapsed[group] {
			t.Fatal("canvas selection did not reveal the element in the tree", id)
		}
	}
	canvas, frame := a.canvasBounds(), a.viewport2D.frame
	click(canvas.X+frame.X+80*a.canvasScale, canvas.Y+frame.Y+20*a.canvasScale, "")
	if a.Selected != "start" {
		t.Fatal("clicking the module root did not select the module")
	}
	// This point is inside the module selection rectangle but between its visuals.
	click(canvas.X+frame.X+100*a.canvasScale, canvas.Y+frame.Y+80*a.canvasScale, "")
	if a.Selected != "" || a.Local != "button" {
		t.Fatal("empty canvas did not clear selection while retaining module editing")
	}
	if a.Document.Revision != revision || len(a.Document.undo) != history {
		t.Fatal("selection-only clicks changed authored content")
	}
	if allocs := testing.AllocsPerRun(100, func() { a.pickViewport2DTarget(canvas.X+frame.X+80*a.canvasScale, canvas.Y+frame.Y+20*a.canvasScale) }); allocs != 0 {
		t.Fatalf("2D picking allocated %g", allocs)
	}
}

func TestWorkspaceCanvasRotatedPickingRejectsBoundingBoxCorners(t *testing.T) {
	a := workspaceEditorFixture(t)
	if err := a.Document.EditMod("button", "Rotate", func(mod *project.ModSpec) error {
		mod.Transform.Rotation = []float32{0, 0, math.Pi / 4}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	context := &ecs.SystemContext{World: ecs.NewWorld()}
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	canvas := a.canvasBounds()
	box, _ := a.viewport2DContentBounds(a.viewport2D.rows[0].root, a.viewport2DView(), false)
	if id, _ := a.pickViewport2DTarget(canvas.X+box.X+a.canvasScale, canvas.Y+box.Y+a.canvasScale); id != "" {
		t.Fatal("rotated sprite's empty bounding-box corner remained selectable", id)
	}
}

func TestWorkspace3DCanvasSwitchesElementAndModuleSelection(t *testing.T) {
	a, context := workspace3DEditorFixture(t)
	a.enterModuleEdit(a.Document.Active, "model")
	syncWorkspaceTest(t, a, context.World)
	revision, history := a.Document.Revision, len(a.Document.undo)
	click := func(entity ecs.Entity, want string) {
		t.Helper()
		x, y := workspace3DCanvasPoint(t, a, entity)
		a.pressViewport3D(x, y)
		a.updatePlacementDrag(input.Pointer{X: x, Y: y, Released: true})
		syncWorkspaceTest(t, a, context.World)
		if a.Selected != "model" || a.selectedElement != want {
			t.Fatal("3D canvas did not switch to the actual mesh", a.Selected, a.selectedElement, want)
		}
	}
	row := &a.viewport3D.rows[0]
	click(row.parts[2].entity, "child")
	click(row.parts[0].entity, "")
	bounds := a.canvasBounds()
	a.pressViewport3D(bounds.X+10, bounds.Y+10)
	if a.Selected != "" || a.selectedElement != "" || a.Local != "model" {
		t.Fatal("3D empty canvas did not clear selection")
	}
	if a.Document.Revision != revision || len(a.Document.undo) != history {
		t.Fatal("3D selection authored a movement")
	}
	x, y := workspace3DCanvasPoint(t, a, row.parts[2].entity)
	if allocs := testing.AllocsPerRun(100, func() { a.pickViewport3DTarget(x, y) }); allocs != 0 {
		t.Fatalf("3D picking allocated %g", allocs)
	}
}
