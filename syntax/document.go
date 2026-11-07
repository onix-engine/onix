package syntax

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/diiyw/onix/text/piecetree"

	textmate "github.com/eugenioenko/textmate-go"
)

// Definitions maps lexical token categories to TextMate scopes. The longest
// capture prefix wins, so function.method.call can inherit function.method.
type Definitions struct {
	Captures     map[string]string `json:"captures"`
	SourceScopes map[string]string `json:"sourceScopes"`
}

func ParseDefinitions(data []byte) (*Definitions, error) {
	var definitions Definitions
	if err := decodeJSON(data, &definitions); err != nil {
		return nil, err
	}
	return &definitions, nil
}

var defaultDefinitions = sync.OnceValue(func() *Definitions {
	data, err := resources.ReadFile("resources/highlights.json")
	if err != nil {
		panic(err)
	}
	definitions, err := ParseDefinitions(data)
	if err != nil {
		panic(err)
	}
	return definitions
})

func DefaultDefinitions() *Definitions { return defaultDefinitions() }

func (d *Definitions) WithOverrides(overrides *Definitions) *Definitions {
	result := &Definitions{Captures: maps.Clone(d.Captures), SourceScopes: maps.Clone(d.SourceScopes)}
	maps.Copy(result.Captures, overrides.Captures)
	maps.Copy(result.SourceScopes, overrides.SourceScopes)
	return result
}

func (d *Definitions) Scope(capture string) string {
	for {
		if scope, ok := d.Captures[capture]; ok {
			return scope
		}
		end := strings.LastIndexByte(capture, '.')
		if end < 0 {
			return ""
		}
		capture = capture[:end]
	}
}

type Token struct {
	Capture    string
	Scopes     string // Space-separated TextMate scopes, outermost first.
	CSS        string
	Start, End int
	Style      Style
}

type tokenKey struct {
	capture, scopes string
}

type tokenStyle struct {
	css   string
	style Style
}

// Capture is a render-neutral UTF-8 byte range. Scopes preserves the full
// TextMate scope stack; Capture is its coarse category for editor behavior.
type Capture struct {
	Capture, Scopes    string
	StartByte, EndByte uint32
}

type lexicalLine struct {
	endState          *textmate.StateStack
	start, tokenStart int
}

// Document caches each line's ending state and tokens. Updates tokenize from
// the first changed line until the unchanged suffix has the same start state.
// Its buffers are reused; a Document belongs to one worker.
type Document struct {
	grammar            *textmate.Grammar
	scopes             map[*textmate.ScopeStack]tokenKey
	filename, language string
	source             piecetree.Snapshot
	painter            Painter
	lineScratch        []lexicalLine
	ranges, scratch    []Capture
	lines              []lexicalLine
	tokens             []Token
	retokenized        int
	initialized        bool
	styled             bool
}

func (d *Document) Language() string { return d.language }
func (d *Document) Close()           { *d = Document{} }

// Update detects file extensions or language IDs. Unknown languages stay plain.
func (d *Document) Update[V piecetree.Value](filename string, value V) error {
	source := piecetree.Text(value)
	if d.initialized && d.filename == filename && d.source == source {
		d.retokenized = 0
		return nil
	}
	start, oldEnd, newEnd := d.source.ChangedRange(source)
	return d.UpdateRange(filename, source, start, oldEnd, newEnd)
}

// UpdateSnapshot is the worker interface for immutable document text.
func (d *Document) UpdateSnapshot(filename string, source piecetree.Snapshot) error {
	return d.Update(filename, source)
}

// UpdateRange accepts a known UTF-8 edit. Bytes outside the edit must be equal.
func (d *Document) UpdateRange[V piecetree.Value](filename string, value V, start, oldEnd, newEnd int) error {
	source := piecetree.Text(value)
	if start < 0 || oldEnd < start || oldEnd > d.source.Len() || newEnd < start || newEnd > source.Len() || start > source.Len() ||
		!snapshotBoundary(d.source, start) || !snapshotBoundary(d.source, oldEnd) || !snapshotBoundary(source, start) || !snapshotBoundary(source, newEnd) {
		return fmt.Errorf("invalid syntax edit range")
	}
	if d.initialized && d.filename == filename && d.source == source {
		d.retokenized = 0
		return nil
	}
	if !d.initialized || d.filename != filename {
		language, scope := detectLanguage(filename)
		if d.grammar == nil || language != d.language {
			grammar, err := loadGrammar(scope)
			if err != nil {
				return err
			}
			d.grammar = grammar
			clear(d.scopes)
		}
		d.language, d.filename = language, filename
		clear(d.lines)
		clear(d.ranges)
		d.lines, d.ranges = d.lines[:0], d.ranges[:0]
		start, oldEnd, newEnd = 0, d.source.Len(), source.Len()
	}
	d.retokenized = 0
	d.styled = false
	if d.grammar != nil {
		d.tokenize(source, start, oldEnd, newEnd)
	} else {
		clear(d.lines)
		clear(d.ranges)
		d.lines, d.ranges = d.lines[:0], d.ranges[:0]
	}
	d.ranges = trimBuffer(d.ranges, len(d.ranges))
	d.scratch = trimBuffer(d.scratch, len(d.ranges))
	d.lines = trimBuffer(d.lines, len(d.lines))
	d.lineScratch = trimBuffer(d.lineScratch, len(d.lines))
	clear(d.tokens)
	d.tokens = trimBuffer(d.tokens[:0], len(d.ranges))
	clear(d.painter.tokens)
	d.painter.tokens = trimBuffer(d.painter.tokens[:0], len(d.ranges))
	d.source, d.initialized = source, true
	return nil
}

func (d *Document) tokenize(source piecetree.Snapshot, start, oldEnd, newEnd int) {
	first := 0
	if len(d.lines) != 0 {
		first, _ = slices.BinarySearchFunc(d.lines, start, func(line lexicalLine, offset int) int { return line.start - offset })
		if first == len(d.lines) || d.lines[first].start > start {
			first--
		}
	}
	pos, prefix := 0, 0
	var state *textmate.StateStack
	if len(d.lines) != 0 {
		pos, prefix = d.lines[first].start, d.lines[first].tokenStart
	}
	if first > 0 {
		state = d.lines[first-1].endState
	}
	ranges := append(d.scratch[:0], d.ranges[:prefix]...)
	lines := append(d.lineScratch[:0], d.lines[:first]...)
	shift := source.Len() - d.source.Len()
	for pos <= source.Len() {
		// The suffix text is unchanged by contract. Matching its incoming lexical
		// state proves that all its cached tokens and ending states remain valid.
		if pos >= newEnd && pos-shift >= oldEnd && len(lines) > first {
			old, found := slices.BinarySearchFunc(d.lines, pos-shift, func(line lexicalLine, offset int) int { return line.start - offset })
			if found && old > 0 && state.Equal(d.lines[old-1].endState) {
				tokenShift := len(ranges) - d.lines[old].tokenStart
				for _, capture := range d.ranges[d.lines[old].tokenStart:] {
					capture.StartByte = uint32(int(capture.StartByte) + shift)
					capture.EndByte = uint32(int(capture.EndByte) + shift)
					ranges = append(ranges, capture)
				}
				for _, line := range d.lines[old:] {
					line.start += shift
					line.tokenStart += tokenShift
					lines = append(lines, line)
				}
				break
			}
		}
		end := source.IndexByte(pos, '\n')
		if end < 0 {
			end = source.Len()
		}
		// The tokenizer's long-line fallback only needs the incoming state and
		// a length beyond its budget. Bound copies even for a fragmented,
		// document-sized generated line, then extend its fallback token below.
		const lineBudget = 20_000
		text := source.Slice(pos, min(end, pos+lineBudget+1))
		tokenStart := len(ranges)
		result := d.grammar.TokenizeLineWithOptions(text, state, textmate.TokenizeOptions{MaxLineBytes: lineBudget})
		d.retokenized++
		// Convert monotonically increasing rune offsets in one pass. The editor and
		// wire protocol use bytes, including for CJK and astral Unicode characters.
		runePos, bytePos := 0, 0
		toByte := func(target int) int {
			for runePos < target && bytePos < len(text) {
				_, size := utf8.DecodeRuneInString(text[bytePos:])
				bytePos += size
				runePos++
			}
			return bytePos
		}
		for _, token := range result.Tokens {
			begin, finish := toByte(token.Start), toByte(token.End)
			if end-pos > lineBudget {
				begin, finish = 0, end-pos
			}
			if begin == finish {
				continue
			}
			if d.scopes == nil {
				d.scopes = make(map[*textmate.ScopeStack]tokenKey)
			}
			key, ok := d.scopes[token.ScopeStack]
			if !ok {
				key = tokenKey{capture: lexicalCategory(token), scopes: strings.Join(token.Scopes, " ")}
				d.scopes[token.ScopeStack] = key
			}
			ranges = append(ranges, Capture{StartByte: uint32(pos + begin), EndByte: uint32(pos + finish), Capture: key.capture, Scopes: key.scopes})
		}
		state = result.RuleStack
		lines = append(lines, lexicalLine{start: pos, tokenStart: tokenStart, endState: state})
		if end == source.Len() {
			break
		}
		pos = end + 1
	}
	oldRanges, oldLines := d.ranges, d.lines
	d.ranges, d.lines = ranges, lines
	clear(oldRanges)
	clear(oldLines)
	d.scratch, d.lineScratch = oldRanges[:0], oldLines[:0]
}

func lexicalCategory(token textmate.Token) string {
	// Bracket matching must ignore comment/string delimiters as well as bodies.
	ignored := ""
	for _, scope := range token.Scopes {
		switch {
		case scopePrefix(scope, "meta.embedded"), scopePrefix(scope, "meta.template.expression"):
			ignored = ""
		case scopePrefix(scope, "comment"):
			ignored = "comment"
		case scopePrefix(scope, "string.regexp"):
			ignored = "string.regexp"
		case scopePrefix(scope, "string"), scopePrefix(scope, "constant.character"), scopePrefix(scope, "markup.raw"), scopePrefix(scope, "markup.inline.raw"):
			ignored = "string"
		}
	}
	if ignored != "" {
		return ignored
	}
	category := token.Category()
	if category == textmate.TokenCategoryRegexp {
		return "string.regexp"
	}
	if category == textmate.TokenCategoryHeading {
		return "markup.heading"
	}
	if category == textmate.TokenCategoryBold {
		return "markup.bold"
	}
	if category == textmate.TokenCategoryItalic {
		return "markup.italic"
	}
	if category == textmate.TokenCategoryInvalid {
		return "error"
	}
	return category.String()
}

func scopePrefix(scope, prefix string) bool {
	return scope == prefix || strings.HasPrefix(scope, prefix+".")
}

// Captures borrows the current ranges until the next update or Close.
func (d *Document) Captures() []Capture { return d.ranges }

// Tokens recolors cached scopes. Neither theme changes nor identical updates
// invoke the tokenizer. The unchanged source/theme path allocates no memory.
func (d *Document) Tokens(theme *Theme, definitions *Definitions) []Token {
	if definitions == nil {
		definitions = DefaultDefinitions()
	}
	if d.styled && d.painter.theme == theme && d.painter.definitions == definitions {
		return d.painter.tokens
	}
	if !d.styled {
		clear(d.tokens)
		d.tokens = d.tokens[:0]
		for _, c := range d.ranges {
			d.tokens = append(d.tokens, Token{Start: int(c.StartByte), End: int(c.EndByte), Capture: c.Capture, Scopes: c.Scopes})
		}
	}
	result := d.painter.Tokens(d.language, d.tokens, theme, definitions)
	d.styled = true
	return result
}

// Painter resolves lexical scopes locally, independent of the tokenizer.
type Painter struct {
	language    string
	theme       *Theme
	definitions *Definitions
	palette     map[tokenKey]tokenStyle
	tokens      []Token
}

func (p *Painter) Tokens(language string, captures []Token, theme *Theme, definitions *Definitions) []Token {
	if definitions == nil {
		definitions = DefaultDefinitions()
	}
	if p.language != language || p.theme != theme || p.definitions != definitions {
		clear(p.palette)
		p.language, p.theme, p.definitions = language, theme, definitions
	}
	if p.palette == nil {
		p.palette = make(map[tokenKey]tokenStyle)
	}
	previous := len(p.tokens)
	p.tokens = p.tokens[:0]
	for _, capture := range captures {
		key := tokenKey{capture.Capture, capture.Scopes}
		style, ok := p.palette[key]
		if !ok {
			scopes := strings.Fields(capture.Scopes)
			if root := definitions.SourceScopes[language]; root != "" && len(scopes) > 0 {
				scopes[0] = root
			}
			// Explicit user category overrides remain useful with native TextMate scopes.
			scope := definitions.Scope(capture.Capture)
			if scope != "" && (len(scopes) == 0 || scope != DefaultDefinitions().Scope(capture.Capture)) {
				root := definitions.SourceScopes[language]
				if root == "" {
					root = "source." + language
				}
				if len(scopes) == 0 {
					scopes = append(scopes, root)
				}
				scopes = append(scopes, scope+"."+strings.TrimPrefix(root, "source."))
			}
			style.style = theme.Resolve(scopes...)
			style.css = style.style.CSS()
			p.palette[key] = style
		}
		capture.Style, capture.CSS = style.style, style.css
		p.tokens = append(p.tokens, capture)
	}
	if len(p.tokens) < previous {
		clear(p.tokens[len(p.tokens):previous])
	}
	p.tokens = trimBuffer(p.tokens, len(captures))
	return p.tokens
}

// Keep warm storage, dropping capacity only after a substantial reduction.
func trimBuffer[T any](value []T, live int) []T {
	if cap(value) > max(4096, live*4) {
		return append([]T(nil), value...)
	}
	return value
}

func snapshotBoundary(source piecetree.Snapshot, offset int) bool {
	return offset == source.Len() || utf8.RuneStart(source.ByteAt(offset))
}
