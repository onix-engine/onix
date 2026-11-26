//go:build !onix_release && cgo && (darwin || ios) && !onix_directx && !onix_vulkan && !onix_opengl && !onix_webgpu && !onix_webgl

package render

/*
#cgo noescape onix_metal_target_size
#cgo nocallback onix_metal_target_size
#include "metal_native.h"
*/
import "C"

// VisitRenderTargetSizes samples drawable pixel dimensions without acquiring
// a drawable or realizing a lazy target. Call on the rendering thread.
func (d *metalDriver) VisitRenderTargetSizes(visit func(kind string, id, width, height uint32)) {
	report := func(kind string, nativeID int64, id uint32) {
		var width, height C.uint32_t
		if C.onix_metal_target_size(d.native, C.int64_t(nativeID), &width, &height) != 0 {
			visit(kind, id, uint32(width), uint32(height))
		}
	}
	report("window", 0, 0)
	report("caret", -1, 0)
	for target := range d.targets.Range {
		report("offscreen", int64(target.native), target.native)
	}
}
