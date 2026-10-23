package onix

import "time"

const ratePacerTolerance = time.Millisecond

const (
	DefaultTargetFPS        = 30
	DefaultTargetTPS        = 64
	DefaultMaxFrameDelta    = 250 * time.Millisecond
	DefaultMaxRendersPerRun = 1
)

type ratePacer struct {
	targetRate int
	interval   time.Duration
	next       time.Time
	started    bool
	maxSteps   int
}

func newRatePacer(targetRate int, maxSteps int) ratePacer {
	pacer := ratePacer{maxSteps: maxSteps}
	pacer.setTargetRate(targetRate)
	return pacer
}

func (p *ratePacer) setTargetRate(rate int) {
	if rate < 0 {
		rate = 0
	}
	p.targetRate = rate
	p.started = false
	p.next = time.Time{}
	if rate == 0 {
		p.interval = 0
		return
	}
	p.interval = time.Second / time.Duration(rate)
}

func (p *ratePacer) steps(now time.Time) (int, bool) {
	if p == nil || p.interval <= 0 {
		return 1, false
	}
	if p.maxSteps <= 0 {
		p.maxSteps = 1
	}
	if !p.started {
		p.started = true
		p.next = now.Add(p.interval)
		return 1, true
	}
	deadline := now.Add(min(p.interval/16, ratePacerTolerance))
	if deadline.Before(p.next) {
		return 0, false
	}
	steps := 0
	for !p.next.After(deadline) && steps < p.maxSteps {
		steps++
		p.next = p.next.Add(p.interval)
	}
	if steps == p.maxSteps && !p.next.After(now) {
		p.next = now.Add(p.interval)
	}
	return steps, false
}
