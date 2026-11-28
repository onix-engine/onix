//go:build !js

package audio

import (
	"runtime"
	"sync"
)

const outputBlockFrames = outputSampleRate / 100 // 10 ms

var nativeDevice = nativeMixer{players: make(map[*nativePlayer]struct{})}

type nativeMixer struct {
	players   map[*nativePlayer]struct{}
	output    *outputRun
	mu        sync.Mutex
	suspended bool
}

// A player keeps the error from the output it used even if another player
// has since reopened the device. The mixer mutex protects these fields.
type outputRun struct {
	err      error
	ready    chan struct{}
	done     chan struct{}
	wake     sync.Cond
	stopping bool
}

func (m *nativeMixer) startLocked() *outputRun {
	for !m.suspended {
		run := m.output
		if run == nil {
			run = &outputRun{ready: make(chan struct{}), done: make(chan struct{})}
			run.wake.L = &m.mu
			m.output = run
			for p := range m.players {
				p.output = run
			}
			go m.run(run)
		}
		if run.stopping {
			m.mu.Unlock()
			<-run.done
			m.mu.Lock()
			continue
		}
		ready := run.ready
		if ready == nil {
			return run
		}
		// Report open errors synchronously without holding the mixer lock.
		m.mu.Unlock()
		<-ready
		m.mu.Lock()
		if run.err != nil {
			return run
		}
	}
	return nil
}

func (m *nativeMixer) run(run *outputRun) {
	// WASAPI's COM interfaces must be opened, used and released on one thread.
	if runtime.GOOS == "windows" {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
	}
	out, err := openNativeOutput()
	opened := err == nil
	m.mu.Lock()
	close(run.ready)
	run.ready = nil
	var samples [outputBlockFrames * outputChannels]float32
	// Feed short gaps with silence. For longer gaps, drain the output and
	// sleep until the next player or Suspend wakes this same device thread.
	const maxIdleBlocks = outputSampleRate / outputBlockFrames / 4
	idleBlocks := maxIdleBlocks
	for !run.stopping && err == nil {
		if len(m.players) == 0 {
			if idleBlocks == maxIdleBlocks {
				run.wake.Wait()
				continue
			}
			idleBlocks++
		} else {
			idleBlocks = 0
		}
		m.mix(samples[:])
		m.mu.Unlock()
		err = out.write(samples[:])
		if err == nil && idleBlocks == maxIdleBlocks {
			err = out.idle()
		}
		m.mu.Lock()
	}
	run.stopping = true
	run.err = err
	if err != nil {
		for p := range m.players {
			if p.output == run {
				delete(m.players, p)
			}
		}
	}
	if opened {
		m.mu.Unlock()
		closeErr := out.close(err == nil)
		m.mu.Lock()
		if run.err == nil {
			run.err = closeErr
		}
	}
	m.output = nil
	close(run.done)
	m.mu.Unlock()
}

func (m *nativeMixer) mix(samples []float32) int {
	clear(samples)
	n := 0
	for p := range m.players {
		n = max(n, p.mix(samples))
		if p.frame == p.frames || p.err != nil {
			delete(m.players, p)
		}
	}
	for i, sample := range samples[:n] {
		samples[i] = max(-1, min(sample, 1))
	}
	return n
}

func suspendPlatform() error {
	m := &nativeDevice
	m.mu.Lock()
	m.suspended = true
	run := m.output
	if run != nil {
		run.stopping = true
		run.wake.Signal()
	}
	m.mu.Unlock()
	if run != nil {
		<-run.done
		return run.err
	}
	return nil
}

func resumePlatform() error {
	m := &nativeDevice
	m.mu.Lock()
	defer m.mu.Unlock()
	m.suspended = false
	if len(m.players) == 0 {
		return nil
	}
	if run := m.startLocked(); run != nil {
		return run.err
	}
	return nil
}
