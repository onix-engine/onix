package font

import (
	"encoding/binary"
	"fmt"
)

type cmap interface {
	GlyphIndex(r rune) uint16
}

type cmapFormat4 struct {
	EndCodes      []uint16
	StartCodes    []uint16
	IDDeltas      []int16
	IDRangeOffset []uint16
	GlyphIDArray  []uint16
	Data          []byte
}

type cmapFormat12Group struct {
	StartCharCode uint32
	EndCharCode   uint32
	StartGlyphID  uint32
}

type cmapFormat12 struct {
	Groups []cmapFormat12Group
}

func parseCmap(data []byte) (cmap, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("cmap table too short")
	}
	numTables := int(binary.BigEndian.Uint16(data[2:4]))
	if len(data) < 4+numTables*8 {
		return nil, fmt.Errorf("cmap encoding records truncated")
	}

	var format12Offset uint32
	var format4Offset uint32
	for i := 0; i < numTables; i++ {
		offset := 4 + i*8
		platformID := binary.BigEndian.Uint16(data[offset : offset+2])
		encodingID := binary.BigEndian.Uint16(data[offset+2 : offset+4])
		subtableOffset := binary.BigEndian.Uint32(data[offset+4 : offset+8])
		if int(subtableOffset)+2 > len(data) {
			continue
		}
		format := binary.BigEndian.Uint16(data[subtableOffset : subtableOffset+2])
		if platformID == 3 && encodingID == 10 && format == 12 {
			format12Offset = subtableOffset
		}
		if platformID == 3 && (encodingID == 1 || encodingID == 0) && format == 4 {
			format4Offset = subtableOffset
		}
	}

	if format12Offset != 0 {
		return parseCmapFormat12(data[format12Offset:])
	}
	if format4Offset != 0 {
		return parseCmapFormat4(data[format4Offset:])
	}

	return nil, fmt.Errorf("no supported cmap subtable found")
}

func parseCmapFormat4(data []byte) (*cmapFormat4, error) {
	if len(data) < 16 {
		return nil, fmt.Errorf("cmap format 4 too short")
	}
	length := int(binary.BigEndian.Uint16(data[2:4]))
	if length > len(data) {
		return nil, fmt.Errorf("cmap format 4 length out of bounds")
	}
	segCount := int(binary.BigEndian.Uint16(data[6:8]) / 2)
	needed := 16 + segCount*8
	if length < needed {
		return nil, fmt.Errorf("cmap format 4 truncated segments")
	}

	endCodes := make([]uint16, segCount)
	startCodes := make([]uint16, segCount)
	idDeltas := make([]int16, segCount)
	idRangeOffset := make([]uint16, segCount)

	offset := 14
	for i := 0; i < segCount; i++ {
		endCodes[i] = binary.BigEndian.Uint16(data[offset+i*2 : offset+i*2+2])
	}
	offset += segCount * 2
	offset += 2
	for i := 0; i < segCount; i++ {
		startCodes[i] = binary.BigEndian.Uint16(data[offset+i*2 : offset+i*2+2])
	}
	offset += segCount * 2
	for i := 0; i < segCount; i++ {
		idDeltas[i] = int16(binary.BigEndian.Uint16(data[offset+i*2 : offset+i*2+2]))
	}
	offset += segCount * 2
	rangeOffsetStart := offset
	for i := 0; i < segCount; i++ {
		idRangeOffset[i] = binary.BigEndian.Uint16(data[offset+i*2 : offset+i*2+2])
	}
	arrayStart := rangeOffsetStart + segCount*2
	arrayBytes := length - arrayStart
	if arrayBytes < 0 {
		return nil, fmt.Errorf("cmap format 4 invalid glyph array")
	}
	glyphIDArray := make([]uint16, arrayBytes/2)
	for i := range glyphIDArray {
		glyphIDArray[i] = binary.BigEndian.Uint16(data[arrayStart+i*2 : arrayStart+i*2+2])
	}

	return &cmapFormat4{
		EndCodes:      endCodes,
		StartCodes:    startCodes,
		IDDeltas:      idDeltas,
		IDRangeOffset: idRangeOffset,
		GlyphIDArray:  glyphIDArray,
		Data:          data[:length],
	}, nil
}

func parseCmapFormat12(data []byte) (*cmapFormat12, error) {
	if len(data) < 16 {
		return nil, fmt.Errorf("cmap format 12 too short")
	}
	length := int(binary.BigEndian.Uint32(data[4:8]))
	if length > len(data) {
		return nil, fmt.Errorf("cmap format 12 length out of bounds")
	}
	numGroups := int(binary.BigEndian.Uint32(data[12:16]))
	if length < 16+numGroups*12 {
		return nil, fmt.Errorf("cmap format 12 truncated groups")
	}
	groups := make([]cmapFormat12Group, numGroups)
	for i := 0; i < numGroups; i++ {
		offset := 16 + i*12
		groups[i] = cmapFormat12Group{
			StartCharCode: binary.BigEndian.Uint32(data[offset : offset+4]),
			EndCharCode:   binary.BigEndian.Uint32(data[offset+4 : offset+8]),
			StartGlyphID:  binary.BigEndian.Uint32(data[offset+8 : offset+12]),
		}
	}
	return &cmapFormat12{Groups: groups}, nil
}

func (c *cmapFormat4) GlyphIndex(r rune) uint16 {
	if uint32(r) > 0xffff {
		return 0
	}
	code := uint16(r)
	for i := 0; i < len(c.EndCodes); i++ {
		if code < c.StartCodes[i] || code > c.EndCodes[i] {
			continue
		}
		if c.IDRangeOffset[i] == 0 {
			return uint16(int(code) + int(c.IDDeltas[i]))
		}
		idRangeOffsetPos := 14 + len(c.EndCodes)*2 + 2 + len(c.StartCodes)*2 + len(c.IDDeltas)*2 + i*2
		glyphOffset := idRangeOffsetPos + int(c.IDRangeOffset[i]) + int(code-c.StartCodes[i])*2
		if glyphOffset+2 > len(c.Data) {
			return 0
		}
		glyph := binary.BigEndian.Uint16(c.Data[glyphOffset : glyphOffset+2])
		if glyph == 0 {
			return 0
		}
		return uint16(int(glyph) + int(c.IDDeltas[i]))
	}
	return 0
}

func (c *cmapFormat12) GlyphIndex(r rune) uint16 {
	code := uint32(r)
	for _, group := range c.Groups {
		if code < group.StartCharCode || code > group.EndCharCode {
			continue
		}
		return uint16(group.StartGlyphID + (code - group.StartCharCode))
	}
	return 0
}
