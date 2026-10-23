//go:build linux && cgo && !android && !onix_drm

package scene_test

import (
	"bytes"
	"os"
	"runtime"
	"slices"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	"github.com/diiyw/onix/window"
	"github.com/go-gl/mathgl/mgl32"
)

type compositeNativeDriver struct {
	render.Driver
	render.RenderTargetDriver
	created  []render.RenderTargetHandle
	inTarget bool
	output   []render.SpriteBatch
}

func (d *compositeNativeDriver) BeginRenderTarget(handle render.RenderTargetHandle, clear render.RenderTargetClear) error {
	d.inTarget = true
	return d.RenderTargetDriver.BeginRenderTarget(handle, clear)
}

func (d *compositeNativeDriver) EndRenderTarget() error {
	d.inTarget = false
	return d.RenderTargetDriver.EndRenderTarget()
}

func (d *compositeNativeDriver) DrawSprites(batch render.SpriteBatch) error {
	if !d.inTarget {
		d.output = append(d.output, render.SpriteBatch{Projection: batch.Projection, Instances: slices.Clone(batch.Instances), Draws: slices.Clone(batch.Draws)})
	}
	return d.Driver.DrawSprites(batch)
}

func (d *compositeNativeDriver) CreateRenderTarget(desc render.RenderTargetDescriptor) (render.RenderTargetHandle, render.TextureHandle, error) {
	handle, texture, err := d.RenderTargetDriver.CreateRenderTarget(desc)
	if err == nil {
		d.created = append(d.created, handle)
	}
	return handle, texture, err
}

// Compare raw attachment pixels against a fresh full render. This test never
// launches the editor, captures screenshots or relies on visual inspection.
func TestNativeComposite2DMatchesFullRender(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("requires Xvfb or an X display")
	}
	context := window.NewContext(768, 512, "incremental attachment regression", window.Options{})
	defer runtime.UnlockOSThread()
	defer context.Destroy()
	native := context.Device.Driver()
	driver := &compositeNativeDriver{Driver: native, RenderTargetDriver: native.(render.RenderTargetDriver)}
	device, err := render.NewDevice(driver)
	if err != nil {
		t.Fatal(err)
	}
	device.Activate()
	canvas := graphics.NewCanvas2D(device, 768, 512)
	restore := graphics.SetCurrentContextForTesting(canvas)
	defer restore()
	defer context.Device.Activate()
	world := ecs.NewWorld()
	scene.RegisterSpriteRenderer2D(world)
	scene.RegisterTextRenderer2D(world)
	scene.RegisterCameraRenderer2D(world)
	camera := scene.SpawnCamera2D(world)
	spawn := ecs.NewSpawner5[scene.Sprite2D, scene.UIGlobalTransform, scene.Visibility, scene.Anchor, scene.UIStackIndex](world)
	background := spawn.Spawn(scene.Sprite2D{Shape: scene.RoundedRectShape2D(graphics.ColorFromRGB(20, 60, 110), 0), Width: 350, Height: 230, Alpha: 1}, scene.UIGlobalTransform{Matrix: mgl32.Translate3D(7, 5, 0)}, scene.Visibility{Visible: true}, scene.AnchorTopLeft, 0)
	foreground := spawn.Spawn(scene.Sprite2D{Shape: scene.RoundedRectShape2D(graphics.ColorFromRGBA(240, 70, 20, 150), 17), Width: 180, Height: 100, Alpha: .8}, scene.UIGlobalTransform{Matrix: mgl32.Translate3D(95, 65, 0)}, scene.Visibility{Visible: true}, scene.AnchorTopLeft, 1)
	spawn.Spawn(scene.Sprite2D{Shape: scene.RoundedRectShape2D(graphics.ColorFromRGB(210, 10, 90), 0), Width: 128, Height: 128, Alpha: 1}, scene.UIGlobalTransform{Matrix: mgl32.Translate3D(384, 256, 0)}, scene.Visibility{Visible: true}, scene.AnchorTopLeft, 3)
	clipped := spawn.Spawn(scene.Sprite2D{Shape: scene.RoundedRectShape2D(graphics.White, 0), Width: 256, Height: 256, Alpha: 1}, scene.UIGlobalTransform{Matrix: mgl32.Translate3D(384, 256, 0)}, scene.Visibility{Visible: true}, scene.AnchorTopLeft, 4)
	if err := ecs.Insert(world, clipped, scene.UITransformFromXY(384, 256)); err != nil {
		t.Fatal(err)
	}
	if err := ecs.Insert(world, clipped, scene.ClipRect2D{X: .25, Y: .25, Width: 127.5, Height: 127.5}); err != nil {
		t.Fatal(err)
	}
	text := scene.NewText2D("Tile boundary: Aa界\nTransparent overlap")
	text.Font, text.FontSize, text.LCD = notosanssc.NotoSansSCRegular, 20, true
	text.Color = graphics.ColorFromRGB(240, 230, 180)
	text.ShadowX, text.ShadowY, text.ShadowColor = 3, 2, graphics.ColorFromRGBA(0, 0, 0, 160)
	textSpawn := ecs.NewSpawner5[scene.Text2D, scene.UIGlobalTransform, scene.Visibility, scene.Anchor, scene.UIStackIndex](world)
	label := textSpawn.Spawn(text, scene.UIGlobalTransform{Matrix: mgl32.Translate3D(105, 120, 0)}, scene.Visibility{Visible: true}, scene.AnchorTopLeft, 2)
	state, _ := ecs.GetResource[*scene.RenderWorld](world)
	defer state.Dispose()
	var reference, capture *render.RenderTarget
	defer func() {
		if reference != nil {
			_ = reference.Dispose()
		}
		if capture != nil {
			_ = capture.Dispose()
		}
	}()
	var referenceHandle render.RenderTargetHandle
	var captureHandle render.RenderTargetHandle
	pixels, expected := make([]byte, 768*512*4), make([]byte, 768*512*4)
	check := func(name string) {
		t.Helper()
		driver.output = driver.output[:0]
		if err := scene.SyncUIGlobalTransformSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		context := &ecs.SystemContext{World: world}
		if err := scene.ExtractCameras(context); err != nil {
			t.Fatal(err)
		}
		if err := scene.ExtractRender2DSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		if err := scene.RenderCamerasSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		if err := canvas.FinishFrame(); err != nil {
			t.Fatal(err)
		}
		if err := device.Present(); err != nil {
			t.Fatal(err)
		}
		if state.Composite2D.CacheBytes == 0 {
			t.Fatalf("%s bypassed compositor", name)
		}
		read := native.(render.RenderTargetReadbackDriver)
		if reference == nil {
			reference, err = render.NewRenderTarget(render.RenderTargetDescriptor{Width: 768, Height: 512})
			if err != nil {
				t.Fatal(err)
			}
			_, err = reference.ColorTextureFor(device)
			if err != nil {
				t.Fatal(err)
			}
			referenceHandle = driver.created[len(driver.created)-1]
			capture, err = render.NewRenderTarget(render.RenderTargetDescriptor{Width: 768, Height: 512})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = capture.ColorTextureFor(device); err != nil {
				t.Fatal(err)
			}
			captureHandle = driver.created[len(driver.created)-1]
		}
		clear := ecs.GetRef[scene.Camera](world, camera).ClearColor
		if err := canvas.DrawTo(capture, render.RenderTargetClear{Color: render.Color(clear), ClearColor: true}, func() error {
			for _, batch := range driver.output {
				if err := native.DrawSprites(batch); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if err := canvas.DrawTo(reference, render.RenderTargetClear{Color: render.Color(clear), ClearColor: true}, func() error {
			scene.Draw2D(canvas, world, scene.DrawViews2D{World: mgl32.Ident4(), UI: mgl32.Ident4()})
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if err := canvas.FinishFrame(); err != nil {
			t.Fatal(err)
		}
		if err := device.Present(); err != nil {
			t.Fatal(err)
		}
		if err := read.ReadRenderTarget(captureHandle, render.RenderTargetRegion{Width: 768, Height: 512}, pixels); err != nil {
			t.Fatal(err)
		}
		if err := read.ReadRenderTarget(referenceHandle, render.RenderTargetRegion{Width: 768, Height: 512}, expected); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(pixels, expected) {
			count, first := 0, -1
			for i := range pixels {
				if pixels[i] != expected[i] {
					count++
					if first < 0 {
						first = i
					}
				}
			}
			t.Fatalf("%s: %d differing channels; first pixel=(%d,%d), channel=%d cached=%d full=%d", name, count, first/4%768, first/4/768, first%4, pixels[first], expected[first])
		}
	}
	check("initial")
	check("stable")
	for _, entity := range []ecs.Entity{background, foreground, label} {
		ecs.GetMut[scene.UIGlobalTransform](world, entity).Matrix[12] += 128
	}
	check("tile scroll")
	if state.Composite2D.ReusedTiles == 0 {
		t.Fatal("scroll did not reuse pixels")
	}
	for _, entity := range []ecs.Entity{background, foreground, label} {
		ecs.GetMut[scene.UIGlobalTransform](world, entity).Matrix[12] -= 128
	}
	check("reverse tile scroll")
	ecs.GetMut[scene.ClipRect2D](world, clipped).Width = 95.5
	check("fractional clip update")
	ecs.GetMut[scene.Sprite2D](world, foreground).Shape.Fill = graphics.ColorFromRGBA(0, 220, 30, 90)
	check("alpha color")
	ecs.GetMut[scene.UIGlobalTransform](world, foreground).Matrix = mgl32.Translate3D(205.25, 30.5, 0).Mul4(mgl32.HomogRotate3DZ(.17))
	check("rotated move")
	ecs.GetMut[scene.Text2D](world, label).Value = "Updated text 界\nOverlap"
	check("text")
	ecs.GetMut[scene.Visibility](world, foreground).Visible = false
	check("hide")
	ecs.SetIfChanged(world, background, scene.UIStackIndex(5))
	check("reorder")
	world.Despawn(background)
	check("remove")
	canvas.SetRasterScale(1.25)
	check("fractional raster density")
	ecs.GetMut[scene.UIGlobalTransform](world, label).Matrix[12] += 128
	check("fractional raster density scroll")
}
