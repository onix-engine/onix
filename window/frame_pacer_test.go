package window

import (
	"testing"
	"time"
)

func TestNewFramePacerDefaultsToThirtyFPS(t *testing.T) {
	pacer := newFramePacer()

	if pacer.targetFPS != DefaultTargetFPS {
		t.Fatalf("expected default target fps %d, got %d", DefaultTargetFPS, pacer.targetFPS)
	}
	if pacer.interval != time.Second/time.Duration(DefaultTargetFPS) {
		t.Fatalf("unexpected default interval: %v", pacer.interval)
	}
}

func TestFramePacerReadySkipsFramesBeforeInterval(t *testing.T) {
	pacer := newFramePacer()
	pacer.setTargetFPS(30)

	start := time.Unix(0, 0)
	if !pacer.ready(start) {
		t.Fatal("expected first frame to be ready")
	}
	if pacer.ready(start.Add(20 * time.Millisecond)) {
		t.Fatal("expected frame before interval to be skipped")
	}
	if !pacer.ready(start.Add(34 * time.Millisecond)) {
		t.Fatal("expected frame after interval to be ready")
	}
}

func TestFramePacerAllowsUncappedMode(t *testing.T) {
	pacer := newFramePacer()
	pacer.setTargetFPS(0)

	if pacer.interval != 0 {
		t.Fatalf("expected uncapped interval 0, got %v", pacer.interval)
	}
	if !pacer.ready(time.Unix(0, 0)) {
		t.Fatal("expected uncapped mode to always be ready")
	}
}
