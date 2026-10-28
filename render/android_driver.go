//go:build android && cgo

package render

func NewAndroidDriver(surface VulkanSurface) (Driver, error) {
	return newDriverWithFallback(surface, "Vulkan", NewVulkanDriver, "OpenGL ES", func(surface VulkanSurface) (Driver, error) {
		return NewOpenGLESDriver(surface.Window, surface.Width, surface.Height)
	})
}
