package project

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	sceneapi "github.com/diiyw/onix/scene"
	engineui "github.com/diiyw/onix/ui"
)

func TestCompileSceneSpawnPlanFlattensHierarchyAndStaticValues(t *testing.T) {
	visible := false
	cameraComponents := Components(SceneComponentCamera)
	cameraComponents.Camera.Fov, cameraComponents.Camera.Near, cameraComponents.Camera.ClearColor = 1, 0.1, "#112233ff"
	textComponents := Components(SceneComponentText)
	textComponents.Text.Value, textComponents.Text.Color, textComponents.Text.OutlineColor = "Child", "#336699", "#ffffff"
	plan, err := compileSceneSpawnPlan([]SceneNodeSpec{
		{
			ID: "camera", Components: cameraComponents, ScaleX: 1, ScaleY: 1, ScaleZ: 1,
		},
		{
			ID: "root", ScaleX: 1, ScaleY: 1, ScaleZ: 1,
			Children: []SceneNodeSpec{{
				ID: "child", Components: textComponents, ScaleX: 1, ScaleY: 1, ScaleZ: 1, Visible: &visible,
				Children: []SceneNodeSpec{{ID: "attached", ScaleX: 1, ScaleY: 1, ScaleZ: 1}},
			}},
		},
		{
			ID: "hud", CoordinateSpace: CoordinateSpaceUI, TargetCamera: "camera",
			ScaleX: 1, ScaleY: 1, ScaleZ: 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Nodes) != 5 || !plan.Has3D || !plan.HasUI {
		t.Fatalf("unexpected plan summary: %+v", plan)
	}
	wantParents := []uint32{0, 0, 2, 3, 0}
	for index, want := range wantParents {
		if got := plan.Nodes[index].Parent; got != want {
			t.Errorf("node %d parent = %d, want %d", index, got, want)
		}
	}
	if plan.Nodes[4].TargetCamera != 1 {
		t.Fatalf("hud target camera = %d, want 1", plan.Nodes[4].TargetCamera)
	}
	child := plan.Nodes[2]
	if child.Visible || !child.TextStyle.HasColor || !child.TextStyle.HasOutlineColor {
		t.Fatalf("text node was not compiled: %+v", child)
	}
	if len(child.Spec.Children) != 0 || child.Spec.Components.Text == nil {
		t.Fatalf("redundant source fields remain in plan node: %+v", child.Spec)
	}
}

func TestInstantiateSceneUsesCompiledParentAndTargetCameraIndices(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "components": {
        "camera": {
          "fov": 1,
          "near": 0.1
        }
      },
      "id": "camera"
    },
    {
      "children": [
        {
          "children": [
            {
              "id": "attached"
            }
          ],
          "id": "child"
        }
      ],
      "id": "root"
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
		t.Fatal(err)
	}
	plan := bundle.scenePlansByID["main"]
	if len(plan.Nodes) != 5 || plan.Nodes[3].Parent != 3 || plan.Nodes[4].TargetCamera != 1 {
		t.Fatalf("unexpected compiled plan: %+v", plan)
	}
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	parent, ok := ecs.Get[sceneapi.Parent](scene.World, scene.MustEntity("attached"))
	if !ok || parent.Entity != scene.MustEntity("child") {
		t.Fatalf("compiled parent was not applied: %+v", parent)
	}
	target, ok := ecs.Get[sceneapi.UITargetCamera](scene.World, scene.MustEntity("hud"))
	if !ok || target.Entity != scene.MustEntity("camera") {
		t.Fatalf("compiled target camera was not applied: %+v", target)
	}
}

func TestInstantiateSceneResolvesWidgetTargetByNodeID(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {"id":"canvas", "coordinateSpace":"ui", "width":100, "height":50, "anchor":[-0.5,0.5]},
    {"id":"panel", "coordinateSpace":"ui", "width":10, "height":10,
     "anchor":[-0.5,0.5], "widget":{"target":"canvas", "alignRight":true, "right":"4px"}}
  ]
}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	panel := scene.MustEntity("panel")
	widget, ok := ecs.Get[engineui.Widget](scene.World, panel)
	if !ok || widget.Target != scene.MustEntity("canvas") || widget.TargetID != "canvas" {
		t.Fatalf("widget target = %+v, want canvas entity and id", widget)
	}
	transform, _ := ecs.Get[sceneapi.UITransform](scene.World, panel)
	if x := transform.Translation.Resolve(sceneapi.UIResolveContext{}).X(); x != 86 {
		t.Fatalf("widget target position x = %v, want 86", x)
	}
}

func TestLoadRejectsUnknownWidgetTarget(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [{"id":"panel", "coordinateSpace":"ui", "widget":{"target":"missing"}}]
}`)
	_, err := loadTestProject(os.DirFS(dir), ".")
	if err == nil || !strings.Contains(err.Error(), `widget target "missing" does not exist`) {
		t.Fatalf("widget target error = %v", err)
	}
}

func TestLoadRejectsNonCameraSpawnPlanTarget(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [
    {
      "id": "notCamera"
    },
    {
      "coordinateSpace": "ui",
      "id": "hud",
      "targetCamera": "notCamera"
    }
  ]
}`)
	_, err := loadTestProject(os.DirFS(dir), ".")
	if err == nil || !strings.Contains(err.Error(), "is not a camera") {
		t.Fatalf("target camera error = %v", err)
	}
}

func TestLoadRejectsRemovedSceneNodeFields(t *testing.T) {
	for _, test := range []struct {
		name  string
		field string
		json  string
	}{
		{name: "kind", field: "kind", json: `{"nodes":[{"id":"legacy","kind":"sprite"}]}`},
		{name: "parent", field: "parent", json: `{"nodes":[{"id":"legacy","parent":"root"}]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			writeTestFile(t, dir, "resources/scenes/main.scene.json", test.json)
			_, err := loadTestProject(os.DirFS(dir), ".")
			if err == nil || !strings.Contains(err.Error(), `unknown field "`+test.field+`"`) {
				t.Fatalf("removed field error = %v", err)
			}
		})
	}
}

func TestCompileSceneSpawnPlanAcceptsComposedComponentSets(t *testing.T) {
	composed := Components(SceneComponentSprite, SceneComponentText)
	if !composed.Has(SceneComponentSprite) || !composed.Has(SceneComponentText) {
		t.Fatalf("variadic Components lost an authored component: %+v", composed)
	}
	tests := []struct {
		name string
		spec SceneNodeSpec
	}{
		{name: "multiple render components", spec: SceneNodeSpec{ID: "multiple", Components: SceneComponents{Sprite: &SpriteComponentSpec{}, Text: &TextComponentSpec{}}}},
		{name: "control with render component", spec: SceneNodeSpec{ID: "mixed", Control: &ControlSpec{}, Components: Components(SceneComponentSprite)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateSceneNodes("main.scene.json", []SceneNodeSpec{test.spec}, map[string]struct{}{}); err != nil {
				t.Fatalf("component set %v was rejected: %v", test.spec, err)
			}
		})
	}
}

func TestSceneNodeComponentsHaveNoSingularRole(t *testing.T) {
	tests := []struct {
		name      string
		spec      SceneNodeSpec
		component SceneComponent
	}{
		{name: "empty", spec: SceneNodeSpec{ID: "empty"}},
		{name: "control", spec: SceneNodeSpec{ID: "control", Control: &ControlSpec{}}},
		{name: "sprite", spec: SceneNodeSpec{ID: "sprite", Components: Components(SceneComponentSprite)}, component: SceneComponentSprite},
		{name: "text", spec: SceneNodeSpec{ID: "text", Components: Components(SceneComponentText)}, component: SceneComponentText},
	}
	for _, test := range tests {
		if test.spec.HasComponent(test.component) != (test.component != "") {
			t.Errorf("%s component presence mismatch", test.name)
		}
		if test.spec.IsEmpty() != (test.component == "" && test.name == "empty") {
			t.Errorf("%s empty state mismatch", test.name)
		}
	}
}

func TestDecodeSceneRejectsUnknownComponentKey(t *testing.T) {
	_, err := decodeJSON[sceneManifest]([]byte(`{"nodes":[{"id":"legacy","components":{"button":{}}}]}`), "main.scene.json")
	if err == nil || !strings.Contains(err.Error(), `unknown field "button"`) {
		t.Fatalf("unknown component error = %v", err)
	}
}

func BenchmarkSceneSpawnPlan(b *testing.B) {
	specs := make([]SceneNodeSpec, 0, 400)
	for index := range 400 {
		spec := SceneNodeSpec{
			ID: fmt.Sprintf("node-%d", index), X: float32(index % 20), Y: float32(index / 20),
			ScaleX: 1, ScaleY: 1, ScaleZ: 1, Rotation: float32(index%7) * 0.01,
			RenderLayers: []uint{0, 2, 2},
		}
		if index%4 == 0 {
			spec.Children = []SceneNodeSpec{{
				ID: fmt.Sprintf("child-%d", index), X: 4, Y: 8,
				ScaleX: 1, ScaleY: 1, ScaleZ: 1,
			}}
		}
		specs = append(specs, spec)
	}
	plan, err := compileSceneSpawnPlan(specs)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportMetric(float64(len(plan.Nodes)), "nodes")
	bundle := &Bundle{}

	b.Run("RecursiveSpecs", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			world := ecs.NewWorld()
			index := make(map[string]ecs.Entity, len(plan.Nodes))
			reverse := make(map[ecs.Entity]string, len(plan.Nodes))
			for _, spec := range specs {
				if _, err := bundle.spawnECSEntity(world, spec, 0, false, 0, index, reverse); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
	b.Run("CompiledPlan", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			world := ecs.NewWorld()
			index := make(map[string]ecs.Entity, len(plan.Nodes))
			reverse := make(map[ecs.Entity]string, len(plan.Nodes))
			entities := make([]ecs.Entity, len(plan.Nodes))
			for nodeIndex, node := range plan.Nodes {
				var parent ecs.Entity
				if node.Parent > 0 {
					parent = entities[node.Parent-1]
				}
				entity, err := bundle.spawnECSPlanNode(world, node, parent, index, reverse)
				if err != nil {
					b.Fatal(err)
				}
				entities[nodeIndex] = entity
			}
		}
	})
}
