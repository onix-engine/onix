package style

import (
	"testing"

	"github.com/onix-engine/onix/html/css"
	"github.com/onix-engine/onix/html/dom"
)

func TestBackgroundInheritanceAndInterningKeepLayersIndependent(t *testing.T) {
	root, err := dom.Parse(`<section><div id="first"></div><div id="second"></div></section>`)
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := css.Parse(`section { background: url(first.png) left top / 20px 30px no-repeat, url(second.png) right bottom / 40px 50px repeat-x; } div { background: inherit; }`)
	if err != nil {
		t.Fatal(err)
	}
	var cache MutationCache
	styles, err := ComputeWithCache(root, sheet, Viewport{}, nil, nil, &cache)
	if err != nil {
		t.Fatal(err)
	}
	parent := root.Children[0]
	first, second := parent.Children[0], parent.Children[1]
	if styles[first] != styles[second] || styles[first].BackgroundLayerCount() != 2 {
		t.Fatal("identical inherited backgrounds did not share their computed style")
	}
	for index, image := range [...]string{"first.png", "second.png"} {
		if styles[first].BackgroundLayer(index).Image != image {
			t.Fatalf("inherited layer %d lost its image", index)
		}
	}
	first.SetAttr("style", "background-size: 20px 30px, 80px 90px")
	if _, _, err := RecomputeSubtreesWithChanges(root, sheet, Viewport{}, nil, styles, []*dom.Node{first}, nil, &cache); err != nil {
		t.Fatal(err)
	}
	if styles[first] == styles[second] || styles[first].Background.SizeX != LengthPx(20) || styles[first].ExtraBackgroundLayers[0].SizeX != LengthPx(80) {
		t.Fatal("overriding background sizes did not detach both layers")
	}
	for _, node := range []*dom.Node{parent, second} {
		if styles[node].Background.SizeX != LengthPx(20) || styles[node].ExtraBackgroundLayers[0].SizeX != LengthPx(40) {
			t.Fatal("changing one child corrupted another background")
		}
	}
	first.SetAttr("style", "background: red")
	if _, _, err := RecomputeSubtreesWithChanges(root, sheet, Viewport{}, nil, styles, []*dom.Node{first}, nil, &cache); err != nil {
		t.Fatal(err)
	}
	if styles[first].BackgroundLayerCount() != 1 || styles[first].Background.Image != "" || styles[first].BackgroundColor.Color != (Color{R: 255, A: 255}) {
		t.Fatal("a color shorthand retained the previous images")
	}
}
