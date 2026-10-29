package ui

import (
	"math"
	"slices"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

const (
	defaultListItemSize float32 = 32
	defaultListOverscan         = 2
)

// ListAxis controls the axis along which list items are laid out and scrolled.
type ListAxis uint8

const (
	ListVertical ListAxis = iota
	ListHorizontal
)

// List is a virtual list backed by a fixed pool of item entities. Items may be
// replaced at runtime; changing the viewport or item size requires recreating
// the list so its item pool remains correctly sized.
type List[T any] struct {
	Items      []T
	Axis       ListAxis
	ItemSize   float32
	Overscan   int
	First      int
	Last       int
	BindItem   func(*ecs.World, ecs.Entity, int, T)
	UnbindItem func(*ecs.World, ecs.Entity)

	itemEntities []ecs.Entity
}

// ListItem identifies the item currently represented by a pooled item entity.
// Index is -1 while the entity is not in use.
type ListItem[T any] struct {
	List  ecs.Entity
	Index int
	Value T
}

type ListOptions[T any] struct {
	X, Y          float32
	Width, Height float32
	Items         []T
	Axis          ListAxis
	ItemSize      float32
	Overscan      int
	Controlled    bool
	ScrollX       float32
	ScrollY       float32
	BarSize       float32
	MinThumbSize  float32
	ThumbBarSize  float32
	HoverBarSize  float32
	Radius        CornerRadius
	PassThrough   bool
	Theme         Theme
	Style         *ScrollStyle

	// BuildItem initializes each pooled entity once. Add custom visuals as its
	// children; BindItem then updates them whenever it represents an item.
	// UnbindItem resets those visuals when a pooled entity stops representing an
	// item. It is also called once after BuildItem so empty lists start cleanly.
	BuildItem  func(*ecs.World, ecs.Entity)
	BindItem   func(*ecs.World, ecs.Entity, int, T)
	UnbindItem func(*ecs.World, ecs.Entity)
}

type listEntitiesScratch[T any] struct{}

// SpawnList creates a virtual list. Its entity count is based on the viewport
// and overscan, not on the number of items.
func SpawnList[T any](world *ecs.World, opts ListOptions[T]) ecs.Entity {
	axis := normalizeListAxis(opts.Axis)
	itemSize := normalizeListItemSize(opts.ItemSize)
	contentWidth, contentHeight := listContentSize(axis, itemSize, len(opts.Items))
	list := SpawnScrollArea(world, ScrollAreaOptions{
		X: opts.X, Y: opts.Y, Width: opts.Width, Height: opts.Height,
		ContentWidth: contentWidth, ContentHeight: contentHeight,
		Controlled: opts.Controlled, ScrollX: opts.ScrollX, ScrollY: opts.ScrollY,
		BarSize: opts.BarSize, MinThumbSize: opts.MinThumbSize,
		ThumbBarSize: opts.ThumbBarSize, HoverBarSize: opts.HoverBarSize,
		Radius: opts.Radius, PassThrough: opts.PassThrough, Theme: opts.Theme, Style: opts.Style,
	})
	BindList(world, list, opts)
	return list
}

// BindList turns an existing scroll area into a virtual list. It is useful for
// scroll areas authored in a scene, whose transform and visual parts should be
// preserved.
func BindList[T any](world *ecs.World, entity ecs.Entity, opts ListOptions[T]) bool {
	area := world.GetRef[ScrollArea](entity)
	if area == nil {
		return false
	}
	width, height := area.Width, area.Height
	axis := normalizeListAxis(opts.Axis)
	itemSize := normalizeListItemSize(opts.ItemSize)
	overscan := listOverscan(opts.Overscan)
	component := List[T]{
		Items: slices.Clone(opts.Items), Axis: axis, ItemSize: itemSize, Overscan: overscan,
		BindItem: opts.BindItem, UnbindItem: opts.UnbindItem,
	}
	if err := world.Insert(entity, component); err != nil {
		return false
	}

	theme := themeForWorld(world, opts.Theme)
	viewportSize := listViewportSize(axis, width, height)
	itemWidth, itemHeight := listItemDimensions(axis, itemSize, width, height)
	itemCount := listItemPoolSize(viewportSize, itemSize, overscan)
	for range itemCount {
		item := spawnInteractiveAnchor(world, 0, 0, itemWidth, itemHeight, theme.StackIndex+1, false, opts.PassThrough)
		_ = world.Insert(item, scene.Parent{Entity: entity})
		_ = world.Insert(item, ScrollContent{Area: entity})
		_ = world.Insert(item, ListItem[T]{List: entity, Index: -1})
		setVisible(world, item, false)
		component := world.GetMut[List[T]](entity)
		component.itemEntities = append(component.itemEntities, item)
		if opts.BuildItem != nil {
			opts.BuildItem(world, item)
		}
		if opts.UnbindItem != nil {
			opts.UnbindItem(world, item)
		}
	}
	syncListEntity[T](world, entity)
	return true
}

// SetListItems replaces a list's data with an owned copy.
func SetListItems[T any](world *ecs.World, entity ecs.Entity, items []T) bool {
	list := world.GetMut[List[T]](entity)
	if list == nil {
		return false
	}
	list.Items = slices.Clone(items)
	return true
}

// ListItemEntities returns a copy of the pooled item entity handles.
func ListItemEntities[T any](world *ecs.World, entity ecs.Entity) []ecs.Entity {
	list := world.GetRef[List[T]](entity)
	if list == nil {
		return nil
	}
	return slices.Clone(list.itemEntities)
}

// SyncListSystem updates the pooled item entities after scrolling or replacing items.
// Add one system for every item type used by a world.
func SyncListSystem[T any](name string) ecs.SystemConfig {
	if name == "" {
		name = "ui_list_sync"
	}
	return ecs.SystemConfig{Name: name, System: syncLists[T]}
}

func syncLists[T any](context *ecs.SystemContext) error {
	world := context.World
	if world == nil {
		return nil
	}
	entities := world.FrameScratch[listEntitiesScratch[T], ecs.Entity]()
	for entity, _ := range world.Query[List[T]]().All {
		entities.Values = append(entities.Values, entity)
	}
	for _, entity := range entities.Values {
		syncListEntity[T](world, entity)
	}
	entities.ReleaseUnused()
	return nil
}

func syncListEntity[T any](world *ecs.World, entity ecs.Entity) {
	list := world.GetMut[List[T]](entity)
	area := world.GetMut[ScrollArea](entity)
	if (list == nil) || (area == nil) {
		return
	}
	area.ContentWidth, area.ContentHeight = listContentSize(list.Axis, list.ItemSize, len(list.Items))
	clip := world.GetMut[scene.ClipRect2D](entity)
	prepareScrollArea(world, entity, area, clip)

	offset, _ := GetScrollAreaOffset(world, entity)
	metrics := measureScrollArea(area.Width, area.Height, area.ContentWidth, area.ContentHeight, area.BarSize)
	layout := newListLayout(list.Axis, list.ItemSize, metrics, offset.X(), offset.Y())
	first := listFirstIndex(layout.scroll, list.ItemSize, list.Overscan, len(list.Items), len(list.itemEntities))
	last := min(first+len(list.itemEntities), len(list.Items))
	list.First, list.Last = first, last
	items := list.Items
	bindItem := list.BindItem
	unbindItem := list.UnbindItem
	itemEntities := list.itemEntities
	for slot, itemEntity := range itemEntities {
		index := first + slot
		visible := index < len(items)
		setVisible(world, itemEntity, visible)
		item := world.GetMut[ListItem[T]](itemEntity)
		content := world.GetMut[ScrollContent](itemEntity)
		transform := world.GetMut[scene.UITransform](itemEntity)
		x, y, width, height := layout.itemBounds(float32(index) * list.ItemSize)
		if target := world.GetMut[scene.InputTarget2D](itemEntity); target != nil {
			target.Width, target.Height = width, height
		}
		if size := world.GetMut[scene.Size2D](itemEntity); size != nil {
			size.Width, size.Height = width, height
		}
		if !visible {
			if item != nil {
				if item.Index >= 0 && unbindItem != nil {
					unbindItem(world, itemEntity)
				}
				var zero T
				item.Index, item.Value = -1, zero
			}
			continue
		}
		if item != nil {
			item.Index, item.Value = index, items[index]
		}
		if content != nil {
			content.X, content.Y = x, y
		}
		if transform != nil {
			transform.Translation.X.Value = x - layout.scrollX
			transform.Translation.Y.Value = y - layout.scrollY
		}
		if bindItem != nil {
			bindItem(world, itemEntity, index, items[index])
		}
	}
}

type listLayout struct {
	horizontal       bool
	itemSize         float32
	crossSize        float32
	scroll           float32
	scrollX, scrollY float32
}

func newListLayout(axis ListAxis, itemSize float32, metrics ScrollMetrics, scrollX, scrollY float32) listLayout {
	if axis == ListHorizontal {
		return listLayout{horizontal: true, itemSize: itemSize, crossSize: metrics.ViewportHeight, scroll: scrollX, scrollX: scrollX}
	}
	return listLayout{itemSize: itemSize, crossSize: metrics.ViewportWidth, scroll: scrollY, scrollY: scrollY}
}

func (layout listLayout) itemBounds(position float32) (x, y, width, height float32) {
	if layout.horizontal {
		return position, 0, layout.itemSize, layout.crossSize
	}
	return 0, position, layout.crossSize, layout.itemSize
}

func normalizeListAxis(axis ListAxis) ListAxis {
	if axis == ListHorizontal {
		return ListHorizontal
	}
	return ListVertical
}

func normalizeListItemSize(size float32) float32 {
	if size > 0 {
		return size
	}
	return defaultListItemSize
}

func listContentSize(axis ListAxis, itemSize float32, itemCount int) (width, height float32) {
	size := itemSize * float32(itemCount)
	if axis == ListHorizontal {
		return size, 0
	}
	return 0, size
}

func listViewportSize(axis ListAxis, width, height float32) float32 {
	if axis == ListHorizontal {
		return width
	}
	return height
}

func listItemDimensions(axis ListAxis, itemSize, width, height float32) (itemWidth, itemHeight float32) {
	if axis == ListHorizontal {
		return itemSize, height
	}
	return width, itemSize
}

func listOverscan(overscan int) int {
	if overscan > 0 {
		return overscan
	}
	return defaultListOverscan
}

func listItemPoolSize(viewportSize, itemSize float32, overscan int) int {
	visible := int(math.Ceil(float64(max(viewportSize, 0) / itemSize)))
	return visible + 1 + overscan*2
}

func listFirstIndex(scroll, itemSize float32, overscan, itemCount, poolSize int) int {
	first := max(int(math.Floor(float64(scroll/itemSize)))-overscan, 0)
	return min(first, max(itemCount-poolSize, 0))
}
