package markdown

import (
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	engineui "github.com/diiyw/onix/ui"
	htmlui "github.com/diiyw/onix/ui/html"
)

func TestMarkdownConvertsAndRendersThroughHTML(t *testing.T) {
	world := newUIWorld(t)
	value := "# Heading\n\n**bold** and [docs](https://example.com).\n\n- one\n- two\n\n```go\nfmt.Println(1)\n```\n\n| Name | Value |\n| --- | ---: |\n| A | 10 |"
	entity := SpawnMarkdown(world, MarkdownOptions{Width: 320, Value: value, PassThrough: true})

	html, ok := world.Get[htmlui.Surface](entity)
	if !ok {
		t.Fatal("Markdown entity has no HTML component")
	}
	for _, fragment := range []string{"<h1", "<strong>bold</strong>", "<a href=", "<pre><code", "<table>"} {
		if !strings.Contains(html.Value, fragment) {
			t.Fatalf("converted HTML does not contain %q: %s", fragment, html.Value)
		}
	}
	if html.CSS == "" || html.Width != 320 || !html.PassThrough {
		t.Fatalf("HTML configuration = %#v", html)
	}

	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	layout, ok := htmlui.Bounds(world, entity)
	if !ok || layout.Width <= 0 || layout.Height <= 0 {
		t.Fatalf("Markdown HTML layout = %#v, ok=%v", layout, ok)
	}
	firstChildren := markdownHTMLChildren(world, entity)
	if len(firstChildren) == 0 {
		t.Fatal("Markdown HTML did not create visual children")
	}

	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if children := markdownHTMLChildren(world, entity); !slices.Equal(firstChildren, children) {
		t.Fatalf("stable Markdown HTML was rebuilt: before=%v after=%v", firstChildren, children)
	}

	UpdateMarkdown(world, entity, MarkdownOptions{Width: 320, Value: "## Replacement"})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	for _, child := range firstChildren {
		if world.IsAlive(child) {
			t.Fatalf("stale Markdown HTML child %d is still alive", child)
		}
	}
}

func TestMarkdownCodeBlockRendersHighlightedText(t *testing.T) {
	world := newUIWorld(t)
	value := "```go\nentity := world.Spawn()\n```"
	owner := SpawnMarkdown(world, MarkdownOptions{Width: 320, Value: value, PassThrough: true})

	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	html, ok := world.Get[htmlui.Surface](owner)
	if !ok {
		t.Fatal("Markdown entity has no HTML component")
	}
	if strings.Count(html.Value, "<pre") != 1 || strings.Count(html.Value, "<code") != 1 {
		t.Fatalf("highlighted code generated nested wrappers: %s", html.Value)
	}
	if !strings.Contains(html.Value, `class="syntax language-go"`) || !strings.Contains(html.Value, `<span class="syntax-token" style="color:#a6e22e">Spawn</span>`) {
		t.Fatalf("tree-sitter HTML output is missing colored functions: %s", html.Value)
	}
	if !strings.Contains(html.CSS, ".syntax{color:#f8f8f2;background-color:#272822}") {
		t.Fatalf("syntax CSS output is missing the theme colors: %s", html.CSS)
	}
	if !strings.Contains(html.CSS, "pre{margin:0.75em 0;padding:0.75em 0.9em;background-color:#272822") {
		t.Fatalf("Markdown code block background does not match the syntax theme: %s", html.CSS)
	}
	if !strings.Contains(html.CSS, "pre code{display:block") {
		t.Fatalf("code block stylesheet does not promote code to a layout block: %s", html.CSS)
	}

	var rendered strings.Builder
	for entity, access := range world.Query[scene.Text2D]().All {
		text := access.Get()

		parent, parentOK := world.Get[scene.Parent](entity)
		if parentOK && parent.Entity == owner {
			rendered.WriteString(text.Value)
		}
	}
	if !strings.Contains(rendered.String(), "entity") || !strings.Contains(rendered.String(), "Spawn") {
		t.Fatalf("highlighted code text was not rendered: %q", rendered.String())
	}
}

func TestMarkdownCodeBlockKeepsIndentedLinesTogether(t *testing.T) {
	world := newUIWorld(t)
	owner := SpawnMarkdown(world, MarkdownOptions{
		Width: 904,
		Value: "```go\nentity := ui.SpawnMarkdown(world, ui.MarkdownOptions{\n    Width: 720,\n    Value: \"# Hello, HTML\",\n}\n```",
	})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}

	type fragment struct {
		value string
		x, y  float32
	}
	fragments := make([]fragment, 0, 16)
	for _, access := range world.Query3[scene.Parent, scene.Text2D, scene.UITransform]().All {
		parent := access.A.Mut()
		text := access.B.Mut()
		transform := access.C.Mut()

		if parent.Entity == owner {
			fragments = append(fragments, fragment{
				value: text.Value,
				x:     transform.Translation.X.Value,
				y:     transform.Translation.Y.Value,
			})
		}
	}
	slices.SortFunc(fragments, func(left, right fragment) int {
		if left.y < right.y {
			return -1
		}
		if left.y > right.y {
			return 1
		}
		if left.x < right.x {
			return -1
		}
		if left.x > right.x {
			return 1
		}
		return 0
	})
	if len(fragments) == 0 {
		t.Fatal("code block produced no text fragments")
	}
	lineYs := make([]float32, 0, 4)
	lines := make([]string, 0, 4)
	for _, current := range fragments {
		if len(lineYs) == 0 || math.Abs(float64(current.y-lineYs[len(lineYs)-1])) > 0.01 {
			lineYs = append(lineYs, current.y)
			lines = append(lines, current.value)
			continue
		}
		lines[len(lines)-1] += current.value
	}
	wantLines := []string{
		"entity := ui.SpawnMarkdown(world, ui.MarkdownOptions{",
		"    Width: 720,",
		"    Value: \"# Hello, HTML\",",
		"}",
	}
	if !slices.Equal(lines, wantLines) {
		t.Fatalf("code block lines = %#v, want %#v", lines, wantLines)
	}
}

func TestMarkdownCodeBlockRendersUnhighlightedText(t *testing.T) {
	world := newUIWorld(t)
	value := "```unknown-language\n    raw <code>\n```"
	owner := SpawnMarkdown(world, MarkdownOptions{Width: 320, Value: value})

	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	var rendered strings.Builder
	for entity, access := range world.Query[scene.Text2D]().All {
		text := access.Get()

		parent, parentOK := world.Get[scene.Parent](entity)
		if parentOK && parent.Entity == owner {
			rendered.WriteString(text.Value)
		}
	}
	if !strings.Contains(rendered.String(), "    ") || !strings.Contains(rendered.String(), "raw") || !strings.Contains(rendered.String(), "<code>") {
		t.Fatalf("unhighlighted code text was not rendered: %q", rendered.String())
	}
}

func TestMarkdownSyncConvertsDirectComponentChangesBeforeHTML(t *testing.T) {
	world := ecs.NewWorld()
	engineui.RegisterRuntimeFeature[engineui.HTMLRenderer](world)
	engineui.RegisterRuntimeExtension(world, engineui.RuntimeExtension{Sync: syncMarkdown})
	theme := engineui.DefaultTheme()
	theme.Font = notosanssc.NotoSansSCRegular
	entity := SpawnMarkdown(world, MarkdownOptions{Width: 240, Value: "before", Theme: theme})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	value := world.GetMut[Markdown](entity)
	value.Value = "# After"
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	html, ok := world.Get[htmlui.Surface](entity)
	if !ok || !strings.Contains(html.Value, "<h1 id=\"after\">After</h1>") {
		t.Fatalf("direct Markdown update produced %#v", html)
	}
	found := false
	for _, access := range world.Query[scene.Text2D]().All {
		text := access.Get()

		found = found || text.Value == "After"
		if !(!found) {
			break
		}
	}
	if !found {
		t.Fatal("updated Markdown HTML was not laid out in the same sync pass")
	}
}

func TestMarkdownStyleBecomesHTMLCSS(t *testing.T) {
	world := newUIWorld(t)
	style := DefaultMarkdownStyle(engineui.DefaultTheme())
	style.Font = notosanssc.NotoSansSCRegular
	style.Heading = graphics.ColorFromRGB(12, 34, 56)
	style.CodeBackground = graphics.ColorFromRGB(65, 43, 21)
	entity := SpawnMarkdown(world, MarkdownOptions{Width: 200, Value: "# Styled", Style: &style})
	html, _ := world.Get[htmlui.Surface](entity)
	if html.Font != style.Font || !strings.Contains(html.CSS, style.Heading.String()) || !strings.Contains(html.CSS, style.CodeBackground.String()) {
		t.Fatalf("Markdown style was not forwarded to HTML: %#v", html)
	}
}

func TestMarkdownRendersBoldAndDeletedTextStyles(t *testing.T) {
	world := newUIWorld(t)
	style := DefaultMarkdownStyle(engineui.DefaultTheme())
	style.Font = notosanssc.NotoSansSCRegular
	style.BoldFont = notosanssc.NotoSansSCBold
	owner := SpawnMarkdown(world, MarkdownOptions{Width: 240, Value: "**bold** and ~~deleted~~", Style: &style})
	html, _ := world.Get[htmlui.Surface](owner)
	if html.BoldFont != style.BoldFont || !strings.Contains(html.CSS, "text-decoration-line:line-through") {
		t.Fatalf("Markdown emphasis styles were not forwarded to HTML: %#v", html)
	}
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}

	boldFound, lineFound := false, false
	for _, child := range markdownHTMLChildren(world, owner) {
		if text, ok := world.Get[scene.Text2D](child); ok && text.Value == "bold" {
			boldFound = text.Font == style.BoldFont
		}
		if sprite, ok := world.Get[scene.Sprite2D](child); ok && sprite.Shape.Fill == style.Strike {
			size, _ := world.Get[scene.Size2D](child)
			lineFound = size.Width > size.Height && size.Height > 0
		}
	}
	if !boldFound || !lineFound {
		t.Fatalf("Markdown visuals = bold:%v line-through:%v", boldFound, lineFound)
	}
}

func TestMarkdownBlockquoteStyleUsesSurfaceBackground(t *testing.T) {
	world := newUIWorld(t)
	style := DefaultMarkdownStyle(engineui.DefaultTheme())
	style.Font = notosanssc.NotoSansSCRegular
	style.QuoteBackground = graphics.ColorFromRGB(12, 34, 56)
	entity := SpawnMarkdown(world, MarkdownOptions{Width: 240, Value: "> quote", Style: &style})
	html, _ := world.Get[htmlui.Surface](entity)
	if !strings.Contains(html.CSS, "blockquote{") || !strings.Contains(html.CSS, style.QuoteBackground.String()) {
		t.Fatalf("blockquote background was not forwarded to HTML CSS: %s", html.CSS)
	}
}

func TestMarkdownListsReceiveVisibleHTMLMarkers(t *testing.T) {
	html := markdownToHTML("3. three\n4. four\n\n- bullet")
	for _, marker := range []string{"3. ", "4. ", "&bull; "} {
		if !strings.Contains(html, marker) {
			t.Fatalf("converted list does not contain marker %q: %s", marker, html)
		}
	}
}

func TestMarkdownDisplayMathDoesNotRenderSourceTwice(t *testing.T) {
	value := `before

$$
\begin{aligned}
a &= b \\
c &= d
\end{aligned}
$$

after`
	html := markdownToHTML(value)
	if strings.Count(html, `class="math display"`) != 1 {
		t.Fatalf("display math rendered %d times: %s", strings.Count(html, `class="math display"`), html)
	}
	if strings.Contains(html, `\begin{aligned}`) || strings.Contains(html, `\end{aligned}`) {
		t.Fatalf("display math source leaked into HTML: %s", html)
	}
	if !strings.Contains(html, "<p>after</p>") {
		t.Fatalf("content after display math was not rendered: %s", html)
	}
}

func TestMarkdownDisplayMathKeepsAlignedGlyphsSeparated(t *testing.T) {
	world := newUIWorld(t)
	value := `$$
\begin{aligned}
f(x) &= x^2 + 2x + 1 \\
f'(x) &= 2x + 2 \\
\int_0^1 x^2\,dx &= \frac{1}{3}
\end{aligned}
$$`
	owner := SpawnMarkdown(world, MarkdownOptions{Width: 904, Value: value})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}

	type placement struct {
		value string
		x, y  float32
		width float32
	}
	placements := make([]placement, 0, 16)
	for entity, access := range world.Query2[scene.Parent, scene.Text2D]().All {
		parent := access.A.Get()
		text := access.B.Get()

		if parent.Entity != owner {
			continue
		}
		transform, transformOK := world.Get[scene.UITransform](entity)
		size, sizeOK := world.Get[scene.Size2D](entity)
		if transformOK && sizeOK {
			placements = append(placements, placement{
				value: text.Value,
				x:     transform.Translation.X.Value,
				y:     transform.Translation.Y.Value,
				width: size.Width,
			})
		}
	}

	equals := make([]placement, 0, 3)
	for _, current := range placements {
		if current.value == "=" {
			equals = append(equals, current)
		}
	}
	if len(equals) != 3 {
		t.Fatalf("display math equals placements = %#v", equals)
	}
	for _, current := range equals[1:] {
		if math.Abs(float64(current.x-equals[0].x)) > 0.01 {
			t.Fatalf("aligned equals columns drifted: %#v", equals)
		}
	}

	for _, left := range placements {
		for _, right := range placements {
			if math.Abs(float64(left.y-right.y)) > 0.01 || left.x >= right.x {
				continue
			}
			if left.x+left.width > right.x+0.01 {
				t.Fatalf("display math text overlaps: left=%#v right=%#v placements=%#v", left, right, placements)
			}
		}
	}
}

func TestMarkdownRadicandMovesInsideRootWithoutMovingRadical(t *testing.T) {
	world := newUIWorld(t)
	owner := SpawnMarkdown(world, MarkdownOptions{Width: 240, Value: `$\sqrt{x^2+y^2}\leq 3$`})
	html, _ := world.Get[htmlui.Surface](owner)
	if !strings.Contains(html.Value, `class="math-radicand-body"`) || !strings.Contains(html.CSS, `.math-radicand-body{position:relative;left:-0.32em}`) {
		t.Fatalf("radicand inner offset is missing: value=%s css=%s", html.Value, html.CSS)
	}
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}

	type placement struct {
		x, width float32
	}
	positions := make(map[string]placement, 3)
	for entity, access := range world.Query2[scene.Parent, scene.Text2D]().All {
		parent := access.A.Get()
		text := access.B.Get()

		if parent.Entity != owner || (text.Value != "√" && text.Value != "x" && text.Value != "≤") {
			continue
		}
		transform, transformOK := world.Get[scene.UITransform](entity)
		size, sizeOK := world.Get[scene.Size2D](entity)
		if transformOK && sizeOK {
			positions[text.Value] = placement{x: transform.Translation.X.Value, width: size.Width}
		}
	}
	root, rootOK := positions["√"]
	radical, radicalOK := positions["x"]
	operator, operatorOK := positions["≤"]
	if !rootOK || !radicalOK || !operatorOK {
		t.Fatalf("radical placements = %#v", positions)
	}
	if radical.x >= root.x+root.width {
		t.Fatalf("radicand still starts after the root advance: root=%#v radicand=%#v", root, radical)
	}
	if radical.x <= root.x || radical.x+radical.width > operator.x+0.01 {
		t.Fatalf("radicand offset escaped the radical: root=%#v radicand=%#v operator=%#v", root, radical, operator)
	}
}

func TestMarkdownFractionRendersHorizontalBar(t *testing.T) {
	world := newUIWorld(t)
	owner := SpawnMarkdown(world, MarkdownOptions{Width: 240, Value: `$\frac{1}{3}$`})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}

	shapeCount, barCount := 0, 0
	for _, child := range markdownHTMLChildren(world, owner) {
		sprite, spriteOK := world.Get[scene.Sprite2D](child)
		size, sizeOK := world.Get[scene.Size2D](child)
		if !spriteOK || !sizeOK || sprite.Shape.Kind == 0 {
			continue
		}
		shapeCount++
		if sprite.Shape.Fill.A == 0 || sprite.Shape.Border.A != 0 {
			continue
		}
		if size.Height <= 1.01 && size.Width > size.Height {
			barCount++
		}
	}
	if shapeCount != 1 || barCount != 1 {
		t.Fatalf("fraction shapes = %d, horizontal bars = %d; want one bar-only shape", shapeCount, barCount)
	}
}

func TestMarkdownVisibilityAndOrphanCleanupUseHTMLChildren(t *testing.T) {
	world := newUIWorld(t)
	entity := SpawnMarkdown(world, MarkdownOptions{Width: 200, Value: "**visible**"})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	children := markdownHTMLChildren(world, entity)
	SetMarkdownVisible(world, entity, false)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	for _, child := range children {
		visibility, ok := world.Get[scene.Visibility](child)
		if !ok || visibility.Visible {
			t.Fatalf("Markdown HTML child %d remained visible", child)
		}
	}

	world.Despawn(entity)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	for _, child := range children {
		if world.IsAlive(child) {
			t.Fatalf("orphan Markdown HTML child %d is still alive", child)
		}
	}
}

func TestMarkdownSyncDoesNotAllocateAfterWarmup(t *testing.T) {
	world := newUIWorld(t)
	SpawnMarkdown(world, MarkdownOptions{Width: 240, Value: "**stable** markdown"})
	syncSystem := SyncSystem("markdown_sync")
	if err := syncSystem.Run(world); err != nil {
		t.Fatal(err)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if err := syncSystem.Run(world); err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatalf("stable Markdown sync allocated %.0f objects, want 0", allocations)
	}
}

func markdownHTMLChildren(world *ecs.World, owner ecs.Entity) []ecs.Entity {
	children := make([]ecs.Entity, 0)
	for entity, access := range world.Query[scene.Parent]().All {
		parent := access.Get()

		if parent.Entity == owner {
			children = append(children, entity)
		}
	}
	slices.Sort(children)
	return children
}
