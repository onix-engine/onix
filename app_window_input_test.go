package onix

import (
	"testing"
	"time"
)

func TestWindowDragDoubleClick(t *testing.T) {
	now := time.Unix(1, 0)
	var click windowDragClick
	for index, want := range [...]bool{false, true, false, true} {
		if got := click.press(120, 16, now.Add(time.Duration(index)*100*time.Millisecond)); got != want {
			t.Fatalf("press %d: double click = %v, want %v", index+1, got, want)
		}
	}
}

func TestWindowDragDoubleClickRequiresNearbyConsecutivePresses(t *testing.T) {
	now := time.Unix(1, 0)
	for _, test := range []struct {
		name  string
		x, y  float64
		delay time.Duration
		want  bool
	}{
		{name: "nearby", x: 124, y: 20, delay: 500 * time.Millisecond, want: true},
		{name: "expired", x: 120, y: 16, delay: 501 * time.Millisecond},
		{name: "horizontal distance", x: 125, y: 16, delay: time.Millisecond},
		{name: "vertical distance", x: 120, y: 21, delay: time.Millisecond},
		{name: "negative horizontal distance", x: 115, y: 16, delay: time.Millisecond},
		{name: "negative vertical distance", x: 120, y: 11, delay: time.Millisecond},
		{name: "clock reversed", x: 120, y: 16, delay: -time.Millisecond},
	} {
		t.Run(test.name, func(t *testing.T) {
			var click windowDragClick
			if click.press(120, 16, now) {
				t.Fatal("first press was a double click")
			}
			if got := click.press(test.x, test.y, now.Add(test.delay)); got != test.want {
				t.Fatalf("double click = %v, want %v", got, test.want)
			}
		})
	}
}

func TestWindowDraggingCancelsDoubleClick(t *testing.T) {
	now := time.Unix(1, 0)
	var click windowDragClick
	click.press(120, 16, now)
	click.move(140, 16)
	click.move(120, 16)
	if click.press(120, 16, now.Add(100*time.Millisecond)) {
		t.Fatal("moving away and back was treated as a double click")
	}
	click.move(122, 17)
	if !click.press(120, 16, now.Add(200*time.Millisecond)) {
		t.Fatal("small pointer motion cancelled a double click")
	}
}

func TestChangingWindowDragRegionCancelsDoubleClick(t *testing.T) {
	now := time.Unix(1, 0)
	app := App{}
	app.windowDragClick.press(120, 16, now)
	app.SetWindowDragRegion(func(x, y float64) bool { return y < 40 })
	if app.windowDragClick.press(120, 16, now.Add(100*time.Millisecond)) {
		t.Fatal("changing the drag region retained a pending double click")
	}
}

func TestWindowDragDoubleClickDoesNotAllocate(t *testing.T) {
	now := time.Unix(1, 0)
	var click windowDragClick
	if allocations := testing.AllocsPerRun(100, func() {
		click.press(120, 16, now)
		click.move(121, 17)
		click.press(121, 17, now.Add(100*time.Millisecond))
	}); allocations != 0 {
		t.Fatalf("double click allocated %g times", allocations)
	}
}
