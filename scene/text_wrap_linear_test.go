package scene

import (
	"crypto/sha256"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/diiyw/onix/font"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	onixtext "github.com/diiyw/onix/text"
	"github.com/diiyw/onix/text/piecetree"
)

// The digest was recorded from the preceding prefix-measurement implementation.
// It covers whitespace, carriage returns, kerning, spacing and atomic objects.
func TestTextWordWrapRecordedRanges(t *testing.T) {
	// Use only the fixture font: emoji fallback metrics depend on the OS.
	face, err := font.Parse(notosanssc.NotoSansSCRegular.Data)
	if err != nil {
		t.Fatal(err)
	}
	defer face.Close()
	fixtureFont := &onixtext.Font{Face: face}
	hash := sha256.New()
	random := rand.New(rand.NewPCG(53, 97))
	for fixture := range 24 {
		var source strings.Builder
		for range 12 {
			source.WriteString([]string{"AV", "word", "中文", "🙂界", "long_identifier", "\r"}[random.IntN(6)])
			source.WriteString([]string{" ", "\t", "  ", "\t \t"}[random.IntN(4)])
		}
		value := piecetree.FromString(source.String())
		if fixture%2 != 0 {
			value = value.Replace(0, 0, "XYZ ")
		}
		for _, width := range []float32{1, 30, 80, 160, 512, 1e8} {
			for _, spacing := range []float32{0, 1.25, -.5} {
				text := NewText2D("")
				text.Font = fixtureFont
				text.WrapMode, text.WrapWidth, text.LetterSpacing, text.TabWidth = TextWrapWords, width, spacing, 37
				if !text.ensureFace() {
					t.Fatal("font face")
				}
				var objects []TextInlineObject2D
				if fixture%2 != 0 {
					objects = []TextInlineObject2D{{Start: 0, End: 3, Width: 29, Height: 20, Baseline: 15}}
				}
				for _, line := range text.wrapRanges(value, objects) {
					fmt.Fprintf(hash, "%d:%d;", line.start, line.end)
				}
				fmt.Fprintln(hash)
			}
		}
	}
	const want = "b0e7417886b910c198c0697549c018892196386e0914a943e2344b7d716dc4ec"
	if got := fmt.Sprintf("%x", hash.Sum(nil)); got != want {
		t.Fatalf("word-wrap ranges: %s; want %s", got, want)
	}
}

func BenchmarkTextParagraphWordWrap(b *testing.B) {
	value := piecetree.FromString(strings.Repeat("word ", 1024))
	value = value.Replace(value.Len()/2, value.Len()/2, "x")
	text := NewText2D("")
	text.Font, text.WrapMode, text.WrapWidth = notosanssc.NotoSansSCRegular, TextWrapWords, 640
	if !text.ensureFace() {
		b.Fatal("font face")
	}
	text.wrapRanges(value, nil)
	b.ReportAllocs()
	for b.Loop() {
		text.wrapRanges(value, nil)
	}
}
