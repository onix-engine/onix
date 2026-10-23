package module

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
)

type RenderOutputHandle = scene.RenderOutputHandle

// SetVisual borrows a handle and retains a separate lease for this slot. A
// reply value is borrowed only until its callback returns. Empty clears it.
func (s Slot) SetVisual(handle RenderOutputHandle) error {
	if !s.context.usable() {
		return ErrDisposed
	}
	state := &s.context.slots[s.index]
	if state.value.Type != project.VisualValue {
		return ErrType
	}
	if !state.observerCapacity(s.context.runtime) {
		return ErrQueueFull
	}
	var lease RenderOutputHandle
	var err error
	if !handle.Empty() {
		lease, err = handle.Retain()
		if err != nil {
			return err
		}
	}
	if err = ecs.Insert(s.context.runtime.world, state.entity, scene.RenderOutput2D{Handle: lease}); err != nil {
		_ = lease.Release()
		return err
	}
	previous := state.output
	state.output = lease
	state.revision++
	state.notify(s.context)
	if s.context.runtime.options.Wake != nil {
		s.context.runtime.options.Wake()
	}
	return previous.Release()
}

// Visual is a borrowed view of the slot's lease, not ownership of that lease.
func (s Slot) Visual() (RenderOutputHandle, bool) {
	if !s.context.usable() || s.context.slots[s.index].value.Type != project.VisualValue {
		return RenderOutputHandle{}, false
	}
	return s.context.slots[s.index].output, true
}
