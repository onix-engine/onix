package scene

import (
	"testing"
	"time"
)

func TestTimeAdvance(t *testing.T) {
	var timer Time
	start := time.Unix(0, 0)
	timer.Advance(start)
	timer.Advance(start.Add(16 * time.Millisecond))
	if timer.Frame != 1 {
		t.Fatalf("expected frame 1, got %d", timer.Frame)
	}
	if timer.Delta != 16*time.Millisecond {
		t.Fatalf("unexpected delta: %s", timer.Delta)
	}
}

func TestFixedTimeAccumulatesAndExpendsFixedSteps(t *testing.T) {
	timer := NewFixedTime(16 * time.Millisecond)
	timer.Accumulate(40*time.Millisecond, 0)

	if !timer.Expend() {
		t.Fatal("expected first fixed step")
	}
	if !timer.Expend() {
		t.Fatal("expected second fixed step")
	}
	if timer.Expend() {
		t.Fatal("expected overstep below timestep")
	}
	if timer.Frame != 2 {
		t.Fatalf("expected frame 2, got %d", timer.Frame)
	}
	if timer.Elapsed != 32*time.Millisecond {
		t.Fatalf("unexpected elapsed: %s", timer.Elapsed)
	}
	if timer.Overstep != 8*time.Millisecond {
		t.Fatalf("unexpected overstep: %s", timer.Overstep)
	}
	if got := timer.OverstepFraction(); got != 0.5 {
		t.Fatalf("unexpected overstep fraction: %f", got)
	}
}

func TestFixedTimeClampsAccumulatedDelta(t *testing.T) {
	timer := NewFixedTime(16 * time.Millisecond)
	timer.Accumulate(time.Second, 100*time.Millisecond)

	if timer.Overstep != 100*time.Millisecond {
		t.Fatalf("unexpected clamped overstep: %s", timer.Overstep)
	}
}
