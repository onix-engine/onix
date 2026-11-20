package css

import (
	"fmt"
	"testing"

	uinode "github.com/onix-engine/onix/scene/node"
)

func BenchmarkMatchContextStorage(b *testing.B) {
	for _, count := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			parent := uinode.NewElement("section")
			for range count {
				parent.AppendChild(uinode.NewElement("div", uinode.Attribute{Name: "class", Value: "item"}))
			}
			sheet, err := Parse(`.item:nth-child(odd) {color:red}`)
			if err != nil {
				b.Fatal(err)
			}
			selector := sheet.Rules[0].Selectors[0]
			for _, warm := range []bool{false, true} {
				name := "new"
				if warm {
					name = "reuse"
				}
				b.Run(name, func(b *testing.B) {
					ctx := NewMatchContext()
					run := func() {
						if warm {
							ctx.Reset()
						} else {
							ctx = NewMatchContext()
						}
						for _, node := range parent.Children {
							MatchWithStateAndContext(selector, node, nil, ctx)
						}
					}
					run()
					b.ReportAllocs()
					b.ResetTimer()
					for b.Loop() {
						run()
					}
				})
			}
		})
	}
}
