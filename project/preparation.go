package project

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/scene/render3d"
)

// QueueEntityUploads collects authored resources and prepared text into the
// shared upload queue. It does not acquire handles or invoke a render driver.
func QueueEntityUploads(queue *graphics.UploadQueue, world *ecs.World, entity ecs.Entity) {
	if s := ecs.GetRef[scene.Sprite2D](world, entity); s != nil {
		queue.AddTexture(s.Texture)
	}
	if layers := ecs.GetRef[scene.SpriteLayers2D](world, entity); layers != nil {
		for _, layer := range layers.Layers {
			queue.AddTexture(layer.Sprite.Texture)
		}
	}
	if a := ecs.GetRef[scene.Animator](world, entity); a != nil && a.Clip != nil {
		for _, track := range a.Clip.TextureTracks {
			for _, key := range track.Keyframes {
				queue.AddTexture(key.Value)
			}
		}
	}
	if p := ecs.GetRef[scene.ParticleEmitter2D](world, entity); p != nil {
		queue.AddTexture(p.Texture)
		for _, appearance := range p.Appearances {
			queue.AddTexture(appearance.Texture)
		}
	}
	if tilemap := ecs.GetRef[scene.Tilemap2D](world, entity); tilemap != nil {
		queue.AddTexture(tilemap.Texture)
	}
	if s := ecs.GetRef[scene.Spine2D](world, entity); s != nil && s.Player != nil && s.Player.Resource != nil {
		queue.AddTexture(s.Player.Resource.Texture)
	}
	if text := ecs.GetRef[scene.Text2D](world, entity); text != nil {
		text.QueueUploads(queue)
	}
	if mesh := ecs.GetRef[render3d.Mesh](world, entity); mesh != nil {
		queue.AddMesh(mesh.Mesh)
	}
	scene.QueueMaterial2DUploads(queue, world, entity)
	render3d.QueueMaterialUploads(queue, world, entity)
	if environment := ecs.GetRef[render3d.Environment](world, entity); environment != nil {
		queue.AddTexture(environment.Atlas)
		queue.AddTexture(environment.Panorama)
	}
}
