package viewer

import (
	"fmt"
	"math"
	"time"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/onix-engine/onix/input"
	"github.com/onix-engine/onix/module"
	"github.com/onix-engine/onix/scene"
	"github.com/onix-engine/onix/scene/render3d"
)

type Mod struct {
	camera                            module.Entity
	center                            mgl32.Vec3
	fitDistance, distance, yaw, pitch float32
	keys                              uint8
}

func (m *Mod) Install(c *module.Context) error {
	var err error
	if m.camera, err = c.Element("camera"); err != nil {
		return err
	}
	helmet, err := c.Element("helmet")
	if err != nil {
		return err
	}
	geometry, ok := c.Get[render3d.Mesh](helmet)
	if !ok || geometry.Mesh == nil || len(geometry.Mesh.Vertices) == 0 {
		return fmt.Errorf("DamagedHelmet has no imported geometry")
	}
	transform, ok := c.Get[scene.Transform](helmet)
	if !ok {
		return module.ErrType
	}
	matrix := transform.LocalMatrix()
	low := mgl32.Vec3{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32}
	high := low.Mul(-1)
	// Imported vertices already include the glTF node rotation. Fit once,
	// using one traversal, following the captured three.js example.
	for _, vertex := range geometry.Mesh.Vertices {
		point := matrix.Mul4x1(mgl32.Vec3(vertex.Position).Vec4(1)).Vec3()
		for axis := range 3 {
			low[axis], high[axis] = min(low[axis], point[axis]), max(high[axis], point[axis])
		}
	}
	m.center = low.Add(high).Mul(.5)
	size := high.Sub(low)
	m.fitDistance = 1.3 * max(size.X(), size.Y(), size.Z()) / (2 * float32(math.Atan(math.Pi/8)))
	projection, ok := c.Get[scene.Projection](m.camera)
	if !ok {
		return module.ErrType
	}
	projection.Perspective.Near = m.fitDistance / 100
	if err := c.Set(m.camera, projection); err != nil {
		return err
	}
	m.reset()
	return m.apply(c)
}

func (m *Mod) reset() {
	m.distance = m.fitDistance
	m.yaw = float32(math.Atan2(-1.8, 2.9))
	m.pitch = float32(math.Atan2(.6, math.Hypot(1.8, 2.9)))
}

func (m *Mod) Input(_ *module.Context, event module.Input) error {
	switch event.Event.Kind {
	case scene.InputEventKeyDown:
		m.keys |= keyMask(event.Event.Key)
		if event.Event.Key == input.KeyR {
			m.reset()
		}
	case scene.InputEventKeyUp:
		m.keys &^= keyMask(event.Event.Key)
	}
	return nil
}

func (m *Mod) OnMessage(c *module.Context, message module.Message) error {
	if message.Topic != module.InputTopic {
		return nil
	}
	event, ok := message.Value[module.Input]()
	if !ok {
		return module.ErrType
	}
	return m.Input(c, event)
}

func keyMask(key input.KeyCode) uint8 {
	switch key {
	case input.KeyLeft:
		return 1
	case input.KeyRight:
		return 2
	case input.KeyUp:
		return 4
	case input.KeyDown:
		return 8
	case input.KeyQ:
		return 16
	case input.KeyE:
		return 32
	}
	return 0
}

func axis(positive, negative bool) float32 {
	var result float32
	if positive {
		result++
	}
	if negative {
		result--
	}
	return result
}

func (m *Mod) FrameUpdate(c *module.Context, tick module.Tick) error {
	if state, ok := c.Content().Resource[*input.State](); ok && state.MouseHeld(input.MouseLeft) {
		pointer := state.Pointer()
		if pointer.Device == input.PointerDeviceMouse {
			m.yaw -= pointer.DeltaX * .005
			m.pitch += pointer.DeltaY * .005
		}
	}
	delta := min(tick.Delta, .1)
	m.yaw += axis(m.keys&2 != 0, m.keys&1 != 0) * delta
	m.pitch = max(-1.5, min(1.5, m.pitch+axis(m.keys&4 != 0, m.keys&8 != 0)*delta))
	m.distance *= float32(math.Exp(float64(axis(m.keys&32 != 0, m.keys&16 != 0) * delta)))
	m.distance = max(m.fitDistance/10, min(m.fitDistance*10, m.distance))
	if m.keys != 0 {
		if host := c.Host(); host != nil && host.WakeAt != nil {
			// VSync already paces active frames; adding another frame interval
			// here would delay input and miss refreshes after presentation.
			host.WakeAt(time.Now())
		}
	}
	return m.apply(c)
}

func (m *Mod) apply(c *module.Context) error {
	sinYaw, cosYaw := math.Sincos(float64(m.yaw))
	sinPitch, cosPitch := math.Sincos(float64(m.pitch))
	position := m.center.Add(mgl32.Vec3{float32(sinYaw * cosPitch), float32(sinPitch), float32(cosYaw * cosPitch)}.Mul(m.distance))
	transform := scene.TransformFromTranslation(position).LookingAt(m.center, mgl32.Vec3{0, 1, 0})
	if previous, ok := c.Get[scene.Transform](m.camera); ok && previous == transform {
		return nil
	}
	if err := c.Set(m.camera, transform); err != nil {
		return err
	}
	if host := c.Host(); host != nil && host.RequestRender != nil {
		host.RequestRender()
	}
	return nil
}
