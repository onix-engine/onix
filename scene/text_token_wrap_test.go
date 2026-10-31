package scene

import (
	"math"
	"slices"
	"testing"
	"unicode/utf8"

	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	"github.com/diiyw/onix/text/piecetree"
)

func TestTokenWrapPreservesWordsWhitespaceAndNewlines(t *testing.T) {
	for _, fixture := range []struct {
		value string
		lines []string
	}{
		{"hello_世界42(alpha, e\u0301clair);\r\n\n", []string{"hello_世界42", "(", "alpha", ", ", "e\u0301clair", ")", ";\r", "", ""}},
		{"\tlong_identifier\t next_word  ", []string{"\t", "long_identifier\t ", "next_word  "}},
		{"\t  \r\n", []string{"\t  \r", ""}},
	} {
		text := NewText2D(fixture.value)
		text.Font = notosanssc.NotoSansSCRegular
		text.WrapMode, text.WrapWidth, text.TabWidth = TextWrapTokens, 1, 30
		if !text.ensureMeasuredLayout() {
			t.Fatal("layout failed")
		}
		var got []string
		for _, line := range text.lines {
			got = append(got, text.Value[line.start:line.end])
		}
		if !slices.Equal(got, fixture.lines) {
			t.Fatalf("%q: lines=%q want=%q", fixture.value, got, fixture.lines)
		}
	}
}

func TestTokenWrapMetricsCaretAndCache(t *testing.T) {
	text := NewText2D("prefix extraordinarily_long_identifier suffix\nAV界(foo, bar)")
	text.Font = notosanssc.NotoSansSCRegular
	text.WrapMode, text.WrapWidth = TextWrapTokens, 65
	text.LetterSpacing, text.TabWidth = 1.5, 30
	if !text.ensureMeasuredLayout() {
		t.Fatal("layout failed")
	}
	for _, line := range text.lines {
		reference := NewText2D(text.Value[line.start:line.end])
		reference.Font, reference.WrapMode = text.Font, TextWrapNone
		reference.LetterSpacing, reference.TabWidth = text.LetterSpacing, text.TabWidth
		want, _, _ := MeasureText2D(&reference)
		if math.Abs(float64(line.width-want)) > .01 {
			t.Fatalf("%q: line width=%g want=%g", text.Value[line.start:line.end], line.width, want)
		}
	}
	_, startY, _, _, startOK := TextCaretRect2D(&text, text.Value, len("prefix ")+1)
	_, endY, _, _, endOK := TextCaretRect2D(&text, text.Value, len("prefix extraordinarily_long_identifier")-1)
	if !startOK || !endOK || startY != endY || startY == 0 {
		t.Fatal("long identifier did not move intact onto the next visual line")
	}
	for caret := range utf8.RuneCountInString(text.Value) + 1 {
		x, y, _, height, ok := TextCaretRect2D(&text, text.Value, caret)
		// A shared soft-wrap boundary belongs to the preceding line.
		if !ok || TextCaretIndex2D(&text, text.Value, x, y+height/2) != caret {
			t.Fatalf("caret %d did not survive hit testing", caret)
		}
	}
	if allocs := testing.AllocsPerRun(100, func() {
		text.wrapPreservedRanges(piecetree.FromString(text.Value), nil, text.WrapWidth, false)
	}); allocs != 0 {
		t.Fatalf("token wrapping allocated %g times after warmup", allocs)
	}
}

func TestTokenWrapKeepsInlineObjectsAtomic(t *testing.T) {
	text := NewText2D("leftXYZright")
	text.Font = notosanssc.NotoSansSCRegular
	text.WrapMode, text.WrapWidth = TextWrapTokens, 1
	text.InlineObjects = []TextInlineObject2D{{Start: 4, End: 7, Width: 30, Height: 20, Baseline: 15}}
	if !text.ensureMeasuredLayout() || len(text.lines) != 3 || len(text.inlineObjectLayouts) != 1 {
		t.Fatalf("inline object layout failed: %v", text.lines)
	}
	if text.lines[1].start != 4 || text.lines[1].end != 7 {
		t.Fatalf("inline object split: %v", text.lines)
	}
}
