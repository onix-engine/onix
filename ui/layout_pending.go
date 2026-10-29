package ui

import "github.com/diiyw/onix/ecs"

// LayoutPending reports whether nested constraints need another settling pass.
func LayoutPending(world *ecs.World) bool {
	return world.FrameValue[uiLayoutScratchTag, uiLayoutScratch]().pending
}
