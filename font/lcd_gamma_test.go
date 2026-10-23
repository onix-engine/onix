package font

import (
	"image"
	"testing"
)

func TestLCDPreblendPreservesEndpointsAndMonotonicity(t *testing.T) {
	for level, table := range lcdPreblend {
		if table[0] != 0 || table[255] != 255 {
			t.Fatalf("level %d changed endpoints", level)
		}
		for i := 1; i < 256; i++ {
			if table[i] < table[i-1] {
				t.Fatalf("level %d is not monotonic at %d", level, i)
			}
		}
	}
	identity := buildLCDPreblend(1, 0)
	for _, table := range identity {
		for value, got := range table {
			if got != uint8(value) {
				t.Fatalf("linear gamma changed %d to %d", value, got)
			}
		}
	}
	// White-on-black needs stronger mask coverage when blended in encoded RGB.
	if lcdPreblend[7][128] <= 128 {
		t.Fatal("white foreground gamma correction was omitted")
	}
}

func TestLCDPreblendUsesIndependentForegroundChannels(t *testing.T) {
	bitmap := image.NewRGBA(image.Rect(0, 0, 1, 1))
	copy(bitmap.Pix, []byte{128, 128, 128, 255})
	LCDColorFromRGB(0, 128, 255).PreblendLCD(bitmap)
	if bitmap.Pix[0] == bitmap.Pix[2] {
		t.Fatal("foreground colors incorrectly share one gamma LUT")
	}
	if bitmap.Pix[3] != 255 {
		t.Fatal("preblend changed destination-alpha preservation")
	}
	if LCDColorFromRGB(0, 0, 0) != LCDColorFromRGB(31, 31, 31) {
		t.Fatal("canonical color cache not quantized")
	}
	if LCDColorFromRGB(255, 255, 255) != 511 {
		t.Fatal("color cache exceeds 512 canonical colors")
	}
	for value := 0; value < 256; value++ {
		got := lcdBlendCoverage(uint8(value))
		if value == 0 && got != 0 || value == 255 && got != 255 {
			t.Fatal("LCD blend quantization changed endpoints")
		}
	}
}

func TestLCDFIRChannelSymmetryAndBlankPadding(t *testing.T) {
	// A symmetric one-pixel stem has mirrored R/B coverage, equal alpha,
	// and no halo outside the filter's one-pixel extension.
	mask := filterLCD([]byte{0, 0, 0, 0, 255, 255, 255, 255, 0, 0, 0, 0}, 12, 1)
	for x := 0; x < mask.Rect.Dx(); x++ {
		left, right := mask.RGBAAt(x, 0), mask.RGBAAt(mask.Rect.Dx()-1-x, 0)
		if left.R != right.B || left.G != right.G || left.A != 255 {
			t.Fatalf("asymmetric FIR at %d: %v %v", x, left, right)
		}
	}
	if c := mask.RGBAAt(0, 0); c.R != 0 || c.G != 0 || c.B != 0 {
		t.Fatal("filter halo exceeded one pixel")
	}
}

func TestGrayPreblendRetainsEightBitCoverage(t *testing.T) {
	bitmap := image.NewAlpha(image.Rect(0, 0, 256, 1))
	for i := range bitmap.Pix {
		bitmap.Pix[i] = uint8(i)
	}
	GrayColorFromRGB(255, 255, 255).PreblendGray(bitmap)
	distinct := 0
	for i, value := range bitmap.Pix {
		if value != lcdPreblend[7][i] {
			t.Fatal("gray gamma used LCD quantization")
		}
		if i == 0 || value != bitmap.Pix[i-1] {
			distinct++
		}
	}
	if distinct <= 32 {
		t.Fatalf("only %d gray coverage levels survived", distinct)
	}
}
