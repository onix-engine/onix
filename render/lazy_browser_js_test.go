//go:build js && wasm

package render

import (
	"encoding/binary"
	"syscall/js"
	"testing"
)

func TestBrowserLazySurfaceDepthPixels(t *testing.T) {
	for _, antialias := range []bool{false, true} {
		name := "single"
		if antialias {
			name = "msaa"
		}
		t.Run(name, func(t *testing.T) { testBrowserLazySurfaceDepthPixels(t, antialias) })
	}
}

func testBrowserLazySurfaceDepthPixels(t *testing.T, antialias bool) {
	if !js.Global().Get("document").Truthy() {
		t.Skip("requires a browser")
	}
	canvas := js.Global().Get("document").Call("createElement", "canvas")
	canvas.Set("__onixTestAntialias", antialias)
	canvas.Set("width", 16)
	canvas.Set("height", 16)
	js.Global().Get("document").Get("body").Call("appendChild", canvas)
	defer canvas.Call("remove")
	driver, _, check := newSpriteBrowserDriver(t, canvas)
	defer driver.Destroy()
	prepareLazyBrowserCapture(driver)
	assertLazyBrowserResources(t, driver, false, false)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	texture, err := driver.CreateTexture(TextureDescriptor{Width: 1, Height: 1}, []byte{255, 255, 255, 255})
	must(err)
	defer driver.DestroyTexture(texture)
	identity := [16]float32{0: 1, 5: 1, 10: 1, 15: 1}
	sprite := SpriteBatch{Projection: [16]float32{0: .125, 5: -.125, 10: 1, 12: -1, 13: 1, 15: 1}, Instances: []Instance2D{{Model: [6]float32{16, 0, 0, 0, 16, 0}, TextureModel: [6]float32{1, 0, 0, 0, 1, 0}, Color: [4]float32{1, 0, 0, 1}}}, Draws: []SpriteDraw{{Texture: texture, InstanceCount: 1}}}
	for frame := 0; frame < 6; frame++ {
		if frame == 3 {
			canvas.Set("width", 20)
			canvas.Set("height", 20)
			must(driver.ResizeSurface(20, 20))
		}
		must(driver.BeginFrame(Color{}))
		must(driver.DrawSprites(sprite))
		must(driver.Present())
		check()
		assertLazyBrowserResources(t, driver, false, false)
	}
	canvas.Set("width", 16)
	canvas.Set("height", 16)
	must(driver.ResizeSurface(16, 16))
	driver.SetViewport(Rect{Width: 16, Height: 16})
	meshes := driver.(MeshDriver)
	targets := driver.(RenderTargetDriver)
	target, _, err := targets.CreateRenderTarget(RenderTargetDescriptor{Width: 16, Height: 16, Depth: true})
	must(err)
	defer targets.DestroyRenderTarget(target)
	mesh, err := meshes.CreateMesh([]Vertex3D{{Position: [3]float32{-.5, -.5, .75}, Normal: [3]float32{0, 0, 1}}, {Position: [3]float32{.5, -.5, .75}, Normal: [3]float32{0, 0, 1}}, {Position: [3]float32{.5, .5, .75}, Normal: [3]float32{0, 0, 1}}, {Position: [3]float32{-.5, .5, .75}, Normal: [3]float32{0, 0, 1}}}, []uint32{0, 1, 2, 0, 2, 3})
	must(err)
	defer meshes.DestroyMesh(mesh)
	batch := MeshBatch3D{ViewProjection: identity, Instances: []Instance3D{{Model: identity, BaseColor: [4]float32{0, 1, 0, 1}, Material: [4]float32{-1, 1.5, .5, 256}, UVTransform: [4]float32{1, 1}}}, Draws: []MeshDraw3D{{Mesh: mesh, Texture: texture, IndexCount: 6, InstanceCount: 1}}}
	// Offscreen 3D must leave the window depthless.
	must(driver.BeginFrame(Color{}))
	must(targets.BeginRenderTarget(target, RenderTargetClear{ClearColor: true, ClearDepth: true}))
	must(meshes.DrawMeshes(batch))
	must(targets.EndRenderTarget())
	must(driver.Present())
	check()
	assertLazyBrowserResources(t, driver, false, true)
	for frame := 0; frame < 6; frame++ {
		size := 16
		if frame >= 3 {
			size = 20
		}
		if frame == 3 {
			canvas.Set("width", size)
			canvas.Set("height", size)
			must(driver.ResizeSurface(size, size))
		}
		driver.SetViewport(Rect{Width: int32(size), Height: int32(size)})
		sprite.Projection[0] = 2 / float32(size)
		sprite.Projection[5] = -2 / float32(size)
		sprite.Instances[0].Model = [6]float32{float32(size), 0, 0, 0, float32(size), 0}
		sprite.Instances[0].Color = [4]float32{1, 0, 0, 1}
		must(driver.BeginFrame(Color{}))
		must(driver.DrawSprites(sprite))
		driver.SetScissor(&Rect{X: 4, Y: 4, Width: int32(size - 8), Height: int32(size - 8)})
		batch.Instances[0].Model = identity
		batch.Instances[0].BaseColor = [4]float32{0, 1, 0, 1}
		must(meshes.DrawMeshes(batch))
		driver.SetScissor(nil)
		must(targets.BeginRenderTarget(target, RenderTargetClear{ClearColor: true, ClearDepth: true}))
		must(meshes.DrawMeshes(batch))
		must(targets.EndRenderTarget())
		driver.SetViewport(Rect{Width: int32(size), Height: int32(size)})
		batch.Instances[0].Model[14] = -.5
		batch.Instances[0].BaseColor = [4]float32{0, 0, 1, 1}
		must(meshes.DrawMeshes(batch))
		sprite.Instances[0].Model = [6]float32{4, 0, 0, 0, 4, 0}
		sprite.Instances[0].Color = [4]float32{0, 0, 1, 1}
		must(driver.DrawSprites(sprite))
		read := captureLazyBrowserSurface(t, driver)
		must(driver.Present())
		pixels := read()
		check()
		assertLazyBrowserResources(t, driver, true, true)
		for _, sample := range []struct {
			x, y  int
			color [4]byte
		}{{size - 2, size - 2, [4]byte{255, 0, 0, 255}}, {size / 2, size / 2, [4]byte{0, 255, 0, 255}}, {2, 2, [4]byte{0, 0, 255, 255}}} {
			offset := (sample.y*size + sample.x) * 4
			for channel, want := range sample.color {
				if delta := int(pixels[offset+channel]) - int(want); delta < -1 || delta > 1 {
					t.Fatalf("frame %d pixel (%d,%d)=%v want %v", frame, sample.x, sample.y, pixels[offset:offset+4], sample.color)
				}
			}
		}
	}
	picking, _, err := targets.CreateRenderTarget(RenderTargetDescriptor{Width: 16, Height: 16, Depth: true, ColorFormat: RenderTargetUint32})
	must(err)
	defer targets.DestroyRenderTarget(picking)
	batch.Instances[0].Model = identity
	batch.Instances[0].SetPickingID(0x123abc)
	var pickingPipeline js.Value
	for frame := 0; frame < 2; frame++ {
		must(driver.BeginFrame(Color{}))
		driver.SetViewport(Rect{Width: 16, Height: 16})
		must(targets.BeginRenderTarget(picking, RenderTargetClear{ClearColor: true, ClearDepth: true}))
		must(driver.(MeshPickingDriver).DrawMeshPicking(batch))
		must(targets.EndRenderTarget())
		must(driver.Present())
		check()
		var pixel [4]byte
		must(driver.(RenderTargetReadbackDriver).ReadRenderTarget(picking, RenderTargetRegion{X: 8, Y: 8, Width: 1, Height: 1}, pixel[:]))
		if binary.LittleEndian.Uint32(pixel[:]) != 0x123abc {
			t.Fatalf("picking pixel=%x", pixel)
		}
		pipeline := lazyBrowserPickingPipeline(driver)
		if !pipeline.Truthy() || frame > 0 && !pipeline.Equal(pickingPipeline) {
			t.Fatal("picking pipeline was not cached")
		}
		pickingPipeline = pipeline
		check()
	}
	t.Log("depthless 2D/resize, offscreen isolation, 2D→3D color preservation, scissor, reversed depth, target resume, 2D overlay, presentation and lazy cached picking passed")
}
