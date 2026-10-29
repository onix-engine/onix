package project

import (
	"fmt"
	iofs "io/fs"
	pathpkg "path"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/resource"
	"github.com/diiyw/onix/scene"
)

// Render2DProfile is the smallest scene profile. It materializes only the
// 2D transform, sprite, and text components used by the renderer.
type Render2DProfile struct{}

// Render2DAnimationsProfile adds the 2D animator to Render2DProfile.
type Render2DAnimationsProfile struct{}

func new2DResources() *Resources {
	return &Resources{
		textures:          newResourceCache[textureResourceKey, *graphics.Texture](0),
		environments:      newResourceCache[resource.ContentID, environmentResource](0),
		atlasMetadata:     newResourceCache[resource.ContentID, *graphics.AtlasMetadata](0),
		atlases:           newResourceCache[atlasResourceKey, *graphics.Atlas](0),
		fonts:             newResourceCache[string, fontResource](0),
		shaders:           newResourceCache[string, shaderResource](0),
		material2DShaders: newResourceCache[string, *graphics.Material2DShader](0),
		named:             make(map[string]any),
		fontDecoder:       builtinFontDecoder{},
	}
}

func (Render2DProfile) loadProfileResources(source iofs.FS, projectDir, file string, feature ResourceFeature) (*Resources, error) {
	return load2DResources(source, projectDir, file, feature)
}

func (Render2DProfile) componentBuilder() sceneComponentBuilder {
	return build2DSceneComponents
}

func (Render2DProfile) componentClearer() sceneComponentClearer { return clear2DSceneComponents }

func (Render2DProfile) uiSystems() sceneUISync { return sceneUISync{} }

func (Render2DAnimationsProfile) loadProfileResources(source iofs.FS, projectDir, file string, feature ResourceFeature) (*Resources, error) {
	return load2DResources(source, projectDir, file, feature)
}

func (Render2DAnimationsProfile) componentBuilder() sceneComponentBuilder {
	return build2DAnimationsSceneComponents
}

func (Render2DAnimationsProfile) componentClearer() sceneComponentClearer {
	return clear2DAnimationSceneComponents
}

func (Render2DAnimationsProfile) uiSystems() sceneUISync { return sceneUISync{} }

func build2DSceneComponents(bundle *Bundle, node sceneSpawnNode) (ecs.Bundle, error) {
	return buildProfile2DSceneComponents(bundle, node, false)
}

func build2DAnimationsSceneComponents(bundle *Bundle, node sceneSpawnNode) (ecs.Bundle, error) {
	return buildProfile2DSceneComponents(bundle, node, true)
}

func buildProfile2DSceneComponents(bundle *Bundle, node sceneSpawnNode, animations bool) (ecs.Bundle, error) {
	if node.Has3DComponents || node.HasHTML || node.HasParticle || node.HasTilemap || node.HasSpine || node.HasPhysics2D || node.HasPhysics3D || node.Spec.Control != nil || node.Spec.Layout != nil || node.Spec.Widget != nil {
		return ecs.Bundle{}, fmt.Errorf("scene node %q uses a component unavailable in the selected 2D profile", node.Spec.ID)
	}
	if node.HasAnimation && !animations {
		return ecs.Bundle{}, fmt.Errorf("scene node %q requires the Animations 2D profile", node.Spec.ID)
	}
	spec := node.Spec
	components := ecs.NewBundle(16)
	components.Put(scene.Name{Value: spec.ID})
	if node.UI {
		components.Put(node.UITransform.Component())
		components.Put(scene.UIGlobalTransform{})
		components.Put(scene.UIStackIndex(spec.Order))
		components.Put(scene.SiblingOrder(node.LayoutOrder))
	} else {
		components.Put(node.Transform)
		components.Put(scene.GlobalTransform{})
		components.Put(scene.RenderOrder2D(spec.Order))
	}
	components.Put(scene.Visibility{Visible: node.Visible})
	components.Put(spec.Anchor)
	var animator scene.Animator
	requiresSprite := false
	if animations && node.HasAnimation {
		var err error
		animator, requiresSprite, err = bundle.ecsAnimationForSpec(spec, &node)
		if err != nil {
			return ecs.Bundle{}, err
		}
	}
	if spec.Components.VisualsEmpty() {
		components.Put(scene.Size2D{Width: float32(spec.Width), Height: float32(spec.Height)})
	}
	if node.HasSprite {
		sprite, err := bundle.ecsSpriteForSpec(spec)
		if err != nil {
			return ecs.Bundle{}, err
		}
		if requiresSprite {
			if texture := animator.CurrentTexture(); texture != nil {
				sprite.Texture = texture
				sprite.Slice = nil
				if spec.Width <= 0 {
					sprite.Width = float32(texture.LogicalWidth())
				}
				if spec.Height <= 0 {
					sprite.Height = float32(texture.LogicalHeight())
				}
			}
		}
		components.Put(sprite)
	}
	if node.HasText {
		text, err := bundle.ecsTextForPlanNode(node)
		if err != nil {
			return ecs.Bundle{}, err
		}
		components.Put(text)
	}
	if animations && node.HasAnimation {
		if requiresSprite && !node.HasSprite {
			sprite := scene.Sprite2DFromTexture(animator.CurrentTexture())
			sprite.Alpha = spec.Alpha
			if spec.Width > 0 {
				sprite.Width = float32(spec.Width)
			}
			if spec.Height > 0 {
				sprite.Height = float32(spec.Height)
			}
			components.Put(sprite)
		}
		components.Put(animator)
	}
	return components, nil
}

func load2DResources(source iofs.FS, projectDir, file string, feature ResourceFeature) (*Resources, error) {
	exists, err := projectFileExists(source, file)
	if err != nil {
		return nil, err
	}
	store := new2DResources()
	store.source, store.baseDir = source, projectDir
	if !exists {
		if err := configureResourceFeature(store, source, projectDir, feature, ResourceEntries{}); err != nil {
			return nil, err
		}
		return store, nil
	}
	manifest, err := readJSON[resourceManifest](source, file)
	if err != nil {
		return nil, err
	}
	if len(manifest.Audio) > 0 || len(manifest.Videos) > 0 || len(manifest.Effects) > 0 || len(manifest.Tilemaps) > 0 || len(manifest.Models) > 0 || len(manifest.Spines) > 0 || len(manifest.HTML) > 0 {
		return nil, fmt.Errorf("%s contains resources unavailable in the selected 2D profile", pathpkg.Base(file))
	}
	prefabSpecs, err := normalizePrefabResourceSpecs(manifest.Prefabs)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", pathpkg.Base(file), err)
	}
	shaderSpecs, err := indexManifestItems(file, manifest.Shaders, "shader", func(spec ShaderSpec) string { return spec.ID }, func(spec ShaderSpec) error {
		if err := spec.Validate(); err != nil {
			return fmt.Errorf("%s: %w", pathpkg.Base(file), err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	store.fonts = newResourceCache[string, fontResource](len(manifest.Fonts))
	store.shaders = newResourceCache[string, shaderResource](len(shaderSpecs))
	store.material2DShaders = newResourceCache[string, *graphics.Material2DShader](len(shaderSpecs))
	store.prefabSpecs = prefabSpecs
	if err := configureResourceFeature(store, source, projectDir, feature, ResourceEntries{}); err != nil {
		return nil, err
	}
	for _, item := range manifest.Fonts {
		if item.ID == "" || item.Path == "" {
			return nil, fmt.Errorf("resources.json font entry requires id and path")
		}
		resourcePath, err := ResolveResourcePath(projectDir, item.Path)
		if err != nil {
			return nil, fmt.Errorf("font %q path: %w", item.ID, err)
		}
		item := item
		if err := store.fonts.register(item.ID, func() (fontResource, error) { return store.loadFont(resourcePath, item.Path) }, fontResource.dispose); err != nil {
			return nil, fmt.Errorf("register font %q: %w", item.ID, err)
		}
	}
	for id, spec := range shaderSpecs {
		id, spec := id, spec
		if err := store.shaders.register(id, func() (shaderResource, error) { return spec.load(source, projectDir) }, shaderResource.dispose); err != nil {
			return nil, fmt.Errorf("register shader %q: %w", id, err)
		}
		if err := store.material2DShaders.register(id, func() (*graphics.Material2DShader, error) {
			descriptor, err := store.Shader(id)
			if err != nil {
				return nil, err
			}
			return graphics.NewMaterial2DShader(descriptor)
		}, (*graphics.Material2DShader).Dispose); err != nil {
			return nil, fmt.Errorf("register Material2D shader %q: %w", id, err)
		}
	}
	return store, nil
}
