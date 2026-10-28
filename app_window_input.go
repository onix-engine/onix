package onix

import "time"

// windowDragClick tracks consecutive presses in the custom titlebar. Native
// dragging consumes these presses before they reach the UI input state.
type windowDragClick struct {
	at   time.Time
	x, y float64
}

func (click *windowDragClick) press(x, y float64, now time.Time) bool {
	click.move(x, y)
	if elapsed := now.Sub(click.at); !click.at.IsZero() && elapsed >= 0 && elapsed <= 500*time.Millisecond {
		*click = windowDragClick{}
		return true
	}
	*click = windowDragClick{at: now, x: x, y: y}
	return false
}

func (click *windowDragClick) move(x, y float64) {
	if click.at.IsZero() {
		return
	}
	dx, dy := x-click.x, y-click.y
	if dx < -4 || dx > 4 || dy < -4 || dy > 4 {
		*click = windowDragClick{}
	}
}
