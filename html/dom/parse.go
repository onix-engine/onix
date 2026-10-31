package dom

import (
	"errors"
	"math/bits"
	"strings"
	"unsafe"
)

// Parser builds DOM nodes directly from UTF-8 source. Its zero value is ready
// to use. Nodes, attributes, child slices and decoded text are reused on the
// next Parse or Reset call, which invalidates the previous result. A Parser
// must not be copied after use or used concurrently.
//
// Unchanged text and names borrow the source string. Decoded strings borrow
// parser storage; they are immutable until the next Parse or Reset call.
type Parser struct {
	builder treeBuilder
	scanner htmlScanner
	blocks  [][]parsedNode
	used    int
	decoded []byte
	attrs   []htmlAttribute
	names   map[string]struct{}
}

type parsedNode struct {
	node Node
	text []byte // Only used when adjacent text tokens must be joined.
}

type htmlAttribute struct{ Key, Val string }
type htmlToken struct {
	Type TokenType
	Data string
	Attr []htmlAttribute
}

// ErrNestingDepth reports HTML nesting beyond the parser's 512-element limit.
var ErrNestingDepth = errors.New("dom: maximum element depth (512) exceeded")

// Parse returns an independently owned DOM. Use a retained Parser when the
// previous DOM can be discarded to avoid allocations after warming its storage.
// Malformed HTML is repaired using the HTML tree construction rules. As before,
// sources without a leading doctype/html/head/body are parsed as body fragments.
func Parse(input string) (*Node, error) {
	var parser Parser
	return parser.Parse(input)
}

// Reset invalidates the last DOM and releases source references while retaining
// reusable capacity. Assign Parser{} to release the retained capacity as well.
func (p *Parser) Reset() {
	remaining := p.used
	for _, block := range p.blocks {
		count := min(remaining, len(block))
		for i := range count {
			slot := &block[i]
			n := &slot.node
			clear(n.Attributes)
			clear(n.Children)
			*n = Node{Attributes: n.Attributes, Children: n.Children[:0]}
			slot.text = slot.text[:0]
		}
		remaining -= count
	}
	p.used = 0
	p.decoded = p.decoded[:0]
	clear(p.attrs)
	p.attrs = p.attrs[:0]
	clear(p.names)
	p.scanner.reset("")
	b := &p.builder
	clear(b.oe[:cap(b.oe)])
	clear(b.afe[:cap(b.afe)])
	*b = treeBuilder{oe: b.oe[:0], afe: b.afe[:0], templateStack: b.templateStack[:0]}
}

// Parse replaces the previous result. Once capacity is sufficient, parsing
// reuses all storage, including entity decoding and malformed-tree repairs.
func (p *Parser) Parse(input string) (root *Node, err error) {
	p.Reset()
	p.scanner.reset(input)
	b := &p.builder
	b.arena, b.tokenizer = p, &p.scanner
	b.doc = p.newNode(DocumentNode, "#document")
	b.scripting, b.framesetOK = true, true
	b.im = initialIM
	// Body fragments use the document node as the synthetic html root, so no
	// wrapper nodes, child copies or second DOM traversal are needed.
	defer func() {
		if failure := recover(); failure != nil {
			if failure != ErrNestingDepth {
				panic(failure)
			}
			root, err = nil, ErrNestingDepth
		}
	}()
	for first := true; ; first = false {
		current := b.oe.top()
		p.scanner.allowCDATA = current != nil && current.namespace != ""
		kind := p.scanner.scan()
		if first {
			// Classify from the first significant token and reuse that token.
			for kind == tokenComment || kind == TokenText && p.leadingSpace() {
				kind = p.scanner.scan()
			}
			if !documentToken(&p.scanner, kind) {
				b.fragment, b.context = true, &bodyContext
				b.doc.Type, b.doc.Data = ElementNode, "html"
				b.oe = append(b.oe, b.doc)
				b.im = inBodyIM
				// Replay only the skipped prefix, never the significant token.
				if end := p.scanner.raw.start; end > 0 {
					saved := p.scanner
					p.scanner.reset(input[:end])
					for prefix := p.scanner.scan(); prefix != TokenEOF; prefix = p.scanner.scan() {
						if prefix == TokenText {
							b.addText(p.token(prefix).Data)
						}
					}
					p.scanner = saved
				}
			}
		}
		b.tok = p.token(kind)
		if kind != tokenComment {
			b.parseCurrentToken()
		}
		b.ignoreLF = (kind == TokenStartTag || kind == TokenSelfClosingTag) &&
			(b.tok.Data == "pre" || b.tok.Data == "listing" || b.tok.Data == "textarea")
		if kind == TokenEOF {
			break
		}
	}
	b.doc.Type, b.doc.Data = DocumentNode, "#document"
	InitializeElementStates(b.doc)
	return b.doc, nil
}

var bodyContext = Node{Type: ElementNode, Data: "body"}

func (p *Parser) newNode(kind NodeType, data string) *Node {
	index := p.used
	block := bits.Len(uint(index/4+1)) - 1
	if block == len(p.blocks) {
		p.blocks = append(p.blocks, make([]parsedNode, 4<<block))
	}
	n := &p.blocks[block][index-4*((1<<block)-1)].node
	n.Type, n.Data, n.Order = kind, data, uint32(index)
	p.used++
	return n
}

func (p *Parser) cloneNode(source *Node) *Node {
	n := p.newNode(source.Type, source.Data)
	n.namespace = source.namespace
	if len(source.Attributes) > 0 && n.Attributes == nil {
		n.Attributes = make(map[string]string, len(source.Attributes))
	}
	for key, value := range source.Attributes {
		n.Attributes[key] = value
	}
	return n
}

func (p *Parser) appendText(n *Node, text string) {
	index := int(n.Order)
	block := bits.Len(uint(index/4+1)) - 1
	slot := &p.blocks[block][index-4*((1<<block)-1)]
	if len(slot.text) == 0 {
		slot.text = append(slot.text, n.Data...)
	}
	slot.text = append(slot.text, text...)
	n.Data = byteString(slot.text)
}

// byteString exposes an immutable view into append-only parser storage. Growing
// the buffer leaves previous strings valid; overwriting only occurs on Reset.
func byteString(value []byte) string {
	return unsafe.String(unsafe.SliceData(value), len(value))
}

func copyTokenAttributes(dst *Node, src htmlToken) {
	if len(src.Attr) > 0 && dst.Attributes == nil {
		dst.Attributes = make(map[string]string, len(src.Attr))
	}
	for _, attr := range src.Attr {
		if _, exists := dst.Attributes[attr.Key]; !exists {
			dst.Attributes[attr.Key] = attr.Val
		}
	}
}

func (p *Parser) token(kind TokenType) htmlToken {
	z := &p.scanner
	token := htmlToken{Type: kind}
	switch kind {
	case TokenText:
		token.Data = p.decode(z.input[z.data.start:z.data.end], false, !z.textIsRaw, z.convertNUL, false)
	case tokenDoctype:
		token.Data = p.decode(z.input[z.data.start:z.data.end], false, false, true, false)
	case TokenStartTag, TokenEndTag, TokenSelfClosingTag:
		token.Data = p.lower(z.input[z.data.start:z.data.end])
		clear(p.attrs)
		p.attrs = p.attrs[:0]
		clear(p.names)
		for _, attr := range z.attr {
			key := p.lower(z.input[attr[0].start:attr[0].end])
			if _, duplicate := p.names[key]; duplicate {
				continue
			}
			if p.names == nil {
				p.names = make(map[string]struct{})
			}
			p.names[key] = struct{}{}
			value := p.decode(z.input[attr[1].start:attr[1].end], true, true, true, false)
			p.attrs = append(p.attrs, htmlAttribute{key, value})
		}
		token.Attr = p.attrs
	}
	return token
}

func documentToken(z *htmlScanner, kind TokenType) bool {
	return kind == tokenDoctype || (kind == TokenStartTag || kind == TokenSelfClosingTag) && z.startTagIn("html", "head", "body")
}

func (p *Parser) leadingSpace() bool {
	// Only decode a small entity/whitespace prefix. A visible first character
	// rules out a document without copying or scanning the rest of a long token.
	s := p.scanner.input[p.scanner.data.start:p.scanner.data.end]
	for len(s) > 0 {
		if s[0] == '&' {
			r1, r2, size := consumeReference(s, false)
			if size == 1 || !isDetectionSpace(r1) || r2 != 0 && !isDetectionSpace(r2) {
				return false
			}
			s = s[size:]
			continue
		}
		r, size := detectionRune(s)
		if !isDetectionSpace(r) {
			return false
		}
		s = s[size:]
	}
	return true
}

func isHTMLDocument(input string) bool {
	var p Parser
	p.scanner.reset(input)
	for {
		kind := p.scanner.scan()
		if kind == tokenComment || kind == TokenText && p.leadingSpace() {
			continue
		}
		return documentToken(&p.scanner, kind)
	}
}

// replaceNUL is used by insertion modes: HTML data drops NUL, foreign data
// replaces it. Ordinary strings remain source views.
func (p *Parser) replaceNUL(s string, replacement bool) string {
	if strings.IndexByte(s, 0) < 0 {
		return s
	}
	start := len(p.decoded)
	for i := range len(s) {
		if s[i] != 0 {
			p.decoded = append(p.decoded, s[i])
		} else if replacement {
			p.decoded = append(p.decoded, "\ufffd"...)
		}
	}
	return byteString(p.decoded[start:])
}
