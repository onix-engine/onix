package html

import (
	"crypto/sha256"
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/layout"
	"github.com/diiyw/onix/html/style"
)

func TestHTMLShadowRecordedPixels(t *testing.T) {
	hash := sha256.New()
	var scratch htmlFilterScratch
	for _, size := range []image.Point{{1, 61}, {73, 1}, {19, 13}, {97, 89}, {128, 128}} {
		for _, sigma := range []float64{0, .5, 3.9, 4, 8, 24, 100} {
			base := image.NewRGBA(image.Rect(-3, 4, size.X+9, size.Y+17))
			for i := range base.Pix {
				base.Pix[i] = byte(i*37 + i/11*17)
			}
			dst := base.SubImage(image.Rect(0, 7, size.X, size.Y+7)).(*image.RGBA)
			for y := range size.Y {
				for x := range size.X {
					dst.SetRGBA(x, y+7, color.RGBA{37, 121, 213, byte(x*19 + y*31)})
				}
			}
			htmlBlurShadowMask(dst, sigma, &scratch)
			hash.Write(base.Pix)
		}
	}
	if got := fmt.Sprintf("%x", hash.Sum(nil)); got != "8a4ad50ee85b4f15863d6e453cd6bc1d27026b4dc1532f5ef8a18d1a66410c66" {
		t.Fatalf("shadow pixels changed: %s", got)
	}
}

func roundTwoOpacityTree(depth int) *layout.Box {
	root := &layout.Box{Node: &dom.Node{}, Style: &style.ComputedStyle{Opacity: style.Number(.5)}, Content: layout.Rect{Width: 128, Height: 128}}
	parent := root
	for i := range depth {
		child := &layout.Box{Node: &dom.Node{}, Style: &style.ComputedStyle{Opacity: style.Number(1), BackgroundColor: style.ColorValue(style.Color{R: byte(i * 13), G: 97, B: 211, A: 255})}, Content: layout.Rect{Width: 128, Height: 128}}
		parent.Children = []*layout.Box{child}
		parent = child
	}
	return root
}

func TestHTMLOpacityRecordedPixels(t *testing.T) {
	root := roundTwoOpacityTree(8)
	leaf := root
	for len(leaf.Children) != 0 {
		leaf = leaf.Children[0]
	}
	leaf.Style.Opacity = style.Number(.63)
	leaf.Style.Filters = []style.Filter{{Kind: style.FilterBlur, Amount: 1.5}, {Kind: style.FilterInvert, Amount: .7}}
	leaf.Content = layout.Rect{X: 3, Y: 7, Width: 61, Height: 59}
	cache := htmlTextureCache{}
	hash := sha256.New()
	for i := range 4 {
		leaf.Style.BackgroundColor.Color.R = byte(i * 31)
		raster := htmlOpacityRasterFor(&cache, root)
		composited := append([]byte(nil), raster.image.Pix...)
		for index := 3; index < len(composited); index += 4 {
			composited[index] = uint8((uint16(composited[index]) + 1) / 2)
		}
		hash.Write(composited)
	}
	if got := fmt.Sprintf("%x", hash.Sum(nil)); got != "c0e30d63956b38fa5585862e4de6d5ce22aea8c677b7c5b7736de7b404005e00" {
		t.Fatalf("opacity pixels changed: %s", got)
	}
}

func TestHTMLDropShadowRecordedPixels(t *testing.T) {
	hash := sha256.New()
	var scratch htmlFilterScratch
	for _, sigma := range []float64{0, .5, 3, 12} {
		for _, offset := range []float64{-17.5, 0, 2.75} {
			base := image.NewRGBA(image.Rect(-3, 4, 49, 59))
			for i := range base.Pix {
				base.Pix[i] = byte(i*37 + i/11*17)
			}
			dst := base.SubImage(image.Rect(-1, 7, 43, 51)).(*image.RGBA)
			htmlRasterDropShadow(dst, style.Shadow{BlurRadius: style.LengthPx(sigma), OffsetX: style.LengthPx(offset), OffsetY: style.LengthPx(-offset / 2), Color: style.Color{R: 37, G: 121, B: 213, A: 137}}, &scratch)
			hash.Write(base.Pix)
		}
	}
	if got := fmt.Sprintf("%x", hash.Sum(nil)); got != "5400cb3cf6c37e0aea8cf01705e1e97266a275a88000d6784c146491ef3af799" {
		t.Fatalf("drop-shadow pixels changed: %s", got)
	}
}

func BenchmarkHTMLDropShadowWorkingSet(b *testing.B) {
	for _, cold := range []bool{true, false} {
		b.Run(fmt.Sprintf("cold=%t", cold), func(b *testing.B) {
			dst := image.NewRGBA(image.Rect(0, 0, 256, 256))
			shadow := style.Shadow{BlurRadius: style.LengthPx(3), OffsetX: style.LengthPx(2.75), Color: style.Color{A: 137}}
			var scratch htmlFilterScratch
			htmlRasterDropShadow(dst, shadow, &scratch)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if cold {
					scratch = htmlFilterScratch{}
				}
				htmlRasterDropShadow(dst, shadow, &scratch)
			}
		})
	}
}

func BenchmarkHTMLShadowWorkingSet(b *testing.B) {
	for _, size := range []int{128, 1024} {
		for _, cold := range []bool{true, false} {
			b.Run(fmt.Sprintf("%d/cold=%t", size, cold), func(b *testing.B) {
				dst := image.NewRGBA(image.Rect(0, 0, size, size))
				var scratch htmlFilterScratch
				htmlBlurShadowMask(dst, 8, &scratch)
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					if cold {
						scratch = htmlFilterScratch{}
					}
					dst.Pix[len(dst.Pix)/2+3] = 255
					htmlBlurShadowMask(dst, 8, &scratch)
				}
			})
		}
	}
}

func BenchmarkHTMLOpacityWorkingSet(b *testing.B) {
	for _, depth := range []int{1, 16} {
		for _, mode := range []string{"cold", "repaint", "unchanged"} {
			b.Run(fmt.Sprintf("%d/%s", depth, mode), func(b *testing.B) {
				root := roundTwoOpacityTree(depth)
				cache := htmlTextureCache{}
				htmlOpacityRasterFor(&cache, root)
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					if mode == "cold" {
						cache = htmlTextureCache{}
					} else if mode == "repaint" {
						root.Style.BackgroundColor.Color.R ^= 1
					}
					htmlOpacityRasterFor(&cache, root)
				}
				bytes := 0
				for _, raster := range cache.rasters {
					bytes += cap(raster.image.Pix)
				}
				b.ReportMetric(float64(bytes), "retained-image-B")
			})
		}
	}
}
