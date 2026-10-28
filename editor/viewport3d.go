package editor

import (
	"errors"
	"math"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

// editorViewport3D owns state whose lifetime belongs to the scene viewport,
// independently from the editor application and project document.
type editorViewport3D struct {
	camera ecs.Entity
	editorRenderTarget
	yaw           float32
	pitch         float32
	distance      float32
	target        mgl32.Vec3
	gridTexture   *graphics.Texture
	axisTexture   *graphics.Texture
	studioTexture *graphics.Texture
	gridVertices  [4]render.Vertex3D
	gridIndices   [6]uint32
	gridInstances [1]render.Instance3D
	gridDraws     [1]render.MeshDraw3D
	axisVertices  [24 * preview3DAxisFadeSegments]render.Vertex3D
	axisIndices   [36 * preview3DAxisFadeSegments]uint32
	axisInstances [3]render.Instance3D
	axisDraws     [3]render.MeshDraw3D
	labels        map[string]*scene.Text2D
	sky           graphics.SkyRadiance
	orthographic  bool
	orientation   preview3DOrientationState
	settingsOpen  bool
	settingsPage  int
}

type editorRenderTarget struct {
	renderTarget *render.RenderTarget
	colorTexture *graphics.Texture
}

func preview3DTargetSize(width, height, scale int) (int, int) {
	width, height = max(width, 1), max(height, 1)
	scale = max(scale, 1)
	return width * scale, height * scale
}

func (viewport *editorRenderTarget) ensureTarget(device *render.Device, width, height, scale int) (bool, error) {
	driver := device.Driver()
	if driver == nil {
		return false, nil
	}
	if _, ok := driver.(render.RenderTargetDriver); !ok {
		return false, errors.New("editor: active renderer does not support viewport targets")
	}
	width, height = preview3DTargetSize(width, height, scale)
	if viewport.renderTarget == nil {
		target, err := render.NewRenderTarget(render.RenderTargetDescriptor{Width: width, Height: height, Depth: true})
		if err != nil {
			return false, err
		}
		viewport.renderTarget = target
	} else {
		viewport.renderTarget.Resize(width, height)
	}
	if viewport.colorTexture == nil {
		color, err := graphics.CreateTextureFromRenderTarget(viewport.renderTarget)
		if err != nil {
			return false, err
		}
		viewport.colorTexture = color
	}
	return true, nil
}

func (viewport *editorRenderTarget) disposeTarget() error {
	var disposeErr error
	if viewport.colorTexture != nil {
		disposeErr = viewport.colorTexture.Dispose()
		viewport.colorTexture = nil
	}
	if viewport.renderTarget != nil {
		if err := viewport.renderTarget.Dispose(); disposeErr == nil {
			disposeErr = err
		}
		viewport.renderTarget = nil
	}
	return disposeErr
}

type viewportRay3D struct {
	origin    mgl32.Vec3
	direction mgl32.Vec3
}

func preview3DViewportRay(
	viewport rect,
	screenX, screenY float32,
	projection scene.Projection,
	cameraFromWorld mgl32.Mat4,
) (viewportRay3D, bool) {
	if viewport.W <= 0 || viewport.H <= 0 || screenX < viewport.X || screenY < viewport.Y || screenX > viewport.X+viewport.W || screenY > viewport.Y+viewport.H {
		return viewportRay3D{}, false
	}
	return preview3DScreenRay(viewport, screenX, screenY, projection, cameraFromWorld)
}

// Pointer capture can continue outside the viewport while dragging.
func preview3DScreenRay(viewport rect, screenX, screenY float32, projection scene.Projection, cameraFromWorld mgl32.Mat4) (viewportRay3D, bool) {
	if viewport.W <= 0 || viewport.H <= 0 {
		return viewportRay3D{}, false
	}
	normalizedX := (screenX-viewport.X)/viewport.W*2 - 1
	normalizedY := 1 - (screenY-viewport.Y)/viewport.H*2
	forward := cameraFromWorld.Col(2).Vec3().Mul(-1)
	if forward.Len() <= 0.000001 {
		return viewportRay3D{}, false
	}

	if projection.Kind == scene.ProjectionOrthographic {
		orthographic := projection.Orthographic
		orthographic.Update(viewport.W, viewport.H)
		area := orthographic.Area
		viewPosition := mgl32.Vec3{
			area.MinX + (normalizedX+1)*0.5*(area.MaxX-area.MinX),
			area.MinY + (normalizedY+1)*0.5*(area.MaxY-area.MinY),
			0,
		}
		origin := cameraFromWorld.Mul4x1(viewPosition.Vec4(1)).Vec3()
		return viewportRay3D{origin: origin, direction: forward.Normalize()}, true
	}

	fov := projection.Perspective.Fov
	if fov <= 0 {
		fov = float32(math.Pi / 4)
	}
	tangent := float32(math.Tan(float64(fov) * 0.5))
	viewDirection := mgl32.Vec3{normalizedX * viewport.W / viewport.H * tangent, normalizedY * tangent, -1}.Normalize()
	direction := cameraFromWorld.Mul4x1(viewDirection.Vec4(0)).Vec3()
	if direction.Len() <= 0.000001 {
		return viewportRay3D{}, false
	}
	return viewportRay3D{origin: cameraFromWorld.Col(3).Vec3(), direction: direction.Normalize()}, true
}

func preview3DMeshRayDistance(ray viewportRay3D, transform mgl32.Mat4, mesh *graphics.Mesh) (float32, bool) {
	if mesh == nil || len(mesh.Vertices) == 0 || len(mesh.Indices) < 3 {
		return 0, false
	}
	determinant := transform.Det()
	if math.IsNaN(float64(determinant)) || math.IsInf(float64(determinant), 0) || math.Abs(float64(determinant)) <= 0.000000001 {
		return 0, false
	}
	localFromWorld := transform.Inv()
	localRay := viewportRay3D{
		origin:    localFromWorld.Mul4x1(ray.origin.Vec4(1)).Vec3(),
		direction: localFromWorld.Mul4x1(ray.direction.Vec4(0)).Vec3(),
	}
	if !preview3DRayHitsSphere(localRay, mesh.Bounds) {
		return 0, false
	}

	best := float32(math.MaxFloat32)
	for triangle := 0; triangle+2 < len(mesh.Indices); triangle += 3 {
		first := mesh.Indices[triangle]
		second := mesh.Indices[triangle+1]
		third := mesh.Indices[triangle+2]
		if uint64(first) >= uint64(len(mesh.Vertices)) || uint64(second) >= uint64(len(mesh.Vertices)) || uint64(third) >= uint64(len(mesh.Vertices)) {
			continue
		}
		distance, ok := preview3DRayTriangleDistance(
			localRay,
			mgl32.Vec3(mesh.Vertices[first].Position),
			mgl32.Vec3(mesh.Vertices[second].Position),
			mgl32.Vec3(mesh.Vertices[third].Position),
		)
		if ok && distance < best {
			best = distance
		}
	}
	return best, best != float32(math.MaxFloat32)
}

func preview3DRayHitsSphere(ray viewportRay3D, bounds graphics.BoundingSphere) bool {
	offset := ray.origin.Sub(bounds.Center)
	a := ray.direction.Dot(ray.direction)
	if a <= 0.000000001 {
		return false
	}
	b := offset.Dot(ray.direction)
	c := offset.Dot(offset) - bounds.Radius*bounds.Radius
	return c <= 0 || b <= 0 && b*b-a*c >= 0
}

func preview3DRayTriangleDistance(ray viewportRay3D, a, b, c mgl32.Vec3) (float32, bool) {
	const epsilon = float32(0.000001)
	edgeAB := b.Sub(a)
	edgeAC := c.Sub(a)
	p := ray.direction.Cross(edgeAC)
	determinant := edgeAB.Dot(p)
	if abs(determinant) <= epsilon {
		return 0, false
	}
	inverse := 1 / determinant
	fromA := ray.origin.Sub(a)
	u := fromA.Dot(p) * inverse
	if u < 0 || u > 1 {
		return 0, false
	}
	q := fromA.Cross(edgeAB)
	v := ray.direction.Dot(q) * inverse
	if v < 0 || u+v > 1 {
		return 0, false
	}
	distance := edgeAC.Dot(q) * inverse
	return distance, distance > epsilon
}
