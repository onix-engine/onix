package project

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	enginephysics "github.com/diiyw/onix/physics"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/scene/render3d"
	engineui "github.com/diiyw/onix/ui"
	"github.com/diiyw/onix/ui/html"
)

type sceneComponentClearer func(*ecs.World, ecs.Entity)

func prepareAuthoredSceneRemoval(world *ecs.World) ecs.PreparedRemoval {
	return world.PrepareRemoval(
		ecs.Type[scene.Size2D](),
		ecs.Type[engineui.Layout](),
		ecs.Type[engineui.Widget](),
		ecs.Type[scene.Sprite2D](),
		ecs.Type[scene.SpriteLayers2D](),
		ecs.Type[scene.Text2D](),
		ecs.Type[html.Surface](),
		ecs.Type[scene.ParticleEmitter2D](),
		ecs.Type[scene.Tilemap2D](),
		ecs.Type[scene.Spine2D](),
		ecs.Type[scene.SpriteTrail2D](),
		ecs.Type[scene.Animator](),
		ecs.Type[render3d.Mesh](),
		ecs.Type[render3d.Material[graphics.StandardMaterial]](),
		ecs.Type[scene.Camera](),
		ecs.Type[render3d.Camera](),
		ecs.Type[scene.Projection](),
		ecs.Type[render3d.AmbientLight](),
		ecs.Type[render3d.Environment](),
		ecs.Type[render3d.DirectionalLight](),
		ecs.Type[render3d.PointLight](),
		ecs.Type[render3d.SpotLight](),
		ecs.Type[render3d.AreaLight](),
		ecs.Type[enginephysics.RigidBody2D](),
		ecs.Type[enginephysics.CircleCollider2D](),
		ecs.Type[enginephysics.BoxCollider2D](),
		ecs.Type[enginephysics.SegmentCollider2D](),
		ecs.Type[enginephysics.RigidBody3D](),
		ecs.Type[enginephysics.TriangleMeshCollider3D](),
		ecs.Type[enginephysics.ConvexHullCollider3D](),
		ecs.Type[enginephysics.CompoundCollider3D](),
	)
}

func clearAuthoredSceneComponents(world *ecs.World, entity ecs.Entity) {
	sceneRemovalFor[authoredSceneRemoval](world, prepareAuthoredSceneRemoval).Apply(entity)
}

func prepareAuthored2DSceneRemoval(world *ecs.World) ecs.PreparedRemoval {
	return world.PrepareRemoval(
		ecs.Type[scene.Size2D](),
		ecs.Type[scene.Sprite2D](),
		ecs.Type[scene.Text2D](),
	)
}

func clear2DSceneComponents(world *ecs.World, entity ecs.Entity) {
	sceneRemovalFor[authored2DSceneRemoval](world, prepareAuthored2DSceneRemoval).Apply(entity)
}

func prepareAuthored2DAnimationSceneRemoval(world *ecs.World) ecs.PreparedRemoval {
	return world.PrepareRemoval(
		ecs.Type[scene.Size2D](),
		ecs.Type[scene.Sprite2D](),
		ecs.Type[scene.Text2D](),
		ecs.Type[scene.Animator](),
	)
}

func clear2DAnimationSceneComponents(world *ecs.World, entity ecs.Entity) {
	sceneRemovalFor[authored2DAnimationSceneRemoval](world, prepareAuthored2DAnimationSceneRemoval).Apply(entity)
}

type authoredSceneRemoval struct{}
type authored2DSceneRemoval struct{}
type authored2DAnimationSceneRemoval struct{}

type sceneRemovalCache struct {
	removal     ecs.PreparedRemoval
	initialized bool
}

func sceneRemovalFor[Tag any](world *ecs.World, prepare func(*ecs.World) ecs.PreparedRemoval) ecs.PreparedRemoval {
	cache := world.FrameValue[Tag, sceneRemovalCache]()
	if !cache.initialized {
		cache.removal = prepare(world)
		cache.initialized = true
	}
	return cache.removal
}

func cloneOptional[T any](value *T) *T {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

// Clone returns an independent authoring copy, including nested animation and
// texture data.
func (components SceneComponents) Clone() SceneComponents {
	clone := SceneComponents{
		Sprite: cloneOptional(components.Sprite), Text: cloneOptional(components.Text), HTML: cloneOptional(components.HTML),
		Particle: cloneOptional(components.Particle), Tilemap: cloneOptional(components.Tilemap),
		Spine: cloneOptional(components.Spine), Model: cloneOptional(components.Model),
		Camera: cloneOptional(components.Camera), AmbientLight: cloneOptional(components.AmbientLight),
		DirectionalLight: cloneOptional(components.DirectionalLight), PointLight: cloneOptional(components.PointLight), SpotLight: cloneOptional(components.SpotLight), AreaLight: cloneOptional(components.AreaLight),
		RigidBody2D: cloneOptional(components.RigidBody2D), CircleCollider2D: cloneOptional(components.CircleCollider2D),
		BoxCollider2D: cloneOptional(components.BoxCollider2D), SegmentCollider2D: cloneOptional(components.SegmentCollider2D),
		RigidBody3D: cloneOptional(components.RigidBody3D), TriangleMeshCollider3D: cloneOptional(components.TriangleMeshCollider3D),
		ConvexHullCollider3D: cloneOptional(components.ConvexHullCollider3D), CompoundCollider3D: cloneOptional(components.CompoundCollider3D),
	}
	if clone.Camera != nil {
		clone.Camera.Fog = cloneOptional(components.Camera.Fog)
		clone.Camera.Environment = cloneOptional(components.Camera.Environment)
		clone.Camera.SSAO = cloneOptional(components.Camera.SSAO)
		clone.Camera.SSR = cloneOptional(components.Camera.SSR)
	}
	if clone.DirectionalLight != nil {
		clone.DirectionalLight.Shadows = cloneOptional(components.DirectionalLight.Shadows)
	}
	if clone.PointLight != nil {
		clone.PointLight.Shadows = cloneOptional(components.PointLight.Shadows)
	}
	if clone.SpotLight != nil {
		clone.SpotLight.Shadows = cloneOptional(components.SpotLight.Shadows)
	}
	if components.Animation != nil {
		value := *components.Animation
		value.Clip = value.Clip.Clone()
		clone.Animation = &value
	}
	if clone.Sprite != nil {
		clone.Sprite.Texture = cloneOptional(components.Sprite.Texture)
		clone.Sprite.Trail = cloneOptional(components.Sprite.Trail)
		if clone.Sprite.Texture != nil {
			clone.Sprite.Texture.Slice = cloneOptional(components.Sprite.Texture.Slice)
		}
	}
	if clone.Text != nil {
		clone.Text.Shadow = cloneOptional(components.Text.Shadow)
	}
	return clone
}
