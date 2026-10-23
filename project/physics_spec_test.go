//go:build !onix_release

package project

import (
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	enginephysics "github.com/diiyw/onix/physics"
)

func TestPhysicsSceneComponentsMaterializeOnNode(t *testing.T) {
	var spec SceneNodeSpec
	if err := decodeStrictJSONValue([]byte(`{
		"id":"ball","x":12,"y":34,"width":20,"height":20,
		"components":{
			"rigidBody2D":{"type":"dynamic","mass":2},
			"circleCollider2D":{"restitution":0.4}
		}
	}`), &spec); err != nil {
		t.Fatal(err)
	}
	node, err := compileSceneSpawnNode(spec)
	if err != nil {
		t.Fatal(err)
	}
	bundle := &Bundle{components: buildAllSceneComponents}
	components, err := bundle.ecsComponentsForPlanNode(node)
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	entity := world.Spawn(components)
	body, bodyOK := ecs.Get[enginephysics.RigidBody2D](world, entity)
	collider, colliderOK := ecs.Get[enginephysics.CircleCollider2D](world, entity)
	if !bodyOK || !colliderOK {
		t.Fatalf("physics components missing: body=%v collider=%v", bodyOK, colliderOK)
	}
	if body.Mass != 2 || body.GravityScale != 1 || collider.Shape.Radius != 10 || collider.Friction != .5 {
		t.Fatalf("unexpected physics defaults: body=%+v collider=%+v", body, collider)
	}
}

func TestPhysicsColliderRequiresBodyAndWorldCoordinates(t *testing.T) {
	for _, source := range []string{
		`{"id":"orphan","components":{"circleCollider2D":{"radius":2}}}`,
		`{"id":"ui","coordinateSpace":"ui","components":{"rigidBody2D":{}}}`,
	} {
		var spec SceneNodeSpec
		if err := decodeStrictJSONValue([]byte(source), &spec); err != nil {
			t.Fatal(err)
		}
		err := validateSceneNodes("test", []SceneNodeSpec{spec}, map[string]struct{}{})
		if err == nil {
			err = validateGameObjectCoordinateSpaces("test", []SceneNodeSpec{spec}, nil)
		}
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "rigidbody2d") && !strings.Contains(err.Error(), "Physics 2D") {
			t.Fatalf("source %s validation error = %v", source, err)
		}
	}
}

func TestPhysics3DSceneComponentsUseCookedModelCollision(t *testing.T) {
	bodySpec := NewRigidBody3DSpec()
	colliderSpec := NewConvexHullCollider3DSpec()
	spec := SceneNodeSpec{
		ID: "crate",
		Components: SceneComponents{
			Model:       &ResourceComponentSpec{Resource: "crate"},
			RigidBody3D: &bodySpec, ConvexHullCollider3D: &colliderSpec,
		},
	}
	node, err := compileSceneSpawnNode(spec)
	if err != nil {
		t.Fatal(err)
	}
	bundle := &Bundle{
		components: buildAllSceneComponents,
		modelSpecs: map[string]ModelSpec{
			"crate": {ID: "crate", Collision: "singleConvex", Mesh: &MeshSpec{Primitive: "cube", Size: 2}},
		},
		modelResources: newResourceCache[string, modelResource](1),
	}
	components, err := bundle.ecsComponentsForPlanNode(node)
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	entity := world.Spawn(components)
	if _, ok := ecs.Get[enginephysics.RigidBody3D](world, entity); !ok {
		t.Fatal("missing RigidBody3D component")
	}
	collider, ok := ecs.Get[enginephysics.ConvexHullCollider3D](world, entity)
	if !ok || !collider.Shape.Cooked() {
		t.Fatalf("missing cooked convex hull collider: %+v", collider)
	}
}

func TestPhysics3DRejectsDynamicTriangleMesh(t *testing.T) {
	body := NewRigidBody3DSpec()
	mesh := NewTriangleMeshCollider3DSpec()
	if err := validatePhysics3DComponents("test", "terrain", SceneComponents{RigidBody3D: &body, TriangleMeshCollider3D: &mesh}); err == nil || !strings.Contains(err.Error(), "dynamic") {
		t.Fatalf("expected dynamic triangle mesh validation error, got %v", err)
	}
}
