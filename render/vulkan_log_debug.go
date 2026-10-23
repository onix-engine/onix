//go:build (windows || linux || android) && cgo && !onix_release

package render

import "log/slog"

func logSelectedVulkanDevice(name string) {
	slog.Debug("selected Vulkan device", "name", name)
}
