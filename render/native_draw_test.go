//go:build (darwin || ios || windows || linux || android) && cgo

package render

import (
	"fmt"
	"testing"
	"unsafe"
)

func TestNativeUniformLayout(t *testing.T) {
	var uniforms nativeMeshUniforms
	if unsafe.Sizeof(uniforms) != meshLightUniformByteCount {
		t.Fatalf("native uniform size = %d, want %d", unsafe.Sizeof(uniforms), meshLightUniformByteCount)
	}
	for _, field := range []struct{ offset, floats uintptr }{
		{unsafe.Offsetof(uniforms.view_projection), 0},
		{unsafe.Offsetof(uniforms.camera_position), 16},
		{unsafe.Offsetof(uniforms.ambient_light), 20},
		{unsafe.Offsetof(uniforms.light_direction), 24},
		{unsafe.Offsetof(uniforms.directional_light), 28},
		{unsafe.Offsetof(uniforms.light_position), 32},
		{unsafe.Offsetof(uniforms.light_directions), 32 + MaxMeshLights*4},
		{unsafe.Offsetof(uniforms.light_colors), 32 + MaxMeshLights*8},
		{unsafe.Offsetof(uniforms.light_params), 32 + MaxMeshLights*12},
		{unsafe.Offsetof(uniforms.light_meta), 32 + MaxMeshLights*16},
		{unsafe.Offsetof(uniforms.lighting), meshLightingOffset},
		{unsafe.Offsetof(uniforms.light_right), meshLightRightOffset},
		{unsafe.Offsetof(uniforms.light_up), meshLightUpOffset},
		{unsafe.Offsetof(uniforms.shadow_matrices), meshShadowMatrixOffset},
		{unsafe.Offsetof(uniforms.shadow_rects), meshShadowRectOffset},
		{unsafe.Offsetof(uniforms.shadow_params), meshShadowParamsOffset},
		{unsafe.Offsetof(uniforms.light_shadows), meshLightShadowOffset},
		{unsafe.Offsetof(uniforms.occlusion_inverse), meshOcclusionInverseOffset},
	} {
		if field.offset != field.floats*4 {
			t.Fatalf("native uniform offset = %d, want %d", field.offset, field.floats*4)
		}
	}
	var d nativeDrawState
	if unsafe.Sizeof(d.draws[0]) != 28 || unsafe.Sizeof(d.draws3D[0]) != 104 {
		t.Fatal("native draw ABI changed")
	}
	data := nativeMeshUniformData(&uniforms)
	data[0], data[meshLightUniformFloatCount-1] = 3, 9
	batch := MeshBatch3D{CameraPosition: [4]float32{1, 2, 3, 4}, Lights: []Light3D{{Kind: Light3DPoint}}}
	if count := writeMeshSceneUniforms(data, &batch); count != 1 || uniforms.view_projection[0] != 3 || uniforms.camera_position[3] != 4 || uniforms.light_meta[0] != 1 || uniforms.occlusion_inverse[15] != 0 {
		t.Fatal("uniforms were not written directly into the native block")
	}
	if allocations := testing.AllocsPerRun(100, func() { writeMeshSceneUniforms(nativeMeshUniformData(&uniforms), &batch) }); allocations != 0 {
		t.Fatal("native uniform packing allocated", allocations)
	}
}

func TestNativeDrawPackingReusesStorageAndRejectsStaleHandles(t *testing.T) {
	var d nativeDrawState
	texture := d.textures.Insert(41)
	other := d.textures.Insert(73)
	shader := d.shaders2D.Insert(29)
	sprites := SpriteBatch{Draws: []SpriteDraw{{Shader: shader, Texture: texture, AdditionalTextures: [3]TextureHandle{other}, FirstInstance: 2, InstanceCount: 3}}}
	count, err := d.packSpriteDraws(&sprites, nativeMetal)
	if err != nil || count != 3 {
		t.Fatalf("pack sprites = %d, %v", count, err)
	}
	sprite := d.draws[0]
	if sprite.shader != 29 || sprite.texture != 41 || sprite.additional_textures[0] != 73 || sprite.additional_textures[1] != 41 || sprite.additional_textures[2] != 41 || sprite.first_instance != 2 || sprite.instance_count != 3 {
		t.Fatal("sprite packing changed resource bindings or instance range")
	}
	mesh := d.meshes.Insert(nativeMesh{native: 53})
	meshShader := d.shaders.Insert(19)
	batch := MeshBatch3D{Draws: []MeshDraw3D{{Mesh: mesh, Shader: meshShader, Texture: texture, NormalTexture: other, MetallicRoughnessTexture: other,
		FirstIndex: 2, IndexCount: 5, BaseVertex: -1, FirstInstance: 1, InstanceCount: 2, AlphaMode: 2, BlendMode: MeshBlendAdditive, DeformationIndex: 7}},
		Lighting: MeshLighting{EnvironmentTexture: other}}
	counts, err := d.packMeshDraws(&batch, nativeVulkan, true)
	if err != nil || counts != (meshDrawStatistics{instances: 2, vertices: 10, triangles: 2}) {
		t.Fatalf("pack meshes = %+v, %v", counts, err)
	}
	draw := d.draws3D[0]
	if draw.mesh != 53 || draw.shader != 19 || draw.texture != 41 || draw.normal_texture != 73 || draw.metallic_roughness_texture != 73 || draw.extra_textures[4] != 73 || draw.first_index != 2 || draw.index_count != 5 || draw.base_vertex != -1 || draw.first_instance != 1 || draw.instance_count != 2 || draw.alpha_mode != 2 || draw.blend_mode != 1 || draw.deformation_index != 7 {
		t.Fatal("mesh packing changed native resource IDs, draw range or material")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if _, err := d.packSpriteDraws(&sprites, nativeMetal); err != nil {
			panic(err)
		}
		if _, err := d.packMeshDraws(&batch, nativeVulkan, false); err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatal("warmed native draw packing allocated", allocations)
	}
	if d.draws3D[0].deformation_index != 0 {
		t.Fatal("ordinary drawing retained a deformation index")
	}
	d.shaders.Remove(meshShader)
	d.textures.Remove(texture)
	if err := d.packMeshPickingDraws(&batch, nativeMetal); err != nil {
		t.Fatal("picking resolved material resources", err)
	}
	if draw := d.draws3D[0]; draw.mesh != 53 || draw.texture != 0 || draw.shader != 0 || draw.deformation_index != 0 {
		t.Fatal("picking retained material state")
	}
	if _, err := d.packSpriteDraws(&sprites, nativeMetal); err == nil {
		t.Fatal("sprite accepted a stale texture")
	}
	d.meshes.Remove(mesh)
	if err := d.packMeshPickingDraws(&batch, nativeVulkan); err == nil || err.Error() != fmt.Sprintf("render: invalid Vulkan mesh handle %s", mesh) {
		t.Fatal("picking did not report the stale mesh", err)
	}
	batch.Draws[0].Mesh = MeshHandle{}
	for _, backend := range []nativeBackend{nativeMetal, nativeVulkan} {
		if _, err := d.packMeshDraws(&batch, backend, false); err == nil || err.Error() != "render: "+backend.name()+" mesh draw has no geometry" {
			t.Fatal("missing geometry lost its backend context", err)
		}
	}
}

func BenchmarkNativeDrawPacking(b *testing.B) {
	for _, count := range []int{1, 1024} {
		b.Run(fmt.Sprintf("draws=%d", count), func(b *testing.B) {
			var d nativeDrawState
			texture := d.textures.Insert(41)
			mesh := d.meshes.Insert(nativeMesh{native: 53})
			batch := MeshBatch3D{Draws: make([]MeshDraw3D, count)}
			for i := range batch.Draws {
				batch.Draws[i] = MeshDraw3D{Mesh: mesh, Texture: texture, IndexCount: 3, InstanceCount: 1}
			}
			if _, err := d.packMeshDraws(&batch, nativeVulkan, false); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, err := d.packMeshDraws(&batch, nativeVulkan, false); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkNativeUniformPacking(b *testing.B) {
	var uniforms nativeMeshUniforms
	batch := MeshBatch3D{Lights: make([]Light3D, MaxMeshLights)}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		data := nativeMeshUniformData(&uniforms)
		copy(data[:16], batch.ViewProjection[:])
		writeMeshSceneUniforms(data, &batch)
	}
}

func TestNativeDrawScratchTrimsAfterScenePeak(t *testing.T) {
	var d nativeDrawState
	texture := d.textures.Insert(1)
	mesh := d.meshes.Insert(nativeMesh{native: 1})
	sprites := SpriteBatch{Draws: make([]SpriteDraw, 4096)}
	meshes := MeshBatch3D{Draws: make([]MeshDraw3D, 4096)}
	for i := range sprites.Draws {
		sprites.Draws[i] = SpriteDraw{Texture: texture, InstanceCount: 1}
		meshes.Draws[i] = MeshDraw3D{Mesh: mesh, Texture: texture, IndexCount: 3, InstanceCount: 1}
	}
	pack := func() {
		t.Helper()
		if _, err := d.packSpriteDraws(&sprites, nativeVulkan); err != nil {
			t.Fatal(err)
		}
		if _, err := d.packMeshDraws(&meshes, nativeVulkan, false); err != nil {
			t.Fatal(err)
		}
	}
	pack()
	before := sliceCapacityBytes(d.draws) + sliceCapacityBytes(d.draws3D)
	d.trimScratch()
	sprites.Draws, meshes.Draws = sprites.Draws[:1], meshes.Draws[:1]
	for range bufferTrimDelay {
		pack()
		d.trimScratch()
	}
	after := sliceCapacityBytes(d.draws) + sliceCapacityBytes(d.draws3D)
	if cap(d.draws) != 64 || cap(d.draws3D) != 64 {
		t.Fatalf("retained draw capacities: %d/%d", cap(d.draws), cap(d.draws3D))
	}
	t.Logf("draw scratch: %d -> %d bytes", before, after)
	// A large batch followed by a smaller batch must stay warm, including picking.
	meshes.Draws = meshes.Draws[:4096]
	for range bufferTrimDelay * 2 {
		if err := d.packMeshPickingDraws(&meshes, nativeVulkan); err != nil {
			t.Fatal(err)
		}
		small := meshes
		small.Draws = small.Draws[:1]
		if _, err := d.packMeshDraws(&small, nativeVulkan, false); err != nil {
			t.Fatal(err)
		}
		d.trimScratch()
		if cap(d.draws3D) < 4096 {
			t.Fatal("picking demand was lost")
		}
	}
	for range bufferTrimDelay {
		d.trimScratch()
	}
	if d.draws3D != nil {
		t.Fatal("idle scene retained mesh draw peak")
	}
}
