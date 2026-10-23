package scene

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/go-gl/mathgl/mgl32"
)

type spineRenderBatch2DScratch struct{}

func drawSpineRenderBatch2D(c *graphics.Canvas2D, world *ecs.World, items []renderItem2D, viewport renderViewport2D) {
	batch := ecs.FrameValue[spineRenderBatch2DScratch, render.MeshBatch3D](world)
	*batch = render.MeshBatch3D{
		Vertices: batch.Vertices[:0], Indices: batch.Indices[:0],
		Instances: batch.Instances[:0], Draws: batch.Draws[:0],
	}
	projection, view := c.ProjectionMatrix(), c.ViewMatrix()
	for index := range items {
		item := &items[index]
		model := SpineModelMatrix2D(item.transform, item.ui)
		minX, minY, maxX, maxY, valid := item.spine.Player.MeshBatchInto(&item.spine.batch, projection, view.Mul4(model))
		if !spineRenderItemVisible2D(viewport, model, item.clip, minX, minY, maxX, maxY, valid) {
			continue
		}
		if len(items) == 1 {
			_ = c.DrawMeshes(item.spine.batch)
			return
		}
		appendSpineMeshBatch(batch, item.spine.batch)
	}
	if len(batch.Draws) != 0 {
		_ = c.DrawMeshes(*batch)
	}
}

// Player.MeshBatchInto produces dynamic geometry and one instance. Baking
// affine positions lets adjacent players share that instance and index draw;
// Normal holds packed Spine tint data and must remain unchanged.
func appendSpineMeshBatch(dst *render.MeshBatch3D, src render.MeshBatch3D) {
	if len(src.Draws) == 0 || len(src.Instances) == 0 {
		return
	}
	if len(dst.Draws) == 0 {
		dst.ViewProjection = src.ViewProjection
		dst.CameraPosition = src.CameraPosition
		dst.AmbientLight = src.AmbientLight
		dst.LightDirection = src.LightDirection
		dst.DirectionalLight = src.DirectionalLight
		dst.Lights = src.Lights
		instance := src.Instances[0]
		instance.Model = [16]float32(mgl32.Ident4())
		dst.Instances = append(dst.Instances, instance)
	}
	model := mgl32.Mat4(src.Instances[0].Model)
	vertexBase := uint32(len(dst.Vertices))
	indexBase := uint32(len(dst.Indices))
	for _, vertex := range src.Vertices {
		position := model.Mul4x1(mgl32.Vec4{vertex.Position[0], vertex.Position[1], vertex.Position[2], 1})
		vertex.Position = [3]float32{position[0], position[1], position[2]}
		dst.Vertices = append(dst.Vertices, vertex)
	}
	for _, index := range src.Indices {
		dst.Indices = append(dst.Indices, vertexBase+index)
	}
	for _, draw := range src.Draws {
		draw.FirstIndex += indexBase
		draw.FirstInstance = 0
		if len(dst.Draws) != 0 {
			previous := &dst.Draws[len(dst.Draws)-1]
			if previous.Mesh == draw.Mesh && previous.Shader == draw.Shader && previous.Texture == draw.Texture &&
				previous.NormalTexture == draw.NormalTexture && previous.BaseVertex == draw.BaseVertex &&
				previous.AlphaMode == draw.AlphaMode && previous.BlendMode == draw.BlendMode &&
				previous.DeformationIndex == draw.DeformationIndex && previous.FirstIndex+previous.IndexCount == draw.FirstIndex {
				previous.IndexCount += draw.IndexCount
				continue
			}
		}
		dst.Draws = append(dst.Draws, draw)
	}
}
