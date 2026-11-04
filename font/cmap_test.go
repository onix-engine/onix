package font

import (
	"encoding/binary"
	"testing"
)

func testUnicodeEmojiCmap() []byte {
	data := make([]byte, 12+16+3*12)
	put16, put32 := binary.BigEndian.PutUint16, binary.BigEndian.PutUint32
	put16(data[2:], 1)
	put16(data[6:], 4) // Unicode platform, full repertoire
	put32(data[8:], 12)
	put16(data[12:], 12)
	put32(data[16:], uint32(len(data)-12))
	put32(data[24:], 3)
	for i, r := range []rune{'✅', '❌', '🙂'} {
		group := data[28+i*12:]
		put32(group, uint32(r))
		put32(group[4:], uint32(r))
		put32(group[8:], 1)
	}
	return data
}

func TestUnicodePlatformCharacterMaps(t *testing.T) {
	for _, format := range []int{4, 12} {
		var data []byte
		if format == 4 {
			data = buildTestCmapTable()
			binary.BigEndian.PutUint16(data[4:], 0)
			binary.BigEndian.PutUint16(data[6:], 3)
		} else {
			data = testUnicodeEmojiCmap()
		}
		mapping, err := parseCmap(data)
		if err != nil {
			t.Fatalf("Unicode format %d: %v", format, err)
		}
		chars := "A"
		if format == 12 {
			chars = "✅❌🙂"
		}
		for _, r := range chars {
			if mapping.GlyphIndex(r) != 1 {
				t.Errorf("Unicode format %d: lost %U", format, r)
			}
		}
		if mapping.GlyphIndex('\U0010ffff') != 0 {
			t.Fatal("unsupported character acquired a glyph")
		}
	}
}

func TestCharacterMapPrefersFullUnicodeCoverage(t *testing.T) {
	bmp, full := buildTestCmapTable()[12:], testUnicodeEmojiCmap()[12:]
	data := make([]byte, 20+len(bmp)+len(full))
	put16, put32 := binary.BigEndian.PutUint16, binary.BigEndian.PutUint32
	put16(data[2:], 2)
	put16(data[6:], 4)
	put32(data[8:], uint32(20+len(bmp)))
	put16(data[12:], 3)
	put16(data[14:], 1)
	put32(data[16:], 20)
	copy(data[20:], bmp)
	copy(data[20+len(bmp):], full)
	mapping, err := parseCmap(data)
	if err != nil || mapping.GlyphIndex('🙂') != 1 {
		t.Fatalf("full Unicode map lost to Windows BMP map: %v", err)
	}
}
