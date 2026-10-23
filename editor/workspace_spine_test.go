package editor

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func workspaceSpineFixture(t *testing.T) *ModuleEditor {
	t.Helper()
	a := workspaceEditorFixture(t)
	document, err := LoadWorkspaceDocument(filepath.Join("..", "projects", "spine"))
	if err != nil {
		t.Fatal(err)
	}
	a.Document = document
	if err := a.sync(&ecs.SystemContext{World: ecs.NewWorld()}); err != nil {
		t.Fatal(err)
	}
	return a
}

type workspaceSpineRenderDriver struct {
	*viewport3DTestDriver
	points []mgl32.Vec2
}

func (d *workspaceSpineRenderDriver) DrawMeshes(batch render.MeshBatch3D) error {
	projection := mgl32.Mat4(batch.ViewProjection)
	for _, draw := range batch.Draws {
		model := mgl32.Mat4(batch.Instances[draw.FirstInstance].Model)
		clipFromLocal := projection.Mul4(model)
		for _, index := range batch.Indices[draw.FirstIndex : draw.FirstIndex+draw.IndexCount] {
			vertex := batch.Vertices[index]
			point := clipFromLocal.Mul4x1(mgl32.Vec4{vertex.Position[0], vertex.Position[1], vertex.Position[2], 1})
			d.points = append(d.points, mgl32.Vec2{(point[0]/point[3] + 1) * float32(d.viewport.Width) / 2, (1 - point[1]/point[3]) * float32(d.viewport.Height) / 2})
		}
	}
	return nil
}

func TestWorkspaceSpineModuleRendersAndPicksAllFourQuadrants(t *testing.T) {
	driver := &workspaceSpineRenderDriver{viewport3DTestDriver: &viewport3DTestDriver{viewport: render.Rect{Width: 1600, Height: 900}}}
	render.Uninstall()
	if err := render.Install(driver); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(render.Uninstall)
	canvas := graphics.NewCanvas2D(render.ActiveDevice(), 1600, 900)
	t.Cleanup(graphics.SetCurrentContextForTesting(canvas))
	a := workspaceSpineFixture(t)
	a.enterModuleEdit(a.Document.Active, "gallery")
	context := &ecs.SystemContext{World: a.world}
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	v := &a.viewport2D
	entity := v.rows[0].parts[0]
	a.selectElement("gallery", "coin")
	for _, zoom := range []float32{1, .5} {
		a.zoom = zoom
		v.offset = mgl32.Vec2{23, -11}
		a.layoutViewport2D()
		a.focusViewport2D()
		selection, ok := a.viewport2DContentBounds(entity, a.viewport2DView(), false)
		if !ok {
			t.Fatal("Coin has no selection bounds")
		}
		frame := func() {
			driver.points, driver.shapes, driver.clears = driver.points[:0], driver.shapes[:0], driver.clears[:0]
			if err := a.renderViewport2D(canvas); err != nil {
				panic(err)
			}
			if err := canvas.FinishFrame(); err != nil {
				panic(err)
			}
		}
		frame()
		if len(driver.points) == 0 {
			t.Fatal("module did not submit Spine geometry")
		}
		bounds := a.canvasBounds()
		var quadrants [4]bool
		origin := mgl32.Vec2{v.frame.X, v.frame.Y}
		for _, point := range driver.points {
			if point[0] < 0 || point[0] > bounds.W || point[1] < 0 || point[1] > bounds.H {
				t.Fatalf("zoom %g: Coin vertex %v was clipped by the render target", zoom, point)
			}
			if point[0] < selection.X-.001 || point[0] > selection.X+selection.W+.001 || point[1] < selection.Y-.001 || point[1] > selection.Y+selection.H+.001 {
				t.Fatalf("zoom %g: rendered Coin vertex %v disagrees with selection %+v", zoom, point, selection)
			}
			quadrant := 0
			if point[0] > origin[0] {
				quadrant++
			}
			if point[1] > origin[1] {
				quadrant += 2
			}
			quadrants[quadrant] = true
		}
		if quadrants != ([4]bool{true, true, true, true}) {
			t.Fatalf("Coin did not render in all four quadrants: %v", quadrants)
		}
		for _, direction := range []mgl32.Vec2{{-1, -1}, {1, -1}, {-1, 1}, {1, 1}} {
			x, y := origin[0]+direction[0]*selection.W/4, origin[1]+direction[1]*selection.H/4
			if id, element := a.pickViewport2DTarget(bounds.X+x, bounds.Y+y); id != "gallery" || element != "coin" {
				t.Fatalf("Coin quadrant %v was not pickable: %q/%q", direction, id, element)
			}
		}
		for range 4 {
			frame()
		}
		if allocations := testing.AllocsPerRun(100, frame); allocations != 0 {
			t.Fatalf("warm module Spine render allocated %g", allocations)
		}
	}
}

func TestWorkspaceSpinePreviewMatchesInitialGalleryLayout(t *testing.T) {
	a := workspaceSpineFixture(t)
	v := &a.viewport2D
	row := &v.rows[0]
	if len(row.parts) != 8 {
		t.Fatalf("Spine gallery has %d parts, want 8", len(row.parts))
	}
	for index, entity := range row.parts {
		player := ecs.GetRef[scene.Spine2D](v.world, entity).Player
		if viewportContentVisible(v.world, entity) != (index == 0) {
			t.Fatalf("Spine %d visibility differs from gallery startup", index)
		}
		minX, minY, maxX, maxY, ok := player.AnimationBounds()
		if !ok {
			t.Fatalf("Spine %d has no animation bounds", index)
		}
		transform := ecs.GetRef[scene.UITransform](v.world, entity)
		scale := min(900/(maxX-minX), 600/(maxY-minY), 1.6)
		if abs(transform.Scale[0]-scale) > 1e-6 || abs(transform.Scale[1]-scale) > 1e-6 ||
			abs(transform.Translation.X.Value+(minX+maxX)*scale*.5) > 1e-4 ||
			abs(transform.Translation.Y.Value-(minY+maxY)*scale*.5) > 1e-4 {
			t.Fatalf("Spine %d authored transform differs from runtime fit: %+v", index, transform)
		}
	}
	// Selection and visibility refreshes must preserve the declared hidden parts.
	a.Selected = "gallery"
	a.changed()
	if err := a.syncViewport2D(); err != nil {
		t.Fatal(err)
	}
	clock, _ := ecs.GetResource[*scene.Time](v.world)
	step := func() {
		clock.Set(10*time.Millisecond, clock.Elapsed+10*time.Millisecond, clock.Frame+1)
		if err := v.syncVisuals(true); err != nil {
			panic(err)
		}
	}
	step()
	for index, entity := range row.parts {
		player := ecs.GetRef[scene.Spine2D](v.world, entity).Player
		if index == 0 && player.AnimationTime() == 0 || index != 0 && player.AnimationTime() != 0 {
			t.Fatalf("Spine %d preview playback time = %g", index, player.AnimationTime())
		}
	}
	// An invisible ancestor also pauses preview playback.
	ecs.GetMut[scene.Visibility](v.world, row.root).Visible = false
	player := ecs.GetRef[scene.Spine2D](v.world, row.parts[0]).Player
	before := player.AnimationTime()
	step()
	if player.AnimationTime() != before {
		t.Fatal("hidden gallery continued preview playback")
	}
	ecs.GetMut[scene.Visibility](v.world, row.root).Visible = true
	for range 4 {
		step()
	}
	if allocations := testing.AllocsPerRun(100, step); allocations != 0 {
		t.Fatalf("warm Spine preview update allocated %g", allocations)
	}
}

func TestWorkspaceSpineBoundsAndPickingFollowRenderedGeometry(t *testing.T) {
	a := workspaceSpineFixture(t)
	v := &a.viewport2D
	entity := v.rows[0].parts[3]
	player := ecs.GetRef[scene.Spine2D](v.world, entity).Player
	player.Update(.25)
	minX, minY, maxX, maxY, valid := player.Bounds()
	if !valid {
		t.Fatal("Spineboy has no drawable bounds")
	}
	// A translated, rotated, nonuniformly scaled bone exercises both coordinate spaces.
	matrix := mgl32.Translate3D(400, 350, 0).Mul4(mgl32.HomogRotate3DZ(float32(math.Pi / 3))).Mul4(mgl32.Scale3D(.4, .6, 1))
	for _, ui := range []bool{true, false} {
		bounds, ok := viewportContentBounds(v.world, entity, matrix, ui)
		if !ok {
			t.Fatal("Spine content has no editor bounds")
		}
		// Compare with transformed mesh vertices, independent of the bounds helper.
		batch := player.MeshBatch(mgl32.Ident4(), scene.SpineModelMatrix2D(matrix, ui))
		model := mgl32.Mat4(batch.Instances[0].Model)
		for _, vertex := range batch.Vertices {
			point := model.Mul4x1(mgl32.Vec4{vertex.Position[0], vertex.Position[1], vertex.Position[2], 1})
			if point[0] < bounds.X-.001 || point[0] > bounds.X+bounds.W+.001 || point[1] < bounds.Y-.001 || point[1] > bounds.Y+bounds.H+.001 {
				t.Fatalf("ui=%v: rendered Spine vertex %v outside editor bounds %+v", ui, point, bounds)
			}
		}
		if bounds.W <= 0 || bounds.H <= 0 {
			t.Fatal("Spine selection collapsed to a point")
		}
	}
	ecs.GetMut[scene.Visibility](v.world, entity).Visible = true
	ecs.GetMut[scene.UIGlobalTransform](v.world, entity).Matrix = matrix
	center := scene.SpineModelMatrix2D(matrix, true).Mul4x1(mgl32.Vec4{(minX + maxX) / 2, (minY + maxY) / 2, 0, 1})
	if !v.hitContent(entity, center[0], center[1]) || v.hitContent(entity, center[0]+2000, center[1]) {
		t.Fatal("Spine picking does not follow transformed bounds")
	}
	a.zoom, v.offset = 2, mgl32.Vec2{31, -47}
	a.layoutViewport2D()
	bounds, ok := a.viewport2DContentBounds(entity, a.viewport2DView(), false)
	point := mgl32.Vec2{v.frame.X + center[0]*a.canvasScale, v.frame.Y + center[1]*a.canvasScale}
	if !ok || !viewportContains(bounds, point[0], point[1]) {
		t.Fatal("zoomed and panned Spine selection lost its rendered center")
	}
	step := func() {
		if _, ok := a.viewport2DContentBounds(entity, a.viewport2DView(), false); !ok || !v.hitContent(entity, center[0], center[1]) {
			panic("Spine bounds or picking lost geometry")
		}
	}
	if allocations := testing.AllocsPerRun(100, step); allocations != 0 {
		t.Fatalf("warm Spine bounds and picking allocated %g", allocations)
	}
}
