package scene

import (
	"testing"

	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
)

func TestMeasureTextLineOffsets2D(t *testing.T) {
	text := NewText2D("native document\nsecond line")
	text.Font = notosanssc.NotoSansSCRegular
	text.TabWidth, text.LetterSpacing = 32, .25
	const value = "\tAV你好\r\tend"
	var offsets []int
	for index := range value {
		offsets = append(offsets, index)
	}
	offsets = append(offsets, len(value), len(value))
	widths := make([]float32, len(offsets))
	if !MeasureTextLineOffsets2D(&text, value, offsets, widths) {
		t.Fatal("could not measure UTF-8 offsets")
	}
	for index, offset := range offsets {
		want, _, ok := MeasureTextValue2D(&text, value[:offset])
		if !ok || widths[index] != want {
			t.Fatalf("offset %d: width=%g want=%g", offset, widths[index], want)
		}
	}
	if allocations := testing.AllocsPerRun(100, func() {
		MeasureTextLineOffsets2D(&text, value, offsets, widths)
	}); allocations != 0 {
		t.Fatalf("measuring cached offsets allocated %g times", allocations)
	}
	for _, invalid := range [][]int{{-1}, {len(value) + 1}, {4}, {2, 1}} {
		if MeasureTextLineOffsets2D(&text, value, invalid, widths) {
			t.Fatalf("accepted invalid offsets %v", invalid)
		}
	}
	if MeasureTextLineOffsets2D(&text, "a\nb", []int{3}, widths) || MeasureTextLineOffsets2D(&text, value, offsets, nil) {
		t.Fatal("accepted a multiline value or insufficient output storage")
	}
}
