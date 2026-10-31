package scene

import (
	"math"
	"strings"
	"testing"

	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	"github.com/diiyw/onix/text/piecetree"
)

func TestTextSelectionRectsMatchMeasuredEndpoints(t *testing.T) {
	for _, mode := range []TextWrapMode{TextWrapNone, TextWrapCharacters, TextWrapWords, TextWrapTokens} {
		for _, spacing := range []float32{0, 1.25, -1} {
			value := piecetree.FromString("AV中文🙂\talpha beta\r\nnext\n\nlast line")
			value = value.Replace(0, 0, "prefix ")
			text := NewText2D("")
			text.Source, text.Font = value, notosanssc.NotoSansSCRegular
			text.WrapMode, text.WrapWidth, text.TabWidth, text.LetterSpacing = mode, 90, 37, spacing
			if !text.ensureMeasuredLayout() {
				t.Fatal("layout")
			}
			for _, selection := range [][2]int{{0, value.Runes()}, {2, 14}, {value.Runes() - 1, 1}, {-10, 900}, {3, 3}} {
				start, end := min(selection[0], selection[1]), max(selection[0], selection[1])
				lo, hi := value.ByteOffset(max(start, 0)), value.ByteOffset(end)
				index := 0
				for rect := range TextSelectionRects2D(&text, value, selection[0], selection[1], 0, 0) {
					for index < len(text.lines) && (max(lo, text.lines[index].start) >= min(hi, text.lines[index].end)) {
						index++
					}
					if index == len(text.lines) {
						t.Fatal("unexpected selection row")
					}
					line := text.lines[index]
					left, _, _ := MeasureTextValue2D(&text, value.Slice(line.start, max(lo, line.start)))
					right, _, _ := MeasureTextValue2D(&text, value.Slice(line.start, min(hi, line.end)))
					want := TextRangeRect2D{X: line.x + left, Y: line.y, Width: max(right-left, 1), Height: line.height}
					if math.Abs(float64(rect.X-want.X)) > .001 || math.Abs(float64(rect.Width-want.Width)) > .001 || rect.Y != want.Y || rect.Height != want.Height {
						t.Fatalf("mode=%d spacing=%g selection=%v: rect=%+v want=%+v", mode, spacing, selection, rect, want)
					}
					index++
				}
				for ; index < len(text.lines); index++ {
					if max(lo, text.lines[index].start) < min(hi, text.lines[index].end) {
						t.Fatal("selected row was skipped")
					}
				}
			}
			if value.Materialized() {
				t.Fatal("selection materialized the document")
			}
		}
	}
}

func TestTextSelectionViewportAndWarmAllocations(t *testing.T) {
	value := piecetree.FromString(strings.Repeat("AV中文🙂\r\n", 10000))
	value = value.Replace(0, 0, "first\n")
	text := NewText2D("")
	text.Source, text.Font = value, notosanssc.NotoSansSCRegular
	if !text.ensureMeasuredLayout() {
		t.Fatal("layout")
	}
	top, bottom := text.lines[9000].y, text.lines[9020].y
	check := func() {
		count := 0
		for rect := range TextSelectionRects2D(&text, value, 0, value.Runes(), top, bottom) {
			if rect.Y < top || rect.Y >= bottom {
				panic("offscreen selection")
			}
			count++
		}
		if count != 20 {
			panic("visible rows changed")
		}
	}
	check()
	if allocs := testing.AllocsPerRun(100, check); allocs != 0 {
		t.Fatalf("visible selection allocated %g times", allocs)
	}
}

func TestTextSelectionKeepsInlineObjectsAtomic(t *testing.T) {
	text := NewText2D("leftXYZright")
	text.Font = notosanssc.NotoSansSCRegular
	text.InlineObjects = []TextInlineObject2D{{Start: 4, End: 7, Width: 30, Height: 20, Baseline: 15}}
	if !text.ensureMeasuredLayout() || len(text.inlineObjectLayouts) != 1 {
		t.Fatal("inline object layout")
	}
	object := text.inlineObjectLayouts[0].rect
	count := 0
	for rect := range TextSelectionRects2D(&text, text.Text(), 5, 6, 0, 0) {
		if rect.X != object.X || rect.Width != object.Width {
			t.Fatalf("selection split an atomic object: %+v; object=%+v", rect, object)
		}
		count++
	}
	if count != 1 {
		t.Fatal("inline selection row was skipped")
	}
}
