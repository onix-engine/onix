package render

import "testing"

func TestBufferRetentionShrinksAfterSustainedUnderuse(t *testing.T) {
	var retention bufferRetention
	for frame := 0; frame < bufferTrimDelay-1; frame++ {
		if capacity := retention.trimCapacity(8<<20, 64<<10, 1<<20); capacity != 0 {
			t.Fatalf("trimmed after only %d frames", frame+1)
		}
	}
	if capacity := retention.trimCapacity(8<<20, 64<<10, 1<<20); capacity != 1<<20 {
		t.Fatalf("trim capacity = %d, want 1 MiB", capacity)
	}
}

func TestBufferRetentionPreservesIntermittentDemand(t *testing.T) {
	var retention bufferRetention
	for frame := 0; frame < bufferTrimDelay*3; frame++ {
		used := 64 << 10
		if frame%7 == 0 {
			used = 3 << 20
		}
		if capacity := retention.trimCapacity(8<<20, used, 1<<20); capacity != 0 {
			t.Fatalf("trimmed oscillating demand to %d", capacity)
		}
	}
	retention.trimCapacity(8<<20, 3<<20, 1<<20)
	for frame := 0; frame < bufferTrimDelay-1; frame++ {
		retention.trimCapacity(8<<20, 1<<20, 1<<20)
	}
	if capacity := retention.trimCapacity(8<<20, 0, 1<<20); capacity != 2<<20 {
		t.Fatalf("lost recent peak: capacity = %d", capacity)
	}
}

func TestUploadBufferReleasesIdleStorage(t *testing.T) {
	buffer := make([]byte, 0, 8<<20)
	var retention bufferRetention
	for range bufferTrimDelay {
		buffer = trimUploadBuffer(buffer, &retention, 0, 64<<10)
	}
	if buffer != nil {
		t.Fatalf("idle buffer still retains %d bytes", cap(buffer))
	}
}

func TestUploadBufferRetainsRecentPeakOnIdleFrame(t *testing.T) {
	buffer := make([]uint32, 0, 8<<10)
	var retention bufferRetention
	for range bufferTrimDelay - 1 {
		buffer = trimUploadBuffer(buffer, &retention, 1024, 64)
	}
	buffer = trimUploadBuffer(buffer, &retention, 0, 64)
	if len(buffer) != 0 || cap(buffer) != 2048 {
		t.Fatalf("trimmed buffer len/cap = %d/%d, want 0/2048", len(buffer), cap(buffer))
	}
	if allocations := testing.AllocsPerRun(100, func() {
		buffer = trimUploadBuffer(buffer, &retention, 1024, 64)
	}); allocations != 0 {
		t.Fatalf("steady upload buffer allocated: %g", allocations)
	}
}
