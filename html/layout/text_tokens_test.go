package layout

import (
	"slices"
	"strings"
	"testing"
)

func TestTextTokensStreamingModes(t *testing.T) {
	for _, test := range []struct {
		name, input, mode string
		want              []textToken
		breakAll          bool
	}{
		{name: "empty", input: "", mode: "normal", breakAll: false, want: nil},
		{name: "only whitespace", input: " \t\r\n", mode: "normal", breakAll: false, want: nil},
		{name: "break-all UTF-8", input: "éA界 e\u0301", mode: "normal", breakAll: true, want: []textToken{{Text: "é"}, {Text: "A"}, {Text: "界"}, {Text: " ", IsSpace: true}, {Text: "e"}, {Text: "\u0301"}}},
		{name: "invalid UTF-8", input: "a\xffb 中", mode: "normal", breakAll: false, want: []textToken{{Text: "a\xffb"}, {Text: " ", IsSpace: true}, {Text: "中"}}},
		{name: "preserved CR", input: "a\rb\r\nc\t ", mode: "pre-wrap", breakAll: true, want: []textToken{{Text: "a"}, {Text: "b"}, {IsBreak: true}, {Text: "c"}, {IsTab: true}, {Text: " ", IsSpace: true}}},
		{name: "blank pre-lines", input: "\r\n\n \t\r\nend\n", mode: "pre-line", breakAll: false, want: []textToken{{IsBreak: true}, {IsBreak: true}, {IsBreak: true}, {Text: "end"}, {IsBreak: true}}},
		{name: "nowrap", input: " A\t中文  B ", mode: "nowrap", breakAll: false, want: []textToken{{Text: "A"}, {Text: " ", IsSpace: true}, {Text: "中"}, {Text: "文"}, {Text: " ", IsSpace: true}, {Text: "B"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := slices.Collect(textTokens(test.input, test.mode, true, test.breakAll)); !slices.Equal(got, test.want) {
				t.Fatalf("tokens = %#v; want %#v", got, test.want)
			}
		})
	}
}

func TestTextTokensStopAndWarmAllocations(t *testing.T) {
	input := strings.Repeat("word 中文\tline\r\n", 4096)
	for _, mode := range []string{"normal", "pre", "pre-wrap", "pre-line"} {
		t.Run(mode, func(t *testing.T) {
			sequence := textTokens(input, mode, true, false)
			calls := 0
			sequence(func(token textToken) bool {
				calls++
				return false
			})
			if calls != 1 {
				t.Fatalf("scanner continued after cancellation: %d tokens", calls)
			}
			if allocs := testing.AllocsPerRun(20, func() {
				count := 0
				for range sequence {
					count++
				}
				if count < 4096 {
					panic("text scan stopped early")
				}
			}); allocs != 0 {
				t.Fatalf("streaming tokens allocated %g times", allocs)
			}
		})
	}
}
