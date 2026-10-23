package scene

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

func TestOrthographic3DReverseZDepthRange(t *testing.T) {
	p := NewOrthographicProjection2D()
	p.ReverseZ = true
	p.Near, p.Far = 0.1, 100
	p.ScalingMode = FixedVerticalScaling(10)
	p.Update(800, 400)
	m := p.Matrix()
	for _, sample := range []struct{ z, want float32 }{{-p.Near, 1}, {-p.Far, 0}, {-(p.Near + p.Far) / 2, 0.5}} {
		if depth := m.Mul4x1(mgl32.Vec4{0, 0, sample.z, 1}).Z(); absFloat32(depth-sample.want) > 0.00001 {
			t.Fatalf("depth at %g = %g, want %g", sample.z, depth, sample.want)
		}
	}
	if corner := m.Mul4x1(mgl32.Vec4{10, 5, -1, 1}); !near32(corner.X(), 1) || !near32(corner.Y(), 1) {
		t.Fatalf("orthographic area = %v", corner)
	}
}

func TestOrthographicProjectionScalingModesMatchBevy(t *testing.T) {
	tests := []struct {
		name       string
		mode       ScalingMode
		wantWidth  float32
		wantHeight float32
	}{
		{name: "window size", mode: ScalingMode{}, wantWidth: 1200, wantHeight: 600},
		{name: "fixed", mode: FixedScaling(320, 180), wantWidth: 320, wantHeight: 180},
		{name: "auto min", mode: AutoMinScaling(400, 400), wantWidth: 800, wantHeight: 400},
		{name: "auto max", mode: AutoMaxScaling(400, 400), wantWidth: 400, wantHeight: 200},
		{name: "fixed vertical", mode: FixedVerticalScaling(300), wantWidth: 600, wantHeight: 300},
		{name: "fixed horizontal", mode: FixedHorizontalScaling(500), wantWidth: 500, wantHeight: 250},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projection := NewOrthographicProjection2D()
			projection.ScalingMode = test.mode
			projection.Scale = 1.5
			projection.Update(1200, 600)

			if got := projection.Area.MaxX - projection.Area.MinX; !near32(got, test.wantWidth*1.5) {
				t.Fatalf("projection width = %v, want %v", got, test.wantWidth*1.5)
			}
			if got := projection.Area.MaxY - projection.Area.MinY; !near32(got, test.wantHeight*1.5) {
				t.Fatalf("projection height = %v, want %v", got, test.wantHeight*1.5)
			}
			if !near32(projection.Area.MinX, -test.wantWidth*0.75) || !near32(projection.Area.MinY, -test.wantHeight*0.75) {
				t.Fatalf("projection is not centered: %+v", projection.Area)
			}
		})
	}
}

func TestOrthographicProjectionViewportOriginMatchesBevy(t *testing.T) {
	projection := NewOrthographicProjection2D()
	projection.ViewportOrigin = mgl32.Vec2{0, 0}
	projection.Update(320, 180)

	want := OrthographicArea{MinX: 0, MinY: 0, MaxX: 320, MaxY: 180}
	if projection.Area != want {
		t.Fatalf("projection area = %+v, want %+v", projection.Area, want)
	}
}

func TestCamera2DViewportWorldRoundTrip(t *testing.T) {
	camera := NewCamera()
	transform := TransformFromXYZ(120, -35, 0)
	transform.Rotation = mgl32.QuatRotate(float32(math.Pi/6), mgl32.Vec3{0, 0, 1})
	globalTransform := GlobalTransform{Matrix: transform.LocalMatrix()}
	projection := OrthographicProjection2D()
	projection.Orthographic.Scale = 1.75
	camera.UpdateViewport(1280, 720)

	viewport := mgl32.Vec2{237.5, 611.25}
	world, ok := camera.ViewportToWorld2D(globalTransform, projection, viewport)
	if !ok {
		t.Fatal("viewport to world failed")
	}
	roundTrip, ok := camera.WorldToViewport2D(globalTransform, projection, world)
	if !ok {
		t.Fatal("world to viewport failed")
	}
	if !near32(roundTrip.X(), viewport.X()) || !near32(roundTrip.Y(), viewport.Y()) {
		t.Fatalf("round trip = %v, want %v", roundTrip, viewport)
	}

	center, ok := camera.ViewportToWorld2D(globalTransform, projection, mgl32.Vec2{640, 360})
	if !ok || !near32(center.X(), transform.Translation.X()) || !near32(center.Y(), transform.Translation.Y()) {
		t.Fatalf("viewport center = %v, want camera translation (%v, %v)", center, transform.Translation.X(), transform.Translation.Y())
	}
}

func TestCamera2DViewportWorldRoundTripWithViewportOffset(t *testing.T) {
	camera := NewCamera()
	camera.UpdateViewport(800, 600)
	camera.Viewport = CameraViewport{X: 100, Y: 50, Width: 400, Height: 300}
	projection := OrthographicProjection2D()
	transform := GlobalTransform{Matrix: mgl32.Translate3D(25, 40, 0)}
	viewport := mgl32.Vec2{325, 175}
	world, ok := camera.ViewportToWorld2D(transform, projection, viewport)
	if !ok {
		t.Fatal("viewport to world failed")
	}
	roundTrip, ok := camera.WorldToViewport2D(transform, projection, world)
	if !ok || !near32(roundTrip.X(), viewport.X()) || !near32(roundTrip.Y(), viewport.Y()) {
		t.Fatalf("offset viewport round trip = %v, want %v", roundTrip, viewport)
	}
}

func near32(a, b float32) bool {
	return float32(math.Abs(float64(a-b))) <= 1e-3
}
