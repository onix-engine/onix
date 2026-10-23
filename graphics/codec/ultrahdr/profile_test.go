package ultrahdr

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/diiyw/onix/graphics"
	"github.com/go-gl/mathgl/mgl32"
)

func TestMatrixICCAndSRGBDefault(t *testing.T) {
	// Synthetic sRGB matrix/TRC ICC, with D50-adapted primaries and sRGB curve.
	data := make([]byte, 132+6*12)
	copy(data[16:], "RGB ")
	copy(data[20:], "XYZ ")
	copy(data[36:], "acsp")
	binary.BigEndian.PutUint32(data[128:], 6)
	primaries := [3][3]float64{{.4360747, .2225045, .0139322}, {.3850649, .7168786, .0971045}, {.1430804, .0606169, .7141733}}
	for c := range 3 {
		xyz := make([]byte, 20)
		copy(xyz, "XYZ ")
		for row, v := range primaries[c] {
			binary.BigEndian.PutUint32(xyz[8+row*4:], uint32(int32(math.Round(v*65536))))
		}
		curve := make([]byte, 32)
		copy(curve, "para")
		binary.BigEndian.PutUint16(curve[8:], 3)
		for i, v := range [...]float64{2.4, 1 / 1.055, .055 / 1.055, 1 / 12.92, .04045} {
			binary.BigEndian.PutUint32(curve[12+i*4:], uint32(int32(math.Round(v*65536))))
		}
		for k, payload := range [][]byte{xyz, curve} {
			tag := data[132+(c*2+k)*12:][:12]
			name := [3]string{"r", "g", "b"}[c] + [2]string{"XYZ", "TRC"}[k]
			copy(tag, name)
			binary.BigEndian.PutUint32(tag[4:], uint32(len(data)))
			binary.BigEndian.PutUint32(tag[8:], uint32(len(payload)))
			data = append(data, payload...)
		}
	}
	binary.BigEndian.PutUint32(data, uint32(len(data)))
	transform, err := decodeProfile(data)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := decodeProfile(nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 256 {
		want := float64(graphics.SRGBToLinear(float32(i) / 255))
		if math.Abs(transform.transfer[0][i]-want) > .00005 || plain.transfer[0][i] != want {
			t.Fatalf("ICC sRGB curve[%d] differs", i)
		}
	}
	if v := transform.matrix.Mul3x1(mgl32.Vec3{.2, .4, .8}); v.Sub(mgl32.Vec3{.2, .4, .8}).Len() > .0001 {
		t.Fatal("ICC primaries transform differs", v)
	}
	for i := 0; i < len(data); i++ {
		if _, err := decodeProfile(data[:i]); i != 0 && err == nil {
			t.Fatal("accepted truncated ICC", i)
		}
	}
	binary.BigEndian.PutUint32(data[136:], 0xffffffff)
	if _, err := decodeProfile(data); err == nil {
		t.Fatal("accepted invalid ICC offset")
	}
}

func TestICCTableCurves(t *testing.T) {
	for _, values := range [][]uint16{nil, {512}, {0, 16384, 65535}} {
		data := make([]byte, 12)
		copy(data, "curv")
		binary.BigEndian.PutUint32(data[8:], uint32(len(values)))
		for _, v := range values {
			data = binary.BigEndian.AppendUint16(data, v)
		}
		var table [256]float64
		if err := decodeCurve(data, &table); err != nil {
			t.Fatal(err)
		}
		if table[0] != 0 || table[255] != 1 {
			t.Fatal("curve endpoints changed")
		}
		if len(values) == 1 && math.Abs(table[128]-math.Pow(128.0/255, 2)) > 1e-8 {
			t.Fatal("gamma curve misread")
		}
	}
}
