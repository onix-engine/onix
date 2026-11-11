package input

import "testing"

func TestButtonStorageSignedCodesAndRepeatedTransitions(t *testing.T) {
	s := NewState()
	for _, key := range []KeyCode{-1000000, -1, 0, 1000000} {
		s.PressKey(key)
		s.ReleaseKey(key)
		s.PressKey(key)
		if !s.KeyHeld(key) || !s.KeyPressed(key) || !s.KeyReleased(key) {
			t.Fatalf("same-frame transitions lost for %d", key)
		}
	}
	s.EndFrame()
	for _, key := range []KeyCode{-1000000, -1, 0, 1000000} {
		if !s.KeyHeld(key) || s.KeyPressed(key) || s.KeyReleased(key) {
			t.Fatalf("frame reset lost held state for %d", key)
		}
		s.ReleaseKey(key)
	}
	s.EndFrame()
	if allocations := testing.AllocsPerRun(100, func() {
		s.PressKey(KeyA)
		s.ReleaseKey(KeyA)
		s.EndFrame()
	}); allocations != 0 {
		t.Fatalf("reused key storage allocated %g", allocations)
	}
}

func TestButtonStorageRepressReplacesModifierSnapshot(t *testing.T) {
	s := NewState()
	s.PressKey(KeyLeftControl)
	s.PressKey(KeyA)
	s.ReleaseKey(KeyA)
	s.ReleaseKey(KeyLeftControl)
	s.PressKey(KeyA)
	if s.KeyModifiers(KeyA) != 0 {
		t.Fatal("repress retained modifiers from the earlier press")
	}
	copy := NewState()
	copy.CopyFrom(s)
	s.ReleaseKey(KeyA)
	s.EndFrame()
	if !copy.KeyHeld(KeyA) || !copy.KeyPressed(KeyA) || !copy.KeyReleased(KeyA) {
		t.Fatal("snapshot shares mutable button state")
	}
}
