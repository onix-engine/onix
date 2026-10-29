package main

import (
	"reflect"
	"testing"

	"github.com/diiyw/onix/font"
)

func TestFlappyFontSubsetPreservesGlyphs(t *testing.T) {
	subset, err := font.Load("resources/presentation/fonts/ui.ttf")
	if err != nil {
		t.Fatal(err)
	}
	defer subset.Close()
	original, err := font.Load("../../testdata/fonts/Noto_Sans_SC/static/NotoSansSC-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	defer original.Close()
	scaled, _ := font.NewFace(subset, &font.FaceOptions{Size: 26})
	wantScaled, _ := font.NewFace(original, &font.FaceOptions{Size: 26})
	if scaled.Metrics() != wantScaled.Metrics() {
		t.Fatal("font subset changed line metrics")
	}
	for character := rune(0x20); character <= 0x7e; character++ {
		id := subset.GlyphIndex(character)
		if id == 0 {
			t.Fatalf("font subset is missing %q", character)
		}
		outline, err := subset.Outline(id)
		if err != nil {
			t.Fatal(err)
		}
		want, err := original.Outline(original.GlyphIndex(character))
		if err != nil {
			t.Fatal(err)
		}
		advance, _ := scaled.GlyphAdvance(character)
		wantAdvance, _ := wantScaled.GlyphAdvance(character)
		if advance != wantAdvance || !reflect.DeepEqual(outline, want) {
			t.Fatalf("font subset changed the shape or advance of %q", character)
		}
	}
}
