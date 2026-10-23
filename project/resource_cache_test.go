package project

import (
	"errors"
	iofs "io/fs"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/diiyw/onix/resource"
)

func TestResourceCacheDeduplicatesConcurrentLoad(t *testing.T) {
	cache := newResourceCache[string, int](1)
	started := make(chan struct{})
	release := make(chan struct{})
	var loads atomic.Int32
	if err := cache.register("shared", func() (int, error) {
		loads.Add(1)
		close(started)
		<-release
		return 42, nil
	}, nil); err != nil {
		t.Fatal(err)
	}

	values := make([]int, 8)
	errorsByIndex := make([]error, len(values))
	var wait sync.WaitGroup
	wait.Add(len(values))
	for index := range values {
		go func() {
			defer wait.Done()
			values[index], _, errorsByIndex[index] = cache.get("shared")
		}()
	}
	<-started
	close(release)
	wait.Wait()
	for index, value := range values {
		if errorsByIndex[index] != nil || value != 42 {
			t.Fatalf("load %d = %d, %v", index, value, errorsByIndex[index])
		}
	}
	if loads.Load() != 1 {
		t.Fatalf("loads = %d, want 1", loads.Load())
	}
	statistics := cache.statistics()
	if statistics.Misses != 1 || statistics.Hits != uint64(len(values)-1) || statistics.Resident != 1 {
		t.Fatalf("statistics = %+v", statistics)
	}
}

func TestResourceCacheLoadsDifferentKeysConcurrently(t *testing.T) {
	cache := newResourceCache[string, int](2)
	started := make(chan string, 2)
	release := make(chan struct{})
	for _, key := range []string{"a", "b"} {
		key := key
		if err := cache.register(key, func() (int, error) {
			started <- key
			<-release
			return len(key), nil
		}, nil); err != nil {
			t.Fatal(err)
		}
	}
	var wait sync.WaitGroup
	wait.Add(2)
	for _, key := range []string{"a", "b"} {
		key := key
		go func() {
			defer wait.Done()
			_, _, _ = cache.get(key)
		}()
	}
	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("different cache keys did not load concurrently")
		}
	}
	close(release)
	wait.Wait()
}

func TestResourceCacheRetriesPackageNotLoaded(t *testing.T) {
	cache := newResourceCache[string, int](1)
	var attempts atomic.Int32
	if err := cache.register("optional", func() (int, error) {
		if attempts.Add(1) == 1 {
			return 0, &resource.PackageNotLoadedError{Package: "chapter", Path: "asset"}
		}
		return 7, nil
	}, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := cache.get("optional"); !errors.Is(err, resource.ErrPackageNotLoaded) {
		t.Fatalf("first load error = %v", err)
	}
	value, _, err := cache.get("optional")
	if err != nil || value != 7 {
		t.Fatalf("retry = %d, %v", value, err)
	}
	if attempts.Load() != 2 {
		t.Fatalf("attempts = %d, want 2", attempts.Load())
	}
}

func TestResourceCacheDisposesResidentValueOnce(t *testing.T) {
	cache := newResourceCache[string, int](1)
	var disposals atomic.Int32
	if err := cache.register("value", func() (int, error) { return 3, nil }, func(value int) error {
		if value != 3 {
			t.Fatalf("disposed value = %d", value)
		}
		disposals.Add(1)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := cache.get("value"); err != nil {
		t.Fatal(err)
	}
	if err := cache.disposeResources(); err != nil {
		t.Fatal(err)
	}
	if err := cache.disposeResources(); err != nil {
		t.Fatal(err)
	}
	if disposals.Load() != 1 || cache.statistics().Resident != 0 {
		t.Fatalf("disposals = %d, statistics = %+v", disposals.Load(), cache.statistics())
	}
	if _, _, err := cache.get("value"); !errors.Is(err, iofs.ErrClosed) {
		t.Fatalf("closed cache error = %v", err)
	}
}
