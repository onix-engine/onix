package physics

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

type Runtime3D struct {
	Simulation *World3D
	frame      uint64
}

func NewRuntime3D() *Runtime3D {
	return &Runtime3D{Simulation: NewWorld3D()}
}

func StepSystem3D(name string) ecs.SystemConfig {
	if name == "" {
		name = "step_physics_3d"
	}
	return ecs.SystemConfig{Name: name, System: stepECS3D}
}

func stepECS3D(context *ecs.SystemContext) error {
	if context == nil || context.World == nil {
		return nil
	}
	world := context.World
	runtime, ok := ecs.GetResource[*Runtime3D](world)
	if !ok || runtime == nil {
		runtime = NewRuntime3D()
		ecs.InsertResource(world, runtime)
	}
	if runtime.Simulation == nil {
		runtime.Simulation = NewWorld3D()
	}
	runtime.frame++
	if runtime.frame == 0 {
		runtime.frame = 1
	}
	if err := runtime.syncBodies(world); err != nil {
		return err
	}
	syncECSColliders3D[Sphere](runtime, world)
	syncECSColliders3D[Box3D](runtime, world)
	syncECSColliders3D[Capsule](runtime, world)
	syncECSColliders3D[TriangleMesh](runtime, world)
	syncECSColliders3D[ConvexHull](runtime, world)
	syncECSColliders3D[Compound](runtime, world)
	runtime.sweepManaged()
	delta := 1.0 / 60
	if time, ok := ecs.GetResource[*scene.Time](world); ok && time != nil && time.Delta > 0 {
		delta = time.Delta.Seconds()
	}
	runtime.Simulation.Step(delta)
	runtime.writeBodies(world)
	runtime.writeEvents(world)
	return nil
}

func (runtime *Runtime3D) syncBodies(world *ecs.World) error {
	for entity, access := range ecs.Query2[RigidBody3D, scene.Transform](world).All {
		if !scene.IsActive(world, entity) {
			continue
		}
		bodyAccess := access.A
		transformAccess := access.B

		value := bodyAccess.Get()
		component, transform := &value, transformAccess.Ref()
		component.normalize()
		pose, err := ecsWorldPose(world, entity, *transform, false)
		if err != nil {
			return err
		}
		position := vectorFromMGL(pose.Translation)
		rotation := quaternionFromMGL(pose.Rotation)
		if !component.body.Valid() || component.body.world != runtime.Simulation {
			def := NewBodyDef3D(component.Type)
			def.Position, def.Rotation = position, rotation
			def.Velocity, def.AngularVelocity = component.Velocity, component.AngularVelocity
			def.Mass, def.Inertia, def.GravityScale = component.Mass, component.Inertia, component.GravityScale
			def.LinearDamping, def.AngularDamping = component.LinearDamping, component.AngularDamping
			def.FixedRotation, def.UserData = component.FixedRotation, uint64(entity)
			component.body = runtime.Simulation.CreateBody(def)
			component.body.state().managed = true
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
		state.torque = state.torque.Add(component.Torque)
		if component.Impulse != (Vector3D{}) && state.bodyType == BodyDynamic {
			state.velocity = state.velocity.Add(component.Impulse.Mul(state.inverseMass))
		}
		component.Force, component.Torque, component.Impulse = Vector3D{}, Vector3D{}, Vector3D{}
		component.lastPosition, component.lastRotation = state.position, state.rotation
		if value != bodyAccess.Get() {
			*bodyAccess.Mut() = value
		}
	}
	return nil
}

func syncECSColliders3D[S Shape3D](runtime *Runtime3D, world *ecs.World) {
	for entity, access := range ecs.Query2[RigidBody3D, Collider3D[S]](world).All {
		if !scene.IsActive(world, entity) {
			continue
		}
		bodyAccess := access.A
		colliderAccess := access.B

		body, collider := bodyAccess.Ref(), colliderAccess.Ref()
		if !body.body.Valid() {
			continue
		}
		if !collider.handle.Valid() || collider.handle.world != runtime.Simulation {
			definition := ColliderDef[S]{
				Shape: collider.Shape, Friction: max(collider.Friction, 0),
				Restitution: clamp(collider.Restitution, 0, 1), Sensor: collider.Sensor,
				Filter: collider.Filter.normalized(), CollisionType: collider.CollisionType,
			}
			collider = colliderAccess.Mut()
			collider.handle = AddCollider3D(runtime.Simulation, body.body, definition)
			runtime.Simulation.colliderState3D(collider.handle.id).managed = true
		} else {
			state := runtime.Simulation.colliderState3D(collider.handle.id)
			setColliderShape3D(state, collider.Shape)
			state.friction, state.restitution = max(collider.Friction, 0), clamp(collider.Restitution, 0, 1)
			state.sensor, state.filter, state.collisionType = collider.Sensor, collider.Filter.normalized(), collider.CollisionType
		}
		runtime.Simulation.colliderState3D(collider.handle.id).touched = runtime.frame
	}
}

func (runtime *Runtime3D) sweepManaged() {
	for index := range runtime.Simulation.colliders {
		state := &runtime.Simulation.colliders[index]
		if state.alive && state.managed && state.touched != runtime.frame {
			runtime.Simulation.removeColliderIndex(uint32(index))
		}
	}
	for index := range runtime.Simulation.bodies {
		state := &runtime.Simulation.bodies[index]
		if state.alive && state.managed && state.touched != runtime.frame {
			body := Body3D{world: runtime.Simulation, id: makeBodyID3D(uint32(index), state.generation)}
			runtime.Simulation.RemoveBody(body)
		}
	}
}

func (runtime *Runtime3D) writeBodies(world *ecs.World) {
	for entity, access := range ecs.Query2[RigidBody3D, scene.Transform](world).All {
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
		transform.Translation = vectorToMGL(state.position)
		transform.Rotation = quaternionToMGL(state.rotation)
		local, rounded := ecsLocalPhysicsPose(world, entity, *transform)
		*transform = local
		// Compare the next edit against the float32 pose actually written to ECS.
		component.lastPosition = vectorFromMGL(rounded.Translation)
		component.lastRotation = quaternionFromMGL(rounded.Rotation)
		if value != bodyAccess.Get() {
			*bodyAccess.Mut() = value
		}
		if pose != transformAccess.Get() {
			*transformAccess.Mut() = pose
		}
	}
}

func (runtime *Runtime3D) writeEvents(world *ecs.World) {
	for _, event := range runtime.Simulation.Events() {
		entityA, entityB := ecs.Entity(event.UserDataA), ecs.Entity(event.UserDataB)
		if entityA != 0 {
			ecs.WriteMessage(world, CollisionMessage3D{
				Phase: event.Phase, Entity: entityA, Other: entityB,
				Collider: event.ColliderA, OtherCollider: event.ColliderB,
				CollisionType: event.CollisionTypeA, OtherType: event.CollisionTypeB,
				Normal: event.Normal, Point: event.Point, Penetration: event.Penetration,
			})
		}
		if entityB != 0 {
			ecs.WriteMessage(world, CollisionMessage3D{
				Phase: event.Phase, Entity: entityB, Other: entityA,
				Collider: event.ColliderB, OtherCollider: event.ColliderA,
				CollisionType: event.CollisionTypeB, OtherType: event.CollisionTypeA,
				Normal: event.Normal.Mul(-1), Point: event.Point, Penetration: event.Penetration,
			})
		}
	}
}

func vectorFromMGL(value mgl32.Vec3) Vector3D {
	return V3(float64(value.X()), float64(value.Y()), float64(value.Z()))
}

func vectorToMGL(value Vector3D) mgl32.Vec3 {
	return mgl32.Vec3{float32(value.X), float32(value.Y), float32(value.Z)}
}

func quaternionFromMGL(value mgl32.Quat) Quaternion {
	if value.Len() == 0 {
		return QuaternionIdentity()
	}
	return Q(float64(value.X()), float64(value.Y()), float64(value.Z()), float64(value.W)).Normalize()
}

func quaternionToMGL(value Quaternion) mgl32.Quat {
	value = value.Normalize()
	return mgl32.Quat{W: float32(value.W), V: mgl32.Vec3{float32(value.X), float32(value.Y), float32(value.Z)}}
}
