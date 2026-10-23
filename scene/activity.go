package scene

import "github.com/diiyw/onix/ecs"

// Activity controls simulation independently of visibility. An entity may be
// invisible while running; suspended scene content must do neither.
type Activity struct{ Active bool }

func IsActive(world *ecs.World, entity ecs.Entity) bool {
	return inheritedActivity(world, entity, true, false)
}

// IsInteractive requires both activity and visibility along the parent chain.
// Hidden UI cannot retain keyboard focus or receive semantic form input.
func IsInteractive(world *ecs.World, entity ecs.Entity) bool {
	return inheritedActivity(world, entity, true, true)
}

// IsVisible allows paused overlay backgrounds to render without enabling
// their simulation or input. Query scopes determine scene participation.
func IsVisible(world *ecs.World, entity ecs.Entity) bool {
	return inheritedActivity(world, entity, false, true)
}

func inheritedActivity(world *ecs.World, entity ecs.Entity, active, visible bool) bool {
	for remaining := world.EntityCount() + 1; entity != 0 && remaining > 0; remaining-- {
		if active {
			if value, ok := ecs.Get[Activity](world, entity); ok && !value.Active {
				return false
			}
		}
		if visible {
			if value, ok := ecs.Get[Visibility](world, entity); ok && !value.Visible {
				return false
			}
		}
		parent, ok := ecs.Get[Parent](world, entity)
		if !ok {
			return true
		}
		entity = parent.Entity
	}
	return entity == 0
}
