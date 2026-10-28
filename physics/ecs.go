package physics

import (
	"math"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

type Runtime2D struct {
	Simulation    *World
	frame         uint64
	syncedInputs  physicsInputs
	managedBodies int
}

func NewRuntime2D() *Runtime2D {
	return &Runtime2D{Simulation: NewWorld()}
}

func StepSystem2D(name string) ecs.SystemConfig {
	if name == "" {
		name = "step_physics_2d"
	}
	return ecs.SystemConfig{Name: name, System: stepECS2D}
}

func stepECS2D(context *ecs.SystemContext) error {
	if context == nil || context.World == nil {
		return nil
	}
	world := context.World
	runtime, ok := ecs.GetResource[*Runtime2D](world)
	if !ok || runtime == nil {
		runtime = NewRuntime2D()
		ecs.InsertResource(world, runtime)
	}
	if runtime.Simulation == nil {
		runtime.Simulation = NewWorld()
	}
	runtime.frame++
	if runtime.frame == 0 {
		runtime.frame = 1
	}
	if err := runtime.syncBodies(world); err != nil {
		return err
	}
	syncECSColliders[Circle](runtime, world)
	syncECSColliders[Box](runtime, world)
	syncECSColliders[Segment](runtime, world)
	runtime.sweepManaged()
	delta := 1.0 / 60
	if time, ok := ecs.GetResource[*scene.Time](world); ok && time != nil && time.Delta > 0 {
		delta = time.Delta.Seconds()
	}
	runtime.Simulation.Step(delta)
	runtime.writeBodies(world)
	runtime.writeEvents(world)
	runtime.syncedInputs = physicsInputs2D(world)
	return nil
}

func (runtime *Runtime2D) syncBodies(world *ecs.World) error {
	runtime.managedBodies = 0
	for entity, access := range ecs.Query2[RigidBody2D, scene.Transform](world).All {
		if !scene.IsActive(world, entity) {
			continue
		}
		runtime.managedBodies++
		bodyAccess := access.A
		transformAccess := access.B

		value := bodyAccess.Get()
		component, transform := &value, transformAccess.Ref()
		component.normalize()
		pose, err := ecsWorldPose(world, entity, *transform, true)
		if err != nil {
			return err
		}
		position := Vector{X: float64(pose.Translation.X()), Y: float64(pose.Translation.Y())}
		rotation := transformRotationZ(pose)
		if !component.body.Valid() || component.body.world != runtime.Simulation {
			def := NewBodyDef(component.Type)
			def.Position, def.Rotation = position, rotation
			def.Velocity, def.AngularVelocity = component.Velocity, component.AngularVelocity
			def.Mass, def.Inertia, def.GravityScale = component.Mass, component.Inertia, component.GravityScale
			def.LinearDamping, def.AngularDamping = component.LinearDamping, component.AngularDamping
			def.FixedRotation, def.UserData = component.FixedRotation, uint64(entity)
			component.body = runtime.Simulation.CreateBody(def)
			component.body.state().managed = true
			component.initialized = true
		} else {
			state := component.body.state()
			if component.Type != BodyDynamic || position != component.lastPosition || rotation != component.lastRotation {
				state.position, state.rotation = position, rotation
			}
			state.bodyType, state.mass = component.Type, component.Mass
			state.gravityScale = component.GravityScale
			state.linearDamping, state.angularDamping = component.LinearDamping, component.AngularDamping
			state.fixedRotation = component.FixedRotation
			if component.Inertia > 0 {
				state.inertia = component.Inertia
			}
			state.velocity, state.angularVelocity = component.Velocity, component.AngularVelocity
			state.updateInverseMass()
		}
		state := component.body.state()
		state.touched = runtime.frame
		state.force = state.force.Add(component.Force)
		state.torque += component.Torque
		if component.Impulse != (Vector{}) && state.bodyType == BodyDynamic {
			state.velocity = state.velocity.Add(component.Impulse.Mul(state.inverseMass))
		}
		component.Force, component.Torque, component.Impulse = Vector{}, 0, Vector{}
		component.lastPosition, component.lastRotation = state.position, state.rotation
		if value != bodyAccess.Get() {
			*bodyAccess.Mut() = value
		}
	}
	return nil
}

func syncECSColliders[S Shape2D](runtime *Runtime2D, world *ecs.World) {
	for entity, access := range ecs.Query2[RigidBody2D, Collider2D[S]](world).All {
		if !scene.IsActive(world, entity) {
			continue
		}
		bodyAccess := access.A
		colliderAccess := access.B

		body, collider := bodyAccess.Ref(), colliderAccess.Ref()
		state := body.body.state()
		if state == nil || body.body.world != runtime.Simulation || state.touched != runtime.frame {
			continue
		}
		if !collider.handle.Valid() || collider.handle.world != runtime.Simulation {
			definition := ColliderDef[S]{
				Shape: collider.Shape, Friction: max(collider.Friction, 0),
				Restitution: clamp(collider.Restitution, 0, 1), Sensor: collider.Sensor,
				Filter: collider.Filter.normalized(), CollisionType: collider.CollisionType,
			}
			collider = colliderAccess.Mut()
			collider.handle = AddCollider(runtime.Simulation, body.body, definition)
			runtime.Simulation.colliderState(collider.handle.id).managed = true
		} else {
			state := runtime.Simulation.colliderState(collider.handle.id)
			setColliderShape(state, collider.Shape)
			state.friction, state.restitution = max(collider.Friction, 0), clamp(collider.Restitution, 0, 1)
			state.sensor, state.filter, state.collisionType = collider.Sensor, collider.Filter.normalized(), collider.CollisionType
		}
		runtime.Simulation.colliderState(collider.handle.id).touched = runtime.frame
	}
}

func (runtime *Runtime2D) sweepManaged() {
	for index := range runtime.Simulation.colliders {
		state := &runtime.Simulation.colliders[index]
		if state.alive && state.managed && state.touched != runtime.frame {
			runtime.Simulation.removeColliderIndex(uint32(index))
		}
	}
	for index := range runtime.Simulation.bodies {
		state := &runtime.Simulation.bodies[index]
		if state.alive && state.managed && state.touched != runtime.frame {
			body := Body{world: runtime.Simulation, id: makeBodyID(uint32(index), state.generation)}
			runtime.Simulation.RemoveBody(body)
		}
	}
}

func (runtime *Runtime2D) writeBodies(world *ecs.World) {
	for entity, access := range ecs.Query2[RigidBody2D, scene.Transform](world).All {
		if !scene.IsActive(world, entity) {
			continue
		}
		bodyAccess := access.A
		transformAccess := access.B

		value, pose := bodyAccess.Get(), transformAccess.Get()
		component, transform := &value, &pose
		state := component.body.state()
		if state == nil {
			continue
		}
		component.Velocity, component.AngularVelocity = state.velocity, state.angularVelocity
		transform.Translation[0], transform.Translation[1] = float32(state.position.X), float32(state.position.Y)
		transform.Rotation = mgl32.QuatRotate(float32(state.rotation), mgl32.Vec3{0, 0, 1})
		local, rounded := ecsLocalPhysicsPose(world, entity, *transform)
		*transform = local
		// Compare against the pose actually written to float32 ECS storage so
		// rounding is not interpreted as a teleport on the next fixed step.
		component.lastPosition = V(float64(rounded.Translation[0]), float64(rounded.Translation[1]))
		component.lastRotation = transformRotationZ(rounded)
		if value != bodyAccess.Get() {
			*bodyAccess.Mut() = value
		}
		if pose != transformAccess.Get() {
			*transformAccess.Mut() = pose
		}
	}
}

func (runtime *Runtime2D) writeEvents(world *ecs.World) {
	for _, event := range runtime.Simulation.Events() {
		entityA, entityB := ecs.Entity(event.UserDataA), ecs.Entity(event.UserDataB)
		if entityA != 0 {
			ecs.WriteMessage(world, CollisionMessage2D{
				Phase: event.Phase, Entity: entityA, Other: entityB,
				Collider: event.ColliderA, OtherCollider: event.ColliderB,
				CollisionType: event.CollisionTypeA, OtherType: event.CollisionTypeB,
				Normal: event.Normal, Point: event.Point, Penetration: event.Penetration,
			})
		}
		if entityB != 0 {
			ecs.WriteMessage(world, CollisionMessage2D{
				Phase: event.Phase, Entity: entityB, Other: entityA,
				Collider: event.ColliderB, OtherCollider: event.ColliderA,
				CollisionType: event.CollisionTypeB, OtherType: event.CollisionTypeA,
				Normal: event.Normal.Mul(-1), Point: event.Point, Penetration: event.Penetration,
			})
		}
	}
}

func transformRotationZ(transform scene.Transform) float64 {
	rotation := transform.Rotation
	if rotation.Len() == 0 {
		return 0
	}
	matrix := rotation.Normalize().Mat4()
	return math.Atan2(float64(matrix[1]), float64(matrix[0]))
}
