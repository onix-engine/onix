package render3d

import (
	"fmt"
	"math"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

// PointLight is an isotropic source. Range, Radius and LuxDistance are meters.
type PointLight struct {
	Shadows     PunctualShadowSettings
	Color       graphics.ColorRGBA
	Intensity   float32
	Unit        graphics.LightUnit
	LuxDistance float32
	Range       float32
	Radius      float32
}

func NewPointLight() PointLight {
	return PointLight{Color: graphics.White, Intensity: 600, Unit: graphics.LightLumen, LuxDistance: 1, Range: 10}
}

type SpotLightShape string

const (
	SpotCone    SpotLightShape = "cone"
	SpotPyramid SpotLightShape = "pyramid"
	SpotBox     SpotLightShape = "box"
)

// SpotLight emits along local -Z. Cone angles are full angles in radians,
// matching HDRP's Inner/Outer Angle after converting degrees to radians.
type SpotLight struct {
	Shadows          PunctualShadowSettings
	Color            graphics.ColorRGBA
	Intensity        float32
	Unit             graphics.LightUnit
	LuxDistance      float32
	Range            float32
	Radius           float32
	Shape            SpotLightShape
	InnerConeAngle   float32
	OuterConeAngle   float32
	AspectRatio      float32
	Width            float32
	Height           float32
	Reflector        bool
	RangeAttenuation bool // box only; punctual lights always attenuate
}

func NewSpotLight() SpotLight {
	return SpotLight{Color: graphics.White, Intensity: 600, Unit: graphics.LightLumen, LuxDistance: 1, Range: 10, Shape: SpotCone, InnerConeAngle: math.Pi / 6, OuterConeAngle: math.Pi / 3, AspectRatio: 1, Width: 1, Height: 1, Reflector: true, RangeAttenuation: true}
}

type AreaLightShape string

const (
	AreaRectangle AreaLightShape = "rectangle"
	AreaDisc      AreaLightShape = "disc"
	AreaTube      AreaLightShape = "tube"
)

// AreaLight uses nits internally. Rectangle/disc emit towards local -Z;
// tube emits in all directions along local X. Width is tube length; Radius
// is disc radius. Dimensions are independent of the entity's transform scale.
type AreaLight struct {
	Color     graphics.ColorRGBA
	Intensity float32
	Unit      graphics.LightUnit
	Shape     AreaLightShape
	Range     float32
	Width     float32
	Height    float32
	Radius    float32
}

func NewAreaLight() AreaLight {
	return AreaLight{Color: graphics.White, Intensity: 200, Unit: graphics.LightLumen, Shape: AreaRectangle, Range: 10, Width: 1, Height: 1, Radius: 0.5}
}

func validLightNumbers(values ...float32) bool {
	for _, v := range values {
		if v < 0 || math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return false
		}
	}
	return true
}

func validateLightColor(c graphics.ColorRGBA) error {
	if !validLightNumbers(c.R, c.G, c.B) {
		return fmt.Errorf("light RGB must be finite and non-negative")
	}
	return nil
}

func (l DirectionalLight) Validate() error {
	if !validLightNumbers(l.Illuminance) {
		return fmt.Errorf("directional illuminance must be finite and non-negative lux")
	}
	if err := validateLightColor(l.Color); err != nil {
		return err
	}
	return l.Shadows.Validate()
}

func (l PointLight) Validate() error {
	if err := graphics.ValidateLightIntensity(l.Intensity, l.Unit, graphics.LightLumen, graphics.LightCandela, graphics.LightLux, graphics.LightEV100); err != nil {
		return err
	}
	if !validLightNumbers(l.Range, l.Radius, l.LuxDistance) || l.Range <= 0 || l.Unit == graphics.LightLux && l.LuxDistance <= 0 {
		return fmt.Errorf("point light requires positive range and lux reference distance, non-negative radius")
	}
	if !validLightNumbers(l.Candela()) {
		return fmt.Errorf("point light intensity overflows")
	}
	if err := validateLightColor(l.Color); err != nil {
		return err
	}
	return l.Shadows.Validate(l.Range)
}

func (l PointLight) Candela() float32 {
	return graphics.PunctualCandela(l.Intensity, l.Unit, 4*math.Pi, l.LuxDistance)
}

func (l SpotLight) Validate() error {
	if l.Shape == SpotBox {
		if err := graphics.ValidateLightIntensity(l.Intensity, l.Unit, graphics.LightLux); err != nil {
			return err
		}
	} else if err := graphics.ValidateLightIntensity(l.Intensity, l.Unit, graphics.LightLumen, graphics.LightCandela, graphics.LightLux, graphics.LightEV100); err != nil {
		return err
	}
	if !validLightNumbers(l.Range, l.Radius, l.LuxDistance, l.InnerConeAngle, l.OuterConeAngle, l.AspectRatio, l.Width, l.Height) || l.Range <= 0 || l.Unit == graphics.LightLux && l.Shape != SpotBox && l.LuxDistance <= 0 {
		return fmt.Errorf("invalid spot light dimensions or lux reference distance")
	}
	switch l.Shape {
	case SpotCone, SpotPyramid:
		if l.OuterConeAngle <= 0 || l.OuterConeAngle >= math.Pi || l.InnerConeAngle > l.OuterConeAngle || l.Shape == SpotPyramid && l.AspectRatio <= 0 {
			return fmt.Errorf("spot full angles must satisfy 0 <= inner <= outer < pi, and pyramid aspect ratio must be positive")
		}
	case SpotBox:
		if l.Width <= 0 || l.Height <= 0 {
			return fmt.Errorf("box light dimensions must be positive")
		}
	default:
		return fmt.Errorf("unsupported spot shape %q", l.Shape)
	}
	if l.Shape == SpotPyramid {
		x, y := l.PyramidTangents()
		if !validLightNumbers(x, y) {
			return fmt.Errorf("pyramid dimensions overflow")
		}
	}
	if !validLightNumbers(l.PhysicalIntensity()) {
		return fmt.Errorf("spot light intensity overflows")
	}
	if err := validateLightColor(l.Color); err != nil {
		return err
	}
	return l.Shadows.Validate(l.Range)
}

func (l SpotLight) PyramidTangents() (float32, float32) {
	t := float32(math.Tan(float64(l.OuterConeAngle) * 0.5))
	if l.AspectRatio >= 1 {
		return t * l.AspectRatio, t
	}
	return t, t / l.AspectRatio
}

func (l SpotLight) SolidAngle() float32 {
	if !l.Reflector {
		return 4 * math.Pi
	}
	if l.Shape == SpotPyramid {
		x, y := l.PyramidTangents()
		return float32(4 * math.Atan2(float64(x)*float64(y), math.Sqrt(1+float64(x)*float64(x)+float64(y)*float64(y))))
	}
	return float32(2 * math.Pi * (1 - math.Cos(float64(l.OuterConeAngle)*0.5)))
}

func (l SpotLight) PhysicalIntensity() float32 {
	if l.Shape == SpotBox {
		return l.Intensity
	}
	return graphics.PunctualCandela(l.Intensity, l.Unit, l.SolidAngle(), l.LuxDistance)
}

func (l AreaLight) ProjectedArea() float32 {
	switch l.Shape {
	case AreaRectangle:
		return math.Pi * l.Width * l.Height
	case AreaDisc:
		return math.Pi * math.Pi * l.Radius * l.Radius
	case AreaTube:
		return 4 * math.Pi * l.Width
	default:
		return 0
	}
}

func (l AreaLight) Nits() float32 { return graphics.AreaNits(l.Intensity, l.Unit, l.ProjectedArea()) }

func (l AreaLight) Validate() error {
	if err := graphics.ValidateLightIntensity(l.Intensity, l.Unit, graphics.LightLumen, graphics.LightNits, graphics.LightEV100); err != nil {
		return err
	}
	if !validLightNumbers(l.Range, l.Width, l.Height, l.Radius) || l.Range <= 0 || l.ProjectedArea() <= 0 || !validLightNumbers(l.ProjectedArea(), l.Nits()) {
		return fmt.Errorf("invalid area light shape, dimensions or intensity")
	}
	return validateLightColor(l.Color)
}

func lightBasis(m mgl32.Mat4) (mgl32.Vec3, mgl32.Vec3, mgl32.Vec3) {
	right := m.Col(0).Vec3()
	forward := m.Col(2).Vec3().Mul(-1)
	if forward.LenSqr() < 1e-12 {
		forward = mgl32.Vec3{0, 0, -1}
	}
	forward = forward.Normalize()
	right = right.Sub(forward.Mul(right.Dot(forward)))
	if right.LenSqr() < 1e-12 {
		right = forward.Cross(mgl32.Vec3{0, 1, 0})
		if right.LenSqr() < 1e-12 {
			right = mgl32.Vec3{1, 0, 0}
		}
	}
	right = right.Normalize()
	up := right.Cross(forward).Normalize()
	return right, up, forward
}

func localLight(m mgl32.Mat4, color graphics.ColorRGBA, radius float32) render.Light3D {
	r, u, f := lightBasis(m)
	p := m.Col(3)
	return render.Light3D{PositionDirection: [4]float32{p[0], p[1], p[2], 1}, Direction: [4]float32{f[0], f[1], f[2]}, Right: [4]float32{r[0], r[1], r[2]}, Up: [4]float32{u[0], u[1], u[2]}, Color: [4]float32{color.R, color.G, color.B, radius}}
}

func (l PointLight) renderLight(m mgl32.Mat4) render.Light3D {
	v := localLight(m, l.Color, l.Radius)
	v.Kind = render.Light3DPoint
	v.Params = [4]float32{l.Range, l.Candela()}
	return v
}

func (l PointLight) shadowSettings() PunctualShadowSettings { return l.Shadows }
func (l SpotLight) shadowSettings() PunctualShadowSettings  { return l.Shadows }
func (l AreaLight) shadowSettings() PunctualShadowSettings  { return PunctualShadowSettings{} }
func (l SpotLight) renderLight(m mgl32.Mat4) render.Light3D {
	v := localLight(m, l.Color, l.Radius)
	v.Kind = render.Light3DSpot
	v.Params = [4]float32{l.Range, l.PhysicalIntensity(), float32(math.Cos(float64(l.InnerConeAngle) * 0.5)), float32(math.Cos(float64(l.OuterConeAngle) * 0.5))}
	switch l.Shape {
	case SpotPyramid:
		v.Kind = render.Light3DPyramid
		v.Right[3], v.Up[3] = l.PyramidTangents()
	case SpotBox:
		v.Kind = render.Light3DBox
		v.Right[3], v.Up[3] = l.Width*0.5, l.Height*0.5
		v.Params[2] = 0
		if l.RangeAttenuation {
			v.Params[2] = 1
		}
	}
	return v
}
func (l AreaLight) renderLight(m mgl32.Mat4) render.Light3D {
	v := localLight(m, l.Color, 0)
	v.Params = [4]float32{l.Range, l.Nits()}
	v.Right[3], v.Up[3] = l.Width, l.Height
	switch l.Shape {
	case AreaRectangle:
		v.Kind = render.Light3DRectangle
	case AreaDisc:
		v.Kind = render.Light3DDisc
		v.Right[3] = l.Radius
	case AreaTube:
		v.Kind = render.Light3DTube
	}
	return v
}

// The generic extraction path reuses the world's typed query and light storage.
func (state *World) extractLocalLights[L interface {
	Validate() error
	renderLight(mgl32.Mat4) render.Light3D
	shadowSettings() PunctualShadowSettings
}](world *ecs.World) {
	for entity, access := range world.Query2[L, scene.GlobalTransform]().All {
		light := access.A.Get()
		transform := access.B.Get()

		if visibility, ok := world.Get[scene.Visibility](entity); ok && !visibility.Visible {
			continue
		}
		if light.Validate() == nil {
			state.lights = append(state.lights, extractedLight3D{entity: entity, scene: scene.LayerForEntity(world, entity).ID, layers: scene.LayersForEntity(world, entity), shadows: extractedShadowLight3D{punctual: light.shadowSettings()}, Light3D: light.renderLight(transform.Matrix)})
		}
	}
}
