package input

import "testing"

var mapStateSink *State

func BenchmarkInputStateMaps(b *testing.B) {
	b.Run("create", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			s := NewState()
			s.PressKey(KeyLeftControl)
			s.PressKey(KeyA)
			s.PressMouse(MouseLeft, 1, 2)
			mapStateSink = s
		}
	})
	b.Run("frame", func(b *testing.B) {
		s := NewState()
		frame := func() {
			s.PressKey(KeyLeftControl)
			s.PressKey(KeyA)
			s.PressMouse(MouseLeft, 1, 2)
			if !s.KeyHeld(KeyA) || s.KeyModifiers(KeyA) != ModifierControl || !s.MouseHeld(MouseLeft) {
				b.Fatal("input chord lost")
			}
			s.ReleaseKey(KeyA)
			s.ReleaseKey(KeyLeftControl)
			s.ReleaseMouse(MouseLeft, 1, 2)
			s.EndFrame()
		}
		frame()
		b.ReportAllocs()
		for b.Loop() {
			frame()
		}
	})
	b.Run("copy", func(b *testing.B) {
		source, target := NewState(), NewState()
		source.PressKey(KeyLeftControl)
		source.PressKey(KeyA)
		source.PressMouse(MouseLeft, 1, 2)
		target.CopyFrom(source)
		b.ReportAllocs()
		for b.Loop() {
			target.CopyFrom(source)
		}
		mapStateSink = target
	})
}
