//go:build !onix_release

package project

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	sceneapi "github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/scene/render3d"
)

func TestLoadModelResourcesAndInstantiate3DGameObjects(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "shaders/effect.metal", `metal source`)
	writeTestFile(t, dir, "models/ship.glb", string(testTriangleGLB()))
	writeTestFile(t, dir, "resources/resources.json", `{
	"shaders": [{"id":"effect","metal":"shaders/effect.metal"}],
  "models": [
	{"id":"cube","mesh":{"primitive":"cube","size":2},"material":{"shader":"effect","baseColor":"#ff0000","metallic":0.4,"roughness":0.2}},
    {"id":"ship","path":"models/ship.glb"},
    {"id":"character","path":"models/character.fbx"}
  ]
}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "components": {
        "model": {
          "resource": "cube"
        }
      },
      "id": "visible",
      "rotationY": 0.5,
      "scaleX": 2,
      "scaleY": 3,
      "scaleZ": 4,
      "x": 1.5,
      "y": 2,
      "z": -3
    },
    {
      "components": {
        "model": {
          "resource": "ship"
        }
      },
      "id": "configured",
      "scaleX": 1,
      "scaleY": 1,
      "scaleZ": 1
    }
  ]
}`)

	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatalf("load model project: %v", err)
	}
	if !bundle.Has3D() {
		t.Fatal("expected bundle to report 3D content")
	}
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatalf("instantiate model project: %v", err)
	}
	visible := scene.MustEntity("visible")
	mesh, ok := ecs.Get[render3d.Mesh](scene.World, visible)
	if !ok || mesh.Mesh == nil || len(mesh.Mesh.Indices) != 36 {
		t.Fatalf("expected cube mesh component, got %+v", mesh)
	}
	transform, ok := ecs.Get[sceneapi.Transform](scene.World, visible)
	if !ok || transform.Translation != [3]float32{1.5, 2, -3} || transform.Scale != [3]float32{2, 3, 4} {
		t.Fatalf("unexpected 3D transform: %+v", transform)
	}
	material, ok := ecs.Get[render3d.Material[graphics.StandardMaterial]](scene.World, visible)
	if !ok || material.Material == nil || material.Material.Shader == nil || material.Material.BaseColor.R != 1 || material.Material.Metallic != 0.4 {
		t.Fatalf("unexpected model material: %+v", material)
	}
	configured := scene.MustEntity("configured")
	imported, ok := ecs.Get[render3d.Mesh](scene.World, configured)
	if !ecs.Has[sceneapi.Transform](scene.World, configured) || !ok || imported.Mesh == nil || len(imported.Mesh.Indices) != 3 {
		t.Fatal("expected GLB model to produce a display mesh")
	}
}

func TestCameraAndDirectionalLightAre3DGameObjects(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "components": {
        "camera": {
          "clearColor": "#123456ff",
          "fov": 1.1,
          "near": 0.25,
          "order": 3
        }
      },
      "id": "camera",
      "rotationX": -0.4,
      "rotationY": 0.6,
      "scaleX": 1,
      "scaleY": 1,
      "scaleZ": 1,
      "x": 6,
      "y": 4.5,
      "z": 9
    },
    {
      "components": {
        "directionallight": {
          "color": "#ffeeddff",
          "illuminance": 24000
        }
      },
      "id": "sun",
      "rotationX": -1,
      "rotationY": 0.8,
      "scaleX": 1,
      "scaleY": 1,
      "scaleZ": 1
    },
    {
      "coordinateSpace": "ui",
      "id": "hud",
      "targetCamera": "camera"
    }
  ]
}`)

	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatalf("load 3D game objects: %v", err)
	}
	if !bundle.Has3D() {
		t.Fatal("camera and light must mark the project as 3D")
	}
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatalf("instantiate 3D game objects: %v", err)
	}
	cameraEntity := scene.MustEntity("camera")
	camera, cameraOK := ecs.Get[sceneapi.Camera](scene.World, cameraEntity)
	projection, projectionOK := ecs.Get[sceneapi.Projection](scene.World, cameraEntity)
	_, markerOK := ecs.Get[render3d.Camera](scene.World, cameraEntity)
	if !cameraOK || !projectionOK || !markerOK || projection.Perspective.Fov != 1.1 || projection.Perspective.Near != 0.25 || camera.Order != 3 || camera.ClearColor.R == 0 {
		t.Fatalf("unexpected camera components: camera=%+v projection=%+v", camera, projection)
	}
	if transform, ok := ecs.Get[sceneapi.Transform](scene.World, cameraEntity); !ok || transform.Translation != [3]float32{6, 4.5, 9} {
		t.Fatalf("unexpected camera transform: %+v", transform)
	}
	light, ok := ecs.Get[render3d.DirectionalLight](scene.World, scene.MustEntity("sun"))
	if !ok || light.Illuminance != 24000 || light.Color.R <= light.Color.B {
		t.Fatalf("unexpected directional light: %+v", light)
	}
	target, ok := ecs.Get[sceneapi.UITargetCamera](scene.World, scene.MustEntity("hud"))
	if !ok || target.Entity != cameraEntity {
		t.Fatalf("unexpected UI target camera: %+v ok=%v", target, ok)
	}

	updatedSpec := SceneNodeSpec{ID: "camera", Components: Components(SceneComponentCamera), X: 2, Y: 3, Z: 4, ScaleX: 1, ScaleY: 1, ScaleZ: 1}
	updatedSpec.Components.Camera.Fov, updatedSpec.Components.Camera.Near, updatedSpec.Components.Camera.Order = 0.9, 0.5, 7
	updatedSpec.Components.Camera.ClearColor = "#000000ff"
	if err := scene.ApplyNode(updatedSpec); err != nil {
		t.Fatalf("update camera game object: %v", err)
	}
	camera, _ = ecs.Get[sceneapi.Camera](scene.World, cameraEntity)
	projection, _ = ecs.Get[sceneapi.Projection](scene.World, cameraEntity)
	transform, _ := ecs.Get[sceneapi.Transform](scene.World, cameraEntity)
	if math.Abs(float64(projection.Perspective.Fov-0.9)) > 1e-6 || camera.Order != 7 || transform.Translation != [3]float32{2, 3, 4} {
		t.Fatalf("camera hot update was not applied: camera=%+v projection=%+v transform=%+v", camera, projection, transform)
	}
}

func TestLoadRejectsLegacyFlatCameraField(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
	  "camera": {},
      "id": "camera"
    }
  ]
}`)
	_, err := loadTestProject(os.DirFS(dir), ".")
	if err == nil || !strings.Contains(err.Error(), `unknown field "camera"`) {
		t.Fatalf("expected legacy flat camera error, got %v", err)
	}
}

func TestRawMeshSpecBuildsIndexedGeometry(t *testing.T) {
	spec := MeshSpec{
		Vertices: []MeshVertexSpec{
			{Position: [3]float32{0, 0, 0}, Normal: [3]float32{0, 0, 1}},
			{Position: [3]float32{1, 0, 0}, Normal: [3]float32{0, 0, 1}},
			{Position: [3]float32{0, 1, 0}, Normal: [3]float32{0, 0, 1}},
		},
		Indices: []uint32{0, 1, 2},
	}
	mesh, err := spec.build()
	if err != nil {
		t.Fatalf("build raw mesh: %v", err)
	}
	if len(mesh.Vertices) != 3 || len(mesh.Indices) != 3 {
		t.Fatalf("unexpected raw mesh: %+v", mesh)
	}
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal raw mesh: %v", err)
	}
	if !strings.Contains(string(data), `"position"`) || strings.Contains(string(data), `"Position"`) {
		t.Fatalf("unexpected vertex JSON fields: %s", data)
	}
}

func TestModelCollisionModeCooksSelectedShape(t *testing.T) {
	spec := ModelSpec{ID: "convex", Mesh: &MeshSpec{Primitive: "cube", Size: 2}, Collision: "singleConvex"}
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	mesh, err := spec.Mesh.build()
	if err != nil {
		t.Fatal(err)
	}
	cooked, err := spec.CookCollision(mesh)
	if err != nil {
		t.Fatal(err)
	}
	if cooked.Mode.String() != "singleConvex" || cooked.ConvexHull.FaceCount() == 0 {
		t.Fatalf("unexpected collision resource: %+v", cooked)
	}
}

func TestModelCollisionPathAllowsModuleLayout(t *testing.T) {
	base := ModelSpec{ID: "crate", Path: "models/crate.glb", CollisionPath: "resources/collisions/crate.collision.json"}
	if err := base.Validate(); err != nil {
		t.Fatalf("collision resource should validate: %v", err)
	}
	base.CollisionPath = "collisions/crate.collision.json"
	if err := base.Validate(); err != nil {
		t.Fatalf("private module layout should validate: %v", err)
	}
	base.CollisionPath = "resources/collisions/crate.json"
	if err := base.Validate(); err == nil || !strings.Contains(err.Error(), ".collision.json") {
		t.Fatalf("expected collision JSON suffix error, got %v", err)
	}
}

func TestLoadRejectsUnsupportedModelFile(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{"models":[{"id":"bad","path":"model.obj"}]}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": []
}`)
	_, err := loadTestProject(os.DirFS(dir), ".")
	if err == nil || !strings.Contains(err.Error(), ".fbx, .gltf, or .glb") {
		t.Fatalf("expected unsupported model extension error, got %v", err)
	}
}
