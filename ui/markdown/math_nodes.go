package markdown

import (
	uinode "github.com/onix-engine/onix/scene/node"
)

func (b *markdownNodeBuilder) appendMarkdownMathNode(parent *uinode.Node, node mathNode) {
	switch node.kind {
	case mathText:
		b.markdownText(parent, node.value)
	case mathSequence:
		for _, child := range node.children {
			b.appendMarkdownMathNode(parent, child)
		}
	case mathFraction:
		b.markdownText(parent, node.left)
		class := "math-frac"
		if node.noBar {
			class += " math-frac-no-bar"
		}
		fraction := b.markdownElement(parent, "span", uinode.Attribute{Name: "class", Value: class})
		b.appendMathChild(b.markdownElement(fraction, "span", uinode.Attribute{Name: "class", Value: "math-num"}), node, 0)
		b.appendMathChild(b.markdownElement(fraction, "span", uinode.Attribute{Name: "class", Value: "math-den"}), node, 1)
		b.markdownText(parent, node.right)
	case mathRadical:
		radical := b.markdownElement(parent, "span", uinode.Attribute{Name: "class", Value: "math-radical"})
		if len(node.children) > 1 && node.children[1].kind != mathEmpty {
			b.appendMarkdownMathNode(b.markdownElement(radical, "span", uinode.Attribute{Name: "class", Value: "math-root-index"}), node.children[1])
		}
		b.markdownText(b.markdownElement(radical, "span", uinode.Attribute{Name: "class", Value: "math-root-symbol"}), "√")
		b.appendMathChild(b.markdownElement(b.markdownElement(radical, "span", uinode.Attribute{Name: "class", Value: "math-radicand"}), "span", uinode.Attribute{Name: "class", Value: "math-radicand-body"}), node, 0)
	case mathScripts:
		scripts := b.markdownElement(parent, "span", uinode.Attribute{Name: "class", Value: "math-scripts"})
		b.appendMathChild(b.markdownElement(scripts, "span", uinode.Attribute{Name: "class", Value: "math-base"}), node, 0)
		stack := b.markdownElement(scripts, "span", uinode.Attribute{Name: "class", Value: "math-script-stack"})
		if len(node.children) > 1 && node.children[1].kind != mathEmpty {
			b.appendMarkdownMathNode(b.markdownElement(stack, "sup"), node.children[1])
		}
		if len(node.children) > 2 && node.children[2].kind != mathEmpty {
			b.appendMarkdownMathNode(b.markdownElement(stack, "sub"), node.children[2])
		}
	case mathMatrix:
		left, right := mathEnvironmentDelimiters(node.name)
		matrix := b.markdownElement(parent, "span", uinode.Attribute{Name: "class", Value: "math-matrix"})
		if left != "" {
			b.markdownText(b.markdownElement(matrix, "span", uinode.Attribute{Name: "class", Value: "math-delimiter"}), left)
		}
		grid := b.markdownElement(matrix, "span", uinode.Attribute{Name: "class", Value: "math-grid"})
		for _, row := range node.rows {
			line := b.markdownElement(grid, "span", uinode.Attribute{Name: "class", Value: "math-row"})
			for column, cell := range row {
				b.appendMarkdownMathNode(b.markdownElement(line, "span", uinode.Attribute{Name: "class", Value: "math-cell math-align-" + mathCellAlignment(node, column)}), cell)
			}
		}
		if right != "" {
			b.markdownText(b.markdownElement(matrix, "span", uinode.Attribute{Name: "class", Value: "math-delimiter"}), right)
		}
	case mathStyle:
		b.appendMathChild(b.markdownElement(parent, "span", uinode.Attribute{Name: "class", Value: "math-style math-" + node.name}), node, 0)
	}
}

func (b *markdownNodeBuilder) appendMathChild(parent *uinode.Node, node mathNode, index int) {
	if index < len(node.children) {
		b.appendMarkdownMathNode(parent, node.children[index])
	}
}
