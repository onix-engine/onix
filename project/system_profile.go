package project

import (
	"encoding/json"
	"errors"
	"fmt"
	iofs "io/fs"
	"path"
)

// RuntimeFeatureID identifies one engine-owned runtime feature in a project
// manifest. IDs are persisted instead of Go type names so manifests remain
// stable when a feature's implementation changes.
type RuntimeFeatureID string

// ResourceID identifies one resource family enabled by a project.
type ResourceID string

// ImageDecoderID identifies one statically linked image format decoder.
type ImageDecoderID string

const (
	ResourceImages    ResourceID = "images"
	ResourceFonts     ResourceID = "fonts"
	ResourceAudio     ResourceID = "audio"
	ResourceVideos    ResourceID = "videos"
	ResourceParticles ResourceID = "particles"
	ResourceTilemaps  ResourceID = "tilemaps"
	ResourceModels    ResourceID = "models"
	ResourceSpine     ResourceID = "spine"
	ResourceShaders   ResourceID = "shaders"
	ResourceHTML      ResourceID = "html"
)

const (
	ImageDecoderGIF  ImageDecoderID = "gif"
	ImageDecoderJPEG ImageDecoderID = "jpeg"
	ImageDecoderPNG  ImageDecoderID = "png"
	ImageDecoderWebP ImageDecoderID = "webp"
)

const (
	FeatureRender2D       RuntimeFeatureID = "render2d"
	FeatureRender3D       RuntimeFeatureID = "render3d"
	FeatureInput2D        RuntimeFeatureID = "input2d"
	FeatureAnimations2D   RuntimeFeatureID = "animations2d"
	FeatureSpriteTrails2D RuntimeFeatureID = "sprite-trails2d"
	FeatureParticles2D    RuntimeFeatureID = "particles2d"
	FeatureTilemaps2D     RuntimeFeatureID = "tilemaps2d"
	FeatureSpine2D        RuntimeFeatureID = "spine2d"
	FeaturePhysics2D      RuntimeFeatureID = "physics2d"
	FeaturePhysics3D      RuntimeFeatureID = "physics3d"
	FeatureUIControls     RuntimeFeatureID = "ui-controls"
	FeatureUIWidgets      RuntimeFeatureID = "ui-widgets"
	FeatureUITextInput    RuntimeFeatureID = "ui-text-input"
	FeatureUIMarkdown     RuntimeFeatureID = "ui-markdown"
	FeatureUIHTML         RuntimeFeatureID = "ui-html"
	FeatureWebView        RuntimeFeatureID = "webview"
	FeatureFileDialogs    RuntimeFeatureID = "file-dialogs"
)

// EngineConfig contains explicit project-level runtime and resource choices.
// The editor turns feature IDs into static InstallSystems calls during code
// generation.
type EngineConfig struct {
	Features      []RuntimeFeatureID `json:"features,omitempty"`
	Resources     []ResourceID       `json:"resources,omitempty"`
	ImageDecoders []ImageDecoderID   `json:"imageDecoders,omitempty"`
	// TextureCompression enables build-time block compression. When an engine
	// section is present and the field is omitted, project loading defaults it to
	// true; set it to false for source-texture builds.
	TextureCompression bool `json:"textureCompression,omitempty"`
	// TextureCompressionExcludes lists project-relative resources that must keep
	// their source pixels, for example "resources/ui.png". This is useful for
	// pixel-art and other assets where block compression is visibly lossy.
	TextureCompressionExcludes []string `json:"textureCompressionExcludes,omitempty"`
}

// ReadEngineConfig reads the engine section of a project manifest. A project
// without a manifest reports the zero config, which leaves optional features
// off.
func ReadEngineConfig(source iofs.FS, root string) (EngineConfig, error) {
	manifest, _, err := loadGameManifest(source, root)
	if err != nil {
		return EngineConfig{}, err
	}
	config := manifest.Engine
	config.TextureCompression = true
	data, err := iofs.ReadFile(source, path.Join(root, "resources", "project.json"))
	if err != nil {
		if errors.Is(err, iofs.ErrNotExist) {
			return manifest.Engine, nil
		}
		return EngineConfig{}, err
	}
	var raw struct {
		Engine map[string]json.RawMessage `json:"engine"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return EngineConfig{}, err
	}
	if raw.Engine == nil {
		return manifest.Engine, nil
	}
	config.TextureCompression = true
	if value, ok := raw.Engine["textureCompression"]; ok {
		if err := json.Unmarshal(value, &config.TextureCompression); err != nil {
			return EngineConfig{}, fmt.Errorf("textureCompression: %w", err)
		}
	}
	return config, nil
}

// Validate checks IDs, duplicates, and explicit feature dependencies.
func (config EngineConfig) Validate() error {
	for index, id := range config.Resources {
		if !validResourceID(id) {
			return fmt.Errorf("unsupported project resource %q", id)
		}
		for _, previous := range config.Resources[:index] {
			if previous == id {
				return fmt.Errorf("duplicate project resource %q", id)
			}
		}
	}
	for index, id := range config.ImageDecoders {
		if !validImageDecoderID(id) {
			return fmt.Errorf("unsupported image decoder %q", id)
		}
		for _, previous := range config.ImageDecoders[:index] {
			if previous == id {
				return fmt.Errorf("duplicate image decoder %q", id)
			}
		}
	}
	for index, id := range config.Features {
		if !validRuntimeFeatureID(id) {
			return fmt.Errorf("unsupported runtime feature %q", id)
		}
		for _, previous := range config.Features[:index] {
			if previous == id {
				return fmt.Errorf("duplicate runtime feature %q", id)
			}
		}
	}
	for _, id := range config.Resources {
		if id == ResourceSpine && !containsResource(config.Resources, ResourceImages) {
			return fmt.Errorf("resource %q requires %q", id, ResourceImages)
		}
	}
	for _, id := range config.Features {
		for _, dependency := range runtimeFeatureDependencies(id) {
			if !containsRuntimeFeature(config.Features, dependency) {
				return fmt.Errorf("runtime feature %q requires %q", id, dependency)
			}
		}
	}
	return nil
}

func containsResource(values []ResourceID, id ResourceID) bool {
	for _, value := range values {
		if value == id {
			return true
		}
	}
	return false
}

func containsRuntimeFeature(values []RuntimeFeatureID, id RuntimeFeatureID) bool {
	for _, value := range values {
		if value == id {
			return true
		}
	}
	return false
}

func runtimeFeatureDependencies(id RuntimeFeatureID) []RuntimeFeatureID {
	switch id {
	case FeatureAnimations2D, FeatureSpriteTrails2D, FeatureParticles2D, FeatureTilemaps2D, FeatureSpine2D:
		return []RuntimeFeatureID{FeatureRender2D}
	case FeatureUIControls, FeatureUIWidgets:
		return []RuntimeFeatureID{FeatureRender2D, FeatureInput2D}
	case FeatureUITextInput, FeatureUIMarkdown, FeatureUIHTML:
		return []RuntimeFeatureID{FeatureRender2D, FeatureInput2D, FeatureUIWidgets}
	default:
		return nil
	}
}

func validImageDecoderID(id ImageDecoderID) bool {
	switch id {
	case ImageDecoderGIF, ImageDecoderJPEG, ImageDecoderPNG, ImageDecoderWebP:
		return true
	default:
		return false
	}
}

// HasFeature reports whether a runtime feature is explicitly enabled.
func (config EngineConfig) HasFeature(id RuntimeFeatureID) bool {
	for _, enabled := range config.Features {
		if enabled == id {
			return true
		}
	}
	return false
}

// HasResource reports whether a resource family is explicitly enabled.
func (config EngineConfig) HasResource(id ResourceID) bool {
	for _, enabled := range config.Resources {
		if enabled == id {
			return true
		}
	}
	return false
}

func validResourceID(id ResourceID) bool {
	switch id {
	case ResourceImages, ResourceFonts, ResourceAudio, ResourceVideos, ResourceParticles, ResourceTilemaps, ResourceModels, ResourceSpine, ResourceShaders, ResourceHTML:
		return true
	default:
		return false
	}
}

func validRuntimeFeatureID(id RuntimeFeatureID) bool {
	switch id {
	case FeatureRender2D, FeatureRender3D,
		FeatureInput2D, FeatureAnimations2D, FeatureSpriteTrails2D,
		FeatureParticles2D, FeatureTilemaps2D, FeatureSpine2D, FeaturePhysics2D, FeaturePhysics3D,
		FeatureUIControls, FeatureUIWidgets,
		FeatureUITextInput, FeatureUIMarkdown, FeatureUIHTML, FeatureWebView, FeatureFileDialogs:
		return true
	default:
		return false
	}
}

// ValidateSceneFeatures verifies that installed reports all runtime features
// needed to materialize scene id. It keeps the check at the scene boundary so
// loading a project cannot silently produce an unrendered or unanimated scene.
func (b *Bundle) ValidateSceneFeatures(id SceneID, installed func(RuntimeFeatureID) bool) error {
	if b == nil {
		return fmt.Errorf("project bundle is nil")
	}
	if installed == nil {
		return fmt.Errorf("project feature checker is nil")
	}
	plan, ok := b.scenePlansByID[id]
	if !ok {
		return fmt.Errorf("scene %q is not registered", id)
	}
	var required [14]RuntimeFeatureID
	count := 0
	add := func(feature RuntimeFeatureID) {
		for index := 0; index < count; index++ {
			if required[index] == feature {
				return
			}
		}
		required[count] = feature
		count++
	}
	for _, node := range plan.Nodes {
		if node.Has3DComponents {
			add(FeatureRender3D)
		}
		if node.HasSprite || node.HasText || node.HasParticle || node.HasTilemap || node.HasSpine || node.Spec.Control != nil {
			add(FeatureRender2D)
		}
		if node.HasAnimation {
			add(FeatureAnimations2D)
		}
		if node.Spec.Components.Sprite != nil && node.Spec.Components.Sprite.Trail != nil {
			add(FeatureSpriteTrails2D)
		}
		if node.HasParticle {
			add(FeatureParticles2D)
		}
		if node.HasTilemap {
			add(FeatureTilemaps2D)
		}
		if node.HasSpine {
			add(FeatureSpine2D)
		}
		if node.HasPhysics2D {
			add(FeaturePhysics2D)
		}
		if node.HasPhysics3D {
			add(FeaturePhysics3D)
		}
		if node.Spec.Control != nil {
			add(FeatureUIControls)
		}
		if node.Spec.Layout != nil || node.Spec.Widget != nil {
			add(FeatureUIWidgets)
		}
	}
	for index := 0; index < count; index++ {
		if !installed(required[index]) {
			return fmt.Errorf("scene %q requires runtime feature %q", id, required[index])
		}
	}
	return nil
}
