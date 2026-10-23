package graphics

import (
	"errors"
	"testing"

	"github.com/diiyw/onix/render"
	"github.com/go-gl/mathgl/mgl32"
)

type canvasTargetTestDriver struct {
	canvasTestDriver
	beginTargetErr error
	targets, ends  int
}

func (d *canvasTargetTestDriver) CreateRenderTarget(render.RenderTargetDescriptor) (render.RenderTargetHandle, render.TextureHandle, error) {
	d.nextID += 2
	return render.NewHandle[render.RenderTargetResource](d.nextID-1, 1), render.NewHandle[render.TextureResource](d.nextID, 1), nil
}
func (d *canvasTargetTestDriver) ResizeRenderTarget(render.RenderTargetHandle, render.RenderTargetDescriptor) (render.TextureHandle, error) {
	d.nextID++
	return render.NewHandle[render.TextureResource](d.nextID, 1), nil
}
func (*canvasTargetTestDriver) DestroyRenderTarget(render.RenderTargetHandle) error { return nil }
func (d *canvasTargetTestDriver) BeginRenderTarget(render.RenderTargetHandle, render.RenderTargetClear) error {
	d.targets++
	return d.beginTargetErr
}
func (d *canvasTargetTestDriver) EndRenderTarget() error { d.ends++; return nil }

func TestCanvasDrawToRestoresStateOnCallbackError(t *testing.T) {
	driver := &canvasTargetTestDriver{canvasTestDriver: canvasTestDriver{viewport: render.Rect{Width: 800, Height: 600}}}
	installCanvasTestDriver(t, driver)
	c := NewCanvas2D(render.ActiveDevice(), 800, 600)
	target, err := render.NewRenderTarget(render.RenderTargetDescriptor{Width: 320, Height: 240})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = target.Dispose() })
	view := mgl32.Translate3D(10, 20, 0)
	projection := mgl32.Ortho2D(0, 400, 300, 0)
	c.SetViewMatrix(view)
	c.SetProjectionMatrix(projection)
	c.PushClipRect(5, 6, 100, 80)
	drawErr := errors.New("draw failed")
	if err := c.DrawTo(target, render.RenderTargetClear{ClearColor: true}, func() error {
		if w, h := c.Size(); w != 320 || h != 240 {
			t.Fatal("target did not set logical size", w, h)
		}
		c.PushClipRect(1, 2, 30, 40)
		c.SetViewMatrix(mgl32.Scale3D(2, 2, 1))
		c.DrawShapeMatrices(mgl32.Ident4(), mgl32.Ident3(), 10, 20, render.ShapeRoundedRect, White, ColorRGBA{}, 0, 0, 1)
		return drawErr
	}); !errors.Is(err, drawErr) {
		t.Fatal("callback error was lost", err)
	}
	if w, h := c.Size(); w != 800 || h != 600 {
		t.Fatal("logical size was not restored", w, h)
	}
	if c.ViewMatrix() != view || c.ProjectionMatrix() != projection {
		t.Fatal("matrices were not restored")
	}
	if driver.viewport != (render.Rect{Width: 800, Height: 600}) || len(c.clipStack) != 1 {
		t.Fatal("viewport or clips were not restored")
	}
	if driver.scissor == nil || *driver.scissor != (render.Rect{X: 5, Y: 6, Width: 100, Height: 80}) {
		t.Fatal("scissor was not restored", driver.scissor)
	}
	if driver.targets != 1 || driver.ends != 1 || driver.submits != 1 {
		t.Fatal("target work was not flushed", driver.targets, driver.ends, driver.submits)
	}
	driver.beginTargetErr = errors.New("target failed")
	if err := c.DrawTo(target, render.RenderTargetClear{}, func() error { t.Fatal("callback ran after begin failed"); return nil }); !errors.Is(err, driver.beginTargetErr) {
		t.Fatal("begin error was lost", err)
	}
	if c.ViewMatrix() != view || driver.ends != 1 {
		t.Fatal("failed begin altered canvas state")
	}
}
