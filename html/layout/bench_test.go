package layout

import (
	"strings"
	"testing"

	"github.com/diiyw/onix/font"
	"github.com/diiyw/onix/html/css"
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/style"
)

func benchFace(b *testing.B) *font.Face {
	b.Helper()
	face, err := font.LoadDefault()
	if err != nil {
		b.Fatalf("load default font: %v", err)
	}
	return face
}

func buildBenchDocument(b *testing.B, html, cssSource string, width, height float64) (*dom.Node, style.Map, *font.Face) {
	b.Helper()
	doc, err := dom.Parse(html)
	if err != nil {
		b.Fatalf("parse dom: %v", err)
	}
	stylesheet, err := css.Parse(cssSource)
	if err != nil {
		b.Fatalf("parse css: %v", err)
	}
	styles, err := style.ComputeWithViewport(doc, stylesheet, style.Viewport{Width: width, Height: height})
	if err != nil {
		b.Fatalf("compute styles: %v", err)
	}
	face := benchFace(b)
	if _, err := BuildWithFonts(doc, styles, face, face, width, height); err != nil {
		b.Fatalf("build layout: %v", err)
	}
	return doc, styles, face
}

func benchLayout(b *testing.B, html, cssSource string, width, height float64) {
	b.Helper()
	doc, styles, face := buildBenchDocument(b, html, cssSource, width, height)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := BuildWithFonts(doc, styles, face, face, width, height); err != nil {
			b.Fatalf("build layout: %v", err)
		}
	}
}

func benchStyle(b *testing.B, html, cssSource string, width, height float64) {
	b.Helper()
	doc, err := dom.Parse(html)
	if err != nil {
		b.Fatalf("parse dom: %v", err)
	}
	stylesheet, err := css.Parse(cssSource)
	if err != nil {
		b.Fatalf("parse css: %v", err)
	}
	viewport := style.Viewport{Width: width, Height: height}
	input := &css.State{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := style.ComputeWithViewportAndState(doc, stylesheet, viewport, input); err != nil {
			b.Fatalf("compute styles: %v", err)
		}
	}
}

func benchTextDocument() (string, string) {
	var body strings.Builder
	for i := 0; i < 40; i++ {
		body.WriteString(`<p class="para">The quick brown fox jumps over the lazy dog while 引擎渲染器 continuously shapes 中文文本 with mixed scripts and numbers 12345.</p>`)
		body.WriteString(`<p class="para"><b>Bold</b> segment with <span class="hl">highlighted</span> inline spans and <span class="dim">secondary text</span>.</p>`)
	}
	css := `
		body { font-size: 14px; color: #222; }
		p.para { margin: 8px 0; line-height: 1.5; }
		.hl { color: #c00; }
		.dim { color: #888; font-size: 12px; }
	`
	return "<html><body>" + body.String() + "</body></html>", css
}

func benchFlexDocument() (string, string) {
	var body strings.Builder
	for i := 0; i < 30; i++ {
		body.WriteString(`<div class="row"><div class="cell">Name</div><div class="cell grow">Value content here</div><div class="cell">42</div></div>`)
		body.WriteString(`<div class="grid"><div class="tile"></div><div class="tile"></div><div class="tile"></div><div class="tile"></div></div>`)
	}
	css := `
		body { font-size: 14px; }
		.row { display: flex; flex-direction: row; gap: 8px; padding: 4px; }
		.cell { padding: 4px 8px; }
		.grow { flex-grow: 1; }
		.grid { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 4px; }
		.tile { width: 60px; height: 40px; background: #345; }
	`
	return "<html><body>" + body.String() + "</body></html>", css
}

func benchControlsDocument() (string, string) {
	var body strings.Builder
	for i := 0; i < 20; i++ {
		body.WriteString(`<div class="form-row"><label>User</label><input type="text" value="hello world" placeholder="type here"/><button type="submit">Submit</button></div>`)
		body.WriteString(`<div class="form-row"><textarea cols="30" rows="2">text</textarea><select><option>One</option><option selected>Two</option></select></div>`)
	}
	css := `
		body { font-size: 14px; }
		.form-row { display: flex; align-items: center; gap: 6px; margin: 4px 0; }
		input, button, select, textarea { padding: 4px 6px; border: 1px solid #999; }
	`
	return "<html><body>" + body.String() + "</body></html>", css
}

func BenchmarkBuildTextDocument(b *testing.B) {
	html, cssSource := benchTextDocument()
	benchLayout(b, html, cssSource, 800, 2000)
}

func BenchmarkBuildFlexDocument(b *testing.B) {
	html, cssSource := benchFlexDocument()
	benchLayout(b, html, cssSource, 800, 2000)
}

func BenchmarkBuildControlsDocument(b *testing.B) {
	html, cssSource := benchControlsDocument()
	benchLayout(b, html, cssSource, 800, 2000)
}

func BenchmarkComputeTextDocument(b *testing.B) {
	html, cssSource := benchTextDocument()
	benchStyle(b, html, cssSource, 800, 2000)
}

func BenchmarkComputeFlexDocument(b *testing.B) {
	html, cssSource := benchFlexDocument()
	benchStyle(b, html, cssSource, 800, 2000)
}

func BenchmarkComputeControlsDocument(b *testing.B) {
	html, cssSource := benchControlsDocument()
	benchStyle(b, html, cssSource, 800, 2000)
}
