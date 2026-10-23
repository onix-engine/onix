//go:build (windows || linux || android) && cgo && (!(linux && onix_drm) || onix_drm_vulkan)

package render

import (
	"fmt"
	"testing"
)

var nativeResourceBenchmarkValue uint32

func benchmarkInsertNativeMesh(d *vulkanDriver, native uint32) MeshHandle {
	return d.meshes.Insert(nativeMesh{native: native})
}

func benchmarkInsertNativeTarget(d *vulkanDriver, native uint32) RenderTargetHandle {
	return d.targets.Insert(nativeRenderTarget{native: native, color: NewHandle[TextureResource](native, 1)})
}

// Exercise the actual driver registries without allocating GPU resources.
func BenchmarkNativeResourceRegistry(b *testing.B) {
	for _, count := range []int{1, 1024} {
		b.Run(fmt.Sprintf("meshes=%d", count), func(b *testing.B) {
			var d vulkanDriver
			handles := make([]MeshHandle, count)
			for i := range handles {
				handles[i] = benchmarkInsertNativeMesh(&d, uint32(i+1))
			}
			for _, handle := range handles {
				d.meshes.Remove(handle)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				for i := range handles {
					handles[i] = benchmarkInsertNativeMesh(&d, uint32(i+1))
				}
				for _, handle := range handles {
					value := d.meshes.getRef(handle)
					nativeResourceBenchmarkValue ^= value.native
					d.meshes.Remove(handle)
				}
			}
		})
		b.Run(fmt.Sprintf("targets=%d", count), func(b *testing.B) {
			var d vulkanDriver
			handles := make([]RenderTargetHandle, count)
			for i := range handles {
				handles[i] = benchmarkInsertNativeTarget(&d, uint32(i+1))
			}
			for _, handle := range handles {
				d.targets.Remove(handle)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				for i := range handles {
					handles[i] = benchmarkInsertNativeTarget(&d, uint32(i+1))
				}
				for _, handle := range handles {
					value := d.targets.getRef(handle)
					nativeResourceBenchmarkValue ^= value.native
					d.targets.Remove(handle)
				}
			}
		})
	}
}

func BenchmarkNativeMeshLookup(b *testing.B) {
	var d vulkanDriver
	var handles [1024]MeshHandle
	for i := range handles {
		handles[i] = benchmarkInsertNativeMesh(&d, uint32(i+1))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		value := d.meshes.getRef(handles[i%len(handles)])
		nativeResourceBenchmarkValue ^= value.native
	}
}
