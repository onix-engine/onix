package project

import (
	"fmt"
	iofs "io/fs"

	"github.com/diiyw/onix/audio"
	"github.com/diiyw/onix/graphics"
	graphicsgif "github.com/diiyw/onix/graphics/codec/gif"
	graphicsjpeg "github.com/diiyw/onix/graphics/codec/jpeg"
	graphicspng "github.com/diiyw/onix/graphics/codec/png"
	graphicssvg "github.com/diiyw/onix/graphics/codec/svg"
)

type testResourceFeature struct{}

func (testResourceFeature) ConfigureResources(context ResourceLoadContext, entries ResourceEntries) error {
	context.SetImageDecoder(graphics.ImageDecoderSet[
		graphicssvg.Decoder,
		graphicsgif.Decoder,
		graphicsjpeg.Decoder,
		graphicspng.Decoder,
		graphics.NoImageDecoder,
	]{})
	for _, entry := range entries.Audio {
		if _, err := audio.FormatFromPath(entry.Path); err != nil {
			return err
		}
		entry := entry
		if err := RegisterNamedResource(context, "test-audio", entry.ID, ResourceRegistration[string]{
			Load: func() (string, error) {
				data, err := context.ReadFile(entry.Path)
				if err != nil {
					return "", err
				}
				return string(data), nil
			},
		}); err != nil {
			return fmt.Errorf("register audio %q: %w", entry.ID, err)
		}
	}
	return nil
}

func loadTestProject(source iofs.FS, root string) (*Bundle, error) {
	return LoadConfiguredWith[testResourceFeature](source, root, testProjectEngineConfig())
}

func testProjectEngineConfig() EngineConfig {
	return EngineConfig{
		Features: []RuntimeFeatureID{
			FeatureRender2D,
			FeatureRender3D,
			FeatureInput2D,
			FeatureAnimations2D,
			FeatureSpriteTrails2D,
			FeatureParticles2D,
			FeatureTilemaps2D,
			FeatureSpine2D,
			FeaturePhysics2D,
			FeatureUIControls,
			FeatureUIWidgets,
			FeatureUITextInput,
			FeatureUIMarkdown,
			FeatureUIHTML,
		},
		Resources: []ResourceID{
			ResourceImages,
			ResourceFonts,
			ResourceAudio,
			ResourceVideos,
			ResourceParticles,
			ResourceTilemaps,
			ResourceModels,
			ResourceSpine,
			ResourceShaders,
		},
	}
}
