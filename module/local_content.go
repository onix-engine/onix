package module

import (
	"fmt"
	"io"
	"iter"
	"reflect"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/text"
)

// Content confines procedural UI and model operations to one module. ECS IDs
// returned by UI libraries are checked against ownership at every access.
type Content struct {
	context *Context
	state   map[reflect.Type]any
	input   *input.State
	clock   scene.Time
}

func (c *Context) Content() *Content {
	if c == nil || !c.live {
		return nil
	}
	if c.content == nil {
		c.content = &Content{context: c}
	}
	return c.content
}

func (c *Content) owns(e ecs.Entity) bool {
	if c == nil || c.context == nil || !c.context.live || e == 0 {
		return false
	}
	return c.context.runtime.privateSpawns[e].owner == c.context || c.context.runtime.entityContext(e) == c.context
}

func (c *Content) IsAlive(e ecs.Entity) bool { return c.owns(e) && c.context.runtime.world.IsAlive(e) }
func (c *Content) Root() ecs.Entity          { return c.context.root }
func (c *Content) Element(id string) (ecs.Entity, error) {
	e, err := c.context.Element(id)
	return e.entity, err
}
func (c *Content) MustElement(id string) ecs.Entity {
	e, err := c.Element(id)
	if err != nil {
		panic(err)
	}
	return e
}
func (c *Content) Font(name string) (*text.Font, error) { return c.context.resources.store.Font(name) }
func (c *Content) ModelRig(name string) (*project.ModelRig, error) {
	return c.context.resources.store.ModelRig(name)
}

func (c *Content) ReadFile(name string) ([]byte, error) {
	file, err := c.context.OpenFile(name)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(file)
	closeErr := file.Close()
	if err != nil {
		return nil, err
	}
	return data, closeErr
}

func (c *Content) Get[T any](e ecs.Entity) (T, bool) {
	if !c.owns(e) {
		var zero T
		return zero, false
	}
	return c.context.runtime.world.Get[T](e)
}
func (c *Content) Mut[T any](e ecs.Entity) *T {
	if !c.owns(e) || contentProtected[T]() {
		return nil
	}
	return c.context.runtime.world.GetMut[T](e)
}
func contentProtected[T any]() bool {
	t := reflect.TypeFor[T]()
	return t == reflect.TypeFor[scene.PainterGroup2D]() || t == reflect.TypeFor[entityOwner]() || t == reflect.TypeFor[scene.Parent]() || t == reflect.TypeFor[scene.InputHandler2D]() || t == reflect.TypeFor[scene.UITargetCamera]()
}
func (c *Content) Insert[T any](e ecs.Entity, value T) error {
	if !c.owns(e) {
		return ErrOwnership
	}
	if parent, ok := any(value).(scene.Parent); ok {
		if e == c.context.root || !c.owns(parent.Entity) {
			return ErrOwnership
		}
		return c.context.Reparent(Entity{c.context, e}, Entity{c.context, parent.Entity})
	}
	if contentProtected[T]() {
		return ErrOwnership
	}
	return c.context.insertPrivate(e, value)
}
func (c *Content) Resource[T any]() (T, bool) {
	var zero T
	if c == nil || !c.context.live {
		return zero, false
	}
	if slot, ok := c.state[reflect.TypeFor[T]()].(*T); ok {
		return *slot, true
	}
	// Platform input is copied into reusable private storage, so release and
	// consumption in one module cannot change another module's input state.
	switch any(zero).(type) {
	case *input.State:
		state, ok := c.context.runtime.world.GetResource[*input.State]()
		if !ok || state == nil {
			return zero, false
		}
		if c.input == nil {
			c.input = input.NewState()
		}
		c.input.CopyFrom(state)
		return any(c.input).(T), true
	case *scene.Time:
		clock, ok := c.context.runtime.world.GetResource[*scene.Time]()
		if !ok || clock == nil {
			return zero, false
		}
		c.clock = *clock
		return any(&c.clock).(T), true
	case scene.ViewportSize:
		v, ok := c.context.runtime.world.GetResource[scene.ViewportSize]()
		return any(v).(T), ok
	}
	return zero, false
}
func (c *Content) Store[T any](value T) {
	if c == nil || !c.context.live {
		return
	}
	if c.state == nil {
		c.state = make(map[reflect.Type]any)
	}
	t := reflect.TypeFor[T]()
	if slot, ok := c.state[t].(*T); ok {
		*slot = value
		return
	}
	c.state[t] = &value
}
func (c *Content) ResourceMut[T any]() (*T, bool) {
	if c == nil || !c.context.live {
		return nil, false
	}
	value, ok := c.state[reflect.TypeFor[T]()].(*T)
	return value, ok
}
func (c *Content) RemoveResource[T any]() (T, bool) {
	value, ok := c.Resource[T]()
	delete(c.state, reflect.TypeFor[T]())
	return value, ok
}
func (c *Content) Query[T any]() iter.Seq2[ecs.Entity, ecs.ComponentAccess[T]] {
	return func(yield func(ecs.Entity, ecs.ComponentAccess[T]) bool) {
		if c == nil || !c.context.live || contentProtected[T]() || !c.queryAvailable() {
			return
		}
		boundary := c.beginQuery()
		defer c.endQuery(boundary)
		for e, access := range c.context.runtime.world.Query[T]().In(c.context.ownedEntities).All {
			if !yield(e, access) {
				return
			}
		}
	}
}
func (c *Content) adopt(e ecs.Entity) ecs.Entity {
	w := c.context.runtime.world
	_ = w.Insert(e, entityOwner{context: c.context})
	if w.Has[scene.UIStackIndex](e) || w.Has[scene.RenderOrder2D](e) {
		_ = w.Insert(e, explicitPainterOrder{})
	}
	if p, ok := w.Get[scene.Parent](e); !ok || p.Entity == 0 {
		_ = w.Insert(e, scene.Parent{Entity: c.context.root})
	}
	_ = w.Insert(e, scene.Activity{Active: c.context.enabled})
	c.context.entities = append(c.context.entities, e)
	c.context.dirtyDrawOrder()
	return e
}
func (c *Content) Spawn(bundle ecs.Bundle) ecs.Entity {
	if !c.context.usable() {
		panic(ErrDisposed)
	}
	if _, grouped := bundle.Value[scene.PainterGroup2D](); grouped {
		panic(ErrOwnership)
	}
	_, uiOrder := bundle.Value[scene.UIStackIndex]()
	_, worldOrder := bundle.Value[scene.RenderOrder2D]()
	if uiOrder || worldOrder {
		bundle.Put(explicitPainterOrder{})
	}
	parent, hasParent := bundle.Value[scene.Parent]()
	if parent.Entity != 0 && !c.owns(parent.Entity) {
		panic(ErrOwnership)
	}
	if !hasParent {
		bundle.Put(scene.Parent{Entity: c.context.root})
	}
	bundle.Put(entityOwner{context: c.context})
	_, hasActivity := bundle.Value[scene.Activity]()
	if !hasActivity {
		bundle.Put(scene.Activity{Active: c.context.enabled})
	}
	r := c.context.runtime
	var entity ecs.Entity
	if r.busy {
		count := 1
		if hasParent && parent.Entity == 0 {
			count++
		}
		if hasActivity {
			count++
		}
		if len(r.commands)+r.privateCommands+count > r.options.CommandCapacity {
			panic(ErrCommandFull)
		}
		r.privateCommands += count
		entity = r.world.Commands().Spawn(bundle)
		if hasParent && parent.Entity == 0 {
			r.world.Commands().Insert(entity, scene.Parent{Entity: c.context.root})
		}
		if hasActivity {
			r.world.Commands().Insert(entity, scene.Activity{Active: c.context.enabled})
		}
		if parent.Entity == 0 {
			parent.Entity = c.context.root
		}
		r.privateSpawns[entity] = privateSpawn{owner: c.context, parent: parent.Entity}
	} else {
		entity = r.world.Spawn(bundle)
		if hasParent && parent.Entity == 0 {
			_ = r.world.Insert(entity, scene.Parent{Entity: c.context.root})
		}
		if hasActivity {
			_ = r.world.Insert(entity, scene.Activity{Active: c.context.enabled})
		}
	}
	c.context.entities = append(c.context.entities, entity)
	c.context.dirtyDrawOrder()
	return entity
}

func (c *Content) queryAvailable() bool {
	r := c.context.runtime
	return c.context.scene.state == SceneActive || r.preparation != nil && r.preparation.scene == c.context.scene
}
func (c *Content) beginQuery() bool {
	r := c.context.runtime
	if r.queryDepth == 0 {
		r.SyncScopes()
	}
	boundary := !r.busy
	r.busy = true
	r.queryDepth++
	return boundary
}
func (c *Content) endQuery(boundary bool) {
	r := c.context.runtime
	r.queryDepth--
	if !boundary {
		return
	}
	r.busy = false
	var err error
	if r.loading {
		err = r.world.FlushCommands()
		r.privateCommands = 0
		clear(r.privateSpawns)
	} else {
		err = r.flushStructural()
	}
	if err != nil {
		panic(err)
	}
}

func (c *Content) Despawn(e ecs.Entity) bool {
	if !c.owns(e) || e == c.context.root {
		return false
	}
	return c.context.Despawn(Entity{c.context, e}) == nil
}
func (c *Content) RefreshTransforms() error {
	c.context.runtime.SyncScopes()
	w := c.context.runtime.world
	if err := scene.SyncGlobalTransformSystem("").Run(w); err != nil {
		return err
	}
	return scene.SyncUIGlobalTransformSystem("").Run(w)
}
func (c *Content) MustMut[T any](e ecs.Entity) *T {
	value := c.Mut[T](e)
	if value == nil {
		panic(fmt.Sprintf("module: private entity %d has no writable %s", e, reflect.TypeFor[T]()))
	}
	return value
}
