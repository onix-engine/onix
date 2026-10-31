package html

import (
	"crypto/sha256"
	"fmt"
	"image"
	"testing"
)

func TestHTMLGaussianBlurRecordedPixels(t *testing.T) {
	// Recordings from the full-image convolution cover ring wrapping, kernels
	// larger than the image, and subimages with an offset and a wider stride.
	var scratch htmlFilterScratch
	for _, test := range []struct {
		width, height int
		sigma         float64
		want          string
	}{
		{1, 65, 1.5, "c5697f1140427d92e0950a91bd2d734ff4a2ece735be34f9428faab259a5b8d4"},
		{65, 1, 1.5, "5f58a48508551e4db53cbaf7105f5dc05e64a734b0bd2580a1b938b537de64a5"},
		{13, 97, .25, "c746143241671dcebc06f3fc39b90ae39af61bc400a2401c11c16532abf8b7f1"},
		{17, 83, 2, "34c21f9f7fd19b49c5ffefc62b7a5d39048be91db3bdc7993df60fd9f9a3c3ee"},
		{31, 97, 5, "3fbb9d061e60dba7becc2702ebbb8388712ee56a854a1ad0704fa5e9243d7f3e"},
		{19, 7, 17.5, "13fe003845b299760aff5661ded783764f30b9e21790da3480864cf63d805a67"},
		{23, 101, 1.5, "1bce3d181b23f1b1e8be849d3928a054b74e7ba1a229d84801eb1cb89235674c"},
		{13, 97, 0, "ce70d83c6e8a4357eb8c99a06f7c00f7d43e83836f23c67e87628412b94506e6"},
	} {
		t.Run(fmt.Sprintf("%dx%d/sigma%g", test.width, test.height, test.sigma), func(t *testing.T) {
			base := image.NewRGBA(image.Rect(-5, 3, test.width+8, test.height+12))
			for index := range base.Pix {
				base.Pix[index] = byte(index*37 + index/11*17)
			}
			dst := base.SubImage(image.Rect(-2, 7, test.width-2, test.height+7)).(*image.RGBA)
			htmlGaussianBlur(dst, test.sigma, &scratch)
			got := fmt.Sprintf("%x", sha256.Sum256(base.Pix))
			if got != test.want {
				t.Fatalf("blur changed recorded pixels: got %s, want %s", got, test.want)
			}
		})
	}
}

func TestHTMLGaussianBlurMemoryBudget(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 1024, 1024))
	var scratch htmlFilterScratch
	htmlGaussianBlur(dst, 5, &scratch)
	if bytes := cap(scratch.rows) * 32; bytes > 2<<20 {
		t.Fatalf("blur intermediate retains %d bytes; budget is 2 MiB", bytes)
	}
}

func BenchmarkHTMLGaussianBlur(b *testing.B) {
	for _, size := range []int{128, 1024} {
		for _, cold := range []bool{true, false} {
			name := "warm"
			if cold {
				name = "cold"
			}
			b.Run(fmt.Sprintf("%d/%s", size, name), func(b *testing.B) {
				dst := image.NewRGBA(image.Rect(0, 0, size, size))
				var scratch htmlFilterScratch
				htmlGaussianBlur(dst, 5, &scratch)
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					if cold {
						scratch = htmlFilterScratch{}
					}
					htmlGaussianBlur(dst, 5, &scratch)
				}
			})
		}
	}
}
