package scene

import (
	"fmt"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/go-gl/mathgl/mgl32"
)

// This driver measures engine CPU submission work, not GPU execution time.
type incrementalTestDriver struct {
	canvasTestDriver
	targetBegins int
	targetClears int
}

func (d *incrementalTestDriver) CreateRenderTarget(render.RenderTargetDescriptor) (render.RenderTargetHandle, render.TextureHandle, error) {
	d.nextID += 2
	return render.NewHandle[render.RenderTargetResource](d.nextID-1, 1), render.NewHandle[render.TextureResource](d.nextID, 1), nil
}
func (d *incrementalTestDriver) ResizeRenderTarget(_ render.RenderTargetHandle, _ render.RenderTargetDescriptor) (render.TextureHandle, error) {
	d.nextID++
	return render.NewHandle[render.TextureResource](d.nextID, 1), nil
}
func (*incrementalTestDriver) DestroyRenderTarget(render.RenderTargetHandle) error { return nil }
func (d *incrementalTestDriver) BeginRenderTarget(_ render.RenderTargetHandle, clear render.RenderTargetClear) error {
	d.targetBegins++
	if clear.ClearColor {
		d.targetClears++
	}
	return nil
}
func (*incrementalTestDriver) EndRenderTarget() error { return nil }

func incrementalRenderFixture(tb testing.TB, count int, covered bool) (*ecs.World, *graphics.Canvas2D, *incrementalTestDriver, []ecs.Entity) {
	tb.Helper()
	driver := &incrementalTestDriver{canvasTestDriver: canvasTestDriver{viewport: render.Rect{Width: 1280, Height: 720}}}
	previous := render.ActiveDevice()
	device, err := render.NewDevice(driver)
	if err != nil {
		tb.Fatal(err)
	}
	device.Activate()
	canvas := graphics.NewCanvas2D(device, 1280, 720)
	restore := graphics.SetCurrentContextForTesting(canvas)
	tb.Cleanup(func() { _ = canvas.FinishFrame(); device.Destroy(); previous.Activate(); restore() })
	world := ecs.NewWorld()
	graphics.BindFrame(world, graphics.CurrentContext())
	RegisterSpriteRenderer2D(world)
	RegisterCameraRenderer2D(world)
	state := RenderWorldFor(world)
	tb.Cleanup(func() { _ = state.Dispose() })
	state.Cameras = []ExtractedCamera{{Entity: 1, Camera: Camera{Clear: true, ClearColor: graphics.Black}, Pipeline: CameraPipeline2D}}
	entities := make([]ecs.Entity, count)
	for i := range entities {
		bundle := ecs.NewBundle(5)
		bundle.Put(Sprite2D{Shape: RoundedRectShape2D(graphics.White, 0), Width: 10, Height: 5, Alpha: 1})
		bundle.Put(UIGlobalTransform{Matrix: mgl32.Translate3D(float32(i%100)*12, float32(i/100%120)*6, 0)})
		bundle.Put(Visibility{Visible: true})
		bundle.Put(AnchorTopLeft)
		bundle.Put(UIStackIndex(i))
		entities[i] = world.Spawn(bundle)
	}
	if covered {
		bundle := ecs.NewBundle(5)
		bundle.Put(Sprite2D{Shape: RoundedRectShape2D(graphics.Black, 0), Width: 1280, Height: 720, Alpha: 1})
		bundle.Put(UIGlobalTransform{Matrix: mgl32.Ident4()})
		bundle.Put(Visibility{Visible: true})
		bundle.Put(AnchorTopLeft)
		bundle.Put(UIStackIndex(count))
		world.Spawn(bundle)
	}
	return world, canvas, driver, entities
}

func BenchmarkIncrementalRender2D(b *testing.B) {
	for _, count := range []int{1000, 10000} {
		for _, mode := range []string{"stable", "color", "move", "scroll", "all", "covered"} {
			b.Run(fmt.Sprintf("%d/%s", count, mode), func(b *testing.B) {
				world, canvas, driver, entities := incrementalRenderFixture(b, count, mode == "covered")
				context := &ecs.SystemContext{World: world}
				frame := func() {
					if err := extractRender2D(context); err != nil {
						b.Fatal(err)
					}
					if err := renderCameras(context); err != nil {
						b.Fatal(err)
					}
					if err := canvas.FinishFrame(); err != nil {
						b.Fatal(err)
					}
				}
				step := func(i int) {
					switch mode {
					case "color", "covered":
						world.GetMut[Sprite2D](entities[0]).Shape.Fill.R = float32(i & 1)
					case "move":
						world.GetMut[UIGlobalTransform](entities[0]).Matrix[12] = float32(i&1) * 600
					case "all":
						for _, entity := range entities {
							world.GetMut[Sprite2D](entity).Shape.Fill.R = float32(i & 1)
						}
					case "scroll":
						for j, entity := range entities {
							world.GetMut[UIGlobalTransform](entity).Matrix[12] = float32(j%100)*12 + float32(i&1)*128
						}
					}
					frame()
				}
				for i := range 8 {
					step(i)
				}
				driver.sprites, driver.shapes = 0, 0
				b.ReportAllocs()
				b.ResetTimer()
				for i := range b.N {
					step(i)
				}
				b.StopTimer()
				b.ReportMetric(float64(driver.sprites+driver.shapes)/float64(b.N), "instances/op")
			})
		}
	}
}
