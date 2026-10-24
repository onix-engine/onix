package render3d

import (
	"math"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

type extractedLight3D struct {
	render.Light3D
	entity  ecs.Entity
	scene   uint64
	layers  scene.RenderLayers
	shadows extractedShadowLight3D
}

// Select a fixed-size ordered set in one scan. Directional lights precede
// local lights; locals rank by estimated influence at the camera. Range
// spheres conservatively cull off-screen lights, including box/area extents.
func selectViewLights3D(view *renderView3D, candidates []extractedLight3D, layers scene.RenderLayers, limit int) [render.MaxMeshLights]extractedShadowLight3D {
	limit = min(max(limit, 1), render.MaxMeshLights)
	view.frustum = newFrustum3D(view.frame.Camera.screenProjection)
	var shadows [render.MaxMeshLights]extractedShadowLight3D
	var selected [render.MaxMeshLights]int
	var scores [render.MaxMeshLights]float64
	var priorities [render.MaxMeshLights]uint8
	count := 0
	sceneAmbient := false
	for index, candidate := range candidates {
		if candidate.scene != 0 && view.scene != 0 && candidate.scene != view.scene {
			continue
		}
		if candidate.Kind != render.Light3DAmbient && !layers.Intersects(candidate.layers) {
			continue
		}
		light := candidate.Light3D
		if light.Kind == render.Light3DAmbient {
			if candidate.scene == 0 && sceneAmbient {
				continue
			}
			if candidate.scene != 0 && candidate.scene == view.scene {
				if sceneAmbient {
					continue
				}
				sceneAmbient = true
				for i := 0; i < count; i++ {
					if candidates[selected[i]].Kind == render.Light3DAmbient && candidates[selected[i]].scene == 0 {
						copy(selected[i:], selected[i+1:count])
						copy(scores[i:], scores[i+1:count])
						copy(priorities[i:], priorities[i+1:count])
						count--
						break
					}
				}
			}
		}
		score := float64(max(light.Color[0], light.Color[1], light.Color[2]))
		priority := uint8(0)
		if light.Kind == render.Light3DAmbient {
			priority = 2
			if candidate.scene != 0 && candidate.scene == view.scene {
				priority = 3
			}
		}
		if light.Kind >= render.Light3DPoint {
			center := mgl32.Vec3(light.PositionDirection[:3])
			radius := light.Params[0]
			switch light.Kind {
			case render.Light3DBox:
				center = center.Add(mgl32.Vec3(light.Direction[:3]).Mul(radius * .5))
				radius = float32(math.Sqrt(float64(radius*radius*.25 + light.Right[3]*light.Right[3] + light.Up[3]*light.Up[3])))
			case render.Light3DRectangle:
				radius += .5 * float32(math.Hypot(float64(light.Right[3]), float64(light.Up[3])))
			case render.Light3DDisc:
				radius += light.Right[3]
			case render.Light3DTube:
				radius += light.Right[3] * .5
			}
			if !view.frustum.containsSphere(center, radius) {
				continue
			}
			distance := center.Sub(view.frame.Camera.position).LenSqr()
			brightness := max(light.Color[0], light.Color[1], light.Color[2])
			intensity := float64(light.Params[1])
			switch light.Kind {
			case render.Light3DRectangle:
				intensity *= float64(light.Right[3]) * float64(light.Up[3])
			case render.Light3DDisc:
				intensity *= math.Pi * float64(light.Right[3]) * float64(light.Right[3])
			case render.Light3DTube:
				intensity *= float64(light.Right[3])
			}
			score = float64(brightness) * intensity / float64(max(distance, 1))
			if score <= 0 {
				continue
			}
		} else if light.Kind == render.Light3DDirectional {
			if score <= 0 {
				continue
			}
			priority = 1
		}
		position := count
		for position > 0 {
			prior := selected[position-1]
			if priority < priorities[position-1] || priority == priorities[position-1] && (score < scores[position-1] || score == scores[position-1] && candidate.entity >= candidates[prior].entity) {
				break
			}
			position--
		}
		if position >= limit {
			continue
		}
		end := min(count, limit-1)
		for i := end; i > position; i-- {
			selected[i], scores[i] = selected[i-1], scores[i-1]
			priorities[i] = priorities[i-1]
		}
		selected[position], scores[position] = index, score
		priorities[position] = priority
		count = min(count+1, limit)
	}
	for i, index := range selected[:count] {
		view.selectedLights[i] = candidates[index].Light3D
		shadows[i] = candidates[index].shadows
	}
	view.lights = view.selectedLights[:count]
	return shadows
}
