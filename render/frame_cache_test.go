package render

import "testing"

func checkFrameCache(t *testing.T, cache *frameCache[int, int]) {
	t.Helper()
	count := 0
	var previous *frameCacheEntry[int, int]
	for entry := cache.oldest; entry != nil; entry = entry.next {
		count++
		if count > len(cache.entries) || entry.previous != previous || cache.entries[entry.key] != entry {
			t.Fatal("cache list/map disagree")
		}
		if previous != nil && previous.frame > entry.frame {
			t.Fatal("cache age order changed")
		}
		previous = entry
	}
	if count != len(cache.entries) || previous != cache.newest {
		t.Fatal("cache endpoints disagree")
	}
}

func TestFrameCacheExpiresColdEntriesAndShrinksBuckets(t *testing.T) {
	var cache frameCache[int, int]
	for key := range 256 {
		entry, created := cache.getOrCreate(key, 1)
		if !created {
			t.Fatal("new key was already cached")
		}
		entry.value = key + 1
	}
	hot := cache.entries[0]
	released := 0
	release := func(key int, value *int) {
		if *value != key+1 || key == 0 {
			t.Fatal("evicted a hot or incorrect entry")
		}
		released++
	}
	for frame := uint64(2); frame <= frameCacheIdleFrames+1; frame++ {
		cache.getOrCreate(0, frame)
		cache.trim(frame, release)
	}
	if released != 0 {
		t.Fatal("evicted before idle deadline")
	}
	for frame := uint64(frameCacheIdleFrames + 2); frame < frameCacheIdleFrames+2+bufferTrimDelay+4; frame++ {
		cache.getOrCreate(0, frame)
		before := released
		cache.trim(frame, release)
		if released-before > frameCacheTrimBudget {
			t.Fatal("exceeded per-frame eviction budget")
		}
		checkFrameCache(t, &cache)
	}
	if released != 255 || len(cache.entries) != 1 || cache.capacity != 32 || cache.entries[0] != hot || hot.value != 1 {
		t.Fatalf("lost hot entry or retained peak: released=%d entries=%d capacity=%d", released, len(cache.entries), cache.capacity)
	}
	cache.clear(nil)
	if cache.entries != nil || cache.oldest != nil || cache.newest != nil || hot.value != 0 {
		t.Fatal("clear retained cache storage or values")
	}
}

func TestFrameCacheRemovalAndWarmFramesDoNotAllocate(t *testing.T) {
	var cache frameCache[int, int]
	for key := range 256 {
		cache.getOrCreate(key, 1)
	}
	// Exercise middle, newest, oldest, and repeated hits within one frame.
	for _, key := range []int{123, 255, 0} {
		entry := cache.entries[key]
		cache.remove(entry, nil)
		if *entry != (frameCacheEntry[int, int]{}) {
			t.Fatal("removed entry retains references")
		}
		checkFrameCache(t, &cache)
		cache.getOrCreate(key, 2)
		cache.getOrCreate(key, 2)
		checkFrameCache(t, &cache)
	}
	frame := uint64(3)
	if allocations := testing.AllocsPerRun(100, func() {
		frame++
		for key := range 256 {
			cache.getOrCreate(key, frame)
		}
		cache.trim(frame, nil)
	}); allocations != 0 {
		t.Fatalf("warm frame allocated %g times", allocations)
	}
	checkFrameCache(t, &cache)
	for i := 0; i < frameCacheIdleFrames+bufferTrimDelay+8; i++ {
		frame++
		cache.trim(frame, nil)
	}
	if cache.entries != nil || cache.capacity != 0 {
		t.Fatal("idle cache retained oversized map buckets")
	}
}

func TestFrameCacheEvictionKeepsUpWithContinuousChurn(t *testing.T) {
	var cache frameCache[int, int]
	const perFrame = frameCacheTrimBudget + 17
	for frame := uint64(1); frame <= frameCacheIdleFrames*4; frame++ {
		cache.trim(frame, nil)
		for key := range perFrame {
			cache.getOrCreate(int(frame)*perFrame+key, frame)
		}
		if len(cache.entries) > (frameCacheIdleFrames+1)*perFrame {
			t.Fatalf("stale backlog grew faster than eviction: frame=%d entries=%d", frame, len(cache.entries))
		}
	}
	checkFrameCache(t, &cache)
}
