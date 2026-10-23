//go:build !onix_release

package project

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene/render3d"
)

func TestCameraHDREnvironmentSharesBakeAndReleasesTexture(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "game/resources/project.json", `{}`)
	writeTestFile(t, dir, "game/resources/resources.json", `{}`)
	writeTestFile(t, dir, "game/resources/scenes/main.scene.json", `{"nodes":[
		{"id":"camera","components":{"camera":{"environment":{"path":"resources/environment/sky.hdr","intensity":0.6,"exposure":1.2,"background":true}}}},
		{"id":"second","components":{"camera":{"environment":{"path":"resources/environment/duplicate.HDR","intensity":2,"exposure":0.8}}}}
	]}`)
	data := append([]byte("#?RADIANCE\nFORMAT=32-bit_rle_rgbe\n\n-Y 1 +X 2\n"), bytes.Repeat([]byte{128, 64, 32, 134}, 2)...)
	writeTestBinaryFile(t, dir, "game/resources/environment/sky.hdr", data)
	writeTestBinaryFile(t, dir, "game/resources/environment/duplicate.HDR", data)
	bundle, err := loadTestProject(os.DirFS(dir), "game")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bundle.Dispose() })
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(scene.Dispose)
	first, ok := ecs.Get[render3d.Environment](scene.World, scene.MustEntity("camera"))
	if !ok || first.Atlas == nil || first.Encoding != render.EnvironmentRGBE || first.Intensity != .6 || first.Exposure != 1.2 || !first.Background {
		t.Fatalf("HDR environment = %+v", first)
	}
	second, _ := ecs.Get[render3d.Environment](scene.World, scene.MustEntity("second"))
	if first.Atlas != second.Atlas || first.Panorama != second.Panorama || first.Encoding != second.Encoding || second.Intensity != 2 || second.Exposure != .8 || second.Background {
		t.Fatalf("second camera must share the bake but retain settings: %+v", second)
	}
	if first.Atlas.Width != 256 || first.Atlas.Height != 1024 || first.Atlas.Format() != render.TextureFormatRGBA8 || first.Atlas.PremultipliedAlpha() || first.Atlas.Mipmapped() {
		t.Fatal("HDR bake has incorrect texture dimensions/encoding")
	}
	if statistics := bundle.resources.environments.statistics(); statistics.Resident != 1 || statistics.Misses != 1 || statistics.Hits != 1 {
		t.Fatalf("HDR cache did not share content: %+v", statistics)
	}
	if err := bundle.Dispose(); err != nil {
		t.Fatal(err)
	}
	if first.Atlas.Valid() || first.Panorama.Valid() {
		t.Fatal("disposed bundle retained the HDR atlas")
	}
}

func TestEnvironmentPathsAndMalformedHDR(t *testing.T) {
	for _, path := range []string{"resources/sky.hdr", "resources/sky.HDR", "resources/sky.png", "resources/sky.jpg", "resources/sky.JPEG", "sky.hdr", "resources/../sky.hdr"} {
		if err := validateEnvironmentSpec(&EnvironmentSpec{Path: path, Intensity: 1}); err != nil {
			t.Fatalf("valid path %q: %v", path, err)
		}
	}
	for _, path := range []string{"https://example.com/sky.jpg", "/sky.hdr", "resources/sky.exr", "resources/sky.gif"} {
		if err := validateEnvironmentSpec(&EnvironmentSpec{Path: path}); err == nil {
			t.Fatalf("accepted invalid environment path %q", path)
		}
	}
	if _, err := ResolveResourcePath("", "../resources/sky.hdr"); err == nil {
		t.Fatal("environment escaped mounted resources")
	}
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/broken.hdr", "not an HDR file")
	resources := newResources()
	resources.source = os.DirFS(dir)
	t.Cleanup(func() { _ = resources.Dispose() })
	if _, err := resources.panoramaEnvironmentFromPath("resources/broken.hdr"); err == nil {
		t.Fatal("accepted malformed HDR")
	}
	if statistics := resources.environments.statistics(); statistics.Resident != 0 {
		t.Fatalf("failed decode cached a resident texture: %+v", statistics)
	}
}

func TestCameraEnvironmentLoadsRelativeToProjectAndUpdates(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "game/resources/project.json", `{}`)
	writeTestFile(t, dir, "game/resources/resources.json", `{}`)
	writeTestFile(t, dir, "game/resources/scenes/main.scene.json", `{"nodes":[{"id":"camera","components":{"camera":{"environment":{"path":"resources/sky.png","intensity":0.6,"exposure":1.2,"background":true}}}}]}`)
	file, err := os.Create(filepath.Join(dir, "game/resources/sky.png"))
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(file, image.NewNRGBA(image.Rect(0, 0, 16, 64)))
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("write atlas: %v, %v", err, closeErr)
	}
	bundle, err := loadTestProject(os.DirFS(dir), "game")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bundle.Dispose() })
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(scene.Dispose)
	entity := scene.MustEntity("camera")
	environment, ok := ecs.Get[render3d.Environment](scene.World, entity)
	if !ok || environment.Atlas == nil || environment.Atlas.Format() != render.TextureFormatRGBA8 || environment.Intensity != .6 || environment.Exposure != 1.2 || !environment.Background {
		t.Fatalf("camera environment not loaded: %+v", environment)
	}
	camera := NewCamera3DSpec()
	camera.Environment = &EnvironmentSpec{Exposure: 2}
	components := SceneComponents{Camera: &camera}
	cloned := components.Clone()
	cloned.Camera.Environment.Exposure = 3
	if components.Camera.Environment.Exposure != 2 {
		t.Fatal("camera clone shares environment settings")
	}
	if err := scene.ApplyNode(SceneNodeSpec{ID: "camera", Components: components}); err != nil {
		t.Fatal(err)
	}
	environment, _ = ecs.Get[render3d.Environment](scene.World, entity)
	if environment.Atlas != nil || environment.Exposure != 2 {
		t.Fatalf("camera retained its previous atlas: %+v", environment)
	}
	if err := scene.ApplyNode(SceneNodeSpec{ID: "camera"}); err != nil {
		t.Fatal(err)
	}
	if ecs.Has[render3d.Environment](scene.World, entity) {
		t.Fatal("removed camera retained its environment component")
	}
}

func TestUltraHDREnvironmentCachesOriginalJPEGAndDisposesAtlas(t *testing.T) {
	data, err := os.ReadFile("../testdata/resources/environment/royal_esplanade_2k.hdr.jpg")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeTestBinaryFile(t, dir, "resources/sky.jpg", data)
	writeTestBinaryFile(t, dir, "resources/same.JPEG", data)
	resources := newResources()
	resources.source = os.DirFS(dir)
	t.Cleanup(func() { _ = resources.Dispose() })
	load := func(path string) render3d.Environment {
		t.Helper()
		e, err := resources.modEnvironment(&Camera3DSpec{Environment: &EnvironmentSpec{Path: path, Intensity: 1, Background: true}})
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	first, second := load("resources/sky.jpg"), load("resources/same.JPEG")
	if first.Atlas != second.Atlas || first.Panorama != second.Panorama || first.Encoding != render.EnvironmentRGBE {
		t.Fatal("Ultra HDR did not share its RGBE bake")
	}
	if first.Panorama == nil || first.Panorama.Width != 2048 || first.Panorama.Height != 1024 || first.Panorama.Format() != render.TextureFormatRGBA8 || first.Panorama.PremultipliedAlpha() || first.Panorama.Mipmapped() {
		t.Fatal("Ultra HDR background lost source resolution or RGBE data")
	}
	if stats := resources.environments.statistics(); stats.Resident != 1 || stats.Misses != 1 || stats.Hits != 1 {
		t.Fatal("incorrect Ultra HDR cache", stats)
	}
	if err := resources.Dispose(); err != nil {
		t.Fatal(err)
	}
	if first.Atlas.Valid() || first.Panorama.Valid() {
		t.Fatal("disposed Ultra HDR atlas remains resident")
	}
}
