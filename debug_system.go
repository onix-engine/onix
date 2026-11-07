//go:build !onix_release

package onix

import (
	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/scene"
)

func appendDefaultDebugRenderSystem(systems []ecs.SystemConfig) []ecs.SystemConfig {
	return append(systems, ecs.SystemConfig{
		Name: "render_debug_info", After: []string{"render_cameras"}, System: scene.RenderDebugInfoSystem("").System,
	})
}
