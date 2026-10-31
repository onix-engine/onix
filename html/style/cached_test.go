package style

import (
	"reflect"
	"strings"
	"testing"

	"github.com/diiyw/onix/html/css"
	"github.com/diiyw/onix/html/dom"
)

func TestComputeWithCacheMatchesFreshCascade(t *testing.T) {
	root, err := dom.Parse(`<html><body><section id="panel"><span id="target">Text</span></section><div id="removed">Old</div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := css.Parse(`html{font-size:20px} section{color:blue;container-type:inline-size} span{font-size:1.5rem} span:focus{color:red} @media(min-width:400px){section{color:green}} @container(min-width:200px){span{height:42px}}`)
	if err != nil {
		t.Fatal(err)
	}
	panel := cachedStyleNode(root, "panel")
	target := cachedStyleNode(root, "target")
	removed := cachedStyleNode(root, "removed")
	var cache MutationCache
	var styles Map
	var input css.State
	viewport := Viewport{Width: 320, Height: 200}
	check := func() {
		t.Helper()
		styles, err = ComputeWithCache(root, sheet, viewport, &input, styles, &cache)
		if err != nil {
			t.Fatal(err)
		}
		fresh, err := ComputeWithViewportAndStateAndQueries(root, sheet, viewport, &input, &QueryEnvironment{ContainerMetrics: cache.ContainerMetrics})
		if err != nil || !reflect.DeepEqual(styles, fresh) {
			t.Fatalf("cached cascade differs from fresh computation: %v", err)
		}
	}
	check()
	before := styles[target]
	check()
	if styles[target] != before {
		t.Fatal("unchanged full cascade replaced an immutable style")
	}
	input.Focused = target
	check()
	input.Focused = nil
	viewport.Width = 640
	cache.ContainerMetrics = map[*dom.Node]ContainerMetrics{panel: {Width: 240, Height: 80}}
	check()
	root.Children[0].Attributes = map[string]string{"style": "font-size:24px"}
	check()
	removed.Parent.Children = removed.Parent.Children[:1]
	removed.Parent = nil
	check()
	if _, ok := styles[removed]; ok {
		t.Fatal("full cascade retained a removed node")
	}
	oldRoot := root
	root, err = dom.Parse(`<html><body><span id="target">Replacement</span></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	check()
	if _, ok := styles[oldRoot]; ok {
		t.Fatal("full cascade retained a replaced document")
	}
	sheet, err = css.Parse(`span{color:purple;font-size:30px}`)
	if err != nil {
		t.Fatal(err)
	}
	check()
}

func TestComputeWithCacheWarmAllocations(t *testing.T) {
	root, err := dom.Parse(`<section><span>Text</span></section>`)
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := css.Parse(`section{color:red}span{font-size:20px}`)
	if err != nil {
		t.Fatal(err)
	}
	var cache MutationCache
	var styles Map
	if allocs := testing.AllocsPerRun(50, func() {
		styles, err = ComputeWithCache(root, sheet, Viewport{}, nil, styles, &cache)
		if err != nil {
			panic(err)
		}
	}); allocs != 0 {
		t.Fatalf("warm full cascade allocated %g times", allocs)
	}
}

func cachedStyleNode(node *dom.Node, id string) *dom.Node {
	if value, _ := node.Attr("id"); value == id {
		return node
	}
	for _, child := range node.Children {
		if found := cachedStyleNode(child, id); found != nil {
			return found
		}
	}
	return nil
}

func TestPersistentStyleIndexBound(t *testing.T) {
	var cache MutationCache
	cache.prepareStyleIndex()
	computed := defaultStyle(dom.NewElement("div", nil))
	for i := range maxDocumentStyleInterned + 20 {
		computed.Width = LengthPx(float64(i))
		cache.styles.commit(&computed, nil)
	}
	if cache.styles.used != maxDocumentStyleInterned {
		t.Fatalf("persistent index retained %d styles", cache.styles.used)
	}
	cache.prepareStyleIndex()
	if cache.styles.used != 0 {
		t.Fatal("full index was not retired at the next pass")
	}
	for _, slot := range cache.styles.slots {
		if slot.value != nil {
			t.Fatal("retired index retained a style")
		}
	}
}

func BenchmarkComputeFullStyles(b *testing.B) {
	root, err := dom.Parse(`<section>` + strings.Repeat(`<div class="item"><span>Text</span></div>`, 100) + `</section>`)
	if err != nil {
		b.Fatal(err)
	}
	sheet, err := css.Parse(`section{font-size:16px;color:#123456}.item{display:flex;width:100px;height:20px}.item:nth-child(even){color:#654321}span{display:block}`)
	if err != nil {
		b.Fatal(err)
	}
	for _, cached := range []bool{false, true} {
		name := "fresh"
		if cached {
			name = "cached"
		}
		b.Run(name, func(b *testing.B) {
			var cache MutationCache
			var styles Map
			compute := func() {
				if cached {
					styles, err = ComputeWithCache(root, sheet, Viewport{}, nil, styles, &cache)
				} else {
					styles, err = Compute(root, sheet)
				}
				if err != nil {
					b.Fatal(err)
				}
			}
			compute()
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				compute()
			}
		})
	}
}

func TestInputStateWithCacheMatchesFreshCascade(t *testing.T) {
	root, err := dom.Parse(`<section id="panel"><button id="first"><span>First</span></button><button id="second">Second</button></section>`)
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := css.Parse(`section{container-type:inline-size}button{color:black}button:hover{color:red}button:active{width:2em}button:focus-visible{background:blue}section:focus-within{font-size:20px}section:hover #second{color:green}@container(min-width:200px){button:hover{height:30px}}`)
	if err != nil {
		t.Fatal(err)
	}
	first, second := cachedStyleNode(root, "first"), cachedStyleNode(root, "second")
	var cache MutationCache
	cache.ContainerMetrics = map[*dom.Node]ContainerMetrics{cachedStyleNode(root, "panel"): {Width: 250, Height: 100}}
	states := []css.State{{}, {Hovered: first}, {Hovered: second}, {Focused: first, FocusVisible: true}, {Focused: second}, {Active: first}, {}}
	var styles Map
	viewport := Viewport{Width: 800, Height: 600}
	styles, _, err = RecomputeInputStateWithCache(root, sheet, viewport, nil, &states[0], nil, &cache)
	if err != nil {
		t.Fatal(err)
	}
	initial := styles[first]
	for round := 0; round < 2; round++ {
		for i := 1; i < len(states); i++ {
			styles, _, err = RecomputeInputStateWithCache(root, sheet, viewport, &states[i-1], &states[i], styles, &cache)
			if err != nil {
				t.Fatal(err)
			}
			want, err := ComputeWithViewportAndStateAndQueries(root, sheet, viewport, &states[i], &QueryEnvironment{ContainerMetrics: cache.ContainerMetrics})
			if err != nil || !reflect.DeepEqual(styles, want) {
				t.Fatalf("cached input transition %d differs from fresh cascade: %v", i, err)
			}
		}
	}
	if styles[first] != initial {
		t.Fatal("returning to initial input state did not reuse its immutable style")
	}
}

func TestInputStateWithCacheWarmAllocations(t *testing.T) {
	root, err := dom.Parse(`<button>Hover</button>`)
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := css.Parse(`button:hover{color:red}`)
	if err != nil {
		t.Fatal(err)
	}
	var cache MutationCache
	styles, err := ComputeWithCache(root, sheet, Viewport{}, nil, nil, &cache)
	if err != nil {
		t.Fatal(err)
	}
	states := [2]css.State{{}, {Hovered: root.Children[0]}}
	if allocs := testing.AllocsPerRun(100, func() {
		for i := range states {
			if _, _, err := RecomputeInputStateWithCache(root, sheet, Viewport{}, &states[i], &states[1-i], styles, &cache); err != nil {
				panic(err)
			}
		}
	}); allocs != 0 {
		t.Fatalf("warm hover cycle allocated %g times", allocs)
	}
}
