// Package syntax parses code with gotreesitter and colors its captures with
// VS Code color themes. Documents retain their trees and highlight buffers.
package syntax

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strconv"
	"strings"
	"sync"
)

//go:embed resources
var resources embed.FS

type Color struct{ R, G, B, A uint8 }

func (c Color) Hex() string {
	if c.A != 255 {
		return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A)
	}
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

func parseColor(value string) (Color, error) {
	if len(value) == 0 || value[0] != '#' {
		return Color{}, fmt.Errorf("invalid theme color %q", value)
	}
	digits := value[1:]
	if len(digits) == 3 || len(digits) == 4 {
		var expanded strings.Builder
		for _, digit := range digits {
			expanded.WriteRune(digit)
			expanded.WriteRune(digit)
		}
		digits = expanded.String()
	}
	if len(digits) != 6 && len(digits) != 8 {
		return Color{}, fmt.Errorf("invalid theme color %q", value)
	}
	n, err := strconv.ParseUint(digits, 16, 32)
	if err != nil {
		return Color{}, fmt.Errorf("invalid theme color %q: %w", value, err)
	}
	if len(digits) == 6 {
		n = n<<8 | 255
	}
	return Color{uint8(n >> 24), uint8(n >> 16), uint8(n >> 8), uint8(n)}, nil
}

type FontStyle uint8

const (
	Italic FontStyle = 1 << iota
	Bold
	Underline
	Strikethrough
)

type Style struct {
	Foreground Color
	FontStyle  FontStyle
}

func (s Style) CSS() string {
	var result strings.Builder
	result.WriteString("color:" + s.Foreground.Hex())
	if s.FontStyle&Italic != 0 {
		result.WriteString(";font-style:italic")
	}
	if s.FontStyle&Bold != 0 {
		result.WriteString(";font-weight:bold")
	}
	if s.FontStyle&(Underline|Strikethrough) != 0 {
		result.WriteString(";text-decoration:")
		if s.FontStyle&Underline != 0 {
			result.WriteString("underline ")
		}
		if s.FontStyle&Strikethrough != 0 {
			result.WriteString("line-through")
		}
	}
	return result.String()
}

type themeSettings struct {
	Foreground string  `json:"foreground"`
	FontStyle  *string `json:"fontStyle"`
}

type themeRule struct {
	Scope    json.RawMessage `json:"scope"`
	Settings themeSettings   `json:"settings"`
}

type themeFile struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	Include     string            `json:"include"`
	Colors      map[string]string `json:"colors"`
	TokenColors []themeRule       `json:"tokenColors"`
}

type selector struct {
	parents []string
	leaf    string
	exclude []string
}

type colorRule struct {
	selector selector
	color    Color
	font     FontStyle
	hasColor bool
	hasFont  bool
}

// Theme is immutable after loading and can be shared by multiple documents.
type Theme struct {
	Name         string
	Type         string
	colors       map[string]Color
	rules        []colorRule
	base         Style
	hasBaseColor bool
}

// LoadTheme reads a JSON/JSONC color theme and resolves relative includes.
func LoadTheme(source fs.FS, filename string) (*Theme, error) {
	theme := &Theme{colors: make(map[string]Color), base: Style{Foreground: Color{212, 212, 212, 255}}}
	if err := theme.load(source, path.Clean(filename), make(map[string]bool)); err != nil {
		return nil, err
	}
	if theme.Name == "" {
		theme.Name = strings.TrimSuffix(path.Base(filename), path.Ext(filename))
	}
	if !theme.hasBaseColor && theme.Type == "light" {
		theme.base.Foreground = Color{0, 0, 0, 255}
	}
	return theme, nil
}

func (t *Theme) load(source fs.FS, filename string, visiting map[string]bool) error {
	if visiting[filename] {
		return fmt.Errorf("theme include cycle at %q", filename)
	}
	visiting[filename] = true
	defer delete(visiting, filename)
	data, err := fs.ReadFile(source, filename)
	if err != nil {
		return err
	}
	var file themeFile
	if err := decodeJSON(data, &file); err != nil {
		return fmt.Errorf("theme %s: %w", filename, err)
	}
	if file.Include != "" {
		if err := t.load(source, path.Join(path.Dir(filename), file.Include), visiting); err != nil {
			return err
		}
	}
	if file.Name != "" {
		t.Name = file.Name
	}
	if file.Type != "" {
		t.Type = file.Type
	}
	for key, value := range file.Colors {
		color, err := parseColor(value)
		if err != nil {
			return fmt.Errorf("theme %s, %s: %w", filename, key, err)
		}
		t.colors[key] = color
		if key == "editor.foreground" {
			t.base.Foreground = color
			t.hasBaseColor = true
		}
	}
	for _, rule := range file.TokenColors {
		compiled := colorRule{hasColor: rule.Settings.Foreground != "", hasFont: rule.Settings.FontStyle != nil}
		if compiled.hasColor {
			compiled.color, err = parseColor(rule.Settings.Foreground)
			if err != nil {
				return err
			}
		}
		if compiled.hasFont {
			for field := range strings.FieldsSeq(*rule.Settings.FontStyle) {
				switch field {
				case "italic":
					compiled.font |= Italic
				case "bold":
					compiled.font |= Bold
				case "underline":
					compiled.font |= Underline
				case "strikethrough":
					compiled.font |= Strikethrough
				default:
					return fmt.Errorf("invalid fontStyle %q", field)
				}
			}
		}
		if len(rule.Scope) == 0 || string(rule.Scope) == "null" {
			if compiled.hasColor {
				t.base.Foreground = compiled.color
				t.hasBaseColor = true
			}
			if compiled.hasFont {
				t.base.FontStyle = compiled.font
			}
			continue
		}
		var scopes []string
		if rule.Scope[0] == '"' {
			var scope string
			if err := json.Unmarshal(rule.Scope, &scope); err != nil {
				return err
			}
			scopes = []string{scope}
		} else if err := json.Unmarshal(rule.Scope, &scopes); err != nil {
			return err
		}
		for _, scopes := range scopes {
			for scope := range strings.SplitSeq(scopes, ",") {
				parts := strings.Split(scope, " - ")
				fields := strings.Fields(parts[0])
				if len(fields) == 0 {
					continue
				}
				compiled.selector = selector{parents: fields[:len(fields)-1], leaf: fields[len(fields)-1]}
				for _, excluded := range parts[1:] {
					compiled.selector.exclude = append(compiled.selector.exclude, strings.Fields(excluded)...)
				}
				t.rules = append(t.rules, compiled)
			}
		}
	}
	return nil
}

// LookupColor distinguishes an omitted color from an explicitly transparent one.
func (t *Theme) LookupColor(key string) (Color, bool) {
	color, ok := t.colors[key]
	return color, ok
}

func (t *Theme) Color(key string) Color {
	if color, ok := t.colors[key]; ok {
		return color
	}
	switch key {
	case "editor.background", "editorGutter.background":
		if key == "editorGutter.background" {
			return t.Color("editor.background")
		}
		if t.Type == "light" {
			return Color{255, 255, 255, 255}
		}
		return Color{30, 30, 30, 255}
	default:
		return t.base.Foreground
	}
}

func scopeMatches(selector, scope string) bool {
	return selector == "*" || scope == selector || strings.HasPrefix(scope, selector) && len(scope) > len(selector) && scope[len(selector)] == '.'
}

func (s selector) score(stack []string) uint64 {
	for _, excluded := range s.exclude {
		for _, scope := range stack {
			if scopeMatches(excluded, scope) {
				return 0
			}
		}
	}
	for i := len(stack) - 1; i >= 0; i-- {
		if !scopeMatches(s.leaf, stack[i]) {
			continue
		}
		parent := len(s.parents) - 1
		parentSpecificity := 0
		for j := i - 1; j >= 0 && parent >= 0; j-- {
			if scopeMatches(s.parents[parent], stack[j]) {
				parentSpecificity += len(s.parents[parent])
				parent--
			}
		}
		if parent < 0 {
			return uint64(i+1)<<48 | uint64(len(s.leaf))<<32 | uint64(len(s.parents)+1)<<24 | uint64(parentSpecificity)
		}
	}
	return 0
}

// Resolve applies scope specificity, property inheritance and last-rule wins.
func (t *Theme) Resolve(scopes ...string) Style {
	style := t.base
	var colorScore, fontScore uint64
	for _, rule := range t.rules {
		score := rule.selector.score(scopes)
		if score == 0 {
			continue
		}
		if rule.hasColor && score >= colorScore {
			style.Foreground, colorScore = rule.color, score
		}
		if rule.hasFont && score >= fontScore {
			style.FontStyle, fontScore = rule.font, score
		}
	}
	return style
}

var builtinThemes sync.Map

var BuiltinThemeIDs = [...]string{"monokai", "dark-plus", "light-plus"}

func BuiltinTheme(id string) (*Theme, error) {
	if theme, ok := builtinThemes.Load(id); ok {
		return theme.(*Theme), nil
	}
	theme, err := LoadTheme(resources, "resources/themes/"+id+".json")
	if err != nil {
		return nil, err
	}
	actual, _ := builtinThemes.LoadOrStore(id, theme)
	return actual.(*Theme), nil
}
