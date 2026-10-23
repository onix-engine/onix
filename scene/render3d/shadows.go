package render3d

import (
	"fmt"
	"math"

	"github.com/diiyw/onix/render"
	"github.com/go-gl/mathgl/mgl32"
)

type extractedShadowLight3D struct {
	directional ShadowSettings
	punctual    PunctualShadowSettings
}

type shadowAtlas3D struct {
	target *render.RenderTarget
	views  [render.MaxShadowMaps]*renderView3D
	count  int
}

func (s PunctualShadowSettings) Validate(lightRange float32) error {
	if !s.Enabled {
		return nil
	}
	if s.Resolution < 64 || s.Resolution > 8192 || s.Resolution&(s.Resolution-1) != 0 {
		return fmt.Errorf("shadow resolution must be a power of two between 64 and 8192")
	}
	if !validLightNumbers(s.Near, s.Bias, s.NormalBias) || s.Near <= 0 || s.Near >= lightRange {
		return fmt.Errorf("local shadows require 0 < near < light range and finite non-negative bias")
	}
	return nil
}

func shadowMapCount3D(light render.Light3D, settings extractedShadowLight3D) int {
	if light.Kind == render.Light3DDirectional && settings.directional.Enabled {
		return max(1, settings.directional.Cascades)
	}
	if settings.punctual.Enabled {
		switch light.Kind {
		case render.Light3DPoint:
			return 6
		case render.Light3DSpot, render.Light3DPyramid, render.Light3DBox:
			return 1
		}
	}
	return 0
}

func setupShadowAtlas3D(view *renderView3D, settings [render.MaxMeshLights]extractedShadowLight3D, disabled bool, resolutionOverride int) error {
	view.shadowActive = false
	clear(view.lighting.LightShadows[:])
	clear(view.lighting.Shadows[:])
	count := 0
	budget := view.budget.shadowMaps
	if budget == 0 {
		budget = render.MaxShadowMaps
	}
	var sizes [render.MaxShadowMaps]int
	if !disabled {
		for index, light := range view.lights {
			if shadowMapCount3D(light, settings[index]) == 0 {
				continue
			}
			s := settings[index]
			requested := s.punctual.Resolution
			blend := float32(0)
			if light.Kind == render.Light3DDirectional {
				if err := s.directional.Validate(); err != nil {
					return err
				}
				requested, blend = s.directional.Resolution, s.directional.CascadeBlend
			} else if err := s.punctual.Validate(light.Params[0]); err != nil {
				return err
			}
			if view.budget.shadowResolution > 0 {
				requested = min(requested, view.budget.shadowResolution)
				s.directional.Resolution = min(s.directional.Resolution, view.budget.shadowResolution)
				s.punctual.Resolution = min(s.punctual.Resolution, view.budget.shadowResolution)
			}
			settings[index] = s
			maps := shadowMapCount3D(light, s)
			if light.Kind == render.Light3DDirectional && view.budget.cascades > 0 {
				maps = min(maps, view.budget.cascades)
			}
			if count+maps > budget {
				continue
			}
			for tile := count; tile < count+maps; tile++ {
				sizes[tile] = requested
			}
			view.lighting.LightShadows[index] = [4]float32{float32(count + 1), float32(maps), blend}
			count += maps
		}
	}
	if count == 0 {
		if view.shadow != nil {
			if err := view.shadow.target.Dispose(); err != nil {
				return err
			}
			view.shadow = nil
		}
		return nil
	}
	if resolutionOverride > 0 {
		if resolutionOverride < 64 || resolutionOverride > 8192 || resolutionOverride&(resolutionOverride-1) != 0 {
			return fmt.Errorf("ecs: invalid shadow resolution override")
		}
		for index := range sizes[:count] {
			sizes[index] = resolutionOverride
			if view.budget.shadowResolution > 0 {
				sizes[index] = min(sizes[index], view.budget.shadowResolution)
			}
		}
	}
	tiles, width, height := packShadowAtlas3D(sizes, count)
	if view.shadow == nil {
		target, err := render.NewRenderTarget(render.RenderTargetDescriptor{Width: width, Height: height, Depth: true})
		if err != nil {
			return err
		}
		view.shadow = &shadowAtlas3D{target: target}
	}
	atlas := view.shadow
	atlas.count = count
	atlas.target.Resize(width, height)
	for index, light := range view.lights {
		mapping := view.lighting.LightShadows[index]
		if mapping[0] == 0 {
			continue
		}
		first, maps := int(mapping[0])-1, int(mapping[1])
		var matrices [6]mgl32.Mat4
		var ranges [4][2]float32
		s := settings[index]
		bias, normalBias := s.punctual.Bias, s.punctual.NormalBias
		if light.Kind == render.Light3DDirectional {
			direction := mgl32.Vec3(light.Direction[:3])
			d := s.directional
			d.Resolution = tiles[first].size
			bias, normalBias = d.Bias, d.NormalBias
			if maps == 1 && d.Cascades <= 1 {
				matrices[0] = directionalShadowMatrix(view.frame.Camera.position, direction, d)
			} else {
				var err error
				d.Cascades = maps
				camera := view.frame.Camera
				camera.viewProjection = camera.screenProjection
				matrices, ranges, view.lighting.CameraForward, err = cascadeShadowMatrices3D(camera, direction, d)
				if err != nil {
					return err
				}
			}
		} else {
			matrices = punctualShadowMatrices3D(light, s.punctual)
		}
		for face := 0; face < maps; face++ {
			mapIndex := first + face
			tile := tiles[mapIndex]
			x, y, resolution := tile.x, tile.y, tile.size
			viewport := render.Rect{X: int32(x), Y: int32(y), Width: int32(resolution), Height: int32(resolution)}
			shadow := atlas.views[mapIndex]
			if shadow == nil {
				shadow = &renderView3D{dynamicGeometry: make(map[meshGeometryKey3D]uint32), deformationIndices: make(map[meshGeometryKey3D]uint32)}
				atlas.views[mapIndex] = shadow
			}
			shadow.frame = render.NewRenderView(extractedView3D{viewProjection: matrices[face]})
			shadow.frame.Target, shadow.frame.Viewport = atlas.target, viewport
			shadow.frame.Mode = render.RenderViewLit
			shadow.frame.Clear = render.RenderTargetClear{Color: render.Color{A: 1}, ClearColor: mapIndex == 0, ClearDepth: mapIndex == 0}
			shadow.extracted = shadow.extracted[:0]
			shadow.lighting = render.MeshLighting{Params: [4]float32{1, 1}}
			params := [4]float32{bias, normalBias}
			if maps > 1 && light.Kind == render.Light3DDirectional {
				params[2], params[3] = ranges[face][0], ranges[face][1]
			}
			view.lighting.Shadows[mapIndex] = render.ShadowMap3D{
				Matrix: [16]float32(matrices[face]),
				Rect:   [4]float32{float32(x) / float32(width), float32(y) / float32(height), float32(resolution) / float32(width), float32(resolution) / float32(height)},
				Params: params,
			}
		}
	}
	view.shadowActive = true
	return nil
}

type shadowTile3D struct{ x, y, size int }

// Pack the fixed map budget by descending tile size. Trying power-of-two row
// widths chooses the smallest attachment area while honoring per-light sizes.
// All scratch storage lives on the stack. Oversized requests are reduced
// together until both target dimensions fit the 8192-pixel atlas cap.
func packShadowAtlas3D(sizes [render.MaxShadowMaps]int, count int) ([render.MaxShadowMaps]shadowTile3D, int, int) {
	var order [render.MaxShadowMaps]int
	for index := 0; index < count; index++ {
		position := index
		for position > 0 && sizes[order[position-1]] < sizes[index] {
			order[position] = order[position-1]
			position--
		}
		order[position] = index
	}
	for {
		var best [render.MaxShadowMaps]shadowTile3D
		bestWidth, bestHeight, bestArea := 0, 0, int(^uint(0)>>1)
		for width := sizes[order[0]]; width <= 8192; width *= 2 {
			var tiles [render.MaxShadowMaps]shadowTile3D
			x, y, rowHeight := 0, 0, 0
			for _, index := range order[:count] {
				size := sizes[index]
				if x+size > width {
					x, y, rowHeight = 0, y+rowHeight, 0
				}
				tiles[index] = shadowTile3D{x: x, y: y, size: size}
				x, rowHeight = x+size, max(rowHeight, size)
			}
			height := y + rowHeight
			area := width * height
			if height <= 8192 && (area < bestArea || area == bestArea && max(width, height) < max(bestWidth, bestHeight)) {
				best, bestWidth, bestHeight, bestArea = tiles, width, height, area
			}
		}
		if bestWidth > 0 {
			return best, bestWidth, bestHeight
		}
		for index := range sizes[:count] {
			sizes[index] = max(64, sizes[index]/2)
		}
	}
}

func cascadeSplit3D(near, far float32, index, count int, lambda float32) float32 {
	t := float32(index) / float32(count)
	linear := near + (far-near)*t
	logarithmic := near * float32(math.Pow(float64(far/near), float64(t)))
	return linear*(1-lambda) + logarithmic*lambda
}

func cascadeShadowMatrices3D(camera extractedView3D, direction mgl32.Vec3, s ShadowSettings) ([6]mgl32.Mat4, [4][2]float32, [4]float32, error) {
	var matrices [6]mgl32.Mat4
	var ranges [4][2]float32
	if determinant := camera.viewProjection.Det(); math.IsNaN(float64(determinant)) || math.IsInf(float64(determinant), 0) || determinant == 0 {
		return matrices, ranges, [4]float32{}, fmt.Errorf("ecs: cascaded shadows require an invertible camera projection")
	}
	inverse := camera.viewProjection.Inv()
	unproject := func(x, y, z float32) mgl32.Vec3 {
		p := inverse.Mul4x1(mgl32.Vec4{x, y, z, 1})
		return p.Vec3().Mul(1 / p.W())
	}
	nearCenter, farCenter := unproject(0, 0, 1), unproject(0, 0, .5)
	forward := farCenter.Sub(nearCenter).Normalize()
	near := max(.001, nearCenter.Sub(camera.position).Dot(forward))
	if s.Distance <= near {
		return matrices, ranges, [4]float32{}, fmt.Errorf("ecs: shadow distance must exceed camera near distance")
	}
	previous, prior := near, near
	for cascade := 0; cascade < s.Cascades; cascade++ {
		end := cascadeSplit3D(near, s.Distance, cascade+1, s.Cascades, s.SplitLambda)
		start := previous - (previous-prior)*s.CascadeBlend
		var corners [8]mgl32.Vec3
		var center mgl32.Vec3
		for corner := 0; corner < 4; corner++ {
			x, y := float32((corner&1)*2-1), float32((corner>>1)*2-1)
			a, b := unproject(x, y, 1), unproject(x, y, .5)
			delta := b.Sub(a)
			depth := a.Sub(camera.position).Dot(forward)
			step := delta.Dot(forward)
			corners[corner] = a.Add(delta.Mul((start - depth) / step))
			corners[corner+4] = a.Add(delta.Mul((end - depth) / step))
			center = center.Add(corners[corner]).Add(corners[corner+4])
		}
		center = center.Mul(.125)
		radius := float32(0)
		for _, corner := range corners {
			radius = max(radius, corner.Sub(center).Len())
		}
		// A sphere keeps extent stable during camera rotation; the margin covers
		// center snapping and PCF footprints at the edge of each cascade.
		radius = float32(math.Ceil(float64(radius)*16)/16) * float32(s.Resolution) / float32(s.Resolution-4)
		projection := s
		projection.Radius, projection.Depth = radius, 2*radius+s.Depth
		matrices[cascade] = directionalShadowMatrix(center, direction, projection)
		ranges[cascade] = [2]float32{previous, end}
		prior, previous = previous, end
	}
	return matrices, ranges, [4]float32{forward.X(), forward.Y(), forward.Z()}, nil
}

func punctualShadowMatrices3D(light render.Light3D, s PunctualShadowSettings) [6]mgl32.Mat4 {
	var matrices [6]mgl32.Mat4
	position := mgl32.Vec3(light.PositionDirection[:3])
	far, near := light.Params[0], s.Near
	projection := func(x, y float32) mgl32.Mat4 {
		return mgl32.Mat4{1 / x, 0, 0, 0, 0, 1 / y, 0, 0, 0, 0, near / (far - near), -1, 0, 0, near * far / (far - near), 0}
	}
	if light.Kind == render.Light3DPoint {
		axes := [...]mgl32.Vec3{{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}}
		ups := [...]mgl32.Vec3{{0, -1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}, {0, -1, 0}, {0, -1, 0}}
		p := projection(1, 1)
		for face, axis := range axes {
			matrices[face] = p.Mul4(mgl32.LookAtV(position, position.Add(axis), ups[face]))
		}
		return matrices
	}
	forward, up := mgl32.Vec3(light.Direction[:3]), mgl32.Vec3(light.Up[:3])
	view := mgl32.LookAtV(position, position.Add(forward), up)
	var p mgl32.Mat4
	switch light.Kind {
	case render.Light3DBox:
		p = mgl32.Mat4{1 / light.Right[3], 0, 0, 0, 0, 1 / light.Up[3], 0, 0, 0, 0, 1 / (far - near), 0, 0, 0, far / (far - near), 1}
	case render.Light3DPyramid:
		p = projection(light.Right[3], light.Up[3])
	default:
		cosine := light.Params[3]
		tangent := float32(math.Sqrt(float64(1-cosine*cosine))) / cosine
		p = projection(tangent, tangent)
	}
	matrices[0] = p.Mul4(view)
	return matrices
}
