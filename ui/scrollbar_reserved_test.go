package ui

import (
	"testing"

	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func TestScrollAreaReservedSpace(t *testing.T) {
	world := newUIWorld(t)
	area := SpawnScrollArea(world, ScrollAreaOptions{
		Width: 120, Height: 80, ContentWidth: 120, ContentHeight: 160,
		ReserveScrollbarSpace: true,
	})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	metrics, _ := GetScrollAreaMetrics(world, area)
	if metrics != (ScrollMetrics{ViewportWidth: 110, ViewportHeight: 70, MaxScrollX: 10, MaxScrollY: 90, HorizontalVisible: true, VerticalVisible: true}) {
		t.Fatalf("metrics=%+v", metrics)
	}
	clip, _ := world.Get[scene.ClipRect2D](area)
	if clip.Width != 110 || clip.Height != 70 {
		t.Fatalf("clip=%+v", clip)
	}
	SetScrollAreaOffset(world, area, mgl32.Vec2{100, 200})
	if offset, _ := GetScrollAreaOffset(world, area); offset != (mgl32.Vec2{10, 90}) {
		t.Fatalf("offset=%v", offset)
	}
	for entity, access := range world.Query[Relation[SurfaceStyle]]().All {
		if access.Ref().Owner != area {
			continue
		}
		size, _ := world.Get[scene.Size2D](entity)
		if world.Has[Part[ScrollTrackHorizontalPart]](entity) && (size.Width != 110 || size.Height != 10) {
			t.Fatalf("horizontal track=%+v", size)
		}
		if world.Has[Part[ScrollTrackVerticalPart]](entity) && (size.Width != 10 || size.Height != 70) {
			t.Fatalf("vertical track=%+v", size)
		}
	}
	world.GetMut[ScrollArea](area).ReserveScrollbarSpace = false
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	metrics, _ = GetScrollAreaMetrics(world, area)
	if metrics.ViewportWidth != 120 || metrics.ViewportHeight != 80 || metrics.HorizontalVisible {
		t.Fatalf("overlay metrics=%+v", metrics)
	}
}
