package html

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/graphics"
	"github.com/onix-engine/onix/render"
	"github.com/onix-engine/onix/scene"
)

type paintListDriver struct {
	shapes []render.ShapeInstance2D
	glyphs []render.Instance2D
	textureTestDriver
}

func (d *paintListDriver) DrawShapes(batch render.ShapeBatch) error {
	d.shapes = append(d.shapes, batch.Instances...)
	return nil
}

func (d *paintListDriver) DrawSprites(batch render.SpriteBatch) error {
	d.glyphs = append(d.glyphs, batch.Instances...)
	return nil
}

func bindPaintListCanvas(t testing.TB, world *ecs.World) (*graphics.Canvas2D, *paintListDriver) {
	t.Helper()
	driver := &paintListDriver{}
	previous := render.ActiveDevice()
	device, err := render.NewDevice(driver)
	if err != nil {
		t.Fatal(err)
	}
	device.Activate()
	canvas := graphics.NewCanvas2D(device, 320, 200)
	graphics.BindFrame(world, canvas)
	scene.RegisterSpriteRenderer2D(world)
	scene.RegisterTextRenderer2D(world)
	t.Cleanup(func() {
		_ = canvas.FinishFrame()
		device.Destroy()
		previous.Activate()
	})
	return canvas, driver
}

func TestHTMLPaintListMatchesECSSubmission(t *testing.T) {
	source := `<article><div id="card" style="width:100px;background:#fde;border:2px solid blue;padding:3px">Hello</div><div style="width:20px;height:10px;border:1px solid red"></div></article>`
	world, owner := incrementalHTMLFixture(t, source)
	canvas, driver := bindPaintListCanvas(t, world)
	if err := world.Insert(owner, scene.UITransformFromXY(6, 7)); err != nil {
		t.Fatal(err)
	}
	extract := func() {
		t.Helper()
		incrementalSync(t, world)
		if err := scene.SyncUIGlobalTransformSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		if err := scene.ExtractRender2DSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
	}
	draw := func() {
		t.Helper()
		driver.shapes, driver.glyphs = driver.shapes[:0], driver.glyphs[:0]
		view := mgl32.Ident4()
		scene.Draw2DSubtree(canvas, world, owner, scene.DrawViews2D{UI: view, World: view})
		if err := canvas.FinishFrame(); err != nil {
			t.Fatal(err)
		}
	}
	extract()
	draw()
	baseline := Metrics(world, owner)
	shapes, glyphs := slices.Clone(driver.shapes), slices.Clone(driver.glyphs)
	if len(shapes) != 2 || len(glyphs) == 0 || baseline.ECSChildren != 3 || baseline.RenderItems < 3 {
		t.Fatalf("baseline metrics=%+v, shapes=%d, glyphs=%d", baseline, len(shapes), len(glyphs))
	}
	world.GetMut[Surface](owner).PassThrough = true
	extract()
	draw()
	metrics := Metrics(world, owner)
	if metrics.DOMNodes != 5 || metrics.Boxes != 4 || metrics.Fragments != 1 ||
		metrics.ECSChildren != 0 || metrics.PaintBoxes != 2 || metrics.RenderItems != baseline.RenderItems ||
		metrics.Slices.PaintListBoxes < 2 || metrics.Slices.PaintListGlyphs == 0 {
		t.Fatalf("paint metrics=%+v", metrics)
	}
	if !slices.Equal(shapes, driver.shapes) || !slices.Equal(glyphs, driver.glyphs) {
		t.Fatalf("paint submission changed: shapes %v / %v, glyphs %v / %v", shapes, driver.shapes, glyphs, driver.glyphs)
	}
	extract()
	if preparation := scene.RenderWorldFor(world).Preparation2D; preparation.Extracted != 0 || preparation.Reused != metrics.RenderItems {
		t.Fatalf("stable scene was extracted again: %+v", preparation)
	}
	if allocs := testing.AllocsPerRun(50, extract); allocs != 0 {
		t.Fatalf("stable sync and extraction allocated %g times", allocs)
	}

	card := incrementalHTMLNode(t, world, owner, "card")
	card.SetText("Updated")
	extract()
	draw()
	if Metrics(world, owner).ECSChildren != 0 || slices.Equal(glyphs, driver.glyphs) {
		t.Fatal("text edit did not refresh retained glyphs")
	}
	card.SetAttribute("style", "width:100px;background:red;border:2px solid blue;padding:3px")
	extract()
	draw()
	if Diagnostics(world, owner).Path != SyncPartial || driver.shapes[0].Fill == shapes[0].Fill {
		t.Fatal("local paint update did not refresh the retained shape")
	}
	beforeX := driver.shapes[0].Model[2]
	if !SetNodeOffset(world, owner, "card", 17, 0) || Metrics(world, owner).PaintBoxes != 0 {
		t.Fatal("offset binding did not move paint back to the movable entity path")
	}
	extract()
	draw()
	if driver.shapes[0].Model[2] != beforeX+17 {
		t.Fatalf("offset binding submitted shape at %v, before x=%v", driver.shapes[0].Model, beforeX)
	}
	Release(world, owner)
	if Metrics(world, owner) != (HTMLMetrics{}) || world.Has[*scene.UIPaintList2D](owner) {
		t.Fatal("release retained the surface paint list")
	}
}

func TestHTMLPaintListTracksStructureAndPointerEligibility(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<article id="rows" style="pointer-events:none"><div id="row" style="background:red">Row</div></article>`)
	row := incrementalHTMLNode(t, world, owner, "row")
	parent := incrementalHTMLNode(t, world, owner, "rows")
	if m := Metrics(world, owner); m.PaintBoxes != 1 || m.ECSChildren != 0 {
		t.Fatalf("passive row metrics=%+v", m)
	}
	row.SetAttribute("style", "background:red;pointer-events:auto")
	incrementalSync(t, world)
	if m := Metrics(world, owner); m.PaintBoxes != 0 || m.ECSChildren != 2 {
		t.Fatalf("target row metrics=%+v", m)
	}
	row.SetAttribute("style", "background:red")
	incrementalSync(t, world)
	if m := Metrics(world, owner); m.PaintBoxes != 1 || m.ECSChildren != 0 {
		t.Fatalf("restored passive row metrics=%+v", m)
	}
	for range 150 {
		parent.Append(row.Clone())
	}
	incrementalSync(t, world)
	peak := Metrics(world, owner)
	if peak.PaintBoxes != 151 {
		t.Fatalf("inserted rows metrics=%+v", peak)
	}
	parent.Clear()
	parent.Append(row)
	incrementalSync(t, world)
	if m := Metrics(world, owner); m.PaintBoxes != 1 || m.Slices.PaintListBoxes >= peak.Slices.PaintListBoxes {
		t.Fatalf("removed rows metrics=%+v", m)
	}
	row.SetText("Changed")
	incrementalSync(t, world)
	if Metrics(world, owner).PaintBoxes != 1 {
		t.Fatal("updating after slot storage shrank lost paint")
	}
}

func TestHTMLPaintListTracksInheritedTransformAndTextureMetrics(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<article id="group" style="pointer-events:none"><div id="label" style="background:red">Label</div></article><div style="width:20px;height:10px;background:linear-gradient(red,blue)"></div>`)
	if m := Metrics(world, owner); m.PaintBoxes != 1 || m.TexturePixels != 200 {
		t.Fatalf("initial metrics=%+v", m)
	}
	if allocations := testing.AllocsPerRun(50, func() { _ = Metrics(world, owner) }); allocations != 0 {
		t.Fatalf("reading metrics allocated %g times", allocations)
	}
	group := incrementalHTMLNode(t, world, owner, "group")
	label := incrementalHTMLNode(t, world, owner, "label")
	group.SetAttribute("style", "pointer-events:none;transform:translateX(5px)")
	incrementalSync(t, world)
	visual := htmlCullRole(world, owner, label, htmlRoleSurface)
	if m := Metrics(world, owner); m.PaintBoxes != 0 || visual == 0 || world.GetRef[scene.UITransform](visual).Translation.X.Value != 5 {
		t.Fatalf("inherited transform metrics=%+v, label=%v", m, visual)
	}
	group.SetAttribute("style", "pointer-events:none")
	incrementalSync(t, world)
	if m := Metrics(world, owner); m.PaintBoxes != 1 || m.TexturePixels != 200 {
		t.Fatalf("restored metrics=%+v", m)
	}
}

func BenchmarkHTMLPaintListMount(b *testing.B) {
	source := strings.Repeat(`<div style="background:#fde;border:1px solid blue">Label</div>`, 1000)
	for _, passThrough := range []bool{false, true} {
		b.Run(fmt.Sprintf("passThrough=%t", passThrough), func(b *testing.B) {
			value := New(source)
			value.PassThrough = passThrough
			mount := func() (*ecs.World, ecs.Entity) {
				world := ecs.NewWorld()
				spawner := world.NewSpawner1[Surface]()
				owner := spawner.Spawn(value)
				incrementalSync(b, world)
				return world, owner
			}
			world, owner := mount()
			children := Metrics(world, owner).ECSChildren
			for b.Loop() {
				mount()
			}
			b.ReportMetric(float64(children), "children/op")
		})
	}
}

func TestHTMLPaintListSparseExtraction(t *testing.T) {
	row := `<div style="background:#fde;border:1px solid blue">Label</div>`
	source := strings.Replace(row, "<div", `<div id="target"`, 1) + strings.Repeat(row, 999)
	world := ecs.NewWorld()
	value := New(source)
	value.PassThrough = true
	spawner := world.NewSpawner2[Surface, scene.UITransform]()
	owner := spawner.Spawn(value, scene.UITransformFromXY(0, 0))
	bindPaintListCanvas(t, world)
	extract := func() scene.RenderPreparation2D {
		incrementalSync(t, world)
		if err := scene.SyncUIGlobalTransformSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		if err := scene.ExtractRender2DSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		return scene.RenderWorldFor(world).Preparation2D
	}
	extract()
	if stable := extract(); stable.Reused != 2000 || Metrics(world, owner).ECSChildren != 0 {
		t.Fatalf("stable preparation %+v", stable)
	}
	node := incrementalHTMLNode(t, world, owner, "target")
	for _, attr := range []string{"background:red;border:1px solid blue", "background:red;color:blue;border:1px solid blue"} {
		node.SetAttribute("style", attr)
		if got := extract(); got.Sorted || got.Extracted != 1 || got.Reused != 1999 {
			t.Fatalf("editing %q: %+v", attr, got)
		}
	}
	node.SetText("Newer")
	if got := extract(); got.Sorted || got.Extracted != 1 || got.Reused != 1999 {
		t.Fatalf("text edit: %+v", got)
	}

	toggle := false
	updates := func() {
		if toggle {
			node.SetAttribute("style", "background:red;color:blue;border:1px solid blue")
		} else {
			node.SetAttribute("style", "background:red;color:green;border:1px solid blue")
		}
		toggle = !toggle
		extract()
	}
	updates()
	updates()
	if allocations := testing.AllocsPerRun(25, updates); allocations != 0 {
		t.Fatalf("warm paint-list update allocated %g times", allocations)
	}
}
