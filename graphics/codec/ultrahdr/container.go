package ultrahdr

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

const xmpHeader = "http://ns.adobe.com/xap/1.0/\x00"
const gainNamespace = "http://ns.adobe.com/hdr-gain-map/1.0/"
const isoHeader = "urn:iso:std:iso:ts:21496:-1\x00"

type metadata struct {
	min, max, gamma, sdr, hdr [3]float64
	capacityMin, capacityMax  float64
}

// jpegHeaders visits only length-delimited headers, stopping before entropy
// data. Marker-like bytes in EXIF, ICC or scan data cannot select another image.
func jpegHeaders(data []byte, visit func(byte, []byte, int) error) error {
	if len(data) < 2 || data[0] != 255 || data[1] != 216 {
		return fmt.Errorf("ultrahdr: missing JPEG signature")
	}
	for offset := 2; offset < len(data); {
		if data[offset] != 255 {
			return fmt.Errorf("ultrahdr: invalid JPEG marker")
		}
		for offset < len(data) && data[offset] == 255 {
			offset++
		}
		if offset >= len(data) {
			break
		}
		marker := data[offset]
		offset++
		if marker == 217 {
			return nil
		}
		if marker == 0 || marker == 216 || marker >= 208 && marker <= 215 {
			return fmt.Errorf("ultrahdr: unexpected JPEG marker")
		}
		if offset+2 > len(data) {
			break
		}
		size := int(binary.BigEndian.Uint16(data[offset:]))
		end := offset + size
		if size < 2 || end > len(data) {
			break
		}
		if err := visit(marker, data[offset+2:end], offset+2); err != nil {
			return err
		}
		if marker == 218 {
			return nil
		}
		offset = end
	}
	return fmt.Errorf("ultrahdr: truncated JPEG headers")
}

func container(data []byte) (base, gain []byte, m metadata, profile []byte, err error) {
	fields := make(map[string][]string)
	var iso []byte
	var chunks [256][]byte
	count := 0
	err = jpegHeaders(data, func(marker byte, payload []byte, offset int) error {
		if marker == 226 && bytes.HasPrefix(payload, []byte("MPF\x00")) {
			if base != nil {
				return fmt.Errorf("ultrahdr: duplicate MPF index")
			}
			var e error
			base, gain, e = mpfImages(data, payload[4:], offset+4)
			return e
		}
		if marker == 226 && bytes.HasPrefix(payload, []byte("ICC_PROFILE\x00")) {
			if len(payload) < 14 {
				return fmt.Errorf("ultrahdr: truncated ICC segment")
			}
			seq, total := int(payload[12]), int(payload[13])
			if seq == 0 || seq > total || count != 0 && count != total || chunks[seq] != nil {
				return fmt.Errorf("ultrahdr: invalid ICC sequence")
			}
			count = total
			chunks[seq] = payload[14:]
			return nil
		}
		return readMetadata(marker, payload, fields, &iso)
	})
	if err != nil {
		return
	}
	if base == nil || gain == nil {
		err = fmt.Errorf("ultrahdr: missing two-image MPF index")
		return
	}
	err = jpegHeaders(gain, func(marker byte, payload []byte, _ int) error { return readMetadata(marker, payload, fields, &iso) })
	if err != nil {
		return
	}
	if iso != nil {
		m, err = parseISO(iso)
	} else {
		m, err = parseXMP(fields)
	}
	if err != nil {
		return
	}
	for i := 1; i <= count; i++ {
		if chunks[i] == nil {
			err = fmt.Errorf("ultrahdr: incomplete ICC profile")
			return
		}
		profile = append(profile, chunks[i]...)
	}
	return
}

func mpfImages(data, tiff []byte, origin int) ([]byte, []byte, error) {
	fail := func() ([]byte, []byte, error) { return nil, nil, fmt.Errorf("ultrahdr: invalid MPF index") }
	if len(tiff) < 8 {
		return fail()
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "MM":
		order = binary.BigEndian
	case "II":
		order = binary.LittleEndian
	default:
		return fail()
	}
	if order.Uint16(tiff[2:]) != 42 {
		return fail()
	}
	start := uint64(order.Uint32(tiff[4:]))
	if start+2 > uint64(len(tiff)) {
		return fail()
	}
	tags := int(order.Uint16(tiff[start:]))
	if start+2+uint64(tags)*12+4 > uint64(len(tiff)) {
		return fail()
	}
	var entries []byte
	images := uint32(0)
	for i := range tags {
		tag := tiff[int(start)+2+i*12:][:12]
		switch order.Uint16(tag) {
		case 0xb001:
			if order.Uint16(tag[2:]) != 4 || order.Uint32(tag[4:]) != 1 {
				return fail()
			}
			images = order.Uint32(tag[8:])
		case 0xb002:
			size, pos := uint64(order.Uint32(tag[4:])), uint64(order.Uint32(tag[8:]))
			if order.Uint16(tag[2:]) != 7 || pos+size > uint64(len(tiff)) {
				return fail()
			}
			entries = tiff[pos : pos+size]
		}
	}
	if images != 2 || len(entries) != 32 {
		return fail()
	}
	primarySize := uint64(order.Uint32(entries[4:]))
	primaryOffset := order.Uint32(entries[8:])
	gainSize := uint64(order.Uint32(entries[20:]))
	gainOffset := uint64(origin) + uint64(order.Uint32(entries[24:]))
	if primaryOffset != 0 || primarySize < 2 || primarySize > uint64(len(data)) || gainSize < 2 || gainOffset < primarySize || gainOffset+gainSize > uint64(len(data)) {
		return fail()
	}
	if !bytes.Equal(data[primarySize-2:primarySize], []byte{255, 217}) || !bytes.Equal(data[gainOffset+gainSize-2:gainOffset+gainSize], []byte{255, 217}) {
		return fail()
	}
	return data[:primarySize], data[gainOffset : gainOffset+gainSize], nil
}

func readMetadata(marker byte, payload []byte, fields map[string][]string, iso *[]byte) error {
	if marker == 226 && bytes.HasPrefix(payload, []byte(isoHeader)) {
		// The primary image may contain only the ISO version advertisement.
		body := payload[len(isoHeader):]
		if len(body) > 4 {
			*iso = body
		}
	}
	if marker != 225 || !bytes.HasPrefix(payload, []byte(xmpHeader)) {
		return nil
	}
	decoder := xml.NewDecoder(bytes.NewReader(bytes.TrimRight(payload[len(xmpHeader):], "\x00")))
	property := ""
	depth := 0
	propertyDepth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("ultrahdr: invalid XMP: %w", err)
		}
		switch t := token.(type) {
		case xml.StartElement:
			depth++
			for _, a := range t.Attr {
				if a.Name.Space == gainNamespace {
					fields[a.Name.Local] = []string{a.Value}
				}
			}
			if t.Name.Space == gainNamespace {
				property = t.Name.Local
				propertyDepth = depth
				fields[property] = nil
			}
		case xml.CharData:
			if property != "" {
				if value := strings.TrimSpace(string(t)); value != "" {
					fields[property] = append(fields[property], value)
				}
			}
		case xml.EndElement:
			if depth == propertyDepth {
				property = ""
			}
			depth--
		}
	}
}

func parseXMP(fields map[string][]string) (metadata, error) {
	var m metadata
	if v := fields["Version"]; len(v) != 1 || v[0] != "1.0" {
		return m, fmt.Errorf("ultrahdr: missing supported gain map version")
	}
	if v := fields["BaseRenditionIsHDR"]; len(v) > 0 && (len(v) != 1 || v[0] != "False") {
		return m, fmt.Errorf("ultrahdr: HDR primary image is unsupported")
	}
	targets := []struct {
		name     string
		value    *[3]float64
		fallback float64
		required bool
	}{
		{"GainMapMin", &m.min, 0, false}, {"GainMapMax", &m.max, 0, true}, {"Gamma", &m.gamma, 1, false},
		{"OffsetSDR", &m.sdr, 1.0 / 64, false}, {"OffsetHDR", &m.hdr, 1.0 / 64, false},
	}
	for _, target := range targets {
		values := fields[target.name]
		if len(values) == 0 && !target.required {
			*target.value = [3]float64{target.fallback, target.fallback, target.fallback}
			continue
		}
		if len(values) != 1 && len(values) != 3 {
			return m, fmt.Errorf("ultrahdr: invalid %s", target.name)
		}
		for c := range 3 {
			value, err := strconv.ParseFloat(values[min(c, len(values)-1)], 64)
			if err != nil {
				return m, fmt.Errorf("ultrahdr: invalid %s", target.name)
			}
			target.value[c] = value
		}
	}
	for _, target := range []struct {
		name     string
		value    *float64
		required bool
	}{{"HDRCapacityMin", &m.capacityMin, false}, {"HDRCapacityMax", &m.capacityMax, true}} {
		v := fields[target.name]
		if len(v) == 0 && !target.required {
			continue
		}
		if len(v) != 1 {
			return m, fmt.Errorf("ultrahdr: invalid %s", target.name)
		}
		value, err := strconv.ParseFloat(v[0], 64)
		if err != nil {
			return m, fmt.Errorf("ultrahdr: invalid %s", target.name)
		}
		*target.value = value
	}
	return m, m.validate()
}

func (m metadata) validate() error {
	for c := range 3 {
		for _, v := range [...]float64{m.min[c], m.max[c], m.gamma[c], m.sdr[c], m.hdr[c], m.capacityMin, m.capacityMax} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return fmt.Errorf("ultrahdr: non-finite gain map metadata")
			}
		}
		if m.min[c] < -120 || m.max[c] > 120 || m.min[c] > m.max[c] || m.gamma[c] <= 0 || m.sdr[c] < 0 || m.hdr[c] < 0 || m.capacityMin < 0 || m.capacityMax <= m.capacityMin {
			return fmt.Errorf("ultrahdr: gain map metadata outside valid range")
		}
	}
	return nil
}

func parseISO(data []byte) (metadata, error) {
	var m metadata
	if len(data) < 5 || binary.BigEndian.Uint16(data) != 0 {
		return m, fmt.Errorf("ultrahdr: unsupported ISO metadata version")
	}
	flags := data[4]
	if flags&4 != 0 || flags&2 == 0 || flags&0xf0 != 0 {
		return m, fmt.Errorf("ultrahdr: ISO HDR primary or alternate color space is unsupported")
	}
	channels := 1
	if flags&1 != 0 {
		channels = 3
	}
	pos := 5
	common := uint32(0)
	read := func() (uint32, error) {
		if pos+4 > len(data) {
			return 0, fmt.Errorf("ultrahdr: truncated ISO metadata")
		}
		v := binary.BigEndian.Uint32(data[pos:])
		pos += 4
		return v, nil
	}
	if flags&8 != 0 {
		v, err := read()
		if err != nil {
			return m, err
		}
		if v == 0 {
			return m, fmt.Errorf("ultrahdr: zero ISO denominator")
		}
		common = v
	}
	rational := func(signed bool) (float64, error) {
		n, err := read()
		if err != nil {
			return 0, err
		}
		d := common
		if d == 0 {
			d, err = read()
			if err != nil {
				return 0, err
			}
		}
		if d == 0 {
			return 0, fmt.Errorf("ultrahdr: zero ISO denominator")
		}
		value := float64(n)
		if signed {
			value = float64(int32(n))
		}
		return value / float64(d), nil
	}
	var err error
	m.capacityMin, err = rational(false)
	if err != nil {
		return m, err
	}
	m.capacityMax, err = rational(false)
	if err != nil {
		return m, err
	}
	for c := range channels {
		for _, target := range []struct {
			value  *[3]float64
			signed bool
		}{{&m.min, true}, {&m.max, true}, {&m.gamma, false}, {&m.sdr, true}, {&m.hdr, true}} {
			target.value[c], err = rational(target.signed)
			if err != nil {
				return m, err
			}
		}
	}
	if channels == 1 {
		for _, v := range []*[3]float64{&m.min, &m.max, &m.gamma, &m.sdr, &m.hdr} {
			v[1], v[2] = v[0], v[0]
		}
	}
	return m, m.validate()
}
