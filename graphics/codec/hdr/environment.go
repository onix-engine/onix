package hdr

import (
	"fmt"
	"image"

	"github.com/diiyw/onix/graphics"
)

// BakeEnvironment decodes a 2:1 HDR panorama and bakes an RGBE environment
// atlas. Each texel retains its own exponent, so bright lights do not reduce
// the precision of dim sky and IBL colors elsewhere in the panorama.
// Decoding and convolution run once at asset loading, never during sampling.
func BakeEnvironment(data []byte, width, samples int) (*image.NRGBA, error) {
	images, err := ImportEnvironment(data, width, samples)
	return images.Atlas, err
}

// ImportEnvironment retains the original background and filters lighting.
func ImportEnvironment(data []byte, width, samples int) (graphics.EnvironmentImages, error) {
	panorama, err := Decode(data)
	if err != nil {
		return graphics.EnvironmentImages{}, err
	}
	if panorama.width != panorama.height*2 {
		return graphics.EnvironmentImages{}, fmt.Errorf("hdr: environment panorama must have a 2:1 aspect ratio")
	}
	return graphics.BakePanoramaEnvironment(panorama, width, samples)
}
