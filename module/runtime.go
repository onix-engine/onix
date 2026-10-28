package module

import (
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"slices"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/physics"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	htmlui "github.com/diiyw/onix/ui/html"
)

type Options struct {
	EventCapacity int
	// PayloadCapacity is the initial capacity per payload type and bank.
	// Zero selects min(8, EventCapacity); storage grows on demand while
	// EventCapacity continues to bound the total event queue.
	PayloadCapacity   int
	RequestCapacity   int
	CommandCapacity   int
	TimeoutTicks      uint64
	OutputCapacity    int
	TextCacheCapacity int
	StartupBatches    int
	StartupEvents     int
	Source            fs.FS
	Resources         project.ResourceFeature
	Wake              func()
	Host              *Host
	// Content installs additional private rendering/animation capabilities.
	// The built-in loader handles empty modules and color sprites.
	Content func(*Context, project.ModComponents) error
}

// Runtime is single-threaded. Plans and registration are immutable for its life.
type Runtime struct {
	world                     *ecs.World
	plan                      *project.ProjectPlan
	options                   Options
	registry                  Registry
	typedStores               map[reflect.Type]valueStore
	nextDelivery              uint64
	inputStore                *store[Input]
	usedStores                [4][]valueStore
	usedSlots                 [4][]*slotBuffer
	scenes                    []*Scene
	queues                    [2][]envelope
	slotBuffers               map[project.ValueType]*slotBuffer
	write                     uint8
	tick                      uint64
	nextIdentity              uint64
	nextRequest               RequestID
	pending                   []pendingRequest
	freePending               []int
	pendingFirst, pendingLast int
	commands                  []structuralCommand
	busy, closed              bool
	queryDepth                int
	despawnEntities           []ecs.Entity
	dirtyEntities             []*Context
	loading                   bool
	privateCommands           int
	privateSpawns             map[ecs.Entity]privateSpawn
	orderDirty                bool
	disposedDirty             bool
	manager                   SceneManager
	outputs                   *scene.RenderOutputPool
	fonts                     map[[32]byte]*fontResource
	contentResources          map[project.ModID]*privateResources
	preparedContent           *preparedSceneContent
	collisions2D              ecs.MessageReader[physics.CollisionMessage2D]
	collisions3D              ecs.MessageReader[physics.CollisionMessage3D]
	preparation               *preparation
	prepareWorkspace          preparation
	runningScope              ecs.QueryScope
	renderScope               ecs.QueryScope
	runningEntities           []ecs.Entity
	runningUpdates            []*Context
	runningFrameUpdates       []*Context
	renderEntities            []ecs.Entity
	scopeNodes                []sceneScopeNode
	scopeHierarchy            uint64
	scopeOwners               uint64
	scopeEntities             uint64
	scopeDirty                bool
	renderBackground          bool
}

type Scene struct {
	uploads           graphics.UploadQueue
	runtime           *Runtime
	plan              *project.ScenePlan
	context           Context
	modules           []Context
	instances         map[string]int
	uiRoot, worldRoot ecs.Entity
	disposeQueued     bool
	state             SceneState
	rendered          bool
	foreground        bool
	layer             scene.SceneLayer
	startup           SceneStartup
	entities          []ecs.Entity
}

func New(world *ecs.World, plan *project.ProjectPlan, registry Registry, options Options) (*Runtime, error) {
	if world == nil || plan == nil || int(plan.Entry) >= len(plan.Scenes) {
		return nil, ErrInvalid
	}
	if options.EventCapacity == 0 {
		options.EventCapacity = 256
	}
	if options.PayloadCapacity == 0 {
		options.PayloadCapacity = min(8, options.EventCapacity)
	}
	if options.RequestCapacity == 0 {
		options.RequestCapacity = 128
	}
	if options.CommandCapacity == 0 {
		options.CommandCapacity = 128
	}
	if options.TimeoutTicks == 0 {
		options.TimeoutTicks = 120
	}
	if options.OutputCapacity == 0 {
		options.OutputCapacity = 512
	}
	if options.TextCacheCapacity == 0 {
		options.TextCacheCapacity = 128
	}
	if options.StartupBatches == 0 {
		options.StartupBatches = 32
	}
	if options.StartupEvents == 0 {
		options.StartupEvents = 4096
	}
	if options.EventCapacity < 1 || options.PayloadCapacity < 1 || options.PayloadCapacity > options.EventCapacity || options.RequestCapacity < 1 || options.CommandCapacity < 1 || options.OutputCapacity < 1 || options.TextCacheCapacity < 1 || options.StartupBatches < 1 || options.StartupEvents < 1 {
		return nil, ErrInvalid
	}
	r := &Runtime{world: world, plan: plan, options: options, pendingFirst: -1, pendingLast: -1,
		typedStores: make(map[reflect.Type]valueStore), pending: make([]pendingRequest, options.RequestCapacity),
		freePending: make([]int, options.RequestCapacity), commands: make([]structuralCommand, 0, options.CommandCapacity)}
	r.slotBuffers = make(map[project.ValueType]*slotBuffer)
	r.privateSpawns = make(map[ecs.Entity]privateSpawn, options.CommandCapacity)
	r.dirtyEntities = make([]*Context, 0, options.CommandCapacity)
	r.outputs = scene.NewRenderOutputPool(options.OutputCapacity)
	r.fonts = make(map[[32]byte]*fontResource)
	r.contentResources = make(map[project.ModID]*privateResources)
	r.manager = SceneManager{runtime: r, switches: make([]switchCommand, 0, options.CommandCapacity)}
	// Registration can subsequently be edited without mutating this runtime.
	r.registry.mods = cloneMap(registry.mods)
	r.registry.scenes = cloneMap(registry.scenes)
	r.registry.values = builtinValues()
	for name, factory := range registry.values {
		r.registry.values[name] = factory
	}
	for i := range r.freePending {
		r.freePending[i] = options.RequestCapacity - 1 - i
	}
	for i := range 2 {
		r.queues[i] = make([]envelope, 0, options.EventCapacity)
	}
	r.inputStore = &store[Input]{values: [4][]Input{make([]Input, 1), nil}}
	world.Commands().Reserve(options.CommandCapacity)
	return r, nil
}

// Each registered type appears at most once per bank. Reserve on registration
// so enqueue needs no allocation for bookkeeping, even on its first use.
func reserveUsedBuffers[T any](buffers *[4][]T, count int) {
	for i := range buffers {
		buffers[i] = slices.Grow(buffers[i], count-len(buffers[i]))
	}
}

func cloneMap[K comparable, V any](source map[K]V) map[K]V {
	result := make(map[K]V, len(source))
	for k, v := range source {
		result[k] = v
	}
	return result
}

// Foreground and scene preparation use separate payload banks.
func (r *Runtime) buffer() uint8 {
	if r.preparation != nil {
		return r.write + 2
	}
	return r.write
}

func (r *Runtime) resetBuffer(buffer uint8) {
	for _, storage := range r.usedStores[buffer] {
		storage.truncate(buffer, 0)
	}
	clear(r.usedStores[buffer])
	r.usedStores[buffer] = r.usedStores[buffer][:0]
	for _, slots := range r.usedSlots[buffer] {
		slots.truncate(buffer, 0)
	}
	clear(r.usedSlots[buffer])
	r.usedSlots[buffer] = r.usedSlots[buffer][:0]
}

func (r *Runtime) initContext(c *Context, s *Scene, mod *project.ModPlan, enabled bool) {
	r.nextIdentity++
	if r.nextIdentity == 0 {
		panic("module: instance identity exhausted")
	}
	*c = Context{runtime: r, scene: s, definition: mod, identity: r.nextIdentity, live: true, enabled: enabled, orderDirty: mod != nil}
}

func (r *Runtime) MountEntry() (*Scene, error) { return r.Mount(r.plan.Spec.Entry) }

// Mount is a loading operation; event callbacks enqueue disposal, never mount
// new scenes while a frozen batch is being traversed.
func (r *Runtime) Mount(id project.SceneID) (*Scene, error) {
	return r.mount(id, SceneActive)
}

func (r *Runtime) mount(id project.SceneID, state SceneState) (*Scene, error) {
	if r.closed {
		return nil, ErrDisposed
	}
	if r.busy || r.loading || r.manager.preload != nil && r.manager.preload.request.Scene == id && r.preparedContent == nil {
		return nil, ErrBusy
	}
	index := slices.IndexFunc(r.plan.Scenes, func(p project.ScenePlan) bool { return p.ID == id })
	if index < 0 {
		return nil, fmt.Errorf("scene %q is not registered", id)
	}
	p := &r.plan.Scenes[index]
	s := &Scene{runtime: r, plan: p, modules: make([]Context, len(p.Instances)), instances: make(map[string]int, len(p.Instances)), state: state}
	if r.preparation != nil {
		r.preparation.scene = s
	}
	s.rendered = state == SceneActive
	s.layer = scene.SceneLayer{ID: r.nextIdentity + 1, Order: r.nextIdentity + 1}
	r.scopeDirty = true
	r.loading = true
	queueLength := len(r.queues[r.write])
	buffer := r.buffer()
	storeCount, slotCount := len(r.usedStores[buffer]), len(r.usedSlots[buffer])
	lengths := make([]int, storeCount)
	slotLengths := make([]int, slotCount)
	for i, storage := range r.usedStores[buffer] {
		lengths[i] = storage.length(buffer)
	}
	for i, slots := range r.usedSlots[buffer] {
		slotLengths[i] = slots.lengths[buffer]
	}
	fail := func(err error) (*Scene, error) {
		r.loading = false
		clear(r.queues[r.write][queueLength:])
		r.queues[r.write] = r.queues[r.write][:queueLength]
		for i, storage := range r.usedStores[buffer] {
			length := 0
			if i < storeCount {
				length = lengths[i]
			}
			storage.truncate(buffer, length)
		}
		for i, slots := range r.usedSlots[buffer] {
			length := 0
			if i < slotCount {
				length = slotLengths[i]
			}
			slots.truncate(buffer, length)
		}
		clear(r.usedStores[buffer][storeCount:])
		r.usedStores[buffer] = r.usedStores[buffer][:storeCount]
		clear(r.usedSlots[buffer][slotCount:])
		r.usedSlots[buffer] = r.usedSlots[buffer][:slotCount]
		cleanup := r.disposeScene(s)
		r.pruneDisposed()
		r.commands = slices.DeleteFunc(r.commands, func(command structuralCommand) bool {
			return command.scene == s || command.context != nil && command.context.scene == s
		})
		return nil, errors.Join(err, cleanup)
	}
	r.initContext(&s.context, s, nil, true)
	r.scenes = append(r.scenes, s)
	s.uiRoot = r.spawnTransform(p.Transform, true, 0, 0, true)
	s.worldRoot = r.spawnTransform(p.Transform, false, 0, 0, true)
	_ = ecs.Insert(r.world, s.uiRoot, s.layer)
	_ = ecs.Insert(r.world, s.worldRoot, s.layer)
	for i, instance := range p.Instances {
		if int(instance.Mod) >= len(r.plan.Mods) || instance.Parent >= int32(i) || instance.Parent < -1 {
			return fail(ErrInvalid)
		}
		definition := &r.plan.Mods[instance.Mod]
		c := &s.modules[i]
		r.initContext(c, s, definition, instance.Enabled)
		c.name = instance.ID
		c.index = i
		c.end = i + 1
		c.ui = definition.Spec.Space != project.CoordinateSpaceWorld
		c.visible = instance.Visible
		parent := s.worldRoot
		if c.ui {
			parent = s.uiRoot
		}
		if instance.Parent >= 0 {
			parent = s.modules[instance.Parent].placement
		}
		// Child placement inherits placement, never its parent's local content.
		c.placement = r.spawnTransform(instance.Placement, c.ui, parent, i+1, instance.Visible)
		var err error
		c.root, err = r.spawnContent(c, definition.Transform, definition.Spec.Components, c.placement, 0, c.ui)
		if err != nil {
			return fail(fmt.Errorf("%s: content: %w", definition.ID, err))
		}
		c.entities = append(c.entities, c.root)
		_ = ecs.Insert(r.world, c.root, entityOwner{context: c})
		_ = ecs.Insert(r.world, c.root, scene.PainterGroup2D(c.identity))
		c.slots = make([]slotState, len(definition.Slots))
		if len(instance.Slots) != len(c.slots) {
			return fail(ErrInvalid)
		}
		for j, plan := range definition.Slots {
			entity := r.spawnTransform(plan.Placement, c.ui, c.root, j+1, instance.Visible)
			_ = ecs.Insert(r.world, entity, entityOwner{context: c})
			c.entities = append(c.entities, entity)
			c.slots[j] = slotState{value: cloneValue(instance.Slots[j]), revision: 1, entity: entity}
			if err := initVisualSlot(r.world, &c.slots[j]); err != nil {
				return fail(err)
			}
			_ = ecs.Insert(r.world, entity, scene.Activity{Active: c.enabled})
		}
		c.elements = make(map[string]ecs.Entity, len(definition.Elements)+len(definition.Slots))
		for j, slot := range definition.Slots {
			c.elements[slot.Name] = c.slots[j].entity
		}
		for j, element := range definition.Elements {
			ui := c.ui
			if element.Spec.Space != "" {
				ui = element.Spec.Space == project.CoordinateSpaceUI
			}
			if ui != c.ui {
				installAlternateTransform(r.world, c.root, definition.Transform, ui)
				installAlternateTransform(r.world, c.placement, instance.Placement, ui)
				installAlternateTransform(r.world, s.uiRoot, p.Transform, false)
				installAlternateTransform(r.world, s.worldRoot, p.Transform, true)
			}
			parent := c.root
			if element.Parent >= 0 {
				parent = c.elements[definition.Elements[element.Parent].Spec.ID]
			}
			entity, err := r.spawnContent(c, element.Transform, element.Spec.Components, parent, j+len(c.slots)+1, ui)
			if err != nil {
				return fail(fmt.Errorf("%s element %s: %w", definition.ID, element.Spec.ID, err))
			}
			_ = ecs.Insert(r.world, entity, entityOwner{context: c})
			_ = ecs.Insert(r.world, entity, scene.Name{Value: element.Spec.ID})
			if element.Spec.Visible != nil {
				_ = ecs.Insert(r.world, entity, scene.Visibility{Visible: c.visible && *element.Spec.Visible})
			}
			c.elements[element.Spec.ID] = entity
			c.entities = append(c.entities, entity)
		}
		s.instances[instance.ID] = i
	}
	for i := len(s.modules) - 1; i >= 0; i-- {
		if parent := p.Instances[i].Parent; parent >= 0 {
			s.modules[parent].end = max(s.modules[parent].end, s.modules[i].end)
		}
	}
	r.orderDirty = true
	r.SyncScopes()
	// All instances exist before Install so startup messages can address any module.
	for i := -1; i < len(s.modules); i++ {
		c := &s.context
		install := r.registry.scenes[id]
		if i == -1 && p.Spec.Behavior != "" && install == nil {
			return fail(fmt.Errorf("%s: behavior %s has no registration", id, p.Spec.Behavior))
		}
		if i >= 0 {
			c = &s.modules[i]
			install = r.registry.mods[c.definition.ID]
			if c.definition.Spec.Behavior != "" && install == nil {
				return fail(fmt.Errorf("%s: behavior %s has no registration", c.definition.ID, c.definition.Spec.Behavior))
			}
			if err := r.installContent(c); err != nil {
				return fail(err)
			}
		}
		c.installing = true
		if install != nil {
			if err := install(c); err != nil {
				c.installing = false
				return fail(fmt.Errorf("%s install: %w", c.name, err))
			}
		}
		c.installing = false
	}
	r.loading = false
	r.scopeDirty = true
	if err := r.flushStructural(); err != nil {
		return fail(err)
	}
	if !s.Alive() {
		return nil, ErrDisposed
	}
	if state == SceneActive && r.manager.active == nil {
		r.manager.active = s
	}
	return s, nil
}

func (r *Runtime) hasQueueSpace() bool {
	return !r.closed && len(r.queues[r.write]) < r.options.EventCapacity
}

// Dispatch freezes exactly one batch per logic tick. Replies/actions emitted by
// callbacks are in the other buffer and cannot recurse into this batch.
func (r *Runtime) Dispatch() error {
	if r.closed {
		return ErrDisposed
	}
	if r.busy {
		return ErrBusy
	}
	r.busy = true
	r.tick++
	read := r.write
	buffer := r.buffer()
	r.write ^= 1
	var result error
	for _, e := range r.queues[read] {
		if e.kind == slotChangeEvent {
			e.observer.queued = false
			if e.source.running() {
				result = joinError(result, e.observer.handler(e.source))
			}
			continue
		}
		if e.kind == slotEvent {
			if e.slot.context.running() {
				result = joinError(result, e.slot.set(e.slotBuffer.values[e.buffer][e.value], true))
			}
			continue
		}
		if e.kind == replyEvent {
			p := r.lookupPending(e.pending, e.id)
			if p == nil || p.failure != nil || !p.source.running() {
				continue
			}
			snapshot := *p
			r.releasePending(e.pending)
			if snapshot.revision.stale() {
				result = joinError(result, r.failRequest(snapshot, ErrStaleRevision))
			} else {
				result = joinError(result, e.store.deliver(snapshot.source, e))
			}
			continue
		}
		if !e.source.usable() || !e.source.enabled || !e.source.running() && !(e.kind == engineEvent && e.source.definition == nil && e.source.scene.state == SceneSuspended) {
			continue
		}
		if e.kind == requestEvent {
			p := r.lookupPending(e.pending, e.id)
			if p == nil || p.failure != nil || r.tick >= p.deadline || p.target != e.target {
				continue
			}
		}
		if e.target != nil && e.target.usable() && e.target.enabled && (e.target.running() || e.kind == engineEvent && e.target.definition == nil && e.target.scene.state == SceneSuspended) {
			result = joinError(result, e.store.deliver(e.target, e))
		}
	}
	clear(r.queues[read])
	r.queues[read] = r.queues[read][:0]
	r.resetBuffer(buffer)
	// The intrusive list preserves request issuance order without scanning free slots.
	for index := r.pendingFirst; index >= 0; {
		p := &r.pending[index]
		next := p.next
		if !p.source.usable() {
			r.releasePending(index)
		} else {
			if p.failure == nil && !p.replied && r.tick >= p.deadline {
				p.failure = ErrTimeout
				p.failureTick = r.tick
			}
			if p.failure != nil && p.failureTick <= r.tick {
				snapshot := *p
				r.releasePending(index)
				result = joinError(result, r.failRequest(snapshot, snapshot.failure))
			}
		}
		index = next
	}
	r.busy = false
	return joinError(result, r.flushStructural())
}

// Update visits only instances with an Update hook, using fixed-tick time.
func (r *Runtime) Update(delta float32) error {
	if r.closed {
		return ErrDisposed
	}
	if r.busy {
		return ErrBusy
	}
	r.busy = true
	var result error
	for _, c := range r.runningUpdates {
		if c.scene.state == SceneActive && c.usable() && c.enabled {
			result = joinError(result, c.update(Tick{r.tick, delta}))
		}
	}
	r.busy = false
	return joinError(result, r.flushStructural())
}

// UpdateFrame runs UI/application hooks once per awake frame, independently of
// fixed-tick accumulation. Callers must dispatch pending events first.
func (r *Runtime) UpdateFrame(delta float32) error {
	if r.closed {
		return ErrDisposed
	}
	if r.busy {
		return ErrBusy
	}
	r.busy = true
	var result error
	for _, c := range r.runningFrameUpdates {
		if c.scene.state == SceneActive && c.usable() && c.enabled {
			result = joinError(result, c.frameUpdate(Tick{r.tick, delta}))
		}
	}
	r.busy = false
	return joinError(result, r.flushStructural())
}

func (r *Runtime) lookupPending(index int, id RequestID) *pendingRequest {
	if index < 0 || index >= len(r.pending) || r.pending[index].id != id || id == 0 {
		return nil
	}
	return &r.pending[index]
}

func (r *Runtime) Cancel(id RequestID) error {
	if id == 0 {
		return ErrCanceled
	}
	p := r.lookupPending(int(id%RequestID(len(r.pending))), id)
	if p == nil {
		return ErrCanceled
	}
	if p.failure != nil {
		return ErrCanceled
	}
	p.failure = ErrCanceled
	p.failureTick = r.tick + 1
	if r.options.Wake != nil {
		r.options.Wake()
	}
	return nil
}

func (r *Runtime) releasePending(index int) {
	p := &r.pending[index]
	if p.previous >= 0 {
		r.pending[p.previous].next = p.next
	} else {
		r.pendingFirst = p.next
	}
	if p.next >= 0 {
		r.pending[p.next].previous = p.previous
	} else {
		r.pendingLast = p.previous
	}
	*p = pendingRequest{}
	r.freePending = append(r.freePending, index)
}

func (r *Runtime) Pending() int { return len(r.pending) - len(r.freePending) }
func (r *Runtime) Queued() int  { return len(r.queues[r.write]) }
func (r *Runtime) NeedsTick() bool {
	if r.Queued() > 0 || r.Pending() > 0 || len(r.manager.switches) > 0 {
		return true
	}
	for _, c := range r.runningUpdates {
		if c.scene.state == SceneActive {
			return true
		}
	}
	return false
}
func (s *Scene) ID() project.SceneID { return s.plan.ID }
func (s *Scene) Generation() uint64  { return s.context.identity }
func (s *Scene) Alive() bool         { return s != nil && s.context.usable() }

// SceneEmitter sends messages to a scene from the application orchestrator.
// A module never receives this Scene object through Context.
func SceneEmitter[T any](s *Scene, name string) (Emitter[T], error) {
	if s == nil {
		return Emitter[T]{}, ErrInvalid
	}
	return BindEmitter[T](&s.context, name)
}

func joinError(a, b error) error {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	return errors.Join(a, b)
}

type structuralCommand struct {
	context *Context
	scene   *Scene
	entity  ecs.Entity
	parent  ecs.Entity
}

type privateSpawn struct {
	owner  *Context
	parent ecs.Entity
}

func (r *Runtime) privateParent(entity ecs.Entity) ecs.Entity {
	for i := len(r.commands) - 1; i >= 0; i-- {
		if command := r.commands[i]; command.entity == entity && command.parent != 0 {
			return command.parent
		}
	}
	if spawn, ok := r.privateSpawns[entity]; ok {
		return spawn.parent
	}
	parent, _ := ecs.Get[scene.Parent](r.world, entity)
	return parent.Entity
}

func (r *Runtime) command(command structuralCommand) error {
	if !r.commandSpace() {
		return ErrCommandFull
	}
	r.commands = append(r.commands, command)
	if !r.busy && !r.loading {
		return r.flushStructural()
	}
	return nil
}

func (s *Scene) Dispose() error {
	if s == nil || !s.context.live || s.context.disposing || s.disposeQueued || s.runtime.closed {
		return nil
	}
	if !s.runtime.commandSpace() {
		return ErrCommandFull
	}
	s.disposeQueued = true
	return s.runtime.command(structuralCommand{scene: s})
}

func (r *Runtime) flushStructural() error {
	var result error
	if err := r.world.FlushCommands(); err != nil {
		result = err
	}
	r.privateCommands = 0
	clear(r.privateSpawns)
	if len(r.commands) != 0 {
		r.SyncScopes()
	}

	r.busy = true // Destroy hooks may enqueue more changes; never recurse.
	for i := 0; i < len(r.commands); i++ {
		command := r.commands[i]
		if command.scene != nil {
			result = joinError(result, r.disposeScene(command.scene))

		} else if command.parent != 0 {
			result = joinError(result, r.reparentPrivate(command))
		} else if command.entity != 0 {
			r.despawnPrivate(command.context, command.entity)
		} else {
			c := command.context
			for j := c.end - 1; j >= c.index; j-- {
				result = joinError(result, r.disposeContext(&c.scene.modules[j]))
			}

		}
	}
	clear(r.commands)
	r.commands = r.commands[:0]
	for _, c := range r.dirtyEntities {
		c.entities = slices.DeleteFunc(c.entities, func(entity ecs.Entity) bool { return !r.world.IsAlive(entity) })
		c.entitiesDirty = false
	}
	clear(r.dirtyEntities)
	r.dirtyEntities = r.dirtyEntities[:0]
	r.busy = false
	r.pruneDisposed()
	if r.orderDirty {
		result = joinError(result, r.syncDrawOrder())
		r.orderDirty = false
	}
	if r.preparation == nil && !r.loading && !r.manager.switching {
		result = joinError(result, r.manager.flush())
	}
	r.SyncScopes()
	return result
}

func (r *Runtime) commandSpace() bool {
	return len(r.commands)+r.privateCommands < r.options.CommandCapacity
}
func (r *Runtime) claimPrivateCommand() error {
	if !r.commandSpace() {
		return ErrCommandFull
	}
	r.privateCommands++
	return nil
}

func (r *Runtime) despawnPrivate(c *Context, root ecs.Entity) {
	if !r.world.IsAlive(root) {
		return
	}
	// Cached adjacency includes procedural descendants and visits each once.
	r.despawnEntities = append(r.despawnEntities[:0], root)
	hierarchy := scene.HierarchyFor(r.world)
	for i := 0; i < len(r.despawnEntities); i++ {
		for child := hierarchy.FirstChild(r.despawnEntities[i]); child != 0; child = hierarchy.NextSibling(child) {
			childIndex, _ := child.Index()
			node := &r.scopeNodes[childIndex]
			if node.owner == c && r.world.IsAlive(child) {
				r.despawnEntities = append(r.despawnEntities, child)
			}
		}
	}
	for i := len(r.despawnEntities) - 1; i >= 0; i-- {
		entity := r.despawnEntities[i]
		if r.world.IsAlive(entity) {
			htmlui.Release(r.world, entity)
			r.world.Despawn(entity)
		}
	}
	if !c.entitiesDirty {
		c.entitiesDirty = true
		r.dirtyEntities = append(r.dirtyEntities, c)
	}
	c.dirtyDrawOrder()
}

func (r *Runtime) reparentPrivate(command structuralCommand) error {
	if !command.context.usable() || !r.world.IsAlive(command.entity) || !r.world.IsAlive(command.parent) {
		return nil
	}
	if err := ecs.Insert(r.world, command.entity, scene.Parent{Entity: command.parent}); err != nil {
		return err
	}
	return nil
}

// Procedural visuals with their own paint order keep it across module changes.
type explicitPainterOrder struct{}

func (r *Runtime) syncDrawOrder() error {
	// The inherited group fixes module placement. Local edits only renumber
	// that module and never rewrite later modules or parked scenes.
	for _, s := range r.scenes {
		for i := range s.modules {
			c := &s.modules[i]
			if !c.live || !c.orderDirty {
				continue
			}
			for index, entity := range c.entities {
				if ecs.Has[explicitPainterOrder](r.world, entity) {
					continue
				}
				var err error
				if c.ui {
					err = setPainterOrder(r.world, entity, scene.UIStackIndex(index+1))
				} else {
					err = setPainterOrder(r.world, entity, scene.RenderOrder2D(index+1))
				}
				if err != nil {
					return err
				}
			}
			c.orderDirty = false
		}
	}
	return nil
}

func setPainterOrder[T comparable](world *ecs.World, entity ecs.Entity, order T) error {
	if current, ok := ecs.Get[T](world, entity); ok && current == order {
		return nil
	}
	return ecs.Insert(world, entity, order)
}

func (r *Runtime) disposeContext(c *Context) error {
	if c == nil || !c.live || c.disposing {
		return nil
	}
	c.disposing = true
	var err error
	if c.destroy != nil {
		err = c.destroy()
	}
	managed := c.managed
	c.managed = nil
	for _, player := range managed {
		err = joinError(err, player.Close())
	}
	c.live = false
	r.scopeDirty = true
	r.orderDirty = true
	r.disposedDirty = true
	for i := range c.slots {
		err = joinError(err, c.slots[i].output.Release())
	}
	for i := len(c.ownedEntities) - 1; i >= 0; i-- {
		entity := c.ownedEntities[i]
		if r.world.IsAlive(entity) {
			htmlui.Release(r.world, entity)
			r.world.Despawn(entity)
		}
	}
	if c.resources != nil {
		c.resources.refs--
		if c.resources.refs == 0 {
			delete(r.contentResources, c.definition.ID)
			err = joinError(err, c.resources.store.Dispose())
		}
		c.resources = nil
	}
	if c.placement != 0 {
		r.world.Despawn(c.placement)
	}
	c.entities = nil
	c.ownedEntities = nil
	c.slots = nil
	c.update = nil
	c.frameUpdate = nil
	c.message = nil
	c.destroy = nil
	c.enter, c.resume, c.exit, c.suspend = nil, nil, nil, nil
	c.elements, c.pausedVisibility = nil, nil
	c.nodeBundle, c.nodePrepared = ecs.PreparedBundle{}, false
	if c.content != nil {
		c.content.state, c.content.input = nil, nil
		c.content = nil
	}
	return err
}

func (r *Runtime) pruneDisposed() {
	if !r.disposedDirty {
		return
	}
	for index := r.pendingFirst; index >= 0; {
		p := &r.pending[index]
		next := p.next
		if !p.source.live {
			r.releasePending(index)
		} else if !p.target.live && !p.replied {
			p.failure = ErrCanceled
			p.failureTick = r.tick + 1
		}
		index = next
	}
	r.disposedDirty = false
}

func (r *Runtime) disposeScene(s *Scene) error {
	var err error
	if s.foreground {
		err = r.manager.leave(s, false)
		s.foreground = false
	}
	err = joinError(err, r.world.FlushCommands())
	r.SyncScopes()
	for i := len(s.modules) - 1; i >= 0; i-- {
		err = joinError(err, r.disposeContext(&s.modules[i]))
	}
	err = joinError(err, r.disposeContext(&s.context))
	// Include procedural descendants created by UI/model libraries, even when
	// they were not individually adopted into a module's private entity list.
	for i := len(s.entities) - 1; i >= 0; i-- {
		e := s.entities[i]
		if r.world.IsAlive(e) {
			htmlui.Release(r.world, e)
			r.world.Despawn(e)
		}
	}
	s.entities = nil
	s.uploads.Reset()
	s.state = SceneDisposed
	s.rendered = false
	r.scopeDirty = true
	if r.manager.active == s {
		r.manager.active = nil
	}
	r.world.Despawn(s.uiRoot)
	r.world.Despawn(s.worldRoot)
	r.scenes = slices.DeleteFunc(r.scenes, func(other *Scene) bool { return other == s })
	return err
}

func (r *Runtime) Close() error {
	if r.closed {
		return nil
	}
	if r.busy {
		return ErrBusy
	}
	r.closed = true
	if r.manager.preload != nil {
		r.manager.preload.Discard()
	}
	var err error
	for len(r.scenes) > 0 {
		err = joinError(err, r.disposeScene(r.scenes[len(r.scenes)-1]))
	}
	r.pruneDisposed()
	for i := range 2 {
		clear(r.queues[i])
		r.queues[i] = r.queues[i][:0]
	}
	for buffer := range 4 {
		r.resetBuffer(uint8(buffer))
	}
	if scope := r.world.QueryScope(); scope == &r.runningScope || scope == &r.renderScope {
		r.world.SetQueryScope(nil)
	}
	return err
}

func (r *Runtime) failRequest(p pendingRequest, err error) error {
	return p.response.deliver(p.source, envelope{kind: replyEvent, source: p.target, target: p.source, topic: p.topic, id: p.id, err: err})
}
func (c *Context) Switch(request SceneSwitchRequest) error {
	if !c.usable() {
		return ErrDisposed
	}
	if c.definition != nil {
		return ErrOwnership
	}
	return c.runtime.manager.enqueue(c.scene, request)
}
func (c *Context) Name() string {
	if c == nil {
		return ""
	}
	return c.name
}

// Engine input is synchronous through the scene; only its explicit forwarding
// reaches a module. Sends from either callback remain queued for Dispatch.
func (r *Runtime) deliverInput(source *Context, value Input) error {
	boundary := !r.busy
	r.inputStore.values[0][0] = value
	r.busy = true
	err := r.inputStore.deliver(&source.scene.context, envelope{kind: engineEvent, source: source, target: &source.scene.context, topic: InputTopic})
	r.inputStore.values[0][0] = Input{}
	if boundary {
		r.busy = false
		return joinError(err, r.flushStructural())
	}
	return err
}
