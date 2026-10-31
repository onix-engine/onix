package reuse

import "testing"

func TestRetentionPreservesValuesAndIntermittentBatches(t *testing.T) {
	values := make([]int, 3, 8192)
	copy(values, []int{17, 29, 41})
	var retention Retention
	for frame := range RetentionFrames * 2 {
		if frame%7 == 0 {
			retention.Record(4096)
		}
		values = retention.Trim(values, 64)
		if cap(values) != 8192 {
			t.Fatal("an earlier batch in the frame lost its capacity")
		}
	}
	for range RetentionFrames {
		values = retention.Trim(values, 64)
	}
	if cap(values) != 64 || len(values) != 3 || values[0] != 17 || values[1] != 29 || values[2] != 41 {
		t.Fatalf("shrinking changed live values: %v, cap=%d", values, cap(values))
	}
	if allocs := testing.AllocsPerRun(100, func() { values = retention.Trim(values, 64) }); allocs != 0 {
		t.Fatalf("stable retention allocated %g times", allocs)
	}
	values = append(values, make([]int, 4096)...)
	for range RetentionFrames {
		values = retention.Trim(values[:0], 64)
	}
	if values != nil {
		t.Fatal("idle storage was not released")
	}
}

func TestRetentionPreservesSmallRecentDemandOnIdleFrame(t *testing.T) {
	values := make([]byte, 0, 8192)
	var retention Retention
	for range RetentionFrames - 1 {
		retention.Record(3)
		values = retention.Trim(values, 64)
	}
	values = retention.Trim(values, 64)
	if cap(values) != 64 {
		t.Fatal("idle final frame discarded recent demand")
	}
}
