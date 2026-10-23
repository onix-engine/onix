package font

import (
	"encoding/binary"
	"os"
	"testing"
)

func TestGPOSNotoKerning(t *testing.T) {
	data, err := os.ReadFile("../testdata/fonts/Noto_Sans_SC/static/NotoSansSC-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	face, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	left, right := face.GlyphIndex('t'), face.GlyphIndex('a')
	// Chromium measures beta 41 font units narrower with kerning enabled.
	if got := face.GlyphKerning(face.GlyphIndex('b'), face.GlyphIndex('e')) + face.GlyphKerning(face.GlyphIndex('e'), left) + face.GlyphKerning(left, right); got != -41 {
		t.Fatalf("beta adjustment=%v", got)
	}
	if n := testing.AllocsPerRun(100, func() { face.GlyphKerning(left, right) }); n != 0 {
		t.Fatalf("kerning allocated %v", n)
	}
}

func TestPairPositionFormatsAndTruncation(t *testing.T) {
	words := []uint16{1, 18, 4, 0, 1, 12, 1, 20, 0xffd8, 1, 1, 10}
	data := make([]byte, len(words)*2)
	for i, w := range words {
		binary.BigEndian.PutUint16(data[i*2:], w)
	}
	if got, ok := pairAdvance(data, 10, 20); !ok || got != -40 {
		t.Fatalf("pair=%v,%v", got, ok)
	}
	if _, ok := pairAdvance(data, 10, 21); ok {
		t.Fatal("unlisted pair matched")
	}
	for i := 0; i < len(data); i++ {
		pairAdvance(data[:i], 10, 20)
		parsePairLookups(data[:i])
	}
}
