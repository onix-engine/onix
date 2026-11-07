package scene

import (
	"image"
	"testing"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/graphics"
	"github.com/onix-engine/onix/internal/reuse"
	"github.com/onix-engine/onix/render"
	"github.com/onix-engine/onix/spine"
	"github.com/go-gl/mathgl/mgl32"
)

type spineBatchTestDriver struct {
	canvasTestDriver
	draws int
}

func (*spineBatchTestDriver) CreateMesh([]render.Vertex3D, []uint32) (render.MeshHandle, error) {
	return render.NewHandle[render.MeshResource](1, 1), nil
}
func (*spineBatchTestDriver) DestroyMesh(render.MeshHandle) error { return nil }
func (d *spineBatchTestDriver) DrawMeshes(batch render.MeshBatch3D) error {
	d.draws += len(batch.Draws)
	return nil
}

func TestSpineSharedBatchRollsBackCulledDrawsAndReclaimsCapacity(t *testing.T) {
	driver := &spineBatchTestDriver{canvasTestDriver: canvasTestDriver{viewport: render.Rect{Width: 100, Height: 80}}}
	previous := render.ActiveDevice()
	device, err := render.NewDevice(driver)
	if err != nil {
		t.Fatal(err)
	}
	device.Activate()
	canvas := graphics.NewCanvas2D(device, 100, 80)
	t.Cleanup(func() { _ = canvas.FinishFrame(); device.Destroy(); previous.Activate() })
	texture := graphics.CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 10, 10)))
	t.Cleanup(func() { _ = texture.Dispose() })
	skeleton := []byte(`{"bones":[{"name":"root"}],"slots":[{"name":"slot","bone":"root","attachment":"region"}],"skins":{"default":{"slot":{"region":{"width":10,"height":10}}}}}`)
	atlas := []byte("page.png\nsize: 10, 10\nregion\n  rotate: false\n  xy: 0, 0\n  size: 10, 10\n  orig: 10, 10\n  offset: 0, 0\n")
	resource, err := spine.Load(skeleton, atlas, texture)
	if err != nil {
		t.Fatal(err)
	}
	player := Spine2D{Player: spine.NewPlayer(resource)}
	items := []renderItem2D{
		{content: &player, transform: mgl32.Translate3D(20, 20, 0), ui: true},
		{content: &player, transform: mgl32.Translate3D(200, 20, 0), ui: true},
		{content: &player, transform: mgl32.Translate3D(40, 20, 0), ui: true},
	}
	world := ecs.NewWorld()
	scratch := world.FrameValue[spineRenderBatch2DScratch, spineRenderScratch2D]()
	scratch.batch.Vertices = make([]render.Vertex3D, 0, 4096)
	scratch.batch.Indices = make([]uint32, 0, 8192)
	scratch.batch.Instances = make([]render.Instance3D, 0, 128)
	scratch.batch.Draws = make([]render.MeshDraw3D, 0, 128)
	viewport := renderViewport2D{clipFromLayer: canvas.ProjectionMatrix(), active: true}
	for _, count := range []int{2, 3} {
		drawSpineRenderBatch2D(canvas, world, renderItemList2D{items: items, count: count}, viewport)
		batch := &scratch.batch
		if len(batch.Vertices) != (count-1)*4 || len(batch.Indices) != (count-1)*6 || len(batch.Draws) != 1 || batch.Draws[0].IndexCount != uint32(len(batch.Indices)) || len(batch.Instances) != 1 {
			t.Fatalf("culled append corrupted the visible draw range: vertices=%d indices=%d draws=%v", len(batch.Vertices), len(batch.Indices), batch.Draws)
		}
		for _, index := range batch.Indices {
			if int(index) >= len(batch.Vertices) {
				t.Fatal("culled append left an index outside visible geometry")
			}
		}
	}
	frame := func() {
		scratch.trim()
		drawSpineRenderBatch2D(canvas, world, renderItemList2D{items: items, count: len(items)}, viewport)
		if err := canvas.FinishFrame(); err != nil {
			t.Fatal(err)
		}
	}
	for range reuse.RetentionFrames + 2 {
		frame()
	}
	if cap(scratch.batch.Vertices) > 64 || cap(scratch.batch.Indices) > 96 || cap(scratch.batch.Instances) > 2 || cap(scratch.batch.Draws) > 8 {
		t.Fatal("shared Spine batch retained its peak geometry capacity")
	}
	if allocations := testing.AllocsPerRun(100, frame); allocations != 0 {
		t.Fatalf("warm Spine draw allocated %v times", allocations)
	}
	if driver.draws < 100 {
		t.Fatal("regression did not submit the shared geometry to the driver")
	}
}
