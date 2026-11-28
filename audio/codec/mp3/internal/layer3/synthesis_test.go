package layer3

import "testing"

func TestMixedBlockKeepsLongWindowInLowSubbands(t *testing.T) {
	var mixed, normal filterbank
	for _, blockType := range []int{1, 2, 2, 3} {
		var x [576]float32
		x[1], x[20] = 0.2, -0.1
		y := x
		var got, want [2 * granuleSamples]float32
		mixed.synthesize(&x, &granule{blockType: blockType, mixed: true}, 0, got[:], 0)
		normal.synthesize(&y, &granule{}, 0, want[:], 0)
		if got != want {
			t.Fatalf("mixed block type %d changed a low subband", blockType)
		}
	}
}

func TestIntensityStereoCutoff(t *testing.T) {
	for _, kind := range []int{shortBlock, mixedBlock} {
		layout := &layouts[0][kind]
		firstShort := 0
		if kind == mixedBlock {
			firstShort = 8
		}
		var x [2][576]float32
		// Put right-channel energy in the third window, one frequency
		// band above the left-channel sample in the first window.
		leftLine := layout.band[firstShort].start
		x[0][leftLine] = 0.5
		x[1][layout.band[firstShort+5].start] = 0.25
		jointStereo(&x, header{granules: 2, stereo: 1}, &granule{}, &scaleFactors{}, layout)
		want := [2]float32{0, 0.5}
		if kind == mixedBlock {
			want = [2]float32{0.5, 0}
		}
		if got := [2]float32{x[0][leftLine], x[1][leftLine]}; got != want {
			t.Fatalf("layout %d intensity stereo = %v, want %v", kind, got, want)
		}
	}
}
