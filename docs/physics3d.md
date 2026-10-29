# 3D physics

`physics` provides fixed-step 2D and 3D rigid-body simulation. The 3D solver uses a
spatial-hash broad phase, filtering, sensors and collision lifecycle messages.
Generic colliders support Sphere, Box3D, Capsule, ConvexHull, Compound and static
TriangleMesh shapes.

## Coordinates and integration

Onix uses right-handed Cartesian coordinates: +X right, +Y up, +Z out of the screen,
with X cross Y = Z. Camera forward is local -Z. Viewport camera rotation changes
projection without changing world axes or scene entities.

Install the system:

```go
if err := app.InstallSystems[onix.Physics3D](); err != nil {
	return err
}
```

Add `physics3d` to `engine.features` in `resources/project.json`, or enable Physics
3D under Project Settings > Runtime Features. Physics 2D and 3D default to disabled
and enter generated installation code only when enabled.

An entity needs Transform, RigidBody3D and a generic collider:

```go
bundle := ecs.NewBundle(3)
bundle.Put(scene.TransformFromXYZ(0, 8, 0))
bundle.Put(physics.NewRigidBody3D(physics.BodyDynamic))
bundle.Put(physics.NewCollider3D(physics.Sphere{Radius: 1}))
entity := app.World.Spawn(bundle)
```

A static floor can use Box3D:

```go
bundle := ecs.NewBundle(3)
bundle.Put(scene.TransformFromXYZ(0, 0, 0))
bundle.Put(physics.NewRigidBody3D(physics.BodyStatic))
bundle.Put(physics.NewCollider3D(physics.NewBox3D(20, 2, 20)))
app.World.Spawn(bundle)
```

Dynamic bodies write position and quaternion orientation to Transform each fixed
step. Static and kinematic bodies read Transform. Editing a dynamic Transform
teleports the body. Internal float64 state survives float32 writeback without
mistaking rounding for a teleport. FixedRotation disables dynamic/kinematic
rotation; static bodies do not push others through leftover velocities.

## Contacts and stability

Boxes clip the actual overlap face and retain up to four support points. Box/hull,
hull/hull and compound/hull pairs use generic separating axes and edge clipping.
Horizontal capsules can retain two endpoints against boxes; spheres use one
contact. Embedded spheres separate toward the nearest exit.

The solver couples normal support constraints, accumulates friction impulses on
two tangent axes and reuses valid cached impulses. It constrains velocity before
integration, then handles new contacts; penetration correction updates position
and orientation. Restitution is suppressed below the greater of one world unit
per second and twice the gravity-induced velocity change per step.

High-friction objects can rest on shallow slopes; insufficient friction permits
sliding, and spheres can roll. Resting bodies remain responsive to forces and
impulses. Collision message positions average support points. Reusing a removed
collider's pool slot produces an End for the old pair and Begin for the new pair.

Fixed contact/linear-solver arrays and reserved caches allow warm box/capsule/hull
regression cases to run without heap allocation. Collision is discrete: fast
objects and thin walls need small time steps. Inertia is a scalar approximation.
Capsule/hull, some compound pairs and non-sphere/triangle-mesh pairs still use
conservative AABB approximations. Compound/hull retains only the deepest child contact.

## Models and collision resources

Models in `resources/resources.json` can reference glTF 2.0, GLB and FBX. Development
imports preserve node transforms, multiple meshes/primitives, positions, normals,
the first UV set and triangle indices; missing normals are generated. FBX applies
file axes and UnitScaleFactor. Skin/morph data uses storage buffers on Vulkan,
Metal and WebGPU, and reusable CPU dynamic vertices on WebGL2. glTF CUBICSPLINE
preserves tangents and uses Hermite interpolation. Non-triangle glTF primitives
and degenerate/nonconvex FBX n-gons are rejected.

```json
{
  "models": [
    {
      "id": "crate",
      "path": "models/crate.glb",
      "collision": "singleConvex",
      "collisionPath": "resources/collisions/crate.collision.json"
    }
  ]
}
```

PreprocessManifests converts model and collision JSON to validated ONIXBIN1.
Release builds load preprocessed binaries without source-format parsers.

## Forces and messages

```go
body := app.World.GetMut[physics.RigidBody3D](entity)
if body != nil {
	body.ApplyForce(physics.V3(0, 20, 0))
	body.ApplyTorque(physics.V3(0, 2, 0))
	body.ApplyImpulse(physics.V3(3, 0, 0))
}
```

Both participants receive CollisionMessage3D with Begin/Stay/End phase, other
entity, collision type, normal, contact point and penetration depth.

## Direct simulation

Without ECS, create a World3D directly. Reserve grows object pools and solver
scratch in advance. Step has no steady-state allocation within reserved capacity.

```go
world := physics.NewWorld3D()
world.Reserve(128, 256)

definition := physics.NewBodyDef3D(physics.BodyDynamic)
definition.Position = physics.V3(0, 8, 0)
body := world.CreateBody(definition)
world.AddCollider(body, physics.NewColliderDef(physics.Sphere{Radius: 1}))

world.Step(1.0 / 60.0)
position := body.Position()
```
