package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/diiyw/onix/scene"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	"github.com/diiyw/onix/text/piecetree"
)

var reviewSelectionSink selectionRect

func BenchmarkReviewSelection(b *testing.B) {
	for _, n := range []int{100, 10000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			original := strings.Repeat("some text line\n", n)
			s := piecetree.FromString(original)
			s = s.Replace(s.Len()/2, s.Len()/2+1, "X")
			text := scene.NewText2D("")
			text.Source = s
			text.Font = notosanssc.NotoSansSCRegular
			if scene.TextLineCount2D(&text) != n+1 {
				b.Fatal("line count")
			}
			_, _, rect, ok := scene.TextLineRange2D(&text, n-21)
			if !ok {
				b.Fatal("line")
			}
			transform := scene.UITransform{}
			transform.Translation.Y.Value = -rect.Y
			count := s.Runes()
			rows := 0
			for range textSelectionRects(&text, transform, s, 0, count, rect.Height*21) {
				rows++
				if rows == 20 {
					break
				}
			}
			if rows != 20 {
				b.Fatal("selection")
			}
			b.ReportAllocs()
			for b.Loop() {
				rows := 0
				for r := range textSelectionRects(&text, transform, s, 0, count, rect.Height*21) {
					reviewSelectionSink = r
					rows++
					if rows == 20 {
						break
					}
				}
			}
		})
	}
	b.Run("edited-long-line", func(b *testing.B) {
		s := piecetree.FromString(strings.Repeat("a", 8192))
		s = s.Replace(4096, 4097, "b")
		text := scene.NewText2D("")
		text.Source = s
		text.Font = notosanssc.NotoSansSCRegular
		scene.TextLineCount2D(&text)
		transform := scene.UITransform{}
		for r := range textSelectionRects(&text, transform, s, 0, s.Runes(), 100) {
			reviewSelectionSink = r
		}
		b.ReportAllocs()
		for b.Loop() {
			for r := range textSelectionRects(&text, transform, s, 0, s.Runes(), 100) {
				reviewSelectionSink = r
			}
		}
	})
}
