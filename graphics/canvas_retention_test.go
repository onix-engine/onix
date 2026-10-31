package graphics

import (
	"testing"

	"github.com/diiyw/onix/internal/reuse"
	"github.com/diiyw/onix/render"
)

func TestCanvasReclaimsBurstCapacityWithoutLosingEarlierBatches(t *testing.T) {
	device, err := render.NewDevice(&textureTestDriver{})
	if err != nil {
		t.Fatal(err)
	}
	defer device.Destroy()
	canvas := NewCanvas2D(device, 100, 100)
	canvas.instances = make([]render.Instance2D, 0, 8192)
	canvas.draws = make([]render.SpriteDraw, 0, 8192)
	canvas.shapes = make([]render.ShapeInstance2D, 0, 8192)
	for range reuse.RetentionFrames * 2 {
		canvas.instances = canvas.instances[:4096]
		canvas.resetDrawBatch()
		canvas.instances = canvas.instances[:1]
		canvas.resetDrawBatch()
		if err := canvas.FinishFrame(); err != nil {
			t.Fatal(err)
		}
		if cap(canvas.instances) != 8192 {
			t.Fatal("flushing a small final batch lost the earlier peak")
		}
	}
	for range reuse.RetentionFrames {
		canvas.instances = canvas.instances[:1]
		canvas.resetDrawBatch()
		if err := canvas.FinishFrame(); err != nil {
			t.Fatal(err)
		}
	}
	if cap(canvas.instances) != initialBatchInstances || cap(canvas.draws) != 0 || cap(canvas.shapes) != 0 {
		t.Fatalf("canvas retained burst buffers: %d/%d/%d", cap(canvas.instances), cap(canvas.draws), cap(canvas.shapes))
	}
	if allocs := testing.AllocsPerRun(100, func() {
		canvas.instances = canvas.instances[:1]
		canvas.resetDrawBatch()
		_ = canvas.FinishFrame()
	}); allocs != 0 {
		t.Fatalf("warm canvas allocated %g times", allocs)
	}
}
