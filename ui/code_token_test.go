package ui

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/diiyw/onix/ecs"
)

func TestCodeTokenRange(t *testing.T) {
	for _, test := range []struct {
		name, value string
		index       int
		start, end  int
	}{
		{"identifier", "call(foo_bar42)", 8, 5, 14},
		{"Unicode", "x := 中文变量_2", 8, 5, 11},
		{"combining mark", "e\u0301cole()", 1, 0, 6},
		{"dollar", "$value + 1", 3, 0, 6},
		{"operator", "a >>= 2", 3, 2, 5},
		{"delimiter", "call(foo)", 4, 4, 5},
		{"member", "foo.bar", 4, 4, 7},
		{"dot", "foo.bar", 3, 3, 4},
		{"whitespace", "a \t b\nc", 2, 1, 4},
		{"line", "one\ntwo", 5, 4, 7},
		{"newline", "one\ntwo", 3, 3, 4},
		{"emoji", "x🙂y", 1, 1, 2},
		{"empty", "", 0, 0, 0},
		{"end", "word", 4, 4, 4},
		{"past end", "word", 20, 4, 4},
		{"negative", "word", -1, 0, 4},
		{"opening quote", `"hello world"`, 0, 0, 1},
		{"closing quote", `"hello world"`, 12, 12, 13},
		{"empty string", `""`, 1, 1, 2},
		{"after string", `"hello world" + next`, 16, 16, 20},
		{"after unterminated line", "\"hello\nnext", 8, 7, 11},
		{"quote in line comment", "// don't select all", 10, 9, 15},
		{"quote in block comment", "/* don't select all */", 10, 9, 15},
	} {
		t.Run(test.name, func(t *testing.T) {
			token := CodeTokenAt(test.value, test.index)
			if token.RuneStart != test.start || token.RuneEnd != test.end {
				t.Fatalf("range = %d:%d, want %d:%d", token.RuneStart, token.RuneEnd, test.start, test.end)
			}
			if utf8.RuneCountInString(test.value[:token.Start]) != test.start || utf8.RuneCountInString(test.value[:token.End]) != test.end || utf8.RuneCountInString(test.value[:token.Offset]) != min(max(0, test.index), utf8.RuneCountInString(test.value)) {
				t.Fatalf("byte offsets disagree with rune offsets: %+v", token)
			}
		})
	}
	if allocations := testing.AllocsPerRun(100, func() {
		CodeTokenAt("prefix 中文变量_2 += value", 10)
	}); allocations != 0 {
		t.Fatalf("token selection allocated %g times", allocations)
	}
}

func TestCodeTokenRangeSelectsQuotedContents(t *testing.T) {
	for _, test := range []struct {
		name, value, selected string
	}{
		{"import path", `import "github.com/diiyw/onix/scene"`, "github.com/diiyw/onix/scene"},
		{"spaces and Unicode", `值 := "你好 世界🙂/scene"`, "你好 世界🙂/scene"},
		{"single quotes", `value = 'hello world/path'`, "hello world/path"},
		{"escaped quotes", `value = "hello \"world\"/path"`, `hello \"world\"/path`},
		{"escaped backslash", `value = "path\\" + "next value"`, `path\\`},
		{"following string", `value = "path\\" + "next value"`, "next value"},
		{"raw multiline", "value := `first\n你好 \\\"/scene`", "first\n你好 \\\"/scene"},
		{"line comment quotes", "// an unmatched \" quote\nvar s = \"whole string\"", "whole string"},
		{"block comment quotes", "/* an unmatched \" quote */ var s = \"whole string\"", "whole string"},
		{"adjacent strings", `"first""second value"`, "second value"},
		{"unterminated", `value = "whole string`, "whole string"},
		{"unterminated line", "value = \"whole string\nnext", "whole string"},
	} {
		t.Run(test.name, func(t *testing.T) {
			startByte := strings.Index(test.value, test.selected)
			if startByte < 0 {
				t.Fatal("missing selected text in fixture")
			}
			wantStart := utf8.RuneCountInString(test.value[:startByte])
			wantEnd := wantStart + utf8.RuneCountInString(test.selected)
			for index := wantStart; index < wantEnd; index++ {
				token := CodeTokenAt(test.value, index)
				if token.RuneStart != wantStart || token.RuneEnd != wantEnd || !token.Quoted || token.Start != startByte || token.End != startByte+len(test.selected) || utf8.RuneCountInString(test.value[:token.Offset]) != index {
					t.Fatalf("index %d: token = %+v, want quoted range %d:%d", index, token, wantStart, wantEnd)
				}
			}
			if allocations := testing.AllocsPerRun(100, func() {
				CodeTokenAt(test.value, wantStart)
			}); allocations != 0 {
				t.Fatalf("string selection allocated %g times", allocations)
			}
		})
	}
}

func TestTokenDoubleClickRequiresNearbyConsecutiveClicks(t *testing.T) {
	world := newUIWorld(t)
	editor := SpawnTextarea(world, TextOptions{Width: 200, Height: 80, Value: "first second"})
	ecs.GetMut[Textarea](world, editor).SelectTokens = true
	ordinary := SpawnTextarea(world, TextOptions{Width: 200, Height: 80, Value: "ordinary"})
	now := time.Now()
	for _, test := range []struct {
		name  string
		x, y  float32
		delay time.Duration
	}{
		{"distant horizontally", 30, 10, time.Millisecond},
		{"distant vertically", 10, 30, time.Millisecond},
		{"expired", 10, 10, inputDoubleClickInterval + time.Millisecond},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := &uiEventState{}
			if selectOnInputDoubleClick(world, state, editor, 10, 10, now) || selectOnInputDoubleClick(world, state, editor, test.x, test.y, now.Add(test.delay)) {
				t.Fatal("separate clicks selected a token")
			}
		})
	}
	state := &uiEventState{}
	selectOnInputDoubleClick(world, state, editor, 10, 10, now)
	if selectOnInputDoubleClick(world, state, ordinary, 10, 10, now.Add(time.Millisecond)) {
		t.Fatal("ordinary textarea selected a token")
	}
	if selectOnInputDoubleClick(world, state, editor, 10, 10, now.Add(2*time.Millisecond)) {
		t.Fatal("clicking another control did not break the double-click")
	}
}
