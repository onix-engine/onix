package ultrahdr

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/diiyw/onix/graphics"
	"github.com/go-gl/mathgl/mgl32"
)

const testMetadata = `<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"><rdf:Description xmlns:g="http://ns.adobe.com/hdr-gain-map/1.0/" g:Version="1.0" g:GainMapMax="4" g:HDRCapacityMax="4"/></rdf:RDF></x:xmpmeta>`

func segment(marker byte, payload []byte) []byte {
	out := []byte{255, marker, 0, 0}
	binary.BigEndian.PutUint16(out[2:], uint16(len(payload)+2))
	return append(out, payload...)
}
func encodeImage(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func constantImage(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}
func fixture(t *testing.T, base, gain image.Image, xml string, order binary.ByteOrder) []byte {
	t.Helper()
	primary, secondary := encodeImage(t, base), encodeImage(t, gain)
	xmp := segment(225, append([]byte(xmpHeader), []byte(xml)...))
	secondary = append(append(append([]byte{}, secondary[:2]...), xmp...), secondary[2:]...)
	tiff := make([]byte, 82)
	copy(tiff, "MM")
	if order == binary.LittleEndian {
		copy(tiff, "II")
	}
	order.PutUint16(tiff[2:], 42)
	order.PutUint32(tiff[4:], 8)
	order.PutUint16(tiff[8:], 3)
	for i, v := range []struct {
		tag, kind    uint16
		count, value uint32
	}{{0xb000, 7, 4, 0x30313030}, {0xb001, 4, 1, 2}, {0xb002, 7, 32, 50}} {
		p := tiff[10+i*12:]
		order.PutUint16(p, v.tag)
		order.PutUint16(p[2:], v.kind)
		order.PutUint32(p[4:], v.count)
		order.PutUint32(p[8:], v.value)
	}
	// APP2 length is 90; the TIFF origin is file offset 10.
	size := len(primary) + 90
	order.PutUint32(tiff[54:], uint32(size))
	order.PutUint32(tiff[70:], uint32(len(secondary)))
	order.PutUint32(tiff[74:], uint32(size-10))
	mpf := segment(226, append([]byte("MPF\x00"), tiff...))
	return append(append(append(append([]byte{}, primary[:2]...), mpf...), primary[2:]...), secondary...)
}
func TestDecodeReconstructsLinearHDR(t *testing.T) {
	base := constantImage(4, 2, color.NRGBA{R: 128, G: 64, B: 32, A: 255})
	gain := constantImage(4, 2, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	for _, order := range []binary.ByteOrder{binary.BigEndian, binary.LittleEndian} {
		data := fixture(t, base, gain, testMetadata, order)
		p, err := Decode(data)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := decodeJPEG(encodeImage(t, base))
		if err != nil {
			t.Fatal(err)
		}
		rgb := rgbAt(decoded, 0, 0)
		for c, value := range p.At(0, 0) {
			want := (float64(graphics.SRGBToLinear(float32(rgb[c])/255))+1.0/64)*16 - 1.0/64
			if math.Abs(float64(value)-want) > 1e-5 {
				t.Fatalf("channel %d = %g, want %g", c, value, want)
			}
		}
		if allocations := testing.AllocsPerRun(100, func() { _ = p.Radiance(mgl32.Vec3{1, 0, 0}) }); allocations != 0 {
			t.Fatal("sampling allocated", allocations)
		}
		atlas, err := BakeEnvironment(data, 16, 16)
		if err != nil || atlas.Bounds() != image.Rect(0, 0, 16, 64) {
			t.Fatalf("bake: %v", err)
		}
	}
}
func TestSmallerGainMapUsesBilinearRecovery(t *testing.T) {
	base := constantImage(4, 2, color.NRGBA{R: 128, G: 128, B: 128, A: 255})
	gain := image.NewGray(image.Rect(0, 0, 2, 1))
	gain.SetGray(1, 0, color.Gray{Y: 255})
	data := fixture(t, base, gain, testMetadata, binary.BigEndian)
	p, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	decoded, _ := decodeJPEG(encodeImage(t, base))
	rgb := rgbAt(decoded, 1, 0)
	gm, _ := decodeJPEG(encodeImage(t, gain))
	a, b := rgbAt(gm, 0, 0)[0], rgbAt(gm, 1, 0)[0]
	recovery := (.75*float64(a) + .25*float64(b)) / 255
	want := (float64(graphics.SRGBToLinear(float32(rgb[0])/255))+1.0/64)*math.Exp2(4*recovery) - 1.0/64
	if math.Abs(float64(p.At(1, 0)[0])-want) > 1e-5 {
		t.Fatalf("interpolated recovery = %v, want %g", p.At(1, 0), want)
	}
	if p.At(0, 0)[0] >= p.At(1, 0)[0] || p.At(1, 0)[0] >= p.At(2, 0)[0] {
		t.Fatal("gain gradient lost")
	}
}
func TestXMPArraysAndValidation(t *testing.T) {
	xml := strings.Replace(testMetadata, `g:GainMapMax="4"`, ``, 1)
	xml = strings.Replace(xml, `/>`, `><g:GainMapMax><rdf:Seq><rdf:li>1</rdf:li><rdf:li>2</rdf:li><rdf:li>3</rdf:li></rdf:Seq></g:GainMapMax></rdf:Description>`, 1)
	fields := make(map[string][]string)
	var iso []byte
	if err := readMetadata(225, append([]byte(xmpHeader), xml...), fields, &iso); err != nil {
		t.Fatal(err)
	}
	meta, err := parseXMP(fields)
	if err != nil || meta.max != ([3]float64{1, 2, 3}) {
		t.Fatalf("RGB array = %+v, %v", meta, err)
	}
	for _, bad := range []string{
		strings.Replace(testMetadata, `g:GainMapMax="4"`, "", 1),
		strings.Replace(testMetadata, `g:GainMapMax="4"`, `g:GainMapMax="NaN"`, 1),
		strings.Replace(testMetadata, `g:GainMapMax="4"`, `g:GainMapMax="-1"`, 1),
		strings.Replace(testMetadata, `g:Version="1.0"`, `g:Version="2.0"`, 1),
		strings.Replace(testMetadata, `g:Version="1.0"`, `g:Version="1.0" g:Gamma="0"`, 1),
		strings.Replace(testMetadata, `g:Version="1.0"`, `g:Version="1.0" g:BaseRenditionIsHDR="True"`, 1),
	} {
		fields = make(map[string][]string)
		if err := readMetadata(225, append([]byte(xmpHeader), bad...), fields, &iso); err == nil {
			if _, err := parseXMP(fields); err == nil {
				t.Fatalf("accepted invalid metadata %s", bad)
			}
		}
	}
}
func TestISOMetadata(t *testing.T) {
	for _, channels := range []int{1, 3} {
		for _, common := range []bool{false, true} {
			flags := byte(2)
			if channels == 3 {
				flags |= 1
			}
			if common {
				flags |= 8
			}
			data := []byte{0, 0, 0, 0, flags}
			word := func(v uint32) { data = binary.BigEndian.AppendUint32(data, v) }
			rational := func(n uint32) {
				word(n)
				if !common {
					word(64)
				}
			}
			if common {
				word(64)
			}
			rational(0)
			rational(4 * 64)
			for c := range channels {
				rational(0)
				rational(uint32(c+2) * 64)
				rational(64)
				rational(1)
				rational(1)
			}
			meta, err := parseISO(data)
			if err != nil {
				t.Fatal(err)
			}
			if meta.capacityMax != 4 || meta.max[0] != 2 || meta.sdr[0] != 1.0/64 || channels == 3 && meta.max[2] != 4 {
				t.Fatal("ISO rationals misread", meta)
			}
			for length := 0; length < len(data); length++ {
				if _, err := parseISO(data[:length]); err == nil {
					t.Fatalf("accepted ISO truncation at %d", length)
				}
			}
			if !common {
				copy(data[9:13], []byte{0, 0, 0, 0})
				if _, err := parseISO(data); err == nil {
					t.Fatal("accepted zero denominator")
				}
			}
		}
	}
}
func TestRejectsOrdinaryAndTruncatedJPEG(t *testing.T) {
	img := constantImage(4, 2, color.NRGBA{R: 128, A: 255})
	if _, err := Decode(encodeImage(t, img)); err == nil {
		t.Fatal("ordinary SDR JPEG accepted as HDR")
	}
	valid := fixture(t, img, img, testMetadata, binary.BigEndian)
	for i := 0; i < len(valid); i++ {
		if _, err := Decode(valid[:i]); err == nil {
			t.Fatalf("accepted truncation at %d", i)
		}
	}
	broken := append([]byte{}, valid...)
	binary.BigEndian.PutUint32(broken[10+74:], 0xffffffff)
	if _, err := Decode(broken); err == nil {
		t.Fatal("out-of-bounds MPF offset accepted")
	}
}
func TestRoyalEsplanadeOriginalJPEG(t *testing.T) {
	data, err := os.ReadFile("../../../testdata/resources/environment/royal_esplanade_2k.hdr.jpg")
	if err != nil {
		t.Fatal(err)
	}
	p, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if w, h := p.Size(); w != 2048 || h != 1024 {
		t.Fatal("incorrect original dimensions", w, h)
	}
	bright, dim := float32(0), 0
	for _, pixel := range p.pixels {
		for _, v := range pixel {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				t.Fatal("nonfinite radiance")
			}
			bright = max(bright, v)
		}
		if pixel[0] > .01 && pixel[0] < .5 && pixel[1] > .01 && pixel[2] > .01 {
			dim++
		}
	}
	if bright < 1000 || dim < 10000 {
		t.Fatalf("HDR range missing: brightest=%g, dim RGB pixels=%d", bright, dim)
	}
	t.Log(fmt.Sprintf("original JPEG: peak %g, %d dim RGB pixels", bright, dim))
}
func FuzzDecode(f *testing.F) {
	f.Add([]byte{255, 216, 255, 217})
	f.Fuzz(func(t *testing.T, data []byte) {
		p, err := Decode(data)
		if err != nil {
			return
		}
		for _, v := range p.Radiance(mgl32.Vec3{1, 0, 0}) {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				t.Fatal("nonfinite sample")
			}
		}
	})
}
