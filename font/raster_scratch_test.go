package font

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"runtime"
	"testing"
)

func TestGlyphRasterizationRecordedPixels(t *testing.T) {
	face, err := Load("../testdata/fonts/Noto_Sans_SC/static/NotoSansSC-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	defer face.Close()
	hash := sha256.New()
	for _, character := range "A渲中éÅ" {
		id := face.GlyphIndex(character)
		for _, size := range []float64{12, 18, 48, 100} {
			scaled, err := NewFace(face, &FaceOptions{Size: size, DPI: 72})
			if err != nil {
				t.Fatal(err)
			}
			for phase := uint8(0); phase < 4; phase++ {
				gray, err := face.RasterizeGlyphAt(id, size, phase, (phase+1)&3)
				if err != nil {
					t.Fatal(err)
				}
				fmt.Fprintf(hash, "gray:%U/%g/%d:%v:%g;", character, size, phase, gray.Bounds, gray.Advance)
				hash.Write(gray.Image.Pix)
				bounds, lcd, advance, ok := scaled.GlyphLCD(character, phase)
				if !ok {
					t.Fatal("missing LCD mask")
				}
				fmt.Fprintf(hash, "lcd:%U/%g/%d:%v:%d;", character, size, phase, bounds, advance)
				if lcd != nil {
					hash.Write(lcd.Pix)
				}
			}
		}
		runtime.GC()
	}
	// Recorded from the implementation before scratch pooling and direct
	// curve appends. Includes composite outlines, wide LCD masks and all phases.
	const expected = "0aa6b7725bb922a86085bda2b3ead5cc0485172a4f6d62f463a3b9846c60dd41"
	if actual := fmt.Sprintf("%x", hash.Sum(nil)); actual != expected {
		t.Fatalf("glyph geometry or pixels changed: got %s, want %s", actual, expected)
	}
}

func TestGlyphRasterScratchDoesNotOverwriteReturnedMasks(t *testing.T) {
	face := loadTestFace(t)
	scaled, err := NewFace(face, &FaceOptions{Size: 18, DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	gray, err := face.RasterizeGlyphAt(face.GlyphIndex('渲'), 18, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	_, lcd, _, ok := scaled.GlyphLCD('渲', 1)
	if !ok || lcd == nil {
		t.Fatal("missing LCD glyph")
	}
	wantGray, wantLCD := bytes.Clone(gray.Image.Pix), bytes.Clone(lcd.Pix)
	for index := range 32 {
		if _, err := face.RasterizeGlyphAt(face.GlyphIndex('A'), float64(12+index), uint8(index&3), 0); err != nil {
			t.Fatal(err)
		}
		scaled.GlyphLCD('中', uint8(index&3))
	}
	runtime.GC()
	if !bytes.Equal(gray.Image.Pix, wantGray) || !bytes.Equal(lcd.Pix, wantLCD) {
		t.Fatal("scratch reuse changed a returned glyph bitmap")
	}
}

func TestSimpleGlyphRejectsExcessiveFlagRepeat(t *testing.T) {
	// One contour ending at point zero, no instructions, then two repeated flags.
	data := make([]byte, 6)
	binary.BigEndian.PutUint16(data[:2], 0)
	data[4], data[5] = flagRepeat|flagOnCurve, 1
	if _, err := parseSimpleGlyph(data, 1); err == nil {
		t.Fatal("flag repeat beyond the declared point count was accepted")
	}
}

func TestGlyphRasterScratchDropsOversizedStorage(t *testing.T) {
	scratch := acquireGlyphRasterScratch()
	scratch.mask = make([]byte, glyphRasterScratchByteLimit+1)
	releaseGlyphRasterScratch(scratch)
	if scratch.mask != nil || scratch.contours != nil {
		t.Fatal("oversized glyph scratch remained pooled")
	}
}

func BenchmarkGlyphRasterization(b *testing.B) {
	for _, character := range []rune{'A', '渲'} {
		b.Run(string(character), func(b *testing.B) {
			face, err := Load("../testdata/fonts/Noto_Sans_SC/static/NotoSansSC-Regular.ttf")
			if err != nil {
				b.Fatal(err)
			}
			defer face.Close()
			id := face.GlyphIndex(character)
			outline, err := face.Outline(id)
			if err != nil {
				b.Fatal(err)
			}
			scaled, err := NewFace(face, &FaceOptions{Size: 18, DPI: 72})
			if err != nil {
				b.Fatal(err)
			}
			b.Run("gray-new-size", func(b *testing.B) {
				index := 0
				b.ReportAllocs()
				for b.Loop() {
					_, err := face.RasterizeGlyph(id, 18+float64(index)/1000000)
					if err != nil {
						b.Fatal(err)
					}
					index++
				}
			})
			b.Run("lcd", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					scaled.GlyphLCD(character, 1)
				}
			})
			b.Run("decode-glyf", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := face.Glyph(id); err != nil {
						b.Fatal(err)
					}
				}
			})
			runtime.KeepAlive(outline)
		})
	}
}
