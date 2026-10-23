//go:build android && cgo

package render

/*
#cgo LDFLAGS: -landroid
#include <android/log.h>

static void onix_android_log_renderer(const char *backend, int backend_length, const char *device, int device_length) {
	__android_log_print(ANDROID_LOG_INFO, "Onix", "selected renderer: %.*s (%.*s)", backend_length, backend, device_length, device);
}
*/
import "C"

import "unsafe"

func NewAndroidDriver(surface VulkanSurface) (Driver, error) {
	driver, err := newDriverWithFallback(surface, "Vulkan", NewVulkanDriver, "OpenGL ES", func(surface VulkanSurface) (Driver, error) {
		return NewOpenGLESDriver(surface.Window, surface.Width, surface.Height)
	})
	if err == nil {
		info := driver.RendererInfo()
		C.onix_android_log_renderer(
			(*C.char)(unsafe.Pointer(unsafe.StringData(info.Backend))), C.int(len(info.Backend)),
			(*C.char)(unsafe.Pointer(unsafe.StringData(info.Device))), C.int(len(info.Device)),
		)
	}
	return driver, err
}
