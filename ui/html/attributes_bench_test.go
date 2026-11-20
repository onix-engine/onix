package html

import (
	"strings"
	"testing"

	"github.com/onix-engine/onix/html/parse"
	uinode "github.com/onix-engine/onix/scene/node"
)

var attributeCloneSink *uinode.Node
var attributeHTMLSink string

func BenchmarkHTMLAttributeTemplates(b *testing.B) {
	root, err := parse.Parse(strings.Repeat(`<div id="item" class="card" style="color:red" data-x="a&amp;b">text</div>`, 1000))
	if err != nil {
		b.Fatal(err)
	}
	b.Run("clone", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var nodes uinode.Arena
			attributeCloneSink = nodes.Clone(root)
		}
	})
	b.Run("serialize", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			attributeHTMLSink = serializeHTMLNode(root)
		}
	})
}
