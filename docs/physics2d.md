# 2D Physics

The `physics` package provides fixed-step rigid bodies with generic `Circle`,
oriented `Box`, and rounded `Segment` colliders. Positions use world units;
rotations and angular velocities use radians and radians per second.

## Direct Simulation

```go
world := physics.NewWorld()
world.Gravity = physics.V(0, -20)
world.Reserve(128, 256)

floor := world.CreateBody(physics.NewBodyDef(physics.BodyStatic))
world.AddCollider(floor, physics.NewColliderDef(physics.NewBox(20, 2)))

def := physics.NewBodyDef(physics.BodyDynamic)
def.Position = physics.V(0, 8)
body := world.CreateBody(def)
world.AddCollider(body, physics.NewColliderDef(physics.Circle{Radius: 1}))

world.Step(1.0 / 60)
position := body.Position()
```

`BodyDynamic` responds to gravity, forces, impulses, and collisions.
`BodyStatic` stays fixed and contributes no contact velocity, including after
switching from a moving body type. `BodyKinematic` moves at its prescribed
velocity and can push dynamic bodies. `FixedRotation` suppresses angular motion
for both dynamic and kinematic bodies.

Use `NewBodyDef` to obtain the defaults; set `GravityScale = 0` on that definition
to disable gravity. Forces and torques are consumed by the next dynamic step.
`ApplyImpulse` takes a world-space application point and can also change angular
velocity. A positive explicit `Inertia` overrides the initial shape estimate.

## ECS Integration

Install `onix.Physics2D`, or enable `"physics2d"` in
`resources/project.json` under `engine.features`. Entities need
`scene.Transform`, `physics.RigidBody2D`, and a typed collider:

```go
bundle := ecs.NewBundle(3)
bundle.Put(scene.TransformFromXYZ(0, 8, 0))
bundle.Put(physics.NewRigidBody2D(physics.BodyDynamic))
bundle.Put(physics.NewCollider2D(physics.Circle{Radius: 1}))
entity := app.World.Spawn(bundle)
```

The fixed update system synchronizes components and writes the resulting pose
back to `scene.Transform`. Dynamic simulation retains float64 precision between
float32 transform writes. Editing the transform explicitly teleports the body.
Removing the rigid body or its transform releases the managed physics objects.
Submit forces, torques, and center impulses through a mutable `RigidBody2D`.

## Contacts and Events

Collider filters require both category/mask intersections to be nonzero.
Matching nonzero groups reject each other. A zero `Filter{}` uses `FilterAll`;
use `NewFilter(0, category, 0)` to explicitly reject all collisions.

Sensors produce events without impulses or position correction, including
between static or kinematic bodies. `World.Events()` exposes a borrowed slice
valid until the next step; copy events when retaining them beyond that point.
ECS emits a `CollisionMessage2D` for each side with its normal directed toward
the other collider. Phases are `CollisionBegin`, `CollisionStay`, and
`CollisionEnd`. Removing and replacing a collider emits `End` for the old
handle and `Begin` for the replacement, even when its pool slot is reused.

Box contacts retain up to two points from the clipped incident edge. The solver
resolves their support impulses together, accumulates friction impulses, and
reuses matching contact impulses across steps. Position correction includes
angular response. Collision events report the midpoint of the contact region.
Rounded segments include corner separation axes. Embedded circles are corrected
toward the nearest box face.

Contact velocities are constrained before advancing positions, preventing
static friction from leaving a small downhill displacement each tick. Newly
formed impacts are resolved after advancing positions and before penetration
correction. Restitution targets are fixed during each velocity solve; impacts
below the greater of 1 world unit/second or twice the gravity velocity added by
one tick are treated as inelastic so resting contacts do not continually bounce.

## Capacity and Performance

`Reserve` can be called on a live world without invalidating handles or losing
state. It reserves body/collider pools, free lists, and estimated scratch space.
Scratch requirements depend on contact density and how many grid cells each
collider occupies, so reservation is not an absolute bound on first-step
allocations. After warming up a representative workload, steps reuse storage;
creation and removal also reuse reserved pool capacity.

Set `CellSize` near the typical collider size in world units. Very large cells
increase pair comparisons; very small cells increase grid entries. Colliders
spanning more than 64 cells use a separate large-shape path. Each pair is emitted
only in its first shared cell; filters run before pair storage and sorting.
Position iterations stop once no correction is needed, while corrections that
move bodies into new contacts still trigger another broadphase pass.

Run the allocation checks and benchmarks without launching the editor:

```sh
go test ./physics
go test -race ./physics
go test ./physics -run '^$' -bench BenchmarkWorld2D -benchmem -count=3
```

## Current Limits

The solver uses discrete collision detection, up to two contact points for
box pairs, and one point for other shape pairs. Use a sufficiently small fixed
timestep for fast objects and thin walls;
there is no continuous collision detection. Sleeping, joints, and automatic
compound-body mass recomputation are not implemented. `IsSleeping` returns
false. Set explicit inertia when changing collider geometry or using compound
bodies that need accurate angular response.
