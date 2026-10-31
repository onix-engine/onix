package markdown

import (
	"github.com/diiyw/onix/html/dom"
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
		fraction := markdownElement(parent, "span", map[string]string{"class": class})
		appendMathChild(markdownElement(fraction, "span", map[string]string{"class": "math-num"}), node, 0)
		appendMathChild(markdownElement(fraction, "span", map[string]string{"class": "math-den"}), node, 1)
		markdownText(parent, node.right)
	case mathRadical:
		radical := markdownElement(parent, "span", map[string]string{"class": "math-radical"})
		if len(node.children) > 1 && node.children[1].kind != mathEmpty {
			appendMarkdownMathNode(markdownElement(radical, "span", map[string]string{"class": "math-root-index"}), node.children[1])
		}
		markdownText(markdownElement(radical, "span", map[string]string{"class": "math-root-symbol"}), "√")
		appendMathChild(markdownElement(markdownElement(radical, "span", map[string]string{"class": "math-radicand"}), "span", map[string]string{"class": "math-radicand-body"}), node, 0)
	case mathScripts:
		scripts := markdownElement(parent, "span", map[string]string{"class": "math-scripts"})
		appendMathChild(markdownElement(scripts, "span", map[string]string{"class": "math-base"}), node, 0)
		stack := markdownElement(scripts, "span", map[string]string{"class": "math-script-stack"})
		if len(node.children) > 1 && node.children[1].kind != mathEmpty {
			appendMarkdownMathNode(markdownElement(stack, "sup", nil), node.children[1])
		}
		if len(node.children) > 2 && node.children[2].kind != mathEmpty {
			appendMarkdownMathNode(markdownElement(stack, "sub", nil), node.children[2])
		}
	case mathMatrix:
		left, right := mathEnvironmentDelimiters(node.name)
		matrix := markdownElement(parent, "span", map[string]string{"class": "math-matrix"})
		if left != "" {
			markdownText(markdownElement(matrix, "span", map[string]string{"class": "math-delimiter"}), left)
		}
		grid := markdownElement(matrix, "span", map[string]string{"class": "math-grid"})
		for _, row := range node.rows {
			line := markdownElement(grid, "span", map[string]string{"class": "math-row"})
			for column, cell := range row {
				appendMarkdownMathNode(markdownElement(line, "span", map[string]string{"class": "math-cell math-align-" + mathCellAlignment(node, column)}), cell)
			}
		}
		if right != "" {
			markdownText(markdownElement(matrix, "span", map[string]string{"class": "math-delimiter"}), right)
		}
	case mathStyle:
		appendMathChild(markdownElement(parent, "span", map[string]string{"class": "math-style math-" + node.name}), node, 0)
	}
}

func appendMathChild(parent *dom.Node, node mathNode, index int) {
	if index < len(node.children) {
		appendMarkdownMathNode(parent, node.children[index])
	}
}
