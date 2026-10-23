package ui

import "github.com/diiyw/onix/ecs"

// LayoutPending reports whether nested constraints need another settling pass.
func LayoutPending(world *ecs.World) bool {
	return ecs.FrameValue[uiLayoutScratchTag, uiLayoutScratch](world).pending
}
