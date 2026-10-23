package ecs

// ComponentAccess provides explicit per-component change tracking during a query.
// Access values and returned pointers must not be retained after the iteration.
type ComponentAccess[T any] struct {
	value *T
	state *columnState
	// Pack the two uint32 values into one register when passing query rows.
	rowTick uint64
}

func (a ComponentAccess[T]) Get() T {
	return *a.value
}

// Ref returns a read-only component reference without marking it changed.
func (a ComponentAccess[T]) Ref() *T {
	return a.value
}

func (a ComponentAccess[T]) Mut() *T {
	a.state.markChanged(int(uint32(a.rowTick)), Tick(a.rowTick>>32))
	return a.value
}

// CommitComponent writes through query access only when the value differs.
// It uses the query's tick and does not mark equal values changed.
func CommitComponent[T comparable](access ComponentAccess[T], value T) bool {
	if *access.value == value {
		return false
	}
	*access.value = value
	access.state.markChanged(int(uint32(access.rowTick)), Tick(access.rowTick>>32))
	return true
}
