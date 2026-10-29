package scene

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/go-gl/mathgl/mgl32"
	"testing"
)

func TestWorldFramesRenderIndependentlyOfImmediateContext(t *testing.T) {
	w1, c1, d1, _ := incrementalRenderFixture(t, 1, false)
	w2, c2, d2, _ := incrementalRenderFixture(t, 2, false)
	c1.Resize(320, 200)
	c2.Resize(640, 400)
	run := func(w *ecs.World, c *graphics.Canvas2D) {
		t.Helper()
		for _, system := range []ecs.SystemConfig{ExtractRender2DSystem(""), RenderCamerasSystem("")} {
			if err := system.Run(w); err != nil {
				t.Fatal(err)
			}
		}
		if err := c.FinishFrame(); err != nil {
			t.Fatal(err)
		}
	}
	// Immediate state deliberately points at the other world's canvas/device.
	graphics.SetCurrentContext(c2)
	c2.Device().Activate()
	run(w1, c1)
	if d1.shapes != 1 || d2.shapes != 0 {
		t.Fatalf("first frame submitted to wrong driver: %d/%d", d1.shapes, d2.shapes)
	}
	graphics.SetCurrentContext(c1)
	c1.Device().Activate()
	run(w2, c2)
	if d1.shapes != 1 || d2.shapes != 2 {
		t.Fatalf("second frame submitted to wrong driver: %d/%d", d1.shapes, d2.shapes)
	}
	// Snapshot camera placement must survive later canvas/context changes.
	camera := NewCamera()
	camera.Viewport = CameraViewport{X: 80, Y: 50, Width: 160, Height: 100}
	b := ecs.NewBundle(4)
	b.Put(camera)
	b.Put(Camera2D{})
	b.Put(OrthographicProjection2D())
	b.Put(GlobalTransform{Matrix: mgl32.Ident4()})
	w1.Spawn(b)
	if err := ExtractCamerasSystem("").Run(w1); err != nil {
		t.Fatal(err)
	}
	snapshot := RenderWorldFor(w1).Cameras[0]
	before := snapshot.ClipFromWorld()
	c1.Resize(900, 700)
	graphics.SetCurrentContext(nil)
	if snapshot.ClipFromWorld() != before {
		t.Fatal("extracted camera read a later output context")
	}
}
