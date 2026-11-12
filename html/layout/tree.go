package layout

import (
	"fmt"
	"slices"
	"strings"

	"github.com/onix-engine/onix/font"
	"github.com/onix-engine/onix/html/dom"
	"github.com/onix-engine/onix/html/style"
)

func Build(root *dom.Node, styles style.Map, face *font.Face, viewportWidth, viewportHeight float64) (*Box, error) {
	return BuildWithFonts(root, styles, face, nil, viewportWidth, viewportHeight)
}

// BuildWithFonts lays out a document using separate regular and bold faces.
// The bold face is selected for text whose computed font weight is 600 or
// heavier, matching the font used when its visual is spawned.
func BuildWithFonts(root *dom.Node, styles style.Map, face, boldFace *font.Face, viewportWidth, viewportHeight float64) (*Box, error) {
	if root == nil {
		return nil, fmt.Errorf("layout root is nil")
	}

	box := buildTree(root, styles)
	if box == nil {
		return nil, fmt.Errorf("no block layout tree could be built")
	}

	viewport := Rect{Width: viewportWidth, Height: viewportHeight}
	ctx := layoutContextPool.Get().(*layoutContext)
	defer releaseLayoutContext(ctx)
	ctx.boldFace = boldFace
	layoutBlock(box, viewport, viewport, face, styles, ctx)
	if ctx.err != nil {
		return nil, ctx.err
	}
	box.setViewport(viewport)
	return box, nil
}

func buildTree(node *dom.Node, styles style.Map) *Box {
	var nextID uint64
	return buildTreeWithIDs(node, styles, &nextID)
}

func buildTreeWithIDs(node *dom.Node, styles style.Map, nextID *uint64) *Box {
	if !nodeCreatesBox(node, styles) {
		return nil
	}
	computed := styles[node]

	*nextID += 1
	box := &Box{StableID: *nextID, Node: node, Style: computed}
	if len(node.Children) > 0 {
		box.Children = make([]*Box, 0, len(node.Children))
	}
	appendLayoutChildren(box, node, styles, nextID)
	if isFlexDisplay(computed.Display.Keyword) {
		slices.SortStableFunc(box.Children, compareFlexOrder)
	}

	return box
}

func appendLayoutChildren(parent *Box, node *dom.Node, styles style.Map, nextID *uint64) {
	for _, child := range node.Children {
		computed, ok := styles.Get(child)
		if !ok {
			continue
		}
		if computed.Display.Kind == style.ValueKeyword && computed.Display.Keyword == "contents" {
			appendLayoutChildren(parent, child, styles, nextID)
			continue
		}
		childBox := buildTreeWithIDs(child, styles, nextID)
		if childBox != nil {
			childBox.Parent = parent
			parent.Children = append(parent.Children, childBox)
			parent.hasFloats = parent.hasFloats || isFloated(childBox) || childBox.hasFloats && !establishesFloatContext(childBox)
		}
	}
}

func isFlexItem(node *dom.Node, styles style.Map) bool {
	if node == nil || node.Parent == nil || node.Type == dom.TextNode && strings.TrimSpace(node.Data) == "" {
		return false
	}
	parentStyle, ok := effectiveLayoutParentStyle(node.Parent, styles)
	return ok && isFlexDisplay(parentStyle.Display.Keyword)
}

func isGridItem(node *dom.Node, styles style.Map) bool {
	if node == nil || node.Parent == nil || node.Type == dom.TextNode && strings.TrimSpace(node.Data) == "" {
		return false
	}
	parentStyle, ok := effectiveLayoutParentStyle(node.Parent, styles)
	return ok && isGridDisplay(parentStyle.Display.Keyword)
}

func effectiveLayoutParentStyle(node *dom.Node, styles style.Map) (*style.ComputedStyle, bool) {
	for current := node; current != nil; current = current.Parent {
		computed, ok := styles.Get(current)
		if !ok {
			return nil, false
		}
		if computed.Display.Kind != style.ValueKeyword || computed.Display.Keyword != "contents" {
			return computed, true
		}
	}
	return nil, false
}

func isLayoutContainerDisplay(display string) bool {
	switch display {
	case "block", "flow-root", "flex", "inline-flex", "grid", "inline-grid", "inline-block", "list-item", "table", "table-caption", "table-row-group", "table-header-group", "table-footer-group", "table-row", "table-cell":
		return true
	default:
		return false
	}
}

// isBlockLevelDisplay marks the display values whose boxes stack vertically in
// normal flow; everything else participates in inline layout.
func isBlockLevelDisplay(display string) bool {
	return display == "block" || display == "flow-root" || display == "flex" || display == "inline-flex" || display == "grid" || display == "list-item" || display == "table"
}

func isFlexDisplay(display string) bool {
	return display == "flex" || display == "inline-flex"
}

func isGridDisplay(display string) bool {
	return display == "grid" || display == "inline-grid"
}

func flexOrder(box *Box) float64 {
	if box == nil || box.Style.Order.Kind != style.ValueNumber {
		return 0
	}
	return box.Style.Order.Number
}

func nodeCreatesBox(node *dom.Node, styles style.Map) bool {
	if node == nil {
		return false
	}

	computed, ok := styles.Get(node)
	if !ok {
		return false
	}
	if computed.Display.Kind == style.ValueKeyword && computed.Display.Keyword == "none" {
		return false
	}

	if node.Type == dom.TextNode && !isFlexItem(node, styles) && !isGridItem(node, styles) {
		return false
	}

	if node.Type == dom.ElementNode {
		// Flex and Grid items are blockified. Inline-block controls retain their
		// own boxes and participate in normal flow as atomic inline boxes.
		flexItem, gridItem := isFlexItem(node, styles), isGridItem(node, styles)
		if !isLayoutContainerDisplay(computed.Display.Keyword) && !flexItem && !gridItem {
			return false
		}
	}

	return true
}

func compareFlexOrder(left, right *Box) int {
	leftOrder, rightOrder := flexOrder(left), flexOrder(right)
	if leftOrder < rightOrder {
		return -1
	}
	if leftOrder > rightOrder {
		return 1
	}
	return 0
}
