package project

import (
	"bytes"
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/diiyw/onix/graphics"
	graphicsgif "github.com/diiyw/onix/graphics/codec/gif"
	graphicsjpeg "github.com/diiyw/onix/graphics/codec/jpeg"
	graphicspng "github.com/diiyw/onix/graphics/codec/png"
	graphicswebp "github.com/diiyw/onix/graphics/codec/webp"
	"github.com/diiyw/onix/resource"
	sceneapi "github.com/diiyw/onix/scene"
)

func TestTextureCacheUsesContentIdentityAndSharesAtlasImage(t *testing.T) {
	directory := t.TempDir()
	image := tinyPNG(t)
	writeTestBinaryFile(t, directory, "a.png", image)
	writeTestBinaryFile(t, directory, "b.png", image)
	writeTestFile(t, directory, "atlas.json", `{
  "frames": [{"filename":"pixel","frame":{"x":0,"y":0,"w":1,"h":1}}],
  "meta": {"image":"a.png","size":{"w":1,"h":1}}
}`)
	resources := newResources()
	resources.source = os.DirFS(directory)
	resources.baseDir = "."
	resources.imageDecoder = graphics.ImageDecoderSet[
		graphicsgif.Decoder,
		graphicsjpeg.Decoder,
		graphicspng.Decoder,
		graphicswebp.Decoder,
		graphics.NoImageDecoder,
	]{}
	defer resources.Dispose()

	first, err := resources.texture(&TextureSpec{Image: "a.png"})
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := resources.texture(&TextureSpec{Image: "b.png"})
	if err != nil {
		t.Fatal(err)
	}
	atlas, err := resources.Atlas("atlas.json")
	if err != nil {
		t.Fatal(err)
	}
	if first != duplicate || first != atlas.Texture {
		t.Fatal("identical image contents were decoded into different textures")
	}
	if resources.textures.len() != 1 {
		t.Fatalf("resident textures = %d, want 1", resources.textures.len())
	}
	statistics := resources.Statistics()
	if statistics.CacheHits < 2 || statistics.CacheMisses != 3 || statistics.ResidentResources != 3 {
		t.Fatalf("resource statistics = %+v", statistics)
	}
}

func TestBlobBackedResourcesRemainPinnedUntilBundleDispose(t *testing.T) {
	fontData, err := os.ReadFile(filepath.Join("..", "testdata", "fonts", "Noto_Sans_SC", "static", "NotoSansSC-Regular.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	manager := newPackedProjectManager(t, map[string][]byte{
		"resources/project.json":           []byte(`{}`),
		"resources/resources.json":         []byte(`{"fonts":[{"id":"ui","path":"font.otf"}],"videos":[{"id":"intro","path":"intro.mp4"}],"shaders":[{"id":"effect","metal":"effect.metal"}]}`),
		"resources/scenes.json":            []byte(`{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`),
		"resources/scenes/main.scene.json": []byte(`{"nodes":[]}`),
		"font.otf":                         fontData,
		"intro.mp4":                        []byte("encoded video"),
		"effect.metal":                     []byte("metal source"),
	})
	defer manager.Close()
	bundle, err := Load(manager, ".")
	if err != nil {
		t.Fatal(err)
	}
	if statistics := manager.Statistics(); statistics.PinnedBlobs != 0 {
		t.Fatalf("bundle load pinned %d blobs before first access", statistics.PinnedBlobs)
	}
	if _, err := bundle.Resources().Font("ui"); err != nil {
		t.Fatal(err)
	}
	video, err := bundle.Resources().Video("intro")
	if err != nil {
		t.Fatal(err)
	}
	if data, err := video.Bytes(); err != nil || string(data) != "encoded video" {
		t.Fatalf("video bytes = %q, %v", data, err)
	}
	if _, err := bundle.Resources().Shader("effect"); err != nil {
		t.Fatal(err)
	}
	if statistics := manager.Statistics(); statistics.PinnedBlobs != 3 {
		t.Fatalf("pinned blobs = %d, want 3", statistics.PinnedBlobs)
	}
	if err := bundle.Dispose(); err != nil {
		t.Fatal(err)
	}
	if statistics := manager.Statistics(); statistics.PinnedBlobs != 0 {
		t.Fatalf("bundle dispose retained %d pinned blobs", statistics.PinnedBlobs)
	}
}

func TestModelCacheDisposesReplacedAndBundledMeshes(t *testing.T) {
	bundle := &Bundle{
		resources:            newResources(),
		modelSpecs:           map[string]ModelSpec{"shape": {ID: "shape", Mesh: &MeshSpec{Primitive: "cube", Size: 1}}},
		modelResources:       newResourceCache[string, modelResource](1),
		inlineAnimationClips: newResourceCache[string, *sceneapi.AnimationClip](0),
	}
	object := SceneNodeSpec{ID: "node", Components: SceneComponents{Model: &ResourceComponentSpec{Resource: "shape"}}}
	first, _, loaded, err := bundle.modelForSpec(object)
	if err != nil || !loaded || first == nil {
		t.Fatalf("load first model = %p, %t, %v", first, loaded, err)
	}
	scene := &SceneInstance{project: bundle}
	if err := scene.SetModelResource(ModelSpec{ID: "shape", Mesh: &MeshSpec{Primitive: "cube", Size: 1}}); err != nil {
		t.Fatal(err)
	}
	if len(first.Vertices) == 0 || len(first.Indices) == 0 {
		t.Fatal("unchanged model resource disposed its mounted mesh")
	}
	if err := scene.SetModelResource(ModelSpec{ID: "shape", Mesh: &MeshSpec{Primitive: "plane", Size: 2}}); err != nil {
		t.Fatal(err)
	}
	if len(first.Vertices) != 0 {
		t.Fatal("replaced model retained its mesh")
	}
	second, _, loaded, err := bundle.modelForSpec(object)
	if err != nil || !loaded || second == nil || second == first {
		t.Fatalf("load replacement model = %p, %t, %v", second, loaded, err)
	}
	if err := bundle.Dispose(); err != nil {
		t.Fatal(err)
	}
	if len(second.Vertices) != 0 {
		t.Fatal("bundle dispose retained its model mesh")
	}
}

func TestVideoReaderRetainsBlobPastVideoClose(t *testing.T) {
	video := newVideo(resource.NewMemoryBlob("intro.mp4", []byte("video")))
	reader, err := video.Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := video.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(reader)
	if err != nil || string(data) != "video" {
		t.Fatalf("reader data = %q, %v", data, err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := video.Bytes(); err == nil {
		t.Fatal("closed video returned borrowed bytes")
	}
}

func newPackedProjectManager(t *testing.T, files map[string][]byte) *resource.Manager {
	t.Helper()
	inputs := make([]resource.PackInput, 0, len(files))
	for name, data := range files {
		inputs = append(inputs, resource.BytesInput(name, data))
	}
	var packageData bytes.Buffer
	result, err := resource.Pack(&packageData, inputs)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(packageData.Bytes())
	catalog := resource.Catalog{Version: resource.CatalogVersion, Packages: []resource.PackageInfo{{
		Name: resource.BasePackage, File: resource.BasePackage + resource.Extension,
		SHA256: digest, Size: int64(packageData.Len()), Required: true, Assets: result.Assets,
	}}}
	catalogData, err := resource.MarshalCatalog(catalog)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := resource.NewMemoryManager(catalogData, map[string][]byte{
		resource.BasePackage + resource.Extension: packageData.Bytes(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return manager
}
