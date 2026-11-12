package style

import (
	"testing"

	"github.com/onix-engine/onix/html/css"
	"github.com/onix-engine/onix/html/dom"
)

var cascadeMapScratchSink *cascadeScratch

func BenchmarkCascadeWorkspacePopulation(b *testing.B) {
	sheet, err := css.Parse(`.item{display:flex;width:100px;height:20px;color:red;background-color:white;margin:2px;padding:3px;border:1px solid black;opacity:.9;position:relative;top:1px;left:2px;gap:4px;flex-grow:1;font-size:16px;font-weight:700}`)
	if err != nil {
		b.Fatal(err)
	}
	node := dom.NewElement("div", dom.Attribute{Name: "class", Value: "item"})
	compiled := compileStylesheet(sheet)
	b.ReportAllocs()
	for b.Loop() {
		scratch := cascadeScratchPool.New().(*cascadeScratch)
		scratch.compiled = compiled
		if _, err := collectSpecifiedValuesInto(node, sheet, nil, nil, scratch); err != nil {
			b.Fatal(err)
		}
		cascadeMapScratchSink = scratch
	}
}
