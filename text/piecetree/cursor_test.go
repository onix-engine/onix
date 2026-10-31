package piecetree

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCursorMatchesUTF8RangesAndCopies(t *testing.T) {
	for _, original := range []string{
		strings.Repeat("abc中文🙂\r\n", 700),
		strings.Repeat("\xff\xe4\xb8\xad\xc0\x80a", 700),
	} {
		for _, segmented := range []bool{false, true} {
			value := FromString(original)
			want := original
			if segmented {
				value = value.Replace(0, 0, "head🙂")
				want = "head🙂" + original
			}
			for _, bounds := range [][2]int{{0, value.Len()}, {1, value.Len() - 1}, {700, 2200}, {0, 0}} {
				start, end := bounds[0], bounds[1]
				cursor := value.Cursor(start, end)
				for index, r := range want[start:end] {
					if cursor.Offset() != start+index || cursor.Byte() != want[start+index] {
						t.Fatal("cursor lost its byte position")
					}
					got, size := cursor.DecodeRune()
					_, width := utf8.DecodeRuneInString(want[start+index : end])
					if got != r || size != width {
						t.Fatalf("at %d: %q/%d; want %q/%d", start+index, got, size, r, width)
					}
					cursor.Advance(size)
				}
				if _, size := cursor.DecodeRune(); size != 0 || cursor.Offset() != end {
					t.Fatal("cursor did not reach the end")
				}
			}
			cursor := value.Cursor(0, value.Len())
			copy := cursor
			cursor.Advance(value.Len() - 3)
			if copy.Offset() != 0 || cursor.Byte() != want[value.Len()-3] {
				t.Fatal("cursor copies share mutable state or skipped incorrectly")
			}
		}
	}
}

func TestSnapshotEqualRangeAndCursorAllocations(t *testing.T) {
	original := strings.Repeat("text 中文🙂\n", 2000)
	base := FromString(original)
	edited := base.Replace(4, 4, "changed")
	if !base.EqualRange(4, base.Len(), edited, 11, edited.Len()) || base.EqualRange(0, 4, edited, 7, 11) {
		t.Fatal("range comparison missed shared pieces or changed bytes")
	}
	if allocs := testing.AllocsPerRun(100, func() {
		cursor := edited.Cursor(0, edited.Len())
		for cursor.Offset() < edited.Len() {
			_, size := cursor.DecodeRune()
			cursor.Advance(size)
		}
		if !base.EqualRange(4, base.Len(), edited, 11, edited.Len()) {
			panic("range changed")
		}
	}); allocs != 0 {
		t.Fatalf("cursor and range reads allocate %g times", allocs)
	}
	if edited.Materialized() {
		t.Fatal("reading materialized a segmented snapshot")
	}
}

func TestCursorDecodesRuneJoinedAcrossPieces(t *testing.T) {
	value := FromString("\xe4x\xb8\xad").Replace(1, 2, "")
	cursor := value.Cursor(0, value.Len())
	if r, size := cursor.DecodeRune(); r != '中' || size != 3 {
		t.Fatalf("joined rune: %q/%d", r, size)
	}
	cursor.Advance(3)
	if _, size := cursor.DecodeRune(); size != 0 {
		t.Fatal("joined rune did not reach the end")
	}
}
