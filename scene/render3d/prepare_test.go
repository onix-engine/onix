package render3d

import (
	"errors"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
)

type preparationFailureDriver struct {
	meshTestDriver
	fail     bool
	textures int
}

func (d *preparationFailureDriver) CreateTexture(desc render.TextureDescriptor, pixels []byte) (render.TextureHandle, error) {
	d.textures++
	if d.fail && d.textures > 1 {
		return render.TextureHandle{}, errors.New("test texture failure")
	}
	return d.meshTestDriver.CreateTexture(desc, pixels)
}

func TestPrepareRenderResourcesQueuesMainViewsAndDiscardsFailure(t *testing.T) {
	driver := &preparationFailureDriver{fail: true}
	render.Uninstall()
	if err := render.Install(driver); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(render.Uninstall)
	world := ecs.NewWorld()
	graphics.BindFrame(world, graphics.NewCanvas2D(render.ActiveDevice(), 320, 200))
	Register(world)
	state := WorldFor(world)
	item := extractedMesh3D{vertices: []render.Vertex3D{{}}, material: graphics.Material3DData{AlphaMode: graphics.AlphaOpaque}}
	opaque, masked, blend := item, item, item
	masked.material.AlphaMode = graphics.AlphaMask
	blend.material.AlphaMode = graphics.AlphaBlend
	main := state.viewFor(1)
	main.extracted = []extractedMesh3D{opaque, masked, blend}
	main.shadowActive = true
	main.shadow = &shadowAtlas3D{count: 1, views: [render.MaxShadowMaps]*renderView3D{{extracted: []extractedMesh3D{opaque}}}}
	picking := state.viewFor(2)
	picking.frame.Mode = render.RenderViewPicking
	picking.extracted = []extractedMesh3D{blend}
	bad := item
	bad.material.BaseColorTexture = graphics.CreateTextureByColor(graphics.Black)
	picking.extracted = append(picking.extracted, bad)
	state.viewOrder = []ecs.Entity{1, 2}
	prepare := PrepareSystem("")
	if err := prepare.Run(world); err == nil {
		t.Fatal("expected late preparation failure")
	}
	for _, view := range []*renderView3D{main, picking} {
		if len(view.opaque.items) != 0 || len(view.transparent.items) != 0 {
			t.Fatal("partial phase survived failed preparation")
		}
	}
	driver.fail = false
	if err := prepare.Run(world); err != nil {
		t.Fatal(err)
	}
	if len(main.opaque.items) != 2 || len(main.transparent.items) != 1 || len(picking.opaque.items) != 2 || len(picking.transparent.items) != 0 {
		t.Fatalf("phase counts: main=%d/%d picking=%d/%d", len(main.opaque.items), len(main.transparent.items), len(picking.opaque.items), len(picking.transparent.items))
	}
	if len(main.shadow.views[0].opaque.items) != 0 || len(main.shadow.views[0].transparent.items) != 0 {
		t.Fatal("shadow incorrectly queued into main phases")
	}
	if !main.opaque.items[0].textureHandle.Valid() || !picking.opaque.items[1].textureHandle.Valid() {
		t.Fatal("queued item has no prepared texture")
	}
}
