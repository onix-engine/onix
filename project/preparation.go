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
	Queue2DEntityUploads(queue, world, entity)
	QueueParticlesEntityUploads(queue, world, entity)
	QueueTilemapsEntityUploads(queue, world, entity)
	QueueSpineEntityUploads(queue, world, entity)
	QueueSpatialEntityUploads(queue, world, entity)
}

func QueueParticlesEntityUploads(queue *graphics.UploadQueue, world *ecs.World, entity ecs.Entity) {
	if p := world.GetRef[scene.ParticleEmitter2D](entity); p != nil {
		queue.AddTexture(p.Texture)
		for _, appearance := range p.Appearances {
			queue.AddTexture(appearance.Texture)
		}
	}
}

func QueueTilemapsEntityUploads(queue *graphics.UploadQueue, world *ecs.World, entity ecs.Entity) {
	if tilemap := world.GetRef[scene.Tilemap2D](entity); tilemap != nil {
		queue.AddTexture(tilemap.Texture)
	}
}

func QueueSpineEntityUploads(queue *graphics.UploadQueue, world *ecs.World, entity ecs.Entity) {
	if s := world.GetRef[scene.Spine2D](entity); s != nil && s.Player != nil && s.Player.Resource != nil {
		queue.AddTexture(s.Player.Resource.Texture)
	}
}

func QueueSpatialEntityUploads(queue *graphics.UploadQueue, world *ecs.World, entity ecs.Entity) {
	if mesh := world.GetRef[render3d.Mesh](entity); mesh != nil {
		queue.AddMesh(mesh.Mesh)
	}
	render3d.QueueMaterialUploads(queue, world, entity)
	if environment := world.GetRef[render3d.Environment](entity); environment != nil {
		queue.AddTexture(environment.Atlas)
		queue.AddTexture(environment.Panorama)
	}
}

// Queue2DEntityUploads collects sprites, animation frames, text and 2D materials.
func Queue2DEntityUploads(queue *graphics.UploadQueue, world *ecs.World, entity ecs.Entity) {
	if s := world.GetRef[scene.Sprite2D](entity); s != nil {
		queue.AddTexture(s.Texture)
	}
	if layers := world.GetRef[scene.SpriteLayers2D](entity); layers != nil {
		for _, layer := range layers.Layers {
			queue.AddTexture(layer.Sprite.Texture)
		}
	}
	if a := world.GetRef[scene.Animator](entity); a != nil && a.Clip != nil {
		for _, track := range a.Clip.TextureTracks {
			for _, key := range track.Keyframes {
				queue.AddTexture(key.Value)
			}
		}
	}
	if text := world.GetRef[scene.Text2D](entity); text != nil {
		text.QueueUploads(queue)
	}
	scene.QueueMaterial2DUploads(queue, world, entity)
}
