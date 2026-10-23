package css

import (
	"fmt"
	"testing"

	"github.com/diiyw/onix/html/dom"
)

func BenchmarkMatchContextStorage(b *testing.B) {
	for _, count := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			parent := dom.NewElement("section", nil)
			for range count {
				parent.AppendChild(dom.NewElement("div", map[string]string{"class": "item"}))
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
