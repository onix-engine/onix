//go:build js && wasm

package render

import (
	"syscall/js"
	"testing"
)

func TestBrowserMeshBatchesAndStaticDeformationPixels(t *testing.T) {
	if !js.Global().Get("document").Truthy() {
		t.Skip("requires a browser")
	}
	canvas := js.Global().Get("document").Call("createElement", "canvas")
	canvas.Set("width", 16)
	canvas.Set("height", 16)
	js.Global().Get("document").Get("body").Call("appendChild", canvas)
	defer canvas.Call("remove")
	driver, _, check := newSpriteBrowserDriver(t, canvas)
	defer driver.Destroy()
	meshes := driver.(MeshDriver)
	targets := driver.(RenderTargetDriver)
	target, _, err := targets.CreateRenderTarget(RenderTargetDescriptor{Width: 16, Height: 16, Depth: true})
	if err != nil {
		t.Fatal(err)
	}
	defer targets.DestroyRenderTarget(target)
	vertices := []Vertex3D{{Position: [3]float32{-.3, -.3, .5}, Normal: [3]float32{0, 0, 1}}, {Position: [3]float32{.3, -.3, .5}, Normal: [3]float32{0, 0, 1}}, {Position: [3]float32{.3, .3, .5}, Normal: [3]float32{0, 0, 1}}, {Position: [3]float32{-.3, .3, .5}, Normal: [3]float32{0, 0, 1}}}
	mesh, err := meshes.CreateMesh(vertices, []uint32{0, 1, 2, 0, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	defer meshes.DestroyMesh(mesh)
	var textures [2]TextureHandle
	for index, color := range [2][4]byte{{255, 0, 0, 255}, {0, 255, 0, 255}} {
		textures[index], err = driver.CreateTexture(TextureDescriptor{Width: 1, Height: 1}, color[:])
		if err != nil {
			t.Fatal(err)
		}
		defer driver.DestroyTexture(textures[index])
	}
	shader, err := driver.(MeshShaderDriver).CreateMeshShader(meshBrowserFragment())
	if err != nil {
		t.Fatal(err)
	}
	defer driver.(MeshShaderDriver).DestroyMeshShader(shader)
	identity := [16]float32{0: 1, 5: 1, 10: 1, 15: 1}
	batch := MeshBatch3D{ViewProjection: identity, Instances: make([]Instance3D, 2), Draws: []MeshDraw3D{{Mesh: mesh, Texture: textures[0], IndexCount: 6, InstanceCount: 1, AlphaMode: 2}, {Mesh: mesh, Texture: textures[1], IndexCount: 6, FirstInstance: 1, InstanceCount: 1, AlphaMode: 2}}}
	gpu, gpuEnabled := driver.(MeshGPUDeformationDriver)
	if gpuEnabled {
		skin := make([]MeshSkinVertex, 4)
		morph := make([]MeshMorphVertex, 4)
		for index := range skin {
			skin[index].Weights[0] = 1
			morph[index].Position[1] = .4
		}
		if err := gpu.SetMeshDeformation(mesh, MeshDeformationDescriptor{Skin: skin, MorphTargets: morph, VertexCount: 4, TargetCount: 1}); err != nil {
			t.Fatal(err)
		}
	}
	for frame := range 8 {
		for index := range batch.Instances {
			batch.Instances[index] = Instance3D{Model: identity, BaseColor: [4]float32{1, 1, 1, 1}, Material: [4]float32{-1, 1.5, .5}, UVTransform: [4]float32{1, 1}}
			batch.Instances[index].Model[12] = float32(index) - .5
		}
		batch.Draws[1].Shader = MeshShaderHandle{}
		if frame%2 == 1 {
			batch.Draws[1].Shader = shader
		}
		if err := driver.BeginFrame(Color{}); err != nil {
			t.Fatal(err)
		}
		if err := targets.BeginRenderTarget(target, RenderTargetClear{ClearColor: true, ClearDepth: true}); err != nil {
			t.Fatal(err)
		}
		if err := meshes.DrawMeshes(batch); err != nil {
			t.Fatal(err)
		}
		if gpuEnabled {
			joint := identity
			joint[12] = -.125
			if frame%2 == 1 {
				joint[12] = .125
			}
			batch.Deformations = []MeshDeformationData{{Mesh: mesh, JointMatrices: [][16]float32{joint}, MorphWeights: []float32{.5}}, {Mesh: mesh, JointMatrices: [][16]float32{joint}, MorphWeights: []float32{.5}}}
			for index := range batch.Draws {
				batch.Draws[index].DeformationIndex = uint32(index + 1)
			}
			// Two animated batches share material, mesh and storage size, but not offsets.
			for range 2 {
				if err := gpu.DrawMeshesDeformed(batch); err != nil {
					t.Fatal(err)
				}
			}
			for index := range batch.Draws {
				batch.Draws[index].DeformationIndex = 0
			}
			batch.Deformations = nil
		} else {
			if err := meshes.DrawMeshes(batch); err != nil {
				t.Fatal(err)
			}
		}
		if err := targets.EndRenderTarget(); err != nil {
			t.Fatal(err)
		}
		if err := driver.Present(); err != nil {
			t.Fatal(err)
		}
		check()
		expected := [2][4]byte{{255, 0, 0, 255}, {0, 255, 0, 255}}
		if frame%2 == 1 {
			expected[1] = [4]byte{0, 0, 255, 255}
		}
		for index, want := range expected {
			var pixel [4]byte
			if err := driver.(RenderTargetReadbackDriver).ReadRenderTarget(target, RenderTargetRegion{X: 4 + index*8, Y: 8, Width: 1, Height: 1}, pixel[:]); err != nil {
				t.Fatal(err)
			}
			for channel := range pixel {
				if delta := int(pixel[channel]) - int(want[channel]); delta < -1 || delta > 1 {
					t.Fatalf("frame %d pixel %d=%v want %v", frame, index, pixel, want)
				}
			}
		}
		if gpuEnabled {
			// These pixels lie outside the base mesh. The upper one requires
			// morph deltas, and the left one follows the animated joint pose.
			for sample, region := range [...]RenderTargetRegion{{X: 4, Y: 4, Width: 1, Height: 1}, {X: 1, Y: 8, Width: 1, Height: 1}} {
				want := [4]byte{255, 0, 0, 255}
				if sample == 1 && frame%2 == 1 {
					want = [4]byte{}
				}
				var pixel [4]byte
				if err := driver.(RenderTargetReadbackDriver).ReadRenderTarget(target, region, pixel[:]); err != nil {
					t.Fatal(err)
				}
				if pixel != want {
					t.Fatalf("frame %d deformation pixel %d=%v want %v", frame, sample, pixel, want)
				}
			}
		}
		check()
	}
}
