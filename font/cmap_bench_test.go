package font

import "testing"

func BenchmarkCharacterMap(b *testing.B) {
	face, err := Load("../testdata/fonts/Noto_Sans_SC/static/NotoSansSC-Regular.ttf")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = face.Close() })
	for _, direct := range []bool{false, true} {
		name := "face"
		if direct {
			name = "table"
		}
		b.Run(name, func(b *testing.B) {
			for r := rune(0x4e00); r < 0x6e00; r++ {
				face.GlyphIndex(r)
			}
			b.ReportAllocs()
			var i uint32
			for b.Loop() {
				r := rune(0x4e00 + i%8192)
				if direct {
					face.Cmap.GlyphIndex(r)
				} else {
					face.GlyphIndex(r)
				}
				i++
			}
		})
	}
}
