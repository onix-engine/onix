package markdown

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/onix-engine/onix/html/dom"
	"github.com/onix-engine/onix/syntax"
	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	goldmarktext "github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// markdownToDOM builds ordinary Markdown nodes directly. Raw HTML and unknown
// Goldmark extensions use the HTML tree builder so malformed markup receives
// the same insertion-mode repairs as authored HTML.
func markdownToDOM(value string) *dom.Node {
	source := []byte(value)
	parsed := markdownRuntimeFor().document.Parser().Parse(goldmarktext.NewReader(source))
	root := dom.NewDocument()
	if !appendMarkdownNode(root, parsed, source) {
		fallback, err := dom.Parse(markdownToHTML(value))
		if err == nil {
			return fallback
		}
	}
	return root
}

func markdownElement(parent *dom.Node, tag string, attrs ...dom.Attribute) *dom.Node {
	node := dom.NewElement(tag, attrs...)
	parent.AppendChild(node)
	return node
}

func markdownText(parent *dom.Node, value string) {
	if value == "" {
		return
	}
	if count := len(parent.Children); count != 0 && parent.Children[count-1].Type == dom.TextNode {
		parent.Children[count-1].Data += value
		return
	}
	parent.AppendChild(dom.NewText(value))
}

func markdownAttributes(node ast.Node) []dom.Attribute {
	if len(node.Attributes()) == 0 {
		return nil
	}
	attrs := make([]dom.Attribute, 0, len(node.Attributes()))
	for _, attribute := range node.Attributes() {
		name := string(attribute.Name)
		switch value := attribute.Value.(type) {
		case []byte:
			attrs = append(attrs, dom.Attribute{Name: name, Value: string(value)})
		case string:
			attrs = append(attrs, dom.Attribute{Name: name, Value: value})
		}
	}
	return attrs
}

func appendMarkdownChildren(parent *dom.Node, node ast.Node, source []byte) bool {
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if !appendMarkdownNode(parent, child, source) {
			return false
		}
	}
	return true
}

func appendMarkdownNode(parent *dom.Node, node ast.Node, source []byte) bool {
	switch value := node.(type) {
	case *ast.Document, *ast.TextBlock:
		return appendMarkdownChildren(parent, node, source)
	case *ast.Paragraph:
		return appendMarkdownChildren(markdownElement(parent, "p", markdownAttributes(node)...), node, source)
	case *ast.Heading:
		return appendMarkdownChildren(markdownElement(parent, "h"+strconv.Itoa(value.Level), markdownAttributes(node)...), node, source)
	case *ast.Blockquote:
		return appendMarkdownChildren(markdownElement(parent, "blockquote", markdownAttributes(node)...), node, source)
	case *ast.List:
		tag := "ul"
		if value.IsOrdered() {
			tag = "ol"
		}
		list := markdownElement(parent, tag, markdownAttributes(node)...)
		if value.IsOrdered() && value.Start != 1 {
			list.SetAttr("start", strconv.Itoa(value.Start))
		}
		return appendMarkdownChildren(list, node, source)
	case *ast.ListItem:
		item := markdownElement(parent, "li", markdownAttributes(node)...)
		marker := markdownElement(item, "span", dom.Attribute{Name: "class", Value: "markdown-list-marker"})
		if list, ok := node.Parent().(*ast.List); ok && list.IsOrdered() {
			markdownText(marker, strconv.Itoa(markdownListItemNumber(value))+". ")
		} else {
			markdownText(marker, "• ")
		}
		return appendMarkdownChildren(item, node, source)
	case *ast.ThematicBreak:
		markdownElement(parent, "hr", markdownAttributes(node)...)
		return true
	case *ast.CodeBlock:
		code := markdownElement(markdownElement(parent, "pre"), "code")
		for index := range value.Lines().Len() {
			line := value.Lines().At(index)
			markdownText(code, string(line.Value(source)))
		}
		return true
	case *ast.FencedCodeBlock:
		appendMarkdownCodeBlock(parent, value, source)
		return true
	case *ast.Text:
		markdownText(parent, string(value.Segment.Value(source)))
		if value.HardLineBreak() {
			markdownElement(parent, "br")
			markdownText(parent, "\n")
		} else if value.SoftLineBreak() {
			markdownText(parent, "\n")
		}
		return true
	case *ast.String:
		markdownText(parent, string(value.Value))
		return true
	case *ast.Emphasis:
		tag := "em"
		if value.Level == 2 {
			tag = "strong"
		}
		return appendMarkdownChildren(markdownElement(parent, tag, markdownAttributes(node)...), node, source)
	case *ast.CodeSpan:
		code := markdownElement(parent, "code", markdownAttributes(node)...)
		for child := value.FirstChild(); child != nil; child = child.NextSibling() {
			text, ok := child.(*ast.Text)
			if !ok {
				return false
			}
			line := text.Segment.Value(source)
			if bytes.HasSuffix(line, []byte("\n")) {
				markdownText(code, string(line[:len(line)-1])+" ")
			} else {
				markdownText(code, string(line))
			}
		}
		return true
	case *ast.Link:
		link := markdownElement(parent, "a", markdownAttributes(node)...)
		link.SetAttr("href", string(util.URLEscape(value.Destination, true)))
		if value.Title != nil {
			link.SetAttr("title", string(value.Title))
		}
		return appendMarkdownChildren(link, node, source)
	case *ast.AutoLink:
		url := util.URLEscape(value.URL(source), false)
		if value.AutoLinkType == ast.AutoLinkEmail && !bytes.HasPrefix(bytes.ToLower(url), []byte("mailto:")) {
			url = append([]byte("mailto:"), url...)
		}
		link := markdownElement(parent, "a", dom.Attribute{Name: "href", Value: string(url)})
		markdownText(link, string(value.Label(source)))
		return true
	case *ast.Image:
		image := markdownElement(parent, "img", markdownAttributes(node)...)
		image.SetAttr("src", string(util.URLEscape(value.Destination, true)))
		image.SetAttr("alt", markdownPlainText(value, source))
		if value.Title != nil {
			image.SetAttr("title", string(value.Title))
		}
		return true
	case *extast.Strikethrough:
		return appendMarkdownChildren(markdownElement(parent, "del", markdownAttributes(node)...), node, source)
	case *extast.TaskCheckBox:
		checkbox := markdownElement(parent, "input", dom.Attribute{Name: "type", Value: "checkbox"}, dom.Attribute{Name: "disabled", Value: ""})
		if value.IsChecked {
			checkbox.SetAttr("checked", "")
		}
		markdownText(parent, " ")
		return true
	case *extast.Table:
		return appendMarkdownChildren(markdownElement(parent, "table", markdownAttributes(node)...), node, source)
	case *extast.TableHeader:
		return appendMarkdownChildren(markdownElement(markdownElement(parent, "thead"), "tr"), node, source)
	case *extast.TableRow:
		body := parent
		if parent.Data == "table" {
			if count := len(parent.Children); count != 0 && parent.Children[count-1].Data == "tbody" {
				body = parent.Children[count-1]
			} else {
				body = markdownElement(parent, "tbody")
			}
		}
		return appendMarkdownChildren(markdownElement(body, "tr", markdownAttributes(node)...), node, source)
	case *extast.TableCell:
		tag := "td"
		if node.Parent().Kind() == extast.KindTableHeader {
			tag = "th"
		}
		cell := markdownElement(parent, tag, markdownAttributes(node)...)
		if value.Alignment != extast.AlignNone {
			cell.SetAttr("style", strings.TrimSuffix(cell.AttrValue("style"), ";")+"text-align:"+value.Alignment.String())
		}
		return appendMarkdownChildren(cell, node, source)
	case *markdownInlineMath:
		appendMarkdownMathNode(markdownElement(parent, "span", dom.Attribute{Name: "class", Value: "math inline"}), parseMarkdownMath(value.Content))
		return true
	case *markdownDisplayMath:
		appendMarkdownMathNode(markdownElement(parent, "div", dom.Attribute{Name: "class", Value: "math display"}), parseMarkdownMath(value.Lines().Value(source)))
		return true
	default:
		return false
	}
}

func markdownPlainText(node ast.Node, source []byte) string {
	var builder strings.Builder
	var visit func(ast.Node)
	visit = func(current ast.Node) {
		switch value := current.(type) {
		case *ast.Text:
			builder.Write(value.Segment.Value(source))
		case *ast.String:
			builder.Write(value.Value)
		}
		for child := current.FirstChild(); child != nil; child = child.NextSibling() {
			visit(child)
		}
	}
	visit(node)
	return builder.String()
}

func appendMarkdownCodeBlock(parent *dom.Node, block *ast.FencedCodeBlock, source []byte) {
	language := block.Language(source)
	code := markdownCodeBlockSource(block, source)
	codeNode := markdownElement(markdownElement(parent, "pre"), "code")
	if len(language) != 0 {
		codeNode.SetAttr("class", "language-"+string(language))
	}
	var document syntax.Document
	defer document.Close()
	if len(language) != 0 && document.Update(string(language), code) == nil && document.Language() != "" {
		codeNode.SetAttr("class", "syntax language-"+string(language))
		offset := 0
		for _, token := range document.Tokens(markdownRuntimeFor().style, nil) {
			markdownText(codeNode, code[offset:token.Start])
			markdownText(markdownElement(codeNode, "span", dom.Attribute{Name: "class", Value: "syntax-token"}, dom.Attribute{Name: "style", Value: token.CSS}), code[token.Start:token.End])
			offset = token.End
		}
		markdownText(codeNode, code[offset:])
		return
	}
	markdownText(codeNode, code)
}
