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
	if c.content == nil {
		c.content = &Content{context: c, state: make(map[reflect.Type]any), input: input.NewState()}
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

func ContentGet[T any](c *Content, e ecs.Entity) (T, bool) {
	if !c.owns(e) {
		var zero T
		return zero, false
	}
	return ecs.Get[T](c.context.runtime.world, e)
}
func ContentMut[T any](c *Content, e ecs.Entity) *T {
	if !c.owns(e) || contentProtected[T]() {
		return nil
	}
	return ecs.GetMut[T](c.context.runtime.world, e)
}
func contentProtected[T any]() bool {
	t := reflect.TypeFor[T]()
	return t == reflect.TypeFor[scene.PainterGroup2D]() || t == reflect.TypeFor[entityOwner]() || t == reflect.TypeFor[scene.Parent]() || t == reflect.TypeFor[scene.InputHandler2D]() || t == reflect.TypeFor[scene.UITargetCamera]()
}
func ContentInsert[T any](c *Content, e ecs.Entity, value T) error {
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
	return insertPrivate(c.context, e, value)
}
func ContentResource[T any](c *Content) (T, bool) {
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
		state, ok := ecs.GetResource[*input.State](c.context.runtime.world)
		if !ok || state == nil {
			return zero, false
		}
		c.input.CopyFrom(state)
		return any(c.input).(T), true
	case *scene.Time:
		clock, ok := ecs.GetResource[*scene.Time](c.context.runtime.world)
		if !ok || clock == nil {
			return zero, false
		}
		c.clock = *clock
		return any(&c.clock).(T), true
	case scene.ViewportSize:
		v, ok := ecs.GetResource[scene.ViewportSize](c.context.runtime.world)
		return any(v).(T), ok
	}
	return zero, false
}
func ContentStore[T any](c *Content, value T) {
	t := reflect.TypeFor[T]()
	if slot, ok := c.state[t].(*T); ok {
		*slot = value
		return
	}
	c.state[t] = &value
}
func ContentResourceMut[T any](c *Content) (*T, bool) {
	value, ok := c.state[reflect.TypeFor[T]()].(*T)
	return value, ok
}
func ContentRemoveResource[T any](c *Content) (T, bool) {
	value, ok := ContentResource[T](c)
	delete(c.state, reflect.TypeFor[T]())
	return value, ok
}
func ContentQuery[T any](c *Content) iter.Seq2[ecs.Entity, ecs.ComponentAccess[T]] {
	return func(yield func(ecs.Entity, ecs.ComponentAccess[T]) bool) {
		if c == nil || !c.context.live || contentProtected[T]() || !c.queryAvailable() {
			return
		}
		boundary := c.beginQuery()
		defer c.endQuery(boundary)
		for e, access := range ecs.Query[T](c.context.runtime.world).In(c.context.ownedEntities).All {
			if !yield(e, access) {
				return
			}
		}
	}
}
func (c *Content) adopt(e ecs.Entity) ecs.Entity {
	w := c.context.runtime.world
	_ = ecs.Insert(w, e, entityOwner{context: c.context})
	if p, ok := ecs.Get[scene.Parent](w, e); !ok || p.Entity == 0 {
		_ = ecs.Insert(w, e, scene.Parent{Entity: c.context.root})
	}
	_ = ecs.Insert(w, e, scene.Activity{Active: c.context.enabled})
	c.context.entities = append(c.context.entities, e)
	c.context.dirtyDrawOrder()
	return e
}
func (c *Content) Spawn(bundle ecs.Bundle) ecs.Entity {
	if !c.context.usable() {
		panic(ErrDisposed)
	}
	if _, grouped := ecs.BundleValue[scene.PainterGroup2D](bundle); grouped {
		panic(ErrOwnership)
	}
	parent, hasParent := ecs.BundleValue[scene.Parent](bundle)
	if parent.Entity != 0 && !c.owns(parent.Entity) {
		panic(ErrOwnership)
	}
	if !hasParent {
		ecs.Put(&bundle, scene.Parent{Entity: c.context.root})
	}
	ecs.Put(&bundle, entityOwner{context: c.context})
	_, hasActivity := ecs.BundleValue[scene.Activity](bundle)
	if !hasActivity {
		ecs.Put(&bundle, scene.Activity{Active: c.context.enabled})
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
		entity = ecs.CommandSpawn(r.world.Commands(), bundle)
		if hasParent && parent.Entity == 0 {
			ecs.CommandInsert(r.world.Commands(), entity, scene.Parent{Entity: c.context.root})
		}
		if hasActivity {
			ecs.CommandInsert(r.world.Commands(), entity, scene.Activity{Active: c.context.enabled})
		}
		if parent.Entity == 0 {
			parent.Entity = c.context.root
		}
		r.privateSpawns[entity] = privateSpawn{owner: c.context, parent: parent.Entity}
	} else {
		entity = r.world.Spawn(bundle)
		if hasParent && parent.Entity == 0 {
			_ = ecs.Insert(r.world, entity, scene.Parent{Entity: c.context.root})
		}
		if hasActivity {
			_ = ecs.Insert(r.world, entity, scene.Activity{Active: c.context.enabled})
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
func ContentMustMut[T any](c *Content, e ecs.Entity) *T {
	value := ContentMut[T](c, e)
	if value == nil {
		panic(fmt.Sprintf("module: private entity %d has no writable %s", e, reflect.TypeFor[T]()))
	}
	return value
}
