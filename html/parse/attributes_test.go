package parse

import (
	"fmt"
	"strings"
	"testing"
)

func TestParserLargeFormattingAttributesReuse(t *testing.T) {
	var source strings.Builder
	source.WriteString("<b")
	for index := range 32 {
		fmt.Fprintf(&source, " data-%02d=value", index)
	}
	source.WriteString("><p>bold<i>italic</b>after</i>")
	var parser Parser
	root, err := parser.Parse(source.String())
	if err != nil {
		t.Fatal(err)
	}
	want := root.String()
	if allocations := testing.AllocsPerRun(100, func() {
		if _, err := parser.Parse(source.String()); err != nil {
			t.Fatal(err)
		}
	}); allocations != 0 {
		t.Fatalf("large formatting attributes allocated after reuse: %g", allocations)
	}
	if parser.builder.doc.String() != want {
		t.Fatal("formatting reconstruction changed the attributes")
	}
}

func TestParserAttributeMergingAndForeignNames(t *testing.T) {
	root, err := Parse(`<!doctype html><html id=first><html id=second lang=en><body class=first><body class=second title=extra><svg Z=last viewbox="0 0 1 1" xlink:href="#x" href="#y"><path/></svg>`)
	if err != nil {
		t.Fatal(err)
	}
	html := root.Children[0]
	body := html.Children[1]
	svg := body.Children[0]
	if html.AttrValue("id") != "first" || html.AttrValue("lang") != "en" || body.AttrValue("class") != "first" || body.AttrValue("title") != "extra" {
		t.Fatal("repeated html/body tags overwrote existing attributes or lost new ones")
	}
	if svg.AttrValue("viewBox") != "0 0 1 1" || svg.AttrValue("xlink:href") != "#x" || svg.AttrValue("href") != "#y" || svg.AttrValue("z") != "last" {
		t.Fatal("foreign canonical or qualified attribute names changed")
	}
}

func TestTokenizerAttributesRemainIndependent(t *testing.T) {
	tokenizer := NewTokenizer(`<div class=first><div class=second>`)
	first, _ := tokenizer.Next()
	second, _ := tokenizer.Next()
	second.SetAttr("class", "changed")
	if first.AttrValue("class") != "first" || second.AttrValue("class") != "changed" {
		t.Fatal("tokenizer reused a previously returned token's attributes")
	}
}

func TestDuplicateAttributesKeepFirstAcrossTokenSizes(t *testing.T) {
	var parser Parser
	for _, count := range []int{8, 9, 2} {
		var source strings.Builder
		source.WriteString("<div ID=first")
		for i := 0; i < count-2; i++ {
			fmt.Fprintf(&source, " data-%d=x", i)
		}
		source.WriteString(" id=last>")
		root, err := parser.Parse(source.String())
		if err != nil {
			t.Fatal(err)
		}
		node := root.Children[0]
		if node.AttrCount() != count-1 || node.AttrValue("id") != "first" {
			t.Fatalf("%d attributes: %s", count, root.String())
		}
	}
}
