package scene

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/go-gl/mathgl/mgl32"
)

func inputPickCamera(entity ecs.Entity, x, y float32) inputCamera2D {
	camera := NewCamera()
	camera.UpdateViewport(200, 100)
	return inputCamera2D{
		entity: entity, camera: camera, pipeline: CameraPipeline2D, layers: RenderLayer(0),
		projection: Projection{Kind: ProjectionOrthographic, Orthographic: NewOrthographicProjection2D()},
		transform:  GlobalTransform{Matrix: mgl32.Translate3D(x, y, 0)},
	}
}

func inputPickQuad(entity ecs.Entity, x, y float32, ui bool) inputPickTarget2D {
	target := inputPickTarget2D{
		entity: entity, receiver: entity,
		width: 20, height: 20, anchor: AnchorCenter, layers: RenderLayer(0), ui: ui,
	}
	target.prepare(mgl32.Translate3D(x, y, 0))
	return target
}

func TestInputPickingKeepsCameraAndUICoordinatesSeparate(t *testing.T) {
	primary := inputPickCamera(1, 1000, 2000)
	secondary := inputPickCamera(2, -1000, -2000)
	worldHit := inputPickQuad(3, 1000, 2000, false)
	worldMiss := inputPickQuad(4, 3000, 4000, false)
	uiHit := inputPickQuad(5, 100, 50, true)
	uiMiss := inputPickQuad(6, 500, 500, true)
	invalidWorld := primary
	invalidWorld.projection.Kind = ProjectionPerspective
	for _, test := range []struct {
		name    string
		cameras []inputCamera2D
		targets []inputPickTarget2D
		x, y    float32
		entity  ecs.Entity
		wantX   float32
		wantY   float32
	}{
		{"world after UI miss", []inputCamera2D{primary}, []inputPickTarget2D{worldHit, uiMiss}, 100, 50, 3, 1000, 2000},
		{"UI after world miss", []inputCamera2D{primary}, []inputPickTarget2D{uiHit, worldMiss}, 100, 50, 5, 100, 50},
		{"camera fallback", []inputCamera2D{primary, secondary}, []inputPickTarget2D{worldHit, worldMiss}, 100, 50, 3, 1000, 2000},
		{"UI despite invalid world projection", []inputCamera2D{invalidWorld}, []inputPickTarget2D{uiHit, worldMiss}, 100, 50, 5, 100, 50},
		{"outside viewport", []inputCamera2D{primary}, []inputPickTarget2D{uiHit, worldHit}, 300, 50, 0, 0, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			target, x, y, picked := pickInputTarget2D(test.cameras, 1, test.targets, test.x, test.y)
			if picked != (test.entity != 0) || target.entity != test.entity || x != test.wantX || y != test.wantY {
				t.Fatalf("pick = (%d, %v, %v, %v), want (%d, %v, %v)", target.entity, x, y, picked, test.entity, test.wantX, test.wantY)
			}
		})
	}
}

func BenchmarkInputPickWorldMiss1000(b *testing.B) {
	cameras := []inputCamera2D{inputPickCamera(1, 1000, 2000)}
	targets := make([]inputPickTarget2D, 1000)
	for i := range targets {
		targets[i] = inputPickQuad(ecs.Entity(i+2), 3000+float32(i)*30, 4000, false)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, _, _, picked := pickInputTarget2D(cameras, 1, targets, 100, 50); picked {
			b.Fatal("unexpected hit")
		}
	}
}
