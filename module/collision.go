package module

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/physics"
)

// Geometry uses solver world coordinates. Element is the sender's private name;
// OtherType is the collider's authored category, never an external entity handle.
type Collision2D struct {
	Element         string
	Phase           physics.CollisionPhase
	Type, OtherType uint32
	Normal, Point   [2]float32
	Penetration     float32
}

type Collision3D struct {
	Element         string
	Phase           physics.CollisionPhase
	Type, OtherType uint32
	Normal, Point   [3]float32
	Penetration     float32
}

type collisionEmitter[T any] struct {
	emit    Emitter[T]
	element string
}

func (r *Runtime) bindCollision(c *Context, entity ecs.Entity, element, port string, threeD bool) error {
	if threeD {
		emit, err := c.BindEmitter[Collision3D](port)
		if err != nil {
			return err
		}
		return r.world.Insert(entity, collisionEmitter[Collision3D]{emit, element})
	}
	emit, err := c.BindEmitter[Collision2D](port)
	if err != nil {
		return err
	}
	return r.world.Insert(entity, collisionEmitter[Collision2D]{emit, element})
}

// PublishCollisions runs after physics. It queues the next module batch, so a
// collision callback cannot mutate the solver or recurse into this simulation.
func (r *Runtime) PublishCollisions() error {
	if r.closed {
		return ErrDisposed
	}
	if r.busy {
		return ErrBusy
	}
	var result error
	for event := range r.collisions2D.All(r.world) {
		binding := r.world.GetRef[collisionEmitter[Collision2D]](event.Entity)
		if binding == nil || !collisionSourceActive(binding.emit.context) {
			continue
		}
		result = joinError(result, binding.emit.Emit(Collision2D{Element: binding.element, Phase: event.Phase, Type: event.CollisionType, OtherType: event.OtherType, Normal: [2]float32{float32(event.Normal.X), float32(event.Normal.Y)}, Point: [2]float32{float32(event.Point.X), float32(event.Point.Y)}, Penetration: float32(event.Penetration)}))
	}
	for event := range r.collisions3D.All(r.world) {
		binding := r.world.GetRef[collisionEmitter[Collision3D]](event.Entity)
		if binding == nil || !collisionSourceActive(binding.emit.context) {
			continue
		}
		result = joinError(result, binding.emit.Emit(Collision3D{Element: binding.element, Phase: event.Phase, Type: event.CollisionType, OtherType: event.OtherType, Normal: [3]float32{float32(event.Normal.X), float32(event.Normal.Y), float32(event.Normal.Z)}, Point: [3]float32{float32(event.Point.X), float32(event.Point.Y), float32(event.Point.Z)}, Penetration: float32(event.Penetration)}))
	}
	return result
}

func collisionSourceActive(c *Context) bool {
	return c.usable() && c.enabled && c.scene.state == SceneActive
}
