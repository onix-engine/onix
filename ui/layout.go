package ui

import (
	"cmp"
	"math"
	"slices"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/internal/reuse"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

// LayoutType selects the arrangement performed by Layout.
type LayoutType uint8

const (
	LayoutNone LayoutType = iota
	LayoutHorizontal
	LayoutVertical
	LayoutGrid
)

// LayoutResizeMode controls whether a layout changes its children or itself.
type LayoutResizeMode uint8

const (
	LayoutResizeNone LayoutResizeMode = iota
	LayoutResizeChildren
	LayoutResizeContainer
)

type LayoutHorizontalDirection uint8

const (
	LayoutLeftToRight LayoutHorizontalDirection = iota
	LayoutRightToLeft
)

type LayoutVerticalDirection uint8

const (
	LayoutTopToBottom LayoutVerticalDirection = iota
	LayoutBottomToTop
)

type LayoutStartAxis uint8

const (
	LayoutStartHorizontal LayoutStartAxis = iota
	LayoutStartVertical
)

type LayoutConstraint uint8

const (
	LayoutConstraintNone LayoutConstraint = iota
	LayoutConstraintFixedRow
	LayoutConstraintFixedColumn
)

// Layout is a UI container layout component. Children are the entities that
// carry Parent{Entity: owner}; authored SiblingOrder values preserve sibling
// order and Entity values provide a deterministic runtime fallback.
// PaddingLeft/Right/Top/Bottom are in logical pixels. Padding can be used when
// percentage or viewport-relative padding is desired.
type Layout struct {
	Type                LayoutType                `json:"type,omitempty"`
	ResizeMode          LayoutResizeMode          `json:"resizeMode,omitempty"`
	PaddingLeft         float32                   `json:"paddingLeft,omitempty"`
	PaddingRight        float32                   `json:"paddingRight,omitempty"`
	PaddingTop          float32                   `json:"paddingTop,omitempty"`
	PaddingBottom       float32                   `json:"paddingBottom,omitempty"`
	Padding             scene.UIRect              `json:"padding,omitzero"`
	SpacingX            float32                   `json:"spacingX,omitempty"`
	SpacingY            float32                   `json:"spacingY,omitempty"`
	HorizontalDirection LayoutHorizontalDirection `json:"horizontalDirection,omitempty"`
	VerticalDirection   LayoutVerticalDirection   `json:"verticalDirection,omitempty"`
	CellSize            mgl32.Vec2                `json:"cellSize,omitzero"`
	StartAxis           LayoutStartAxis           `json:"startAxis,omitempty"`
	AffectedByScale     bool                      `json:"affectedByScale,omitempty"`
	AutoAlignment       bool                      `json:"autoAlignment,omitempty"`
	Constraint          LayoutConstraint          `json:"constraint,omitempty"`
	ConstraintNum       int                       `json:"constraintNum,omitempty"`
}

func NewLayout() Layout {
	return Layout{Type: LayoutNone, AutoAlignment: true, AffectedByScale: true}
}

// WidgetAlignMode determines how often a Widget recomputes its constraints.
type WidgetAlignMode uint8

const (
	WidgetAlignAlways WidgetAlignMode = iota
	WidgetAlignOnce
	WidgetAlignOnWindowResize
)

// Widget pins a UI node to its parent's constraint box. A zero Target uses the
// entity's Parent; an explicit Target supports pinning to another UI node.
// TargetID is the stable scene-facing form of Target. Margins use Val, so Px,
// Percent and viewport units can be mixed.
type Widget struct {
	AlignLeft   bool            `json:"alignLeft,omitempty"`
	AlignRight  bool            `json:"alignRight,omitempty"`
	AlignTop    bool            `json:"alignTop,omitempty"`
	AlignBottom bool            `json:"alignBottom,omitempty"`
	Left        scene.Val       `json:"left,omitzero"`
	Right       scene.Val       `json:"right,omitzero"`
	Top         scene.Val       `json:"top,omitzero"`
	Bottom      scene.Val       `json:"bottom,omitzero"`
	AlignMode   WidgetAlignMode `json:"alignMode,omitempty"`
	Target      ecs.Entity      `json:"-"`
	TargetID    string          `json:"target,omitempty"`
	Enabled     bool            `json:"enabled,omitempty"`
}

func NewWidget() Widget {
	return Widget{Enabled: true}
}

type uiLayoutChild struct {
	parent ecs.Entity
	child  ecs.Entity
	order  int
}

type uiLayoutNode struct {
	entity ecs.Entity
	depth  int
}

type uiLayoutScratch struct {
	scope              *ecs.QueryScope
	scopeVersion       uint64
	initialized        bool
	childInputs        []uiLayoutChild
	nodeInputs         []ecs.Entity
	children           []uiLayoutChild
	nodes              []uiLayoutNode
	layoutChildren     []ecs.Entity
	depths             map[ecs.Entity]int
	visitingDepths     map[ecs.Entity]bool
	depthPath          []ecs.Entity
	hierarchyVersions  [4]uint64
	hierarchyStructure uint64
	versions           [10]uint64
	structure          uint64
	viewport           scene.ViewportSize
	runtime            map[ecs.Entity]*uiLayoutRuntimeState
	runtimeInitialized bool
	pending            bool
	recomputed         int
}

type uiLayoutSizeInput struct {
	entity        ecs.Entity
	width, height float32
	valid         bool
	anchor        scene.Anchor
	transform     scene.UITransform
}

type uiLayoutRuntimeState struct {
	layout      Layout
	parent      uiLayoutSizeInput
	children    []uiLayoutSizeInput
	viewport    scene.ViewportSize
	initialized bool
}

func uiLayoutVersions(world *ecs.World) [10]uint64 {
	return [10]uint64{ecs.ComponentVersion[Layout](world), ecs.ComponentVersion[Widget](world), ecs.ComponentVersion[scene.Parent](world), ecs.ComponentVersion[scene.SiblingOrder](world),
		ecs.ComponentVersion[scene.Size2D](world), ecs.ComponentVersion[scene.UITransform](world), ecs.ComponentVersion[scene.Anchor](world), ecs.ComponentVersion[scene.Sprite2D](world), ecs.ComponentVersion[scene.Text2D](world), ecs.ComponentVersion[scene.ClipRect2D](world)}
}

func uiLayoutInput(world *ecs.World, entity ecs.Entity, scaled bool) uiLayoutSizeInput {
	width, height, valid := uiNodeSize(world, entity, scaled)
	anchor, _ := ecs.Get[scene.Anchor](world, entity)
	transform, _ := ecs.Get[scene.UITransform](world, entity)
	return uiLayoutSizeInput{entity, width, height, valid, anchor, transform}
}

func captureUILayoutInputs(world *ecs.World, state *uiLayoutRuntimeState, entity ecs.Entity, layout Layout, children []ecs.Entity, viewport scene.ViewportSize) bool {
	parent := uiLayoutInput(world, entity, layout.AffectedByScale)
	dirty := !state.initialized || state.layout != layout || state.parent != parent || state.viewport != viewport
	for index, child := range children {
		dirty = reuse.UpdateAt(&state.children, index, uiLayoutInput(world, child, layout.AffectedByScale)) || dirty
	}
	dirty = len(state.children) != len(children) || dirty
	state.children = state.children[:len(children)]
	state.layout, state.parent, state.viewport, state.initialized = layout, parent, viewport, true
	return dirty
}

type uiLayoutScratchTag struct{}

type uiWidgetRuntimeState struct {
	config             Widget
	lastParentWidth    float32
	lastParentHeight   float32
	lastViewportWidth  float32
	lastViewportHeight float32
	aligned            bool
}

type uiWidgetRuntimeStates struct {
	values map[ecs.Entity]uiWidgetRuntimeState
}

type uiWidgetRuntimeStateTag struct{}

// SyncLayoutSystem applies Layout and Widget components before UI global
// transforms are propagated. It is intentionally independent of the higher
// level ui widget runtime so scenes can use these components on their own.
func SyncLayoutSystem(name string) ecs.SystemConfig {
	if name == "" {
		name = "sync_ui_layout"
	}
	return ecs.SystemConfig{Name: name, System: syncUILayouts}
}

// UpdateLayout immediately applies all Layout and Widget components in world.
// It is the programmatic equivalent of waiting for the next UI sync frame.
func UpdateLayout(world *ecs.World) error {
	return SyncLayoutSystem("").Run(world)
}

func syncUILayouts(context *ecs.SystemContext) error {
	if context == nil || context.World == nil {
		return nil
	}
	world := context.World
	scratch := ecs.ScopeValue[uiLayoutScratchTag, uiLayoutScratch](world)
	scope, version := world.QueryScopeKey()
	if scratch.scope != scope || scratch.scopeVersion != version {
		scratch.initialized, scratch.runtimeInitialized = false, false
		scratch.scope, scratch.scopeVersion = scope, version
	}
	scratch.recomputed = 0
	versions := uiLayoutVersions(world)
	viewport, _ := ecs.GetResource[scene.ViewportSize](world)
	if scratch.runtimeInitialized && scratch.structure == world.StructureVersion() && scratch.versions == versions && scratch.viewport == viewport && !scratch.pending {
		return nil
	}
	refreshUILayoutHierarchy(world, scratch)
	if scratch.runtime == nil {
		scratch.runtime = make(map[ecs.Entity]*uiLayoutRuntimeState)
	}
	for entity := range scratch.runtime {
		if !world.IsAlive(entity) || !ecs.Has[Layout](world, entity) || !ecs.Has[scene.UITransform](world, entity) {
			delete(scratch.runtime, entity)
		}
	}
	scratch.layoutChildren = scratch.layoutChildren[:0]
	// Resolve parent constraints first so a layout can immediately consume a
	// Widget-stretched container. A second pass below resolves widgets on the
	// children after their new layout positions and sizes are known.
	syncUIWidgets(world)
	for _, node := range scratch.nodes {
		layout, ok := ecs.Get[Layout](world, node.entity)
		if !ok || layout.Type == LayoutNone {
			continue
		}
		start, end := layoutChildRange(scratch.children, node.entity)
		scratch.layoutChildren = scratch.layoutChildren[:0]
		for _, child := range scratch.children[start:end] {
			scratch.layoutChildren = append(scratch.layoutChildren, child.child)
		}
		state := scratch.runtime[node.entity]
		if state == nil {
			state = &uiLayoutRuntimeState{}
			scratch.runtime[node.entity] = state
		}
		if captureUILayoutInputs(world, state, node.entity, layout, scratch.layoutChildren, viewport) {
			applyUILayout(world, node.entity, layout, scratch.layoutChildren)
			scratch.recomputed++
			// Resizing and positioning write inputs consumed by later layout passes.
			captureUILayoutInputs(world, state, node.entity, layout, scratch.layoutChildren, viewport)
		}
	}
	beforeWidgets := uiLayoutVersions(world)
	syncUIWidgets(world)
	scratch.versions = uiLayoutVersions(world)
	scratch.pending = scratch.recomputed != 0 || beforeWidgets != scratch.versions
	scratch.structure, scratch.viewport, scratch.runtimeInitialized = world.StructureVersion(), viewport, true
	return nil
}

func refreshUILayoutHierarchy(world *ecs.World, scratch *uiLayoutScratch) bool {
	versions := [4]uint64{ecs.ComponentVersion[scene.Parent](world), ecs.ComponentVersion[scene.SiblingOrder](world), ecs.ComponentStructureVersion[Layout](world), ecs.ComponentStructureVersion[scene.UITransform](world)}
	if scratch.initialized && scratch.hierarchyStructure == world.StructureVersion() && scratch.hierarchyVersions == versions {
		return false
	}
	scratch.hierarchyStructure, scratch.hierarchyVersions = world.StructureVersion(), versions
	dirty := !scratch.initialized
	childCount := 0
	for child, access := range ecs.Query[scene.Parent](world).All {
		parent := access.Get()

		if parent.Entity != 0 && world.IsAlive(parent.Entity) {
			order := int(child)
			if authoredOrder, ok := ecs.Get[scene.SiblingOrder](world, child); ok {
				order = int(authoredOrder)
			}
			input := uiLayoutChild{parent: parent.Entity, child: child, order: order}
			dirty = reuse.UpdateAt(&scratch.childInputs, childCount, input) || dirty
			childCount++
		}
	}
	dirty = childCount != len(scratch.childInputs) || dirty
	scratch.childInputs = scratch.childInputs[:childCount]
	nodeCount := 0
	for entity, _ := range ecs.Query2[Layout, scene.UITransform](world).All {
		dirty = reuse.UpdateAt(&scratch.nodeInputs, nodeCount, entity) || dirty
		nodeCount++
	}
	dirty = nodeCount != len(scratch.nodeInputs) || dirty
	scratch.nodeInputs = scratch.nodeInputs[:nodeCount]
	if !dirty {
		return false
	}
	scratch.initialized = true
	scratch.children = append(scratch.children[:0], scratch.childInputs...)
	scratch.nodes = scratch.nodes[:0]
	if scratch.depths == nil {
		scratch.depths = make(map[ecs.Entity]int)
	} else {
		clear(scratch.depths)
	}
	if scratch.visitingDepths == nil {
		scratch.visitingDepths = make(map[ecs.Entity]bool)
	} else {
		clear(scratch.visitingDepths)
	}
	scratch.depthPath = scratch.depthPath[:0]
	for _, entity := range scratch.nodeInputs {
		scratch.nodes = append(scratch.nodes, uiLayoutNode{entity: entity, depth: uiHierarchyDepth(world, entity, scratch.depths, scratch.visitingDepths, &scratch.depthPath)})
	}
	slices.SortFunc(scratch.children, func(a, b uiLayoutChild) int {
		if value := cmp.Compare(a.parent, b.parent); value != 0 {
			return value
		}
		if value := cmp.Compare(a.order, b.order); value != 0 {
			return value
		}
		return cmp.Compare(a.child, b.child)
	})
	slices.SortFunc(scratch.nodes, func(a, b uiLayoutNode) int {
		if value := cmp.Compare(a.depth, b.depth); value != 0 {
			return value
		}
		return cmp.Compare(a.entity, b.entity)
	})
	return true
}

func uiHierarchyDepth(world *ecs.World, entity ecs.Entity, depths map[ecs.Entity]int, visiting map[ecs.Entity]bool, path *[]ecs.Entity) int {
	if depth, ok := depths[entity]; ok {
		return depth
	}
	start := len(*path)
	current := entity
	depth := 0
	for remaining := world.EntityCount() + 1; current != 0 && remaining > 0; remaining-- {
		if cached, ok := depths[current]; ok {
			depth = cached
			break
		}
		if visiting[current] {
			depth = world.EntityCount() + 1
			break
		}
		visiting[current] = true
		*path = append(*path, current)
		parent, ok := ecs.Get[scene.Parent](world, current)
		if !ok || parent.Entity == 0 || !world.IsAlive(parent.Entity) {
			break
		}
		current = parent.Entity
	}
	for index := len(*path) - 1; index >= start; index-- {
		value := (*path)[index]
		depths[value] = depth
		delete(visiting, value)
		depth++
	}
	result := depths[entity]
	*path = (*path)[:start]
	return result
}

func layoutChildRange(children []uiLayoutChild, parent ecs.Entity) (int, int) {
	start, limit := 0, len(children)
	for start < limit {
		middle := int(uint(start+limit) >> 1)
		if children[middle].parent < parent {
			start = middle + 1
		} else {
			limit = middle
		}
	}
	end, limit := start, len(children)
	for end < limit {
		middle := int(uint(end+limit) >> 1)
		if children[middle].parent <= parent {
			end = middle + 1
		} else {
			limit = middle
		}
	}
	return start, end
}

func applyUILayout(world *ecs.World, entity ecs.Entity, layout Layout, children []ecs.Entity) {
	if len(children) == 0 && layout.ResizeMode != LayoutResizeContainer {
		return
	}
	parentWidth, parentHeight, ok := uiNodeSize(world, entity, layout.AffectedByScale)
	if !ok {
		return
	}
	padding := layoutPadding(world, entity, layout, parentWidth, parentHeight)
	cellSize := layout.CellSize
	if layout.Type == LayoutGrid {
		cellSize = uiGridCellSize(world, layout, children)
	}
	if layout.ResizeMode == LayoutResizeContainer {
		width, height := uiLayoutContainerSize(world, layout, children, padding, parentWidth, parentHeight, cellSize)
		if size := ecs.GetRef[scene.Size2D](world, entity); size != nil {
			if size.Width != width || size.Height != height {
				value := ecs.GetMut[scene.Size2D](world, entity)
				value.Width, value.Height = width, height
			}
			parentWidth, parentHeight = width, height
		} else {
			_ = ecs.Insert(world, entity, scene.Size2D{Width: width, Height: height})
			parentWidth, parentHeight = width, height
		}
		padding = layoutPadding(world, entity, layout, parentWidth, parentHeight)
	}
	originX, originY := uiNodeOrigin(world, entity, parentWidth, parentHeight)

	if layout.Type == LayoutGrid {
		applyUIGridLayout(world, layout, children, parentWidth, parentHeight, padding, originX, originY, cellSize)
		return
	}
	if layout.Type == LayoutHorizontal {
		applyUIHorizontalLayout(world, layout, children, parentWidth, parentHeight, padding, originX, originY)
		return
	}
	if layout.Type == LayoutVertical {
		applyUIVerticalLayout(world, layout, children, parentWidth, parentHeight, padding, originX, originY)
	}
}

func layoutPadding(world *ecs.World, entity ecs.Entity, layout Layout, width, height float32) scene.ResolvedUIRect {
	viewport := mgl32.Vec2{width, height}
	scaleFactor := float32(1)
	if value, ok := ecs.GetResource[scene.ViewportSize](world); ok {
		viewport = mgl32.Vec2{value.Width, value.Height}
		scaleFactor = value.ScaleFactor
	}
	return scene.ResolvedUIRect{
		Left:   layoutPaddingValue(layout.Padding.Left, layout.PaddingLeft, width, scaleFactor, viewport),
		Right:  layoutPaddingValue(layout.Padding.Right, layout.PaddingRight, width, scaleFactor, viewport),
		Top:    layoutPaddingValue(layout.Padding.Top, layout.PaddingTop, height, scaleFactor, viewport),
		Bottom: layoutPaddingValue(layout.Padding.Bottom, layout.PaddingBottom, height, scaleFactor, viewport),
	}
}

func layoutPaddingValue(value scene.Val, pixels, base, scaleFactor float32, viewport mgl32.Vec2) float32 {
	if !value.IsZero() {
		resolved, ok := value.Resolve(scaleFactor, base, viewport)
		if ok {
			return resolved
		}
	}
	return pixels
}

func uiLayoutContainerSize(world *ecs.World, layout Layout, children []ecs.Entity, padding scene.ResolvedUIRect, parentWidth, parentHeight float32, cellSize mgl32.Vec2) (float32, float32) {
	if layout.Type == LayoutHorizontal {
		width, height := padding.Horizontal(), padding.Vertical()
		for index, child := range children {
			childWidth, childHeight, valid := uiNodeSize(world, child, layout.AffectedByScale)
			if !valid {
				continue
			}
			width += childWidth
			height = max(height, padding.Vertical()+childHeight)
			if index > 0 {
				width += max(layout.SpacingX, float32(0))
			}
		}
		return width, height
	}
	if layout.Type == LayoutVertical {
		width, height := padding.Horizontal(), padding.Vertical()
		for index, child := range children {
			childWidth, childHeight, valid := uiNodeSize(world, child, layout.AffectedByScale)
			if !valid {
				continue
			}
			width = max(width, padding.Horizontal()+childWidth)
			height += childHeight
			if index > 0 {
				height += max(layout.SpacingY, float32(0))
			}
		}
		return width, height
	}
	if layout.Type == LayoutGrid {
		columns, rows := uiGridDimensions(layout, len(children), parentWidth, parentHeight, padding, cellSize)
		cellWidth, cellHeight := cellSize.X(), cellSize.Y()
		return padding.Left + padding.Right + float32(columns)*cellWidth + float32(max(columns-1, 0))*layout.SpacingX,
			padding.Top + padding.Bottom + float32(rows)*cellHeight + float32(max(rows-1, 0))*layout.SpacingY
	}
	return parentWidth, parentHeight
}

func applyUIHorizontalLayout(world *ecs.World, layout Layout, children []ecs.Entity, width, height float32, padding scene.ResolvedUIRect, originX, originY float32) {
	available := max(width-padding.Horizontal()-float32(max(len(children)-1, 0))*layout.SpacingX, float32(0))
	factor := float32(0)
	resize := false
	if layout.ResizeMode == LayoutResizeChildren {
		total := float32(0)
		for _, child := range children {
			childWidth, _, valid := uiNodeSize(world, child, layout.AffectedByScale)
			if valid {
				total += childWidth
			}
		}
		if total > 0 {
			factor = available / total
			resize = true
		}
	}
	rightToLeft := layout.HorizontalDirection == LayoutRightToLeft
	position := originX + padding.Left
	if rightToLeft {
		position = originX + width - padding.Right
	}
	for _, child := range children {
		if resize {
			childWidth, childHeight, valid := uiNodeSize(world, child, false)
			if valid {
				setUILayoutSize(world, child, childWidth*factor, childHeight)
			}
		}
		childWidth, childHeight, valid := uiNodeSize(world, child, layout.AffectedByScale)
		if !valid {
			continue
		}
		top := originY + padding.Top
		if !layout.AutoAlignment {
			top = uiCurrentTop(world, child, childWidth, childHeight)
		}
		if rightToLeft {
			position -= childWidth
		}
		setUILayoutPosition(world, child, position, top, childWidth, childHeight)
		if !rightToLeft {
			position += childWidth + layout.SpacingX
		} else {
			position -= layout.SpacingX
		}
	}
}

func applyUIVerticalLayout(world *ecs.World, layout Layout, children []ecs.Entity, width, height float32, padding scene.ResolvedUIRect, originX, originY float32) {
	available := max(height-padding.Vertical()-float32(max(len(children)-1, 0))*layout.SpacingY, float32(0))
	factor := float32(0)
	resize := false
	if layout.ResizeMode == LayoutResizeChildren {
		total := float32(0)
		for _, child := range children {
			_, childHeight, valid := uiNodeSize(world, child, layout.AffectedByScale)
			if valid {
				total += childHeight
			}
		}
		if total > 0 {
			factor = available / total
			resize = true
		}
	}
	bottomToTop := layout.VerticalDirection == LayoutBottomToTop
	position := originY + padding.Top
	if bottomToTop {
		position = originY + height - padding.Bottom
	}
	for _, child := range children {
		if resize {
			childWidth, childHeight, valid := uiNodeSize(world, child, false)
			if valid {
				setUILayoutSize(world, child, childWidth, childHeight*factor)
			}
		}
		childWidth, childHeight, valid := uiNodeSize(world, child, layout.AffectedByScale)
		if !valid {
			continue
		}
		left := originX + padding.Left
		if !layout.AutoAlignment {
			left = uiCurrentLeft(world, child, childWidth, childHeight)
		}
		if bottomToTop {
			position -= childHeight
		}
		setUILayoutPosition(world, child, left, position, childWidth, childHeight)
		if !bottomToTop {
			position += childHeight + layout.SpacingY
		} else {
			position -= layout.SpacingY
		}
	}
}

// uiGridCellSize measures the children once, independently of container size.
func uiGridCellSize(world *ecs.World, layout Layout, children []ecs.Entity) mgl32.Vec2 {
	cellWidth, cellHeight := layout.CellSize.X(), layout.CellSize.Y()
	if cellWidth <= 0 || cellHeight <= 0 {
		for _, child := range children {
			width, height, valid := uiNodeSize(world, child, layout.AffectedByScale)
			if valid {
				cellWidth, cellHeight = max(cellWidth, width), max(cellHeight, height)
			}
		}
	}
	return mgl32.Vec2{cellWidth, cellHeight}
}

func uiGridDimensions(layout Layout, count int, width, height float32, padding scene.ResolvedUIRect, cellSize mgl32.Vec2) (int, int) {
	if count == 0 {
		return 0, 0
	}
	constraint := layout.ConstraintNum
	if constraint < 1 {
		constraint = 1
	}
	switch layout.Constraint {
	case LayoutConstraintFixedRow:
		return max((count+constraint-1)/constraint, 1), constraint
	case LayoutConstraintFixedColumn:
		return constraint, max((count+constraint-1)/constraint, 1)
	}
	cellWidth, cellHeight := cellSize.X(), cellSize.Y()
	columns := 1
	rows := count
	if layout.StartAxis == LayoutStartHorizontal {
		if cellWidth > 0 {
			columns = max(int(math.Floor(float64((width-padding.Horizontal()+layout.SpacingX)/(cellWidth+layout.SpacingX)))), 1)
		}
		columns = min(columns, count)
		rows = (count + columns - 1) / columns
	} else {
		if cellHeight > 0 {
			rows = max(int(math.Floor(float64((height-padding.Vertical()+layout.SpacingY)/(cellHeight+layout.SpacingY)))), 1)
		}
		rows = min(rows, count)
		columns = (count + rows - 1) / rows
	}
	return columns, rows
}

func applyUIGridLayout(world *ecs.World, layout Layout, children []ecs.Entity, width, height float32, padding scene.ResolvedUIRect, originX, originY float32, cellSize mgl32.Vec2) {
	columns, rows := uiGridDimensions(layout, len(children), width, height, padding, cellSize)
	cellWidth, cellHeight := cellSize.X(), cellSize.Y()
	for index, child := range children {
		column, row := index%columns, index/columns
		if layout.StartAxis == LayoutStartVertical {
			column, row = index/rows, index%rows
		}
		if layout.HorizontalDirection == LayoutRightToLeft {
			column = columns - 1 - column
		}
		if layout.VerticalDirection == LayoutBottomToTop {
			row = rows - 1 - row
		}
		childWidth, childHeight, valid := uiNodeSize(world, child, layout.AffectedByScale)
		if !valid {
			continue
		}
		if layout.ResizeMode == LayoutResizeChildren && layout.CellSize.X() > 0 && layout.CellSize.Y() > 0 {
			setUILayoutSize(world, child, layout.CellSize.X(), layout.CellSize.Y())
			childWidth, childHeight = layout.CellSize.X(), layout.CellSize.Y()
		}
		setUILayoutPosition(world, child,
			originX+padding.Left+float32(column)*(cellWidth+layout.SpacingX),
			originY+padding.Top+float32(row)*(cellHeight+layout.SpacingY), childWidth, childHeight)
	}
}

func uiNodeSize(world *ecs.World, entity ecs.Entity, affectedByScale bool) (float32, float32, bool) {
	var width, height float32
	if size, ok := ecs.Get[scene.Size2D](world, entity); ok {
		width, height = size.Width, size.Height
	} else if sprite := ecs.GetRef[scene.Sprite2D](world, entity); sprite != nil {
		width, height = sprite.Size()
	} else if text := ecs.GetRef[scene.Text2D](world, entity); text != nil {
		width, height, _ = scene.MeasureText2D(text)
	} else if clip, ok := ecs.Get[scene.ClipRect2D](world, entity); ok {
		width, height = clip.Width, clip.Height
	} else {
		return 0, 0, false
	}
	if affectedByScale {
		if transform, ok := ecs.Get[scene.UITransform](world, entity); ok {
			scaleX, scaleY := float32(math.Abs(float64(transform.Scale.X()))), float32(math.Abs(float64(transform.Scale.Y())))
			if scaleX > 0 {
				width *= scaleX
			}
			if scaleY > 0 {
				height *= scaleY
			}
		}
	}
	return max(width, float32(0)), max(height, float32(0)), true
}

func uiCurrentLeft(world *ecs.World, entity ecs.Entity, width, height float32) float32 {
	transform, ok := ecs.Get[scene.UITransform](world, entity)
	if !ok {
		return 0
	}
	anchor, _ := ecs.Get[scene.Anchor](world, entity)
	offsetX, _ := anchor.TopLeft(width, height, true)
	return transform.Translation.Resolve(scene.UIResolveContext{}).X() + offsetX
}

func uiCurrentTop(world *ecs.World, entity ecs.Entity, width, height float32) float32 {
	transform, ok := ecs.Get[scene.UITransform](world, entity)
	if !ok {
		return 0
	}
	anchor, _ := ecs.Get[scene.Anchor](world, entity)
	_, offsetY := anchor.TopLeft(width, height, true)
	return transform.Translation.Resolve(scene.UIResolveContext{}).Y() + offsetY
}

func setUILayoutPosition(world *ecs.World, entity ecs.Entity, left, top, width, height float32) {
	transform := ecs.GetRef[scene.UITransform](world, entity)
	if transform == nil {
		return
	}
	anchor, _ := ecs.Get[scene.Anchor](world, entity)
	offsetX, offsetY := anchor.TopLeft(width, height, true)
	translation := scene.Px2(left-offsetX, top-offsetY)
	if transform.Translation == translation {
		return
	}
	updated := ecs.GetMut[scene.UITransform](world, entity)
	updated.Translation = translation
}

func uiNodeOrigin(world *ecs.World, entity ecs.Entity, width, height float32) (float32, float32) {
	anchor, _ := ecs.Get[scene.Anchor](world, entity)
	return anchor.TopLeft(width, height, true)
}

func syncUIWidgets(world *ecs.World) {
	runtime := ecs.FrameValue[uiWidgetRuntimeStateTag, uiWidgetRuntimeStates](world)
	if runtime.values == nil {
		runtime.values = make(map[ecs.Entity]uiWidgetRuntimeState)
	} else {
		// Dynamic UI nodes can be removed between frames. Drop their cached
		// alignment state so the runtime map does not grow for the lifetime of a
		// world.
		for entity := range runtime.values {
			if !world.IsAlive(entity) || !ecs.Has[Widget](world, entity) {
				delete(runtime.values, entity)
			}
		}
	}
	viewport := mgl32.Vec2{}
	scaleFactor := float32(1)
	if value, ok := ecs.GetResource[scene.ViewportSize](world); ok {
		viewport = mgl32.Vec2{value.Width, value.Height}
		scaleFactor = value.ScaleFactor
	}
	for entity, access := range ecs.Query[Widget](world).All {
		widget := access.Ref()
		state := runtime.values[entity]
		if state.config != *widget {
			state = uiWidgetRuntimeState{config: *widget}
		}
		parent := widget.Target
		if parent == 0 {
			relation, exists := ecs.Get[scene.Parent](world, entity)
			if !exists {
				continue
			}
			parent = relation.Entity
		}
		parentWidth, parentHeight, valid := uiNodeSize(world, parent, false)
		if !valid {
			continue
		}
		if !widget.Enabled {
			continue
		}
		if widget.AlignMode == WidgetAlignOnce && state.aligned {
			continue
		}
		if widget.AlignMode == WidgetAlignOnWindowResize && state.aligned && state.lastParentWidth == parentWidth && state.lastParentHeight == parentHeight && state.lastViewportWidth == viewport.X() && state.lastViewportHeight == viewport.Y() {
			continue
		}
		left, right, top, bottom := widgetMargins(*widget, parentWidth, parentHeight, scaleFactor, viewport)
		originX, originY := uiNodeOrigin(world, parent, parentWidth, parentHeight)
		width, height, valid := uiNodeSize(world, entity, false)
		if !valid {
			continue
		}
		stretchX, stretchY := widget.AlignLeft && widget.AlignRight, widget.AlignTop && widget.AlignBottom
		if stretchX {
			width = max(parentWidth-left-right, float32(0))
		}
		if stretchY {
			height = max(parentHeight-top-bottom, float32(0))
		}
		if stretchX || stretchY {
			setUILayoutSize(world, entity, width, height)
		}
		leftPos := uiCurrentLeft(world, entity, width, height)
		topPos := uiCurrentTop(world, entity, width, height)
		if widget.AlignLeft {
			leftPos = originX + left
		} else if widget.AlignRight {
			leftPos = originX + parentWidth - right - width
		}
		if widget.AlignTop {
			topPos = originY + top
		} else if widget.AlignBottom {
			topPos = originY + parentHeight - bottom - height
		}
		setUILayoutPosition(world, entity, leftPos, topPos, width, height)
		state.lastParentWidth, state.lastParentHeight = parentWidth, parentHeight
		state.lastViewportWidth, state.lastViewportHeight, state.aligned = viewport.X(), viewport.Y(), true
		runtime.values[entity] = state
	}
}

func widgetMargins(widget Widget, width, height, scaleFactor float32, viewport mgl32.Vec2) (float32, float32, float32, float32) {
	left, _ := widget.Left.Resolve(scaleFactor, width, viewport)
	right, _ := widget.Right.Resolve(scaleFactor, width, viewport)
	top, _ := widget.Top.Resolve(scaleFactor, height, viewport)
	bottom, _ := widget.Bottom.Resolve(scaleFactor, height, viewport)
	return left, right, top, bottom
}

func setUILayoutSize(world *ecs.World, entity ecs.Entity, width, height float32) {
	if size := ecs.GetRef[scene.Size2D](world, entity); size != nil {
		if size.Width != width || size.Height != height {
			value := ecs.GetMut[scene.Size2D](world, entity)
			value.Width, value.Height = width, height
		}
	} else if sprite := ecs.GetRef[scene.Sprite2D](world, entity); sprite != nil {
		if sprite.Width != width || sprite.Height != height {
			value := ecs.GetMut[scene.Sprite2D](world, entity)
			value.Width, value.Height = width, height
		}
	} else if clip := ecs.GetRef[scene.ClipRect2D](world, entity); clip != nil {
		if clip.Width != width || clip.Height != height {
			value := ecs.GetMut[scene.ClipRect2D](world, entity)
			value.Width, value.Height = width, height
		}
	} else {
		return
	}
	if target := ecs.GetRef[scene.InputTarget2D](world, entity); target != nil {
		if target.Width != width || target.Height != height {
			value := ecs.GetMut[scene.InputTarget2D](world, entity)
			value.Width, value.Height = width, height
		}
	}
	if blocker := ecs.GetRef[scene.InputBlocker2D](world, entity); blocker != nil {
		if blocker.Width != width || blocker.Height != height {
			value := ecs.GetMut[scene.InputBlocker2D](world, entity)
			value.Width, value.Height = width, height
		}
	}
}
