package markdown

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/onix-engine/onix/html/parse"
	uinode "github.com/onix-engine/onix/scene/node"
	"github.com/onix-engine/onix/syntax"
	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	goldmarktext "github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// markdownToDocument builds ordinary Markdown nodes directly. Raw HTML and unknown
// Goldmark extensions use the HTML tree builder so malformed markup receives
// the same insertion-mode repairs as authored HTML.
func markdownToDocument(value string) *uinode.Node {
	source := []byte(value)
	document := markdownRuntimeFor().document
	parsed := document.Parser().Parse(goldmarktext.NewReader(source))
	var builder markdownNodeBuilder
	root := builder.nodes.New(uinode.DocumentNode, "#document")
	if !builder.appendMarkdownNode(root, parsed, source) {
		var output strings.Builder
		if err := document.Renderer().Render(&output, source, parsed); err == nil {
			if fallback, err := parse.ParseTree(output.String()); err == nil {
				return fallback
			}
		}
	}
	return root
}

type markdownNodeBuilder struct {
	nodes uinode.Arena
}

func (b *markdownNodeBuilder) markdownElement(parent *uinode.Node, tag string, attrs ...uinode.Attribute) *uinode.Node {
	node := b.nodes.New(uinode.ElementNode, tag)
	node.SetAttributes(attrs...)
	parent.AppendChild(node)
	return node
}

func (b *markdownNodeBuilder) markdownText(parent *uinode.Node, value string) {
	if value == "" {
		return
	}
	if count := len(parent.Children); count != 0 && parent.Children[count-1].Type == uinode.TextNode {
		parent.Children[count-1].Data += value
		return
	}
	parent.AppendChild(b.nodes.New(uinode.TextNode, value))
}

func markdownAttributes(node ast.Node) []uinode.Attribute {
	if len(node.Attributes()) == 0 {
		return nil
	}
	attrs := make([]uinode.Attribute, 0, len(node.Attributes()))
	for _, attribute := range node.Attributes() {
		name := string(attribute.Name)
		switch value := attribute.Value.(type) {
		case []byte:
			attrs = append(attrs, uinode.Attribute{Name: name, Value: string(value)})
		case string:
			attrs = append(attrs, uinode.Attribute{Name: name, Value: value})
		}
	}
	return attrs
}

func (b *markdownNodeBuilder) appendMarkdownChildren(parent *uinode.Node, node ast.Node, source []byte) bool {
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if !b.appendMarkdownNode(parent, child, source) {
			return false
		}
	}
	return true
}

func (b *markdownNodeBuilder) appendMarkdownNode(parent *uinode.Node, node ast.Node, source []byte) bool {
	switch value := node.(type) {
	case *ast.Document, *ast.TextBlock:
		return b.appendMarkdownChildren(parent, node, source)
	case *ast.Paragraph:
		return b.appendMarkdownChildren(b.markdownElement(parent, "p", markdownAttributes(node)...), node, source)
	case *ast.Heading:
		return b.appendMarkdownChildren(b.markdownElement(parent, "h"+strconv.Itoa(value.Level), markdownAttributes(node)...), node, source)
	case *ast.Blockquote:
		return b.appendMarkdownChildren(b.markdownElement(parent, "blockquote", markdownAttributes(node)...), node, source)
	case *ast.List:
		tag := "ul"
		if value.IsOrdered() {
			tag = "ol"
		}
		list := b.markdownElement(parent, tag, markdownAttributes(node)...)
		if value.IsOrdered() && value.Start != 1 {
			list.SetAttr("start", strconv.Itoa(value.Start))
		}
		return b.appendMarkdownChildren(list, node, source)
	case *ast.ListItem:
		item := b.markdownElement(parent, "li", markdownAttributes(node)...)
		marker := b.markdownElement(item, "span", uinode.Attribute{Name: "class", Value: "markdown-list-marker"})
		if list, ok := node.Parent().(*ast.List); ok && list.IsOrdered() {
			b.markdownText(marker, strconv.Itoa(markdownListItemNumber(value))+". ")
		} else {
			b.markdownText(marker, "• ")
		}
		return b.appendMarkdownChildren(item, node, source)
	case *ast.ThematicBreak:
		b.markdownElement(parent, "hr", markdownAttributes(node)...)
		return true
	case *ast.CodeBlock:
		code := b.markdownElement(b.markdownElement(parent, "pre"), "code")
		for index := range value.Lines().Len() {
			line := value.Lines().At(index)
			b.markdownText(code, string(line.Value(source)))
		}
		return true
	case *ast.FencedCodeBlock:
		b.appendMarkdownCodeBlock(parent, value, source)
		return true
	case *ast.Text:
		b.markdownText(parent, string(value.Segment.Value(source)))
		if value.HardLineBreak() {
			b.markdownElement(parent, "br")
			b.markdownText(parent, "\n")
		} else if value.SoftLineBreak() {
			b.markdownText(parent, "\n")
		}
		return true
	case *ast.String:
		b.markdownText(parent, string(value.Value))
		return true
	case *ast.Emphasis:
		tag := "em"
		if value.Level == 2 {
			tag = "strong"
		}
		return b.appendMarkdownChildren(b.markdownElement(parent, tag, markdownAttributes(node)...), node, source)
	case *ast.CodeSpan:
		code := b.markdownElement(parent, "code", markdownAttributes(node)...)
		for child := value.FirstChild(); child != nil; child = child.NextSibling() {
			text, ok := child.(*ast.Text)
			if !ok {
				return false
			}
			line := text.Segment.Value(source)
			if bytes.HasSuffix(line, []byte("\n")) {
				b.markdownText(code, string(line[:len(line)-1])+" ")
			} else {
				b.markdownText(code, string(line))
			}
		}
		return true
	case *ast.Link:
		link := b.markdownElement(parent, "a", markdownAttributes(node)...)
		link.SetAttr("href", string(util.URLEscape(value.Destination, true)))
		if value.Title != nil {
			link.SetAttr("title", string(value.Title))
		}
		return b.appendMarkdownChildren(link, node, source)
	case *ast.AutoLink:
		url := util.URLEscape(value.URL(source), false)
		if value.AutoLinkType == ast.AutoLinkEmail && !bytes.HasPrefix(bytes.ToLower(url), []byte("mailto:")) {
			url = append([]byte("mailto:"), url...)
		}
		link := b.markdownElement(parent, "a", uinode.Attribute{Name: "href", Value: string(url)})
		b.markdownText(link, string(value.Label(source)))
		return true
	case *ast.Image:
		image := b.markdownElement(parent, "img", markdownAttributes(node)...)
		image.SetAttr("src", string(util.URLEscape(value.Destination, true)))
		image.SetAttr("alt", markdownPlainText(value, source))
		if value.Title != nil {
			image.SetAttr("title", string(value.Title))
		}
		return true
	case *extast.Strikethrough:
		return b.appendMarkdownChildren(b.markdownElement(parent, "del", markdownAttributes(node)...), node, source)
	case *extast.TaskCheckBox:
		checkbox := b.markdownElement(parent, "input", uinode.Attribute{Name: "type", Value: "checkbox"}, uinode.Attribute{Name: "disabled", Value: ""})
		if value.IsChecked {
			checkbox.SetAttr("checked", "")
		}
		b.markdownText(parent, " ")
		return true
	case *extast.Table:
		return b.appendMarkdownChildren(b.markdownElement(parent, "table", markdownAttributes(node)...), node, source)
	case *extast.TableHeader:
		return b.appendMarkdownChildren(b.markdownElement(b.markdownElement(parent, "thead"), "tr"), node, source)
	case *extast.TableRow:
		body := parent
		if parent.Data == "table" {
			if count := len(parent.Children); count != 0 && parent.Children[count-1].Data == "tbody" {
				body = parent.Children[count-1]
			} else {
				body = b.markdownElement(parent, "tbody")
			}
		}
		return b.appendMarkdownChildren(b.markdownElement(body, "tr", markdownAttributes(node)...), node, source)
	case *extast.TableCell:
		tag := "td"
		if node.Parent().Kind() == extast.KindTableHeader {
			tag = "th"
		}
		cell := b.markdownElement(parent, tag, markdownAttributes(node)...)
		if value.Alignment != extast.AlignNone {
			cell.SetAttr("style", strings.TrimSuffix(cell.AttrValue("style"), ";")+"text-align:"+value.Alignment.String())
		}
		return b.appendMarkdownChildren(cell, node, source)
	case *markdownInlineMath:
		b.appendMarkdownMathNode(b.markdownElement(parent, "span", uinode.Attribute{Name: "class", Value: "math inline"}), parseMarkdownMath(value.Content))
		return true
	case *markdownDisplayMath:
		b.appendMarkdownMathNode(b.markdownElement(parent, "div", uinode.Attribute{Name: "class", Value: "math display"}), parseMarkdownMath(value.Lines().Value(source)))
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

func (b *markdownNodeBuilder) appendMarkdownCodeBlock(parent *uinode.Node, block *ast.FencedCodeBlock, source []byte) {
	language := block.Language(source)
	code := markdownCodeBlockSource(block, source)
	codeNode := b.markdownElement(b.markdownElement(parent, "pre"), "code")
	if len(language) != 0 {
		codeNode.SetAttr("class", "language-"+string(language))
	}
	var document syntax.Document
	defer document.Close()
	if len(language) != 0 && document.Update(string(language), code) == nil && document.Language() != "" {
		codeNode.SetAttr("class", "syntax language-"+string(language))
		offset := 0
		for _, token := range document.Tokens(markdownRuntimeFor().style, nil) {
			b.markdownText(codeNode, code[offset:token.Start])
			b.markdownText(b.markdownElement(codeNode, "span", uinode.Attribute{Name: "class", Value: "syntax-token"}, uinode.Attribute{Name: "style", Value: token.CSS}), code[token.Start:token.End])
			offset = token.End
		}
		b.markdownText(codeNode, code[offset:])
		return
	}
	b.markdownText(codeNode, code)
}
