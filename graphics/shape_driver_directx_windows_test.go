//go:build windows && cgo && !onix_metal && !onix_vulkan && !onix_opengl && !onix_webgpu && !onix_webgl

package graphics

import "github.com/diiyw/onix/render"

func newShapeCoverageDriver(surface render.NativeSurface) (render.Driver, error) {
	return render.NewDirectXDriver(surface)
}
