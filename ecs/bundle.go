package ecs

type bundleEntry interface {
	componentID(*World) componentID
}

type bundleValue interface {
	appendTo(*columnState, Tick)
}

type entityBundleEntry interface {
	bundleEntry
	materialize(Entity) bundleValue
}

type componentValue[T any] struct {
	value T
}

func (v componentValue[T]) componentID(world *World) componentID {
	return world.registerComponent[T]()
}

func (v componentValue[T]) appendTo(target *columnState, tick Tick) {
	target.typed[T]().append(v.value, tick)
}

type entityComponentValue[T any] struct {
	build func(Entity) T
}

func (v entityComponentValue[T]) componentID(world *World) componentID {
	return world.registerComponent[T]()
}

func (v entityComponentValue[T]) materialize(entity Entity) bundleValue {
	return componentValue[T]{value: v.build(entity)}
}

type Bundle struct {
	entries []bundleEntry
}

// PreparedBundle is a reusable, world-bound spawn plan. Copies share the
// sorted entries and the target table bound on the first successful spawn.
type PreparedBundle struct {
	plan *bundlePlan
}

type bundlePlan struct {
	world       *World
	table       *archetype
	entries     []preparedBundleEntry
	hasBuilders bool
}

func (b PreparedBundle) forWorld(world *World) *bundlePlan {
	if b.plan == nil || b.plan.world != world {
		panic("ecs: prepared bundle belongs to a different world")
	}
	return b.plan
}

type preparedBundleEntry struct {
	entry bundleEntry
	id    componentID
}

func NewBundle(capacity int) Bundle {
	if capacity < 0 {
		panic("ecs: bundle capacity is negative")
	}
	return Bundle{entries: make([]bundleEntry, 0, capacity)}
}

// Extend appends a separately authored component group before spawning. Groups
// must have disjoint component types, as with individual Bundle.Put calls.
func (b *Bundle) Extend(group Bundle) { b.entries = append(b.entries, group.entries...) }

func (bundle *Bundle) Put[T any](value T) {
	bundle.putEntry(componentValue[T]{value: value})
}

// Share slice growth across component types when assembling dynamic bundles.
//
//go:noinline
func (bundle *Bundle) putEntry(entry bundleEntry) {
	if bundle == nil {
		panic("ecs: bundle is nil")
	}
	bundle.entries = append(bundle.entries, entry)
}

// Value inspects a statically supplied component before spawning.
func (bundle Bundle) Value[T any]() (T, bool) {
	for _, entry := range bundle.entries {
		if value, ok := entry.(componentValue[T]); ok {
			return value.value, true
		}
	}
	var zero T
	return zero, false
}

func (bundle *Bundle) PutEntity[T any](build func(Entity) T) {
	if bundle == nil {
		panic("ecs: bundle is nil")
	}
	if build == nil {
		panic("ecs: entity component builder is nil")
	}
	bundle.putEntry(entityComponentValue[T]{build: build})
}
