package parse

import (
	"errors"
	"strings"
	"unsafe"

	"github.com/onix-engine/onix/html/element"
	uinode "github.com/onix-engine/onix/scene/node"
)

// Parser builds scene nodes directly from UTF-8 source. Its zero value is ready
// to use. Nodes, attributes, child slices and decoded text are reused on the
// next Parse or Reset call, which invalidates the previous result. A Parser
// must not be copied after use or used concurrently.
//
// Unchanged text and names borrow the source string. Decoded strings borrow
// parser storage; they are immutable until the next Parse or Reset call.
type Parser struct {
	builder   treeBuilder
	names     map[string]struct{}
	joinIndex map[*uinode.Node]int
	nodes     uinode.Arena
	joined    [][]byte
	decoded   []byte
	attrs     []htmlAttribute
	scanner   htmlScanner
}

type htmlAttribute = uinode.Attribute
type htmlToken struct {
	Data string
	Attr []htmlAttribute
	Type TokenType
}

// ErrNestingDepth reports HTML nesting beyond the parser's 512-element limit.
var ErrNestingDepth = errors.New("html: maximum element depth (512) exceeded")

// Parse returns an independently owned logical tree. Use a retained Parser when the
// previous tree can be discarded to avoid allocations after warming its storage.
// Malformed HTML is repaired using the HTML tree construction rules. As before,
// sources without a leading doctype/html/head/body are parsed as body fragments.
func Parse(input string) (*uinode.Node, error) {
	var parser Parser
	return parser.Parse(input)
}

// ParseTree constructs an independently owned tree for callers that initialize
// document order and form state as part of their own document preparation.
func ParseTree(input string) (*uinode.Node, error) {
	var parser Parser
	return parser.parseTree(input)
}

// Reset invalidates the previous tree and releases source references while retaining
// reusable capacity. Assign Parser{} to release the retained capacity as well.
func (p *Parser) Reset() {
	p.nodes.Reset()
	clear(p.joinIndex)
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
func (p *Parser) Parse(input string) (*uinode.Node, error) {
	root, err := p.parseTree(input)
	if err == nil {
		element.InitializeElementStates(root)
	}
	return root, err
}

func (p *Parser) parseTree(input string) (root *uinode.Node, err error) {
	p.Reset()
	p.scanner.reset(input)
	b := &p.builder
	b.arena, b.tokenizer = p, &p.scanner
	b.doc = p.newNode(uinode.DocumentNode, "#document")
	b.scripting, b.framesetOK = true, true
	b.im = initialIM
	// Body fragments use the document node as the synthetic html root, so no
	// wrapper nodes, child copies or second tree traversal are needed.
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
		p.scanner.allowCDATA = current != nil && current.Namespace != uinode.HTML
		kind := p.scanner.scan()
		if first {
			// Classify from the first significant token and reuse that token.
			for kind == tokenComment || kind == TokenText && p.leadingSpace() {
				kind = p.scanner.scan()
			}
			if !documentToken(&p.scanner, kind) {
				b.fragment, b.context = true, &bodyContext
				b.doc.Type, b.doc.Data = uinode.ElementNode, "html"
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
	b.doc.Type, b.doc.Data = uinode.DocumentNode, "#document"
	return b.doc, nil
}

var bodyContext = uinode.Node{Type: uinode.ElementNode, Data: "body"}

func (p *Parser) newNode(kind uinode.NodeType, data string) *uinode.Node {
	return p.nodes.New(kind, data)
}

func (p *Parser) cloneNode(source *uinode.Node) *uinode.Node {
	n := p.newNode(source.Type, source.Data)
	n.Namespace = source.Namespace
	n.CopyAttributes(source)
	return n
}

func (p *Parser) appendText(n *uinode.Node, text string) {
	index, exists := p.joinIndex[n]
	if !exists {
		if p.joinIndex == nil {
			p.joinIndex = make(map[*uinode.Node]int)
		}
		index = len(p.joinIndex)
		p.joinIndex[n] = index
		if index == len(p.joined) {
			p.joined = append(p.joined, nil)
		}
		p.joined[index] = append(p.joined[index][:0], n.Data...)
	}
	p.joined[index] = append(p.joined[index], text...)
	n.Data = byteString(p.joined[index])
}

// byteString exposes an immutable view into append-only parser storage. Growing
// the buffer leaves previous strings valid; overwriting only occurs on Reset.
func byteString(value []byte) string {
	return unsafe.String(unsafe.SliceData(value), len(value))
}

func copyTokenAttributes(dst *uinode.Node, src htmlToken) {
	if dst.AttrCount() == 0 {
		dst.SetAttributes(src.Attr...)
		return
	}
	for _, attr := range src.Attr {
		if _, exists := dst.Attr(attr.Name); !exists {
			dst.SetAttr(attr.Name, attr.Value)
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
			p.attrs = append(p.attrs, htmlAttribute{Name: key, Value: value})
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
