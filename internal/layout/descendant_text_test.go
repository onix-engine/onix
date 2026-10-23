package layout

import (
	"testing"

	"github.com/diiyw/onix/internal/dom"
)

func TestDescendantTextOwnership(t *testing.T) {
	root := dom.NewElement("div", nil)
	root.AppendChild(dom.NewText(""))
	root.AppendChild(dom.NewElement("img", nil))
	wrapper := dom.NewElement("span", nil)
	root.AppendChild(wrapper)
	text := dom.NewText(" 中文\tGo ")
	wrapper.AppendChild(text)
	wrapper.AppendChild(dom.NewText(""))
	if descendantText(nil) != "" || descendantText(root) != text.Data {
		t.Fatal("lost nested text or whitespace")
	}
	first := descendantText(root)
	text.Data = "updated"
	if first != " 中文\tGo " || descendantText(root) != "updated" {
		t.Fatal("text was stale after mutation")
	}
	second := dom.NewText("尾")
	root.AppendChild(second)
	joined := descendantText(root)
	if joined != "updated尾" {
		t.Fatalf("concatenated text = %q", joined)
	}
	second.Data = "!"
	if joined != "updated尾" || descendantText(root) != "updated!" {
		t.Fatal("concatenated text does not own its storage")
	}
	root.Children = nil
	if descendantText(root) != "" {
		t.Fatal("retained removed descendants")
	}
}

func BenchmarkDescendantText(b *testing.B) {
	root := dom.NewElement("div", nil)
	root.AppendChild(dom.NewElement("img", nil))
	wrapper := dom.NewElement("span", nil)
	root.AppendChild(wrapper)
	wrapper.AppendChild(dom.NewText("nested label"))
	wrapper.AppendChild(dom.NewText(""))
	b.ReportAllocs()
	for b.Loop() {
		if descendantText(root) != "nested label" {
			b.Fatal("lost label")
		}
	}
}

func TestDescendantTextSingleValueAllocations(t *testing.T) {
	root := dom.NewElement("div", nil)
	wrapper := dom.NewElement("span", nil)
	root.AppendChild(wrapper)
	wrapper.AppendChild(dom.NewText("nested label"))
	root.AppendChild(dom.NewText(""))
	if allocs := testing.AllocsPerRun(100, func() {
		if descendantText(root) != "nested label" {
			panic("lost label")
		}
	}); allocs != 0 {
		t.Fatalf("nested single text allocated %g times", allocs)
	}
}
