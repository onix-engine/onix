//go:build !onix_release

package onix

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func appendDefaultDebugRenderSystem(systems []ecs.SystemConfig) []ecs.SystemConfig {
	return append(systems, ecs.SystemConfig{
		Name: "render_debug_info", After: []string{"render_cameras"}, System: scene.RenderDebugInfoSystem("").System,
	})
}
