package layout

import "testing"

func TestCaretIndexForTextFragmentsUsesNearestGlyphCaretPosition(t *testing.T) {
	fragments := []TextFragment{{
		Text: "abcd",
		Rect: Rect{X: 10, Y: 0, Width: 40, Height: 20},
		Glyphs: []GlyphFragment{
			{Rune: 'a', X: 10, Advance: 10},
			{Rune: 'b', X: 20, Advance: 10},
			{Rune: 'c', X: 30, Advance: 10},
			{Rune: 'd', X: 40, Advance: 10},
		},
	}}

	for _, tt := range []struct {
		name string
		x    float64
		want int
	}{
		{name: "first glyph left half", x: 14, want: 0},
		{name: "first glyph right half", x: 16, want: 1},
		{name: "second glyph left half", x: 24, want: 1},
		{name: "second glyph right half", x: 26, want: 2},
		{name: "third glyph left half", x: 34, want: 2},
		{name: "third glyph right half", x: 36, want: 3},
		{name: "fourth glyph left half", x: 44, want: 3},
		{name: "fourth glyph right half", x: 46, want: 4},
		{name: "after text", x: 60, want: 4},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := CaretIndexForTextFragments(fragments, tt.x)
			if !ok {
				t.Fatal("caret lookup failed")
			}
			if got != tt.want {
				t.Fatalf("caret index = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCaretIndexForTextFragmentsUsesGlyphPositionsWithinWholeFragment(t *testing.T) {
	fragments := []TextFragment{{
		Text: "abcd",
		Rect: Rect{X: 10, Y: 0, Width: 40, Height: 20},
		Glyphs: []GlyphFragment{
			{Rune: 'a', X: 10, Advance: 40},
			{Rune: 'b', X: 20, Advance: 40},
			{Rune: 'c', X: 30, Advance: 40},
			{Rune: 'd', X: 40, Advance: 40},
		},
	}}

	got, ok := CaretIndexForTextFragments(fragments, 34)
	if !ok {
		t.Fatal("caret lookup failed")
	}
	if got != 2 {
		t.Fatalf("caret index = %d, want 2", got)
	}

	caretX, ok := CaretXForTextFragments(fragments, 2)
	if !ok {
		t.Fatal("caret x lookup failed")
	}
	if caretX != 30 {
		t.Fatalf("caret x = %v, want 30", caretX)
	}
}

func TestFallbackCaretLookupWarmPathDoesNotAllocate(t *testing.T) {
	fragments := []TextFragment{{
		Text: "A界B",
		Rect: Rect{X: 10, Width: 30, Height: 20},
	}}
	allocations := testing.AllocsPerRun(100, func() {
		if _, ok := CaretXForTextFragments(fragments, 2); !ok {
			t.Fatal("caret x lookup failed")
		}
		if _, ok := CaretIndexForTextFragments(fragments, 26); !ok {
			t.Fatal("caret index lookup failed")
		}
	})
	if allocations != 0 {
		t.Fatalf("fallback caret lookup allocated %.0f objects, want 0", allocations)
	}
}
