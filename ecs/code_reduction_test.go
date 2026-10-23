package ecs

import "testing"

func TestCachedValuesKeepOwnersTagsAndTypesIndependent(t *testing.T) {
	w := NewWorld()
	var scope QueryScope
	var context SystemContext
	frame := FrameValue[int, int](w)
	*frame = 1
	if ScopeValue[int, int](w) != frame {
		t.Fatal("unscoped value lost its frame cache")
	}
	w.SetQueryScope(&scope)
	scoped := ScopeValue[int, int](w)
	local := Local[int, int](&context)
	*scoped, *local = 2, 3
	if *frame != 1 || *FrameValue[string, int](w) != 0 || *FrameValue[int, *int](w) != nil {
		t.Fatal("cache owners, tags or value types overlap")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if FrameValue[int, int](w) != frame || ScopeValue[int, int](w) != scoped || Local[int, int](&context) != local {
			panic("cache did not retain its value")
		}
	}); allocations != 0 {
		t.Fatalf("warm cache lookup allocated %g times", allocations)
	}
}

func TestScheduleRecompileKeepsLocalsAndReaddedSystemStartsFresh(t *testing.T) {
	w := NewWorld()
	s := NewSchedule("locals")
	var values []int
	config := SystemConfig{Name: "counter", System: func(context *SystemContext) error {
		value := Local[int, int](context)
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
	FrameValue[int, int](w)
	ScopeValue[int, int](w)
	Local[int, int](&context)
	b.ReportAllocs()
	for b.Loop() {
		*FrameValue[int, int](w)++
		*ScopeValue[int, int](w)++
		*Local[int, int](&context)++
	}
}
