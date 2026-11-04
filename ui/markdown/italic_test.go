package markdown

import (
	"github.com/diiyw/onix/scene"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	engineui "github.com/diiyw/onix/ui"
	"testing"
)

func TestMarkdownItalicAndBoldItalic(t *testing.T) {
	world := newUIWorld(t)
	style := DefaultMarkdownStyle(engineui.DefaultTheme())
	style.Font = notosanssc.NotoSansSCRegular
	style.BoldFont = notosanssc.NotoSansSCBold
	owner := SpawnMarkdown(world, MarkdownOptions{Width: 300, Value: "*italic* and ***both***", Style: &style})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	italic, both := false, false
	for _, child := range markdownHTMLChildren(world, owner) {
		text, ok := world.Get[scene.Text2D](child)
		if !ok {
			continue
		}
		if text.Value == "italic" {
			italic = text.Font == style.Font.Italic()
		}
		if text.Value == "both" {
			both = text.Font == style.BoldFont.Italic()
		}
	}
	if !italic || !both {
		t.Fatalf("italic=%v bold italic=%v", italic, both)
	}
}
