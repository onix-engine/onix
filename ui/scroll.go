package ui

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

type ScrollMetrics struct {
	ViewportWidth     float32
	ViewportHeight    float32
	MaxScrollX        float32
	MaxScrollY        float32
	HorizontalVisible bool
	VerticalVisible   bool
}

func MeasureScrollArea(width, height, contentWidth, contentHeight float32) ScrollMetrics {
	return measureScrollArea(width, height, contentWidth, contentHeight, 0)
}

func (area ScrollArea) bindControl(world *ecs.World, entity ecs.Entity, opts ControlOptions) error {
	if area.Width <= 0 {
		area.Width = opts.Width
	}
	if area.Height <= 0 {
		area.Height = opts.Height
	}
	area.StackIndex = opts.StackIndex
	if err := ecs.Insert(world, entity, area); err != nil {
		return err
	}
	return installScrollAreaGeometry(world, entity, &area)
}

func installScrollAreaGeometry(world *ecs.World, entity ecs.Entity, area *ScrollArea) error {
	metrics := measureScrollArea(area.Width, area.Height, area.ContentWidth, area.ContentHeight, area.BarSize)
	if err := ecs.Insert(world, entity, scene.Size2D{Width: area.Width, Height: area.Height}); err != nil {
		return err
	}
	if err := ecs.Insert(world, entity, scene.UIScroll{
		Position:              mgl32.Vec2{},
		ViewportSize:          mgl32.Vec2{metrics.ViewportWidth, metrics.ViewportHeight},
		ContentSize:           mgl32.Vec2{area.ContentWidth, area.ContentHeight},
		WheelStep:             scrollWheelStep(area.WheelStep),
		ScrollbarSize:         scrollBarSize(area.BarSize),
		ScrollbarMinThumbSize: scrollMinThumbSize(area.MinThumbSize),
		DisableInput:          area.Controlled,
		DisableDrag:           area.DisableDrag,
		Horizontal:            metrics.HorizontalVisible,
		Vertical:              metrics.VerticalVisible,
	}); err != nil {
		return err
	}
	if !ecs.Has[scene.ClipRect2D](world, entity) {
		return ecs.Insert(world, entity, scene.ClipRect2D{})
	}
	return nil
}

// GetScrollAreaMetrics derives viewport and range data from the current area configuration.
func GetScrollAreaMetrics(world *ecs.World, entity ecs.Entity) (ScrollMetrics, bool) {
	area, ok := ecs.Get[ScrollArea](world, entity)
	if !ok {
		return ScrollMetrics{}, false
	}
	return measureScrollArea(area.Width, area.Height, area.ContentWidth, area.ContentHeight, area.BarSize), true
}

// GetScrollAreaOffset exposes positive content offsets while UIScroll
// retains Clay's non-positive child-offset convention internally.
func GetScrollAreaOffset(world *ecs.World, entity ecs.Entity) (mgl32.Vec2, bool) {
	offset, ok := scene.GetUIScrollOffset(world, entity)
	return offset.Mul(-1), ok
}

// SetScrollAreaOffset updates the single UIScroll state and clamps it to
// the area's current range.
func SetScrollAreaOffset(world *ecs.World, entity ecs.Entity, offset mgl32.Vec2) bool {
	metrics, ok := GetScrollAreaMetrics(world, entity)
	if !ok {
		return false
	}
	offset[0] = min(max(offset.X(), 0), metrics.MaxScrollX)
	offset[1] = min(max(offset.Y(), 0), metrics.MaxScrollY)
	scroll := ecs.GetMut[scene.UIScroll](world, entity)
	if scroll == nil {
		return false
	}
	scroll.Position = offset.Mul(-1)
	scroll.Momentum = mgl32.Vec2{}
	return true
}

// SetSelectorScrollOffset updates the selector list's single UIScroll
// state using a positive content offset.
func SetSelectorScrollOffset(world *ecs.World, selector ecs.Entity, offset float32) bool {
	updated := false
	for _, access := range ecs.Query2[SelectorList, scene.UIScroll](world).All {
		list := access.A.Mut()
		scroll := access.B.Mut()

		if list.Selector != selector {
			continue
		}
		offset = max(offset, 0)
		scroll.Position[1] = -offset
		scroll.Momentum = mgl32.Vec2{}
		updated = true
		break
	}
	return updated
}

func measureScrollArea(width, height, contentWidth, contentHeight, _ float32) ScrollMetrics {
	width = max(width, 0)
	height = max(height, 0)
	contentWidth = max(contentWidth, 0)
	contentHeight = max(contentHeight, 0)

	// Scrollbar visibility is measured against its own axis. A vertical bar
	// does not reduce horizontal content space, and a horizontal bar does not
	// reduce vertical content space.
	horizontal := contentWidth > width
	vertical := contentHeight > height

	return ScrollMetrics{
		ViewportWidth:     width,
		ViewportHeight:    height,
		MaxScrollX:        max(contentWidth-width, 0),
		MaxScrollY:        max(contentHeight-height, 0),
		HorizontalVisible: horizontal,
		VerticalVisible:   vertical,
	}
}

func scrollBarSize(size float32) float32 {
	if size > 0 {
		return size
	}
	return 10
}

func scrollWheelStep(step float32) float32 {
	if step > 0 {
		return step
	}
	return 48
}

func scrollMinThumbSize(size float32) float32 {
	if size > 0 {
		return size
	}
	return 24
}

func scrollThumbBarSize(size, barSize float32) float32 {
	if size > 0 {
		return size
	}
	return scrollBarSize(barSize)
}

func scrollHoverBarSize(size, thumbBarSize, barSize float32) float32 {
	if size > 0 {
		return size
	}
	base := scrollThumbBarSize(thumbBarSize, barSize)
	return max(base+4, base)
}

func scrollThumbSize(trackSize, viewportSize, contentSize, minThumbSize float32) float32 {
	if trackSize <= 0 {
		return 0
	}
	if contentSize <= 0 || viewportSize <= 0 || contentSize <= viewportSize {
		return trackSize
	}
	size := trackSize * (viewportSize / contentSize)
	size = max(size, scrollMinThumbSize(minThumbSize))
	return min(size, trackSize)
}

func scrollThumbOffset(trackSize, thumbSize, scroll, maxScroll float32) float32 {
	if maxScroll <= 0 || trackSize <= thumbSize {
		return 0
	}
	return (trackSize - thumbSize) * (scroll / maxScroll)
}
