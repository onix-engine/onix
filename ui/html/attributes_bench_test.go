package html

import (
	"strings"
	"testing"

	"github.com/onix-engine/onix/html/dom"
)

var attributeCloneSink *dom.Node
var attributeHTMLSink string

func BenchmarkHTMLAttributeTemplates(b *testing.B) {
	root, err := dom.Parse(strings.Repeat(`<div id="item" class="card" style="color:red" data-x="a&amp;b">text</div>`, 1000))
	if err != nil {
		b.Fatal(err)
	}
	b.Run("clone", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			attributeCloneSink = cloneHTMLNode(root, nil)
		}
	})
	b.Run("serialize", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			attributeHTMLSink = serializeHTMLNode(root)
		}
	})
}
