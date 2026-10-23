//go:build !onix_release

package project

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/internal/typedbin"
	sceneapi "github.com/diiyw/onix/scene"
)

func TestPreprocessManifestsCompilesReferencedJSONAtlasOnly(t *testing.T) {
	source := fstest.MapFS{
		"resources/scenes.json":            {Data: []byte(`{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)},
		"resources/scenes/main.scene.json": {Data: []byte(`{"nodes":[{"id":"hero","components":{"sprite":{"texture":{"atlas":"resources/sheet.json","frame":"idle"}}}}]}`)},
		"resources/sheet.json": {Data: []byte(`{
			"frames":{"idle":{"frame":{"x":0,"y":0,"w":8,"h":8}}},
			"meta":{"image":"sheet.png","size":{"w":16,"h":16}}
		}`)},
		"resources/business.json": {Data: []byte(`{"frames":{},"meta":{"kind":"business"}}`)},
	}
	files, err := PreprocessManifests(source, PreprocessOptions{Root: "."})
	if err != nil {
		t.Fatal(err)
	}
	compiled := make(map[string]PreprocessedFile, len(files))
	for _, file := range files {
		compiled[file.Path] = file
	}
	if !graphics.IsPreprocessedAtlas(compiled["resources/sheet.json"].Data) {
		t.Fatal("referenced JSON atlas was not compiled")
	}
	if _, ok := compiled["resources/business.json"]; ok {
		t.Fatal("unreferenced business JSON was treated as an atlas")
	}
}

func TestPreprocessManifestsCompilesRegisteredPrefab(t *testing.T) {
	source := fstest.MapFS{
		"resources/resources.json": {Data: []byte(`{
  "prefabs":[{"id":"effect","path":"resources/prefabs/effect.prefab.json"}]
}`)},
		"resources/scenes.json":            {Data: []byte(`{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)},
		"resources/scenes/main.scene.json": {Data: []byte(`{"nodes":[{"id":"host","coordinateSpace":"ui"}]}`)},
		"resources/prefabs/effect.prefab.json": {Data: []byte(`{
  "root":{"id":"root","coordinateSpace":"ui","visible":false}
}`)},
	}
	files, err := PreprocessManifests(source, PreprocessOptions{Root: "."})
	if err != nil {
		t.Fatal(err)
	}
	var prefab prefabManifest
	found := false
	for _, file := range files {
		if file.Path != "resources/prefabs/effect.prefab.json" {
			continue
		}
		found = true
		if !typedbin.IsEncoded(file.Data) {
			t.Fatal("prefab manifest was not compiled")
		}
		if decoded, err := typedbin.Unmarshal[prefabManifest](file.Data); err != nil {
			t.Fatal(err)
		} else {
			prefab = decoded
		}
	}
	if !found {
		t.Fatal("preprocessor did not return the registered prefab")
	}
	if !prefab.Prepared || len(prefab.SpawnPlan.Nodes) != 1 || prefab.Root.ID != "" {
		t.Fatalf("compiled prefab = %#v", prefab)
	}
	for _, file := range files {
		if file.Remove {
			delete(source, file.Path)
			continue
		}
		source[file.Path] = &fstest.MapFile{Data: file.Data}
	}
	bundle, err := loadTestProject(source, ".")
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
	if _, err := scene.MountPrefab(scene.MustEntity("host"), "effect", "compiled-effect"); err != nil {
		t.Fatal(err)
	}
}

func TestPreprocessManifestsPrunesShaderTargets(t *testing.T) {
	source := fstest.MapFS{
		"resources/resources.json": {Data: []byte(`{"shaders":[{"id":"effect","vulkanFragment":"shaders/effect.spv","metal":"shaders/effect.metal","webGL2Fragment":"shaders/effect.glsl"}]}`)},
		"shaders/effect.spv":       {Data: []byte{0, 0, 0, 0}},
		"shaders/effect.metal":     {Data: []byte("metal")},
		"shaders/effect.glsl":      {Data: []byte("fragment")},
	}
	files, err := PreprocessManifests(source, PreprocessOptions{Root: ".", ShaderTarget: ShaderTargetWebGL2})
	if err != nil {
		t.Fatal(err)
	}
	var manifest resourceManifest
	removed := make(map[string]bool)
	for _, file := range files {
		if file.Remove {
			removed[file.Path] = true
			continue
		}
		if file.Path == "resources/resources.json" {
			manifest, err = typedbin.Unmarshal[resourceManifest](file.Data)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(manifest.Shaders) != 1 || manifest.Shaders[0].WebGL2Fragment != "shaders/effect.glsl" {
		t.Fatalf("compiled shader manifest = %+v", manifest.Shaders)
	}
	if manifest.Shaders[0].VulkanFragment != "" || manifest.Shaders[0].Metal != "" {
		t.Fatalf("non-WebGL shader sources were retained: %+v", manifest.Shaders[0])
	}
	for _, name := range []string{"shaders/effect.spv", "shaders/effect.metal"} {
		if !removed[name] {
			t.Errorf("shader source %s was not removed", name)
		}
	}
	if removed["shaders/effect.glsl"] {
		t.Fatal("selected WebGL shader source was removed")
	}
}

func TestPreprocessManifestsRejectsShaderWithoutTargetSource(t *testing.T) {
	source := fstest.MapFS{
		"resources/resources.json": {Data: []byte(`{"shaders":[{"id":"effect","metal":"shaders/effect.metal"}]}`)},
		"shaders/effect.metal":     {Data: []byte("metal")},
	}
	if _, err := PreprocessManifests(source, PreprocessOptions{Root: ".", ShaderTarget: ShaderTargetWebGL2}); err == nil || !strings.Contains(err.Error(), "no WebGL2 source") {
		t.Fatalf("missing target shader error = %v", err)
	}
}

func TestPreprocessManifestsCompilesSpineForAtlasFreeLoading(t *testing.T) {
	var texture bytes.Buffer
	if err := png.Encode(&texture, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	source := fstest.MapFS{
		"resources/project.json":           {Data: []byte(`{}`)},
		"resources/scenes.json":            {Data: []byte(`{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)},
		"resources/scenes/main.scene.json": {Data: []byte(`{"nodes":[]}`)},
		"resources/resources.json": {Data: []byte(`{"spines":[{
			"id":"hero",
			"skeleton":"resources/hero.json",
			"atlas":"resources/hero.atlas",
			"texture":"resources/hero.png"
		}]}`)},
		"resources/hero.json": {Data: []byte(`{
			"skeleton":{"width":1,"height":1},
			"bones":[{"name":"root"}],
			"slots":[{"name":"slot","bone":"root","attachment":"region"}],
			"skins":{"default":{"slot":{"region":{"width":1,"height":1}}}}
		}`)},
		"resources/hero.atlas": {Data: []byte("hero.png\nsize: 1,1\nformat: RGBA8888\nfilter: MipMapLinearLinear,Linear\nrepeat: none\nregion\nrotate: false\nxy: 0,0\nsize: 1,1\norig: 1,1\noffset: 0,0\nindex: -1\n")},
		"resources/hero.png":   {Data: texture.Bytes()},
	}
	files, err := PreprocessManifests(source, PreprocessOptions{Root: "."})
	if err != nil {
		t.Fatal(err)
	}
	spineCompiled := false
	for _, file := range files {
		if file.Remove {
			delete(source, file.Path)
		} else {
			source[file.Path] = &fstest.MapFile{Data: file.Data}
		}
		if file.Path == "resources/hero.json" {
			spineCompiled = true
			if !typedbin.IsEncoded(file.Data) {
				t.Fatal("Spine skeleton was not compiled")
			}
		}
	}
	if !spineCompiled {
		t.Fatal("preprocessor did not return the Spine skeleton")
	}
	if _, ok := source["resources/hero.atlas"]; ok {
		t.Fatal("preprocessor did not remove the compiled Spine atlas")
	}
	bundle, err := loadTestProject(source, ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	resource, err := bundle.Resources().Spine("hero")
	if err != nil {
		t.Fatal(err)
	}
	if resource.Width != 1 || resource.Height != 1 {
		t.Fatalf("Spine dimensions = %fx%f, want 1x1", resource.Width, resource.Height)
	}
	if !resource.Texture.Mipmapped() {
		t.Fatal("compiled Spine mipmap setting was not restored")
	}
}

func TestPreprocessManifestsRejectsConflictingSpineAtlases(t *testing.T) {
	skeleton := []byte(`{"bones":[{"name":"root"}],"skins":{}}`)
	atlas := []byte("page.png\nregion\nsize: 1,1\n")
	source := fstest.MapFS{
		"resources/resources.json": {Data: []byte(`{"spines":[
			{"id":"first","skeleton":"hero.json","atlas":"first.atlas","texture":"hero.png"},
			{"id":"second","skeleton":"hero.json","atlas":"second.atlas","texture":"hero.png"}
		]}`)},
		"hero.json":   {Data: skeleton},
		"first.atlas": {Data: atlas},
	}
	if _, err := PreprocessManifests(source, PreprocessOptions{Root: "."}); err == nil || !strings.Contains(err.Error(), "conflicting atlases") {
		t.Fatalf("conflicting Spine atlas error = %v", err)
	}
}

func TestPreprocessManifestsLoadsEquivalentBundle(t *testing.T) {
	source := fstest.MapFS{
		"resources/project.json":           {Data: []byte(`{"window":{"title":"Compiled","width":640,"height":360}}`)},
		"resources/resources.json":         {Data: []byte(`{"tilemaps":[{"id":"map","width":2,"height":1,"tiles":[1]}]}`)},
		"resources/scenes.json":            {Data: []byte(`{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"},{"id":"menu","path":"scenes/menu.scene.json"}]}`)},
		"resources/scenes/main.scene.json": {Data: []byte(`{"nodes":[{"id":"root","coordinateSpace":"ui","children":[{"id":"label","components":{"text":{"value":"Onix"}},"coordinateSpace":"ui"}]}]}`)},
		"resources/scenes/menu.scene.json": {Data: []byte(`{"nodes":[{"id":"menu","control":{"text":"Play"},"coordinateSpace":"ui","anchor":[-0.5,0.5],"children":[{"id":"menu-body","components":{"sprite":{"texture":{"color":"#2878ffff"}}},"coordinateSpace":"ui","width":120,"height":40}]}]}`)},
	}
	want, err := loadTestProject(source, ".")
	if err != nil {
		t.Fatal(err)
	}
	defer want.Dispose()

	compiled, err := PreprocessManifests(source, PreprocessOptions{Root: "."})
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled) != 5 {
		t.Fatalf("compiled file count = %d, want 5", len(compiled))
	}
	for _, file := range compiled {
		if file.Remove {
			delete(source, file.Path)
			continue
		}
		if !typedbin.IsEncoded(file.Data) {
			t.Errorf("%s was not encoded", file.Path)
		}
		original, err := fs.ReadFile(source, file.Path)
		if err != nil {
			t.Fatal(err)
		}
		if file.SourceSize != len(original) {
			t.Errorf("%s source size = %d, want %d", file.Path, file.SourceSize, len(original))
		}
		if strings.HasSuffix(file.Path, ".scene.json") {
			manifest, err := typedbin.Unmarshal[sceneManifest](file.Data)
			if err != nil {
				t.Fatal(err)
			}
			if !manifest.Prepared {
				t.Errorf("%s was not marked prepared", file.Path)
			}
			if len(manifest.Nodes) != 0 || len(manifest.SpawnPlan.Nodes) == 0 {
				t.Errorf("%s did not replace source objects with a spawn plan", file.Path)
			}
		}
		source[file.Path] = &fstest.MapFile{Data: file.Data}
	}

	got, err := loadTestProject(source, ".")
	if err != nil {
		t.Fatal(err)
	}
	defer got.Dispose()
	if !reflect.DeepEqual(got.appConfig, want.appConfig) {
		t.Fatalf("app config = %#v, want %#v", got.appConfig, want.appConfig)
	}
	if !reflect.DeepEqual(got.sceneIDs, want.sceneIDs) {
		t.Fatalf("scenes = %v, want %v", got.sceneIDs, want.sceneIDs)
	}
	if !reflect.DeepEqual(got.scenePlansByID, want.scenePlansByID) {
		t.Fatalf("game object spawn plans differ after preprocessing")
	}
	if got.scenePlansByID["main"].Nodes[1].UITransform.Scale.X() != 1 {
		t.Fatal("custom JSON defaults were not compiled")
	}
	if got.scenePlansByID["menu"].Nodes[0].Spec.Anchor != sceneapi.AnchorTopLeft {
		t.Fatal("explicit UI anchor was not compiled")
	}
	menuBody := got.scenePlansByID["menu"].Nodes[1]
	if menuBody.Parent != 1 {
		t.Fatalf("UI entity hierarchy was not preserved in the preprocessed spawn plan: %#v", menuBody)
	}
	if got.tilemapSpecs["map"].Tiles[1] != 0 {
		t.Fatal("tilemap normalization was not compiled")
	}
}

func TestPreprocessManifestsIsDeterministic(t *testing.T) {
	source := fstest.MapFS{
		"resources/scenes.json":            {Data: []byte(`{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)},
		"resources/scenes/main.scene.json": {Data: []byte(`{"nodes":[{"id":"b"},{"id":"a"}]}`)},
	}
	first, err := PreprocessManifests(source, PreprocessOptions{Root: "."})
	if err != nil {
		t.Fatal(err)
	}
	second, err := PreprocessManifests(source, PreprocessOptions{Root: "."})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("equivalent preprocessing runs produced different data")
	}
}

func TestPreprocessManifestsRejectsInvalidJSON(t *testing.T) {
	source := fstest.MapFS{"resources/project.json": {Data: []byte(`{"unknown":true}`)}}
	if _, err := PreprocessManifests(source, PreprocessOptions{Root: "."}); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("invalid manifest error = %v", err)
	}
}

func TestPreprocessManifestsDoesNotUseRootManifests(t *testing.T) {
	source := fstest.MapFS{"project.json": {Data: []byte(`{"unknown":true}`)}}
	files, err := PreprocessManifests(source, PreprocessOptions{Root: "."})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("root manifest produced files: %v", files)
	}
}

func TestLoadRejectsCorruptPreprocessedManifest(t *testing.T) {
	source := fstest.MapFS{
		"resources/scenes.json":            {Data: []byte(`{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)},
		"resources/scenes/main.scene.json": {Data: []byte(`{"nodes":[]}`)},
	}
	files, err := PreprocessManifests(source, PreprocessOptions{Root: "."})
	if err != nil {
		t.Fatal(err)
	}
	compiled := slices.IndexFunc(files, func(file PreprocessedFile) bool { return file.Path == "resources/scenes/main.scene.json" })
	if compiled < 0 {
		t.Fatal("preprocessed scene is missing")
	}
	source["resources/scenes/main.scene.json"] = &fstest.MapFile{Data: files[compiled].Data[:len(files[compiled].Data)-1]}
	if _, err := loadTestProject(source, "."); err == nil || !strings.Contains(err.Error(), "preprocessed main.scene.json") {
		t.Fatalf("corrupt manifest error = %v", err)
	}
}

func BenchmarkDecodeSceneManifest(b *testing.B) {
	var source strings.Builder
	source.WriteString(`{"nodes":[`)
	for index := range 500 {
		if index > 0 {
			source.WriteByte(',')
		}
		fmt.Fprintf(&source, `{"id":"item-%d","components":{"text":{"value":"benchmark item %d","fontSize":16,"color":"#ffffffff"}},"x":%d,"y":%d}`,
			index, index, index%20, index/20)
	}
	source.WriteString(`]}`)
	jsonData := []byte(source.String())
	manifest, err := decodeJSON[sceneManifest](jsonData, "main.scene.json")
	if err != nil {
		b.Fatal(err)
	}
	binaryData, err := typedbin.Marshal(manifest)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportMetric(float64(len(jsonData)), "json-bytes")
	b.ReportMetric(float64(len(binaryData)), "binary-bytes")

	b.Run("JSON", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			if _, err := decodeJSON[sceneManifest](jsonData, "main.scene.json"); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("Binary", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			if _, err := typedbin.Unmarshal[sceneManifest](binaryData); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestPreprocessedSceneIsSmaller(t *testing.T) {
	var source strings.Builder
	source.WriteString(`{"nodes":[`)
	for index := range 100 {
		if index > 0 {
			source.WriteByte(',')
		}
		fmt.Fprintf(&source, `{"id":"item-%d","components":{"text":{"value":"repeated label","fontSize":16,"color":"#ffffffff"}},"coordinateSpace":"ui","x":%d}`, index, index)
	}
	source.WriteString(`]}`)
	input := []byte(source.String())
	files, err := PreprocessManifests(fstest.MapFS{
		"resources/scenes.json":            {Data: []byte(`{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)},
		"resources/scenes/main.scene.json": {Data: input},
	}, PreprocessOptions{Root: "."})
	if err != nil {
		t.Fatal(err)
	}
	compiled := slices.IndexFunc(files, func(file PreprocessedFile) bool { return file.Path == "resources/scenes/main.scene.json" })
	if compiled < 0 {
		t.Fatal("preprocessed scene is missing")
	}
	if len(files[compiled].Data) >= len(input) {
		t.Fatalf("preprocessed scene size = %d, source JSON size = %d", len(files[compiled].Data), len(input))
	}
}

func TestDecodeJSONRejectsPreprocessedDataAsSource(t *testing.T) {
	data, err := typedbin.Marshal(gameManifest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeJSON[gameManifest](bytes.Clone(data), "project.json"); err == nil {
		t.Fatal("binary data was accepted by the source JSON decoder")
	}
}
