package scene

import (
	"fmt"
	"strings"
	"testing"

	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	"github.com/diiyw/onix/text/piecetree"
)

var reviewRuneSink rune

func BenchmarkReviewSnapshotScan(b *testing.B) {
	s := piecetree.FromString(strings.Repeat("some text 中文 line\n", 32768))
	s = s.Replace(s.Len()/2, s.Len()/2, "x")
	b.Run("DecodeRune", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var sum rune
			for i := 0; i < s.Len(); {
				r, n := s.DecodeRune(i)
				sum += r
				i += n
			}
			reviewRuneSink = sum
		}
	})
	b.Run("Cursor", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var sum rune
			cursor := s.Cursor(0, s.Len())
			for cursor.Offset() < s.Len() {
				r, size := cursor.DecodeRune()
				sum += r
				cursor.Advance(size)
			}
			reviewRuneSink = sum
		}
	})
	b.Run("Chunks", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var sum rune
			for part := range s.Chunks(0, s.Len()) {
				for _, r := range part {
					sum += r
				}
			}
			reviewRuneSink = sum
		}
	})
}
func BenchmarkReviewWrap(b *testing.B) {
	for _, n := range []int{256, 1024} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			s := piecetree.FromString(strings.Repeat("word ", n))
			s = s.Replace(s.Len()/2, s.Len()/2, "x")
			for _, fragmented := range []bool{false, true} {
				b.Run(fmt.Sprint(fragmented), func(b *testing.B) {
					value := s
					if !fragmented {
						value = piecetree.FromString(s.String())
					}
					t := NewText2D("")
					t.Font = notosanssc.NotoSansSCRegular
					t.WrapMode = TextWrapWords
					t.WrapWidth = 1e8
					if !t.ensureFace() {
						b.Fatal("face")
					}
					t.wrapRanges(value, nil)
					b.ReportAllocs()
					for b.Loop() {
						t.wrapRanges(value, nil)
					}
				})
			}
		})
	}
}
