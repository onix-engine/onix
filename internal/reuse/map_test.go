package reuse

import "testing"

func TestMapRetentionRebuildsAfterPopulationFalls(t *testing.T) {
	values := make(map[int]*int, 4096)
	var retention MapRetention
	for i := range 4096 {
		value := i
		values[i] = &value
	}
	values = retention.Trim(values, 16)
	first := values[0]
	for i := 3; i < 4096; i++ {
		delete(values, i)
	}
	for range RetentionFrames {
		values = retention.Trim(values, 16)
	}
	if retention.capacity != 16 || len(values) != 3 || values[0] != first || *values[2] != 2 {
		t.Fatal("map reclamation lost live values or retained its peak")
	}
	if allocations := testing.AllocsPerRun(100, func() { values = retention.Trim(values, 16) }); allocations != 0 {
		t.Fatalf("warm map maintenance allocated %v times", allocations)
	}
	for i := 3; i < 2048; i++ {
		values[i] = first
	}
	retention.Record(len(values))
	clear(values)
	for range RetentionFrames {
		values = retention.Trim(values, 16)
	}
	if values == nil || len(values) != 0 || retention.capacity != 0 {
		t.Fatal("idle map did not retire its buckets while remaining writable")
	}
	values[42] = first
	if values[42] != first {
		t.Fatal("reclaimed map is not writable")
	}
}
