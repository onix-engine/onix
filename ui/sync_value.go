package ui

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func setUIPosition(world *ecs.World, entity ecs.Entity, x, y float32) {
	if transform, ok := world.Get[scene.UITransform](entity); ok {
		transform.Translation.X.Value, transform.Translation.Y.Value = x, y
		world.SetIfChanged(entity, transform)
	}
}

type syncControlStateTag struct{}
type syncBlockerStateTag struct{}
type componentSyncState struct {
	versions    [4]uint64
	initialized bool
}
