package render3d

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene"
)

// QueueMaterialUploads resolves registered generic materials and submesh slots
// with the same readers as rendering, without acquiring GPU handles.
func QueueMaterialUploads(queue *graphics.UploadQueue, world *ecs.World, entity ecs.Entity) {
	if state, ok := world.GetResource[*World](); ok && state != nil {
		queue.AddMaterial3D(state.materialFor(world, entity, -1))
		if mesh := world.GetRef[Mesh](entity); mesh != nil && mesh.Mesh != nil {
			for _, part := range mesh.Mesh.Submeshes {
				queue.AddMaterial3D(state.materialFor(world, entity, int(part.Material)))
			}
		}
	} else if material := world.GetRef[Material[graphics.StandardMaterial]](entity); material != nil {
		if material.Material != nil {
			queue.AddMaterial3D(material.Material.Material3D())
		}
		for _, value := range material.Materials {
			queue.AddMaterial3D(value.Material3D())
		}
	}
	if shader := world.GetRef[scene.Shader[graphics.MeshShader]](entity); shader != nil {
		queue.AddMeshShader(shader.Program)
	}
}
