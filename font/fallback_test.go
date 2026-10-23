package font

import (
	"bytes"
	"reflect"
	"sync"
	"testing"
)

const fallbackFixture = "../testdata/fonts/Noto_Sans_SC/static/NotoSansSC-Regular.ttf"

func fallbackFixtureFace(t *testing.T) (*Face, *fallbackSource) {
	t.Helper()
	primary, err := Load(fallbackFixture)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = primary.Close() })
	// Keep real outlines and Latin glyph IDs, but remove non-ASCII coverage.
	primary.Cmap = &cmapFormat12{Groups: []cmapFormat12Group{{StartCharCode: 'A', EndCharCode: 'A', StartGlyphID: uint32(primary.GlyphIndex('A'))}}}
	source := &fallbackSource{font: SystemFont{Path: fallbackFixture}}
	set := &fallbackSet{sources: []*fallbackSource{source}, glyphs: make(map[rune]resolvedGlyph)}
	set.once.Do(func() {})
	primary.fallback = set
	t.Cleanup(func() {
		if source.face != nil {
			_ = source.face.Close()
		}
	})
	return primary, source
}

func TestGlyphFallbackKeepsIDsLocalAndSharesOwnership(t *testing.T) {
	primary, source := fallbackFixtureFace(t)
	if source.face != nil || source.coverage != nil {
		t.Fatal("fallback loaded before a missing glyph was requested")
	}
	if face, id := primary.ResolveGlyph('A'); face != primary || id != primary.GlyphIndex('A') || source.face != nil {
		t.Fatal("Latin glyph stopped using the primary face")
	}
	face, id := primary.ResolveGlyph('中')
	if face == primary || face == nil || id == 0 || primary.GlyphIndex('中') != 0 || face.GlyphIndex('中') != id {
		t.Fatal("missing glyph did not resolve to a face-local fallback ID")
	}
	if next, nextID := primary.ResolveGlyph('文'); next != face || nextID == 0 {
		t.Fatal("characters in the same fallback did not reuse its face")
	}
	if got, id := primary.ResolveGlyph('\U0010ffff'); got != primary || id != 0 {
		t.Fatal("unsupported character did not retain the primary .notdef")
	}
	if err := primary.Close(); err != nil {
		t.Fatal(err)
	}
	if face.GlyphIndex('中') != id {
		t.Fatal("closing the primary face closed the shared fallback")
	}
}

func TestScaledGlyphFallbackMatchesOwningFace(t *testing.T) {
	primary, _ := fallbackFixtureFace(t)
	// Force distinct design units to catch accidental use of the primary scale.
	primary.Head.UnitsPerEm *= 2
	fallback, _ := primary.ResolveGlyph('中')
	for _, dpi := range []float64{72, 144} {
		options := &FaceOptions{Size: 17.5, DPI: dpi}
		mixed, _ := NewFace(primary, options)
		want, _ := NewFace(fallback, options)
		advance, ok := mixed.GlyphAdvance('中')
		wantAdvance, _ := want.GlyphAdvance('中')
		if !ok || advance != wantAdvance {
			t.Fatalf("fallback advance = %v, want %v at dpi %v", advance, wantAdvance, dpi)
		}
		bounds, boundsAdvance, ok := mixed.GlyphBounds('中')
		wantBounds, _, _ := want.GlyphBounds('中')
		if !ok || bounds != wantBounds || boundsAdvance != advance {
			t.Fatal("fallback ink bounds or advance used primary design units")
		}
		for phase := range uint8(4) {
			dot := Point26_6{X: Int26_6(phase) * 16, Y: Int26_6(phase) * 16}
			rect, mask, _, gotAdvance, ok := mixed.Glyph(dot, '中')
			wantRect, wantMask, _, _, _ := want.Glyph(dot, '中')
			if !ok || rect != wantRect || gotAdvance != advance || !reflect.DeepEqual(mask, wantMask) {
				t.Fatal("gray rendering differs from the owning fallback face")
			}
			lcdRect, lcd, lcdAdvance, ok := mixed.GlyphLCD('中', phase)
			wantLCDRect, wantLCD, _, _ := want.GlyphLCD('中', phase)
			if !ok || lcdRect != wantLCDRect || lcdAdvance != advance || !bytes.Equal(lcd.Pix, wantLCD.Pix) {
				t.Fatal("LCD rendering differs from the owning fallback face")
			}
		}
		outline, outlineAdvance, ok := mixed.GlyphOutline('中')
		wantOutline, _, _ := want.GlyphOutline('中')
		if !ok || outlineAdvance != advance || !reflect.DeepEqual(outline, wantOutline) {
			t.Fatal("outline rendering differs from the owning fallback face")
		}
		if mixed.Kern('A', '中') != 0 || mixed.Kern('中', 'A') != 0 || mixed.Kern('中', '文') != want.Kern('中', '文') {
			t.Fatal("kerning crossed face boundaries or used the wrong face")
		}
		if mixed.Metrics() == want.Metrics() {
			t.Fatal("primary line metrics were replaced by fallback metrics")
		}
	}
}

func TestGlyphFallbackCacheConcurrentAndAllocationFree(t *testing.T) {
	primary, _ := fallbackFixtureFace(t)
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			for _, r := range "A中文\U0010ffff" {
				primary.ResolveGlyph(r)
			}
		})
	}
	workers.Wait()
	scaled, _ := NewFace(primary, &FaceOptions{Size: 16})
	scaled.GlyphBounds('中')
	if allocations := testing.AllocsPerRun(100, func() {
		for _, r := range "A中文\U0010ffff" {
			primary.ResolveGlyph(r)
			scaled.GlyphAdvance(r)
			scaled.GlyphBounds(r)
		}
		scaled.Kern('中', '文')
	}); allocations != 0 {
		t.Fatalf("cached fallback measurement allocated %g times", allocations)
	}
}

func TestBMPCharacterMapDoesNotAliasSupplementaryRunes(t *testing.T) {
	cmap := &cmapFormat4{StartCodes: []uint16{'A'}, EndCodes: []uint16{'A'}, IDDeltas: []int16{1 - 'A'}, IDRangeOffset: []uint16{0}}
	if cmap.GlyphIndex('A') != 1 || cmap.GlyphIndex('\U00010041') != 0 || cmap.GlyphIndex(-1) != 0 {
		t.Fatal("format 4 aliased a supplementary rune to a BMP glyph")
	}
}
