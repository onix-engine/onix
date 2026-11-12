package markdown

import (
	"github.com/onix-engine/onix/html/dom"
)

func appendMarkdownMathNode(parent *dom.Node, node mathNode) {
	switch node.kind {
	case mathText:
		markdownText(parent, node.value)
	case mathSequence:
		for _, child := range node.children {
			appendMarkdownMathNode(parent, child)
		}
	case mathFraction:
		markdownText(parent, node.left)
		class := "math-frac"
		if node.noBar {
			class += " math-frac-no-bar"
		}
		fraction := markdownElement(parent, "span", dom.Attribute{Name: "class", Value: class})
		appendMathChild(markdownElement(fraction, "span", dom.Attribute{Name: "class", Value: "math-num"}), node, 0)
		appendMathChild(markdownElement(fraction, "span", dom.Attribute{Name: "class", Value: "math-den"}), node, 1)
		markdownText(parent, node.right)
	case mathRadical:
		radical := markdownElement(parent, "span", dom.Attribute{Name: "class", Value: "math-radical"})
		if len(node.children) > 1 && node.children[1].kind != mathEmpty {
			appendMarkdownMathNode(markdownElement(radical, "span", dom.Attribute{Name: "class", Value: "math-root-index"}), node.children[1])
		}
		markdownText(markdownElement(radical, "span", dom.Attribute{Name: "class", Value: "math-root-symbol"}), "√")
		appendMathChild(markdownElement(markdownElement(radical, "span", dom.Attribute{Name: "class", Value: "math-radicand"}), "span", dom.Attribute{Name: "class", Value: "math-radicand-body"}), node, 0)
	case mathScripts:
		scripts := markdownElement(parent, "span", dom.Attribute{Name: "class", Value: "math-scripts"})
		appendMathChild(markdownElement(scripts, "span", dom.Attribute{Name: "class", Value: "math-base"}), node, 0)
		stack := markdownElement(scripts, "span", dom.Attribute{Name: "class", Value: "math-script-stack"})
		if len(node.children) > 1 && node.children[1].kind != mathEmpty {
			appendMarkdownMathNode(markdownElement(stack, "sup"), node.children[1])
		}
		if len(node.children) > 2 && node.children[2].kind != mathEmpty {
			appendMarkdownMathNode(markdownElement(stack, "sub"), node.children[2])
		}
	case mathMatrix:
		left, right := mathEnvironmentDelimiters(node.name)
		matrix := markdownElement(parent, "span", dom.Attribute{Name: "class", Value: "math-matrix"})
		if left != "" {
			markdownText(markdownElement(matrix, "span", dom.Attribute{Name: "class", Value: "math-delimiter"}), left)
		}
		grid := markdownElement(matrix, "span", dom.Attribute{Name: "class", Value: "math-grid"})
		for _, row := range node.rows {
			line := markdownElement(grid, "span", dom.Attribute{Name: "class", Value: "math-row"})
			for column, cell := range row {
				appendMarkdownMathNode(markdownElement(line, "span", dom.Attribute{Name: "class", Value: "math-cell math-align-" + mathCellAlignment(node, column)}), cell)
			}
		}
		if right != "" {
			markdownText(markdownElement(matrix, "span", dom.Attribute{Name: "class", Value: "math-delimiter"}), right)
		}
	case mathStyle:
		appendMathChild(markdownElement(parent, "span", dom.Attribute{Name: "class", Value: "math-style math-" + node.name}), node, 0)
	}
}

func appendMathChild(parent *dom.Node, node mathNode, index int) {
	if index < len(node.children) {
		appendMarkdownMathNode(parent, node.children[index])
	}
}
