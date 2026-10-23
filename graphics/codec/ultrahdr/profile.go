package ultrahdr

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/diiyw/onix/graphics"
	"github.com/go-gl/mathgl/mgl32"
)

type colorTransform struct {
	transfer [3][256]float64
	matrix   mgl32.Mat3
}

// Matrix/TRC ICC profiles are decoded in their native primaries, then adapted
// from ICC's D50 PCS to linear sRGB. Unsupported LUT profiles fail explicitly.
func decodeProfile(data []byte) (colorTransform, error) {
	t := colorTransform{matrix: mgl32.Ident3()}
	if len(data) == 0 {
		for i := range 256 {
			v := float64(graphics.SRGBToLinear(float32(i) / 255))
			for c := range 3 {
				t.transfer[c][i] = v
			}
		}
		return t, nil
	}
	fail := func() (colorTransform, error) {
		return t, fmt.Errorf("ultrahdr: unsupported or invalid RGB matrix/TRC ICC profile")
	}
	if len(data) < 132 || string(data[36:40]) != "acsp" || string(data[16:20]) != "RGB " || string(data[20:24]) != "XYZ " || uint64(binary.BigEndian.Uint32(data)) != uint64(len(data)) {
		return fail()
	}
	count := uint64(binary.BigEndian.Uint32(data[128:]))
	if 132+count*12 > uint64(len(data)) {
		return fail()
	}
	var xyz, trc [3][]byte
	for i := range int(count) {
		tag := data[132+i*12:][:12]
		start, size := uint64(binary.BigEndian.Uint32(tag[4:])), uint64(binary.BigEndian.Uint32(tag[8:]))
		if start+size > uint64(len(data)) {
			return fail()
		}
		value := data[start : start+size]
		switch string(tag[:4]) {
		case "A2B0", "A2B1", "A2B2", "B2A0", "B2A1", "B2A2":
			return fail()
		case "rXYZ":
			xyz[0] = value
		case "gXYZ":
			xyz[1] = value
		case "bXYZ":
			xyz[2] = value
		case "rTRC":
			trc[0] = value
		case "gTRC":
			trc[1] = value
		case "bTRC":
			trc[2] = value
		}
	}
	var primaries mgl32.Mat3
	for c := range 3 {
		if len(xyz[c]) < 20 || string(xyz[c][:4]) != "XYZ " {
			return fail()
		}
		for row := range 3 {
			primaries[c*3+row] = float32(fixed(xyz[c][8+row*4:]))
		}
		if err := decodeCurve(trc[c], &t.transfer[c]); err != nil {
			return t, err
		}
	}
	// D50 XYZ -> D65-adapted sRGB (Bradford adaptation composed with XYZ -> RGB).
	toSRGB := mgl32.Mat3{3.1338561, -.9787684, .0719453, -1.6168667, 1.9161415, -.2289914, -.4906146, .0334540, 1.4052427}
	t.matrix = toSRGB.Mul3(primaries)
	return t, nil
}
func fixed(data []byte) float64 { return float64(int32(binary.BigEndian.Uint32(data))) / 65536 }

func decodeCurve(data []byte, table *[256]float64) error {
	bad := func() error { return fmt.Errorf("ultrahdr: unsupported or invalid ICC transfer curve") }
	if len(data) < 12 {
		return bad()
	}
	var curve func(float64) float64
	switch string(data[:4]) {
	case "curv":
		count := uint64(binary.BigEndian.Uint32(data[8:]))
		if 12+count*2 > uint64(len(data)) {
			return bad()
		}
		switch count {
		case 0:
			curve = func(x float64) float64 { return x }
		case 1:
			gamma := float64(binary.BigEndian.Uint16(data[12:])) / 256
			if gamma <= 0 {
				return bad()
			}
			curve = func(x float64) float64 { return math.Pow(x, gamma) }
		default:
			curve = func(x float64) float64 {
				p := x * float64(count-1)
				lo := int(p)
				hi := min(int(count)-1, lo+1)
				f := p - float64(lo)
				a, b := float64(binary.BigEndian.Uint16(data[12+lo*2:])), float64(binary.BigEndian.Uint16(data[12+hi*2:]))
				return (a*(1-f) + b*f) / 65535
			}
		}
	case "para":
		kind := int(binary.BigEndian.Uint16(data[8:]))
		counts := [5]int{1, 3, 4, 5, 7}
		if kind >= len(counts) || len(data) < 12+counts[kind]*4 {
			return bad()
		}
		var p [7]float64
		for i := range counts[kind] {
			p[i] = fixed(data[12+i*4:])
		}
		if p[0] <= 0 || kind > 0 && p[1] == 0 {
			return bad()
		}
		curve = func(x float64) float64 {
			switch kind {
			case 0:
				return math.Pow(x, p[0])
			case 1:
				if x >= -p[2]/p[1] {
					return math.Pow(p[1]*x+p[2], p[0])
				}
				return 0
			case 2:
				if x >= -p[2]/p[1] {
					return math.Pow(p[1]*x+p[2], p[0]) + p[3]
				}
				return p[3]
			case 3:
				if x >= p[4] {
					return math.Pow(p[1]*x+p[2], p[0])
				}
				return p[3] * x
			default:
				if x >= p[4] {
					return math.Pow(p[1]*x+p[2], p[0]) + p[5]
				}
				return p[3]*x + p[6]
			}
		}
	default:
		return bad()
	}
	previous := 0.0
	for i := range 256 {
		v := curve(float64(i) / 255)
		if math.IsNaN(v) || math.IsInf(v, 0) || v < previous || v > 2 {
			return bad()
		}
		table[i] = v
		previous = v
	}
	return nil
}
