package brotli

import (
	"bytes"
	"io"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	source := bytes.Repeat([]byte("Onix Brotli codec round trip. "), 256)
	var compressed bytes.Buffer
	writer := NewWriterLevel(&compressed, 6)
	if _, err := writer.Write(source); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	decoded, err := io.ReadAll(NewReader(bytes.NewReader(compressed.Bytes())))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(decoded, source) {
		t.Fatalf("decoded %d bytes, want %d", len(decoded), len(source))
	}
}

func TestRoundTripQuality11(t *testing.T) {
	source := bytes.Repeat([]byte("Onix Brotli high quality round trip. "), 64)
	var compressed bytes.Buffer
	writer := NewWriterLevel(&compressed, 11)
	if _, err := writer.Write(source); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	decoded, err := io.ReadAll(NewReader(bytes.NewReader(compressed.Bytes())))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(decoded, source) {
		t.Fatalf("decoded %d bytes, want %d", len(decoded), len(source))
	}
}

func TestFinalUncompressedBlockDirectCopy(t *testing.T) {
	source := make([]byte, 4096)
	for index := range source {
		source[index] = byte(index*37 + 11)
	}
	reader := &Reader{
		br:                       bitReader{input: source, input_len: uint(len(source)), bit_pos_: 64},
		meta_block_remaining_len: len(source),
		is_last_metablock:        1,
		substate_uncompressed:    stateUncompressedNone,
	}
	output := make([]byte, len(source))
	availableOut := uint(len(output))
	nextOut := output
	if result := copyUncompressedBlockToOutput(&availableOut, &nextOut, nil, reader); result != decoderSuccess {
		t.Fatalf("copy result = %d, want success", result)
	}
	if !bytes.Equal(output, source) {
		t.Fatal("direct copy changed uncompressed data")
	}
	if reader.meta_block_remaining_len != 0 || reader.ringbuffer != nil {
		t.Fatalf("reader state after direct copy = remaining %d, ringbuffer %v", reader.meta_block_remaining_len, reader.ringbuffer != nil)
	}
}

func TestH10FindLongestMatchImplementsHasherInterface(t *testing.T) {
	params := encoderParams{lgwin: 10, hasher: hasherParams{num_last_distances_to_check: 4}}
	hasher := &h10{}
	hasher.Initialize(&params)
	hasher.Prepare(false, 0, nil)
	data := bytes.Repeat([]byte("abcd"), 512)
	hasher.StoreRange(data, 1023, 0, 256)
	distances := []int{4, 8, 12, 16}
	result := hasherSearchResult{score: scoreBase + 100}
	hasher.FindLongestMatch(nil, data, 1023, distances, 256, 64, 256, 0, 1023, &result)
	if result.len == 0 || result.distance == 0 {
		t.Fatalf("h10 result = %#v, want a match", result)
	}
}
