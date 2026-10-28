package resource

import (
	"context"
	"sync"
)

// PreparationState separates background CPU work from render-thread upload.
type PreparationState uint8

const (
	PreparingCPU PreparationState = iota
	CPUReady
	PreparingGPU
	PreparationReady
	PreparationFailed
	PreparationCancelled
)

// Preparation transfers one CPU result to its caller. Load must not access ECS
// or render devices. Wake may be called by the worker and must be thread-safe.
type Preparation[T any] struct {
	mu      sync.Mutex
	state   PreparationState
	value   T
	err     error
	cancel  context.CancelFunc
	discard func(T)
	done    chan struct{}
}

func BeginPreparation[T any](load func(context.Context) (T, error), discard func(T), wake func()) *Preparation[T] {
	ctx, cancel := context.WithCancel(context.Background())
	p := &Preparation[T]{cancel: cancel, discard: discard, done: make(chan struct{})}
	go func() {
		defer close(p.done)
		defer cancel()
		value, err := load(ctx)
		p.mu.Lock()
		cancelled := p.state == PreparationCancelled
		if !cancelled {
			p.value, p.err, p.state = value, err, CPUReady
			if err != nil {
				p.state = PreparationFailed
				var zero T
				p.value = zero
			}
		}
		p.mu.Unlock()
		if (cancelled || err != nil) && discard != nil {
			discard(value)
		}
		if wake != nil {
			wake()
		}
	}()
	return p
}

// WaitCPU is for callers explicitly choosing synchronous preparation.
func (p *Preparation[T]) WaitCPU(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.done:
		return nil
	}
}

func (p *Preparation[T]) State() PreparationState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

// Take transfers the CPU result once, without waiting for unfinished work.
// The caller owns its cleanup and all subsequent GPU operations.
func (p *Preparation[T]) Take() (T, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var zero T
	if p.state == PreparationCancelled {
		return zero, false, context.Canceled
	}
	if p.state != CPUReady {
		return zero, false, p.err
	}
	value := p.value
	p.value, p.state = zero, PreparingGPU
	return value, true, nil
}

// Finish records the render-thread outcome after Take.
func (p *Preparation[T]) Finish(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state != PreparingGPU {
		return
	}
	p.err, p.state = err, PreparationReady
	if err != nil {
		p.state = PreparationFailed
	}
}

// Cancel does not wait for I/O. An unpublished CPU result is released by its
// owner; results already transferred by Take must be released by the caller.
func (p *Preparation[T]) Cancel() {
	p.cancel()
	p.mu.Lock()
	var zero T
	value, release := p.value, p.state == CPUReady
	p.value, p.state = zero, PreparationCancelled
	p.mu.Unlock()
	if release && p.discard != nil {
		p.discard(value)
	}
}
