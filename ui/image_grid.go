package ui

import (
	"slices"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene"
)

const (
	defaultImageGridColumns         = 3
	defaultImageGridGap     float32 = 8
	defaultImageGridPadding float32 = 6
	defaultImageGridLabel   float32 = 20
)

// ImageGridItem describes one image card in an ImageGrid.
type ImageGridItem struct {
	Texture   *graphics.Texture
	Label     string
	Removable bool
}

// ImageGrid owns a fixed collection of image cards. Recreate it when the
// item list or layout changes; moving its entity updates the whole grid.
type ImageGrid struct {
	Width       float32
	Columns     int
	Gap         float32
	Padding     float32
	LabelHeight float32
	DeleteIcon  IconStyle
	Items       []ImageGridItem
}

// ImageGridOptions configures an ImageGrid.
type ImageGridOptions struct {
	X, Y        float32
	Width       float32
	Columns     int
	Gap         float32
	Padding     float32
	LabelHeight float32
	DeleteIcon  IconStyle
	Items       []ImageGridItem
	Radius      CornerRadius
	PassThrough bool
	Theme       Theme
	Style       *ImageGridStyle
}

// ImageGridEvent reports a remove request for a card.
type ImageGridEvent struct {
	Grid  ecs.Entity
	Index int
}

type imageGridCell struct {
	Grid  ecs.Entity
	Index int
}

type imageGridRemove struct {
	Grid  ecs.Entity
	Index int
}

type imageGridRect struct{ X, Y, W, H float32 }

// ImageGridCellSize returns the square card size for a grid width.
func ImageGridCellSize(width float32, columns int, gap float32) float32 {
	columns = imageGridColumns(columns)
	gap = imageGridGap(gap)
	return max((width-gap*float32(columns-1))/float32(columns), 0)
}

// ImageGridHeight returns the rendered height of an image grid.
func ImageGridHeight(width float32, itemCount, columns int, gap float32) float32 {
	if itemCount <= 0 {
		return 0
	}
	columns = imageGridColumns(columns)
	gap = imageGridGap(gap)
	cell := ImageGridCellSize(width, columns, gap)
	rows := (itemCount + columns - 1) / columns
	return float32(rows)*cell + float32(rows-1)*gap
}

// SpawnImageGrid creates a thumbnail grid. The grid emits ImageGridEvent and
// EventImageGridRemove when a removable card's close control is activated.
func SpawnImageGrid(world *ecs.World, opts ImageGridOptions) ecs.Entity {
	theme := themeForWorld(world, opts.Theme)
	deleteIcon := iconWithFallback(opts.DeleteIcon, "x")
	columns := imageGridColumns(opts.Columns)
	gap := imageGridGap(opts.Gap)
	padding := imageGridPadding(opts.Padding)
	labelHeight := imageGridLabelHeight(opts.LabelHeight)
	grid := spawnAnchor(world, opts.X, opts.Y)
	_ = ecs.Insert(world, grid, ImageGrid{
		Width: opts.Width, Columns: columns, Gap: gap, Padding: padding, LabelHeight: labelHeight, DeleteIcon: opts.DeleteIcon,
		Items: slices.Clone(opts.Items),
	})
	putStyleOverride(world, grid, opts.Style)

	cellSize := ImageGridCellSize(opts.Width, columns, gap)
	for index, item := range opts.Items {
		column, row := index%columns, index/columns
		cell := spawnBox(world, float32(column)*(cellSize+gap), float32(row)*(cellSize+gap), cellSize, cellSize, theme.StackIndex, theme.Surface)
		setCornerRadius(world, cell, opts.Radius)
		_ = ecs.Insert(world, cell, scene.Parent{Entity: grid})
		_ = ecs.Insert(world, cell, imageGridCell{Grid: grid, Index: index})

		imageBounds := imageGridTextureBounds(cellSize, padding, labelHeight, item.Texture)
		if item.Texture != nil && imageBounds.W > 0 && imageBounds.H > 0 {
			image := spawnBox(world, imageBounds.X, imageBounds.Y, imageBounds.W, imageBounds.H, theme.StackIndex+2, graphics.ColorRGBA{})
			_ = ecs.Insert(world, image, scene.Parent{Entity: cell})
			if visual := ecs.GetMut[scene.UIImage2D](world, image); visual != nil {
				*visual = scene.NewUIImage2D(item.Texture)
			}
		}
		if item.Label != "" {
			label := spawnText[LabelPart](world, cell, padding, cellSize-labelHeight+2, item.Label, theme)
			_ = ecs.Insert(world, label, Label{Value: item.Label, Color: theme.Muted, FontSize: min(theme.FontSize, 12), DPI: theme.DPI})
			if text := ecs.GetMut[scene.Text2D](world, label); text != nil {
				text.FontSize = min(theme.FontSize, 12)
				text.Color = theme.Muted
				text.MaxWidth = max(cellSize-padding*2, 0)
			}
		}
		if item.Removable {
			removeSize := min(max(cellSize*.23, 18), 24)
			remove := spawnInteractiveBox(world, cellSize-removeSize-padding*.5, padding*.5, removeSize, removeSize, theme.StackIndex+3, graphics.ColorRGBA{}, false, opts.PassThrough)
			setCornerRadius(world, remove, Circle)
			_ = ecs.Insert(world, remove, scene.Parent{Entity: cell})
			_ = ecs.Insert(world, remove, imageGridRemove{Grid: grid, Index: index})
			if deleteIcon.Texture != nil {
				iconWidth, iconHeight := fitIconSize(deleteIcon, min(removeSize*.62, 14), min(removeSize*.62, 14))
				icon := spawnBox(world, (removeSize-iconWidth)*.5, (removeSize-iconHeight)*.5, iconWidth, iconHeight, theme.StackIndex+5, graphics.ColorRGBA{})
				_ = ecs.Insert(world, icon, scene.Parent{Entity: remove})
				if visual := ecs.GetMut[scene.UIImage2D](world, icon); visual != nil {
					*visual = scene.NewUIImage2D(deleteIcon.Texture)
				}
			} else {
				close := spawnText[LabelPart](world, remove, 0, 0, iconFallback(deleteIcon), theme)
				_ = ecs.Insert(world, close, Label{Value: iconFallback(deleteIcon), Color: theme.Muted, FontSize: min(theme.FontSize, 13), DPI: theme.DPI})
				if text := ecs.GetMut[scene.Text2D](world, close); text != nil {
					text.FontSize = min(theme.FontSize, 13)
					text.Color = theme.Muted
				}
				centerTextVertically(world, close, removeSize)
				if transform := ecs.GetMut[scene.UITransform](world, close); transform != nil {
					if text, ok := ecs.Get[scene.Text2D](world, close); ok {
						width, _, measured := scene.MeasureTextValue2D(&text, text.Value)
						if measured {
							transform.Translation.X.Value = (removeSize - width) * .5
						}
					}
				}
			}
		}
	}
	return grid
}

func imageGridColumns(columns int) int {
	if columns <= 0 {
		return defaultImageGridColumns
	}
	return columns
}

func imageGridGap(gap float32) float32 {
	if gap <= 0 {
		return defaultImageGridGap
	}
	return gap
}

func imageGridPadding(padding float32) float32 {
	if padding <= 0 {
		return defaultImageGridPadding
	}
	return padding
}

func imageGridLabelHeight(height float32) float32 {
	if height <= 0 {
		return defaultImageGridLabel
	}
	return height
}

func imageGridTextureBounds(cellSize, padding, labelHeight float32, texture *graphics.Texture) imageGridRect {
	area := imageGridRect{X: padding, Y: padding, W: max(cellSize-padding*2, 0), H: max(cellSize-padding-labelHeight, 0)}
	if texture == nil || texture.LogicalWidth() <= 0 || texture.LogicalHeight() <= 0 || area.W == 0 || area.H == 0 {
		return area
	}
	scale := min(area.W/float32(texture.LogicalWidth()), area.H/float32(texture.LogicalHeight()))
	width, height := float32(texture.LogicalWidth())*scale, float32(texture.LogicalHeight())*scale
	return imageGridRect{X: area.X + (area.W-width)*.5, Y: area.Y + (area.H-height)*.5, W: width, H: height}
}
