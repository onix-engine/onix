package piecetree

import (
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

func checkTree(t *testing.T, tree *Tree, want string) {
	t.Helper()
	seen := make(map[int]bool)
	var visit func(int) (int, int, int, int)
	visit = func(n int) (int, int, int, int) {
		if n == 0 {
			return 0, 0, 0, 0
		}
		if seen[n] {
			t.Fatal("node reached twice")
		}
		seen[n] = true
		p := tree.nodes[n]
		lh, lb, lc, ll := visit(p.left)
		rh, rb, rc, rl := visit(p.right)
		if lh-rh > 1 || rh-lh > 1 {
			t.Fatalf("unbalanced node: %d/%d", lh, rh)
		}
		if p.size <= 0 || p.size > pieceBytes {
			t.Fatalf("piece size %d", p.size)
		}
		actual := tree.measure(p.added, p.start, p.start+p.size)
		if actual != p.piece {
			t.Fatalf("piece metadata mismatch: %+v / %+v", actual, p.piece)
		}
		h, b, c, l := max(lh, rh)+1, lb+p.size+rb, lc+p.runes+rc, ll+p.breaks+rl
		if p.height != h || p.bytes != b || p.chars != c || p.lines != l {
			t.Fatalf("bad subtree metadata: %+v", p)
		}
		return h, b, c, l
	}
	visit(tree.root)
	for n := tree.free; n != 0; n = tree.nodes[n].left {
		if seen[n] {
			t.Fatal("free list overlaps or loops")
		}
		seen[n] = true
	}
	if len(seen) != len(tree.nodes)-1 && len(tree.nodes) != 0 {
		t.Fatal("unreachable arena nodes")
	}
	if tree.Len() != len(want) || tree.Runes() != utf8.RuneCountInString(want) || tree.Lines() != strings.Count(want, "\n")+1 || tree.String() != want {
		t.Fatalf("tree mismatch: len=%d chars=%d lines=%d text=%q want=%q", tree.Len(), tree.Runes(), tree.Lines(), tree.String(), want)
	}
	index, line, column := 0, 0, 0
	for offset, r := range want {
		if tree.ByteOffset(index) != offset || tree.RuneOffset(offset) != index {
			t.Fatalf("offset mismatch at rune %d", index)
		}
		l, c := tree.Position(offset)
		if l != line || c != column {
			t.Fatalf("position at %d: %d,%d want %d,%d", offset, l, c, line, column)
		}
		if r == '\n' {
			line++
			column = 0
			if tree.LineStart(line) != offset+1 {
				t.Fatal("line start mismatch")
			}
		} else {
			column++
		}
		index++
	}
	l, c := tree.Position(len(want))
	if l != line || c != column {
		t.Fatalf("EOF position: %d,%d want %d,%d", l, c, line, column)
	}
	if tree.ByteOffset(index) != len(want) || tree.RuneOffset(len(want)) != index {
		t.Fatal("EOF offset mismatch")
	}
}

func TestTreeRandomEdits(t *testing.T) {
	random := rand.New(rand.NewPCG(42, 17))
	want := strings.Repeat("a界😀\r\nb\n", 350)
	var tree Tree
	tree.Reset(want)
	checkTree(t, &tree, want)
	for step := range 2500 {
		count := utf8.RuneCountInString(want)
		start, end := random.IntN(count+1), random.IntN(count+1)
		if start > end {
			start, end = end, start
		}
		if step%3 != 0 {
			end = min(end, start+random.IntN(5))
		}
		a, b := tree.ByteOffset(start), tree.ByteOffset(end)
		inserted := [...]string{"", "x", "界", "😀", "\r\n", "\n\n", strings.Repeat("大\nx", 400)}[random.IntN(7)]
		before := want
		snapshot := tree.String()
		tree.Replace(a, b, inserted)
		want = want[:a] + inserted + want[b:]
		checkTree(t, &tree, want)
		if snapshot != before {
			t.Fatal("snapshot unexpectedly mutated")
		}
		if step%97 == 0 {
			tree.Compact()
			checkTree(t, &tree, want)
		}
	}
}

func TestTreeBoundariesAndSnapshot(t *testing.T) {
	var tree Tree
	checkTree(t, &tree, "")
	original := strings.Repeat("界", 342) + "\r\n😀\n"
	tree.Reset(original)
	snapshot := tree.String()
	tree.Replace(3, 6, "\n😀")
	want := original[:3] + "\n😀" + original[6:]
	checkTree(t, &tree, want)
	if snapshot != original {
		t.Fatal("old snapshot changed")
	}
	if tree.Slice(3, 8) != "\n😀" {
		t.Fatal("range slice mismatch")
	}
	for i := 1; i < 3; i++ {
		if tree.RuneOffset(i) != 0 {
			t.Fatal("inside rune did not round down")
		}
	}
	tree.Replace(0, tree.Len(), "")
	checkTree(t, &tree, "")
}

func TestTreeRejectsSplitRune(t *testing.T) {
	var tree Tree
	tree.Reset("界")
	defer func() {
		if recover() == nil {
			t.Fatal("accepted a split rune")
		}
		checkTree(t, &tree, "界")
	}()
	tree.Replace(1, 2, "x")
}

func TestTreeMutationAndReadAllocations(t *testing.T) {
	var tree Tree
	tree.Reset(strings.Repeat("界\n", 10000))
	tree.Reserve(2048, 64)
	offset := tree.ByteOffset(10001)
	allocations := testing.AllocsPerRun(100, func() {
		tree.Replace(offset, offset, "x")
		tree.Replace(offset, offset+1, "")
		tree.Position(offset)
		tree.ByteOffset(9999)
		tree.LineStart(9000)
	})
	if allocations != 0 {
		t.Fatalf("reserved mutation/read allocated %g times", allocations)
	}
	tree.String()
	if allocations := testing.AllocsPerRun(100, func() { tree.String() }); allocations != 0 {
		t.Fatalf("cached snapshot allocated %g times", allocations)
	}
	if len(tree.nodes) > 100 {
		t.Fatalf("edit nodes were not reused: %d", len(tree.nodes))
	}
}

func BenchmarkTreeDistantEdit(b *testing.B) {
	for _, size := range []int{1 << 10, 1 << 20, 16 << 20} {
		b.Run(strconv.Itoa(size>>10)+"KiB", func(b *testing.B) {
			var tree Tree
			tree.Reset(strings.Repeat("x", size))
			tree.Reserve(b.N+1, 64)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				offset := 0
				if i%2 != 0 {
					offset = tree.Len()
				}
				tree.Replace(offset, offset, "a")
				tree.Replace(offset, offset+1, "")
			}
		})
	}
}
