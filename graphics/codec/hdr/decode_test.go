package hdr

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

func flatHDR(resolution string, pixels []byte) []byte {
	return append([]byte("#?RADIANCE\nFORMAT=32-bit_rle_rgbe\n\n"+resolution+"\n"), pixels...)
}

func TestDecodePreservesLinearHDRAndOwnsPixels(t *testing.T) {
	data := flatHDR("-Y 1 +X 2", []byte{128, 64, 32, 134, 255, 255, 255, 0})
	p, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	clear(data)
	if w, h := p.Size(); w != 2 || h != 1 {
		t.Fatalf("size = %d x %d", w, h)
	}
	if got := p.At(0, 0); got != (mgl32.Vec3{32.125, 16.125, 8.125}) {
		t.Fatalf("linear RGBE = %v", got)
	}
	if p.At(1, 0) != (mgl32.Vec3{}) || p.At(-1, 0) != (mgl32.Vec3{}) {
		t.Fatal("zero exponent or out-of-bounds sample was not black")
	}
}

func TestDecodeAllOrientations(t *testing.T) {
	for _, a := range []string{"-Y", "+Y", "-X", "+X"} {
		for _, sign := range []string{"-", "+"} {
			b := sign + "X"
			rows, columns := 2, 3
			if a[1] == 'X' {
				b, rows, columns = sign+"Y", 3, 2
			}
			t.Run(a+b, func(t *testing.T) {
				var pixels []byte
				for row := range rows {
					for column := range columns {
						x, y := column, row
						if a[1] == 'X' {
							x, y = row, column
						}
						if a == "-X" || b == "-X" {
							x = 2 - x
						}
						if a == "+Y" || b == "+Y" {
							y = 1 - y
						}
						pixels = append(pixels, byte(128+y*3+x), 64, 32, 129)
					}
				}
				p, err := Decode(flatHDR(fmt.Sprintf("%s %d %s %d", a, rows, b, columns), pixels))
				if err != nil {
					t.Fatal(err)
				}
				for y := range 2 {
					for x := range 3 {
						if got, want := p.At(x, y)[0], (128.5+float32(y*3+x))/128; got != want {
							t.Fatalf("(%d,%d) = %g, want %g", x, y, got, want)
						}
					}
				}
			})
		}
	}
}

func TestDecodeRLEAndFlatScanlines(t *testing.T) {
	var pixels []byte
	for i := range 8 {
		pixels = append(pixels, byte(128+i), 64, 32, 132)
	}
	// A flat row followed by a row mixing literal and repeated components.
	encoded := append([]byte(nil), pixels...)
	encoded = append(encoded, 2, 2, 0, 8, 8, 128, 129, 130, 131, 132, 133, 134, 135, 136, 64, 136, 32, 136, 132)
	data := flatHDR("-Y 2 +X 8", encoded)
	data = bytes.ReplaceAll(data, []byte{'\n'}, []byte{'\r', '\n'})
	p, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	for x := range 8 {
		if p.At(x, 0) != p.At(x, 1) {
			t.Fatalf("RLE differs from flat at x=%d", x)
		}
	}
	// A literal packet of length 128 is legal, and is not a zero-length run.
	encoded = []byte{2, 2, 0, 128}
	for _, value := range []byte{128, 64, 32, 132} {
		encoded = append(encoded, 128)
		encoded = append(encoded, bytes.Repeat([]byte{value}, 128)...)
	}
	if _, err := Decode(flatHDR("-Y 1 +X 128", encoded)); err != nil {
		t.Fatal(err)
	}
}

func TestDecodeRejectsMalformedHDR(t *testing.T) {
	valid := flatHDR("-Y 1 +X 8", []byte{2, 2, 0, 8, 136, 128, 136, 64, 136, 32, 136, 132})
	for i := range len(valid) {
		if _, err := Decode(valid[:i]); err == nil {
			t.Fatalf("accepted truncation at byte %d", i)
		}
	}
	for name, data := range map[string][]byte{
		"signature":      []byte("not an HDR file"),
		"xyze":           bytes.ReplaceAll(valid, []byte("rgbe"), []byte("xyze")),
		"missing format": bytes.ReplaceAll(valid, []byte("FORMAT=32-bit_rle_rgbe\n"), nil),
		"duplicate format": bytes.ReplaceAll(valid, []byte("FORMAT=32-bit_rle_rgbe"),
			[]byte("FORMAT=32-bit_rle_rgbe\nFORMAT=32-bit_rle_rgbe")),
		"same axes": flatHDR("-Y 1 +Y 8", make([]byte, 32)),
		"negative":  flatHDR("-Y -1 +X 8", nil),
		"overflow":  flatHDR("-Y 9223372036854775807 +X 9223372036854775807", nil),
		"size cap":  flatHDR("-Y 16384 +X 16384", nil),
		"zero run":  flatHDR("-Y 1 +X 8", []byte{2, 2, 0, 8, 0, 128, 136, 64, 136, 32, 136, 132}),
		"overrun":   flatHDR("-Y 1 +X 8", []byte{2, 2, 0, 8, 137, 128, 136, 64, 136, 32, 136, 132}),
		"width":     flatHDR("-Y 1 +X 8", []byte{2, 2, 0, 9, 136, 128, 136, 64, 136, 32, 136, 132}),
		"old RLE":   flatHDR("-Y 1 +X 2", []byte{128, 64, 32, 132, 1, 1, 1, 1}),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(data); err == nil {
				t.Fatal("accepted malformed HDR")
			}
		})
	}
}

func TestPanoramaBilinearSeamPolesAndAllocations(t *testing.T) {
	p, err := Decode(flatHDR("-Y 2 +X 2", []byte{128, 64, 32, 132, 128, 64, 32, 133, 128, 64, 32, 134, 128, 64, 32, 135}))
	if err != nil {
		t.Fatal(err)
	}
	average := p.At(0, 0).Mul(.25).Add(p.At(1, 0).Mul(.25)).Add(p.At(0, 1).Mul(.25)).Add(p.At(1, 1).Mul(.25))
	for _, n := range []mgl32.Vec3{{1, 0, 0}, {-1, 0, 0}} {
		if p.Radiance(n).Sub(average).Len() > .00001 {
			t.Fatalf("bilinear/seam sample %v = %v, want %v", n, p.Radiance(n), average)
		}
	}
	for _, n := range []mgl32.Vec3{{0, 1, 0}, {0, -1, 0}} {
		y := 0
		if n.Y() < 0 {
			y = 1
		}
		want := p.At(0, y).Mul(.5).Add(p.At(1, y).Mul(.5))
		if p.Radiance(n) != want {
			t.Fatalf("pole sample = %v, want %v", p.Radiance(n), want)
		}
	}
	if allocations := testing.AllocsPerRun(100, func() { _ = p.Radiance(mgl32.Vec3{1, 0, 0}) }); allocations != 0 {
		t.Fatalf("sampling allocated %g times", allocations)
	}
}

func TestEnvironmentBakeRestoresRadianceAboveAtlasRange(t *testing.T) {
	data := flatHDR("-Y 1 +X 2", bytes.Repeat([]byte{128, 64, 32, 134}, 2))
	atlas, err := BakeEnvironment(data, 16, 16)
	if err != nil {
		t.Fatal(err)
	}

	for y := 0; y < 7*8; y++ {
		for x := range 16 {
			c := atlas.NRGBAAt(x, y)
			for i, channel := range [3]byte{c.R, c.G, c.B} {
				value := float32(channel) / 255 * float32(math.Exp2(float64(int(c.A)-128)))
				want := [...]float32{32.125, 16.125, 8.125}[i]
				if math.Abs(float64(value-want)) > .13 {
					t.Fatalf("tile %d restored %g, want %g", y/8, value, want)
				}
			}
		}
	}
	if _, err := BakeEnvironment(flatHDR("-Y 1 +X 1", []byte{128, 64, 32, 130}), 16, 16); err == nil || !strings.Contains(err.Error(), "2:1") {
		t.Fatalf("accepted non-panorama: %v", err)
	}
}

func FuzzDecode(f *testing.F) {
	f.Add(flatHDR("-Y 1 +X 2", []byte{128, 64, 32, 134, 0, 0, 0, 0}))
	f.Add(flatHDR("-Y 1 +X 8", []byte{2, 2, 0, 8, 136, 128, 136, 64, 136, 32, 136, 132}))
	f.Fuzz(func(t *testing.T, data []byte) {
		p, err := Decode(data)
		if err != nil {
			return
		}
		value := p.Radiance(mgl32.Vec3{1, 0, 0})
		for _, c := range value {
			if math.IsNaN(float64(c)) || math.IsInf(float64(c), 0) || c < 0 {
				t.Fatalf("invalid radiance %v", value)
			}
		}
	})
}
