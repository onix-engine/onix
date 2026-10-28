package resource

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestPreparationTransfersResultAndWakesWithoutBlocking(t *testing.T) {
	release := make(chan struct{})
	woken := make(chan struct{}, 1)
	var discarded atomic.Int32
	p := BeginPreparation(func(context.Context) (int, error) {
		<-release
		return 42, nil
	}, func(int) { discarded.Add(1) }, func() { woken <- struct{}{} })
	if _, ready, err := p.Take(); ready || err != nil || p.State() != PreparingCPU {
		t.Fatal("unfinished preparation did not return immediately")
	}
	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := p.WaitCPU(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-woken:
	default:
		t.Fatal("CPU completion did not wake the loop")
	}
	value, ready, err := p.Take()
	if err != nil || !ready || value != 42 || p.State() != PreparingGPU {
		t.Fatalf("transfer = %d, %v, %v", value, ready, err)
	}
	if _, ready, _ := p.Take(); ready {
		t.Fatal("result transferred twice")
	}
	p.Finish(nil)
	if p.State() != PreparationReady {
		t.Fatal("GPU completion was not recorded")
	}
	p.Cancel()
	if discarded.Load() != 0 {
		t.Fatal("cancellation released a transferred result")
	}
}

func TestPreparationDiscardsUnpublishedResultsOnce(t *testing.T) {
	for _, when := range []string{"loading", "ready", "failed"} {
		t.Run(when, func(t *testing.T) {
			release := make(chan struct{})
			var discarded atomic.Int32
			loadErr := errors.New("decode failed")
			p := BeginPreparation(func(context.Context) (int, error) {
				<-release
				if when == "failed" {
					return 7, loadErr
				}
				return 7, nil
			}, func(value int) {
				if value == 7 {
					discarded.Add(1)
				}
			}, nil)
			if when == "loading" {
				p.Cancel()
			}
			close(release)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := p.WaitCPU(ctx); err != nil {
				t.Fatal(err)
			}
			if when == "failed" {
				if _, ready, err := p.Take(); ready || !errors.Is(err, loadErr) || p.State() != PreparationFailed {
					t.Fatal("decode error was not retained")
				}
			}
			p.Cancel()
			p.Cancel()
			if discarded.Load() != 1 {
				t.Fatalf("discarded %d times", discarded.Load())
			}
			if _, ready, err := p.Take(); ready || !errors.Is(err, context.Canceled) {
				t.Fatal("cancelled result remained available")
			}
		})
	}
}
