package ui

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

// FocusControl moves native keyboard focus to a control. Passing zero clears
// focus. Applications can use it after opening a search bar or switching tabs.
func FocusControl(world *ecs.World, entity ecs.Entity) bool {
	if world == nil {
		return false
	}
	if entity != 0 {
		if !world.IsAlive(entity) || focusTarget(world, entity) != entity {
			return false
		}
		if target, ok := ecs.Get[scene.InputTarget2D](world, entity); ok && target.Disabled {
			return false
		}
		if !scene.IsInteractive(world, entity) {
			return false
		}
	}
	state, ok := ecs.GetResource[*uiEventState](world)
	if !ok || state == nil {
		state = &uiEventState{}
		ecs.InsertResource(world, state)
	}
	focusWithVisibility(world, state, entity, true)
	return true
}
