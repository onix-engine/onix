package markup_test

import (
	"strings"
	"testing"

	"github.com/onix-engine/onix/html/markup"
	"github.com/onix-engine/onix/html/parse"
	"github.com/onix-engine/onix/scene/node"
)

func TestNativeAndSerializedTrees(t *testing.T) {
	var b markup.Builder
	b.Open("section")
	field := b.TextElement("textarea", "\n</textarea>&\r", "title", `"<&`)
	b.Element("img", "alt", "")
	b.End()
	if field.Parent != b.Document().Children[0] || field.Children[0].Parent != field {
		t.Fatal("native tree lost parentage")
	}
	wire, subtree := b.StringWithSubtree(field)
	if wire != markup.HTML(b.Document()) || subtree != markup.HTML(field) || strings.Contains(wire, "</img>") {
		t.Fatal(wire, subtree)
	}
	tree, err := parse.ParseTree(wire)
	if err != nil {
		t.Fatal(err)
	}
	got := tree.Children[0].Children[0]
	if len(got.Children) != 1 || got.Children[0].Data != field.Children[0].Data || got.AttrValue("title") != field.AttrValue("title") {
		t.Fatalf("round trip changed authored data: %s", wire)
	}
	b.Reset()
	b.Open("aside")
	b.Embed(subtree)
	b.Text(subtree)
	b.End()
	embedded, err := parse.ParseTree(b.String())
	if err != nil {
		t.Fatal(err)
	}
	children := embedded.Children[0].Children
	if len(children) != 2 || children[0].Data != "textarea" || children[1].Type != node.TextNode || children[1].Data != subtree {
		t.Fatal("cached fragment or literal text changed", b.String())
	}
}

func TestBuilderReusesNodesAndAttributes(t *testing.T) {
	var b markup.Builder
	build := func() {
		b.Reset()
		b.Open("form", "class", "form")
		b.TextElement("button", "Save", "data-ref", "save", "type", "submit")
		b.End()
	}
	if allocs := testing.AllocsPerRun(100, build); allocs != 0 {
		t.Fatalf("warm tree construction allocated %g times", allocs)
	}
}
