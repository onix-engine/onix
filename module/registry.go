// Package module runs independent minimum systems composed by project scene plans.
package module

import (
	"fmt"

	"github.com/diiyw/onix/project"
)

// Registry contains cold registration only. A Runtime snapshots it at creation.
type Registry struct {
	values map[project.ValueType]any
	mods   map[project.ModID]installer
	scenes map[project.SceneID]installer
}

type installer func(*Context) error

// Lifecycle creates private state for every instance. Install and scene.startup
// run once during loading. Enter/Resume prepare activation before its commit;
// errors preserve the current foreground. Exit/Suspend leave the foreground,
// and Destroy releases the instance. Nil hooks have no schedule cost.
type Lifecycle[T any] struct {
	Install func(*Context, *T) error
	Enter   func(*Context, *T, SceneStartup) error
	Exit    func(*Context, *T) error
	Suspend func(*Context, *T) error
	Resume  func(*Context, *T, SceneStartup) error
	Update  func(*Context, *T, Tick) error
	// FrameUpdate runs once per awake frame before UI synchronization. Unlike
	// fixed Update, it does not keep an on-demand application awake.
	FrameUpdate func(*Context, *T, Tick) error
	OnMessage   func(*Context, *T, Message) error
	Destroy     func(*Context, *T) error
}

func RegisterMod[T any](r *Registry, id project.ModID, create func() T, hooks Lifecycle[T]) error {
	if r == nil || create == nil {
		return ErrInvalid
	}
	if r.mods == nil {
		r.mods = make(map[project.ModID]installer)
	}
	if _, ok := r.mods[id]; ok {
		return fmt.Errorf("module %q already registered", id)
	}
	r.mods[id] = lifecycleInstaller(create, hooks)
	return nil
}

func RegisterScene[T any](r *Registry, id project.SceneID, create func() T, hooks Lifecycle[T]) error {
	if r == nil || create == nil {
		return ErrInvalid
	}
	if r.scenes == nil {
		r.scenes = make(map[project.SceneID]installer)
	}
	if _, ok := r.scenes[id]; ok {
		return fmt.Errorf("scene %q already registered", id)
	}
	r.scenes[id] = lifecycleInstaller(create, hooks)
	return nil
}

func lifecycleInstaller[T any](create func() T, hooks Lifecycle[T]) installer {
	return func(c *Context) error {
		state := create()
		if hooks.Enter != nil {
			c.enter = func(v SceneStartup) error { return hooks.Enter(c, &state, v) }
		}
		if hooks.Exit != nil {
			c.exit = func() error { return hooks.Exit(c, &state) }
		}
		if hooks.Suspend != nil {
			c.suspend = func() error { return hooks.Suspend(c, &state) }
		}
		if hooks.Resume != nil {
			c.resume = func(v SceneStartup) error { return hooks.Resume(c, &state, v) }
		}
		if hooks.Destroy != nil {
			c.destroy = func() error { return hooks.Destroy(c, &state) }
		}
		if hooks.Update != nil {
			c.update = func(t Tick) error { return hooks.Update(c, &state, t) }
		}
		if hooks.FrameUpdate != nil {
			c.frameUpdate = func(t Tick) error { return hooks.FrameUpdate(c, &state, t) }
		}
		if hooks.OnMessage != nil {
			c.message = func(e Message) error { return hooks.OnMessage(c, &state, e) }
		}
		if hooks.Install != nil {
			return hooks.Install(c, &state)
		}
		return nil
	}
}

// RegisterValue connects a JSON protocol name to a concrete Go type. Decode is
// used only for slot reads, never to encode message payloads.
// Slice/pointer fields in event values are immutable until delivery completes.
func RegisterValue[T any](r *Registry, name project.ValueType, decode func(project.SlotValue) (T, error)) error {
	if r == nil || name == "" {
		return ErrInvalid
	}
	switch name {
	case project.StringValue, project.BoolValue, project.IntValue, project.NumberValue, project.Vec2Value, project.Vec3Value, project.ColorValue, project.VisualValue, "SceneSwitchRequest", "SceneSwitchResult", "SceneStartup", "TextRenderRequest", "TextRenderResult", "Collision2D", "Collision3D":
		return fmt.Errorf("protocol %q is reserved by the engine", name)
	}
	if r.values == nil {
		r.values = make(map[project.ValueType]any)
	}
	if _, ok := r.values[name]; ok {
		return fmt.Errorf("value %q already registered", name)
	}
	r.values[name] = decode
	return nil
}

type Signal struct{}

func builtinValues() map[project.ValueType]any {
	r := Registry{}
	addValue(&r, "", func(project.SlotValue) (Signal, error) { return Signal{}, nil })
	addValue(&r, project.StringValue, func(v project.SlotValue) (string, error) { return v.String, nil })
	addValue(&r, project.BoolValue, func(v project.SlotValue) (bool, error) { return v.Bool, nil })
	addValue(&r, project.IntValue, func(v project.SlotValue) (int64, error) { return v.Integer, nil })
	addValue(&r, project.NumberValue, func(v project.SlotValue) (float64, error) { return v.Number, nil })
	addValue(&r, project.Vec2Value, func(v project.SlotValue) ([2]float32, error) { return [2]float32{v.Vector[0], v.Vector[1]}, nil })
	addValue(&r, project.Vec3Value, func(v project.SlotValue) ([3]float32, error) {
		return [3]float32{v.Vector[0], v.Vector[1], v.Vector[2]}, nil
	})
	addValue(&r, project.ColorValue, func(v project.SlotValue) ([4]float32, error) { return v.Vector, nil })
	addValue(&r, "SceneSwitchRequest", func(v project.SlotValue) (SceneSwitchRequest, error) {
		if len(v.Fields) != 4 || v.Fields[0].Name != "overlay" || v.Fields[1].Name != "retain" || v.Fields[2].Name != "scene" || v.Fields[3].Name != "startup" {
			return SceneSwitchRequest{}, ErrType
		}
		return SceneSwitchRequest{Scene: project.SceneID(v.Fields[2].Value.String), Retain: v.Fields[1].Value.Bool, Overlay: v.Fields[0].Value.Bool, Startup: v.Fields[3].Value}, nil
	})
	addValue[TextRenderRequest](&r, "TextRenderRequest", nil)
	addValue[SceneStartup](&r, "SceneStartup", nil)
	addValue[SceneSwitchResult](&r, "SceneSwitchResult", nil)
	addValue[Collision2D](&r, "Collision2D", nil)
	addValue[Collision3D](&r, "Collision3D", nil)
	addValue[RenderOutputHandle](&r, project.VisualValue, nil)
	addValue[TextRenderResult](&r, "TextRenderResult", nil)
	return r.values
}

func addValue[T any](r *Registry, name project.ValueType, decode func(project.SlotValue) (T, error)) {
	if r.values == nil {
		r.values = make(map[project.ValueType]any)
	}
	r.values[name] = decode
}
