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
	"github.com/go-gl/mathgl/mgl32"
)

func workspace3DHelpersFixture(t *testing.T, components project.ModComponents, elements []project.ModElementSpec) (*ModuleEditor, *ecs.SystemContext) {
	t.Helper()
	a := workspaceEditorFixture(t)
	if err := a.Document.CreateMod("helpers", project.ModSpec{Space: project.CoordinateSpaceWorld, Components: components, Elements: elements}); err != nil {
		t.Fatal(err)
	}
	if err := a.Document.AddInstance("helpers", "helpers", ""); err != nil {
		t.Fatal(err)
	}
	context := &ecs.SystemContext{World: ecs.NewWorld()}
	a.action("toggle-dimension")
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if a.viewport3D.err != nil {
		t.Fatal(a.viewport3D.err)
	}
	return a, context
}

func TestWorkspace3DHelpersDrawPickAndMoveWithoutMeshes(t *testing.T) {
	driver := &viewport3DTestDriver{viewport: render.Rect{Width: 1600, Height: 900}}
	render.Uninstall()
	if err := render.Install(driver); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(render.Uninstall)
	canvas := graphics.NewCanvas2D(render.ActiveDevice(), 1600, 900)
	camera, ambient, sun := project.NewCamera3DSpec(), project.NewAmbientLightSpec(), project.NewDirectionalLightSpec()
	point, spot, area := project.NewPointLightSpec(), project.NewSpotLightSpec(), project.NewAreaLightSpec()
	point.Radius = .4
	pyramid, box := spot, spot
	pyramid.Shape, box.Shape = render3d.SpotPyramid, render3d.SpotBox
	box.Unit = graphics.LightLux
	disc, tube := area, area
	disc.Shape, tube.Shape = render3d.AreaDisc, render3d.AreaTube
	for _, test := range []struct {
		name       string
		components project.ModComponents
	}{
		{"camera", project.ModComponents{Camera: &camera}},
		{"ambient", project.ModComponents{AmbientLight: &ambient}},
		{"sun", project.ModComponents{DirectionalLight: &sun}},
		{"point", project.ModComponents{PointLight: &point}},
		{"spot", project.ModComponents{SpotLight: &spot}},
		{"pyramid", project.ModComponents{SpotLight: &pyramid}},
		{"box", project.ModComponents{SpotLight: &box}},
		{"rectangle", project.ModComponents{AreaLight: &area}},
		{"disc", project.ModComponents{AreaLight: &disc}},
		{"tube", project.ModComponents{AreaLight: &tube}},
	} {
		t.Run(test.name, func(t *testing.T) {
			a, context := workspace3DHelpersFixture(t, test.components, nil)
			v := &a.viewport3D
			index := len(v.rows) - 1
			entity := v.rows[index].parts[0].entity
			if entity == 0 || v.world.Has[render3d.Mesh](entity) || v.world.Has[scene.Camera](entity) {
				t.Fatal("helper needs a transform entity without a mesh or a running camera")
			}
			if test.name == "camera" && len(v.mods) != 0 {
				t.Fatal("camera transform initialized model resources")
			}
			a.selectInstance("helpers")
			a.focusViewport3D()
			target, distance := v.view.target, v.view.distance
			a.zoom = .25
			a.focusViewport3D()
			if v.view.target != target || v.view.distance != distance {
				t.Fatal("repeated framing depended on the screen icon's size")
			}
			v.view.target = v.world.GetRef[scene.GlobalTransform](entity).Matrix.Col(3).Vec3()
			v.view.distance, v.view.yaw, v.view.pitch = 12, -.4, .2
			for _, orthographic := range []bool{false, true} {
				v.view.orthographic = orthographic
				for _, zoom := range []float32{.25, 1, 4} {
					a.zoom = zoom
					a.syncViewport3DProjection()
					x, y := workspace3DCanvasPoint(t, a, entity)
					if id := a.pickViewport3D(x, y); id != "helpers" {
						t.Fatal("source origin was not selectable", orthographic, zoom, id)
					}
					projection := v.world.GetRef[scene.Projection](v.view.camera)
					view := preview3DCameraTransformAround(v.view.target, v.view.yaw, v.view.pitch, v.view.distance/a.zoom).LocalMatrix()
					driver.shapes = driver.shapes[:0]
					a.drawViewport3DHelpers(canvas, projection, view)
					if err := canvas.FinishFrame(); err != nil {
						t.Fatal(err)
					}
					color := preview3DHelperColor(true)
					if len(driver.shapes) < 3 {
						t.Fatal("helper did not emit visible wire strokes")
					}
					for _, shape := range driver.shapes {
						if shape.Fill != ([4]float32{color.R, color.G, color.B, color.A}) {
							t.Fatal("selected helper did not use the orange outline")
						}
					}
				}
			}
			x, y := workspace3DCanvasPoint(t, a, entity)
			original := v.world.GetRef[scene.GlobalTransform](entity).Matrix
			revision, history, modRevision := a.Document.Revision, len(a.Document.undo), a.Document.Mods["helpers"].Revision
			a.pressViewport3D(x, y)
			a.updatePlacementDrag(input.Pointer{X: x + 30, Y: y + 20, Down: true})
			if err := a.sync(context); err != nil {
				t.Fatal(err)
			}
			if v.world.GetRef[scene.GlobalTransform](entity).Matrix == original || a.Document.Revision != revision {
				t.Fatal("helper drag did not preview without editing the document")
			}
			a.updatePlacementDrag(input.Pointer{X: x + 30, Y: y + 20, Released: true})
			if len(a.Document.undo) != history+1 || a.Document.Mods["helpers"].Revision != modRevision {
				t.Fatal("helper drag did not commit one instance placement edit")
			}
			if err := a.Document.Undo(); err != nil {
				t.Fatal(err)
			}
			a.changed()
			if err := a.sync(context); err != nil {
				t.Fatal(err)
			}
			if v.world.GetRef[scene.GlobalTransform](entity).Matrix != original {
				t.Fatal("undo did not restore the helper transform")
			}
			v.world.GetMut[scene.Visibility](entity).Visible = false
			if id := a.pickViewport3D(x, y); id != "" {
				t.Fatal("hidden helper remained selectable", id)
			}
			projection := v.world.GetRef[scene.Projection](v.view.camera)
			view := preview3DCameraTransformAround(v.view.target, v.view.yaw, v.view.pitch, v.view.distance/a.zoom).LocalMatrix()
			driver.shapes = driver.shapes[:0]
			a.drawViewport3DHelpers(canvas, projection, view)
			if err := canvas.FinishFrame(); err != nil {
				t.Fatal(err)
			}
			if len(driver.shapes) != 0 {
				t.Fatal("hidden helper remained visible")
			}
		})
	}
}

func TestWorkspace3DHelperElementsDragWithTransformedParent(t *testing.T) {
	camera, light := project.NewCamera3DSpec(), project.NewPointLightSpec()
	a, context := workspace3DHelpersFixture(t, project.ModComponents{}, []project.ModElementSpec{
		{ID: "parent", Transform: project.ModTransformSpec{Position: []float32{1, 2, 3}, Rotation: []float32{.2, .4, .6}, Scale: []float32{2, 3, 4}}},
		{ID: "camera", Parent: "parent", Transform: project.ModTransformSpec{Position: []float32{-3, 0, 0}}, Components: project.ModComponents{Camera: &camera}},
		{ID: "light", Parent: "parent", Transform: project.ModTransformSpec{Position: []float32{3, 0, 0}}, Components: project.ModComponents{PointLight: &light}},
	})
	a.enterModuleEdit(a.Document.Active, "helpers")
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	for i, element := range []string{"camera", "light"} {
		entity := a.viewport3D.rows[0].parts[i+2].entity
		x, y := workspace3DCanvasPoint(t, a, entity)
		original := a.viewport3D.world.GetRef[scene.GlobalTransform](entity).Matrix
		sibling := a.viewport3D.rows[0].parts[3-i].entity
		siblingMatrix := a.viewport3D.world.GetRef[scene.GlobalTransform](sibling).Matrix
		history, revision, sceneRevision := len(a.Document.undo), a.Document.Revision, a.Document.Scene().Revision
		a.pressViewport3D(x, y)
		if a.canvasDrag.Element != element || !a.canvasDrag.Definition {
			t.Fatal("local helper did not select its own element", a.canvasDrag)
		}
		a.updatePlacementDrag(input.Pointer{X: x + 25, Y: y + 15, Down: true})
		if err := a.sync(context); err != nil {
			t.Fatal(err)
		}
		got := a.viewport3D.world.GetRef[scene.GlobalTransform](entity).Matrix.Col(3).Vec3().Sub(original.Col(3).Vec3())
		if got.Sub(a.canvasDrag.WorldDelta).Len() > .001 || got.Len() == 0 || a.Document.Revision != revision || a.viewport3D.world.GetRef[scene.GlobalTransform](sibling).Matrix != siblingMatrix {
			t.Fatal("local helper movement lost parent transforms or moved its sibling")
		}
		a.updatePlacementDrag(input.Pointer{X: x + 25, Y: y + 15, Released: true})
		if len(a.Document.undo) != history+1 || a.Document.Scene().Revision != sceneRevision {
			t.Fatal("element drag did not isolate one module definition edit")
		}
		if err := a.Document.Undo(); err != nil {
			t.Fatal(err)
		}
		a.changed()
		if err := a.sync(context); err != nil {
			t.Fatal(err)
		}
		entity = a.viewport3D.rows[0].parts[i+2].entity
		if a.viewport3D.world.GetRef[scene.GlobalTransform](entity).Matrix != original {
			t.Fatal("undo lost the element's original transform")
		}
	}
	if err := a.Document.EditMod("helpers", "Hide parent", func(mod *project.ModSpec) error {
		hidden := false
		mod.Elements[0].Visible = &hidden
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	a.changed()
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	for _, part := range a.viewport3D.rows[0].parts[2:] {
		if a.viewport3D.world.GetRef[scene.Visibility](part.entity).Visible {
			t.Fatal("helper ignored its hidden parent")
		}
	}
}

func TestPreview3DHelperClippingAndStrokePicking(t *testing.T) {
	viewport := rect{X: 20, Y: 30, W: 640, H: 480}
	projection := scene.PerspectiveProjection3D()
	clip := projection.Matrix(viewport.W, viewport.H)
	start, end, ok := preview3DProjectHelperLine(clip, viewport, [2]mgl32.Vec3{{0, 0, 1}, {0, 0, -2}})
	if !ok || abs(start.Z()-1) > .00001 || end.Z() <= 0 || start.X() != viewport.X+viewport.W*.5 {
		t.Fatal("near-plane crossing was not clipped in reverse Z", start, end, ok)
	}
	for _, line := range [][2]mgl32.Vec3{{{0, 0, 1}, {1, 0, 2}}, {{100, 0, -1}, {100, 1, -2}}, {{0, 0, -.01}, {1, 0, -.02}}} {
		if _, _, ok := preview3DProjectHelperLine(clip, viewport, line); ok {
			t.Fatal("invisible helper segment survived clipping", line)
		}
	}
	var geometry preview3DHelperGeometry
	geometry.line(mgl32.Vec3{-.2, 0, -2}, mgl32.Vec3{.2, 0, -2})
	point := mgl32.Vec2{viewport.X + viewport.W*.5, viewport.Y + viewport.H*.5}
	if _, hit := geometry.hit(clip, viewport, point.Add(mgl32.Vec2{0, 5})); !hit {
		t.Fatal("visible wire lost its pixel picking tolerance")
	}
	if _, hit := geometry.hit(clip, viewport, point.Add(mgl32.Vec2{0, 8})); hit {
		t.Fatal("empty space around a wire was selectable")
	}
}

func TestWorkspace3DHelperRenderingAndPickingDoNotAllocate(t *testing.T) {
	driver := &viewport3DTestDriver{viewport: render.Rect{Width: 1600, Height: 900}}
	render.Uninstall()
	if err := render.Install(driver); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(render.Uninstall)
	canvas := graphics.NewCanvas2D(render.ActiveDevice(), 1600, 900)
	camera, ambient, sun := project.NewCamera3DSpec(), project.NewAmbientLightSpec(), project.NewDirectionalLightSpec()
	point, spot, area := project.NewPointLightSpec(), project.NewSpotLightSpec(), project.NewAreaLightSpec()
	point.Radius, area.Shape = .5, render3d.AreaDisc
	a, _ := workspace3DHelpersFixture(t, project.ModComponents{Camera: &camera, AmbientLight: &ambient, DirectionalLight: &sun, PointLight: &point, SpotLight: &spot, AreaLight: &area}, nil)
	a.selectInstance("helpers")
	a.focusViewport3D()
	a.viewport3D.view.yaw, a.viewport3D.view.pitch = math.Pi/4, .3
	x, y := workspace3DCanvasPoint(t, a, a.viewport3D.rows[len(a.viewport3D.rows)-1].parts[0].entity)
	projection := a.viewport3D.world.GetRef[scene.Projection](a.viewport3D.view.camera)
	view := preview3DCameraTransformAround(a.viewport3D.view.target, a.viewport3D.view.yaw, a.viewport3D.view.pitch, a.viewport3D.view.distance/a.zoom).LocalMatrix()
	frame := func() {
		driver.shapes = driver.shapes[:0]
		a.drawViewport3DHelpers(canvas, projection, view)
		a.viewport3DSelectionBounds(len(a.viewport3D.rows) - 1)
		a.pickViewport3D(x, y)
		if err := canvas.FinishFrame(); err != nil {
			panic(err)
		}
	}
	frame()
	if allocations := testing.AllocsPerRun(30, frame); allocations != 0 {
		t.Fatalf("stable helper drawing, bounds and picking allocated %g", allocations)
	}
}
