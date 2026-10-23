//go:build (windows || linux || android) && cgo && (!(linux && onix_drm) || onix_drm_vulkan)

package render

import "testing"

func TestVulkanMeshPushPreservesCameraProjection(t *testing.T) {
	batch := MeshBatch3D{
		ViewProjection: [16]float32{
			1, 2, 3, 4,
			5, 6, 7, 8,
			9, 10, 11, 12,
			13, 14, 15, 16,
		},
		CameraPosition:   [4]float32{17, 18, 19, 20},
		AmbientLight:     [4]float32{21, 22, 23, 24},
		LightDirection:   [4]float32{25, 26, 27, 28},
		DirectionalLight: [4]float32{29, 30, 31, 32},
	}
	push := vulkanMeshPush(&batch)
	for i, want := range batch.ViewProjection {
		if got := float32(push.view_projection[i]); got != want {
			t.Fatalf("projection[%d] = %g, want %g; the shader owns the Vulkan Y flip", i, got, want)
		}
	}
	for i := range 4 {
		if float32(push.camera_position[i]) != batch.CameraPosition[i] ||
			float32(push.ambient_light[i]) != batch.AmbientLight[i] ||
			float32(push.light_direction[i]) != batch.LightDirection[i] ||
			float32(push.directional_light[i]) != batch.DirectionalLight[i] {
			t.Fatalf("camera/light component %d was not preserved", i)
		}
	}
}

func TestVulkanMeshPushDoesNotAllocate(t *testing.T) {
	batch := MeshBatch3D{ViewProjection: [16]float32{0: 1, 5: 1, 10: 1, 15: 1}}
	if allocations := testing.AllocsPerRun(100, func() {
		_ = vulkanMeshPush(&batch)
	}); allocations != 0 {
		t.Fatalf("mesh push allocations = %v, want 0", allocations)
	}
}

func TestVulkanInlineResourcesReuseStorageWithoutAllocating(t *testing.T) {
	var d vulkanDriver
	mesh := d.meshes.Insert(nativeMesh{native: 1})
	target := d.targets.Insert(nativeRenderTarget{native: 1})
	d.meshes.Remove(mesh)
	d.targets.Remove(target)
	if allocations := testing.AllocsPerRun(100, func() {
		mesh := d.meshes.Insert(nativeMesh{native: 2})
		value := d.meshes.getRef(mesh)
		value.deformation = meshDeformationLayout{vertexCount: 3, requiredJoints: 1}
		if layout, ok := d.deformationLayout(mesh); !ok || layout != value.deformation {
			panic("inline mesh update was lost")
		}
		d.meshes.Remove(mesh)
		target := d.targets.Insert(nativeRenderTarget{native: 2, color: NewHandle[TextureResource](2, 1)})
		d.targets.Remove(target)
	}); allocations != 0 {
		t.Fatalf("warm resource registration allocated %g times", allocations)
	}
}
