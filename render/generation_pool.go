package render

import "fmt"

type generationSlot[V any] struct {
	value      V
	generation uint32
	occupied   bool
}

type GenerationPool[R, V any] struct {
	slots []generationSlot[V]
	free  []uint32
}

func (p *GenerationPool[R, V]) Insert(value V) Handle[R] {
	if len(p.free) != 0 {
		last := len(p.free) - 1
		index := p.free[last]
		p.free = p.free[:last]
		slot := &p.slots[index]
		slot.value = value
		slot.occupied = true
		return NewHandle[R](index+1, slot.generation)
	}
	index := uint32(len(p.slots))
	p.slots = append(p.slots, generationSlot[V]{value: value, generation: 1, occupied: true})
	return NewHandle[R](index+1, 1)
}

func (p *GenerationPool[R, V]) Get(handle Handle[R]) (V, bool) {
	var zero V
	// Keep validation local so both value and borrowed lookups can be inlined.
	if !handle.Valid() || handle.ID() > uint32(len(p.slots)) {
		return zero, false
	}
	slot := &p.slots[handle.ID()-1]
	if !slot.occupied || slot.generation != handle.Generation() {
		return zero, false
	}
	return slot.value, true
}

// getRef borrows a live value until the next Insert, Remove or Clear on this
// pool. Native drivers use it within one operation to avoid copying metadata.
func (p *GenerationPool[R, V]) getRef(handle Handle[R]) *V {
	if !handle.Valid() || handle.ID() > uint32(len(p.slots)) {
		return nil
	}
	slot := &p.slots[handle.ID()-1]
	if !slot.occupied || slot.generation != handle.Generation() {
		return nil
	}
	return &slot.value
}

func (p *GenerationPool[R, V]) Remove(handle Handle[R]) (V, bool) {
	value := p.getRef(handle)
	if value == nil {
		var zero V
		return zero, false
	}
	index := handle.ID() - 1
	slot := &p.slots[index]
	removed := *value
	var zero V
	slot.value = zero
	slot.occupied = false
	slot.generation++
	if slot.generation == 0 {
		slot.generation = 1
	}
	p.free = append(p.free, index)
	return removed, true
}

func (p *GenerationPool[R, V]) Clear() {
	p.slots = nil
	p.free = nil
}

func (p *GenerationPool[R, V]) Len() int {
	return len(p.slots) - len(p.free)
}

func (p *GenerationPool[R, V]) Range(yield func(V) bool) {
	for index := range p.slots {
		if p.slots[index].occupied && !yield(p.slots[index].value) {
			return
		}
	}
}

func destroyNativeResource[R any](pool *GenerationPool[R, uint32], handle Handle[R], label string, destroy func(uint32) error) error {
	native, ok := pool.Get(handle)
	if !ok {
		return fmt.Errorf("render: invalid %s handle %s", label, handle)
	}
	if err := destroy(native); err != nil {
		return err
	}
	pool.Remove(handle)
	return nil
}
