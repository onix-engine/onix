package module

import (
	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/physics"
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

type collisionEvent interface{ Collision2D | Collision3D }

type collisionEmitter[T collisionEvent] struct {
	emit    Emitter[T]
	element string
}

type collisionBridge struct {
	bind    func(*Context, ecs.Entity, string, string) error
	publish func(*Runtime) error
}

var (
	collisionBridge2D = collisionBridge{bindCollision[Collision2D], (*Runtime).publishCollisions2D}
	collisionBridge3D = collisionBridge{bindCollision[Collision3D], (*Runtime).publishCollisions3D}
)

func bindCollision[T collisionEvent](c *Context, entity ecs.Entity, element, port string) error {
	emit, err := c.BindEmitter[T](port)
	if err != nil {
		return err
	}
	return c.runtime.world.Insert(entity, collisionEmitter[T]{emit, element})
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
	if bridge := r.content.collision2D; bridge != nil {
		result = bridge.publish(r)
	}
	if bridge := r.content.collision3D; bridge != nil {
		result = joinError(result, bridge.publish(r))
	}
	return result
}

func (r *Runtime) publishCollisions2D() error {
	var result error
	for event := range r.collisions2D.All(r.world) {
		binding := r.world.GetRef[collisionEmitter[Collision2D]](event.Entity)
		if binding == nil || !collisionSourceActive(binding.emit.context) {
			continue
		}
		result = joinError(result, binding.emit.Emit(Collision2D{Element: binding.element, Phase: event.Phase, Type: event.CollisionType, OtherType: event.OtherType, Normal: [2]float32{float32(event.Normal.X), float32(event.Normal.Y)}, Point: [2]float32{float32(event.Point.X), float32(event.Point.Y)}, Penetration: float32(event.Penetration)}))
	}
	return result
}

func (r *Runtime) publishCollisions3D() error {
	var result error
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
