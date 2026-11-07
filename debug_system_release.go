//go:build onix_release

package onix

import "github.com/onix-engine/onix/ecs"

func appendDefaultDebugRenderSystem(systems []ecs.SystemConfig) []ecs.SystemConfig {
	return systems
}
