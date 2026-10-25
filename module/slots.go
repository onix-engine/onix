package module

import (
	"math"
	"slices"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
)

type slotState struct {
	value     project.SlotValue
	revision  uint64
	entity    ecs.Entity
	output    RenderOutputHandle
	observers []*slotObserver
}
type Slot struct {
	context *Context
	index   uint32
}
type SlotRevision struct {
	context  *Context
	index    uint32
	revision uint64
	others   [3]slotVersion
}

type slotVersion struct {
	index    uint32
	revision uint64
}
type slotObserver struct {
	queued  bool
	handler func(*Context) error
}

func (s *slotState) observerCapacity(r *Runtime) bool {
	needed := 0
	for _, o := range s.observers {
		if !o.queued {
			needed++
		}
	}
	return len(r.queues[r.write])+needed <= r.options.EventCapacity
}
func (s *slotState) notify(c *Context) {
	for _, o := range s.observers {
		if o.queued {
			continue
		}
		o.queued = true
		c.runtime.queues[c.runtime.write] = append(c.runtime.queues[c.runtime.write], envelope{kind: slotChangeEvent, source: c, observer: o})
	}
}

// OnSlotsChange coalesces writes to the named slots into one next-batch callback.
// Binding is cold Install work; stable slot changes do not allocate or poll.
func OnSlotsChange(c *Context, names []string, handler func(*Context) error) error {
	if c == nil || !c.installing || handler == nil || len(names) == 0 {
		return ErrInvalid
	}
	slots := make([]Slot, len(names))
	for i, name := range names {
		var err error
		slots[i], err = c.Slot(name)
		if err != nil {
			return err
		}
	}
	o := &slotObserver{handler: handler}
	for _, s := range slots {
		s.context.slots[s.index].observers = append(s.context.slots[s.index].observers, o)
	}
	return nil
}

// With guards a request against changes to up to four slots of its source.
func (v SlotRevision) With(other SlotRevision) SlotRevision {
	if v.context != other.context || v.context == nil {
		panic("module: combined revisions must belong to the same source")
	}
	for i := range v.others {
		if v.others[i].revision == 0 {
			v.others[i] = slotVersion{other.index, other.revision}
			return v
		}
	}
	panic("module: at most four slot revisions per request")
}

func (c *Context) Slot(name string) (Slot, error) {
	if !c.usable() || c.definition == nil {
		return Slot{}, ErrDisposed
	}
	index, ok := slices.BinarySearchFunc(c.definition.Slots, name, func(p project.SlotPlan, name string) int { return compareName(p.Name, name) })
	if !ok {
		return Slot{}, ErrInvalid
	}
	return Slot{c, uint32(index)}, nil
}

// Value is a borrowed read-only view until the next write. Fields storage belongs
// to the instance; use ReadSlot for a concrete scalar/contract value.
func (s Slot) Value() (project.SlotValue, bool) {
	if !s.context.usable() {
		return project.SlotValue{}, false
	}
	return s.context.slots[s.index].value, true
}

func ReadSlot[T any](s Slot) (T, error) {
	value, ok := s.Value()
	if !ok {
		var zero T
		return zero, ErrDisposed
	}
	decode, ok := s.context.runtime.registry.values[value.Type].(func(project.SlotValue) (T, error))
	if !ok || decode == nil {
		var zero T
		return zero, ErrType
	}
	return decode(value)
}

func (s Slot) Root() Entity {
	if !s.context.usable() {
		return Entity{}
	}
	return Entity{s.context, s.context.slots[s.index].entity}
}

func (s Slot) Revision() SlotRevision {
	if !s.context.usable() {
		return SlotRevision{}
	}
	return SlotRevision{context: s.context, index: s.index, revision: s.context.slots[s.index].revision}
}

func (v SlotRevision) stale() bool {
	if v.context == nil {
		return false
	}
	if !v.context.usable() || v.context.slots[v.index].revision != v.revision {
		return true
	}
	for _, other := range v.others {
		if other.revision != 0 && v.context.slots[other.index].revision != other.revision {
			return true
		}
	}
	return false
}

// Set is an internal write. External writes must enter the buffered event channel.
func (s Slot) Set(value project.SlotValue) error { return s.set(value, false) }
func (s Slot) set(value project.SlotValue, external bool) error {
	if !s.context.usable() {
		return ErrDisposed
	}
	c := s.context
	state := &c.slots[s.index]
	if external && !c.definition.Slots[s.index].Writable {
		return ErrReadOnly
	}
	valid, changed := compareValue(value, state.value)
	if !valid {
		return ErrType
	}
	if !changed && (value.Type != project.VisualValue || state.output.Empty()) {
		return nil
	}
	if !state.observerCapacity(c.runtime) {
		return ErrQueueFull
	}
	if value.Type == project.VisualValue {
		if err := s.SetVisual(RenderOutputHandle{}); err != nil {
			return err
		}
	} else {
		state.revision++
	}
	copyValue(&state.value, value)
	state.notify(c)
	if c.runtime.options.Wake != nil {
		c.runtime.options.Wake()
	}
	return nil
}

// Keep the visual component present even when empty, so replacement stays an
// in-place write while an event batch holds ECS component references.
func initVisualSlot(world *ecs.World, state *slotState) error {
	if state.value.Type != project.VisualValue {
		return nil
	}
	return ecs.Insert(world, state.entity, scene.RenderOutput2D{})
}

// SlotWriter is resolved by the application/scene orchestrator. Modules obtain
// no other instance handles; their external slot changes use messages.
type SlotWriter struct {
	slot   Slot
	buffer *slotBuffer
}

// Every declared slot type has its own prepared storage, so alternating
// differently shaped structures cannot force allocations at queue positions.
type slotBuffer struct {
	values  [4][]project.SlotValue
	lengths [4]int
	shape   project.SlotValue
}

func (b *slotBuffer) truncate(buffer uint8, length int) {
	for i := length; i < b.lengths[buffer]; i++ {
		clearValue(&b.values[buffer][i])
	}
	b.lengths[buffer] = length
}

func (r *Runtime) slotBuffer(shape project.SlotValue) *slotBuffer {
	if buffer, ok := r.slotBuffers[shape.Type]; ok {
		return buffer
	}
	buffer := &slotBuffer{shape: cloneValue(shape)}
	clearValue(&buffer.shape)
	for i := range 2 {
		buffer.values[i] = make([]project.SlotValue, r.options.PayloadCapacity)
		for j := range buffer.values[i] {
			buffer.values[i][j] = cloneValue(buffer.shape)
		}
	}
	r.slotBuffers[shape.Type] = buffer
	reserveUsedBuffers(&r.usedSlots, min(len(r.slotBuffers), r.options.EventCapacity))
	return buffer
}

func (s *Scene) SlotWriter(instance, name string) (SlotWriter, error) {
	if !s.Alive() {
		return SlotWriter{}, ErrDisposed
	}
	index, ok := s.instances[instance]
	if !ok {
		return SlotWriter{}, ErrInvalid
	}
	slot, err := s.modules[index].Slot(name)
	if err != nil {
		return SlotWriter{}, err
	}
	if !slot.context.definition.Slots[slot.index].Writable {
		return SlotWriter{}, ErrReadOnly
	}
	return SlotWriter{slot: slot, buffer: s.runtime.slotBuffer(slot.context.slots[slot.index].value)}, nil
}

func (w SlotWriter) Write(value project.SlotValue) error {
	if !w.slot.context.usable() {
		return ErrDisposed
	}
	if !w.slot.context.running() {
		return ErrUnavailable
	}
	r := w.slot.context.runtime
	if !validValue(value, w.slot.context.slots[w.slot.index].value) {
		return ErrType
	}
	if !r.hasQueueSpace() {
		return ErrQueueFull
	}
	buffer := r.buffer()
	index := w.buffer.lengths[buffer]
	if index == len(w.buffer.values[buffer]) {
		w.buffer.values[buffer] = append(w.buffer.values[buffer], cloneValue(w.buffer.shape))
	}
	copyValue(&w.buffer.values[buffer][index], value)
	if index == 0 {
		r.usedSlots[buffer] = append(r.usedSlots[buffer], w.buffer)
	}
	w.buffer.lengths[buffer]++
	r.queues[r.write] = append(r.queues[r.write], envelope{kind: slotEvent, slot: w.slot, buffer: buffer, value: index, slotBuffer: w.buffer})
	if r.options.Wake != nil {
		r.options.Wake()
	}
	return nil
}

func cloneValue(value project.SlotValue) project.SlotValue {
	result := value
	if len(value.Fields) > 0 {
		result.Fields = make([]project.SlotFieldValue, len(value.Fields))
		for i, f := range value.Fields {
			result.Fields[i] = project.SlotFieldValue{Name: f.Name, Value: cloneValue(f.Value)}
		}
	}
	return result
}

func validValue(value, shape project.SlotValue) bool {
	valid, _ := compareValue(value, shape)
	return valid
}

// Validate shape/finite numbers and detect unchanged writes in one traversal.
func compareValue(value, shape project.SlotValue) (bool, bool) {
	if value.Type != shape.Type || len(value.Fields) != len(shape.Fields) {
		return false, false
	}
	if value.Type == project.VisualValue && (value.String != "" || value.Bool || value.Integer != 0 || value.Number != 0 || value.Vector != ([4]float32{})) {
		return false, false
	}
	if math.IsNaN(value.Number) || math.IsInf(value.Number, 0) {
		return false, false
	}
	for _, v := range value.Vector {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return false, false
		}
	}
	changed := value.String != shape.String || value.Bool != shape.Bool || value.Integer != shape.Integer || value.Number != shape.Number || value.Vector != shape.Vector
	for i, f := range value.Fields {
		if f.Name != shape.Fields[i].Name {
			return false, false
		}
		valid, different := compareValue(f.Value, shape.Fields[i].Value)
		if !valid {
			return false, false
		}
		changed = changed || different
	}
	return true, changed
}

func copyValue(target *project.SlotValue, value project.SlotValue) {
	fields := target.Fields
	*target = value
	target.Fields = fields
	for i, f := range value.Fields {
		copyValue(&fields[i].Value, f.Value)
	}
}

func clearValue(value *project.SlotValue) {
	fields := value.Fields
	kind := value.Type
	*value = project.SlotValue{Type: kind, Fields: fields}
	for i := range fields {
		clearValue(&fields[i].Value)
	}
}
