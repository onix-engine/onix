package layout

import (
	"testing"

	uinode "github.com/onix-engine/onix/scene/node"
)

func TestDescendantTextOwnership(t *testing.T) {
	root := uinode.NewElement("div")
	root.AppendChild(uinode.NewText(""))
	root.AppendChild(uinode.NewElement("img"))
	wrapper := uinode.NewElement("span")
	root.AppendChild(wrapper)
	text := uinode.NewText(" 中文\tGo ")
	wrapper.AppendChild(text)
	wrapper.AppendChild(uinode.NewText(""))
	if descendantText(nil) != "" || descendantText(root) != text.Data {
		t.Fatal("lost nested text or whitespace")
	}
	first := descendantText(root)
	text.Data = "updated"
	if first != " 中文\tGo " || descendantText(root) != "updated" {
		t.Fatal("text was stale after mutation")
	}
	second := uinode.NewText("尾")
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
	root := uinode.NewElement("div")
	root.AppendChild(uinode.NewElement("img"))
	wrapper := uinode.NewElement("span")
	root.AppendChild(wrapper)
	wrapper.AppendChild(uinode.NewText("nested label"))
	wrapper.AppendChild(uinode.NewText(""))
	b.ReportAllocs()
	for b.Loop() {
		if descendantText(root) != "nested label" {
			b.Fatal("lost label")
		}
	}
}

func TestDescendantTextSingleValueAllocations(t *testing.T) {
	root := uinode.NewElement("div")
	wrapper := uinode.NewElement("span")
	root.AppendChild(wrapper)
	wrapper.AppendChild(uinode.NewText("nested label"))
	root.AppendChild(uinode.NewText(""))
	if allocs := testing.AllocsPerRun(100, func() {
		if descendantText(root) != "nested label" {
			panic("lost label")
		}
	}); allocs != 0 {
		t.Fatalf("nested single text allocated %g times", allocs)
	}
}
