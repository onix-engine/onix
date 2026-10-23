package project

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/diiyw/onix/internal/typedbin"
	enginephysics "github.com/diiyw/onix/physics"
)

func TestExternalCollisionResourceLoadsSingleConvexIntoModelCache(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{
  "models": [{
    "id": "crate",
    "path": "models/crate.glb",
    "collision": "singleConvex",
    "collisionPath": "resources/collisions/crate.collision.json"
  }]
}`)
	writeTestFile(t, dir, "resources/scenes.json", `{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[]}`)
	writeTestFile(t, dir, "resources/collisions/crate.collision.json", `{
  "mode": "singleConvex",
  "vertices": [[-1,-1,-1],[1,-1,-1],[1,1,-1],[-1,1,-1],[-1,-1,1],[1,-1,1],[1,1,1],[-1,1,1]]
}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatalf("load external collision project: %v", err)
	}
	defer bundle.Dispose()

	object := SceneNodeSpec{ID: "crateNode", Components: SceneComponents{Model: &ResourceComponentSpec{Resource: "crate"}}}
	value, supported, err := bundle.modelResourceForSpec(object)
	if err != nil {
		t.Fatalf("load model collision: %v", err)
	}
	if supported || value.mesh != nil {
		t.Fatal("collision-only path model should not expose a render mesh")
	}
	if !value.hasCollision || value.collision.Mode != enginephysics.CollisionModeSingleConvex || !value.collision.ConvexHull.Cooked() {
		t.Fatalf("unexpected external convex collision: %+v", value.collision)
	}
	second, secondSupported, err := bundle.modelResourceForSpec(object)
	if err != nil || secondSupported || second.mesh != nil || second.collision.ConvexHull.Vertices == nil {
		t.Fatalf("cached collision load = supported=%v value=%+v err=%v", secondSupported, second, err)
	}
	if &second.collision.ConvexHull.Vertices[0] != &value.collision.ConvexHull.Vertices[0] {
		t.Fatal("model cache did not reuse immutable collision slices")
	}
}

func TestExternalCollisionResourceLoadsCompoundPieces(t *testing.T) {
	spec := CollisionResourceSpec{
		Mode: enginephysics.CollisionModeConvexDecomposition,
		Pieces: []CollisionPieceSpec{
			{Vertices: cubeCollisionVertices(-2, 0, 0, 1)},
			{Vertices: cubeCollisionVertices(2, 0, 0, 1), Position: [3]float64{0, 1, 0}},
		},
	}
	cooked, err := spec.cook(enginephysics.CollisionModeConvexDecomposition)
	if err != nil {
		t.Fatalf("cook compound collision: %v", err)
	}
	if cooked.Mode != enginephysics.CollisionModeConvexDecomposition || cooked.Compound.ChildCount() != 2 || !cooked.Compound.Cooked() {
		t.Fatalf("unexpected compound collision: %+v", cooked)
	}
	if cooked.Compound.Children[1].Position.Y != 1 {
		t.Fatalf("piece transform was not retained: %+v", cooked.Compound.Children[1])
	}
}

func TestExternalCollisionResourceRejectsInvalidData(t *testing.T) {
	tests := []struct {
		name     string
		spec     CollisionResourceSpec
		expected enginephysics.CollisionMode
		want     string
	}{
		{
			name: "invalid index",
			spec: CollisionResourceSpec{
				Mode:     enginephysics.CollisionModeTriangleMesh,
				Vertices: [][3]float64{{0, 0, 0}, {1, 0, 0}, {0, 1, 0}},
				Indices:  []uint32{0, 1, 3},
			},
			expected: enginephysics.CollisionModeTriangleMesh,
			want:     "exceeds",
		},
		{
			name:     "empty pieces",
			spec:     CollisionResourceSpec{Mode: enginephysics.CollisionModeConvexDecomposition},
			expected: enginephysics.CollisionModeConvexDecomposition,
			want:     "at least one piece",
		},
		{
			name: "mode mismatch",
			spec: CollisionResourceSpec{
				Mode:     enginephysics.CollisionModeSingleConvex,
				Vertices: cubeCollisionVertices(0, 0, 0, 1),
			},
			expected: enginephysics.CollisionModeTriangleMesh,
			want:     "does not match",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.spec.cook(test.expected)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("cook error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestPreprocessManifestsCompilesCollisionResource(t *testing.T) {
	source := fstest.MapFS{
		"resources/resources.json": {Data: []byte(`{
  "models": [{"id":"crate","path":"models/crate.glb","collision":"singleConvex","collisionPath":"resources/collisions/crate.collision.json"}]
}`)},
		"resources/collisions/crate.collision.json": {Data: []byte(`{
  "mode":"singleConvex",
  "vertices":[[-1,-1,-1],[1,-1,-1],[1,1,-1],[-1,1,-1],[-1,-1,1],[1,-1,1],[1,1,1],[-1,1,1]]
}`)},
	}
	files, err := PreprocessManifests(source, PreprocessOptions{})
	if err != nil {
		t.Fatalf("preprocess collision resource: %v", err)
	}
	for _, file := range files {
		if file.Path != "resources/collisions/crate.collision.json" {
			continue
		}
		if !typedbin.IsEncoded(file.Data) {
			t.Fatal("collision resource was not encoded for release loading")
		}
		decoded, decodeErr := typedbin.Unmarshal[CollisionResourceSpec](file.Data)
		if decodeErr != nil || decoded.Mode != enginephysics.CollisionModeSingleConvex || decoded.Cooked == nil || !decoded.Cooked.ConvexHull.Cooked() {
			t.Fatalf("encoded collision resource = %+v, %v", decoded, decodeErr)
		}
		return
	}
	t.Fatal("preprocessor did not return the referenced collision resource")
}

func cubeCollisionVertices(cx, cy, cz, half float64) [][3]float64 {
	return [][3]float64{
		{cx - half, cy - half, cz - half}, {cx + half, cy - half, cz - half},
		{cx + half, cy + half, cz - half}, {cx - half, cy + half, cz - half},
		{cx - half, cy - half, cz + half}, {cx + half, cy - half, cz + half},
		{cx + half, cy + half, cz + half}, {cx - half, cy + half, cz + half},
	}
}
