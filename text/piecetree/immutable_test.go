package piecetree

import (
	"io"
	"math/rand/v2"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

func TestImmutableSnapshotsRandomEdits(t *testing.T) {
	random := rand.New(rand.NewPCG(11, 57))
	value := strings.Repeat("中文🙂\r\nabc\n", 100)
	s := FromString(value)
	versions := []Snapshot{s}
	values := []string{value}
	for i := 0; i < 200; i++ {
		count := utf8.RuneCountInString(value)
		a, b := random.IntN(count+1), random.IntN(count+1)
		if a > b {
			a, b = b, a
		}
		a, b = s.ByteOffset(a), s.ByteOffset(b)
		inserted := []string{"x", "界🙂", "\r\n", "", "abc\n"}[random.IntN(5)]
		next := s.Replace(a, b, inserted)
		want := value[:a] + inserted + value[b:]
		if next.String() != want {
			t.Fatalf("edit %d mismatch", i)
		}
		start, end, newEnd := s.ChangedRange(next)
		if value[:start]+want[start:newEnd]+value[end:] != want {
			t.Fatal("invalid changed range")
		}
		if next.Runes() != utf8.RuneCountInString(want) || next.Lines() != strings.Count(want, "\n")+1 {
			t.Fatal("invalid index")
		}
		offset := 0
		for index := 0; index <= next.Runes(); index++ {
			if got := next.ByteOffset(index); got != offset || next.RuneOffset(offset) != index {
				t.Fatal("invalid rune lookup")
			}
			if offset < len(want) {
				_, size := utf8.DecodeRuneInString(want[offset:])
				offset += size
			}
		}
		for line := 0; line < next.Lines(); line++ {
			start := next.LineStart(line)
			if strings.Count(want[:start], "\n") != line || line > 0 && want[start-1] != '\n' {
				t.Fatal("invalid line lookup")
			}
		}
		for range 4 {
			start := random.IntN(len(want) + 1)
			end := start + random.IntN(len(want)-start+1)
			if next.Slice(start, end) != want[start:end] {
				t.Fatal("invalid slice")
			}
		}
		if i%7 == 0 {
			versions = append(versions, next)
			values = append(values, want)
		}
		s, value = next, want
	}
	for i, s := range versions {
		if s.String() != values[i] {
			t.Fatal("old version mutated")
		}
	}
}
func TestImmutableSnapshotConcurrentReaders(t *testing.T) {
	base := FromString(strings.Repeat("界🙂\r\nline\n", 10000))
	start, end := base.ByteOffset(20000), base.ByteOffset(20001)
	snapshot := base.Replace(start, end, "new")
	expected := base.flat[:start] + "new" + base.flat[end:]
	ready := make(chan struct{})
	var readers sync.WaitGroup
	for range 8 {
		readers.Go(func() {
			<-ready
			for range 50 {
				if snapshot.String() != expected {
					t.Error("mutated snapshot")
				}
				var data [113]byte
				n, err := snapshot.ReadAt(data[:], 51)
				if err != nil && err != io.EOF || string(data[:n]) != expected[51:51+n] {
					t.Error("invalid read")
				}
			}
		})
	}
	close(ready)
	for i := 0; i < 100; i++ {
		base = base.Replace(base.ByteOffset(80), base.ByteOffset(81), "x")
	}
	readers.Wait()
}

func TestImmutableSnapshotLocalEditsAndBalance(t *testing.T) {
	random := rand.New(rand.NewPCG(71, 93))
	value := strings.Repeat("abc界🙂\r\n", 4000)
	s := FromString(value)
	versions := []Snapshot{s}
	values := []string{value}
	var check func(*snapshotNode) (int, int, int, int)
	check = func(n *snapshotNode) (int, int, int, int) {
		if n == nil {
			return 0, 0, 0, 0
		}
		if n.left == nil {
			if n.right != nil || n.bytes != len(n.text) || n.runes != utf8.RuneCountInString(n.text) || n.breaks != strings.Count(n.text, "\n") || n.height != 1 {
				t.Fatal("invalid leaf")
			}
			return n.bytes, n.runes, n.breaks, 1
		}
		lb, lr, ln, lh := check(n.left)
		rb, rr, rn, rh := check(n.right)
		if n.right == nil || lh-rh > 1 || rh-lh > 1 || n.bytes != lb+rb || n.runes != lr+rr || n.breaks != ln+rn || n.height != max(lh, rh)+1 {
			t.Fatal("unbalanced or invalid branch")
		}
		return n.bytes, n.runes, n.breaks, n.height
	}
	for i := range 3000 {
		index := random.IntN(s.Runes() + 1)
		start, end := s.ByteOffset(index), s.ByteOffset(min(index+random.IntN(4), s.Runes()))
		inserted := []string{"x", "界🙂", "\r\n", "", "abc\n"}[random.IntN(5)]
		s = s.Replace(start, end, inserted)
		value = value[:start] + inserted + value[end:]
		if i%100 == 0 {
			if !s.Equal(FromString(value)) || s.Materialized() {
				t.Fatal("local edits lost text or materialized a version")
			}
			check(s.state.root)
			versions, values = append(versions, s), append(values, value)
		}
	}
	for i, version := range versions {
		if !version.Equal(FromString(values[i])) {
			t.Fatal("retained version mutated")
		}
	}
}

func TestImmutableSnapshotUTF8BoundariesAndReads(t *testing.T) {
	for _, value := range []string{"a界🙂z", strings.Repeat("界🙂", 600), strings.Repeat("\x80", 4096) + "a界"} {
		values := []string{value, "x" + value}
		for version, source := range []Snapshot{FromString(value), FromString(value).Replace(0, 0, "x")} {
			boundaries := make([]bool, source.Len()+1)
			for offset := range values[version] {
				boundaries[offset] = true
			}
			boundaries[source.Len()] = true
			for i := 0; i <= source.Len(); i++ {
				if source.boundary(i) != boundaries[i] {
					t.Fatalf("invalid rune boundary at %d", i)
				}
			}
			if n, err := source.ReadAt(nil, int64(source.Len()+1)); n != 0 || err != nil {
				t.Fatal("zero length read must succeed", n, err)
			}
			if _, err := source.ReadAt(nil, -1); err == nil {
				t.Fatal("negative read must fail")
			}
		}
	}
	defer func() {
		if recover() == nil {
			t.Fatal("replacement splitting a rune must panic")
		}
	}()
	FromString("界").Replace(1, 2, "x")
}
func BenchmarkImmutableMiddleEdit(b *testing.B) {
	s := FromString(strings.Repeat("let value = 42; // text line\n", (2<<20)/27))
	mid := s.ByteOffset(s.Runes() / 2)
	s = s.Replace(mid, mid+1, "x")
	b.ReportAllocs()
	for b.Loop() {
		s = s.Replace(mid, mid+1, "y")
		s = s.Replace(mid, mid+1, "x")
	}
}
