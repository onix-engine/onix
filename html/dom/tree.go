// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license in LICENSE.go-html.
// Adapted for Onix: source views, direct DOM construction and reusable storage.

package dom

import (
	"fmt"
	"maps"
	"strings"
)

// A treeBuilder implements the HTML5 parsing algorithm:
type treeBuilder struct {
	arena                 *Parser
	tokenizer             *htmlScanner
	tok                   htmlToken
	hasSelfClosingToken   bool
	ignoreLF              bool
	doc                   *Node
	oe, afe               nodeStack
	head, form            *Node
	scripting, framesetOK bool
	templateStack         insertionModeStack
	im                    insertionMode
	originalIM            insertionMode
	fosterParenting       bool
	quirks                bool
	fragment              bool
	context               *Node
}

func (p *treeBuilder) top() *Node {
	if n := p.oe.top(); n != nil {
		return n
	}
	return p.doc
}

var (
	defaultScopeStopTags = map[string][]string{
		"":     {"applet", "caption", "html", "table", "td", "th", "marquee", "object", "template", "select"},
		"math": {"annotation-xml", "mi", "mn", "mo", "ms", "mtext"},
		"svg":  {"desc", "foreignObject", "title"},
	}
)

type scope int

const (
	defaultScope scope = iota
	listItemScope
	buttonScope
	tableScope
	tableRowScope
	tableBodyScope
)

func (p *treeBuilder) popUntil(s scope, matchTags ...string) bool {
	if i := p.indexOfElementInScope(s, matchTags...); i != -1 {
		p.oe = p.oe[:i]
		return true
	}
	return false
}

func (p *treeBuilder) indexOfElementInScope(s scope, matchTags ...string) int {
	for i := len(p.oe) - 1; i >= 0; i-- {
		tagName := p.oe[i].Data
		if p.oe[i].namespace == "" {
			for _, t := range matchTags {
				if t == tagName {
					return i
				}
			}
			switch s {
			case defaultScope:
			case listItemScope:
				if tagName == "ol" || tagName == "ul" {
					return -1
				}
			case buttonScope:
				if tagName == "button" {
					return -1
				}
			case tableScope:
				if tagName == "html" || tagName == "table" || tagName == "template" {
					return -1
				}
			default:
				panic(fmt.Sprintf("html: internal error: indexOfElementInScope unknown scope: %d", s))
			}
		}
		switch s {
		case defaultScope, listItemScope, buttonScope:
			for _, t := range defaultScopeStopTags[p.oe[i].namespace] {
				if t == tagName {
					return -1
				}
			}
		}
	}
	return -1
}

func (p *treeBuilder) elementInScope(s scope, matchTags ...string) bool {
	return p.indexOfElementInScope(s, matchTags...) != -1
}

func (p *treeBuilder) clearStackToContext(s scope) {
	for i := len(p.oe) - 1; i >= 0; i-- {
		tagName := p.oe[i].Data
		switch s {
		case tableScope:
			if tagName == "html" || tagName == "table" || tagName == "template" {
				p.oe = p.oe[:i+1]
				return
			}
		case tableRowScope:
			if tagName == "html" || tagName == "tr" || tagName == "template" {
				p.oe = p.oe[:i+1]
				return
			}
		case tableBodyScope:
			if tagName == "html" || tagName == "tbody" || tagName == "tfoot" || tagName == "thead" || tagName == "template" {
				p.oe = p.oe[:i+1]
				return
			}
		default:
			panic(fmt.Sprintf("html: internal error: clearStackToContext unknown scope: %d", s))
		}
	}
}

func (p *treeBuilder) parseGenericRawTextElement() {
	p.addElement()
	p.originalIM = p.im
	p.im = textIM
}

func (p *treeBuilder) generateImpliedEndTags(exceptions ...string) {
	var i int
loop:
	for i = len(p.oe) - 1; i >= 0; i-- {
		n := p.oe[i]
		if n.Type != ElementNode {
			break
		}
		switch n.Data {
		case "dd", "dt", "li", "optgroup", "option", "p", "rb", "rp", "rt", "rtc":
			for _, except := range exceptions {
				if n.Data == except {
					break loop
				}
			}
			continue
		}
		break
	}

	p.oe = p.oe[:i+1]
}

func (p *treeBuilder) addChild(n *Node) {
	if p.shouldFosterParent() {
		p.fosterParent(n)
	} else {
		p.top().AppendChild(n)
	}

	if n.Type == ElementNode {
		p.insertOpenElement(n)
	}
}

func (p *treeBuilder) insertOpenElement(n *Node) {
	p.oe = append(p.oe, n)
	if len(p.oe) > 512 {
		panic(ErrNestingDepth)
	}
}

func (p *treeBuilder) shouldFosterParent() bool {
	if p.fosterParenting {
		switch p.top().Data {
		case "table", "tbody", "tfoot", "thead", "tr":
			return true
		}
	}
	return false
}

func (p *treeBuilder) fosterParent(n *Node) {
	var table, parent, prev, template *Node
	var i int
	for i = len(p.oe) - 1; i >= 0; i-- {
		if p.oe[i].Data == "table" {
			table = p.oe[i]
			break
		}
	}

	var j int
	for j = len(p.oe) - 1; j >= 0; j-- {
		if p.oe[j].Data == "template" {
			template = p.oe[j]
			break
		}
	}

	if template != nil && (table == nil || j > i) {
		template.AppendChild(n)
		return
	}

	if table == nil {
		parent = p.oe[0]
	} else {
		parent = table.Parent
	}
	if parent == nil {
		parent = p.oe[i-1]
	}

	if table != nil {
		prev = previousSibling(table)
	} else {
		prev = lastChild(parent)
	}
	if prev != nil && prev.Type == TextNode && n.Type == TextNode {
		p.arena.appendText(prev, n.Data)
		return
	}

	parent.InsertBefore(n, table)
}

func (p *treeBuilder) addText(text string) {
	if text == "" {
		return
	}

	if p.shouldFosterParent() {
		p.fosterParent(p.arena.newNode(TextNode, text))
		return
	}

	t := p.top()
	if n := lastChild(t); n != nil && n.Type == TextNode {
		p.arena.appendText(n, text)
		return
	}
	p.addChild(p.arena.newNode(TextNode, text))
}

func (p *treeBuilder) addElement() {
	n := p.arena.newNode(ElementNode, p.tok.Data)
	copyTokenAttributes(n, p.tok)
	p.addChild(n)
}

func (p *treeBuilder) addFormattingElement() {
	tagName := p.tok.Data
	p.addElement()

	identicalElements := 0
	for i := len(p.afe) - 1; i >= 0; i-- {
		n := p.afe[i]
		if n.Type == scopeMarkerNode {
			break
		}
		if n.Type != ElementNode {
			continue
		}
		if n.namespace != "" {
			continue
		}
		if n.Data != tagName {
			continue
		}
		if !maps.Equal(n.Attributes, p.top().Attributes) {
			continue
		}

		identicalElements++
		if identicalElements >= 3 {
			p.afe.remove(n)
		}
	}

	top := p.top()

	p.afe = append(p.afe, top)
}

func (p *treeBuilder) clearActiveFormattingElements() {
	for {
		if n := p.afe.pop(); len(p.afe) == 0 || n.Type == scopeMarkerNode {
			return
		}
	}
}

func (p *treeBuilder) reconstructActiveFormattingElements() {
	n := p.afe.top()
	if n == nil {
		return
	}
	if n.Type == scopeMarkerNode || p.oe.index(n) != -1 {
		return
	}
	i := len(p.afe) - 1
	for n.Type != scopeMarkerNode && p.oe.index(n) == -1 {
		if i == 0 {
			i = -1
			break
		}
		i--
		n = p.afe[i]
	}
	for {
		i++
		clone := p.arena.cloneNode(p.afe[i])
		p.addChild(clone)
		p.afe[i] = clone
		if i == len(p.afe)-1 {
			break
		}
	}
}

func (p *treeBuilder) acknowledgeSelfClosingTag() {
	p.hasSelfClosingToken = false
}

type insertionMode func(*treeBuilder) bool

func (p *treeBuilder) setOriginalIM() {
	if p.originalIM != nil {
		panic("html: bad treeBuilder state: originalIM was set twice")
	}
	p.originalIM = p.im
}

func (p *treeBuilder) resetInsertionMode() {
	for i := len(p.oe) - 1; i >= 0; i-- {
		n := p.oe[i]
		last := i == 0
		if last && p.context != nil {
			n = p.context
		}

		switch n.Data {
		case "td", "th":
			p.im = inCellIM
		case "tr":
			p.im = inRowIM
		case "tbody", "thead", "tfoot":
			p.im = inTableBodyIM
		case "caption":
			p.im = inCaptionIM
		case "colgroup":
			p.im = inColumnGroupIM
		case "table":
			p.im = inTableIM
		case "template":
			if n.namespace != "" {
				continue
			}
			p.im = p.templateStack.top()
		case "head":
			p.im = inHeadIM
		case "body":
			p.im = inBodyIM
		case "frameset":
			p.im = inFramesetIM
		case "html":
			if p.head == nil {
				p.im = beforeHeadIM
			} else {
				p.im = afterHeadIM
			}
		default:
			if last {
				p.im = inBodyIM
				return
			}
			continue
		}
		return
	}
}

const whitespace = " \t\r\n\f"

func initialIM(p *treeBuilder) bool {
	switch p.tok.Type {
	case TokenText:
		p.tok.Data = strings.TrimLeft(p.tok.Data, whitespace)
		if len(p.tok.Data) == 0 {
			return true
		}
	case tokenComment:
		return true
	case tokenDoctype:
		p.quirks = p.arena.doctypeQuirks(p.tok.Data)
		p.im = beforeHTMLIM
		return true
	}
	p.quirks = true
	p.im = beforeHTMLIM
	return false
}

func beforeHTMLIM(p *treeBuilder) bool {
	switch p.tok.Type {
	case tokenDoctype:
		return true
	case TokenText:
		p.tok.Data = strings.TrimLeft(p.tok.Data, whitespace)
		if len(p.tok.Data) == 0 {
			return true
		}
	case TokenStartTag:
		if p.tok.Data == "html" {
			p.addElement()
			p.im = beforeHeadIM
			return true
		}
	case TokenEndTag:
		switch p.tok.Data {
		case "head", "body", "html", "br":
			p.parseImpliedToken(TokenStartTag, "html")
			return false
		default:
			return true
		}
	case tokenComment:
		return true
	}
	p.parseImpliedToken(TokenStartTag, "html")
	return false
}

func beforeHeadIM(p *treeBuilder) bool {
	switch p.tok.Type {
	case TokenText:
		p.tok.Data = strings.TrimLeft(p.tok.Data, whitespace)
		if len(p.tok.Data) == 0 {
			return true
		}
	case TokenStartTag:
		switch p.tok.Data {
		case "head":
			p.addElement()
			p.head = p.top()
			p.im = inHeadIM
			return true
		case "html":
			return inBodyIM(p)
		}
	case TokenEndTag:
		switch p.tok.Data {
		case "head", "body", "html", "br":
			p.parseImpliedToken(TokenStartTag, "head")
			return false
		default:
			return true
		}
	case tokenComment:
		return true
	case tokenDoctype:
		return true
	}

	p.parseImpliedToken(TokenStartTag, "head")
	return false
}

func inHeadIM(p *treeBuilder) bool {
	switch p.tok.Type {
	case TokenText:
		s := strings.TrimLeft(p.tok.Data, whitespace)
		if len(s) < len(p.tok.Data) {
			p.addText(p.tok.Data[:len(p.tok.Data)-len(s)])
			if s == "" {
				return true
			}
			p.tok.Data = s
		}
	case TokenStartTag:
		switch p.tok.Data {
		case "html":
			return inBodyIM(p)
		case "base", "basefont", "bgsound", "link", "meta":
			p.addElement()
			p.oe.pop()
			p.acknowledgeSelfClosingTag()
			return true
		case "noscript":
			if p.scripting {
				p.parseGenericRawTextElement()
				return true
			}
			p.addElement()
			p.im = inHeadNoscriptIM
			p.tokenizer.NextIsNotRawText()
			return true
		case "script", "title":
			p.addElement()
			p.setOriginalIM()
			p.im = textIM
			return true
		case "noframes", "style":
			p.parseGenericRawTextElement()
			return true
		case "head":
			return true
		case "template":
			p.addElement()
			p.afe = append(p.afe, &scopeMarker)
			p.framesetOK = false
			p.im = inTemplateIM
			p.templateStack = append(p.templateStack, inTemplateIM)
			return true
		}
	case TokenEndTag:
		switch p.tok.Data {
		case "head":
			p.oe.pop()
			p.im = afterHeadIM
			return true
		case "body", "html", "br":
			p.parseImpliedToken(TokenEndTag, "head")
			return false
		case "template":
			if !p.oe.contains("template") {
				return true
			}
			p.generateImpliedEndTags()
			for i := len(p.oe) - 1; i >= 0; i-- {
				if n := p.oe[i]; n.namespace == "" && n.Data == "template" {
					p.oe = p.oe[:i]
					break
				}
			}
			p.clearActiveFormattingElements()
			p.templateStack.pop()
			p.resetInsertionMode()
			return true
		default:
			return true
		}
	case tokenComment:
		return true
	case tokenDoctype:
		return true
	}

	p.parseImpliedToken(TokenEndTag, "head")
	return false
}

func inHeadNoscriptIM(p *treeBuilder) bool {
	switch p.tok.Type {
	case tokenDoctype:
		return true
	case TokenStartTag:
		switch p.tok.Data {
		case "html":
			return inBodyIM(p)
		case "basefont", "bgsound", "link", "meta", "noframes", "style":
			return inHeadIM(p)
		case "head":
			return true
		case "noscript":
			p.tokenizer.NextIsNotRawText()
			return true
		}
	case TokenEndTag:
		switch p.tok.Data {
		case "noscript", "br":
		default:
			return true
		}
	case TokenText:
		s := strings.TrimLeft(p.tok.Data, whitespace)
		if len(s) == 0 {
			return inHeadIM(p)
		}
	case tokenComment:
		return inHeadIM(p)
	}
	p.oe.pop()
	if p.top().Data != "head" {
		panic("html: the new current node will be a head element.")
	}
	p.im = inHeadIM
	if p.tok.Data == "noscript" {
		return true
	}
	return false
}

func afterHeadIM(p *treeBuilder) bool {
	switch p.tok.Type {
	case TokenText:
		s := strings.TrimLeft(p.tok.Data, whitespace)
		if len(s) < len(p.tok.Data) {
			p.addText(p.tok.Data[:len(p.tok.Data)-len(s)])
			if s == "" {
				return true
			}
			p.tok.Data = s
		}
	case TokenStartTag:
		switch p.tok.Data {
		case "html":
			return inBodyIM(p)
		case "body":
			p.addElement()
			p.framesetOK = false
			p.im = inBodyIM
			return true
		case "frameset":
			p.addElement()
			p.im = inFramesetIM
			return true
		case "base", "basefont", "bgsound", "link", "meta", "noframes", "script", "style", "template", "title":
			p.insertOpenElement(p.head)
			defer p.oe.remove(p.head)
			return inHeadIM(p)
		case "head":
			return true
		}
	case TokenEndTag:
		switch p.tok.Data {
		case "body", "html", "br":
		case "template":
			return inHeadIM(p)
		default:
			return true
		}
	case tokenComment:
		return true
	case tokenDoctype:
		return true
	}

	p.parseImpliedToken(TokenStartTag, "body")
	p.framesetOK = true
	if p.tok.Type == TokenEOF {
		return true
	}
	return false
}

func inBodyIM(p *treeBuilder) bool {
	switch p.tok.Type {
	case TokenText:
		d := p.tok.Data
		switch n := p.oe.top(); n.Data {
		case "pre", "listing":
			if p.ignoreLF {
				if d != "" && d[0] == '\r' {
					d = d[1:]
				}
				if d != "" && d[0] == '\n' {
					d = d[1:]
				}
			}
		}
		d = p.arena.replaceNUL(d, false)
		if d == "" {
			return true
		}
		p.reconstructActiveFormattingElements()
		p.addText(d)
		if p.framesetOK && strings.TrimLeft(d, whitespace) != "" {
			p.framesetOK = false
		}
	case TokenStartTag:
		switch p.tok.Data {
		case "html":
			if p.oe.contains("template") {
				return true
			}
			if !p.fragment {
				copyTokenAttributes(p.oe[0], p.tok)
			}
		case "base", "basefont", "bgsound", "link", "meta", "noframes", "script", "style", "template", "title":
			return inHeadIM(p)
		case "body":
			if p.oe.contains("template") {
				return true
			}
			if len(p.oe) >= 2 {
				body := p.oe[1]
				if body.Type == ElementNode && body.Data == "body" {
					p.framesetOK = false
					copyTokenAttributes(body, p.tok)
				}
			}
		case "frameset":
			if !p.framesetOK || len(p.oe) < 2 || p.oe[1].Data != "body" {
				return true
			}
			body := p.oe[1]
			if body.Parent != nil {
				body.Parent.RemoveChild(body)
			}
			p.oe = p.oe[:1]
			p.addElement()
			p.im = inFramesetIM
			return true
		case "address", "article", "aside", "blockquote", "center", "details", "dialog", "dir", "div", "dl", "fieldset", "figcaption", "figure", "footer", "header", "hgroup", "main", "menu", "nav", "ol", "p", "search", "section", "summary", "ul":
			p.popUntil(buttonScope, "p")
			p.addElement()
		case "h1", "h2", "h3", "h4", "h5", "h6":
			p.popUntil(buttonScope, "p")
			switch n := p.top(); n.Data {
			case "h1", "h2", "h3", "h4", "h5", "h6":
				p.oe.pop()
			}
			p.addElement()
		case "pre", "listing":
			p.popUntil(buttonScope, "p")
			p.addElement()
			p.framesetOK = false
		case "form":
			if p.form != nil && !p.oe.contains("template") {
				return true
			}
			p.popUntil(buttonScope, "p")
			p.addElement()
			if !p.oe.contains("template") {
				p.form = p.top()
			}
		case "li":
			p.framesetOK = false
			for i := len(p.oe) - 1; i >= 0; i-- {
				node := p.oe[i]
				switch node.Data {
				case "li":
					p.oe = p.oe[:i]
				case "address", "div", "p":
					continue
				default:
					if !isSpecialElement(node) {
						continue
					}
				}
				break
			}
			p.popUntil(buttonScope, "p")
			p.addElement()
		case "dd", "dt":
			p.framesetOK = false
			for i := len(p.oe) - 1; i >= 0; i-- {
				node := p.oe[i]
				switch node.Data {
				case "dd", "dt":
					p.oe = p.oe[:i]
				case "address", "div", "p":
					continue
				default:
					if !isSpecialElement(node) {
						continue
					}
				}
				break
			}
			p.popUntil(buttonScope, "p")
			p.addElement()
		case "plaintext":
			p.popUntil(buttonScope, "p")
			p.addElement()
		case "button":
			if p.elementInScope(defaultScope, "button") {
				p.generateImpliedEndTags()
				p.popUntil(defaultScope, "button")
			}
			p.reconstructActiveFormattingElements()
			p.addElement()
			p.framesetOK = false
		case "a":
			for i := len(p.afe) - 1; i >= 0 && p.afe[i].Type != scopeMarkerNode; i-- {
				if n := p.afe[i]; n.Type == ElementNode && n.Data == "a" {
					p.inBodyEndTagFormatting("a")
					p.oe.remove(n)
					p.afe.remove(n)
					break
				}
			}
			p.reconstructActiveFormattingElements()
			p.addFormattingElement()
		case "b", "big", "code", "em", "font", "i", "s", "small", "strike", "strong", "tt", "u":
			p.reconstructActiveFormattingElements()
			p.addFormattingElement()
		case "nobr":
			p.reconstructActiveFormattingElements()
			if p.elementInScope(defaultScope, "nobr") {
				p.inBodyEndTagFormatting("nobr")
				p.reconstructActiveFormattingElements()
			}
			p.addFormattingElement()
		case "applet", "marquee", "object":
			p.reconstructActiveFormattingElements()
			p.addElement()
			p.afe = append(p.afe, &scopeMarker)
			p.framesetOK = false
		case "table":
			if !p.quirks {
				p.popUntil(buttonScope, "p")
			}
			p.addElement()
			p.framesetOK = false
			p.im = inTableIM
			return true
		case "area", "br", "embed", "img", "keygen", "wbr":
			p.reconstructActiveFormattingElements()
			p.addElement()
			p.oe.pop()
			p.acknowledgeSelfClosingTag()
			p.framesetOK = false
		case "input":
			if p.fragment && p.context.Data == "select" {
				return true
			}
			p.popUntil(defaultScope, "select")
			p.reconstructActiveFormattingElements()
			p.addElement()
			p.oe.pop()
			p.acknowledgeSelfClosingTag()
			if p.tok.Data == "input" {
				for _, t := range p.tok.Attr {
					if t.Key == "type" {
						if strings.EqualFold(t.Val, "hidden") {
							return true
						}
					}
				}
			}
			p.framesetOK = false
		case "param", "source", "track":
			p.addElement()
			p.oe.pop()
			p.acknowledgeSelfClosingTag()
		case "hr":
			if p.elementInScope(buttonScope, "p") {
				p.generateImpliedEndTags("p")
				p.popUntil(defaultScope, "p")
			}
			if p.elementInScope(defaultScope, "select") {
				p.generateImpliedEndTags()
			}
			p.addElement()
			p.oe.pop()
			p.acknowledgeSelfClosingTag()
			p.framesetOK = false
		case "image":
			p.tok.Data = "img"
			return false
		case "textarea":
			p.addElement()
			p.setOriginalIM()
			p.framesetOK = false
			p.im = textIM
		case "xmp":
			p.popUntil(buttonScope, "p")
			p.reconstructActiveFormattingElements()
			p.framesetOK = false
			p.parseGenericRawTextElement()
		case "iframe":
			p.framesetOK = false
			p.parseGenericRawTextElement()
		case "noembed":
			p.parseGenericRawTextElement()
		case "noscript":
			if p.scripting {
				p.parseGenericRawTextElement()
				return true
			}
			p.reconstructActiveFormattingElements()
			p.addElement()
			p.tokenizer.NextIsNotRawText()
		case "select":
			if p.fragment && p.context.Data == "select" {
				return true
			} else if p.popUntil(defaultScope, "select") {
				return true
			}
			p.reconstructActiveFormattingElements()
			p.addElement()
			p.framesetOK = false
			return true
		case "option":
			if p.elementInScope(defaultScope, "select") {
				p.generateImpliedEndTags("optgroup")
			} else if p.top().Data == "option" {
				p.oe.pop()
			}
			p.reconstructActiveFormattingElements()
			p.addElement()
		case "optgroup":
			if p.elementInScope(defaultScope, "select") {
				p.generateImpliedEndTags()
			} else if p.top().Data == "option" {
				p.oe.pop()
			}
			p.reconstructActiveFormattingElements()
			p.addElement()
		case "rb", "rtc":
			if p.elementInScope(defaultScope, "ruby") {
				p.generateImpliedEndTags()
			}
			p.addElement()
		case "rp", "rt":
			if p.elementInScope(defaultScope, "ruby") {
				p.generateImpliedEndTags("rtc")
			}
			p.addElement()
		case "math", "svg":
			p.reconstructActiveFormattingElements()
			if p.tok.Data == "math" {
				adjustAttributeNames(p.tok.Attr, mathMLAttributeAdjustments)
			} else {
				adjustAttributeNames(p.tok.Attr, svgAttributeAdjustments)
			}

			p.addElement()
			p.top().namespace = p.tok.Data
			if p.hasSelfClosingToken {
				p.oe.pop()
				p.acknowledgeSelfClosingTag()
			}
			return true
		case "caption", "col", "colgroup", "frame", "head", "tbody", "td", "tfoot", "th", "thead", "tr":
		default:
			p.reconstructActiveFormattingElements()
			p.addElement()
		}
	case TokenEndTag:
		switch p.tok.Data {
		case "body":
			if p.elementInScope(defaultScope, "body") {
				p.im = afterBodyIM
			}
		case "html":
			if p.elementInScope(defaultScope, "body") {
				p.parseImpliedToken(TokenEndTag, "body")
				return false
			}
			return true
		case "address", "article", "aside", "blockquote", "button", "center", "details", "dialog", "dir", "div", "dl", "fieldset", "figcaption", "figure", "footer", "header", "hgroup", "listing", "main", "menu", "nav", "ol", "pre", "search", "section", "select", "summary", "ul":
			if !p.elementInScope(defaultScope, p.tok.Data) {
				return true
			}
			p.generateImpliedEndTags()
			p.popUntil(defaultScope, p.tok.Data)
		case "form":
			if p.oe.contains("template") {
				i := p.indexOfElementInScope(defaultScope, "form")
				if i == -1 {
					return true
				}
				p.generateImpliedEndTags()
				if p.oe[i].Data != "form" {
					return true
				}
				p.popUntil(defaultScope, "form")
			} else {
				node := p.form
				p.form = nil
				i := p.indexOfElementInScope(defaultScope, "form")
				if node == nil || i == -1 || p.oe[i] != node {
					return true
				}
				p.generateImpliedEndTags()
				p.oe.remove(node)
			}
		case "p":
			if !p.elementInScope(buttonScope, "p") {
				p.parseImpliedToken(TokenStartTag, "p")
			}
			p.popUntil(buttonScope, "p")
		case "li":
			p.popUntil(listItemScope, "li")
		case "dd", "dt":
			p.popUntil(defaultScope, p.tok.Data)
		case "h1", "h2", "h3", "h4", "h5", "h6":
			p.popUntil(defaultScope, "h1", "h2", "h3", "h4", "h5", "h6")
		case "a", "b", "big", "code", "em", "font", "i", "nobr", "s", "small", "strike", "strong", "tt", "u":
			p.inBodyEndTagFormatting(p.tok.Data)
		case "applet", "marquee", "object":
			if p.popUntil(defaultScope, p.tok.Data) {
				p.clearActiveFormattingElements()
			}
		case "br":
			p.tok.Type = TokenStartTag
			return false
		case "template":
			return inHeadIM(p)
		default:
			p.inBodyEndTagOther(p.tok.Data)
		}
	case tokenComment:
	case TokenEOF:
		if len(p.templateStack) > 0 {
			p.im = inTemplateIM
			return false
		}
		for _, e := range p.oe {
			switch e.Data {
			case "dd", "dt", "li", "optgroup", "option", "p", "rb", "rp", "rt", "rtc", "tbody", "td", "tfoot", "th",
				"thead", "tr", "body", "html":
			default:
				return true
			}
		}
	}

	return true
}

func (p *treeBuilder) inBodyEndTagFormatting(tagName string) {

	if current := p.oe.top(); current.Data == tagName && p.afe.index(current) == -1 {
		p.oe.pop()
		return
	}

	for i := 0; i < 8; i++ {
		var formattingElement *Node
		for j := len(p.afe) - 1; j >= 0; j-- {
			if p.afe[j].Type == scopeMarkerNode {
				break
			}
			if p.afe[j].Data == tagName {
				formattingElement = p.afe[j]
				break
			}
		}
		if formattingElement == nil {
			p.inBodyEndTagOther(tagName)
			return
		}

		feIndex := p.oe.index(formattingElement)
		if feIndex == -1 {
			p.afe.remove(formattingElement)
			return
		}
		if !p.elementInScope(defaultScope, tagName) {
			return
		}

		var furthestBlock *Node
		for _, e := range p.oe[feIndex:] {
			if isSpecialElement(e) {
				furthestBlock = e
				break
			}
		}
		if furthestBlock == nil {
			e := p.oe.pop()
			for e != formattingElement {
				e = p.oe.pop()
			}
			p.afe.remove(e)
			return
		}

		commonAncestor := p.oe[feIndex-1]
		bookmark := p.afe.index(formattingElement)

		lastNode := furthestBlock
		node := furthestBlock
		x := p.oe.index(node)
		j := 0
		for {
			j++
			x--
			node = p.oe[x]
			if node == formattingElement {
				break
			}
			if ni := p.afe.index(node); j > 3 && ni > -1 {
				p.afe.remove(node)
				if ni <= bookmark {
					bookmark--
				}
				continue
			}
			if p.afe.index(node) == -1 {
				p.oe.remove(node)
				continue
			}
			clone := p.arena.cloneNode(node)
			p.afe[p.afe.index(node)] = clone
			p.oe[p.oe.index(node)] = clone
			node = clone
			if lastNode == furthestBlock {
				bookmark = p.afe.index(node) + 1
			}
			if lastNode.Parent != nil {
				lastNode.Parent.RemoveChild(lastNode)
			}
			node.AppendChild(lastNode)
			lastNode = node
		}

		if lastNode.Parent != nil {
			lastNode.Parent.RemoveChild(lastNode)
		}
		switch commonAncestor.Data {
		case "table", "tbody", "tfoot", "thead", "tr":
			p.fosterParent(lastNode)
		default:
			commonAncestor.AppendChild(lastNode)
		}

		clone := p.arena.cloneNode(formattingElement)
		reparentChildren(clone, furthestBlock)
		furthestBlock.AppendChild(clone)

		if oldLoc := p.afe.index(formattingElement); oldLoc != -1 && oldLoc < bookmark {
			bookmark--
		}
		p.afe.remove(formattingElement)
		p.afe.insert(bookmark, clone)

		p.oe.remove(formattingElement)
		p.oe.insert(p.oe.index(furthestBlock)+1, clone)
	}
}

func (p *treeBuilder) inBodyEndTagOther(tagName string) {
	for i := len(p.oe) - 1; i >= 0; i-- {
		if p.oe[i].namespace == "" && p.oe[i].Data == tagName {
			p.oe = p.oe[:i]
			break
		}
		if isSpecialElement(p.oe[i]) {
			break
		}
	}
}

func textIM(p *treeBuilder) bool {
	switch p.tok.Type {
	case TokenEOF:
		p.oe.pop()
	case TokenText:
		d := p.tok.Data
		if n := p.oe.top(); n.Data == "textarea" && p.ignoreLF {
			if d != "" && d[0] == '\r' {
				d = d[1:]
			}
			if d != "" && d[0] == '\n' {
				d = d[1:]
			}
		}
		if d == "" {
			return true
		}
		p.addText(d)
		return true
	case TokenEndTag:
		p.oe.pop()
	}
	p.im = p.originalIM
	p.originalIM = nil
	return p.tok.Type == TokenEndTag
}

func inTableIM(p *treeBuilder) bool {
	switch p.tok.Type {
	case TokenText:
		p.tok.Data = p.arena.replaceNUL(p.tok.Data, false)
		switch p.oe.top().Data {
		case "table", "tbody", "tfoot", "thead", "tr":
			if strings.Trim(p.tok.Data, whitespace) == "" {
				p.addText(p.tok.Data)
				return true
			}
		}
	case TokenStartTag:
		switch p.tok.Data {
		case "caption":
			p.clearStackToContext(tableScope)
			p.afe = append(p.afe, &scopeMarker)
			p.addElement()
			p.im = inCaptionIM
			return true
		case "colgroup":
			p.clearStackToContext(tableScope)
			p.addElement()
			p.im = inColumnGroupIM
			return true
		case "col":
			p.parseImpliedToken(TokenStartTag, "colgroup")
			return false
		case "tbody", "tfoot", "thead":
			p.clearStackToContext(tableScope)
			p.addElement()
			p.im = inTableBodyIM
			return true
		case "td", "th", "tr":
			p.parseImpliedToken(TokenStartTag, "tbody")
			return false
		case "table":
			if p.popUntil(tableScope, "table") {
				p.resetInsertionMode()
				return false
			}
			return true
		case "style", "script", "template":
			return inHeadIM(p)
		case "input":
			for _, t := range p.tok.Attr {
				if t.Key == "type" && strings.EqualFold(t.Val, "hidden") {
					p.addElement()
					p.oe.pop()
					return true
				}
			}
		case "form":
			if p.oe.contains("template") || p.form != nil {
				return true
			}
			p.addElement()
			p.form = p.oe.pop()
		}
	case TokenEndTag:
		switch p.tok.Data {
		case "table":
			if p.popUntil(tableScope, "table") {
				p.resetInsertionMode()
				return true
			}
			return true
		case "body", "caption", "col", "colgroup", "html", "tbody", "td", "tfoot", "th", "thead", "tr":
			return true
		case "template":
			return inHeadIM(p)
		}
	case tokenComment:
		return true
	case tokenDoctype:
		return true
	case TokenEOF:
		return inBodyIM(p)
	}

	p.fosterParenting = true
	defer func() { p.fosterParenting = false }()

	return inBodyIM(p)
}

func inCaptionIM(p *treeBuilder) bool {
	switch p.tok.Type {
	case TokenStartTag:
		switch p.tok.Data {
		case "caption", "col", "colgroup", "tbody", "td", "tfoot", "thead", "tr":
			if !p.popUntil(tableScope, "caption") {
				return true
			}
			p.clearActiveFormattingElements()
			p.im = inTableIM
			return false
		}
	case TokenEndTag:
		switch p.tok.Data {
		case "caption":
			if p.popUntil(tableScope, "caption") {
				p.clearActiveFormattingElements()
				p.im = inTableIM
			}
			return true
		case "table":
			if !p.popUntil(tableScope, "caption") {
				return true
			}
			p.clearActiveFormattingElements()
			p.im = inTableIM
			return false
		case "body", "col", "colgroup", "html", "tbody", "td", "tfoot", "th", "thead", "tr":
			return true
		}
	}
	return inBodyIM(p)
}

func inColumnGroupIM(p *treeBuilder) bool {
	switch p.tok.Type {
	case TokenText:
		s := strings.TrimLeft(p.tok.Data, whitespace)
		if len(s) < len(p.tok.Data) {
			p.addText(p.tok.Data[:len(p.tok.Data)-len(s)])
			if s == "" {
				return true
			}
			p.tok.Data = s
		}
	case tokenComment:
		return true
	case tokenDoctype:
		return true
	case TokenStartTag:
		switch p.tok.Data {
		case "html":
			return inBodyIM(p)
		case "col":
			p.addElement()
			p.oe.pop()
			p.acknowledgeSelfClosingTag()
			return true
		case "template":
			return inHeadIM(p)
		}
	case TokenEndTag:
		switch p.tok.Data {
		case "colgroup":
			if p.oe.top().Data == "colgroup" {
				p.oe.pop()
				p.im = inTableIM
			}
			return true
		case "col":
			return true
		case "template":
			return inHeadIM(p)
		}
	case TokenEOF:
		return inBodyIM(p)
	}
	if p.oe.top().Data != "colgroup" {
		return true
	}
	p.oe.pop()
	p.im = inTableIM
	return false
}

func inTableBodyIM(p *treeBuilder) bool {
	switch p.tok.Type {
	case TokenStartTag:
		switch p.tok.Data {
		case "tr":
			p.clearStackToContext(tableBodyScope)
			p.addElement()
			p.im = inRowIM
			return true
		case "td", "th":
			p.parseImpliedToken(TokenStartTag, "tr")
			return false
		case "caption", "col", "colgroup", "tbody", "tfoot", "thead":
			if p.popUntil(tableScope, "tbody", "thead", "tfoot") {
				p.im = inTableIM
				return false
			}
			return true
		}
	case TokenEndTag:
		switch p.tok.Data {
		case "tbody", "tfoot", "thead":
			if p.elementInScope(tableScope, p.tok.Data) {
				p.clearStackToContext(tableBodyScope)
				p.oe.pop()
				p.im = inTableIM
			}
			return true
		case "table":
			if p.popUntil(tableScope, "tbody", "thead", "tfoot") {
				p.im = inTableIM
				return false
			}
			return true
		case "body", "caption", "col", "colgroup", "html", "td", "th", "tr":
			return true
		}
	case tokenComment:
		return true
	}

	return inTableIM(p)
}

func inRowIM(p *treeBuilder) bool {
	switch p.tok.Type {
	case TokenStartTag:
		switch p.tok.Data {
		case "td", "th":
			p.clearStackToContext(tableRowScope)
			p.addElement()
			p.afe = append(p.afe, &scopeMarker)
			p.im = inCellIM
			return true
		case "caption", "col", "colgroup", "tbody", "tfoot", "thead", "tr":
			if p.elementInScope(tableScope, "tr") {
				p.clearStackToContext(tableRowScope)
				p.oe.pop()
				p.im = inTableBodyIM
				return false
			}
			return true
		}
	case TokenEndTag:
		switch p.tok.Data {
		case "tr":
			if p.elementInScope(tableScope, "tr") {
				p.clearStackToContext(tableRowScope)
				p.oe.pop()
				p.im = inTableBodyIM
				return true
			}
			return true
		case "table":
			if p.elementInScope(tableScope, "tr") {
				p.clearStackToContext(tableRowScope)
				p.oe.pop()
				p.im = inTableBodyIM
				return false
			}
			return true
		case "tbody", "tfoot", "thead":
			if p.elementInScope(tableScope, p.tok.Data) && p.elementInScope(tableScope, "tr") {
				p.clearStackToContext(tableRowScope)
				p.oe.pop()
				p.im = inTableBodyIM
				return false
			}
			return true
		case "body", "caption", "col", "colgroup", "html", "td", "th":
			return true
		}
	}

	return inTableIM(p)
}

func inCellIM(p *treeBuilder) bool {
	switch p.tok.Type {
	case TokenStartTag:
		switch p.tok.Data {
		case "caption", "col", "colgroup", "tbody", "td", "tfoot", "th", "thead", "tr":
			if p.popUntil(tableScope, "td", "th") {
				p.clearActiveFormattingElements()
				p.im = inRowIM
				return false
			}
			return true
		}
	case TokenEndTag:
		switch p.tok.Data {
		case "td", "th":
			if !p.popUntil(tableScope, p.tok.Data) {
				return true
			}
			p.clearActiveFormattingElements()
			p.im = inRowIM
			return true
		case "body", "caption", "col", "colgroup", "html":
			return true
		case "table", "tbody", "tfoot", "thead", "tr":
			if !p.elementInScope(tableScope, p.tok.Data) {
				return true
			}
			if p.popUntil(tableScope, "td", "th") {
				p.clearActiveFormattingElements()
			}
			p.im = inRowIM
			return false
		}
	}
	return inBodyIM(p)
}

func inTemplateIM(p *treeBuilder) bool {
	switch p.tok.Type {
	case TokenText, tokenComment, tokenDoctype:
		return inBodyIM(p)
	case TokenStartTag:
		switch p.tok.Data {
		case "base", "basefont", "bgsound", "link", "meta", "noframes", "script", "style", "template", "title":
			return inHeadIM(p)
		case "caption", "colgroup", "tbody", "tfoot", "thead":
			p.templateStack.pop()
			p.templateStack = append(p.templateStack, inTableIM)
			p.im = inTableIM
			return false
		case "col":
			p.templateStack.pop()
			p.templateStack = append(p.templateStack, inColumnGroupIM)
			p.im = inColumnGroupIM
			return false
		case "tr":
			p.templateStack.pop()
			p.templateStack = append(p.templateStack, inTableBodyIM)
			p.im = inTableBodyIM
			return false
		case "td", "th":
			p.templateStack.pop()
			p.templateStack = append(p.templateStack, inRowIM)
			p.im = inRowIM
			return false
		default:
			p.templateStack.pop()
			p.templateStack = append(p.templateStack, inBodyIM)
			p.im = inBodyIM
			return false
		}
	case TokenEndTag:
		switch p.tok.Data {
		case "template":
			return inHeadIM(p)
		default:
			return true
		}
	case TokenEOF:
		if !p.oe.contains("template") {
			return true
		}
		p.generateImpliedEndTags()
		for i := len(p.oe) - 1; i >= 0; i-- {
			if n := p.oe[i]; n.namespace == "" && n.Data == "template" {
				p.oe = p.oe[:i]
				break
			}
		}
		p.clearActiveFormattingElements()
		p.templateStack.pop()
		p.resetInsertionMode()
		return false
	}
	return false
}

func afterBodyIM(p *treeBuilder) bool {
	switch p.tok.Type {
	case TokenEOF:
		return true
	case TokenText:
		s := strings.TrimLeft(p.tok.Data, whitespace)
		if len(s) == 0 {
			return inBodyIM(p)
		}
	case TokenStartTag:
		if p.tok.Data == "html" {
			return inBodyIM(p)
		}
	case TokenEndTag:
		if p.tok.Data == "html" {
			if !p.fragment {
				p.im = afterAfterBodyIM
			}
			return true
		}
	case tokenComment:
		if len(p.oe) < 1 || p.oe[0].Data != "html" {
			panic("html: bad treeBuilder state: <html> element not found, in the after-body insertion mode")
		}
		return true
	}
	p.im = inBodyIM
	return false
}

func inFramesetIM(p *treeBuilder) bool {
	switch p.tok.Type {
	case tokenComment:
	case TokenText:
		s := strings.Map(func(c rune) rune {
			switch c {
			case ' ', '\t', '\n', '\f', '\r':
				return c
			}
			return -1
		}, p.tok.Data)
		if s != "" {
			p.addText(s)
		}
	case TokenStartTag:
		switch p.tok.Data {
		case "html":
			return inBodyIM(p)
		case "frameset":
			p.addElement()
		case "frame":
			p.addElement()
			p.oe.pop()
			p.acknowledgeSelfClosingTag()
		case "noframes":
			return inHeadIM(p)
		}
	case TokenEndTag:
		switch p.tok.Data {
		case "frameset":
			if p.oe.top().Data != "html" {
				p.oe.pop()
				if p.oe.top().Data != "frameset" {
					p.im = afterFramesetIM
					return true
				}
			}
		}
	default:
	}
	return true
}

func afterFramesetIM(p *treeBuilder) bool {
	switch p.tok.Type {
	case tokenComment:
	case TokenText:
		s := strings.Map(func(c rune) rune {
			switch c {
			case ' ', '\t', '\n', '\f', '\r':
				return c
			}
			return -1
		}, p.tok.Data)
		if s != "" {
			p.addText(s)
		}
	case TokenStartTag:
		switch p.tok.Data {
		case "html":
			return inBodyIM(p)
		case "noframes":
			return inHeadIM(p)
		}
	case TokenEndTag:
		switch p.tok.Data {
		case "html":
			p.im = afterAfterFramesetIM
			return true
		}
	default:
	}
	return true
}

func afterAfterBodyIM(p *treeBuilder) bool {
	switch p.tok.Type {
	case TokenEOF:
		return true
	case TokenText:
		s := strings.TrimLeft(p.tok.Data, whitespace)
		if len(s) == 0 {
			return inBodyIM(p)
		}
	case TokenStartTag:
		if p.tok.Data == "html" {
			return inBodyIM(p)
		}
	case tokenComment:
		return true
	case tokenDoctype:
		return inBodyIM(p)
	}
	p.im = inBodyIM
	return false
}

func afterAfterFramesetIM(p *treeBuilder) bool {
	switch p.tok.Type {
	case tokenComment:
	case TokenText:
		s := strings.Map(func(c rune) rune {
			switch c {
			case ' ', '\t', '\n', '\f', '\r':
				return c
			}
			return -1
		}, p.tok.Data)
		if s != "" {
			p.tok.Data = s
			return inBodyIM(p)
		}
	case TokenStartTag:
		switch p.tok.Data {
		case "html":
			return inBodyIM(p)
		case "noframes":
			return inHeadIM(p)
		}
	case tokenDoctype:
		return inBodyIM(p)
	default:
	}
	return true
}

const whitespaceOrNUL = whitespace + "\x00"

func parseForeignContent(p *treeBuilder) bool {
	switch p.tok.Type {
	case TokenText:
		if p.framesetOK {
			p.framesetOK = strings.TrimLeft(p.tok.Data, whitespaceOrNUL) == ""
		}
		p.tok.Data = p.arena.replaceNUL(p.tok.Data, true)
		p.addText(p.tok.Data)
	case tokenComment:
	case TokenStartTag:
		b := breakout[p.tok.Data]
		if p.tok.Data == "font" {
		loop:
			for _, attr := range p.tok.Attr {
				switch attr.Key {
				case "color", "face", "size":
					b = true
					break loop
				}
			}
		}
		if b {
			for i := len(p.oe) - 1; i >= 0; i-- {
				n := p.oe[i]
				if n.namespace == "" || htmlIntegrationPoint(n) || mathMLTextIntegrationPoint(n) {
					p.oe = p.oe[:i+1]
					break
				}
			}
			return p.im(p)
		}
		current := p.adjustedCurrentNode()
		switch current.namespace {
		case "math":
			adjustAttributeNames(p.tok.Attr, mathMLAttributeAdjustments)
		case "svg":
			if x := svgTagNameAdjustments[p.tok.Data]; x != "" {
				p.tok.Data = x
			}
			adjustAttributeNames(p.tok.Attr, svgAttributeAdjustments)
		default:
			panic("html: bad treeBuilder state: unexpected namespace")
		}

		namespace := current.namespace
		p.addElement()
		p.top().namespace = namespace
		if namespace != "" {
			p.tokenizer.NextIsNotRawText()
		}
		if p.hasSelfClosingToken {
			p.oe.pop()
			p.acknowledgeSelfClosingTag()
		}
	case TokenEndTag:
		if strings.EqualFold(p.oe[len(p.oe)-1].Data, p.tok.Data) {
			p.oe = p.oe[:len(p.oe)-1]
			return true
		}
		for i := len(p.oe) - 1; i >= 0; i-- {
			if strings.EqualFold(p.oe[i].Data, p.tok.Data) {
				p.oe = p.oe[:i]
				return true
			}
			if i > 0 && p.oe[i-1].namespace == "" {
				break
			}
		}
		return p.im(p)
	default:
	}
	return true
}

func (p *treeBuilder) adjustedCurrentNode() *Node {
	if len(p.oe) == 1 && p.fragment && p.context != nil {
		return p.context
	}
	return p.oe.top()
}

func (p *treeBuilder) inForeignContent() bool {
	if len(p.oe) == 0 {
		return false
	}
	n := p.adjustedCurrentNode()
	if n.namespace == "" {
		return false
	}
	if mathMLTextIntegrationPoint(n) {
		if p.tok.Type == TokenStartTag && p.tok.Data != "mglyph" && p.tok.Data != "malignmark" {
			return false
		}
		if p.tok.Type == TokenText {
			return false
		}
	}
	if n.namespace == "math" && n.Data == "annotation-xml" && p.tok.Type == TokenStartTag && p.tok.Data == "svg" {
		return false
	}
	if htmlIntegrationPoint(n) && (p.tok.Type == TokenStartTag || p.tok.Type == TokenText) {
		return false
	}
	if p.tok.Type == TokenEOF {
		return false
	}
	return true
}

func (p *treeBuilder) parseImpliedToken(t TokenType, data string) {
	realToken, selfClosing := p.tok, p.hasSelfClosingToken
	p.tok = htmlToken{
		Type: t,
		Data: data,
	}
	p.hasSelfClosingToken = false
	p.parseCurrentToken()
	p.tok, p.hasSelfClosingToken = realToken, selfClosing
}

func (p *treeBuilder) parseCurrentToken() {
	if p.tok.Type == TokenSelfClosingTag {
		p.hasSelfClosingToken = true
		p.tok.Type = TokenStartTag
	}

	consumed := false
	for !consumed {
		if p.inForeignContent() {
			consumed = parseForeignContent(p)
		} else {
			consumed = p.im(p)
		}
	}

	if p.hasSelfClosingToken {
		p.hasSelfClosingToken = false
	}
}
