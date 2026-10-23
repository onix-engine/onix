package assets

import (
	"bytes"
	"image"
	"image/color"
	stdpng "image/png"
	"testing"
	"testing/fstest"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	graphicspng "github.com/diiyw/onix/graphics/codec/png"
	"github.com/diiyw/onix/project"
	sceneapi "github.com/diiyw/onix/scene"
)

func TestImageFeatureInstantiatesPNGSprite(t *testing.T) {
	var encoded bytes.Buffer
	pixels := image.NewRGBA(image.Rect(0, 0, 1, 1))
	pixels.SetRGBA(0, 0, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	if err := stdpng.Encode(&encoded, pixels); err != nil {
		t.Fatal(err)
	}
	source := fstest.MapFS{
		"resources/project.json":           {Data: []byte(`{}`)},
		"resources/resources.json":         {Data: []byte(`{}`)},
		"resources/scenes.json":            {Data: []byte(`{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)},
		"resources/scenes/main.scene.json": {Data: []byte(`{"nodes":[{"id":"background","components":{"sprite":{"texture":{"image":"resources/background.png"}}}}]}`)},
		"resources/background.png":         {Data: encoded.Bytes()},
	}
	type decoders = graphics.ImageDecoderSet[
		graphics.NoImageDecoder,
		graphics.NoImageDecoder,
		graphicspng.Decoder,
		graphics.NoImageDecoder,
		graphics.NoImageDecoder,
	]
	type feature = project.ResourceFeatureSet[project.ResourceFeature, project.ImageFeature[decoders]]
	bundle, err := project.LoadConfiguredWith[feature](source, ".", project.EngineConfig{
		Features:  []project.RuntimeFeatureID{project.FeatureRender2D},
		Resources: []project.ResourceID{project.ResourceImages},
	})
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	defer bundle.Dispose()

	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatalf("instantiate PNG sprite: %v", err)
	}
	defer scene.Dispose()
	if !ecs.Has[sceneapi.Sprite2D](scene.World, scene.MustEntity("background")) {
		t.Fatal("PNG sprite was not instantiated")
	}
}
