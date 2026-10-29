package ui

import (
	"math"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

// VirtualJoystickAxis limits a virtual joystick to one or both axes.
type VirtualJoystickAxis uint8

const (
	VirtualJoystickBoth VirtualJoystickAxis = iota
	VirtualJoystickHorizontal
	VirtualJoystickVertical
)

// VirtualJoystick stores the normalized output of a virtual joystick.
// RawValue retains the clamped physical deflection, while Value applies Axis
// and DeadZone. Track and Knob are optional visual part entities. Floating
// joysticks show those parts at the initial press position until release.
type VirtualJoystick struct {
	Value    mgl32.Vec2
	RawValue mgl32.Vec2
	DeadZone float32
	Axis     VirtualJoystickAxis
	Active   bool
	Floating bool
	Track    ecs.Entity
	Knob     ecs.Entity
	origin   mgl32.Vec2
}

// VirtualJoystickOptions configures a generated virtual joystick. Size is the
// diameter of its touch area and visual track. A non-positive size uses 120.
type VirtualJoystickOptions struct {
	X, Y        float32
	Size        float32
	KnobSize    float32
	DeadZone    float32
	Axis        VirtualJoystickAxis
	Floating    bool
	Disabled    bool
	PassThrough bool
	Theme       Theme
	Style       *VirtualJoystickStyle
}

// VirtualJoystickActiveEvent is emitted when a joystick is engaged or reset.
type VirtualJoystickActiveEvent struct {
	Entity ecs.Entity
	Active bool
}

// SpawnVirtualJoystick creates a complete virtual joystick. Set Floating to
// show its visual parts at the press position instead of keeping them visible.
// Read VirtualJoystick.Value from the returned entity, or subscribe to
// ChangedEvent[mgl32.Vec2] and VirtualJoystickActiveEvent.
func SpawnVirtualJoystick(world *ecs.World, opts VirtualJoystickOptions) ecs.Entity {
	size := opts.Size
	if size <= 0 {
		size = 120
	}
	knobSize := opts.KnobSize
	if knobSize <= 0 {
		knobSize = size * 0.46
	}
	knobSize = min(max(knobSize, 0), size)
	theme := themeForWorld(world, opts.Theme)
	entity := spawnInteractiveAnchor(world, opts.X, opts.Y, size, size, theme.StackIndex, opts.Disabled, opts.PassThrough)
	track := spawnSurfacePart[VirtualJoystickTrackPart](world, entity, 0, 0, size, size, theme.Surface, theme.StackIndex)
	setCornerRadius(world, track, Circle)
	knob := spawnSurfacePart[VirtualJoystickKnobPart](world, entity, (size-knobSize)*.5, (size-knobSize)*.5, knobSize, knobSize, theme.Primary, theme.StackIndex+1)
	setCornerRadius(world, knob, Circle)
	_ = world.Insert(entity, VirtualJoystick{
		DeadZone: normalizedVirtualJoystickDeadZone(opts.DeadZone), Axis: normalizedVirtualJoystickAxis(opts.Axis),
		Floating: opts.Floating, Track: track, Knob: knob,
	})
	if opts.Floating {
		setVirtualJoystickVisualsVisible(world, track, knob, false)
	}
	putStyleOverride(world, entity, opts.Style)
	return entity
}

// BindVirtualJoystick adds joystick behavior to an authored UI control root.
// It intentionally does not generate visual parts, letting a scene provide
// its own art while preserving the same value and event behavior.
func BindVirtualJoystick(world *ecs.World, entity ecs.Entity, opts ControlOptions, joystick VirtualJoystick) error {
	return BindControl(world, entity, opts, joystick)
}

func (joystick VirtualJoystick) bindControl(world *ecs.World, entity ecs.Entity, _ ControlOptions) error {
	joystick.DeadZone = normalizedVirtualJoystickDeadZone(joystick.DeadZone)
	joystick.Axis = normalizedVirtualJoystickAxis(joystick.Axis)
	joystick.RawValue = constrainedVirtualJoystickValue(joystick.RawValue, joystick.Axis)
	joystick.Value = virtualJoystickOutput(joystick.RawValue, joystick.DeadZone)
	if err := world.Insert(entity, joystick); err != nil {
		return err
	}
	if joystick.Floating && !joystick.Active {
		setVirtualJoystickVisualsVisible(world, joystick.Track, joystick.Knob, false)
	}
	return nil
}

// ResetVirtualJoystick releases a joystick and publishes state changes.
func ResetVirtualJoystick(world *ecs.World, entity ecs.Entity) bool {
	joystick := world.GetMut[VirtualJoystick](entity)
	if joystick == nil {
		return false
	}
	setVirtualJoystickState(world, entity, joystick, mgl32.Vec2{}, false)
	return true
}

// SetVirtualJoystickRawValue sets a joystick's physical normalized deflection.
// The value is clamped to the unit circle before Axis and DeadZone are applied.
func SetVirtualJoystickRawValue(world *ecs.World, entity ecs.Entity, value mgl32.Vec2) bool {
	joystick := world.GetMut[VirtualJoystick](entity)
	if joystick == nil {
		return false
	}
	setVirtualJoystickState(world, entity, joystick, value, joystick.Active)
	return true
}

func updateVirtualJoystickInput(world *ecs.World, event *scene.InputEvent) {
	if event == nil || event.Entity == 0 {
		return
	}
	joystick := world.GetMut[VirtualJoystick](event.Entity)
	if joystick == nil {
		return
	}
	switch event.Kind {
	case scene.InputEventPress:
		if joystick.Floating {
			origin, valid := virtualJoystickPointerPosition(world, event.Entity, event.X, event.Y)
			if valid {
				joystick.origin = origin
				setVirtualJoystickState(world, event.Entity, joystick, mgl32.Vec2{}, true)
			} else {
				setVirtualJoystickState(world, event.Entity, joystick, mgl32.Vec2{}, false)
			}
			return
		}
		value, valid := virtualJoystickPointerValue(world, event.Entity, event.X, event.Y, joystick.Axis)
		if valid {
			setVirtualJoystickState(world, event.Entity, joystick, value, true)
		} else {
			setVirtualJoystickState(world, event.Entity, joystick, mgl32.Vec2{}, false)
		}
	case scene.InputEventMove:
		value, valid := virtualJoystickPointerValue(world, event.Entity, event.X, event.Y, joystick.Axis)
		if joystick.Floating {
			value, valid = floatingVirtualJoystickPointerValue(world, event.Entity, joystick, event.X, event.Y)
		}
		if valid {
			setVirtualJoystickState(world, event.Entity, joystick, value, true)
		} else {
			setVirtualJoystickState(world, event.Entity, joystick, mgl32.Vec2{}, false)
		}
	case scene.InputEventRelease, scene.InputEventCancel:
		setVirtualJoystickState(world, event.Entity, joystick, mgl32.Vec2{}, false)
	}
}

func setVirtualJoystickState(world *ecs.World, entity ecs.Entity, joystick *VirtualJoystick, rawValue mgl32.Vec2, active bool) {
	if joystick == nil {
		return
	}
	joystick.DeadZone = normalizedVirtualJoystickDeadZone(joystick.DeadZone)
	joystick.Axis = normalizedVirtualJoystickAxis(joystick.Axis)
	rawValue = constrainedVirtualJoystickValue(rawValue, joystick.Axis)
	value := virtualJoystickOutput(rawValue, joystick.DeadZone)
	valueChanged := joystick.Value != value
	activeChanged := joystick.Active != active
	joystick.RawValue, joystick.Value, joystick.Active = rawValue, value, active
	if joystick.Floating {
		setVirtualJoystickVisualsVisible(world, joystick.Track, joystick.Knob, active)
	}
	if valueChanged {
		world.WriteMessage(ChangedEvent[mgl32.Vec2]{Entity: entity, Value: value})
	}
	if activeChanged {
		world.WriteMessage(VirtualJoystickActiveEvent{Entity: entity, Active: active})
	}
}

func virtualJoystickPointerValue(world *ecs.World, entity ecs.Entity, x, y float32, axis VirtualJoystickAxis) (mgl32.Vec2, bool) {
	width, height, ok := ControlBounds(world, entity)
	if !ok || width <= 0 || height <= 0 {
		return mgl32.Vec2{}, false
	}
	local, ok := virtualJoystickPointerPosition(world, entity, x, y)
	if !ok {
		return mgl32.Vec2{}, false
	}
	radius := min(width, height) * .5
	if radius <= 0 {
		return mgl32.Vec2{}, false
	}
	return constrainedVirtualJoystickValue(mgl32.Vec2{
		(local.X() - width*.5) / radius,
		(local.Y() - height*.5) / radius,
	}, axis), true
}

func floatingVirtualJoystickPointerValue(world *ecs.World, entity ecs.Entity, joystick *VirtualJoystick, x, y float32) (mgl32.Vec2, bool) {
	if joystick == nil {
		return mgl32.Vec2{}, false
	}
	local, ok := virtualJoystickPointerPosition(world, entity, x, y)
	if !ok {
		return mgl32.Vec2{}, false
	}
	trackWidth, trackHeight, trackOK := virtualJoystickPartSize(world, joystick.Track)
	if !trackOK {
		trackWidth, trackHeight, trackOK = ControlBounds(world, entity)
	}
	radius := min(trackWidth, trackHeight) * .5
	if !trackOK || radius <= 0 {
		return mgl32.Vec2{}, false
	}
	return constrainedVirtualJoystickValue(local.Sub(joystick.origin).Mul(1/radius), joystick.Axis), true
}

func virtualJoystickPointerPosition(world *ecs.World, entity ecs.Entity, x, y float32) (mgl32.Vec2, bool) {
	transform := world.GetRef[scene.UIGlobalTransform](entity)
	if transform == nil || math.Abs(float64(transform.Matrix.Det())) <= 1e-8 {
		return mgl32.Vec2{}, false
	}
	local := transform.Matrix.Inv().Mul4x1(mgl32.Vec4{x, y, 0, 1})
	if local.W() == 0 {
		return mgl32.Vec2{}, false
	}
	local = local.Mul(1 / local.W())
	return mgl32.Vec2{local.X(), local.Y()}, true
}

func constrainedVirtualJoystickValue(value mgl32.Vec2, axis VirtualJoystickAxis) mgl32.Vec2 {
	if math.IsNaN(float64(value.X())) || math.IsNaN(float64(value.Y())) || math.IsInf(float64(value.X()), 0) || math.IsInf(float64(value.Y()), 0) {
		return mgl32.Vec2{}
	}
	switch normalizedVirtualJoystickAxis(axis) {
	case VirtualJoystickHorizontal:
		value[1] = 0
	case VirtualJoystickVertical:
		value[0] = 0
	}
	lengthSquared := value.LenSqr()
	if lengthSquared > 1 {
		return value.Mul(1 / float32(math.Sqrt(float64(lengthSquared))))
	}
	return value
}

func virtualJoystickOutput(rawValue mgl32.Vec2, deadZone float32) mgl32.Vec2 {
	lengthSquared := rawValue.LenSqr()
	if lengthSquared == 0 {
		return mgl32.Vec2{}
	}
	length := float32(math.Sqrt(float64(lengthSquared)))
	deadZone = normalizedVirtualJoystickDeadZone(deadZone)
	if length <= deadZone || deadZone >= 1 {
		return mgl32.Vec2{}
	}
	return rawValue.Mul((length - deadZone) / (length * (1 - deadZone)))
}

func normalizedVirtualJoystickDeadZone(value float32) float32 {
	if !(value > 0) || math.IsNaN(float64(value)) {
		return 0
	}
	if value >= 1 || math.IsInf(float64(value), 0) {
		return 1
	}
	return value
}

func normalizedVirtualJoystickAxis(axis VirtualJoystickAxis) VirtualJoystickAxis {
	switch axis {
	case VirtualJoystickHorizontal, VirtualJoystickVertical:
		return axis
	default:
		return VirtualJoystickBoth
	}
}

func syncVirtualJoysticks(world *ecs.World, skin Skin) {
	for entity, access := range world.Query[VirtualJoystick]().All {
		joystick := access.Get()

		width, height, ok := ControlBounds(world, entity)
		if !ok || width <= 0 || height <= 0 {
			continue
		}
		control, _ := controlViewFor(world, entity)
		style := styleFor(world, entity, skin.VirtualJoystick)
		trackWidth, trackHeight, trackOK := virtualJoystickPartSize(world, joystick.Track)
		if !trackOK {
			trackWidth, trackHeight = width, height
		}
		if joystick.Track != 0 && world.IsAlive(joystick.Track) {
			applyVirtualJoystickSurface(world, joystick.Track, trackWidth, trackHeight, style.Track.Resolve(controlState(&control, false)), control.StackIndex)
		}
		radius := min(width, height) * .5
		centerX, centerY := width*.5, height*.5
		if joystick.Floating {
			radius = min(trackWidth, trackHeight) * .5
			centerX, centerY = joystick.origin.X(), joystick.origin.Y()
			if trackTransform := world.GetMut[scene.UITransform](joystick.Track); trackTransform != nil {
				trackTransform.Translation.X.Value = centerX - trackWidth*.5
				trackTransform.Translation.Y.Value = centerY - trackHeight*.5
			}
		}
		if joystick.Knob == 0 || !world.IsAlive(joystick.Knob) {
			continue
		}
		knobWidth, knobHeight, sizeOK := virtualJoystickPartSize(world, joystick.Knob)
		transform := world.GetMut[scene.UITransform](joystick.Knob)
		if (!sizeOK) || (transform == nil) {
			continue
		}
		knobRadius := min(knobWidth, knobHeight) * .5
		travel := max(radius-knobRadius, 0)
		transform.Translation.X.Value = centerX + joystick.RawValue.X()*travel - knobWidth*.5
		transform.Translation.Y.Value = centerY + joystick.RawValue.Y()*travel - knobHeight*.5
		applyVirtualJoystickSurface(world, joystick.Knob, knobWidth, knobHeight, style.Knob.Resolve(controlState(&control, false)), control.StackIndex+1)
	}
}

func virtualJoystickPartSize(world *ecs.World, entity ecs.Entity) (float32, float32, bool) {
	if entity == 0 || !world.IsAlive(entity) {
		return 0, 0, false
	}
	if size := world.GetRef[scene.Size2D](entity); size != nil && size.Width > 0 && size.Height > 0 {
		return size.Width, size.Height, true
	}
	if sprite := world.GetRef[scene.Sprite2D](entity); sprite != nil && sprite.Width > 0 && sprite.Height > 0 {
		return sprite.Width, sprite.Height, true
	}
	return 0, 0, false
}

func setVirtualJoystickVisualsVisible(world *ecs.World, track, knob ecs.Entity, visible bool) {
	for _, entity := range [...]ecs.Entity{track, knob} {
		if visibility := world.GetMut[scene.Visibility](entity); visibility != nil {
			visibility.Visible = visible
		}
	}
}

func applyVirtualJoystickSurface(world *ecs.World, entity ecs.Entity, width, height float32, style SurfaceStyle, stackIndex int) {
	if !world.Has[scene.UIBackground2D](entity) {
		return
	}
	applySurface(world, entity, width, height, style)
	setUIStackIndex(world, entity, stackIndex)
}
