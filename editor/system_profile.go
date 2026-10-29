package editor

import (
	"slices"

	"github.com/diiyw/onix/project"
)

type projectImageDecoderDescriptor struct {
	ID         project.ImageDecoderID
	Label      string
	FieldName  string
	ImportName string
	ImportPath string
	GoType     string
}

var projectImageDecoderDescriptors = [...]projectImageDecoderDescriptor{
	{ID: project.ImageDecoderGIF, Label: "GIF", FieldName: "project.engine.imageDecoders.gif", ImportName: "graphicsgif", ImportPath: "github.com/diiyw/onix/graphics/codec/gif", GoType: "graphicsgif.Decoder"},
	{ID: project.ImageDecoderJPEG, Label: "JPEG (.jpg)", FieldName: "project.engine.imageDecoders.jpeg", ImportName: "graphicsjpeg", ImportPath: "github.com/diiyw/onix/graphics/codec/jpeg", GoType: "graphicsjpeg.Decoder"},
	{ID: project.ImageDecoderPNG, Label: "PNG", FieldName: "project.engine.imageDecoders.png", ImportName: "graphicspng", ImportPath: "github.com/diiyw/onix/graphics/codec/png", GoType: "graphicspng.Decoder"},
	{ID: project.ImageDecoderWebP, Label: "WebP", FieldName: "project.engine.imageDecoders.webp", ImportName: "graphicswebp", ImportPath: "github.com/diiyw/onix/graphics/codec/webp", GoType: "graphicswebp.Decoder"},
}

// projectTextureContainerDecoder is not a user-selectable image format. A
// platform that packs block-compressed textures always has to read them back,
// so the container decoder follows the texture compression switch instead of
// the image decoder list.
var projectTextureContainerDecoder = projectImageDecoderDescriptor{
	Label:      "Compressed texture container",
	ImportName: "ontex",
	ImportPath: "github.com/diiyw/onix/graphics/codec/ontex",
	GoType:     "ontex.Decoder",
}

func projectImageDecoderEnabled(decoders []project.ImageDecoderID, id project.ImageDecoderID) bool {
	for _, decoder := range decoders {
		if decoder == id {
			return true
		}
	}
	return false
}

func setProjectImageDecoder(decoders []project.ImageDecoderID, id project.ImageDecoderID, enabled bool) []project.ImageDecoderID {
	if enabled {
		if projectImageDecoderEnabled(decoders, id) {
			return decoders
		}
		return append(decoders, id)
	}
	for index, decoder := range decoders {
		if decoder != id {
			continue
		}
		copy(decoders[index:], decoders[index+1:])
		return decoders[:len(decoders)-1]
	}
	return decoders
}

type projectRuntimeFeatureDescriptor struct {
	Content    []string
	ID         project.RuntimeFeatureID
	Label      string
	GoType     string
	ImportName string
	ImportPath string
	Register   bool
}

// Keep this table in the editor because it is the code-generation boundary:
// manifest IDs are stable data, while Go type names are compile-time API.
var projectRuntimeFeatureDescriptors = [...]projectRuntimeFeatureDescriptor{
	{ID: project.FeatureRender2D, Label: "Render 2D", GoType: "Render2D"},
	{ID: project.FeatureRender3D, Content: []string{"SpatialContent"}, Label: "Render 3D", GoType: "Render3D"},
	{ID: project.FeatureInput2D, Label: "Input 2D", GoType: "Input2D"},
	{ID: project.FeatureAnimations2D, Content: []string{"AnimationsContent"}, Label: "Animations 2D", GoType: "Animations2D"},
	{ID: project.FeatureSpriteTrails2D, Content: []string{"TrailsContent"}, Label: "Sprite Trails 2D", GoType: "SpriteTrails2D"},
	{ID: project.FeatureParticles2D, Content: []string{"ParticlesContent"}, Label: "Particles 2D", GoType: "Particles2D"},
	{ID: project.FeatureTilemaps2D, Content: []string{"TilemapsContent"}, Label: "Tilemaps 2D", GoType: "Tilemaps2D"},
	{ID: project.FeatureSpine2D, Content: []string{"SpineContent"}, Label: "Spine 2D", GoType: "Spine2D"},
	{ID: project.FeaturePhysics2D, Content: []string{"Physics2DContent"}, Label: "Physics 2D", GoType: "Physics2D"},
	{ID: project.FeaturePhysics3D, Content: []string{"SpatialContent"}, Label: "Physics 3D", GoType: "Physics3D"},
	{ID: project.FeatureUIControls, Content: []string{"LayoutContent"}, Label: "UI Controls", GoType: "UIControls"},
	{ID: project.FeatureUIWidgets, Content: []string{"LayoutContent"}, Label: "UI Widgets", GoType: "UIWidgets"},
	{ID: project.FeatureUITextInput, Content: []string{"LayoutContent"}, Label: "UI Text Input", GoType: "UITextInput"},
	{ID: project.FeatureUIMarkdown, Content: []string{"LayoutContent"}, Label: "UI Markdown", GoType: "UIMarkdown[markdown.Feature]", ImportName: "markdown", ImportPath: "github.com/diiyw/onix/ui/markdown"},
	{ID: project.FeatureUIHTML, Content: []string{"LayoutContent", "HTMLContent"}, Label: "UI HTML", GoType: "UIHTML"},
	{ID: project.FeatureWebView, Content: []string{"LayoutContent"}, Label: "WebView", GoType: "WebView", ImportName: "onixwebview", ImportPath: "github.com/diiyw/onix/window/webview", Register: true},
	{ID: project.FeatureFileDialogs, Label: "File Dialogs", ImportName: "filedialog", ImportPath: "github.com/diiyw/onix/window/filedialog", Register: true},
}

type projectResourceDescriptor struct {
	ID        project.ResourceID
	Label     string
	FieldName string
}

var projectResourceDescriptors = [...]projectResourceDescriptor{
	{ID: project.ResourceImages, Label: "Images", FieldName: "project.engine.resources.images"},
	{ID: project.ResourceFonts, Label: "Fonts", FieldName: "project.engine.resources.fonts"},
	{ID: project.ResourceAudio, Label: "Audio", FieldName: "project.engine.resources.audio"},
	{ID: project.ResourceVideos, Label: "Video", FieldName: "project.engine.resources.videos"},
	{ID: project.ResourceParticles, Label: "Particles", FieldName: "project.engine.resources.particles"},
	{ID: project.ResourceTilemaps, Label: "Tilemaps", FieldName: "project.engine.resources.tilemaps"},
	{ID: project.ResourceModels, Label: "3D Models", FieldName: "project.engine.resources.models"},
	{ID: project.ResourceSpine, Label: "Spine", FieldName: "project.engine.resources.spine"},
	{ID: project.ResourceShaders, Label: "Shaders", FieldName: "project.engine.resources.shaders"},
	{ID: project.ResourceHTML, Label: "HTML", FieldName: "project.engine.resources.html"},
}

func setProjectResource(config project.EngineConfig, id project.ResourceID, enabled bool) (project.EngineConfig, []project.ResourceID) {
	resources := slices.Clone(config.Resources)
	added := make([]project.ResourceID, 0, 1)
	set := func(resource project.ResourceID) {
		for _, current := range resources {
			if current == resource {
				return
			}
		}
		resources = append(resources, resource)
		added = append(added, resource)
	}
	if enabled {
		set(id)
		switch id {
		case project.ResourceSpine:
			set(project.ResourceImages)
		}
	} else {
		for index := 0; index < len(resources); {
			current := resources[index]
			if current != id && !(id == project.ResourceImages && current == project.ResourceSpine) {
				index++
				continue
			}
			copy(resources[index:], resources[index+1:])
			resources = resources[:len(resources)-1]
		}
	}
	config.Resources = resources
	return config, added
}

func setProjectRuntimeFeature(config project.EngineConfig, id project.RuntimeFeatureID, enabled bool) (project.EngineConfig, []project.RuntimeFeatureID) {
	features := slices.Clone(config.Features)
	added := make([]project.RuntimeFeatureID, 0, 4)
	set := func(feature project.RuntimeFeatureID) {
		for _, current := range features {
			if current == feature {
				return
			}
		}
		features = append(features, feature)
		added = append(added, feature)
	}
	if enabled {
		set(id)
		switch id {
		case project.FeatureRender3D:
			// 3D has no 2D renderer dependency.
		case project.FeatureAnimations2D, project.FeatureSpriteTrails2D, project.FeatureParticles2D, project.FeatureTilemaps2D, project.FeatureSpine2D:
			set(project.FeatureRender2D)
		case project.FeatureUIControls, project.FeatureUIWidgets:
			set(project.FeatureRender2D)
			set(project.FeatureInput2D)
		case project.FeatureUITextInput, project.FeatureUIMarkdown, project.FeatureUIHTML:
			set(project.FeatureRender2D)
			set(project.FeatureInput2D)
			set(project.FeatureUIWidgets)
		}
	} else {
		for index := 0; index < len(features); {
			current := features[index]
			if current != id && !runtimeFeatureDependsOn(current, id) {
				index++
				continue
			}
			copy(features[index:], features[index+1:])
			features = features[:len(features)-1]
		}
	}
	config.Features = features
	return config, added
}

func runtimeFeatureDependsOn(feature, dependency project.RuntimeFeatureID) bool {
	for _, required := range runtimeFeatureDependenciesForEditor(feature) {
		if required == dependency {
			return true
		}
	}
	return false
}

func runtimeFeatureDependenciesForEditor(feature project.RuntimeFeatureID) []project.RuntimeFeatureID {
	switch feature {
	case project.FeatureAnimations2D, project.FeatureSpriteTrails2D, project.FeatureParticles2D, project.FeatureTilemaps2D, project.FeatureSpine2D:
		return []project.RuntimeFeatureID{project.FeatureRender2D}
	case project.FeatureUIControls, project.FeatureUIWidgets:
		return []project.RuntimeFeatureID{project.FeatureRender2D, project.FeatureInput2D}
	case project.FeatureUITextInput, project.FeatureUIMarkdown, project.FeatureUIHTML:
		return []project.RuntimeFeatureID{project.FeatureRender2D, project.FeatureInput2D, project.FeatureUIWidgets}
	default:
		return nil
	}
}
