package module

import (
	"fmt"
	"io/fs"
	"reflect"
	"slices"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	htmlui "github.com/diiyw/onix/ui/html"
)

// Context deliberately exposes neither the global World nor foreign entities.
// One stable context is allocated at mount and reused for every callback.
type Context struct {
	runtime                                                          *Runtime
	scene                                                            *Scene
	definition                                                       *project.ModPlan
	identity                                                         uint64
	name                                                             string
	index, end                                                       int
	ui, visible, enabled, live, installing, disposing, disposeQueued bool
	root, placement                                                  ecs.Entity
	entities                                                         []ecs.Entity
	ownedEntities                                                    []ecs.Entity
	entitiesDirty                                                    bool
	orderDirty                                                       bool
	elements                                                         map[string]ecs.Entity
	resources                                                        *privateResources
	managed                                                          []ManagedResource
	slots                                                            []slotState
	message                                                          func(Message) error
	delivery                                                         uint64
	update                                                           func(Tick) error
	frameUpdate                                                      func(Tick) error
	destroy                                                          func() error
	enter, resume                                                    func(SceneStartup) error
	exit, suspend                                                    func() error
	pausedVisibility                                                 map[ecs.Entity]scene.Visibility
	content                                                          *Content
	nodeBundle                                                       ecs.PreparedBundle
	nodePrepared                                                     bool
}

type entityOwner struct {
	context *Context
}

// Entity is an ownership-bound handle; it cannot be converted into an ECS ID.
type Entity struct {
	owner  *Context
	entity ecs.Entity
}

func (c *Context) dirtyDrawOrder() {
	c.orderDirty = true
	c.runtime.orderDirty = true
}

func (c *Context) Generation() uint64 { return c.identity }
func (c *Context) Root() Entity       { return Entity{c, c.root} }

// Element resolves only authored private content inside this module.
func (c *Context) Element(name string) (Entity, error) {
	if !c.usable() {
		return Entity{}, ErrDisposed
	}
	e := c.elements[name]
	if e == 0 || !c.runtime.world.IsAlive(e) {
		return Entity{}, fmt.Errorf("module: private element %q does not exist", name)
	}
	return Entity{c, e}, nil
}

// Resource returns a typed, privately declared resource installed by a feature.
func (c *Context) Resource[T any](kind, name string) (T, error) {
	if !c.usable() || c.resources == nil {
		var zero T
		return zero, ErrDisposed
	}
	return c.resources.store.LoadNamedResource[T](kind, name)
}
func (c *Context) usable() bool { return c != nil && c.live && !c.disposing && !c.runtime.closed }
func (c *Context) running() bool {
	return c.usable() && (c.scene.state == SceneActive || c.runtime.preparation != nil && c.runtime.preparation.scene == c.scene)
}
func (c *Context) owns(e Entity) bool {
	return c != nil && c.live && e.owner == c && e.entity != 0 && (c.runtime.privateSpawns[e.entity].owner == c || c.runtime.entityContext(e.entity) == c)
}

func protectedComponent[T any]() bool {
	t := reflect.TypeFor[T]()
	return t == reflect.TypeFor[scene.PainterGroup2D]() || t == reflect.TypeFor[scene.UITargetCamera]() || t == reflect.TypeFor[scene.InputHandler2D]() || t == reflect.TypeFor[scene.Activity]() || t == reflect.TypeFor[scene.Parent]() || t == reflect.TypeFor[entityOwner]() || t == reflect.TypeFor[scene.SiblingOrder]() || t == reflect.TypeFor[scene.UIStackIndex]() || t == reflect.TypeFor[scene.RenderOrder2D]() || t == reflect.TypeFor[scene.RenderOutput2D]()
}

func (c *Context) Get[T any](e Entity) (T, bool) {
	if !c.owns(e) || protectedComponent[T]() {
		var zero T
		return zero, false
	}
	return c.runtime.world.Get[T](e.entity)
}

// Set replaces existing components immediately; archetype changes are commands
// applied after the batch/update traversal. Transform writes affect local content.
func (c *Context) Set[T any](e Entity, value T) error {
	if c != nil && c.disposing {
		return ErrDisposed
	}
	if !c.owns(e) || protectedComponent[T]() {
		return ErrOwnership
	}
	return c.insertPrivate(e.entity, value)
}

func (c *Context) insertPrivate[T any](entity ecs.Entity, value T) error {
	if !c.usable() {
		return ErrDisposed
	}
	if c.runtime.busy {
		if current := c.runtime.world.GetMut[T](entity); current != nil {
			*current = value
			return nil
		}
		if err := c.runtime.claimPrivateCommand(); err != nil {
			return err
		}
		c.runtime.world.Commands().Insert(entity, value)
		return nil
	}
	return c.runtime.world.Insert(entity, value)
}

func (c *Context) Remove[T any](e Entity) error {
	if c != nil && c.disposing {
		return ErrDisposed
	}
	if !c.owns(e) || protectedComponent[T]() {
		return ErrOwnership
	}
	if c.runtime.busy {
		if err := c.runtime.claimPrivateCommand(); err != nil {
			return err
		}
		c.runtime.world.Commands().Remove[T](e.entity)
	} else {
		c.runtime.world.Remove[T](e.entity)
	}
	return nil
}

// Each visits this module's private entity list only. Values are read copies;
// returning false stops iteration. Entities spawned during traversal wait until
// the boundary before becoming visible to queries.
func (c *Context) Each[T any](visit func(Entity, T) bool) (err error) {
	if c == nil || !c.live || visit == nil || protectedComponent[T]() {
		return ErrInvalid
	}
	boundary := !c.runtime.busy
	if boundary {
		c.runtime.busy = true
		defer func() {
			c.runtime.busy = false
			if c.runtime.loading {
				err = joinError(err, c.runtime.world.FlushCommands())
				c.runtime.privateCommands = 0
				clear(c.runtime.privateSpawns)
			} else {
				err = joinError(err, c.runtime.flushStructural())
			}
		}()
	}
	length := len(c.entities)
	for i := 0; i < length; i++ {
		entity := c.entities[i]
		if value, ok := c.runtime.world.Get[T](entity); ok && !visit(Entity{c, entity}, value) {
			return
		}
	}
	return nil
}

func (c *Context) Spawn(parent Entity) (Entity, error) {
	if !c.usable() || c.definition == nil {
		return Entity{}, ErrDisposed
	}
	if parent == (Entity{}) {
		parent = c.Root()
	}
	if !c.owns(parent) {
		return Entity{}, ErrOwnership
	}
	var entity ecs.Entity
	if c.runtime.busy {
		if err := c.runtime.claimPrivateCommand(); err != nil {
			return Entity{}, err
		}
		b := transformBundle(project.ModTransform{Scale: [3]float32{1, 1, 1}}, c.ui, parent.entity, len(c.entities), c.visible, c.enabled, nil)
		b.Put(entityOwner{context: c})
		entity = c.runtime.world.Commands().Spawn(b)
		c.runtime.privateSpawns[entity] = privateSpawn{owner: c, parent: parent.entity}
	} else {
		entity = c.runtime.spawnOwnedTransform(c, parent.entity, len(c.entities))
	}
	c.entities = append(c.entities, entity)
	c.dirtyDrawOrder()
	return Entity{c, entity}, nil
}

func (c *Context) Despawn(e Entity) error {
	if !c.owns(e) || e.entity == c.root {
		return ErrOwnership
	}
	// Slot roots belong to declared interfaces and live until module disposal.
	for _, slot := range c.slots {
		if slot.entity == e.entity {
			return ErrOwnership
		}
	}
	return c.runtime.command(structuralCommand{entity: e.entity, context: c})
}

func (c *Context) Reparent(child, parent Entity) error {
	if !c.usable() {
		return ErrDisposed
	}
	if !c.owns(child) || !c.owns(parent) || child.entity == c.root || child == parent {
		return ErrOwnership
	}
	for _, slot := range c.slots {
		if slot.entity == child.entity {
			return ErrOwnership
		}
	}
	for ancestor := parent.entity; ancestor != 0; ancestor = c.runtime.privateParent(ancestor) {
		if ancestor == child.entity {
			return fmt.Errorf("module: cyclic private hierarchy")
		}
	}
	return c.runtime.command(structuralCommand{context: c, entity: child.entity, parent: parent.entity})
}

func (c *Context) Dispose() error {
	if c == nil || !c.live || c.disposing || c.disposeQueued || c.runtime.closed {
		return nil
	}
	if c.definition == nil {
		return c.scene.Dispose()
	}
	if !c.runtime.commandSpace() {
		return ErrCommandFull
	}
	c.disposeQueued = true
	return c.runtime.command(structuralCommand{context: c})
}

// OpenResource accepts a declared private resource name, not an arbitrary path.
// The module chooses its file layout; no resources/ directory is imposed.
func (c *Context) OpenResource(name string) (fs.File, error) {
	if !c.usable() || c.definition == nil {
		return nil, ErrDisposed
	}
	if c.runtime.options.Source == nil {
		return nil, fmt.Errorf("module: resource source is not configured")
	}
	index, ok := slices.BinarySearchFunc(c.definition.Resources, name, func(p project.ModResourcePlan, name string) int { return compareName(p.Name, name) })
	if !ok {
		return nil, fmt.Errorf("module: resource %q is not declared", name)
	}
	return c.runtime.options.Source.Open(c.definition.Resources[index].Path)
}

// OpenFile serves inline component paths and other module-private files.
// Resolution uses the same ownership boundary as the project compiler.
func (c *Context) OpenFile(name string) (fs.File, error) {
	if !c.usable() || c.definition == nil {
		return nil, ErrDisposed
	}
	if c.runtime.options.Source == nil {
		return nil, fmt.Errorf("module: resource source is not configured")
	}
	path, err := project.ResolveModResourcePath(c.definition.ID, name)
	if err != nil {
		return nil, err
	}
	return c.runtime.options.Source.Open(path)
}

func compareName(a, b string) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// Input removes raw cross-module ECS identities from the engine input event.
type Input struct {
	Event    scene.InputEvent
	Target   Entity
	Element  string
	HTMLID   string
	HTMLKind htmlui.EventKind
	Value    string
}

func (r *Runtime) HandleInput(event scene.InputEvent) error {
	if r.closed {
		return ErrDisposed
	}
	if r.busy {
		return ErrBusy
	}
	if event.Entity == 0 {
		if event.Kind.Mask() == 0 || event.Kind.IsPointer() {
			return nil
		}
		event.Path = nil
		r.busy = true
		var err error
		for _, s := range r.scenes {
			if s.state == SceneActive && s.context.usable() {
				err = joinError(err, r.deliverInput(&s.context, Input{Event: event}))
			}
		}
		r.busy = false
		return joinError(err, r.flushStructural())
	}
	owner, ok := r.world.Get[entityOwner](event.Entity)
	for entity, remaining := event.Entity, r.world.EntityCount(); !ok && remaining > 0; remaining-- {
		parent, found := r.world.Get[scene.Parent](entity)
		if !found || parent.Entity == 0 {
			break
		}
		entity = parent.Entity
		owner, ok = r.world.Get[entityOwner](entity)
	}
	if !ok || !scene.IsInteractive(r.world, event.Entity) || !owner.context.usable() || !owner.context.enabled || owner.context.scene.state != SceneActive {
		return nil
	}
	c := owner.context
	target := Entity{c, event.Entity}
	htmlID := ""
	if r.content.controlID != nil {
		htmlID = r.content.controlID(r.world, event.Entity)
	}
	event.Entity = 0
	event.Path = nil
	element := ""
	if name, ok := r.world.Get[scene.Name](target.entity); ok {
		element = name.Value
	}
	return r.deliverInput(c, Input{Event: event, Target: target, Element: element, HTMLID: htmlID})
}
