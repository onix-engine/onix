package ui

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene"
	onixtext "github.com/diiyw/onix/text"
)

type Tree[T comparable] struct {
	Selected     T
	HasSelection bool
	Width        float32
	RowHeight    float32
}

type treeIconWidthCache struct {
	font                          *onixtext.Font
	fontSize, dpi                 float64
	collapsed, expanded, leaf     IconStyle
	collapsedWidth, expandedWidth float32
	leafWidth                     float32
	valid                         bool
}

func (c treeIconWidthCache) matches(theme Theme, style TreeStyle) bool {
	return c.valid && c.font == theme.Font && c.fontSize == theme.FontSize && c.dpi == theme.DPI &&
		c.collapsed == style.Collapsed && c.expanded == style.Expanded && c.leaf == style.Leaf
}

func (c *treeIconWidthCache) update(theme Theme, style TreeStyle) {
	c.font, c.fontSize, c.dpi = theme.Font, theme.FontSize, theme.DPI
	c.collapsed, c.expanded, c.leaf = style.Collapsed, style.Expanded, style.Leaf
	c.collapsedWidth = measureIconWidth(theme, style.Collapsed)
	c.expandedWidth = measureIconWidth(theme, style.Expanded)
	c.leafWidth = measureIconWidth(theme, style.Leaf)
	c.valid = true
}

func (c treeIconWidthCache) width(leaf, expanded bool) float32 {
	if leaf {
		return c.leafWidth
	}
	if expanded {
		return c.expandedWidth
	}
	return c.collapsedWidth
}

func cachedTreeIconWidth(world *ecs.World, tree ecs.Entity, theme Theme, style TreeStyle, leaf, expanded bool) float32 {
	cache, ok := ecs.Get[treeIconWidthCache](world, tree)
	if !ok || !cache.matches(theme, style) {
		cache.update(theme, style)
		_ = ecs.Insert(world, tree, cache)
	}
	return cache.width(leaf, expanded)
}

type TreeNode[T comparable] struct {
	Tree      ecs.Entity
	Order     int
	ID        T
	Parent    T
	HasParent bool
	Label     string
	Expanded  bool
	Leaf      bool
	Depth     int
	Disabled  bool
	Selected  bool
}

type TreeNodeSpec[T comparable] struct {
	ID        T
	Parent    T
	HasParent bool
	Label     string
	Expanded  bool
	Leaf      bool
}

type TreeOptions[T comparable] struct {
	X, Y        float32
	Width       float32
	RowHeight   float32
	Nodes       []TreeNodeSpec[T]
	Radius      CornerRadius
	PassThrough bool
	Theme       Theme
	Style       *TreeStyle
}

type TreeEvent[T comparable] struct {
	Tree  ecs.Entity
	Node  ecs.Entity
	ID    T
	Label string
}

type treeNodeKey[T comparable] struct {
	tree ecs.Entity
	id   T
}

type treeNodeState[T comparable] struct {
	entity ecs.Entity
	key    treeNodeKey[T]
	node   TreeNode[T]
}

type treeNodesScratch[T comparable] struct{}
type treeVisibilityScratch[T comparable] struct{}
type treeVisitingScratch[T comparable] struct{}
type treeStatesScratch[T comparable] struct{}
type treeRowOffsetsScratch[T comparable] struct{}
type treeVisibleCountsScratch[T comparable] struct{}

// treeActivation routes a click directly to the node's concrete generic type.
// The function has no captured state; all node data remains in TreeNode[T].
type treeActivation struct {
	activate func(*ecs.World, ecs.Entity)
}

// SpawnTree creates typed nodes whose clicks are dispatched by the Widgets
// UpdateSystem. SyncTreeSystem[T] handles their layout and visual state.
func SpawnTree[T comparable](world *ecs.World, opts TreeOptions[T]) ecs.Entity {
	theme := themeForWorld(world, opts.Theme)
	style := skinFor(world, theme).Tree
	if opts.Style != nil {
		style = *opts.Style
	}
	rowHeight := opts.RowHeight
	if rowHeight == 0 {
		rowHeight = 34
	}
	height := rowHeight * float32(len(opts.Nodes))
	tree := spawnBox(world, opts.X, opts.Y, opts.Width, height, theme.StackIndex, theme.Background)
	setCornerRadius(world, tree, opts.Radius)
	_ = ecs.Insert(world, tree, scene.InputBlocker2D{Width: opts.Width, Height: height, PassThrough: opts.PassThrough})
	_ = ecs.Insert(world, tree, Tree[T]{Width: opts.Width, RowHeight: rowHeight})
	iconWidths := treeIconWidthCache{}
	iconWidths.update(theme, style)
	_ = ecs.Insert(world, tree, iconWidths)
	putStyleOverride(world, tree, opts.Style)
	depthByID := map[T]int{}
	rowVisualHeight := treeRowVisualHeight(rowHeight)
	rowInset := treeRowInset(rowHeight)
	for i, spec := range opts.Nodes {
		depth := 0
		if spec.HasParent {
			depth = depthByID[spec.Parent] + 1
		}
		depthByID[spec.ID] = depth
		node := spawnInteractiveBox(world, 0, float32(i)*rowHeight+rowInset, opts.Width, rowVisualHeight, theme.StackIndex+1, graphics.ColorRGBA{}, false, opts.PassThrough)
		setCornerRadius(world, node, opts.Radius)
		_ = ecs.Insert(world, node, scene.Parent{Entity: tree})
		_ = ecs.Insert(world, node, TreeNode[T]{
			Tree:      tree,
			Order:     i,
			ID:        spec.ID,
			Parent:    spec.Parent,
			HasParent: spec.HasParent,
			Label:     spec.Label,
			Expanded:  spec.Expanded,
			Leaf:      spec.Leaf,
			Depth:     depth,
		})
		_ = ecs.Insert(world, node, treeActivation{activate: activateTreeNode[T]})
		icon := treeNodeIcon(style, spec.Leaf, spec.Expanded)
		indicator := spawnText[LabelPart](world, node, treeIndicatorX(depth), textBaseline(rowVisualHeight), iconFallback(icon), theme)
		_ = ecs.Insert(world, indicator, Relation[TreeNode[T]]{Owner: node})
		_ = ecs.Insert(world, indicator, Part[TreeIndicatorTextPart]{})
		indicatorImage := spawnBox(world, treeIndicatorX(depth), 0, 0, 0, theme.StackIndex+2, graphics.ColorRGBA{})
		_ = ecs.Insert(world, indicatorImage, scene.Parent{Entity: node})
		_ = ecs.Insert(world, indicatorImage, Relation[TreeNode[T]]{Owner: node})
		_ = ecs.Insert(world, indicatorImage, Part[TreeIndicatorImagePart]{})
		iconWidth := iconWidths.width(spec.Leaf, spec.Expanded)
		text := spawnText[LabelPart](world, node, treeLabelX(depth, iconWidth), textBaseline(rowVisualHeight), spec.Label, theme)
		_ = ecs.Insert(world, text, Relation[TreeNode[T]]{Owner: node})
		_ = ecs.Insert(world, text, Part[TreeTextPart]{})
	}
	return tree
}

func SyncTreeSystem[T comparable](name string) ecs.SystemConfig {
	if name == "" {
		name = "ui_tree_sync"
	}
	return ecs.SystemConfig{Name: name, System: syncTree[T]}
}

func activateTreeNode[T comparable](world *ecs.World, entity ecs.Entity) {
	node := ecs.GetMut[TreeNode[T]](world, entity)
	if node == nil || node.Disabled {
		return
	}
	if !node.Leaf {
		node.Expanded = !node.Expanded
	}
	if tree := ecs.GetMut[Tree[T]](world, node.Tree); tree != nil {
		tree.Selected = node.ID
		tree.HasSelection = true
	}
	ecs.WriteMessage(world, TreeEvent[T]{Tree: node.Tree, Node: entity, ID: node.ID, Label: node.Label})
}

func syncTree[T comparable](context *ecs.SystemContext) error {
	world := context.World
	if world == nil {
		return nil
	}
	theme, ok := ecs.GetResource[Theme](world)
	if ok {
		theme = themeWithDefaults(theme)
	} else {
		theme = DefaultTheme()
	}
	skin := skinFor(world, theme)
	nodes := ecs.FrameScratchMap[treeNodesScratch[T], treeNodeKey[T], TreeNode[T]](world)
	visibility := ecs.FrameScratchMap[treeVisibilityScratch[T], treeNodeKey[T], bool](world)
	visiting := ecs.FrameScratchMap[treeVisitingScratch[T], treeNodeKey[T], bool](world)
	states := ecs.FrameScratch[treeStatesScratch[T], treeNodeState[T]](world)
	defer states.ReleaseUnused()
	for entity, access := range ecs.Query[TreeNode[T]](world).All {
		node := access.Get()

		key := treeNodeKey[T]{tree: node.Tree, id: node.ID}
		nodes[key] = node
		states.Values = append(states.Values, treeNodeState[T]{entity: entity, key: key, node: node})
	}
	slices.SortFunc(states.Values, func(a, b treeNodeState[T]) int {
		if a.node.Tree == b.node.Tree {
			return cmp.Compare(a.node.Order, b.node.Order)
		}
		return cmp.Compare(a.node.Tree, b.node.Tree)
	})
	for key := range nodes {
		if _, err := treeNodeVisible(nodes, visibility, visiting, key); err != nil {
			return err
		}
	}
	rowOffsets := ecs.FrameScratchMap[treeRowOffsetsScratch[T], treeNodeKey[T], float32](world)
	visibleCountByTree := ecs.FrameScratchMap[treeVisibleCountsScratch[T], ecs.Entity, int](world)
	for _, state := range states.Values {
		if !visibility[state.key] {
			continue
		}
		tree, ok := ecs.Get[Tree[T]](world, state.node.Tree)
		if !ok {
			continue
		}
		rowOffsets[state.key] = float32(visibleCountByTree[state.node.Tree])*tree.RowHeight + treeRowInset(tree.RowHeight)
		visibleCountByTree[state.node.Tree]++
	}
	for entity, access := range ecs.Query3[TreeNode[T], scene.Visibility, scene.Size2D](world).All {
		node := access.A.Mut()
		visible := access.B.Mut()
		size := access.C.Mut()

		key := treeNodeKey[T]{tree: node.Tree, id: node.ID}
		visible.Visible = visibility[key]
		node.Selected = treeNodeSelected(world, node)
		tree, ok := ecs.Get[Tree[T]](world, node.Tree)
		if !ok {
			continue
		}
		rowHeight := treeRowVisualHeight(tree.RowHeight)
		if transform := ecs.GetMut[scene.UITransform](world, entity); transform != nil {
			transform.Translation.X.Value = 0
			transform.Translation.Y.Value = rowOffsets[key]
		}
		SetControlBounds(world, entity, tree.Width, rowHeight)
		SetControlDisabled(world, entity, node.Disabled)
		size.Width = tree.Width
		size.Height = rowHeight
		control, _ := controlViewFor(world, entity)
		state := controlState(&control, false)
		state.Disabled = state.Disabled || node.Disabled
		style := styleFor(world, node.Tree, skin.Tree)
		applySurface(world, entity, size.Width, size.Height, style.Row.Resolve(node.Selected, state))
	}
	for entity, access := range ecs.Query[Relation[TreeNode[T]]](world).All {
		element := access.Get()

		node, ok := ecs.Get[TreeNode[T]](world, element.Owner)
		if !ok {
			continue
		}
		tree, ok := ecs.Get[Tree[T]](world, node.Tree)
		if !ok {
			continue
		}
		style := styleFor(world, node.Tree, skin.Tree)
		icon := treeNodeIcon(style, node.Leaf, node.Expanded)
		iconWidth := cachedTreeIconWidth(world, node.Tree, theme, style, node.Leaf, node.Expanded)
		text := ecs.GetMut[scene.Text2D](world, entity)
		transform := ecs.GetMut[scene.UITransform](world, entity)
		if (text != nil) && (transform != nil) {
			transform.Translation.Y.Value = textBaseline(treeRowVisualHeight(tree.RowHeight))
			switch {
			case ecs.Has[Part[TreeTextPart]](world, entity):
				text.Value = node.Label
				text.Color = theme.Text
				transform.Translation.X.Value = treeLabelX(node.Depth, iconWidth)
			case ecs.Has[Part[TreeIndicatorTextPart]](world, entity):
				text.Value = iconFallback(icon)
				text.Color = treeIndicatorColor(&node, theme)
				transform.Translation.X.Value = treeIndicatorX(node.Depth)
			}
		}
		if (ecs.Has[Part[TreeIndicatorImagePart]](world, entity)) && (transform != nil) {
			rowHeight := treeRowVisualHeight(tree.RowHeight)
			width, height := fitIconSize(icon, max(icon.Width, 18), max(rowHeight-4, 0))
			if icon.Texture == nil {
				width, height = 0, 0
			}
			transform.Translation.X.Value = treeIndicatorX(node.Depth)
			transform.Translation.Y.Value = (rowHeight - height) * 0.5
			image := scene.NewUIImage2D(icon.Texture)
			applySurface(world, entity, width, height, SurfaceStyle{Image: image})
		}
		if current := ecs.GetMut[scene.Visibility](world, entity); current != nil {
			ownerVisibility, exists := ecs.Get[scene.Visibility](world, element.Owner)
			current.Visible = !exists || ownerVisibility.Visible
		}
	}
	for entity, access := range ecs.Query[Tree[T]](world).All {
		tree := access.Get()

		height := tree.RowHeight * float32(visibleCountByTree[entity])
		style := styleFor(world, entity, skin.Tree)
		applySurface(world, entity, tree.Width, height, style.Background)
	}
	return nil
}

func treeNodeVisible[T comparable](nodes map[treeNodeKey[T]]TreeNode[T], visibility, visiting map[treeNodeKey[T]]bool, key treeNodeKey[T]) (bool, error) {
	if visible, ok := visibility[key]; ok {
		return visible, nil
	}
	if visiting[key] {
		return false, fmt.Errorf("ui: cycle detected in tree %d at node %v", key.tree, key.id)
	}
	node, ok := nodes[key]
	if !ok {
		return false, nil
	}
	if !node.HasParent {
		visibility[key] = true
		return true, nil
	}
	visiting[key] = true
	defer delete(visiting, key)
	parentKey := treeNodeKey[T]{tree: node.Tree, id: node.Parent}
	parent, exists := nodes[parentKey]
	if !exists {
		visibility[key] = false
		return false, nil
	}
	parentVisible, err := treeNodeVisible(nodes, visibility, visiting, parentKey)
	if err != nil {
		return false, err
	}
	visible := parentVisible && parent.Expanded
	visibility[key] = visible
	return visible, nil
}

func treeNodeSelected[T comparable](world *ecs.World, node *TreeNode[T]) bool {
	tree, ok := ecs.Get[Tree[T]](world, node.Tree)
	return ok && tree.HasSelection && tree.Selected == node.ID
}

func treeNodeIcon(style TreeStyle, leaf, expanded bool) IconStyle {
	if leaf {
		return style.Leaf
	}
	if expanded {
		return style.Expanded
	}
	return style.Collapsed
}

func treeIndicatorX(depth int) float32 {
	return 12 + float32(depth)*18
}

func treeLabelX(depth int, iconWidth float32) float32 {
	return treeIndicatorX(depth) + max(iconWidth, 10) + 8
}

func treeRowInset(rowHeight float32) float32 {
	if rowHeight < 24 {
		return 1
	}
	return 2
}

func treeRowVisualHeight(rowHeight float32) float32 {
	height := rowHeight - treeRowInset(rowHeight)*2
	if height < 18 {
		return rowHeight
	}
	return height
}

func treeIndicatorColor[T comparable](node *TreeNode[T], theme Theme) graphics.ColorRGBA {
	if node.Disabled {
		return alphaColor(theme.Muted, 0.65)
	}
	if node.Leaf {
		return alphaColor(theme.Muted, 0.9)
	}
	return theme.Muted
}
