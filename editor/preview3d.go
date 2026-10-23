package editor

import (
	"math"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

type previewDimension string

const (
	previewDimension2D previewDimension = "2d"
	previewDimension3D previewDimension = "3d"
)

func preview3DCameraTransformAround(target mgl32.Vec3, yaw, pitch, distance float32) scene.Transform {
	distance = max(distance, 0.001)
	cosPitch := float32(math.Cos(float64(pitch)))
	position := target.Add(mgl32.Vec3{
		float32(math.Sin(float64(yaw))) * cosPitch * distance,
		float32(math.Sin(float64(pitch))) * distance,
		float32(math.Cos(float64(yaw))) * cosPitch * distance,
	})
	// Build the camera basis from the orbit angles instead of using a fixed
	// world-up LookAt. A fixed up vector becomes parallel to the view direction
	// at the poles, while this basis remains valid as pitch crosses them.
	orbitRotation := mgl32.HomogRotate3DY(yaw).Mul4(mgl32.HomogRotate3DX(-pitch))
	transform := scene.TransformFromTranslation(position)
	transform.Rotation = mgl32.Mat4ToQuat(orbitRotation).Normalize()
	return transform
}

func preview3DPanTarget(target mgl32.Vec3, cameraTransform mgl32.Mat4, distance, fov, viewportHeight, deltaX, deltaY float32) mgl32.Vec3 {
	if deltaX == 0 && deltaY == 0 {
		return target
	}
	distance = max(distance, 0.001)
	viewportHeight = max(viewportHeight, 1)
	if fov <= 0 {
		fov = float32(math.Pi / 4)
	}
	right := cameraTransform.Col(0).Vec3()
	up := cameraTransform.Col(1).Vec3()
	if right.Len() <= 0.000001 || up.Len() <= 0.000001 {
		return target
	}
	worldPerPixel := 2 * distance * float32(math.Tan(float64(fov)*0.5)) / viewportHeight
	return target.Sub(right.Normalize().Mul(deltaX * worldPerPixel)).Add(up.Normalize().Mul(deltaY * worldPerPixel))
}

// Map the attachment's texture origin into the editor's top-left canvas space.
func drawPreview3DColorTexture(c *graphics.Canvas2D, texture *graphics.Texture, bounds rect, radius float32) {
	if c == nil || texture == nil || !texture.Valid() {
		return
	}
	c.BindTexture(texture)
	model := mgl32.Translate3D(bounds.X, bounds.Y, 0).Mul4(mgl32.Scale3D(bounds.W, bounds.H, 1))
	custom := [4][4]float32{}
	if radius > 0 {
		custom[2] = [4]float32{bounds.X, bounds.Y, bounds.X + bounds.W, bounds.Y + bounds.H}
		custom[3] = [4]float32{min(radius, min(bounds.W, bounds.H)/2), 1}
	}
	c.DrawTintedMatricesCustom(model, preview3DColorTextureMatrix(texture, render.CurrentRendererInfo().RenderTargetBottomLeft), graphics.White, 1, custom)
}

// Only GL attachments need V reversed. Vulkan, Metal and WebGPU attachments
// already start at the top; flipping them separates the scene from its overlays.
func preview3DColorTextureMatrix(texture *graphics.Texture, bottomLeft bool) mgl32.Mat3 {
	v, height := texture.V, texture.MaxV
	if bottomLeft {
		v += height
		height = -height
	}
	return mgl32.Translate2D(texture.U, v).Mul3(mgl32.Scale2D(texture.MaxU, height))
}
