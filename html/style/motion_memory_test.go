package style

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/diiyw/onix/html/css"
	"github.com/diiyw/onix/html/dom"
)

func TestMotionExtensionsShareAndDetach(t *testing.T) {
	root, err := dom.Parse(`<section style="transition:transform 1s linear;animation:fade 2s linear infinite"><div style="width:10px;transition:inherit;animation:inherit"></div><div style="width:20px;transition:inherit;animation:inherit"></div><div></div></section>`)
	if err != nil {
		t.Fatal(err)
	}
	parent := root.Children[0]
	first, second, static := parent.Children[0], parent.Children[1], parent.Children[2]
	var cache MutationCache
	styles, err := ComputeWithCache(root, nil, Viewport{}, nil, nil, &cache)
	if err != nil {
		t.Fatal(err)
	}
	shared := styles[parent].motion
	if shared == nil || styles[first] == styles[second] || styles[first].motion != shared || styles[second].motion != shared || styles[static].motion != nil {
		t.Fatal("different layout styles did not share motion, or static child inherited it implicitly")
	}
	first.Attributes["style"] = "width:10px;transition:inherit;transition-duration:3s;animation:inherit;animation-delay:4s"
	if _, _, err := RecomputeSubtreesWithChanges(root, nil, Viewport{}, nil, styles, []*dom.Node{first}, nil, &cache); err != nil {
		t.Fatal(err)
	}
	if styles[first].TransitionFor(0).Duration != 3 || styles[first].Animations()[0].Delay != 4 || styles[first].motion == shared {
		t.Fatal("longhand overrides did not detach the motion extension")
	}
	for _, node := range []*dom.Node{parent, second} {
		if styles[node].motion != shared || styles[node].TransitionFor(0).Duration != 1 || styles[node].Animations()[0].Delay != 0 {
			t.Fatal("overriding one child changed a shared motion extension")
		}
	}
	first.Attributes["style"] = "width:10px"
	if _, _, err := RecomputeSubtreesWithChanges(root, nil, Viewport{}, nil, styles, []*dom.Node{first}, nil, &cache); err != nil {
		t.Fatal(err)
	}
	if styles[first].motion != nil || styles[first].HasMotion() {
		t.Fatal("removing motion declarations retained the extension")
	}
	// Explicit mutable copies must also detach from the immutable shared block.
	copy := *styles[second]
	if err := applyTransitionProperty(&copy, "transition-duration", "7s"); err != nil {
		t.Fatal(err)
	}
	resolveMotion(copy.mutableMotion())
	if copy.TransitionFor(0).Duration != 7 || styles[second].TransitionFor(0).Duration != 1 {
		t.Fatal("editing a copied style mutated the interned extension")
	}
}

func TestMotionCascadeWarmAllocations(t *testing.T) {
	for _, inline := range []bool{false, true} {
		t.Run(fmt.Sprint(inline), func(t *testing.T) {
			const declarations = `transition:transform 1s steps(4), color 2s linear;transition-delay:-100ms;animation:fade 2s linear infinite;animation-duration:3s`
			source, cssSource := `<section><div></div><div></div></section>`, `div{`+declarations+`}`
			if inline {
				source, cssSource = `<section><div style="`+declarations+`"></div><div style="`+declarations+`"></div></section>`, ""
			}
			root, err := dom.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			sheet, err := css.Parse(cssSource)
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
				t.Fatalf("warm motion cascade allocated %g times", allocs)
			}
		})
	}
}

func TestMotionExplicitDefaultsRemainInactive(t *testing.T) {
	root, err := dom.Parse(`<div style="transition:none;animation:none;animation-duration:2s"></div>`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := Compute(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	computed := styles[root.Children[0]]
	if computed.HasMotion() || computed.Animations()[0].Duration != 2 {
		t.Fatal("explicit motion defaults became active or lost inheritable longhand values")
	}
}

func TestMotionCSSWideLonghands(t *testing.T) {
	const parentValues = `transition:opacity 2s linear -500ms;animation:spin 3s steps(4) -250ms 2 alternate both paused`
	const childValues = `transition:transform 7s ease-in 500ms;animation:fade 8s ease-out 2s 4 reverse forwards running`
	for _, property := range []struct{ name, parent, initial string }{
		{"transition-property", "opacity", "all"},
		{"transition-duration", "2s", "0s"},
		{"transition-timing-function", "linear", "ease"},
		{"transition-delay", "-500ms", "0s"},
		{"animation-name", "spin", "none"},
		{"animation-duration", "3s", "0s"},
		{"animation-timing-function", "steps(4)", "ease"},
		{"animation-delay", "-250ms", "0s"},
		{"animation-iteration-count", "2", "1"},
		{"animation-direction", "alternate", "normal"},
		{"animation-fill-mode", "both", "none"},
		{"animation-play-state", "paused", "running"},
	} {
		for _, keyword := range []string{"inherit", "initial", "unset"} {
			t.Run(property.name+"/"+keyword, func(t *testing.T) {
				literal := property.initial
				if keyword == "inherit" {
					literal = property.parent
				}
				source := fmt.Sprintf(`<section style="%s"><div style="%s;%s:%s"></div><div style="%s;%s:%s"></div></section>`, parentValues, childValues, property.name, keyword, childValues, property.name, literal)
				root, err := dom.Parse(source)
				if err != nil {
					t.Fatal(err)
				}
				styles, err := Compute(root, nil)
				if err != nil {
					t.Fatal(err)
				}
				children := root.Children[0].Children
				got, want := styles[children[0]], styles[children[1]]
				for i := range AnimatedPropertyCount {
					if got.TransitionFor(i) != want.TransitionFor(i) {
						t.Fatalf("transition %d: %+v; want %+v", i, got.TransitionFor(i), want.TransitionFor(i))
					}
				}
				if !slices.Equal(got.Animations(), want.Animations()) || got.HasMotion() != want.HasMotion() {
					t.Fatalf("animations = %+v; want %+v", got.Animations(), want.Animations())
				}
			})
		}
	}
}

func TestMotionCSSWideShorthandReset(t *testing.T) {
	root, err := dom.Parse(`<section style="transition:transform 1s;animation:fade 2s"><div style="transition:inherit;animation:inherit;transition:initial;animation:unset"></div></section>`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := Compute(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	parent := root.Children[0]
	if styles[parent.Children[0]].motion != nil || !styles[parent].HasMotion() {
		t.Fatal("reset did not release motion or changed the parent")
	}
}

func TestMotionInterningBoundAndReset(t *testing.T) {
	index := styleIndex{}
	index.limit = 4
	for i := range 6 {
		motion := motionStyle{motionInputs: motionInputs{animation: [8]string{fmt.Sprintf("name%d", i)}}}
		if index.resolveMotion(&motion) == nil {
			t.Fatal("motion extension was lost after reaching the index limit")
		}
	}
	if index.motions.used != index.limit {
		t.Fatalf("motion index retained %d entries; want %d", index.motions.used, index.limit)
	}
	index.reset()
	if index.motions.used != 0 {
		t.Fatal("motion index was not reset")
	}
	for _, slot := range index.motions.slots {
		if slot.value != nil {
			t.Fatal("reset motion index retained an extension")
		}
	}
}

func BenchmarkComputeMotionStyles(b *testing.B) {
	var source strings.Builder
	for i := range 256 {
		fmt.Fprintf(&source, `<div style="width:%dpx"></div>`, i)
	}
	root, err := dom.Parse(source.String())
	if err != nil {
		b.Fatal(err)
	}
	for _, declarations := range []struct{ name, value string }{
		{"static", ""},
		{"transition", "transition:transform 1s linear, color 500ms ease-in"},
		{"animation", "animation:fade 1s linear infinite"},
	} {
		sheet, err := css.Parse(`div{` + declarations.value + `}`)
		if err != nil {
			b.Fatal(err)
		}
		for _, cold := range []bool{true, false} {
			name := "warm"
			if cold {
				name = "cold"
			}
			b.Run(declarations.name+"/"+name, func(b *testing.B) {
				var cache MutationCache
				var styles Map
				compute := func() {
					styles, err = ComputeWithCache(root, sheet, Viewport{}, nil, styles, &cache)
					if err != nil {
						b.Fatal(err)
					}
				}
				compute()
				b.ReportAllocs()
				for b.Loop() {
					if cold {
						cache, styles = MutationCache{}, nil
					}
					compute()
				}
			})
		}
	}
}
