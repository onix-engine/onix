package dom

import (
	"fmt"
	"strings"
	"testing"
)

var attributeNodeSink *Node
var attributeValueSink string

func BenchmarkDOMAttributes(b *testing.B) {
	for _, count := range []int{0, 1, 3, 8, 32, 128} {
		names := make([]string, count)
		for i := range names {
			names[i] = fmt.Sprintf("data-%03d", i)
		}
		populate := func() *Node {
			node := &Node{Type: ElementNode, Data: "div"}
			for _, name := range names {
				node.SetAttr(name, "value")
			}
			return node
		}
		b.Run(fmt.Sprintf("%d/create", count), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				attributeNodeSink = populate()
			}
		})
		if count == 0 {
			continue
		}
		b.Run(fmt.Sprintf("%d/lookup", count), func(b *testing.B) {
			node := populate()
			b.ReportAllocs()
			for b.Loop() {
				first, ok := node.Attr(names[0])
				last, found := node.Attr(names[count-1])
				_, missing := node.Attr("missing")
				if !ok || !found || missing || first != last {
					b.Fatal("attribute lookup failed")
				}
				attributeValueSink = last
			}
		})
		b.Run(fmt.Sprintf("%d/mutate", count), func(b *testing.B) {
			node := populate()
			node.SetAttr("hidden", "")
			node.RemoveAttr("hidden")
			b.ReportAllocs()
			for b.Loop() {
				node.SetAttr(names[count-1], "changed")
				node.SetAttr(names[count-1], "value")
				node.SetAttr("hidden", "")
				node.RemoveAttr("hidden")
			}
		})
	}
}

func BenchmarkDOMAttributeParsing(b *testing.B) {
	source := strings.Repeat(`<div id="item" class="card" style="color:red" data-x="a&amp;b"><input type="checkbox" checked></div>`, 1000)
	b.Run("fresh", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			root, err := Parse(source)
			if err != nil {
				b.Fatal(err)
			}
			attributeNodeSink = root
		}
	})
	b.Run("reuse", func(b *testing.B) {
		var parser Parser
		if _, err := parser.Parse(source); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		for b.Loop() {
			root, err := parser.Parse(source)
			if err != nil {
				b.Fatal(err)
			}
			attributeNodeSink = root
		}
	})
	b.Run("tokens", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			tokenizer := NewTokenizer(source)
			for {
				token, err := tokenizer.Next()
				if err != nil {
					b.Fatal(err)
				}
				if token.Type == TokenEOF {
					break
				}
			}
		}
	})
}
