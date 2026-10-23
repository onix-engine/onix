package editor

import (
	"testing"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func TestPreview3DColorTextureMatrixPreservesRenderTargetOrigin(t *testing.T) {
	texture := &graphics.Texture{U: 0.1, V: 0.2, MaxU: 0.75, MaxV: 0.5}
	for _, test := range []struct {
		name       string
		bottomLeft bool
		topV       float32
		bottomV    float32
	}{
		{name: "top-left", topV: 0.2, bottomV: 0.7},
		{name: "bottom-left", bottomLeft: true, topV: 0.7, bottomV: 0.2},
	} {
		t.Run(test.name, func(t *testing.T) {
			matrix := preview3DColorTextureMatrix(texture, test.bottomLeft)
			top := matrix.Mul3x1(mgl32.Vec3{0, 0, 1})
			bottom := matrix.Mul3x1(mgl32.Vec3{1, 1, 1})
			if !top.ApproxEqualThreshold(mgl32.Vec3{0.1, test.topV, 1}, 0.0001) ||
				!bottom.ApproxEqualThreshold(mgl32.Vec3{0.85, test.bottomV, 1}, 0.0001) {
				t.Fatalf("render target UV mapping = top %v, bottom %v", top, bottom)
			}
		})
	}
}

func TestPreview3DViewportRayUsesNegativeZAtPerspectiveCenter(t *testing.T) {
	ray, ok := preview3DViewportRay(
		rect{X: 10, Y: 20, W: 400, H: 200},
		210, 120,
		scene.PerspectiveProjection3D(),
		mgl32.Translate3D(4, 5, 6),
	)
	if !ok {
		t.Fatal("expected a viewport ray")
	}
	assertPreview3DVec3(t, ray.origin, mgl32.Vec3{4, 5, 6})
	assertPreview3DVec3(t, ray.direction, mgl32.Vec3{0, 0, -1})
}

func TestPreview3DMeshRayRejectsBoundingSphereFalsePositive(t *testing.T) {
	mesh := graphics.CubeMesh(1)
	ray := viewportRay3D{origin: mgl32.Vec3{0.75, 0.75, 2}, direction: mgl32.Vec3{0, 0, -1}}
	if _, ok := preview3DMeshRayDistance(ray, mgl32.Ident4(), mesh); ok {
		t.Fatal("ray outside the cube triangles must not hit only because it crosses the bounding sphere")
	}
}

func TestPreview3DMeshRayDistanceDoesNotAllocate(t *testing.T) {
	mesh := graphics.CubeMesh(1)
	ray := viewportRay3D{origin: mgl32.Vec3{0, 0, 2}, direction: mgl32.Vec3{0, 0, -1}}
	allocations := testing.AllocsPerRun(100, func() {
		if _, ok := preview3DMeshRayDistance(ray, mgl32.Ident4(), mesh); !ok {
			t.Fatal("expected cube ray hit")
		}
	})
	if allocations != 0 {
		t.Fatalf("mesh ray intersection allocations = %v, want 0", allocations)
	}
}

func TestPreview3DAxisColorsUseMutedRGB(t *testing.T) {
	want := map[string]graphics.ColorRGBA{
		"x": rgba(192, 72, 72, 255),
		"y": rgba(72, 168, 88, 255),
		"z": rgba(72, 112, 192, 255),
	}
	for name, color := range map[string]graphics.ColorRGBA{
		"x": editorTheme.axisX,
		"y": editorTheme.axisY,
		"z": editorTheme.axisZ,
	} {
		if color != want[name] {
			t.Fatalf("%s axis color = %v, want muted RGB %v", name, color, want[name])
		}
	}
}

func assertPreview3DVec3(t *testing.T, got, want mgl32.Vec3) {
	t.Helper()
	for axis := range got {
		if abs(got[axis]-want[axis]) > 0.0001 {
			t.Fatalf("axis %d: got %v want %v", axis, got, want)
		}
	}
}
