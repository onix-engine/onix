package editor

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	htmlui "github.com/diiyw/onix/ui/html"
	"github.com/go-gl/mathgl/mgl32"
)

func TestWorkspace2DModuleOpeningCentersVisibleContent(t *testing.T) {
	a := workspaceEditorFixture(t)
	if err := a.Document.EditMod("button", "Offset module content", func(mod *project.ModSpec) error {
		mod.Transform.Position = []float32{25, 35}
		mod.Elements = []project.ModElementSpec{
			{ID: "panel", Transform: project.ModTransformSpec{Position: []float32{150, 70}, Rotation: []float32{0, 0, .3}}, Components: project.ModComponents{Sprite: &project.ModSpriteSpec{Color: "#ffffff", Size: []float32{80, 60}}}},
			{ID: "caption", Parent: "panel", Transform: project.ModTransformSpec{Position: []float32{60, 30}}, Components: project.ModComponents{Text: &project.TextComponentSpec{Value: "Score", FontSize: 24}}},
			{ID: "hidden", Visible: new(false), Transform: project.ModTransformSpec{Position: []float32{10000, 10000}}, Components: project.ModComponents{Sprite: &project.ModSpriteSpec{Color: "#ffffff", Size: []float32{100, 100}}}},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	context := &ecs.SystemContext{World: ecs.NewWorld()}
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	revision := a.Document.Revision
	a.viewport2D.offset, a.zoom = mgl32.Vec2{120, -80}, 2
	a.enterModuleEdit(a.Document.Active, "start")
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	assertCentered := func() {
		t.Helper()
		var content rect
		found := false
		view := a.viewport2DView()
		row := &a.viewport2D.rows[0]
		for _, entity := range []ecs.Entity{row.root, row.parts[0], row.parts[1]} {
			bounds, ok := a.viewport2DContentBounds(entity, view, false)
			if !ok {
				t.Fatal("visible module content has no bounds")
			}
			viewportUnionBounds(&content, &found, bounds)
		}
		canvas := a.canvasBounds()
		if abs(content.X+content.W/2-canvas.W/2) > .001 || abs(content.Y+content.H/2-canvas.H/2) > .001 {
			t.Fatalf("module content is not centered: content %+v, canvas %+v", content, canvas)
		}
		if a.Document.Revision != revision || a.zoom != 1 {
			t.Fatal("opening the module changed authored data or kept the previous zoom")
		}
	}
	assertCentered()
	if a.viewport2D.focusPending {
		t.Fatal("module centering was not consumed")
	}
	if allocations := testing.AllocsPerRun(100, a.focusViewport2D); allocations != 0 {
		t.Fatalf("module centering allocated %g", allocations)
	}
	a.viewport2D.offset = a.viewport2D.offset.Add(mgl32.Vec2{30, 20})
	a.zoom = 1.5
	a.changed()
	offset := a.viewport2D.offset
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if a.viewport2D.offset != offset || a.zoom != 1.5 {
		t.Fatal("module refresh reset manual pan or zoom")
	}
	a.enterModuleEdit(a.Document.Active, "again")
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	assertCentered()
	a.action("focus")
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	assertCentered()
}

func TestWorkspace2DFocusFitsOversizedModuleContent(t *testing.T) {
	for _, space := range []project.CoordinateSpace{project.CoordinateSpaceUI, project.CoordinateSpaceWorld} {
		t.Run(string(space), func(t *testing.T) {
			a := workspaceEditorFixture(t)
			if err := a.Document.EditMod("button", "Oversized module", func(mod *project.ModSpec) error {
				mod.Space, mod.Components = space, project.ModComponents{}
				mod.Elements = []project.ModElementSpec{
					{ID: "content", Transform: project.ModTransformSpec{Position: []float32{1100, 800}, Rotation: []float32{0, 0, .3}}, Components: project.ModComponents{Sprite: &project.ModSpriteSpec{Color: "#ffffff", Size: []float32{1680, 1900}}}},
					{ID: "hidden", Visible: new(false), Transform: project.ModTransformSpec{Position: []float32{10000, 10000}}, Components: project.ModComponents{Sprite: &project.ModSpriteSpec{Color: "#ffffff", Size: []float32{100, 100}}}},
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			a.enterModuleEdit(a.Document.Active, "start")
			context := &ecs.SystemContext{World: ecs.NewWorld()}
			if err := a.sync(context); err != nil {
				t.Fatal(err)
			}
			revision := a.Document.Revision
			assertFocused := func() {
				t.Helper()
				canvas := a.canvasBounds()
				content, ok := a.viewport2DContentBounds(a.viewport2D.rows[0].parts[0], a.viewport2DView(), false)
				if !ok || abs(content.X+content.W/2-canvas.W/2) > .001 || abs(content.Y+content.H/2-canvas.H/2) > .001 {
					t.Fatalf("oversized module is not centered: content %+v, canvas %+v", content, canvas)
				}
				if content.X < 12-.001 || content.Y < 20-.001 || content.X+content.W > canvas.W-12+.001 || content.Y+content.H > canvas.H-20+.001 {
					t.Fatalf("focused module is clipped: content %+v, canvas %+v", content, canvas)
				}
				if a.zoom >= 1 || a.Document.Revision != revision || a.viewport2D.focusPending {
					t.Fatal("focus did not fit the view without changing authored data")
				}
			}
			assertFocused()
			a.viewport2D.offset, a.viewport2D.panning, a.zoom = mgl32.Vec2{500, -300}, true, 3
			a.handleEvent(htmlui.Event{ID: "top-frame", Kind: htmlui.EventClick})
			if err := a.sync(context); err != nil {
				t.Fatal(err)
			}
			assertFocused()
			if a.viewport2D.panning {
				t.Fatal("focus did not stop panning")
			}
			if allocations := testing.AllocsPerRun(100, func() {
				a.zoom, a.viewport2D.offset = 1, mgl32.Vec2{}
				a.layoutViewport2D()
				a.focusViewport2D()
			}); allocations != 0 {
				t.Fatalf("fitting module content allocated %g", allocations)
			}
			assertFocused()
		})
	}
}

func TestWorkspace2DModuleContentOutsideNominalViewportRendersAndPicks(t *testing.T) {
	for _, space := range []project.CoordinateSpace{project.CoordinateSpaceUI, project.CoordinateSpaceWorld} {
		t.Run(string(space), func(t *testing.T) {
			driver := &viewport3DTestDriver{viewport: render.Rect{Width: 1600, Height: 900}}
			render.Uninstall()
			if err := render.Install(driver); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(render.Uninstall)
			canvas := graphics.NewCanvas2D(render.ActiveDevice(), 1600, 900)
			t.Cleanup(graphics.SetCurrentContextForTesting(canvas))
			a := workspaceEditorFixture(t)
			if err := a.Document.EditMod("button", "Content on every side of the origin", func(mod *project.ModSpec) error {
				mod.Space, mod.Components = space, project.ModComponents{}
				mod.Elements = []project.ModElementSpec{
					{ID: "top-left", Transform: project.ModTransformSpec{Position: []float32{-800, -600}}},
					{ID: "top-right", Transform: project.ModTransformSpec{Position: []float32{800, -600}}},
					{ID: "bottom-left", Transform: project.ModTransformSpec{Position: []float32{-800, 600}}},
					{ID: "bottom-right", Transform: project.ModTransformSpec{Position: []float32{800, 600}}},
				}
				for i := range mod.Elements {
					mod.Elements[i].Components.Sprite = &project.ModSpriteSpec{Color: "#ff0000", Size: []float32{40, 40}}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			a.enterModuleEdit(a.Document.Active, "start")
			if err := a.sync(&ecs.SystemContext{World: ecs.NewWorld()}); err != nil {
				t.Fatal(err)
			}
			a.zoom = .2
			a.layoutViewport2D()
			a.focusViewport2D()
			if err := a.renderViewport2D(canvas); err != nil {
				t.Fatal(err)
			}
			if err := canvas.FinishFrame(); err != nil {
				t.Fatal(err)
			}
			count := 0
			bounds := a.canvasBounds()
			for _, shape := range driver.shapes {
				if shape.Fill != ([4]float32{1, 0, 0, 1}) {
					continue
				}
				count++
				x, y := shape.Model[2]+shape.Model[0]/2, shape.Model[5]+shape.Model[4]/2
				if x < 0 || x > bounds.W || y < 0 || y > bounds.H {
					t.Fatalf("module content was rendered outside the canvas: %g,%g", x, y)
				}
				if id, element := a.pickViewport2DTarget(bounds.X+x, bounds.Y+y); id != "start" || element == "" {
					t.Fatalf("rendered content outside nominal viewport was not pickable: %q/%q", id, element)
				}
			}
			if count != 4 {
				t.Fatalf("module rendered %d quadrants, want 4", count)
			}
			if id, _ := a.pickViewport2DTarget(bounds.X-1, bounds.Y+bounds.H/2); id != "" {
				t.Fatal("module picking escaped the canvas")
			}
		})
	}
}

func TestWorkspace2DRenderSceneHTMLGridAndAxes(t *testing.T) {
	driver := &viewport3DTestDriver{viewport: render.Rect{Width: 1600, Height: 960}}
	render.Uninstall()
	if err := render.Install(driver); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(render.Uninstall)
	canvas := graphics.NewCanvas2D(render.ActiveDevice(), 1600, 960)
	t.Cleanup(graphics.SetCurrentContextForTesting(canvas))
	a := workspaceEditorFixture(t)
	if err := a.Document.CreateMod("panel", project.ModSpec{Space: project.CoordinateSpaceUI}); err != nil {
		t.Fatal(err)
	}
	if err := writeWorkspaceFile(filepath.Join(a.Document.ProjectDir, "panel/resources/view.html"), []byte(`<style>@keyframes slide {from {transform:translateX(0px)} to {transform:translateX(20px)}} #live {animation:slide 1s linear infinite}</style><div id="live" style="width:80px;height:40px;background:#ff0000"></div>`)); err != nil {
		t.Fatal(err)
	}
	if err := a.Document.EditMod("panel", "HTML", func(mod *project.ModSpec) error {
		mod.Resources = map[string]project.ModResourceSpec{"view": {Kind: "html", Path: "resources/view.html"}}
		mod.Components = project.ModComponents{Size: []float32{80, 40}, HTML: &project.ModHTMLSpec{Resource: "view"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.Document.AddInstance("panel", "panel", ""); err != nil {
		t.Fatal(err)
	}
	a.changed()
	context := &ecs.SystemContext{World: ecs.NewWorld()}
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if a.viewport2D.err != nil {
		t.Fatal(a.viewport2D.err)
	}
	wakes := 0
	a.requestWake = func(time.Time) { wakes++ }
	if err := a.render(context); err != nil {
		t.Fatal(err)
	}
	if err := canvas.FinishFrame(); err != nil {
		t.Fatal(err)
	}
	if wakes == 0 || !a.viewport2D.cssAnimated {
		t.Fatal("CSS animation did not schedule a frame for the on-demand editor")
	}
	if len(driver.clears) != 2 {
		t.Fatalf("scene and editor canvas did not render to separate targets: %+v", driver.clears)
	}
	if driver.viewport != (render.Rect{Width: 1600, Height: 960}) {
		t.Fatal("viewport was not restored", driver.viewport)
	}
	w, h := canvas.Size()
	if w != 1600 || h != 960 {
		t.Fatal("logical canvas was not restored", w, h)
	}
	foundSprite, foundHTML, foundX, foundY := false, false, false, false
	foundMajor, foundMinor, foundOrigin := false, false, false
	blue, _ := project.ParseColor("#3264d9", 1)
	for _, shape := range driver.shapes {
		if shape.Fill == ([4]float32{blue.R, blue.G, blue.B, blue.A}) && shape.Model[2] == 45 && shape.Model[5] == 67 {
			foundSprite = true
		}
		if shape.Fill == ([4]float32{1, 0, 0, 1}) {
			foundHTML = true
		}
		if shape.Fill == ([4]float32{editorTheme.axisX.R, editorTheme.axisX.G, editorTheme.axisX.B, editorTheme.axisX.A}) {
			foundX = true
			if abs(shape.Model[0]-a.canvasBounds().W) > .01 || abs(shape.Model[4]-2) > .01 {
				t.Fatal("X axis geometry does not span the canvas", shape.Model)
			}
		}
		if shape.Fill == ([4]float32{editorTheme.axisY.R, editorTheme.axisY.G, editorTheme.axisY.B, editorTheme.axisY.A}) {
			foundY = true
			if abs(shape.Model[0]-2) > .01 || abs(shape.Model[4]-a.canvasBounds().H) > .01 {
				t.Fatal("Y axis geometry does not span the canvas", shape.Model)
			}
		}
		for i, color := range [...]graphics.ColorRGBA{editorTheme.gridMajor, editorTheme.gridMinor, editorTheme.axisOrigin} {
			if shape.Fill == ([4]float32{color.R, color.G, color.B, color.A}) {
				switch i {
				case 0:
					foundMajor = true
				case 1:
					foundMinor = true
				case 2:
					foundOrigin = true
				}
			}
		}
	}
	if !foundSprite || !foundHTML || !foundX || !foundY {
		t.Fatalf("missing runtime visuals or guides: sprite=%v HTML=%v X=%v Y=%v", foundSprite, foundHTML, foundX, foundY)
	}
	if !foundMajor || !foundMinor || !foundOrigin {
		t.Fatalf("missing legacy guides: major=%v minor=%v origin=%v", foundMajor, foundMinor, foundOrigin)
	}
	v := &a.viewport2D
	if id := a.pickViewport2D(a.canvasBounds().X+v.frame.X+60*a.canvasScale, a.canvasBounds().Y+v.frame.Y+80*a.canvasScale); id != "start" {
		t.Fatal("sprite picking did not match rendered placement", id)
	}
	oldWorld, oldSource := v.world, v.imageName
	a.disposeCanvasResources()
	a.changed()
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if v.imageName == oldSource {
		t.Fatal("rebuild reused a cached canvas image source")
	}
	if v.world == oldWorld {
		t.Fatal("rebuild retained old world")
	}
	if err := htmlui.Sync(context.World); err != nil {
		t.Fatal(err)
	}
	foundCurrent := false
	for _, access := range ecs.Query[scene.Sprite2D](context.World).All {
		if access.Ref().Texture == v.target.colorTexture {
			foundCurrent = true
			break
		}
	}
	if !foundCurrent {
		t.Fatal("canvas HTML did not bind the rebuilt scene texture")
	}
}

func TestWorkspace2DMiddleDragPansViewAndPreservesPicking(t *testing.T) {
	a := workspaceEditorFixture(t)
	context := &ecs.SystemContext{World: ecs.NewWorld()}
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	in := input.NewState()
	ecs.InsertResource(context.World, in)
	bounds, frame := a.canvasBounds(), a.viewport2D.frame
	x, y := float64(bounds.X+frame.X+60*a.canvasScale), float64(bounds.Y+frame.Y+80*a.canvasScale)
	revision, selected, scale := a.Document.Revision, a.Selected, a.canvasScale
	root := a.viewport2D.rows[0].root
	in.PressMouse(input.MouseMiddle, x, y)
	if err := a.update(context); err != nil {
		t.Fatal(err)
	}
	in.EndFrame()
	in.MoveMouse(x+30, y+10)
	if err := a.update(context); err != nil {
		t.Fatal(err)
	}
	if !a.canvasDirty || a.dirty {
		t.Fatal("pan did not update only the canvas")
	}
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if a.viewport2D.offset != (mgl32.Vec2{30, 10}) || abs(a.viewport2D.frame.X-frame.X-30) > .001 || abs(a.viewport2D.frame.Y-frame.Y-10) > .001 {
		t.Fatal("middle drag did not move the view by the pointer delta", a.viewport2D.frame)
	}
	if a.Document.Revision != revision || a.Selected != selected || a.canvasDrag.ID != "" || a.viewport2D.rows[0].root != root || a.canvasScale != scale {
		t.Fatal("pan changed authored content, selection, or zoom")
	}
	if id := a.pickViewport2D(float32(x+30), float32(y+10)); id != "start" {
		t.Fatal("pan broke picking", id)
	}
	in.EndFrame()
	in.MoveMouse(float64(bounds.X+bounds.W+30), y+10)
	a.updateViewport2DInput(in)
	if !a.viewport2D.panning || a.viewport2D.offset[0] <= 30 {
		t.Fatal("pan lost capture when leaving the canvas")
	}
	in.ReleaseMouse(input.MouseMiddle, float64(bounds.X+bounds.W+30), y+10)
	a.updateViewport2DInput(in)
	offset := a.viewport2D.offset
	in.EndFrame()
	in.MoveMouse(x, y)
	a.updateViewport2DInput(in)
	if a.viewport2D.panning || a.viewport2D.offset != offset {
		t.Fatal("view kept moving after middle button release")
	}
	a.action("focus")
	if a.viewport2D.offset != (mgl32.Vec2{}) || a.viewport2D.panning {
		t.Fatal("focus did not reset panning")
	}
	// Pressing outside the canvas must not start panning when entering it.
	in.EndFrame()
	in.PressMouse(input.MouseMiddle, float64(bounds.X-10), y)
	a.updateViewport2DInput(in)
	in.EndFrame()
	in.MoveMouse(x, y)
	a.updateViewport2DInput(in)
	if a.viewport2D.panning || a.viewport2D.offset != (mgl32.Vec2{}) {
		t.Fatal("middle press outside the canvas started panning")
	}
	in.ReleaseMouse(input.MouseMiddle, x, y)
	in.EndFrame()
	in.PressMouse(input.MouseMiddle, x, y)
	a.updateViewport2DInput(in)
	step := func() {
		in.EndFrame()
		in.MoveMouse(x+10, y+10)
		a.updateViewport2DInput(in)
		if err := a.sync(context); err != nil {
			panic(err)
		}
		in.EndFrame()
		in.MoveMouse(x, y)
		a.updateViewport2DInput(in)
		if err := a.sync(context); err != nil {
			panic(err)
		}
	}
	if allocations := testing.AllocsPerRun(100, step); allocations != 0 {
		t.Fatalf("middle drag and canvas sync allocated %g", allocations)
	}
	a.menuOpen = "file"
	in.EndFrame()
	in.MoveMouse(x+20, y+20)
	offset = a.viewport2D.offset
	a.updateViewport2DInput(in)
	if a.viewport2D.panning || a.viewport2D.offset != offset {
		t.Fatal("menu did not cancel panning")
	}
}

func TestWorkspace2DGridGeometryRemainsVisibleAcrossZoom(t *testing.T) {
	driver := &viewport3DTestDriver{viewport: render.Rect{Width: 1600, Height: 960}}
	render.Uninstall()
	if err := render.Install(driver); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(render.Uninstall)
	canvas := graphics.NewCanvas2D(render.ActiveDevice(), 1600, 960)
	t.Cleanup(graphics.SetCurrentContextForTesting(canvas))
	a := workspaceEditorFixture(t)
	context := &ecs.SystemContext{World: ecs.NewWorld()}
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	for _, zoom := range [...]float32{.05, 1, 8} {
		a.zoom = zoom
		a.viewport2D.offset = mgl32.Vec2{37, -19}
		if err := a.syncViewport2D(); err != nil {
			t.Fatal(err)
		}
		driver.shapes = driver.shapes[:0]
		if err := a.render(context); err != nil {
			t.Fatal(err)
		}
		if err := canvas.FinishFrame(); err != nil {
			t.Fatal(err)
		}
		var found [2][2]bool
		bounds := a.canvasBounds()
		for _, shape := range driver.shapes {
			for level, color := range [...]graphics.ColorRGBA{editorTheme.gridMinor, editorTheme.gridMajor} {
				if shape.Fill != ([4]float32{color.R, color.G, color.B, color.A}) {
					continue
				}
				if abs(shape.Model[0]-1) < .01 && abs(shape.Model[4]-bounds.H) < .01 {
					found[level][0] = true
				} else if abs(shape.Model[0]-bounds.W) < .01 && abs(shape.Model[4]-1) < .01 {
					found[level][1] = true
				} else {
					t.Fatalf("zoom %g: grid line shrank to a dot: %+v", zoom, shape.Model)
				}
			}
		}
		if found != ([2][2]bool{{true, true}, {true, true}}) {
			t.Fatalf("zoom %g: missing full-size grid lines: %v", zoom, found)
		}
	}
}

func TestWorkspace2DWheelZoomAnchorsPickingAndDrag(t *testing.T) {
	a := workspaceEditorFixture(t)
	context := &ecs.SystemContext{World: ecs.NewWorld()}
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	in := input.NewState()
	ecs.InsertResource(context.World, in)
	frame, bounds := a.viewport2D.frame, a.canvasBounds()
	x, y := bounds.X+frame.X+60*a.canvasScale, bounds.Y+frame.Y+80*a.canvasScale
	in.MoveMouse(float64(x), float64(y))
	for _, wheel := range []float64{1, 4, -2, -1} {
		previous := a.zoom
		in.ScrollMouse(0, wheel)
		if err := a.update(context); err != nil {
			t.Fatal(err)
		}
		if wheel > 0 && a.zoom <= previous || wheel < 0 && a.zoom >= previous {
			t.Fatal("wheel did not change zoom in the expected direction", previous, a.zoom)
		}
		if in.Pointer().WheelY != 0 {
			t.Fatal("canvas did not consume wheel input")
		}
		if err := a.sync(context); err != nil {
			t.Fatal(err)
		}
		frame = a.viewport2D.frame
		if abs(bounds.X+frame.X+60*a.canvasScale-x) > .001 || abs(bounds.Y+frame.Y+80*a.canvasScale-y) > .001 {
			t.Fatal("zoom moved the scene point under the cursor", frame)
		}
		if id := a.pickViewport2D(x, y); id != "start" {
			t.Fatal("zoom broke instance picking", id)
		}
	}
	a.beginPlacementDrag("start", x, y)
	a.updatePlacementDrag(input.Pointer{X: x + 20, Y: y + 10, Down: true})
	if abs(a.canvasDrag.Delta[0]-20/a.canvasScale) > .001 || abs(a.canvasDrag.Delta[1]-10/a.canvasScale) > .001 {
		t.Fatal("drag did not use the zoomed canvas scale", a.canvasDrag.Delta)
	}
	a.canvasDrag = workspacePlacementDrag{}
	a.action("focus")
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if a.zoom != 1 || a.viewport2D.offset != (mgl32.Vec2{}) {
		t.Fatal("focus did not restore the canvas view")
	}
	frame, bounds = a.viewport2D.frame, a.canvasBounds()
	if abs(frame.X+frame.W/2-bounds.W/2) > .001 || abs(frame.Y+frame.H/2-bounds.H/2) > .001 {
		t.Fatalf("focused scene is not centered: frame %+v, canvas %+v", frame, bounds)
	}
}

func TestWorkspace2DWheelZoomBoundsAndAllocations(t *testing.T) {
	a := workspaceEditorFixture(t)
	context := &ecs.SystemContext{World: ecs.NewWorld()}
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	in := input.NewState()
	bounds := a.canvasBounds()
	for _, point := range [...]mgl32.Vec2{{bounds.X - 1, bounds.Y + 20}, {bounds.X + 20, bounds.Y - 1}, {bounds.X + bounds.W, bounds.Y + 20}, {bounds.X + 20, bounds.Y + bounds.H}} {
		in.ConsumePointerWheel()
		in.MoveMouse(float64(point[0]), float64(point[1]))
		in.ScrollMouse(0, 1)
		a.updateViewport2DInput(in)
		if a.zoom != 1 || in.Pointer().WheelY != 1 {
			t.Fatal("canvas intercepted wheel outside its bounds", point)
		}
	}
	in.MoveMouse(float64(bounds.X+bounds.W/2), float64(bounds.Y+bounds.H/2))
	a.contextOpen = true
	a.updateViewport2DInput(in)
	a.contextOpen = false
	if a.zoom != 1 {
		t.Fatal("canvas zoomed with a context menu open")
	}
	a.menuOpen = "file"
	a.updateViewport2DInput(in)
	a.menuOpen = ""
	a.dialog = "preferences"
	a.updateViewport2DInput(in)
	a.dialog = ""
	if a.zoom != 1 {
		t.Fatal("canvas zoomed with a menu or dialog open")
	}
	for _, sample := range []struct{ wheel, want float32 }{{1000, 8}, {-1000, .05}} {
		in.ConsumePointerWheel()
		in.ScrollMouse(0, float64(sample.wheel))
		a.updateViewport2DInput(in)
		if a.zoom != sample.want {
			t.Fatal("wheel zoom exceeded its limits", a.zoom)
		}
	}
	step := func() {
		in.ScrollMouse(0, 1)
		a.updateViewport2DInput(in)
		in.ScrollMouse(0, -1)
		a.updateViewport2DInput(in)
	}
	if allocations := testing.AllocsPerRun(100, step); allocations != 0 {
		t.Fatalf("wheel zoom input allocated %g", allocations)
	}
}

func TestWorkspace2DAnimationPrivateBindingAndHiddenHierarchy(t *testing.T) {
	a := workspaceEditorFixture(t)
	hidden := false
	if err := a.Document.EditMod("button", "Animate child", func(mod *project.ModSpec) error {
		mod.Elements = []project.ModElementSpec{
			{ID: "hidden", Visible: &hidden},
			{ID: "child", Parent: "hidden", Components: project.ModComponents{Sprite: &project.ModSpriteSpec{Color: "#ff0000", Size: []float32{20, 20}}}},
		}
		mod.Components.Animation = &project.AnimationComponentSpec{Clip: project.AnimationClipSpec{Duration: 1, Loop: true, FloatTracks: []project.AnimationTrackSpec[float32]{{Binding: scene.AnimationBinding{Path: "child", Target: scene.AnimationTargetTransformPositionX}, Keyframes: []project.AnimationKeyframeSpec[float32]{{Time: 0, Value: 0}, {Time: 1, Value: 100}}}}}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	a.changed()
	context := &ecs.SystemContext{World: ecs.NewWorld()}
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	v := &a.viewport2D
	clock, _ := ecs.GetResource[*scene.Time](v.world)
	clock.Set(500*time.Millisecond, 500*time.Millisecond, 1)
	if err := v.syncVisuals(true); err != nil {
		t.Fatal(err)
	}
	child := v.rows[0].parts[1]
	global, _ := ecs.Get[scene.UIGlobalTransform](v.world, child)
	if abs(global.Matrix[12]-95) > .01 {
		t.Fatal("private animation did not update live hierarchy", global.Matrix[12])
	}
	if v.hitContent(child, global.Matrix[12]+5, global.Matrix[13]+5) {
		t.Fatal("hidden ancestor remained pickable")
	}
	root := v.rows[0].root
	a.Selected = "start"
	a.changed()
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if v.rows[0].root != root {
		t.Fatal("selection reset animation entities")
	}
	global, _ = ecs.Get[scene.UIGlobalTransform](v.world, child)
	if abs(global.Matrix[12]-95) > .01 {
		t.Fatal("selection advanced animation twice", global.Matrix[12])
	}
	wakes := 0
	a.requestRender = func() { wakes++ }
	if err := a.updateViewport2D(); err != nil {
		t.Fatal(err)
	}
	if wakes == 0 {
		t.Fatal("animated scene did not request another frame")
	}
	step := func() {
		clock.Set(time.Millisecond, clock.Elapsed+time.Millisecond, clock.Frame+1)
		if err := v.syncVisuals(true); err != nil {
			panic(err)
		}
	}
	for range 4 {
		step()
	}
	if allocations := testing.AllocsPerRun(100, step); allocations != 0 {
		t.Fatalf("warm 2D animation update allocated %g", allocations)
	}
}

func TestWorkspace2DCameraPickingAndPlacementDrag(t *testing.T) {
	a := workspaceEditorFixture(t)
	if err := a.Document.CreateMod("world", project.ModSpec{Space: project.CoordinateSpaceWorld, Components: project.ModComponents{Sprite: &project.ModSpriteSpec{Color: "#00ff00", Size: []float32{80, 80}}}}); err != nil {
		t.Fatal(err)
	}
	if err := a.Document.AddInstance("world", "world", ""); err != nil {
		t.Fatal(err)
	}
	if err := a.Document.CreateMod("camera", project.ModSpec{Space: project.CoordinateSpaceWorld, Transform: project.ModTransformSpec{Rotation: []float32{0, 0, float32(math.Pi / 2)}}, Components: project.ModComponents{Camera2D: &project.ModCamera2DSpec{Scale: 2}}}); err != nil {
		t.Fatal(err)
	}
	if err := a.Document.AddInstance("camera", "camera", ""); err != nil {
		t.Fatal(err)
	}
	a.changed()
	context := &ecs.SystemContext{World: ecs.NewWorld()}
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	v := &a.viewport2D
	camera, projection, transform, ok := scene.PrimaryCamera2DView(v.world)
	if !ok {
		t.Fatal("no camera")
	}
	camera.UpdateViewport(v.size[0], v.size[1])
	point, ok := camera.WorldToViewport2D(transform, projection, mgl32.Vec2{10, 20})
	if !ok {
		t.Fatal("could not project sprite")
	}
	x, y := a.canvasBounds().X+v.frame.X+point[0]*a.canvasScale, a.canvasBounds().Y+v.frame.Y+point[1]*a.canvasScale
	if id := a.pickViewport2D(x, y); id != "world" {
		t.Fatal("authored camera was not used for picking", id)
	}
	in := input.NewState()
	in.MoveMouse(float64(x), float64(y))
	in.ScrollMouse(0, 3)
	a.updateViewport2DInput(in)
	if err := a.syncViewport2D(); err != nil {
		t.Fatal(err)
	}
	projected := a.viewport2DView().Mul4x1(mgl32.Vec4{10, 20, 0, 1})
	bounds := a.canvasBounds()
	if abs(x-bounds.X-projected[0]) > .001 || abs(y-bounds.Y-projected[1]) > .001 {
		t.Fatal("wheel zoom did not anchor the authored camera projection", projected)
	}
	if id := a.pickViewport2D(x, y); id != "world" {
		t.Fatal("zoom broke picking through the authored camera", id)
	}
	a.beginPlacementDrag("world", x, y)
	a.updatePlacementDrag(input.Pointer{X: x + 20, Y: y, Down: true})
	if abs(a.canvasDrag.Delta[0]) > .02 || abs(a.canvasDrag.Delta[1]-40/a.canvasScale) > .05 {
		t.Fatal("drag ignored camera rotation or scale", a.canvasDrag.Delta)
	}
}

func TestWorkspace2DStaticRenderAllocations(t *testing.T) {
	driver := &viewport3DTestDriver{viewport: render.Rect{Width: 1600, Height: 960}}
	render.Uninstall()
	if err := render.Install(driver); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(render.Uninstall)
	canvas := graphics.NewCanvas2D(render.ActiveDevice(), 1600, 960)
	t.Cleanup(graphics.SetCurrentContextForTesting(canvas))
	a := workspaceEditorFixture(t)
	context := &ecs.SystemContext{World: ecs.NewWorld()}
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	frame := func() {
		driver.shapes = driver.shapes[:0]
		driver.clears = driver.clears[:0]
		if err := a.render(context); err != nil {
			panic(err)
		}
		if err := canvas.FinishFrame(); err != nil {
			panic(err)
		}
	}
	for range 4 {
		frame()
	}
	if allocations := testing.AllocsPerRun(100, frame); allocations != 0 {
		t.Fatalf("warm 2D canvas render allocated %g", allocations)
	}
}
