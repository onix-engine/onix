package ecs

import "testing"

func TestCachedValuesKeepOwnersTagsAndTypesIndependent(t *testing.T) {
	w := NewWorld()
	var scope QueryScope
	var context SystemContext
	frame := w.FrameValue[int, int]()
	*frame = 1
	if w.ScopeValue[int, int]() != frame {
		t.Fatal("unscoped value lost its frame cache")
	}
	w.SetQueryScope(&scope)
	scoped := w.ScopeValue[int, int]()
	local := context.Local[int, int]()
	*scoped, *local = 2, 3
	if *frame != 1 || *w.FrameValue[string, int]() != 0 || *w.FrameValue[int, *int]() != nil {
		t.Fatal("cache owners, tags or value types overlap")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if w.FrameValue[int, int]() != frame || w.ScopeValue[int, int]() != scoped || context.Local[int, int]() != local {
			panic("cache did not retain its value")
		}
	}); allocations != 0 {
		t.Fatalf("warm cache lookup allocated %g times", allocations)
	}
}

func TestFrameCacheKindsKeepPointerElementScratchIndependent(t *testing.T) {
	type tag struct{}
	for _, scratchFirst := range []bool{false, true} {
		w := NewWorld()
		var value *int
		var scratch *Scratch[*int]
		if scratchFirst {
			scratch = w.FrameScratch[tag, *int]()
			value = w.FrameValue[tag, int]()
		} else {
			value = w.FrameValue[tag, int]()
			scratch = w.FrameScratch[tag, *int]()
		}
		*value = 7
		scratch.Values = append(scratch.Values, value)
		if w.FrameValue[tag, int]() != value || w.ScopeValue[tag, int]() != value {
			t.Fatal("frame value and unscoped cache must share their own storage")
		}
		if w.FrameScratch[tag, *int]() != scratch || len(scratch.Values) != 0 || *value != 7 {
			t.Fatal("resetting pointer scratch changed the persistent value")
		}
		if allocations := testing.AllocsPerRun(100, func() {
			w.FrameValue[tag, int]()
			w.FrameScratch[tag, *int]()
		}); allocations != 0 {
			t.Fatalf("mixed frame caches allocated %g times", allocations)
		}
	}
}

func TestScheduleRecompileKeepsLocalsAndReaddedSystemStartsFresh(t *testing.T) {
	w := NewWorld()
	s := NewSchedule("locals")
	var values []int
	config := SystemConfig{Name: "counter", System: func(context *SystemContext) error {
		value := context.Local[int, int]()
		values = append(values, *value)
		*value++
		return nil
	}}
	for run := range 3 {
		if run != 1 {
			if err := s.Add(config); err != nil {
				t.Fatal(err)
			}
		}
		// Removing an absent system also forces a rebuild.
		s.Remove("absent")
		if err := s.Run(w); err != nil {
			t.Fatal(err)
		}
		if run == 1 {
			s.Remove(config.Name)
		}
	}
	if values[0] != 0 || values[1] != 1 || values[2] != 0 {
		t.Fatalf("local values across recompiles and replacement = %v", values)
	}
}

func BenchmarkCachedValues(b *testing.B) {
	w := NewWorld()
	var scope QueryScope
	var context SystemContext
	w.SetQueryScope(&scope)
	w.FrameValue[int, int]()
	w.ScopeValue[int, int]()
	context.Local[int, int]()
	b.ReportAllocs()
	for b.Loop() {
		*w.FrameValue[int, int]()++
		*w.ScopeValue[int, int]()++
		*context.Local[int, int]()++
	}
}
