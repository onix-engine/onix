package style

import (
	"reflect"
	"slices"
	"testing"
)

func TestCSSValueIteratorBoundaries(t *testing.T) {
	for _, test := range []struct {
		input  string
		fields bool
		want   []string
	}{
		{``, true, nil},
		{" \t\n", false, nil},
		{`calc(1px + (2px))  "two words" a\ b`, true, []string{`calc(1px + (2px))`, `"two words"`, `a\ b`}},
		{"1px\t2px\r\n3px\f4px", true, []string{"1px", "2px", "3px", "4px"}},
		{`rgb(1, 2, 3), "a,b", a\,b,,`, false, []string{`rgb(1, 2, 3)`, `"a,b"`, `a\,b`, "", ""}},
		{`"a\" b" c`, true, []string{`"a\" b"`, "c"}},
	} {
		seq := cssValues(test.input, test.fields)
		for range 2 {
			if got := slices.Collect(seq); !reflect.DeepEqual(got, test.want) {
				t.Errorf("cssValues(%q, %v) = %q, want %q", test.input, test.fields, got, test.want)
			}
		}
		// A caller can stop without scanning the remaining tokens.
		count := 0
		for range seq {
			count++
			break
		}
		if count > 1 {
			t.Fatal("iterator ignored early stop")
		}
	}
}

func TestCompositeEdgeValues(t *testing.T) {
	ctx := parseContext{FontSize: 20, RootFontSize: 10, Viewport: Viewport{Width: 800, Height: 600}}
	for _, test := range []struct {
		input string
		want  Edges
	}{
		{"1px", uniformEdges(LengthPx(1))},
		{"1px 2px", Edges{LengthPx(1), LengthPx(2), LengthPx(1), LengthPx(2)}},
		{"1px 2px 3px", Edges{LengthPx(1), LengthPx(2), LengthPx(3), LengthPx(2)}},
		{"1em 2rem 3vw 4vh", Edges{LengthPx(20), LengthPx(20), LengthPx(24), LengthPx(24)}},
		{"1EM 2REM 3VW 4VH", Edges{LengthPx(20), LengthPx(20), LengthPx(24), LengthPx(24)}},
	} {
		for _, parse := range []func(string) (Edges, error){
			func(input string) (Edges, error) { return parseBoxValues(input, false, ctx) },
			func(input string) (Edges, error) { return parseCornerAxis(input, ctx) },
		} {
			got, err := parse(test.input)
			if err != nil || got != test.want {
				t.Errorf("parse(%q) = %+v, %v; want %+v", test.input, got, err, test.want)
			}
		}
	}
	edges, err := parseBoxValues("auto 25%", true, ctx)
	if err != nil || edges.Top != Auto() || edges.Right != LengthPercent(.25) || edges.Bottom != Auto() || edges.Left != LengthPercent(.25) {
		t.Fatalf("auto/percent edges = %+v, %v", edges, err)
	}
	for _, input := range []string{"", "1px 2px 3px 4px 5px", "1px bad", "auto"} {
		if _, err := parseBoxValues(input, false, ctx); err == nil {
			t.Errorf("accepted invalid box %q", input)
		}
		if _, err := parseCornerAxis(input, ctx); err == nil {
			t.Errorf("accepted invalid corner %q", input)
		}
	}
	for _, input := range []string{"", "red blue green black white", "red bad"} {
		if _, err := parseColorEdges(input); err == nil {
			t.Errorf("accepted invalid colors %q", input)
		}
	}
	for _, input := range []string{"", "solid none solid none solid", "solid bad"} {
		if _, err := parseBorderStyle(input); err == nil {
			t.Errorf("accepted invalid styles %q", input)
		}
	}
}

func TestCompositeBorderTokenOrder(t *testing.T) {
	ctx := parseContext{FontSize: 20}
	color, err := parseColorValue("#123456")
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{"2px solid #123456", "#123456 2px solid", "solid #123456 2px", "SOLID #123456 2PX", ".1em solid #123456"} {
		width, style, colors, err := parseBorder(input, ctx)
		if err != nil || width != uniformEdges(LengthPx(2)) || style != uniformEdges(Keyword("solid")) || colors != uniformEdges(color) {
			t.Errorf("border %q = %+v, %+v, %+v, %v", input, width, style, colors, err)
		}
	}
	for _, input := range []string{"1px bogus", "solid 2bad", "rgb(no)"} {
		if _, _, _, err := parseBorder(input, ctx); err == nil {
			t.Errorf("accepted invalid border %q", input)
		}
	}
	if _, _, _, err := parseFlex("1 2 3", ctx); err == nil {
		t.Fatal("accepted a third flex number as a basis")
	}
}

func TestCompositeFlexBasis(t *testing.T) {
	ctx := parseContext{FontSize: 20, RootFontSize: 10}
	for _, test := range []struct {
		input string
		basis Value
	}{
		{"2 3 0", LengthPx(0)},
		{"2 3 auto", Auto()},
		{"2 3 1em", LengthPx(20)},
		{"2 3 1REM", LengthPx(10)},
		{"2 3 25%", LengthPercent(.25)},
		{"1px 2 3", LengthPx(1)},
	} {
		grow, shrink, basis, err := parseFlex(test.input, ctx)
		if err != nil || grow != Number(2) || shrink != Number(3) || basis != test.basis {
			t.Errorf("flex %q = %+v, %+v, %+v, %v", test.input, grow, shrink, basis, err)
		}
	}
	for _, input := range []string{"2 3 1px", "2 3 auto", "2 3 25%"} {
		if allocs := testing.AllocsPerRun(100, func() {
			if _, _, _, err := parseFlex(input, ctx); err != nil {
				panic(err)
			}
		}); allocs != 0 {
			t.Errorf("valid flex basis %q allocated %g times", input, allocs)
		}
	}
}

var compositeCSSCases = []struct{ property, input string }{
	{"padding", "4px 8px"}, {"margin", "0 auto"}, {"border-radius", "4px 8px 12px 16px"},
	{"border-top-left-radius", "4px 8px"}, {"border", "1px solid #123456"},
	{"border-color", "red blue green black"}, {"border-style", "solid none"},
	{"flex", "1 1 0"}, {"gap", "4px 8px"}, {"overflow", "hidden auto"},
}

func TestCompositeCSSAllocations(t *testing.T) {
	for _, test := range compositeCSSCases {
		t.Run(test.property, func(t *testing.T) {
			computed := defaultStyle(nil)
			if allocs := testing.AllocsPerRun(100, func() {
				if err := applyProperty(&computed, test.property, test.input, nil, parseContext{}); err != nil {
					panic(err)
				}
			}); allocs != 0 {
				t.Fatalf("valid composite allocated %g times", allocs)
			}
		})
	}
}

func BenchmarkCompositeCSSProperties(b *testing.B) {
	for _, test := range compositeCSSCases {
		b.Run(test.property, func(b *testing.B) {
			computed := defaultStyle(nil)
			b.ReportAllocs()
			for b.Loop() {
				if err := applyProperty(&computed, test.property, test.input, nil, parseContext{}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
