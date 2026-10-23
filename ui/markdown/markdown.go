package markdown

import (
	"bytes"
	"fmt"
	"strconv"
	"sync"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/syntax"
	onixtext "github.com/diiyw/onix/text"
	ui "github.com/diiyw/onix/ui"
	htmlui "github.com/diiyw/onix/ui/html"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	htmlrenderer "github.com/yuin/goldmark/renderer/html"
	goldmarktext "github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// MarkdownStyle controls the stylesheet applied to converted Markdown HTML.
type MarkdownStyle struct {
	Font       *onixtext.Font
	BoldFont   *onixtext.Font
	FontSize   float64
	DPI        float64
	StackIndex int
	QuoteWidth float32

	Text            graphics.ColorRGBA
	Quote           graphics.ColorRGBA
	QuoteBackground graphics.ColorRGBA
	Italic          graphics.ColorRGBA
	Strike          graphics.ColorRGBA
	Bold            graphics.ColorRGBA
	Heading         graphics.ColorRGBA
	TableHeader     graphics.ColorRGBA
	Link            graphics.ColorRGBA
	Code            graphics.ColorRGBA
	Math            graphics.ColorRGBA

	CodeBackground        graphics.ColorRGBA
	MathBackground        graphics.ColorRGBA
	TableHeaderBackground graphics.ColorRGBA
	TableOddBackground    graphics.ColorRGBA
	TableEvenBackground   graphics.ColorRGBA
	Radius                ui.CornerRadius
	TableFontSize         float64
	InlineMathFontSize    float64
	DisplayMathFontSize   float64
}

// Markdown stores authored Markdown. Its rendered HTML lives in the html.Surface
// component on the same entity.
type Markdown struct {
	Value       string
	Width       float32
	PassThrough bool
	Style       MarkdownStyle
}

type MarkdownOptions struct {
	X, Y        float32
	Width       float32
	Value       string
	PassThrough bool
	Theme       ui.Theme
	Style       *MarkdownStyle
}

type MarkdownLayout = htmlui.Layout

type markdownRenderState struct {
	Key Markdown
}

type markdownSyncEntry struct {
	Entity ecs.Entity
	Value  Markdown
}

type markdownEntriesScratch struct{}

type markdownRuntime struct {
	style      *syntax.Theme
	background string
	css        string
	document   goldmark.Markdown
}

var markdownRuntimeOnce sync.Once
var markdownRuntimeValue markdownRuntime

func markdownRuntimeFor() *markdownRuntime {
	markdownRuntimeOnce.Do(func() {
		theme, err := syntax.BuiltinTheme("monokai")
		if err != nil {
			panic(err)
		}
		runtime := &markdownRuntime{style: theme, background: theme.Color("editor.background").Hex()}
		runtime.css = ".syntax{color:" + theme.Color("editor.foreground").Hex() + ";background-color:" + runtime.background + "}"
		runtime.document = goldmark.New(
			goldmark.WithExtensions(extension.GFM),
			goldmark.WithParserOptions(
				parser.WithAutoHeadingID(),
				parser.WithBlockParsers(util.Prioritized(markdownDisplayMathParser{}, 650)),
				parser.WithInlineParsers(util.Prioritized(markdownInlineMathParser{}, 150)),
			),
			goldmark.WithRendererOptions(
				htmlrenderer.WithUnsafe(),
				renderer.WithNodeRenderers(util.Prioritized(markdownHTMLNodeRenderer{}, 900)),
			),
		)
		markdownRuntimeValue = *runtime
	})
	return &markdownRuntimeValue
}

func DefaultMarkdownStyle(theme ui.Theme) MarkdownStyle {
	theme = ui.ThemeWithDefaults(theme)
	return MarkdownStyle{
		Font:                  theme.Font,
		FontSize:              theme.FontSize,
		DPI:                   theme.DPI,
		StackIndex:            theme.StackIndex,
		QuoteWidth:            3,
		Text:                  theme.Text,
		Quote:                 theme.Accent,
		QuoteBackground:       markdownAlphaColor(theme.SurfaceActive, 0.35),
		Italic:                theme.Accent,
		Strike:                theme.Muted,
		Bold:                  theme.Text,
		Heading:               theme.Accent,
		TableHeader:           theme.Accent,
		Link:                  theme.Primary,
		Code:                  theme.Danger,
		Math:                  theme.Accent,
		CodeBackground:        theme.Surface,
		MathBackground:        theme.Surface,
		TableHeaderBackground: theme.SurfaceActive,
		TableOddBackground:    theme.Surface,
		TableEvenBackground:   theme.SurfaceHover,
		Radius:                4,
		TableFontSize:         max(theme.FontSize-1, 6),
		InlineMathFontSize:    theme.FontSize,
		DisplayMathFontSize:   theme.FontSize + 3,
	}
}

func SpawnMarkdown(world *ecs.World, opts MarkdownOptions) ecs.Entity {
	component := markdownFromOptions(world, opts)
	html := Convert(component)
	spawner := ecs.NewSpawner7[
		scene.UITransform,
		scene.UIGlobalTransform,
		scene.Visibility,
		scene.Anchor,
		scene.UIStackIndex,
		Markdown,
		htmlui.Surface,
	](world)
	entity := spawner.Spawn(
		scene.UITransformFromXY(opts.X, opts.Y),
		scene.UIGlobalTransform{},
		scene.Visibility{Visible: true},
		scene.AnchorTopLeft,
		scene.UIStackIndex(component.Style.StackIndex),
		component,
		html,
	)
	_ = ecs.Insert(world, entity, scene.Size2D{Width: opts.Width})
	_ = ecs.Insert(world, entity, markdownRenderState{Key: component})
	return entity
}

func UpdateMarkdown(world *ecs.World, entity ecs.Entity, opts MarkdownOptions) MarkdownLayout {
	component := markdownFromOptions(world, opts)
	if current := ecs.GetMut[Markdown](world, entity); current != nil {
		*current = component
	} else {
		_ = ecs.Insert(world, entity, component)
	}
	renderMarkdown(world, entity, component)
	layout, _ := htmlui.Bounds(world, entity)
	return layout
}

func SetMarkdownVisible(world *ecs.World, entity ecs.Entity, visible bool) {
	if visibility := ecs.GetMut[scene.Visibility](world, entity); visibility != nil {
		visibility.Visible = visible
	}
}

func DespawnMarkdown(world *ecs.World, entity ecs.Entity) bool {
	return world.Despawn(entity)
}

func markdownFromOptions(world *ecs.World, opts MarkdownOptions) Markdown {
	return Markdown{
		Value: opts.Value, Width: opts.Width, PassThrough: opts.PassThrough,
		Style: resolvedMarkdownStyle(world, opts),
	}
}

func resolvedMarkdownStyle(world *ecs.World, opts MarkdownOptions) MarkdownStyle {
	defaults := DefaultMarkdownStyle(ui.ThemeForWorld(world, opts.Theme))
	if opts.Style == nil {
		return defaults
	}
	style := *opts.Style
	if style.Font == nil {
		style.Font = defaults.Font
	}
	style.FontSize = sizeOr(style.FontSize, defaults.FontSize)
	style.DPI = sizeOr(style.DPI, defaults.DPI)
	style.Text = colorOr(style.Text, defaults.Text)
	style.Quote = colorOr(style.Quote, defaults.Quote)
	style.QuoteBackground = colorOr(style.QuoteBackground, defaults.QuoteBackground)
	style.Italic = colorOr(style.Italic, defaults.Italic)
	style.Strike = colorOr(style.Strike, defaults.Strike)
	style.Bold = colorOr(style.Bold, defaults.Bold)
	style.Heading = colorOr(style.Heading, defaults.Heading)
	style.TableHeader = colorOr(style.TableHeader, defaults.TableHeader)
	style.Link = colorOr(style.Link, defaults.Link)
	style.Code = colorOr(style.Code, defaults.Code)
	style.Math = colorOr(style.Math, defaults.Math)
	style.CodeBackground = colorOr(style.CodeBackground, defaults.CodeBackground)
	style.MathBackground = colorOr(style.MathBackground, defaults.MathBackground)
	style.TableHeaderBackground = colorOr(style.TableHeaderBackground, defaults.TableHeaderBackground)
	style.TableOddBackground = colorOr(style.TableOddBackground, defaults.TableOddBackground)
	style.TableEvenBackground = colorOr(style.TableEvenBackground, defaults.TableEvenBackground)
	style.TableFontSize = sizeOr(style.TableFontSize, max(style.FontSize-1, 6))
	style.InlineMathFontSize = sizeOr(style.InlineMathFontSize, style.FontSize)
	style.DisplayMathFontSize = sizeOr(style.DisplayMathFontSize, style.FontSize+3)
	style.QuoteWidth = max(style.QuoteWidth, defaults.QuoteWidth)
	return style
}

func markdownAlphaColor(color graphics.ColorRGBA, alpha float32) graphics.ColorRGBA {
	color.A *= alpha
	return color
}

func sizeOr(value, fallback float64) float64 {
	if value == 0 {
		return fallback
	}
	return value
}

func colorOr(value, fallback graphics.ColorRGBA) graphics.ColorRGBA {
	if value.R == 0 && value.G == 0 && value.B == 0 && value.A == 0 {
		return fallback
	}
	return value
}

func syncMarkdownComponents(world *ecs.World) {
	entries := ecs.FrameScratch[markdownEntriesScratch, markdownSyncEntry](world)
	defer entries.ReleaseUnused()
	for entity, access := range ecs.Query[Markdown](world).All {
		value := access.Get()

		entries.Values = append(entries.Values, markdownSyncEntry{Entity: entity, Value: value})
	}
	for _, entry := range entries.Values {
		renderMarkdown(world, entry.Entity, entry.Value)
	}
}

func renderMarkdown(world *ecs.World, entity ecs.Entity, value Markdown) {
	state := ecs.GetMut[markdownRenderState](world, entity)
	if state != nil && state.Key == value {
		return
	}
	html := Convert(value)
	if current := ecs.GetMut[htmlui.Surface](world, entity); current != nil {
		*current = html
	} else {
		_ = ecs.Insert(world, entity, html)
	}
	if state == nil {
		_ = ecs.Insert(world, entity, markdownRenderState{Key: value})
		return
	}
	state.Key = value
}

// Convert renders Markdown and its stylesheet for an owned HTML surface.
// Call it when the source/style changes; retained HTML handles stable frames.
func Convert(value Markdown) htmlui.Surface {
	runtime := markdownRuntimeFor()
	html := htmlui.New(markdownToHTML(value.Value))
	html.CSS = markdownStylesheet(value.Style, runtime)
	html.Width = value.Width
	html.Font = value.Style.Font
	html.BoldFont = value.Style.BoldFont
	html.FontSize = value.Style.FontSize
	html.DPI = value.Style.DPI
	html.StackIndex = value.Style.StackIndex
	html.PassThrough = value.PassThrough
	return html
}

func markdownToHTML(value string) string {
	runtime := markdownRuntimeFor()
	var output bytes.Buffer
	if err := runtime.document.Convert([]byte(value), &output); err != nil {
		return ""
	}
	return output.String()
}

type markdownHTMLNodeRenderer struct{}

func (markdownHTMLNodeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindListItem, renderMarkdownListItem)
	reg.Register(ast.KindFencedCodeBlock, renderMarkdownFencedCodeBlock)
	reg.Register(kindMarkdownInlineMath, renderMarkdownInlineMath)
	reg.Register(kindMarkdownDisplayMath, renderMarkdownDisplayMath)
}

type markdownInlineMath struct {
	ast.BaseInline
	Content []byte
}

var kindMarkdownInlineMath = ast.NewNodeKind("MarkdownInlineMath")

func (*markdownInlineMath) Kind() ast.NodeKind { return kindMarkdownInlineMath }

func (math *markdownInlineMath) Dump(source []byte, level int) {
	ast.DumpHelper(math, source, level, map[string]string{"Content": string(math.Content)}, nil)
}

type markdownInlineMathParser struct{}

func (markdownInlineMathParser) Trigger() []byte { return []byte{'$'} }

func (markdownInlineMathParser) Parse(_ ast.Node, reader goldmarktext.Reader, _ parser.Context) ast.Node {
	line, _ := reader.PeekLine()
	if len(line) < 3 || line[1] == '$' {
		return nil
	}
	for end := 1; end < len(line); end++ {
		if line[end] == '\\' {
			end++
			continue
		}
		if line[end] != '$' {
			continue
		}
		content := line[1:end]
		if len(bytes.TrimSpace(content)) == 0 {
			return nil
		}
		reader.Advance(end + 1)
		return &markdownInlineMath{Content: content}
	}
	return nil
}

type markdownDisplayMath struct {
	ast.BaseBlock
}

var kindMarkdownDisplayMath = ast.NewNodeKind("MarkdownDisplayMath")

func (*markdownDisplayMath) Kind() ast.NodeKind { return kindMarkdownDisplayMath }

func (*markdownDisplayMath) IsRaw() bool { return true }

func (math *markdownDisplayMath) Dump(source []byte, level int) {
	ast.DumpHelper(math, source, level, map[string]string{"Content": string(math.Lines().Value(source))}, nil)
}

type markdownDisplayMathParser struct{}

func (markdownDisplayMathParser) Trigger() []byte { return []byte{'$'} }

func (markdownDisplayMathParser) Open(_ ast.Node, reader goldmarktext.Reader, context parser.Context) (ast.Node, parser.State) {
	line, _ := reader.PeekLine()
	offset := context.BlockOffset()
	if offset < 0 || !bytes.Equal(bytes.TrimSpace(line[offset:]), []byte("$$")) {
		return nil, parser.NoChildren
	}
	reader.AdvanceToEOL()
	return &markdownDisplayMath{}, parser.NoChildren
}

func (markdownDisplayMathParser) Continue(node ast.Node, reader goldmarktext.Reader, _ parser.Context) parser.State {
	line, segment := reader.PeekLine()
	if len(line) == 0 {
		return parser.Close
	}
	if bytes.Equal(bytes.TrimSpace(line), []byte("$$")) {
		reader.AdvanceToEOL()
		return parser.Close
	}
	segment.ForceNewline = true
	node.Lines().Append(segment)
	reader.AdvanceToEOL()
	return parser.Continue | parser.NoChildren
}

func (markdownDisplayMathParser) Close(ast.Node, goldmarktext.Reader, parser.Context) {}

func (markdownDisplayMathParser) CanInterruptParagraph() bool { return true }

func (markdownDisplayMathParser) CanAcceptIndentedLine() bool { return false }

// The HTML layout engine does not synthesize list markers, so emit markers as
// ordinary HTML text while Goldmark walks list item nodes.
func renderMarkdownListItem(writer util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	item := node.(*ast.ListItem)
	if entering {
		_, _ = writer.WriteString(`<li><span class="markdown-list-marker">`)
		list, _ := item.Parent().(*ast.List)
		if list == nil || !list.IsOrdered() {
			_, _ = writer.WriteString("&bull; ")
		} else {
			var number [20]byte
			_, _ = writer.Write(strconv.AppendInt(number[:0], int64(markdownListItemNumber(item)), 10))
			_, _ = writer.WriteString(". ")
		}
		_, _ = writer.WriteString("</span>")
	} else {
		_, _ = writer.WriteString("</li>\n")
	}
	return ast.WalkContinue, nil
}

func renderMarkdownInlineMath(writer util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	math := node.(*markdownInlineMath)
	_, _ = writer.WriteString(`<span class="math inline">`)
	writeMarkdownMathHTML(writer, math.Content)
	_, _ = writer.WriteString(`</span>`)
	return ast.WalkContinue, nil
}

func renderMarkdownDisplayMath(writer util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	math := node.(*markdownDisplayMath)
	_, _ = writer.WriteString(`<div class="math display">`)
	writeMarkdownMathHTML(writer, math.Lines().Value(source))
	_, _ = writer.WriteString("</div>\n")
	return ast.WalkContinue, nil
}

func renderMarkdownFencedCodeBlock(writer util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	runtime := markdownRuntimeFor()
	if !entering {
		return ast.WalkContinue, nil
	}
	codeBlock := node.(*ast.FencedCodeBlock)
	language := codeBlock.Language(source)
	if len(language) != 0 {
		code := markdownCodeBlockSource(codeBlock, source)
		var document syntax.Document
		defer document.Close()
		if err := document.Update(string(language), code); err == nil && document.Language() != "" {
			writeMarkdownCodeBlockStart(writer, language, true)
			offset := 0
			for _, token := range document.Tokens(runtime.style, nil) {
				htmlrenderer.DefaultWriter.RawWrite(writer, []byte(code[offset:token.Start]))
				_, _ = writer.WriteString(`<span class="syntax-token" style="`)
				_, _ = writer.WriteString(token.CSS)
				_, _ = writer.WriteString(`">`)
				htmlrenderer.DefaultWriter.RawWrite(writer, []byte(code[token.Start:token.End]))
				_, _ = writer.WriteString("</span>")
				offset = token.End
			}
			htmlrenderer.DefaultWriter.RawWrite(writer, []byte(code[offset:]))
			_, _ = writer.WriteString("</code></pre>\n")
			return ast.WalkContinue, nil
		}
	}

	writeMarkdownCodeBlockStart(writer, language, false)
	for i := range codeBlock.Lines().Len() {
		line := codeBlock.Lines().At(i)
		htmlrenderer.DefaultWriter.RawWrite(writer, line.Value(source))
	}
	_, _ = writer.WriteString("</code></pre>\n")
	return ast.WalkContinue, nil
}

func markdownCodeBlockSource(codeBlock *ast.FencedCodeBlock, source []byte) string {
	var code bytes.Buffer
	for i := range codeBlock.Lines().Len() {
		line := codeBlock.Lines().At(i)
		_, _ = code.Write(line.Value(source))
	}
	return code.String()
}

func writeMarkdownCodeBlockStart(writer util.BufWriter, language []byte, highlighted bool) {
	_, _ = writer.WriteString("<pre><code")
	if highlighted || len(language) != 0 {
		_, _ = writer.WriteString(` class="`)
		if highlighted {
			_, _ = writer.WriteString("syntax")
		}
		if len(language) != 0 {
			if highlighted {
				_ = writer.WriteByte(' ')
			}
			_, _ = writer.WriteString("language-")
			_, _ = writer.Write(util.EscapeHTML(language))
		}
		_ = writer.WriteByte('"')
	}
	_ = writer.WriteByte('>')
}

func markdownListItemNumber(item *ast.ListItem) int {
	list, ok := item.Parent().(*ast.List)
	if !ok {
		return 1
	}
	number := max(list.Start, 1)
	for child := list.FirstChild(); child != nil; child = child.NextSibling() {
		if child == item {
			break
		}
		if _, ok := child.(*ast.ListItem); ok {
			number++
		}
	}
	return number
}

func markdownStylesheet(style MarkdownStyle, runtime *markdownRuntime) string {
	return fmt.Sprintf(`
*,html,body{color:%s}
html,body{margin:0;padding:0;background-color:rgba(0,0,0,0)}
body{width:100%%;line-height:1.45}
h1,h2,h3,h4,h5,h6{margin:0.8em 0 0.4em 0;color:%s;font-weight:700;line-height:1.2}
h1{font-size:2em}h2{font-size:1.5em}h3{font-size:1.25em}h4{font-size:1.1em}h5,h6{font-size:1em}
p{margin:0 0 0.75em 0}
strong{color:%s;font-weight:700}em{color:%s}del{color:%s;text-decoration-line:line-through}
a{color:%s}
blockquote{margin:0.75em 0;padding:0.65em 0.9em;border-left:%gpx solid %s;color:%s;background-color:%s;border-radius:%gpx}
blockquote > p{margin:0}
pre{margin:0.75em 0;padding:0.75em 0.9em;background-color:%s;border-radius:%gpx;white-space:pre-wrap}
code{padding:0.1em 0.3em;color:%s;background-color:%s;border-radius:%gpx;white-space:pre-wrap}
pre code{display:block;padding:0;background-color:rgba(0,0,0,0)}
ul,ol{margin:0.5em 0 0.75em 0;padding:0}li{margin:0.25em 0}.markdown-list-marker{color:%s}
hr{display:block;width:100%%;height:1px;margin:0.8em 0;background-color:%s}
table{display:block;width:100%%;margin:0.75em 0;font-size:%gpx}
thead,tbody,tfoot{display:block}tr{display:flex;width:100%%}
th,td{display:block;flex:1;padding:0.45em 0.6em}
th{color:%s;background-color:%s;font-weight:700}
tbody tr:nth-child(odd) td{background-color:%s}tbody tr:nth-child(even) td{background-color:%s}
.math{align-items:center;white-space:nowrap;line-height:1.15}
.math.inline{display:inline-flex;color:%s;font-size:%gpx}
.math.display{display:flex;justify-content:center;margin:0.75em 0;padding:0.65em;color:%s;background-color:%s;border-radius:%gpx;font-size:%gpx}
.math-frac,.math-radical,.math-scripts,.math-style,.math-matrix,.math-row{display:inline-flex;align-items:center}
.math-frac{flex-direction:column;padding:0 0.12em}.math-num,.math-den{display:flex;justify-content:center;padding:0 0.12em}
.math-den{border-top:1px solid %s}.math-frac-no-bar .math-den{border-top-style:none}
.math-script-stack,.math-grid{display:inline-flex;flex-direction:column}.math-script-stack{font-size:0.72em;line-height:1}
.math-script-stack sup,.math-script-stack sub{display:block;line-height:1}
.math-base,.math-radicand,.math-radicand-body,.math-cell{display:flex;align-items:center}.math-radicand{border-top:1px solid %s;padding:0 0.08em}.math-radicand-body{position:relative;left:-0.32em}
.math-root-index{font-size:0.62em}.math-root-symbol,.math-delimiter{font-size:1.25em}
.math-grid{row-gap:0.18em}.math-row{column-gap:0.45em}.math-cell{flex:1}.math-align-left{justify-content:flex-start}.math-align-center{justify-content:center}.math-align-right{justify-content:flex-end}
.math-bold{font-weight:700}.math-mono{font-family:monospace}
.math-bar,.math-overline{border-top:1px solid %s}.math-underline{border-bottom:1px solid %s}

%s
`,
		style.Text.String(), style.Heading.String(), style.Bold.String(), style.Italic.String(), style.Strike.String(),
		style.Link.String(), style.QuoteWidth, style.Quote.String(), style.Quote.String(), style.QuoteBackground.String(), float64(style.Radius), runtime.background, float64(style.Radius),
		style.Code.String(), style.CodeBackground.String(), float64(style.Radius), style.Quote.String(), style.Quote.String(), style.TableFontSize,
		style.TableHeader.String(), style.TableHeaderBackground.String(), style.TableOddBackground.String(), style.TableEvenBackground.String(),
		style.Math.String(), style.InlineMathFontSize, style.Math.String(), style.MathBackground.String(), float64(style.Radius), style.DisplayMathFontSize,
		style.Math.String(), style.Math.String(), style.Math.String(), style.Math.String(), runtime.css,
	)
}
