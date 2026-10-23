package texcompress

import (
	"fmt"
	"math"
)

// The transfer function tables keep mip generation in integer arithmetic, so a
// build produces byte-identical results on every machine.
var (
	srgbToLinear [256]uint16
	linearToSRGB [4096]uint8
)

func init() {
	for value := range 256 {
		srgbToLinear[value] = uint16(math.Round(srgbToLinearUnit(float64(value)/255) * 65535))
	}
	for index := range 4096 {
		linearToSRGB[index] = uint8(math.Round(linearToSRGBUnit(float64(index*16)/65535) * 255))
	}
}

func srgbToLinearUnit(value float64) float64 {
	if value <= 0.04045 {
		return value / 12.92
	}
	return math.Pow((value+0.055)/1.055, 2.4)
}

func linearToSRGBUnit(value float64) float64 {
	if value <= 0.0031308 {
		return value * 12.92
	}
	return 1.055*math.Pow(value, 1/2.4) - 0.055
}

// downsample halves a level with a box filter. sRGB levels are averaged in
// linear light, which is what the device blit path already does for sRGB image
// formats; averaging encoded values instead would darken every mip.
func downsample(pixels []byte, width, height int, srgb bool) []byte {
	targetWidth, targetHeight := max(1, width/2), max(1, height/2)
	target := make([]byte, targetWidth*targetHeight*4)
	downsampleInto(target, pixels, width, height, srgb)
	return target
}

func downsampleInto(target, pixels []byte, width, height int, srgb bool) {
	targetWidth, targetHeight := max(1, width/2), max(1, height/2)
	for y := range targetHeight {
		for x := range targetWidth {
			var sums [4]uint32
			for row := range 2 {
				sourceY := min(2*y+row, height-1)
				for column := range 2 {
					sourceX := min(2*x+column, width-1)
					offset := (sourceY*width + sourceX) * 4
					for channel := range 3 {
						value := pixels[offset+channel]
						if srgb {
							sums[channel] += uint32(srgbToLinear[value])
						} else {
							sums[channel] += uint32(value)
						}
					}
					sums[3] += uint32(pixels[offset+3])
				}
			}
			offset := (y*targetWidth + x) * 4
			for channel := range 3 {
				average := sums[channel] / 4
				if srgb {
					target[offset+channel] = linearToSRGB[average>>4]
				} else {
					target[offset+channel] = uint8(average)
				}
			}
			// Alpha is never transferred through the sRGB curve.
			target[offset+3] = uint8(sums[3] / 4)
		}
	}
}

// DownsampleRGBA halves one RGBA8 level with the same deterministic box filter
// used by EncodeChain. External block encoders use it so every target ships an
// identical mip hierarchy.
func DownsampleRGBA(pixels []byte, width, height int, srgb bool) ([]byte, error) {
	if width <= 0 || height <= 0 || len(pixels) != width*height*4 {
		return nil, fmt.Errorf("texcompress: level %dx%d needs %d bytes, got %d", width, height, width*height*4, len(pixels))
	}
	return downsample(pixels, width, height, srgb), nil
}
