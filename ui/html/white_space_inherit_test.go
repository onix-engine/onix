package html

import (
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/html/layout"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
)

func collectHTMLFragments(t *testing.T, page string) []layout.TextFragment {
	t.Helper()
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	html := New(page)
	html.Width, html.Height, html.Font = 1200, 650, notosanssc.NotoSansSCRegular
	bundle.Put(html)
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	state, _ := world.Get[htmlRenderState](owner)
	var fragments []layout.TextFragment
	var visit func(*layout.Box)
	visit = func(box *layout.Box) {
		fragments = append(fragments, box.TextFragments...)
		for _, child := range box.Children {
			visit(child)
		}
	}
	visit(state.Boxes)
	return fragments
}

// Chromium keeps line breaks and spaces for a <code> inside <pre> because
// white-space inherits; the CSS-wide keyword must not fall back to `normal`.
func TestHTMLWhiteSpaceInheritKeepsPreLayout(t *testing.T) {
	page := `<style>
		.code{white-space:pre}
		.code code{display:block;white-space:inherit}
	</style><pre class="code"><code>package main
import (
	"errors"
)</code></pre>`
	fragments := collectHTMLFragments(t, page)
	lines := map[float64]bool{}
	spaces := 0
	for _, fragment := range fragments {
		if fragment.Style == nil || fragment.Style.WhiteSpace.Keyword != "pre" {
			continue
		}
		lines[fragment.Rect.Y] = true
		if strings.TrimSpace(fragment.Text) == "" {
			spaces++
		}
	}
	if len(lines) != 4 {
		t.Fatalf("distinct line ys = %d, want 4 (fragments=%q)", len(lines), fragmentTexts(fragments))
	}
	if spaces == 0 {
		t.Fatalf("expected preserved whitespace fragments, got %q", fragmentTexts(fragments))
	}
}

func fragmentTexts(fragments []layout.TextFragment) []string {
	texts := make([]string, 0, len(fragments))
	for _, fragment := range fragments {
		texts = append(texts, fragment.Text)
	}
	return texts
}
