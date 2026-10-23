package onix

import (
	"testing"
	"time"
)

func TestNewRatePacerSetsTargetRate(t *testing.T) {
	pacer := newRatePacer(DefaultTargetFPS, DefaultMaxRendersPerRun)

	if pacer.targetRate != DefaultTargetFPS {
		t.Fatalf("expected default target rate %d, got %d", DefaultTargetFPS, pacer.targetRate)
	}
	if pacer.interval != time.Second/time.Duration(DefaultTargetFPS) {
		t.Fatalf("unexpected default interval: %v", pacer.interval)
	}
}

func TestRatePacerAllowsUncappedMode(t *testing.T) {
	pacer := newRatePacer(DefaultTargetFPS, DefaultMaxRendersPerRun)
	pacer.setTargetRate(0)

	if pacer.interval != 0 {
		t.Fatalf("expected uncapped interval 0, got %v", pacer.interval)
	}
	steps, first := pacer.steps(time.Unix(0, 0))
	if steps != 1 || first {
		t.Fatalf("expected one uncapped step, got steps=%d first=%v", steps, first)
	}
}

func TestRatePacerSkipsAndCatchesUpSteps(t *testing.T) {
	pacer := newRatePacer(DefaultTargetFPS, 2)
	start := time.Unix(0, 0)

	steps, first := pacer.steps(start)
	if steps != 1 || !first {
		t.Fatalf("expected first step, got steps=%d first=%v", steps, first)
	}

	steps, first = pacer.steps(start.Add(pacer.interval / 2))
	if steps != 0 || first {
		t.Fatalf("expected no step before interval, got steps=%d first=%v", steps, first)
	}

	steps, first = pacer.steps(start.Add(2 * pacer.interval))
	if steps != 2 || first {
		t.Fatalf("expected two catch-up steps, got steps=%d first=%v", steps, first)
	}
}

func TestRatePacerCapsCatchUpSteps(t *testing.T) {
	pacer := newRatePacer(DefaultTargetFPS, 1)
	start := time.Unix(0, 0)
	pacer.steps(start)

	steps, _ := pacer.steps(start.Add(10 * pacer.interval))
	if steps != 1 {
		t.Fatalf("expected capped catch-up step, got %d", steps)
	}
}

func TestRatePacerAcceptsSubMillisecondFrameJitter(t *testing.T) {
	pacer := newRatePacer(60, 1)
	start := time.Unix(0, 0)
	pacer.steps(start)

	steps, _ := pacer.steps(start.Add(pacer.interval - 500*time.Microsecond))
	if steps != 1 {
		t.Fatalf("expected near-deadline frame to render, got %d steps", steps)
	}
}
