package woff2

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/diiyw/onix/font"
	"github.com/diiyw/onix/internal/brotli"
)

// Decoder handles WOFF2 fonts. Importing this package is the link boundary
// for WOFF2 reconstruction and Brotli.
type Decoder struct{}

type Point = font.Point
type Bounds = font.Bounds

const (
	flagOnCurve         = 0x01
	flagXShortVector    = 0x02
	flagYShortVector    = 0x04
	flagRepeat          = 0x08
	flagXSameOrPositive = 0x10
	flagYSameOrPositive = 0x20

	compositeArg1And2AreWords   = 0x0001
	compositeWeHaveAScale       = 0x0008
	compositeMoreComponents     = 0x0020
	compositeWeHaveAnXAndYScale = 0x0040
	compositeWeHaveATwoByTwo    = 0x0080
	compositeWeHaveInstructions = 0x0100
)

func (Decoder) MatchesFont(data []byte, path string) bool {
	return isWOFF2(data) || strings.EqualFold(filepath.Ext(path), ".woff2")
}

func (Decoder) DecodeFont(data []byte, _ string) (*font.Face, error) {
	sfntData, err := decodeWOFF2(data)
	if err != nil {
		return nil, fmt.Errorf("decode woff2: %w", err)
	}
	return font.Parse(sfntData)
}

const (
	woff2Signature = 0x774f4632
	woff2HeaderLen = 48
	woff2CustomTag = 0x3f
)

var woff2KnownTags = []string{
	"cmap", "head", "hhea", "hmtx", "maxp", "name", "OS/2", "post",
	"cvt ", "fpgm", "glyf", "loca", "prep", "CFF ", "VORG", "EBDT",
	"EBLC", "gasp", "hdmx", "kern", "LTSH", "PCLT", "VDMX", "vhea",
	"vmtx", "BASE", "GDEF", "GPOS", "GSUB", "EBSC", "JSTF", "MATH",
	"CBDT", "CBLC", "COLR", "CPAL", "SVG ", "sbix", "acnt", "avar",
	"bdat", "bloc", "bsln", "cvar", "fdsc", "feat", "fmtx", "fvar",
	"gvar", "hsty", "just", "lcar", "mort", "morx", "opbd", "prop",
	"trak", "Zapf", "Silf", "Glat", "Gloc", "Feat", "Sill",
}

type woff2Table struct {
	tag               string
	origLength        uint32
	transformLength   uint32
	transformVersion  byte
	transformed       bool
	decompressedStart uint32
}

func isWOFF2(data []byte) bool {
	return len(data) >= 4 && binary.BigEndian.Uint32(data[:4]) == woff2Signature
}

func decodeWOFF2(data []byte) ([]byte, error) {
	if len(data) < woff2HeaderLen {
		return nil, fmt.Errorf("woff2 header too short")
	}
	if binary.BigEndian.Uint32(data[:4]) != woff2Signature {
		return nil, fmt.Errorf("invalid woff2 signature")
	}

	flavor := binary.BigEndian.Uint32(data[4:8])
	length := binary.BigEndian.Uint32(data[8:12])
	if int(length) != len(data) {
		return nil, fmt.Errorf("woff2 length = %d, want %d", length, len(data))
	}
	numTables := int(binary.BigEndian.Uint16(data[12:14]))
	if numTables == 0 {
		return nil, fmt.Errorf("woff2 has no tables")
	}
	if reserved := binary.BigEndian.Uint16(data[14:16]); reserved != 0 {
		return nil, fmt.Errorf("woff2 reserved field = %d", reserved)
	}
	totalSFNTSize := binary.BigEndian.Uint32(data[16:20])
	totalCompressedSize := binary.BigEndian.Uint32(data[20:24])
	if totalCompressedSize > uint32(len(data)-woff2HeaderLen) {
		return nil, fmt.Errorf("woff2 compressed block out of bounds")
	}

	tables, dirEnd, decompressedSize, err := parseWOFF2Directory(data[woff2HeaderLen:], numTables)
	if err != nil {
		return nil, err
	}
	for _, table := range tables {
		if table.transformed && table.tag != "glyf" && table.tag != "loca" {
			return nil, fmt.Errorf("woff2 transformed table %q is not supported", table.tag)
		}
	}

	compressedEnd := dirEnd + int(totalCompressedSize)
	if compressedEnd > len(data) {
		return nil, fmt.Errorf("woff2 compressed block truncated")
	}
	block, err := decompressWOFF2(data[dirEnd:compressedEnd])
	if err != nil {
		return nil, err
	}
	if len(block) != int(decompressedSize) {
		return nil, fmt.Errorf("woff2 decompressed size = %d, want %d", len(block), decompressedSize)
	}

	sfntData, err := buildSFNT(flavor, tables, block)
	if err != nil {
		return nil, err
	}
	if !hasTransformedGlyf(tables) && len(sfntData) != int(totalSFNTSize) {
		return nil, fmt.Errorf("woff2 sfnt size = %d, want %d", len(sfntData), totalSFNTSize)
	}
	return sfntData, nil
}

func parseWOFF2Directory(data []byte, numTables int) ([]woff2Table, int, uint32, error) {
	tables := make([]woff2Table, numTables)
	offset := 0
	var decompressedSize uint32
	for i := range tables {
		if offset >= len(data) {
			return nil, 0, 0, fmt.Errorf("woff2 table directory truncated")
		}
		flags := data[offset]
		offset++

		tagIndex := flags & 0x3f
		transformVersion := flags >> 6
		tag := ""
		if tagIndex == woff2CustomTag {
			if offset+4 > len(data) {
				return nil, 0, 0, fmt.Errorf("woff2 custom tag truncated")
			}
			tag = string(data[offset : offset+4])
			offset += 4
		} else {
			if int(tagIndex) >= len(woff2KnownTags) {
				return nil, 0, 0, fmt.Errorf("woff2 known tag index %d out of range", tagIndex)
			}
			tag = woff2KnownTags[tagIndex]
		}

		origLength, n, err := readUIntBase128(data[offset:])
		if err != nil {
			return nil, 0, 0, fmt.Errorf("read woff2 table %q length: %w", tag, err)
		}
		offset += n

		transformed := tableIsTransformed(tag, transformVersion)
		transformLength := origLength
		if transformed {
			transformLength, n, err = readUIntBase128(data[offset:])
			if err != nil {
				return nil, 0, 0, fmt.Errorf("read woff2 table %q transform length: %w", tag, err)
			}
			offset += n
		}
		if decompressedSize > ^uint32(0)-transformLength {
			return nil, 0, 0, fmt.Errorf("woff2 decompressed size overflow")
		}
		tables[i] = woff2Table{
			tag:               tag,
			origLength:        origLength,
			transformLength:   transformLength,
			transformVersion:  transformVersion,
			transformed:       transformed,
			decompressedStart: decompressedSize,
		}
		decompressedSize += transformLength
	}
	return tables, woff2HeaderLen + offset, decompressedSize, nil
}

func tableIsTransformed(tag string, transformVersion byte) bool {
	if tag == "glyf" || tag == "loca" {
		return transformVersion != 3
	}
	return transformVersion != 0
}

func readUIntBase128(data []byte) (uint32, int, error) {
	var value uint32
	for i := 0; i < 5; i++ {
		if i >= len(data) {
			return 0, 0, fmt.Errorf("truncated UIntBase128")
		}
		b := data[i]
		if i == 0 && b == 0x80 {
			return 0, 0, fmt.Errorf("leading zero in UIntBase128")
		}
		if value&0xfe000000 != 0 {
			return 0, 0, fmt.Errorf("UIntBase128 overflow")
		}
		value = (value << 7) | uint32(b&0x7f)
		if b&0x80 == 0 {
			return value, i + 1, nil
		}
	}
	return 0, 0, fmt.Errorf("UIntBase128 too long")
}

func decompressWOFF2(data []byte) ([]byte, error) {
	reader := brotli.NewReader(bytes.NewReader(data))
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildSFNT(flavor uint32, tables []woff2Table, block []byte) ([]byte, error) {
	tableData, err := woff2TableData(tables, block)
	if err != nil {
		return nil, err
	}

	sfntLen := 12 + len(tables)*16
	for _, table := range tables {
		sfntLen += paddedLength(len(tableData[table.tag]))
	}

	out := make([]byte, sfntLen)
	binary.BigEndian.PutUint32(out[0:4], flavor)
	binary.BigEndian.PutUint16(out[4:6], uint16(len(tables)))
	searchRange, entrySelector, rangeShift := sfntSearchParams(len(tables))
	binary.BigEndian.PutUint16(out[6:8], searchRange)
	binary.BigEndian.PutUint16(out[8:10], entrySelector)
	binary.BigEndian.PutUint16(out[10:12], rangeShift)

	tableOffset := 12 + len(tables)*16
	for i, table := range tables {
		entry := 12 + i*16
		copy(out[entry:entry+4], table.tag)
		data := tableData[table.tag]
		binary.BigEndian.PutUint32(out[entry+4:entry+8], checksum(data))
		binary.BigEndian.PutUint32(out[entry+8:entry+12], uint32(tableOffset))
		binary.BigEndian.PutUint32(out[entry+12:entry+16], uint32(len(data)))
		copy(out[tableOffset:tableOffset+len(data)], data)
		tableOffset += paddedLength(len(data))
	}

	return out, nil
}

func woff2TableData(tables []woff2Table, block []byte) (map[string][]byte, error) {
	data := make(map[string][]byte, len(tables))
	var glyfTable *woff2Table
	for i := range tables {
		table := &tables[i]
		if table.tag == "glyf" && table.transformed {
			glyfTable = table
			continue
		}
		if table.tag == "loca" && table.transformed {
			continue
		}
		start := int(table.decompressedStart)
		end := start + int(table.origLength)
		if table.transformed {
			end = start + int(table.transformLength)
		}
		if end > len(block) || end < start {
			return nil, fmt.Errorf("woff2 table %q data out of bounds", table.tag)
		}
		data[table.tag] = block[start:end]
	}
	if glyfTable != nil {
		start := int(glyfTable.decompressedStart)
		end := start + int(glyfTable.transformLength)
		if end > len(block) || end < start {
			return nil, fmt.Errorf("woff2 table %q data out of bounds", glyfTable.tag)
		}
		glyf, loca, err := reconstructWOFF2Glyf(block[start:end])
		if err != nil {
			return nil, err
		}
		data["glyf"] = glyf
		data["loca"] = loca
	}
	for _, table := range tables {
		if _, ok := data[table.tag]; !ok {
			return nil, fmt.Errorf("woff2 missing reconstructed table %q", table.tag)
		}
	}
	return data, nil
}

func hasTransformedGlyf(tables []woff2Table) bool {
	for _, table := range tables {
		if table.tag == "glyf" && table.transformed {
			return true
		}
	}
	return false
}

type woff2GlyphStreams struct {
	nContour          []byte
	nPoints           []byte
	flags             []byte
	glyph             []byte
	composite         []byte
	bbox              []byte
	instructions      []byte
	overlapSimple     []byte
	nPointsOffset     int
	flagsOffset       int
	glyphOffset       int
	compositeOffset   int
	bboxOffset        int
	instructionOffset int
}

func reconstructWOFF2Glyf(data []byte) ([]byte, []byte, error) {
	if len(data) < 36 {
		return nil, nil, fmt.Errorf("woff2 transformed glyf header too short")
	}
	reserved := binary.BigEndian.Uint16(data[0:2])
	if reserved != 0 {
		return nil, nil, fmt.Errorf("woff2 transformed glyf reserved = %d", reserved)
	}
	optionFlags := binary.BigEndian.Uint16(data[2:4])
	if optionFlags&^uint16(1) != 0 {
		return nil, nil, fmt.Errorf("woff2 transformed glyf option flags = %d", optionFlags)
	}
	numGlyphs := int(binary.BigEndian.Uint16(data[4:6]))
	indexFormat := int16(binary.BigEndian.Uint16(data[6:8]))
	if indexFormat != 0 && indexFormat != 1 {
		return nil, nil, fmt.Errorf("woff2 transformed glyf index format = %d", indexFormat)
	}

	streamSizes := [7]int{}
	for i := range streamSizes {
		size := binary.BigEndian.Uint32(data[8+i*4 : 12+i*4])
		if size > uint32(len(data)) {
			return nil, nil, fmt.Errorf("woff2 transformed glyf stream too large")
		}
		streamSizes[i] = int(size)
	}

	offset := 36
	nextStream := func(size int) ([]byte, error) {
		if size < 0 || offset+size > len(data) || offset+size < offset {
			return nil, fmt.Errorf("woff2 transformed glyf stream truncated")
		}
		stream := data[offset : offset+size]
		offset += size
		return stream, nil
	}

	var streams woff2GlyphStreams
	var err error
	if streams.nContour, err = nextStream(streamSizes[0]); err != nil {
		return nil, nil, err
	}
	if streams.nPoints, err = nextStream(streamSizes[1]); err != nil {
		return nil, nil, err
	}
	if streams.flags, err = nextStream(streamSizes[2]); err != nil {
		return nil, nil, err
	}
	if streams.glyph, err = nextStream(streamSizes[3]); err != nil {
		return nil, nil, err
	}
	if streams.composite, err = nextStream(streamSizes[4]); err != nil {
		return nil, nil, err
	}
	if streams.bbox, err = nextStream(streamSizes[5]); err != nil {
		return nil, nil, err
	}
	if streams.instructions, err = nextStream(streamSizes[6]); err != nil {
		return nil, nil, err
	}
	if optionFlags&1 != 0 {
		bitmapSize := woff2GlyphBitmapSize(numGlyphs)
		if streams.overlapSimple, err = nextStream(bitmapSize); err != nil {
			return nil, nil, err
		}
	}
	if offset != len(data) {
		return nil, nil, fmt.Errorf("woff2 transformed glyf has trailing data")
	}
	if len(streams.nContour) != numGlyphs*2 {
		return nil, nil, fmt.Errorf("woff2 transformed glyf nContour size = %d, want %d", len(streams.nContour), numGlyphs*2)
	}
	if len(streams.bbox) < woff2GlyphBitmapSize(numGlyphs) {
		return nil, nil, fmt.Errorf("woff2 transformed glyf bbox stream too short")
	}
	streams.bboxOffset = woff2GlyphBitmapSize(numGlyphs)

	offsets := make([]uint32, numGlyphs+1)
	var glyf bytes.Buffer
	for glyphID := 0; glyphID < numGlyphs; glyphID++ {
		offsets[glyphID] = uint32(glyf.Len())
		nContours := int16(binary.BigEndian.Uint16(streams.nContour[glyphID*2 : glyphID*2+2]))
		if nContours == 0 {
			if woff2GlyphBit(streams.bbox, glyphID) {
				return nil, nil, fmt.Errorf("woff2 empty glyph %d has explicit bbox", glyphID)
			}
			continue
		}

		glyphData, err := streams.reconstructGlyph(glyphID, nContours)
		if err != nil {
			return nil, nil, fmt.Errorf("reconstruct glyph %d: %w", glyphID, err)
		}
		glyf.Write(glyphData)
		for glyf.Len()%4 != 0 {
			glyf.WriteByte(0)
		}
	}
	offsets[numGlyphs] = uint32(glyf.Len())

	loca, err := buildWOFF2Loca(offsets, indexFormat)
	if err != nil {
		return nil, nil, err
	}
	if streams.nPointsOffset != len(streams.nPoints) || streams.flagsOffset != len(streams.flags) ||
		streams.glyphOffset != len(streams.glyph) || streams.compositeOffset != len(streams.composite) ||
		streams.instructionOffset != len(streams.instructions) || streams.bboxOffset != len(streams.bbox) {
		return nil, nil, fmt.Errorf("woff2 transformed glyf stream data remains")
	}
	return glyf.Bytes(), loca, nil
}

func (s *woff2GlyphStreams) reconstructGlyph(glyphID int, nContours int16) ([]byte, error) {
	var body bytes.Buffer
	var bounds Bounds
	var err error
	if nContours > 0 {
		body, bounds, err = s.reconstructSimpleGlyph(glyphID, int(nContours))
	} else if nContours == -1 {
		body, bounds, err = s.reconstructCompositeGlyph(glyphID)
	} else {
		return nil, fmt.Errorf("unsupported contour count %d", nContours)
	}
	if err != nil {
		return nil, err
	}

	var out bytes.Buffer
	writeInt16(&out, nContours)
	writeInt16(&out, bounds.XMin)
	writeInt16(&out, bounds.YMin)
	writeInt16(&out, bounds.XMax)
	writeInt16(&out, bounds.YMax)
	out.Write(body.Bytes())
	return out.Bytes(), nil
}

func (s *woff2GlyphStreams) reconstructSimpleGlyph(glyphID int, nContours int) (bytes.Buffer, Bounds, error) {
	var body bytes.Buffer
	endPts := make([]uint16, nContours)
	pointCount := 0
	for i := 0; i < nContours; i++ {
		n, err := s.readNPoints()
		if err != nil {
			return body, Bounds{}, err
		}
		if n == 0 || pointCount > int(^uint16(0))-int(n) {
			return body, Bounds{}, fmt.Errorf("invalid contour point count %d", n)
		}
		pointCount += int(n)
		endPts[i] = uint16(pointCount - 1)
	}
	for _, endPt := range endPts {
		writeUint16(&body, endPt)
	}

	woff2Flags, err := s.readFlags(pointCount)
	if err != nil {
		return body, Bounds{}, err
	}
	points := make([]Point, pointCount)
	var x, y int
	for i, flag := range woff2Flags {
		dx, dy, err := s.readTriplet(flag)
		if err != nil {
			return body, Bounds{}, err
		}
		x += dx
		y += dy
		if x < -32768 || x > 32767 || y < -32768 || y > 32767 {
			return body, Bounds{}, fmt.Errorf("glyph coordinate out of int16 range")
		}
		points[i] = Point{X: int16(x), Y: int16(y), OnCurve: flag&0x80 == 0}
	}

	instructionLength, err := s.readGlyph255UInt16()
	if err != nil {
		return body, Bounds{}, err
	}
	instructions, err := s.readInstructions(int(instructionLength))
	if err != nil {
		return body, Bounds{}, err
	}
	writeUint16(&body, instructionLength)
	body.Write(instructions)

	ttFlags, xData, yData := encodeTrueTypePoints(points, woff2GlyphBit(s.overlapSimple, glyphID))
	body.Write(ttFlags)
	body.Write(xData)
	body.Write(yData)

	bounds := inferBounds(points)
	if woff2GlyphBit(s.bbox, glyphID) {
		bounds, err = s.readBounds()
		if err != nil {
			return body, Bounds{}, err
		}
	}
	return body, bounds, nil
}

func (s *woff2GlyphStreams) reconstructCompositeGlyph(glyphID int) (bytes.Buffer, Bounds, error) {
	var body bytes.Buffer
	hasInstructions := false
	for {
		flags, err := s.readCompositeUint16()
		if err != nil {
			return body, Bounds{}, err
		}
		writeUint16(&body, flags)
		argBytes := compositeGlyphArgumentSize(flags)
		args, err := s.readCompositeBytes(argBytes)
		if err != nil {
			return body, Bounds{}, err
		}
		body.Write(args)
		if flags&compositeWeHaveInstructions != 0 {
			hasInstructions = true
		}
		if flags&compositeMoreComponents == 0 {
			break
		}
	}
	if !woff2GlyphBit(s.bbox, glyphID) {
		return body, Bounds{}, fmt.Errorf("composite glyph missing explicit bbox")
	}
	if hasInstructions {
		instructionLength, err := s.readGlyph255UInt16()
		if err != nil {
			return body, Bounds{}, err
		}
		instructions, err := s.readInstructions(int(instructionLength))
		if err != nil {
			return body, Bounds{}, err
		}
		writeUint16(&body, instructionLength)
		body.Write(instructions)
	}
	bounds, err := s.readBounds()
	if err != nil {
		return body, Bounds{}, err
	}
	return body, bounds, nil
}

func (s *woff2GlyphStreams) readNPoints() (uint16, error) {
	value, next, err := read255UInt16(s.nPoints, s.nPointsOffset)
	if err != nil {
		return 0, err
	}
	s.nPointsOffset = next
	return value, nil
}

func (s *woff2GlyphStreams) readFlags(n int) ([]byte, error) {
	if n < 0 || s.flagsOffset+n > len(s.flags) {
		return nil, fmt.Errorf("glyph flags stream truncated")
	}
	flags := s.flags[s.flagsOffset : s.flagsOffset+n]
	s.flagsOffset += n
	return flags, nil
}

func (s *woff2GlyphStreams) readTriplet(flag byte) (int, int, error) {
	index := flag & 0x7f
	need := woff2TripletByteCount(index) - 1
	if need < 0 || s.glyphOffset+need > len(s.glyph) {
		return 0, 0, fmt.Errorf("glyph coordinate stream truncated")
	}
	data := s.glyph[s.glyphOffset : s.glyphOffset+need]
	s.glyphOffset += need
	return decodeWOFF2Triplet(index, data)
}

func (s *woff2GlyphStreams) readGlyph255UInt16() (uint16, error) {
	value, next, err := read255UInt16(s.glyph, s.glyphOffset)
	if err != nil {
		return 0, err
	}
	s.glyphOffset = next
	return value, nil
}

func (s *woff2GlyphStreams) readInstructions(n int) ([]byte, error) {
	if n < 0 || s.instructionOffset+n > len(s.instructions) {
		return nil, fmt.Errorf("glyph instruction stream truncated")
	}
	data := s.instructions[s.instructionOffset : s.instructionOffset+n]
	s.instructionOffset += n
	return data, nil
}

func (s *woff2GlyphStreams) readCompositeUint16() (uint16, error) {
	data, err := s.readCompositeBytes(2)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(data), nil
}

func (s *woff2GlyphStreams) readCompositeBytes(n int) ([]byte, error) {
	if n < 0 || s.compositeOffset+n > len(s.composite) {
		return nil, fmt.Errorf("glyph composite stream truncated")
	}
	data := s.composite[s.compositeOffset : s.compositeOffset+n]
	s.compositeOffset += n
	return data, nil
}

func (s *woff2GlyphStreams) readBounds() (Bounds, error) {
	if s.bboxOffset+8 > len(s.bbox) {
		return Bounds{}, fmt.Errorf("glyph bbox stream truncated")
	}
	bounds := Bounds{
		XMin: int16(binary.BigEndian.Uint16(s.bbox[s.bboxOffset : s.bboxOffset+2])),
		YMin: int16(binary.BigEndian.Uint16(s.bbox[s.bboxOffset+2 : s.bboxOffset+4])),
		XMax: int16(binary.BigEndian.Uint16(s.bbox[s.bboxOffset+4 : s.bboxOffset+6])),
		YMax: int16(binary.BigEndian.Uint16(s.bbox[s.bboxOffset+6 : s.bboxOffset+8])),
	}
	s.bboxOffset += 8
	return bounds, nil
}

func read255UInt16(data []byte, offset int) (uint16, int, error) {
	if offset >= len(data) {
		return 0, offset, fmt.Errorf("255UInt16 truncated")
	}
	code := data[offset]
	offset++
	switch code {
	case 253:
		if offset+2 > len(data) {
			return 0, offset, fmt.Errorf("255UInt16 word truncated")
		}
		value := binary.BigEndian.Uint16(data[offset : offset+2])
		return value, offset + 2, nil
	case 254:
		if offset >= len(data) {
			return 0, offset, fmt.Errorf("255UInt16 one-byte truncated")
		}
		return uint16(506 + uint16(data[offset])), offset + 1, nil
	case 255:
		if offset >= len(data) {
			return 0, offset, fmt.Errorf("255UInt16 one-byte truncated")
		}
		return uint16(253 + uint16(data[offset])), offset + 1, nil
	default:
		return uint16(code), offset, nil
	}
}

func woff2TripletByteCount(index byte) int {
	switch {
	case index < 84:
		return 2
	case index < 120:
		return 3
	case index < 124:
		return 4
	default:
		return 5
	}
}

func decodeWOFF2Triplet(index byte, data []byte) (int, int, error) {
	switch {
	case index < 10:
		dy := int(data[0]) + int(index/2)*256
		if index&1 == 0 {
			dy = -dy
		}
		return 0, dy, nil
	case index < 20:
		dx := int(data[0]) + int((index-10)/2)*256
		if index&1 == 0 {
			dx = -dx
		}
		return dx, 0, nil
	case index < 84:
		group := int(index - 20)
		dx := int(data[0]>>4) + (group/16)*16 + 1
		dy := int(data[0]&0x0f) + ((group%16)/4)*16 + 1
		dx, dy = applyWOFF2TripletSigns(group, dx, dy)
		return dx, dy, nil
	case index < 120:
		group := int(index - 84)
		dx := int(data[0]) + (group/12)*256 + 1
		dy := int(data[1]) + ((group%12)/4)*256 + 1
		dx, dy = applyWOFF2TripletSigns(group, dx, dy)
		return dx, dy, nil
	case index < 124:
		group := int(index - 120)
		dx := (int(data[0]) << 4) | int(data[1]>>4)
		dy := (int(data[1]&0x0f) << 8) | int(data[2])
		dx++
		dy++
		dx, dy = applyWOFF2TripletSigns(group, dx, dy)
		return dx, dy, nil
	default:
		group := int(index - 124)
		dx := int(binary.BigEndian.Uint16(data[0:2]))
		dy := int(binary.BigEndian.Uint16(data[2:4]))
		dx, dy = applyWOFF2TripletSigns(group, dx, dy)
		return dx, dy, nil
	}
}

func applyWOFF2TripletSigns(group, dx, dy int) (int, int) {
	if group&1 == 0 {
		dx = -dx
	}
	if group&2 == 0 {
		dy = -dy
	}
	return dx, dy
}

func encodeTrueTypePoints(points []Point, overlapSimple bool) ([]byte, []byte, []byte) {
	rawFlags := make([]byte, len(points))
	var xData bytes.Buffer
	var yData bytes.Buffer
	var lastX, lastY int16
	for i, point := range points {
		flag := byte(0)
		if point.OnCurve {
			flag |= flagOnCurve
		}
		if i == 0 && overlapSimple {
			flag |= 0x40
		}
		dx := int(point.X - lastX)
		dy := int(point.Y - lastY)
		flag = encodeTrueTypeCoordinate(flag, dx, flagXShortVector, flagXSameOrPositive, &xData)
		flag = encodeTrueTypeCoordinate(flag, dy, flagYShortVector, flagYSameOrPositive, &yData)
		rawFlags[i] = flag
		lastX = point.X
		lastY = point.Y
	}
	return encodeTrueTypeFlags(rawFlags), xData.Bytes(), yData.Bytes()
}

func encodeTrueTypeCoordinate(flag byte, delta int, shortFlag, sameFlag byte, out *bytes.Buffer) byte {
	if delta == 0 {
		return flag | sameFlag
	}
	if delta > 0 && delta <= 255 {
		out.WriteByte(byte(delta))
		return flag | shortFlag | sameFlag
	}
	if delta < 0 && delta >= -255 {
		out.WriteByte(byte(-delta))
		return flag | shortFlag
	}
	writeInt16(out, int16(delta))
	return flag
}

func encodeTrueTypeFlags(flags []byte) []byte {
	out := make([]byte, 0, len(flags))
	for i := 0; i < len(flags); i++ {
		flag := flags[i]
		run := 0
		for i+run+1 < len(flags) && flags[i+run+1] == flag && run < 255 {
			run++
		}
		if run > 1 {
			out = append(out, flag|flagRepeat, byte(run))
			i += run
			continue
		}
		out = append(out, flag)
	}
	return out
}

func inferBounds(points []Point) Bounds {
	if len(points) == 0 {
		return Bounds{}
	}
	bounds := Bounds{XMin: points[0].X, YMin: points[0].Y, XMax: points[0].X, YMax: points[0].Y}
	for _, point := range points[1:] {
		if point.X < bounds.XMin {
			bounds.XMin = point.X
		}
		if point.Y < bounds.YMin {
			bounds.YMin = point.Y
		}
		if point.X > bounds.XMax {
			bounds.XMax = point.X
		}
		if point.Y > bounds.YMax {
			bounds.YMax = point.Y
		}
	}
	return bounds
}

func compositeGlyphArgumentSize(flags uint16) int {
	size := 2
	if flags&compositeArg1And2AreWords != 0 {
		size += 4
	} else {
		size += 2
	}
	if flags&compositeWeHaveAScale != 0 {
		size += 2
	} else if flags&compositeWeHaveAnXAndYScale != 0 {
		size += 4
	} else if flags&compositeWeHaveATwoByTwo != 0 {
		size += 8
	}
	return size
}

func buildWOFF2Loca(offsets []uint32, indexFormat int16) ([]byte, error) {
	var out bytes.Buffer
	switch indexFormat {
	case 0:
		for _, offset := range offsets {
			if offset%2 != 0 || offset/2 > uint32(^uint16(0)) {
				return nil, fmt.Errorf("woff2 loca short offset out of range")
			}
			writeUint16(&out, uint16(offset/2))
		}
	case 1:
		for _, offset := range offsets {
			writeUint32(&out, offset)
		}
	default:
		return nil, fmt.Errorf("unsupported loca index format %d", indexFormat)
	}
	return out.Bytes(), nil
}

func woff2GlyphBitmapSize(numGlyphs int) int {
	return 4 * ((numGlyphs + 31) / 32)
}

func woff2GlyphBit(bitmap []byte, glyphID int) bool {
	if len(bitmap) == 0 {
		return false
	}
	byteIndex := glyphID / 8
	if byteIndex >= len(bitmap) {
		return false
	}
	mask := byte(0x80 >> uint(glyphID%8))
	return bitmap[byteIndex]&mask != 0
}

func writeUint16(out *bytes.Buffer, value uint16) {
	var buf [2]byte
	binary.BigEndian.PutUint16(buf[:], value)
	out.Write(buf[:])
}

func writeInt16(out *bytes.Buffer, value int16) {
	writeUint16(out, uint16(value))
}

func writeUint32(out *bytes.Buffer, value uint32) {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], value)
	out.Write(buf[:])
}

func sfntSearchParams(numTables int) (uint16, uint16, uint16) {
	maxPower := 1
	entrySelector := 0
	for maxPower*2 <= numTables {
		maxPower *= 2
		entrySelector++
	}
	searchRange := uint16(maxPower * 16)
	rangeShift := uint16(numTables*16 - int(searchRange))
	return searchRange, uint16(entrySelector), rangeShift
}

func paddedLength(n int) int {
	return (n + 3) &^ 3
}

func checksum(data []byte) uint32 {
	var sum uint32
	for len(data) >= 4 {
		sum += binary.BigEndian.Uint32(data[:4])
		data = data[4:]
	}
	if len(data) > 0 {
		var tail [4]byte
		copy(tail[:], data)
		sum += binary.BigEndian.Uint32(tail[:])
	}
	return sum
}
