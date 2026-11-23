package editor

import (
	"bytes"
	"encoding/binary"
	"io"
	"slices"
	"testing"
)

func TestPackedWebWASM(t *testing.T) {
	section := func(id byte, payload []byte) []byte {
		return append(binary.AppendUvarint([]byte{id}, uint64(len(payload))), payload...)
	}
	wasm := []byte("\x00asm\x01\x00\x00\x00")
	memory := section(5, []byte{1, 0, 1})
	custom := section(0, []byte{1, 'x', 1, 2})
	// Out-of-order initializers, a signed LEB offset, and a 64-byte zero run.
	wide := slices.Concat([]byte{0, 0x41, 0xc9, 0, 0x0b, 66, 'E'}, make([]byte, 64), []byte{'F'})
	data := slices.Concat([]byte{3, 0, 0x41, 0xc7, 0, 0x0b, 1, 'D', 0, 0x41, 0xc0, 0, 0x0b, 5, 'A', 0, 0, 0, 'B'}, wide)
	packed := []byte{2, 0, 0x41, 0xc0, 0, 0x0b, 10, 'A', 0, 0, 0, 'B', 0, 0, 'D', 0, 'E',
		0, 0x41, 0x8a, 1, 0x0b, 1, 'F'}
	check := func(t *testing.T, module, want []byte) {
		t.Helper()
		reader, err := packedWebWASM(bytes.NewReader(module), int64(len(module)))
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(reader)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("packed module %x, want %x (err %v)", got, want, err)
		}
	}
	functionImport := section(2, []byte{1, 1, 'm', 1, 'f', 0, 0})
	prefix := slices.Concat(wasm, functionImport, memory)
	check(t, slices.Concat(prefix, section(11, data), custom), slices.Concat(prefix, section(11, packed), custom))

	// Some modules can observe a gap's old contents, initializer order or data
	// indices. They must keep their complete original encoding.
	overlap := slices.Clone(data)
	overlap[3] = 0xc1
	negative := slices.Clone(data)
	negative[3], negative[4] = 0xc7, 0x7f
	passive := slices.Concat([]byte{1, 1, 66, 'E'}, make([]byte, 64), []byte{'F'})
	for name, module := range map[string][]byte{
		"imported memory": slices.Concat(wasm, section(2, []byte{1, 1, 'm', 1, 'M', 2, 0, 1}), memory, section(11, data)),
		"data count":      slices.Concat(prefix, section(12, []byte{3}), section(11, data)),
		"passive":         slices.Concat(prefix, section(11, passive)),
		"overlap":         slices.Concat(prefix, section(11, overlap)),
		"negative offset": slices.Concat(prefix, section(11, negative)),
		"out of bounds":   slices.Concat(prefix, section(11, []byte{1, 0, 0x41, 0x80, 0x80, 4, 0x0b, 1, 42})),
		"more segments":   slices.Concat(prefix, section(11, slices.Concat([]byte{1}, wide))),
		"already packed":  slices.Concat(prefix, section(11, packed)),
	} {
		t.Run(name, func(t *testing.T) { check(t, module, module) })
	}
}
