package piecetree

import (
	"errors"
	"io"
	"math/rand/v2"
	"strings"
	"testing"
)

func TestReadAtAcrossEditedPieces(t *testing.T) {
	var tree Tree
	text := strings.Repeat("中🙂\r\n", 1000)
	tree.Reset(text)
	random := rand.New(rand.NewPCG(1, 2))
	for range 80 {
		start := tree.ByteOffset(random.IntN(tree.Runes() + 1))
		end := tree.ByteOffset(random.IntN(tree.Runes() + 1))
		if start > end {
			start, end = end, start
		}
		tree.Replace(start, end, "inserted界\n")
		text = text[:start] + "inserted界\n" + text[end:]
		for range 20 {
			offset := random.IntN(len(text) + 10)
			dst := make([]byte, random.IntN(256))
			n, err := tree.ReadAt(dst, int64(offset))
			want := min(len(dst), max(0, len(text)-offset))
			wantErr := error(nil)
			if len(dst) > 0 && (offset >= len(text) || want < len(dst)) {
				wantErr = io.EOF
			}
			if n != want || !errors.Is(err, wantErr) || string(dst[:n]) != text[min(offset, len(text)):min(offset+n, len(text))] {
				t.Fatalf("offset=%d length=%d: read %d/%d bytes, error=%v", offset, len(dst), n, want, err)
			}
		}
	}
	if _, err := tree.ReadAt(make([]byte, 1), -1); err == nil {
		t.Fatal("accepted negative offset")
	}
	var dst [32]byte
	if n := testing.AllocsPerRun(100, func() { _, _ = tree.ReadAt(dst[:], 0) }); n != 0 {
		t.Fatalf("tree read allocated %g times", n)
	}
}
