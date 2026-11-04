package style

import "testing"

func BenchmarkInlineStyleCache(b *testing.B) {
	const input = "width:120px;height:30px;color:#123456;transition:color .2s;animation:pulse 1s linear"
	for _, cached := range []bool{false, true} {
		name := "parse"
		if cached {
			name = "hit"
		}
		b.Run(name, func(b *testing.B) {
			run := func() {
				var values []inlineDeclaration
				var err error
				if cached {
					values, err = inlineStyleCache.parse(input)
				} else {
					values, _, err = parseInlineDeclarations(input)
				}
				if err != nil || len(values) != 5 {
					b.Fatalf("inline parsing failed: %v", err)
				}
			}
			run()
			b.ReportAllocs()
			for b.Loop() {
				run()
			}
		})
	}
}
