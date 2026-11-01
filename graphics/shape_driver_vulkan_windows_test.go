//go:build windows && cgo && onix_vulkan

package graphics

import "github.com/diiyw/onix/render"

func newShapeCoverageDriver(surface render.NativeSurface) (render.Driver, error) {
	return render.NewVulkanDriver(surface)
}
