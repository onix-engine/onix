package editor

import (
	"bytes"
	"cmp"
	"encoding/binary"
	"io"
	"math"
	"slices"
)

// packedWebWASM streams the Go executable with a smaller data section. The
// linker can exhaust its segment budget early and emit large zero-filled runs
// at the end. Only the data section needs to be resident during compression.
func packedWebWASM(source io.ReaderAt, size int64) (io.Reader, error) {
	original := io.NewSectionReader(source, 0, size)
	var header [8]byte
	if _, err := source.ReadAt(header[:], 0); err != nil || string(header[:]) != "\x00asm\x01\x00\x00\x00" {
		return original, nil
	}
	var memoryBytes uint64
	for pos := int64(len(header)); pos < size; {
		n, err := source.ReadAt(header[:6], pos)
		if err != nil && err != io.EOF {
			return nil, err
		}
		if n < 2 {
			return original, nil
		}
		length, used := binary.Uvarint(header[1:n])
		body := pos + 1 + int64(used)
		if used <= 0 || length > min(uint64(math.MaxInt), uint64(math.MaxUint32)) || length > uint64(size-body) {
			return original, nil
		}
		end := body + int64(length)
		switch header[0] {
		case 12: // Data indices are observable through memory.init or data.drop.
			return original, nil
		case 2, 5, 11: // Imports, memory, data.
			payload := make([]byte, length)
			if _, err := source.ReadAt(payload, body); err != nil {
				return nil, err
			}
			switch header[0] {
			case 2:
				if !wasmFunctionImports(payload) {
					return original, nil
				}
			case 5:
				// Go defines one unshared wasm32 memory with a minimum page count.
				if len(payload) < 3 || payload[0] != 1 || payload[1] != 0 {
					return original, nil
				}
				pages, n := binary.Uvarint(payload[2:])
				if n != len(payload)-2 || pages > 65536 {
					return original, nil
				}
				memoryBytes = pages << 16
			case 11:
				packed := packWASMData(payload, memoryBytes)
				if packed == nil {
					return original, nil
				}
				prefix := binary.AppendUvarint([]byte{11}, uint64(len(packed)))
				return io.MultiReader(io.NewSectionReader(source, 0, pos), bytes.NewReader(prefix), bytes.NewReader(packed), io.NewSectionReader(source, end, size-end)), nil
			}
		}
		pos = end
	}
	return original, nil
}

type wasmBytes []byte

func (p *wasmBytes) u32() (uint32, bool) {
	value, n := binary.Uvarint(*p)
	if n <= 0 || n > 5 || value > math.MaxUint32 {
		return 0, false
	}
	*p = (*p)[n:]
	return uint32(value), true
}

func (p *wasmBytes) blob() ([]byte, bool) {
	n, ok := p.u32()
	if !ok || uint64(n) > uint64(len(*p)) {
		return nil, false
	}
	value := (*p)[:n]
	*p = (*p)[n:]
	return value, true
}

// Go uses active initializers for memory zero, with a nonnegative signed
// LEB128 i32.const followed by end. Other offset expressions are left alone.
func (p *wasmBytes) dataOffset() (uint32, bool) {
	code := *p
	if len(code) < 4 || code[0] != 0 || code[1] != 0x41 {
		return 0, false
	}
	value, n := binary.Uvarint(code[2:])
	end := 2 + n
	if n <= 0 || n > 5 || end >= len(code) || code[end] != 0x0b {
		return 0, false
	}
	if value > math.MaxInt32 || code[end-1]&0x40 != 0 {
		return 0, false
	}
	*p = code[end+1:]
	return uint32(value), true
}

func wasmFunctionImports(p wasmBytes) bool {
	count, ok := p.u32()
	if !ok {
		return false
	}
	for range count {
		_, moduleOK := p.blob()
		_, nameOK := p.blob()
		if !moduleOK || !nameOK || len(p) == 0 || p[0] != 0 {
			return false
		}
		p = p[1:]
		if _, ok := p.u32(); !ok {
			return false
		}
	}
	return len(p) == 0
}

type wasmDataRun struct {
	data   []byte
	offset uint32
}

func packWASMData(payload []byte, memoryBytes uint64) []byte {
	p := wasmBytes(payload)
	count, ok := p.u32()
	// Each supported segment requires at least five bytes.
	if !ok || uint64(count) > uint64(len(p)/5) || memoryBytes == 0 {
		return nil
	}
	runs := make([]wasmDataRun, 0, count)
	for range count {
		offset, ok := p.dataOffset()
		if !ok {
			return nil
		}
		data, ok := p.blob()
		if !ok || uint64(offset)+uint64(len(data)) > memoryBytes {
			return nil
		}
		if len(data) != 0 {
			runs = append(runs, wasmDataRun{data, offset})
		}
	}
	if len(p) != 0 {
		return nil
	}
	slices.SortFunc(runs, func(a, b wasmDataRun) int { return cmp.Compare(a.offset, b.offset) })

	// Short gaps compress well with Brotli. Omit longer zero runs, which a
	// freshly allocated memory already contains. Keep the original segment
	// count and encoded length as upper bounds.
	const joinGap = 16
	var zeros [32]byte
	var pending wasmDataRun
	var segments uint64
	encoded := make([]byte, binary.MaxVarintLen32, len(payload)+binary.MaxVarintLen32)
	flush := func() {
		if len(pending.data) == 0 {
			return
		}
		segments++
		encoded = append(encoded, 0, 0x41)
		encoded = binary.AppendUvarint(encoded, uint64(pending.offset))
		if last := len(encoded) - 1; encoded[last]&0x40 != 0 {
			encoded[last] |= 0x80
			encoded = append(encoded, 0)
		}
		encoded = append(encoded, 0x0b)
		encoded = binary.AppendUvarint(encoded, uint64(len(pending.data)))
		encoded = append(encoded, pending.data...)
		pending.data = pending.data[:0]
	}

	var previousEnd uint64
	for _, run := range runs {
		address := uint64(run.offset)
		if address < previousEnd {
			return nil // Overlapping initializers depend on their original order.
		}
		previousEnd = address + uint64(len(run.data))
		for data := run.data; len(data) != 0; {
			n := bytes.Index(data, zeros[:])
			if n < 0 {
				n = len(data)
			}
			if n != 0 {
				gap := address - (uint64(pending.offset) + uint64(len(pending.data)))
				if len(pending.data) == 0 || gap > joinGap {
					flush()
					if address > math.MaxInt32 {
						return nil
					}
					pending.offset = uint32(address)
				} else {
					pending.data = append(pending.data, zeros[:gap]...)
				}
				pending.data = append(pending.data, data[:n]...)
			}
			end := n
			for end < len(data) && data[end] == 0 {
				end++
			}
			data = data[end:]
			address += uint64(end)
		}
	}
	flush()
	var prefix [binary.MaxVarintLen32]byte
	if segments > uint64(count) {
		return nil
	}
	n := binary.PutUvarint(prefix[:], segments)
	result := encoded[len(prefix)-n:]
	if len(result) >= len(payload) {
		return nil
	}
	copy(result, prefix[:n])
	return result
}
