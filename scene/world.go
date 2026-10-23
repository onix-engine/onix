package scene

import "github.com/diiyw/onix/ecs"

func requireWorld(world *ecs.World) {
	if world == nil {
		panic("scene: world is nil")
	}
}
