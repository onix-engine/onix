package html

import (
	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/html/layout"
	"github.com/onix-engine/onix/scene"
	uinode "github.com/onix-engine/onix/scene/node"
)

// HTMLSliceCapacities reports the retained slice capacities owned by one HTML
// surface. The values describe backing storage, so they remain useful after a
// partial update has reduced the current lengths.
type HTMLSliceCapacities struct {
	DOMChildren         int
	BoxChildren         int
	TextFragments       int
	TextGlyphs          int
	ImageFragments      int
	ControlFragments    int
	LayoutChanged       int
	LayoutRemoved       int
	LayoutReordered     int
	PaintChildren       int
	PaintQueue          int
	PaintCullRoots      int
	PaintCullGroups     int
	PaintCullNodes      int
	RenderEntities      int
	EntityBindings      int
	TextureUnused       int
	InlineBackgrounds   int
	BoxEntities         int
	BoxGroups           int
	ScrollPaintRows     int
	PaintShapeFragments int
	PaintListBoxes      int
	PaintListFree       int
	PaintListGlyphs     int
	FocusTargets        int
	StyleChanges        int
	MutationRoots       int
	LayoutNodes         int
	LayoutOrderNodes    int
	RemovedBoxes        int
}

// HTMLMetrics is a retained-memory and extraction snapshot for one HTML
// surface. RenderItems is the number of 2D items in the latest extraction
// whose source entity belongs to this surface. TexturePixels counts unique
// surface-owned textures at physical resolution. Shared images and font atlases
// are accounted for by their providers.
type HTMLMetrics struct {
	DOMNodes      int
	Boxes         int
	Fragments     int
	ECSChildren   int
	RenderItems   int
	PaintBoxes    int
	TexturePixels uint64
	Slices        HTMLSliceCapacities
}

// Metrics returns the current retained HTML metrics for owner. It performs no
// allocations and returns zero values when owner has no HTML render state.
// It scans the current document, layout, and last extracted scene on demand.
func Metrics(world *ecs.World, owner ecs.Entity) HTMLMetrics {
	if world == nil {
		return HTMLMetrics{}
	}
	state := world.GetRef[htmlRenderState](owner)
	if state == nil {
		return HTMLMetrics{}
	}
	metrics := HTMLMetrics{ECSChildren: len(state.Children)}
	metrics.Slices = htmlSliceCapacities(state)
	metrics.DOMNodes = measureHTMLNodes(state.Root, &metrics.Slices)
	metrics.Boxes, metrics.Fragments = measureHTMLBoxes(state.Boxes, &metrics.Slices)
	metrics.TexturePixels = measureHTMLTexturePixels(state)
	metrics.RenderItems = measureHTMLRenderItems(world, owner)
	if state.PaintList != nil {
		metrics.PaintBoxes = state.PaintList.Len()
		metrics.Slices.PaintListBoxes, metrics.Slices.PaintListFree, metrics.Slices.PaintListGlyphs = state.PaintList.Capacity()
	}
	for _, box := range state.BoxRenderings {
		metrics.Slices.BoxEntities += cap(box.Entities)
		metrics.Slices.BoxGroups += cap(box.Groups)
		if box.PaintBand != nil {
			metrics.Slices.ScrollPaintRows += cap(box.PaintBand.rows)
		}
	}
	for _, node := range state.RenderNodes {
		metrics.Slices.RenderEntities += cap(node.Entities)
	}
	for _, group := range state.EntityGroups {
		metrics.Slices.EntityBindings += cap(group.Bindings)
	}
	for _, entity := range state.Children {
		if paint := world.GetRef[scene.UIPaint2D](entity); paint != nil {
			metrics.Slices.PaintShapeFragments += cap(paint.Fragments())
		}
	}
	return metrics
}

func measureHTMLNodes(node *uinode.Node, capacities *HTMLSliceCapacities) int {
	if node == nil {
		return 0
	}
	count := 1
	capacities.DOMChildren += cap(node.Children)
	for _, child := range node.Children {
		count += measureHTMLNodes(child, capacities)
	}
	return count
}

func measureHTMLBoxes(box *layout.Box, capacities *HTMLSliceCapacities) (boxes, fragments int) {
	if box == nil {
		return 0, 0
	}
	boxes = 1
	fragments = len(box.TextFragments) + len(box.ImageFragments) + len(box.ControlFragments) + len(box.InlineBackgrounds)
	capacities.BoxChildren += cap(box.Children)
	capacities.InlineBackgrounds += cap(box.InlineBackgrounds)
	capacities.TextFragments += cap(box.TextFragments)
	capacities.ImageFragments += cap(box.ImageFragments)
	capacities.ControlFragments += cap(box.ControlFragments)
	for _, fragment := range box.TextFragments {
		capacities.TextGlyphs += cap(fragment.Glyphs)
	}
	for _, fragment := range box.ControlFragments {
		fragments += len(fragment.TextFragments) + len(fragment.ImageFragments)
		capacities.TextFragments += cap(fragment.TextFragments)
		capacities.ImageFragments += cap(fragment.ImageFragments)
		for _, text := range fragment.TextFragments {
			capacities.TextGlyphs += cap(text.Glyphs)
		}
	}
	for _, child := range box.Children {
		childBoxes, childFragments := measureHTMLBoxes(child, capacities)
		boxes += childBoxes
		fragments += childFragments
	}
	return boxes, fragments
}

func measureHTMLTexturePixels(state *htmlRenderState) uint64 {
	var pixels uint64
	for texture := range state.Textures.textures {
		pixels += uint64(texture.Width) * uint64(texture.Height)
	}
	for _, scratch := range [...]htmlGPUBlurScratch{state.Textures.blurScratch, state.Textures.shadowScratch} {
		if scratch.target != nil {
			width, height := scratch.target.Size()
			pixels += uint64(width) * uint64(height)
		}
	}
	return pixels
}

func measureHTMLRenderItems(world *ecs.World, owner ecs.Entity) int {
	renderState, ok := world.GetResource[*scene.RenderWorld]()
	if !ok || renderState == nil {
		return 0
	}
	count := 0
	for index := range renderState.Items2D {
		entity := renderState.Items2D[index].Entity()
		if entity == owner {
			count++
		} else if child := world.GetRef[htmlChild](entity); child != nil && child.Owner == owner {
			count++
		}
	}
	return count
}

func htmlSliceCapacities(state *htmlRenderState) HTMLSliceCapacities {
	return HTMLSliceCapacities{
		LayoutChanged:    cap(state.LayoutCache.Changed),
		LayoutRemoved:    cap(state.LayoutCache.Removed),
		LayoutReordered:  cap(state.LayoutCache.Reordered),
		PaintChildren:    cap(state.Children),
		PaintQueue:       cap(state.RenderQueue),
		PaintCullRoots:   cap(state.PaintCullRoots),
		PaintCullGroups:  cap(state.PaintCullGroups),
		PaintCullNodes:   cap(state.PaintCullNodes),
		TextureUnused:    cap(state.Textures.unused),
		FocusTargets:     cap(state.FocusTargets),
		StyleChanges:     cap(state.StyleChanges),
		MutationRoots:    cap(state.MutationRoots),
		LayoutNodes:      cap(state.LayoutNodes),
		LayoutOrderNodes: cap(state.LayoutOrderNodes),
		RemovedBoxes:     cap(state.RemovedBoxes),
	}
}
