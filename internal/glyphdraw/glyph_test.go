package glyphdraw

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"testing"
)

func glyphPixels(bounds image.Rectangle, seed int) *image.RGBA {
	img := image.NewRGBA(bounds)
	for i := 0; i < len(img.Pix); i += 4 {
		a := (seed + i*37/4) % 256
		img.Pix[i] = byte(i * 7 % (a + 1))
		img.Pix[i+1] = byte(i * 11 % (a + 1))
		img.Pix[i+2] = byte(i * 13 % (a + 1))
		img.Pix[i+3] = byte(a)
	}
	return img
}

func TestGlyphDrawingMatchesDraw(t *testing.T) {
	srcParent := glyphPixels(image.Rect(-2, -3, 29, 24), 19)
	src := srcParent.SubImage(image.Rect(1, -1, 22, 20)).(*image.RGBA)
	alpha := image.NewAlpha(srcParent.Rect)
	for i := range alpha.Pix {
		alpha.Pix[i] = byte(i)
	}
	mask := alpha.SubImage(src.Rect).(*image.Alpha)
	dstBounds := image.Rect(-4, -6, 28, 22)
	view := image.Rect(-1, 1, 24, 19)
	gotParent, wantParent := glyphPixels(dstBounds, 127), glyphPixels(dstBounds, 127)
	initial := bytes.Clone(gotParent.Pix)
	got := gotParent.SubImage(view).(*image.RGBA)
	want := wantParent.SubImage(view).(*image.RGBA)
	for _, r := range []image.Rectangle{image.Rect(0, 0, 22, 20), image.Rect(-5, 3, 25, 27), image.Rect(4, -3, 31, 14), {}, image.Rect(27, 22, 30, 25)} {
		check := func(op string, opacity int) {
			t.Helper()
			if !bytes.Equal(gotParent.Pix, wantParent.Pix) {
				t.Fatalf("%s differs for rectangle %v, opacity %d", op, r, opacity)
			}
			copy(gotParent.Pix, initial)
			copy(wantParent.Pix, initial)
		}
		for _, sp := range []image.Point{{}, {-3, -2}, {5, 8}} {
			Copy(got, r, src, sp)
			draw.Draw(want, r, src, sp, draw.Src)
			check("Copy", 0)
		}
		for a := range 256 {
			fill := color.RGBA{R: byte(a / 7), G: byte(a / 2), B: byte(a), A: byte(a)}
			MaskOver(got, r, fill, mask)
			draw.DrawMask(want, r, image.NewUniform(fill), image.Point{}, mask, image.Point{}, draw.Over)
			check("MaskOver", a)
			ColorOver(got, r, src, byte(a))
			draw.DrawMask(want, r, src, image.Point{}, image.NewUniform(color.Alpha{A: byte(a)}), image.Point{}, draw.Over)
			check("ColorOver", a)
		}
	}
}
