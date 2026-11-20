package node

// StateSet stores explicitly resolved boolean states in two compact bitsets.
// A valid bit distinguishes a live false value from an uninitialized state.
type StateSet[T ~uint16] struct {
	values T
	valid  T
}

func (s StateSet[T]) Get(flag T) (bool, bool) {
	return s.values&flag != 0, s.valid&flag != 0
}

func (s *StateSet[T]) Set(flag T, value bool) {
	if s == nil {
		return
	}
	s.valid |= flag
	if value {
		s.values |= flag
	} else {
		s.values &^= flag
	}
}

func (s *StateSet[T]) Clear(flag T) {
	if s == nil {
		return
	}
	s.values &^= flag
	s.valid &^= flag
}
