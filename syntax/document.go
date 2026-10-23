package syntax

import (
	"fmt"
	"maps"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Definitions maps tree-sitter query captures to TextMate scopes. The longest
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
	Start, End int
	Capture    string
	Style      Style
	CSS        string
}

type tokenStyle struct {
	style Style
	css   string
}

// Document owns one syntax tree. Update and Tokens reuse cached results when
// the source and theme are unchanged. It must not be used concurrently.
type Document struct {
	filename    string
	source      string
	initialized bool
	language    string
	rootScope   string
	highlighter *gotreesitter.Highlighter
	tree        *gotreesitter.Tree
	ranges      []gotreesitter.HighlightRange
	theme       *Theme
	definitions *Definitions
	palette     map[string]tokenStyle
	tokens      []Token
	styled      bool
}

func (d *Document) Tree() *gotreesitter.Tree { return d.tree }
func (d *Document) Language() string         { return d.language }

// Close releases retained parser arenas; the document can subsequently be reused.
func (d *Document) Close() {
	if d.tree != nil {
		d.tree.Release()
	}
	*d = Document{}
}

// Release closes a worker-owned tree and optionally trims idle parser arenas.
func (d *Document) Release(trim bool) {
	d.Close()
	if trim {
		ReleaseParserMemory()
	}
}

// ReleaseParserMemory drops idle tree arenas held by the parser's global pools.
// Live documents remain valid. Call at an idle boundary, not on every edit.
func ReleaseParserMemory() { gotreesitter.DrainArenaPools() }

// Update detects a filename or language name, parses it, and retains captures.
// Unknown languages are plain text. Syntax errors still yield a partial tree.
func (d *Document) Update(filename, source string) error {
	return d.update(filename, source, nil)
}

// UpdateRange consumes an already known UTF-8 edit and avoids rediscovering it.
// Offsets refer to the previous source and the new source, respectively.
func (d *Document) UpdateRange(filename, source string, start, oldEnd, newEnd int) error {
	if start < 0 || oldEnd < start || oldEnd > len(d.source) || newEnd < start || newEnd > len(source) {
		return fmt.Errorf("invalid syntax edit range")
	}
	point := advancePoint(gotreesitter.Point{}, d.source[:start])
	edit := gotreesitter.InputEdit{StartByte: uint32(start), OldEndByte: uint32(oldEnd), NewEndByte: uint32(newEnd), StartPoint: point, OldEndPoint: advancePoint(point, d.source[start:oldEnd]), NewEndPoint: advancePoint(point, source[start:newEnd])}
	return d.update(filename, source, &edit)
}

func (d *Document) update(filename, source string, edit *gotreesitter.InputEdit) error {
	if d.initialized && d.filename == filename && d.source == source {
		return nil
	}
	if !d.initialized || d.filename != filename {
		entry := grammars.DetectLanguage(filename)
		if entry == nil {
			entry = grammars.DetectLanguageByName(filename)
		}
		// Vim help shares .txt with plain text; require an explicit vimdoc language.
		// Keep named files such as CMakeLists.txt associated with their grammar.
		if entry != nil && entry.Name == "vimdoc" && strings.EqualFold(filepath.Ext(filename), ".txt") {
			entry = nil
		}
		languageName := ""
		if entry != nil {
			languageName = entry.Name
		}
		if d.tree != nil {
			d.tree.Release()
			d.tree = nil
		}
		d.source, d.ranges, d.initialized, d.styled = "", nil, false, false
		if d.language != languageName {
			d.highlighter = nil
			d.language = languageName
			d.definitions = nil // Language scopes also determine the palette.
			clear(d.palette)
		}
		d.filename = filename
		if entry != nil && d.highlighter == nil {
			d.language = entry.Name
			language := entry.Language()
			if language == nil {
				return fmt.Errorf("syntax grammar %q is unavailable", entry.Name)
			}
			var options []gotreesitter.HighlighterOption
			if entry.TokenSourceFactory != nil {
				options = append(options, gotreesitter.WithTokenSourceFactory(func(source []byte) gotreesitter.TokenSource {
					return entry.TokenSourceFactory(source, language)
				}))
			}
			query, err := highlightQuery(entry.HighlightQuery, language)
			if err == nil {
				d.highlighter, err = gotreesitter.NewHighlighter(language, query, options...)
			}
			if err != nil {
				return fmt.Errorf("syntax query %s: %w", entry.Name, err)
			}
		}
	}
	d.styled = false
	if d.highlighter != nil {
		oldTree := d.tree
		if oldTree != nil {
			if edit == nil {
				value := changedRange(d.source, source)
				edit = &value
			}
			oldTree.Edit(*edit)
		}
		// A wall-clock deadline can turn an ordinary file into an empty or
		// partial highlight under load. Let parsing finish, and never cache a
		// tree stopped by the parser's safety limits as a successful update.
		var err error
		d.ranges, d.tree, err = d.highlighter.HighlightIncrementalStrict([]byte(source), oldTree)
		if oldTree != nil {
			oldTree.Release()
		}
		if err != nil {
			d.Close()
			return fmt.Errorf("syntax parse %s: %w", filename, err)
		}
	}
	d.source, d.initialized = source, true
	return nil
}

func changedRange(before, after string) gotreesitter.InputEdit {
	prefix := 0
	point := gotreesitter.Point{}
	for prefix < len(before) && prefix < len(after) {
		oldRune, oldSize := utf8.DecodeRuneInString(before[prefix:])
		newRune, newSize := utf8.DecodeRuneInString(after[prefix:])
		if oldRune != newRune || oldSize != newSize {
			break
		}
		if oldRune == '\n' {
			point.Row++
			point.Column = 0
		} else {
			point.Column += uint32(oldSize)
		}
		prefix += oldSize
	}
	oldEnd, newEnd := len(before), len(after)
	for oldEnd > prefix && newEnd > prefix {
		oldRune, oldSize := utf8.DecodeLastRuneInString(before[:oldEnd])
		newRune, newSize := utf8.DecodeLastRuneInString(after[:newEnd])
		if oldRune != newRune || oldSize != newSize {
			break
		}
		oldEnd -= oldSize
		newEnd -= newSize
	}
	return gotreesitter.InputEdit{
		StartByte: uint32(prefix), OldEndByte: uint32(oldEnd), NewEndByte: uint32(newEnd),
		StartPoint: point, OldEndPoint: advancePoint(point, before[prefix:oldEnd]), NewEndPoint: advancePoint(point, after[prefix:newEnd]),
	}
}

func advancePoint(point gotreesitter.Point, value string) gotreesitter.Point {
	if last := strings.LastIndexByte(value, '\n'); last >= 0 {
		point.Row += uint32(strings.Count(value, "\n"))
		point.Column = uint32(len(value) - last - 1)
	} else {
		point.Column += uint32(len(value))
	}

	return point
}

// Tokens returns sorted, non-overlapping byte ranges. Switching the theme or
// definitions recolors cached captures without parsing or executing queries.
func (d *Document) Tokens(theme *Theme, definitions *Definitions) []Token {
	if definitions == nil {
		definitions = DefaultDefinitions()
	}
	if d.theme != theme || d.definitions != definitions {
		d.theme, d.definitions, d.styled = theme, definitions, false
		clear(d.palette)
		d.rootScope = definitions.SourceScopes[d.language]
		if d.rootScope == "" {
			d.rootScope = "source." + d.language
		}
	}
	if d.styled {
		return d.tokens
	}
	if d.palette == nil {
		d.palette = make(map[string]tokenStyle)
	}
	d.tokens = d.tokens[:0]
	for _, capture := range d.ranges {
		start, end := int(capture.StartByte), int(capture.EndByte)
		if start >= end || end > len(d.source) {
			continue
		}
		style, ok := d.palette[capture.Capture]
		if !ok {
			scope := definitions.Scope(capture.Capture)
			if scope == "" {
				continue
			}
			// The language suffix permits conventional language-specific rules.
			suffix := d.language
			if strings.HasPrefix(d.rootScope, "source.") {
				suffix = strings.TrimPrefix(d.rootScope, "source.")
			}
			style.style = theme.Resolve(d.rootScope, scope+"."+suffix)
			style.css = style.style.CSS()
			d.palette[capture.Capture] = style
		}
		d.tokens = append(d.tokens, Token{start, end, capture.Capture, style.style, style.css})
	}
	d.styled = true
	return d.tokens
}

// Captures exposes render-neutral byte ranges, borrowed until Update or Close.
func (d *Document) Captures() []gotreesitter.HighlightRange { return d.ranges }

// Painter resolves remote captures using the GUI's current theme. It holds no
// parser, tree, or source text and reuses its palette and output storage.
type Painter struct {
	language    string
	theme       *Theme
	definitions *Definitions
	palette     map[string]tokenStyle
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
		p.palette = make(map[string]tokenStyle)
	}
	root := definitions.SourceScopes[language]
	if root == "" {
		root = "source." + language
	}
	suffix := strings.TrimPrefix(root, "source.")
	p.tokens = p.tokens[:0]
	for _, capture := range captures {
		style, ok := p.palette[capture.Capture]
		if !ok {
			scope := definitions.Scope(capture.Capture)
			if scope == "" {
				continue
			}
			style.style = theme.Resolve(root, scope+"."+suffix)
			style.css = style.style.CSS()
			p.palette[capture.Capture] = style
		}
		capture.Style, capture.CSS = style.style, style.css
		p.tokens = append(p.tokens, capture)
	}
	return p.tokens
}
