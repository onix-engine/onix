// Package ultrahdr reconstructs linear HDR from JPEG base images and gain maps.
// It reads Adobe XMP and ISO 21496-1 metadata in MPF containers. Decoding is
// asset-time work; no intermediate HDR file or runtime JPEG decoding is needed.
package ultrahdr

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math"

	"github.com/diiyw/onix/graphics"
	"github.com/go-gl/mathgl/mgl32"
)

// Panorama owns linear sRGB pixels. Decode restores the full HDR rendition,
// independently of the output display's headroom; tone mapping happens later.
type Panorama struct {
	width, height int
	pixels        []mgl32.Vec3
}

func (p *Panorama) Size() (int, int) { return p.width, p.height }
func (p *Panorama) At(x, y int) mgl32.Vec3 {
	if x < 0 || y < 0 || x >= p.width || y >= p.height {
		return mgl32.Vec3{}
	}
	return p.pixels[y*p.width+x]
}
func (p *Panorama) Radiance(n mgl32.Vec3) mgl32.Vec3 { return graphics.SamplePanorama(p, n) }

// BakeEnvironment decodes a 2:1 Ultra HDR JPG into an RGBE lighting atlas.
func BakeEnvironment(data []byte, width, samples int) (*image.NRGBA, error) {
	images, err := ImportEnvironment(data, width, samples)
	return images.Atlas, err
}

// ImportEnvironment retains the original background and filters lighting.
func ImportEnvironment(data []byte, width, samples int) (graphics.EnvironmentImages, error) {
	p, err := Decode(data)
	if err != nil {
		return graphics.EnvironmentImages{}, err
	}
	if p.width != p.height*2 {
		return graphics.EnvironmentImages{}, fmt.Errorf("ultrahdr: environment panorama must have a 2:1 aspect ratio")
	}
	return graphics.BakePanoramaEnvironment(p, width, samples)
}

func Decode(data []byte) (*Panorama, error) {
	base, gain, meta, profile, err := container(data)
	if err != nil {
		return nil, err
	}
	transform, err := decodeProfile(profile)
	if err != nil {
		return nil, err
	}
	primary, err := decodeJPEG(base)
	if err != nil {
		return nil, err
	}
	gainmap, err := decodeJPEG(gain)
	if err != nil {
		return nil, err
	}
	b, g := primary.Bounds(), gainmap.Bounds()
	if b.Dx()*g.Dy() != b.Dy()*g.Dx() {
		return nil, fmt.Errorf("ultrahdr: base and gain map aspect ratios differ")
	}
	p := &Panorama{width: b.Dx(), height: b.Dy(), pixels: make([]mgl32.Vec3, b.Dx()*b.Dy())}
	// Transfer functions are evaluated once, and integer gain values share a LUT.
	var boosts [3][256]float64
	for c := range 3 {
		for i := range 256 {
			recovery := math.Pow(float64(i)/255, 1/meta.gamma[c])
			boosts[c][i] = math.Exp2(meta.min[c] + (meta.max[c]-meta.min[c])*recovery)
		}
	}
	for y := range p.height {
		for x := range p.width {
			rgb := rgbAt(primary, x, y)
			gainRGB := sampleGain(gainmap, x, y, p.width, p.height)
			var linear mgl32.Vec3
			for c := range 3 {
				recovery := gainRGB[c]
				boost := boosts[c][int(recovery)]
				if recovery != math.Floor(recovery) {
					boost = math.Exp2(meta.min[c] + (meta.max[c]-meta.min[c])*math.Pow(recovery/255, 1/meta.gamma[c]))
				}
				value := (transform.transfer[c][rgb[c]]+meta.sdr[c])*boost - meta.hdr[c]
				if math.IsNaN(value) || math.IsInf(value, 0) || value > math.Exp2(120) {
					return nil, fmt.Errorf("ultrahdr: recovered radiance exceeds RGBE range")
				}
				linear[c] = float32(max(0, value))
			}
			converted := transform.matrix.Mul3x1(linear)
			for c, value := range converted {
				if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) || value > float32(math.Exp2(120)) {
					return nil, fmt.Errorf("ultrahdr: converted radiance exceeds RGBE range")
				}
				converted[c] = max(0, value)
			}
			p.pixels[y*p.width+x] = converted
		}
	}
	return p, nil
}

func decodeJPEG(data []byte) (image.Image, error) {
	config, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("ultrahdr: JPEG header: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > 16384 || config.Height > 16384 || config.Width > (1<<24)/config.Height {
		return nil, fmt.Errorf("ultrahdr: excessive JPEG dimensions")
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("ultrahdr: JPEG pixels: %w", err)
	}
	switch img.(type) {
	case *image.YCbCr, *image.Gray:
	default:
		return nil, fmt.Errorf("ultrahdr: expected RGB or grayscale JPEG")
	}
	return img, nil
}

func rgbAt(img image.Image, x, y int) [3]byte {
	switch p := img.(type) {
	case *image.YCbCr:
		c := p.YCbCrAt(x, y)
		r, g, b := color.YCbCrToRGB(c.Y, c.Cb, c.Cr)
		return [3]byte{r, g, b}
	case *image.Gray:
		v := p.GrayAt(x, y).Y
		return [3]byte{v, v, v}
	}
	return [3]byte{}
}

func sampleGain(img image.Image, x, y, width, height int) [3]float64 {
	b := img.Bounds()
	if b.Dx() == width && b.Dy() == height {
		c := rgbAt(img, x, y)
		return [3]float64{float64(c[0]), float64(c[1]), float64(c[2])}
	}
	gx, gy := (float64(x)+.5)*float64(b.Dx())/float64(width)-.5, (float64(y)+.5)*float64(b.Dy())/float64(height)-.5
	x0, y0 := int(math.Floor(gx)), int(math.Floor(gy))
	tx, ty := gx-float64(x0), gy-float64(y0)
	x1, y1 := max(0, min(b.Dx()-1, x0+1)), max(0, min(b.Dy()-1, y0+1))
	x0, y0 = max(0, min(b.Dx()-1, x0)), max(0, min(b.Dy()-1, y0))
	a, bb, c, d := rgbAt(img, x0, y0), rgbAt(img, x1, y0), rgbAt(img, x0, y1), rgbAt(img, x1, y1)
	var result [3]float64
	for i := range 3 {
		result[i] = (float64(a[i])*(1-tx)+float64(bb[i])*tx)*(1-ty) + (float64(c[i])*(1-tx)+float64(d[i])*tx)*ty
	}
	return result
}
