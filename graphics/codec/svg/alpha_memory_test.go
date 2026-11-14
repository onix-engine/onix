package svg

import (
	"crypto/sha256"
	"fmt"
	"image"
	"image/color"
	"sync"
	"testing"

	"github.com/onix-engine/onix/internal/reuse"
)

var transparentColorReferences = [...]struct {
	want          string
	width, height int
	seed          uint32
}{
	{"1f903811a863c0aec22f4660cb115397633419423def595af834181b9035997e", 64, 41, 7},
	{"f4e7b4bc8e6a0f6d7c113803a18ac40e5decfcaacfd4a87a241fc247804ea080", 37, 91, 19},
	{"908f824b848ec5f2cfddcfa292cc183cb337c7c92f37b8496d1b1dd117c70e9c", 17, 13, 93},
}

func transparentColorFixture(width, height int, seed uint32) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for offset := 0; offset < len(img.Pix); offset += 4 {
		seed = seed*1664525 + 1013904223
		img.Pix[offset], img.Pix[offset+1], img.Pix[offset+2] = byte(seed), byte(seed>>8), byte(seed>>16)
		if (seed>>24)%7 == 0 {
			img.Pix[offset+3] = byte(seed>>16) | 1
		}
	}
	return img
}

func TestTransparentColorsReferencePixels(t *testing.T) {
	// Golden hashes include transparent RGB, coverage, and equidistant color
	// choices from the original integer distance transform.
	for _, test := range transparentColorReferences {
		img := transparentColorFixture(test.width, test.height, test.seed)
		extendTransparentColors(img)
		got := fmt.Sprintf("%x", sha256.Sum256(img.Pix))
		if got != test.want {
			t.Errorf("%dx%d seed %d: %s", test.width, test.height, test.seed, got)
		}
	}
}

func TestTransparentColorsReinitializeScratch(t *testing.T) {
	for _, test := range transparentColorReferences {
		img := transparentColorFixture(test.width, test.height, test.seed)
		distances := make([]uint16, test.width*test.height)
		for i := range distances {
			distances[i] = 65535
		}
		extendTransparentColorsWithDistances(img, distances)
		if got := fmt.Sprintf("%x", sha256.Sum256(img.Pix)); got != test.want {
			t.Fatalf("reused distances changed %dx%d pixels: %s", test.width, test.height, got)
		}
	}
}

func TestTransparentColorsConcurrent(t *testing.T) {
	var wait sync.WaitGroup
	for worker := range 8 {
		wait.Go(func() {
			for pass := range 16 {
				test := transparentColorReferences[(worker+pass)%len(transparentColorReferences)]
				img := transparentColorFixture(test.width, test.height, test.seed)
				extendTransparentColors(img)
				if got := fmt.Sprintf("%x", sha256.Sum256(img.Pix)); got != test.want {
					t.Errorf("concurrent decode changed %dx%d pixels: %s", test.width, test.height, got)
					return
				}
			}
		})
	}
	wait.Wait()
}

func TestTransparentColorScratchRetention(t *testing.T) {
	first, second := acquireTransparentColorScratch(64), acquireTransparentColorScratch(64)
	if first == second || &first.distances[0] == &second.distances[0] {
		t.Fatal("concurrent borrowers share a distance table")
	}
	releaseTransparentColorScratch(first)
	releaseTransparentColorScratch(second)

	scratch := transparentColorScratch{distances: make([]uint16, 32768)}
	for range reuse.RetentionFrames {
		scratch.distances = scratch.distances[:32]
		scratch.trim()
	}
	if cap(scratch.distances) != 4096 {
		t.Fatalf("small images retained peak capacity: %d", cap(scratch.distances))
	}
	scratch.distances = make([]uint16, transparentColorScratchLimit/2+1)
	scratch.trim()
	if scratch.distances != nil {
		t.Fatal("oversized distance table was retained")
	}
}

func TestTransparentColorsLongDistances(t *testing.T) {
	for _, length := range []int{65534, 65535, 65537} {
		for _, vertical := range []bool{false, true} {
			width, height := length, 1
			if vertical {
				width, height = height, width
			}
			img := image.NewNRGBA(image.Rect(0, 0, width, height))
			img.SetNRGBA(width-1, height-1, color.NRGBA{R: 193, G: 87, B: 41, A: 127})
			extendTransparentColors(img)
			for offset := 0; offset < len(img.Pix); offset += 4 {
				alpha := byte(0)
				if offset == len(img.Pix)-4 {
					alpha = 127
				}
				if img.Pix[offset] != 193 || img.Pix[offset+1] != 87 || img.Pix[offset+2] != 41 || img.Pix[offset+3] != alpha {
					t.Fatalf("%dx%d: invalid distant pixel %d: %v", width, height, offset/4, img.Pix[offset:offset+4])
				}
			}
		}
	}
}

func BenchmarkTransparentColors(b *testing.B) {
	for _, size := range []int{64, 1024, 2048} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			img := transparentColorFixture(size, size, 7)
			extendTransparentColors(img)
			b.ReportAllocs()
			b.SetBytes(int64(len(img.Pix)))
			for b.Loop() {
				extendTransparentColors(img)
			}
		})
	}
}
