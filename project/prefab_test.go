package project

import (
	"os"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	sceneapi "github.com/diiyw/onix/scene"
)

func TestPrefabRegisteredByResourcesMountsUnderSceneNode(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{
  "prefabs": [{"id":"effect","path":"prefabs/effect.prefab.json"}]
}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [{"id":"host","coordinateSpace":"ui"}]
}`)
	writeTestFile(t, dir, "prefabs/effect.prefab.json", `{
  "root": {
    "id":"root",
    "coordinateSpace":"ui",
    "x":8,
    "y":12,
    "width":80,
    "height":64,
    "visible":false,
    "children":[{"id":"child","coordinateSpace":"ui","x":90,"width":20,"height":20,"visible":false}]
  }
}`)

	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	if ids := bundle.PrefabIDs(); len(ids) != 1 || ids[0] != "effect" {
		t.Fatalf("prefab ids = %v", ids)
	}

	world := ecs.NewWorld()
	scene, err := bundle.InstantiateEntryScene(world)
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()
	host := scene.MustEntity("host")
	instance, err := scene.MountPrefab(host, "effect", "mounted-effect")
	if err != nil {
		t.Fatal(err)
	}
	root := instance.Root()
	if root == 0 || scene.MustEntity("mounted-effect.root") != root {
		t.Fatalf("prefab root = %d", root)
	}
	child, ok := instance.Entity("child")
	if !ok {
		t.Fatal("prefab child was not indexed by local id")
	}
	if parent, ok := ecs.Get[sceneapi.Parent](world, root); !ok || parent.Entity != host {
		t.Fatalf("prefab root parent = %#v, want %d", parent, host)
	}
	if parent, ok := ecs.Get[sceneapi.Parent](world, child); !ok || parent.Entity != root {
		t.Fatalf("prefab child parent = %#v, want %d", parent, root)
	}

	transform := ecs.GetMut[sceneapi.UITransform](world, root)
	ok = transform != nil
	if !ok || transform == nil {
		t.Fatal("prefab root UI transform is missing")
	}
	transform.Translation = sceneapi.Px2(100, 200)
	instance.SetVisible(true)
	if !instance.Reset() {
		t.Fatal("reset prefab instance")
	}
	if got := transform.Translation; got != sceneapi.Px2(8, 12) {
		t.Fatalf("reset translation = %#v, want %#v", got, sceneapi.Px2(8, 12))
	}
	if size, ok := ecs.Get[sceneapi.Size2D](world, root); !ok || size.Width != 80 || size.Height != 64 {
		t.Fatalf("preserved prefab bounds = %#v, want 80x64", size)
	}
	for _, entity := range []ecs.Entity{root, child} {
		visibility, ok := ecs.Get[sceneapi.Visibility](world, entity)
		if !ok || visibility.Visible {
			t.Fatalf("reset visibility for %d = %#v, want hidden", entity, visibility)
		}
	}
}

func TestPrefabPoolSteadyStateDoesNotAllocate(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{
  "prefabs": [{"id":"effect","path":"prefabs/effect.prefab.json"}]
}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [{"id":"host","coordinateSpace":"ui"}]
}`)
	writeTestFile(t, dir, "prefabs/effect.prefab.json", `{
  "root": {"id":"root","coordinateSpace":"ui","visible":false}
}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()

	world := ecs.NewWorld()
	scene, err := bundle.InstantiateEntryScene(world)
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()
	pool, err := scene.NewPrefabPool(scene.MustEntity("host"), "effect", "effect-slot", 4)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Dispose()
	if pool.Len() != 4 {
		t.Fatalf("pool length = %d", pool.Len())
	}

	instance := pool.Acquire()
	if instance == nil {
		t.Fatal("acquire prefab pool instance")
	}
	instance.SetVisible(true)
	if !pool.Release(instance) {
		t.Fatal("release prefab pool instance")
	}
	if allocations := testing.AllocsPerRun(1_000, func() {
		instance := pool.Acquire()
		instance.SetVisible(true)
		if !pool.Release(instance) {
			t.Fatal("release prefab pool instance")
		}
	}); allocations != 0 {
		t.Fatalf("prefab pool steady-state allocations = %f, want 0", allocations)
	}
}

func TestLoadRejectsInvalidPrefabRegistration(t *testing.T) {
	tests := []struct {
		name      string
		resources string
		want      string
	}{
		{
			name: "duplicate id",
			resources: `{"prefabs":[
  {"id":"effect","path":"prefabs/first.prefab.json"},
  {"id":"effect","path":"prefabs/second.prefab.json"}
]}`,
			want: "duplicate prefab id",
		},
		{
			name:      "invalid path",
			resources: `{"prefabs":[{"id":"effect","path":"../effect.prefab.json"}]}`,
			want:      "relative *.prefab.json",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			writeTestFile(t, dir, "resources/project.json", `{}`)
			writeTestFile(t, dir, "resources/resources.json", test.resources)
			writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[]}`)
			if _, err := loadTestProject(os.DirFS(dir), "."); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("load error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestNormalizePrefabPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: " resources/temp/../prefabs/effect.prefab.json ", want: "resources/prefabs/effect.prefab.json"},
		{path: "../effect.prefab.json"},
		{path: `resources\\prefabs\\effect.prefab.json`},
		{path: "resources/prefabs/effect.json"},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			got, err := NormalizePrefabPath(test.path)
			if test.want == "" {
				if err == nil {
					t.Fatalf("NormalizePrefabPath(%q) succeeded", test.path)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("NormalizePrefabPath(%q) = %q, %v; want %q, nil", test.path, got, err, test.want)
			}
		})
	}
}
