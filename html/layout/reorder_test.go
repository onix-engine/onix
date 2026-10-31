package layout

import (
	"reflect"
	"testing"

	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/style"
)

func TestLocalFlexOrderRestoresDOMTiesAndDeduplicatesParents(t *testing.T) {
	root, styles, face := retainedFixture(t, `<div id="parent" style="display:flex;flex-wrap:wrap;width:100px"><div id="a" style="width:60px">A</div><div id="b" style="width:60px">B</div><div id="c" style="width:60px">C</div></div><div id="other" style="display:flex"><div>Other</div></div>`)
	var cache Cache
	parent := retainedNode(root, "parent")
	a := retainedNode(root, "a")
	b := retainedNode(root, "b")
	updateRetained(t, &cache, root, styles, face, nil, true)
	for _, orders := range [][2]float64{{2, 1}, {0, 0}, {-1, -2}, {0, 0}, {2, 2}, {1, 1}} {
		styles.Exclusive(a).Order = style.Number(orders[0])
		styles.Exclusive(b).Order = style.Number(orders[1])
		nodes := []*dom.Node{a, b, a}
		got, err := cache.Update(root, styles, face, face, 320, 200, nodes, nodes, false, false)
		if err != nil {
			t.Fatal(err)
		}
		if cache.Work.TopologyVisited != 0 || cache.Work.OrderContainers != 1 {
			t.Fatalf("nonlocal sorting: %+v", cache.Work)
		}
		full, err := BuildWithFonts(root, styles, face, face, 320, 200)
		if err != nil {
			t.Fatal(err)
		}
		assertRetainedMatchesFull(t, got, full)
		if len(cache.Reordered) > 1 || len(cache.Reordered) == 1 && cache.Reordered[0] != cache.nodes[parent] {
			t.Fatal("wrong reordered parent")
		}
	}
	// Changing order values without changing relative order needs no owner reorder.
	styles.Exclusive(a).Order = style.Number(3)
	styles.Exclusive(b).Order = style.Number(3)
	nodes := []*dom.Node{a, b}
	if _, err := cache.Update(root, styles, face, face, 320, 200, nodes, nodes, false, false); err != nil {
		t.Fatal(err)
	}
	if len(cache.Reordered) != 0 {
		t.Fatal("unchanged relative order was reported as reordered")
	}
}

func init() {
	debugStyleDiff = func(a, b *style.ComputedStyle) string {
		x, y := reflect.ValueOf(*a), reflect.ValueOf(*b)
		out := ""
		for i := 0; i < x.NumField(); i++ {
			f, g := x.Field(i), y.Field(i)
			if f.Kind() == reflect.Struct {
				for j := 0; j < f.NumField(); j++ {
					if f.Field(j).CanInterface() && !reflect.DeepEqual(f.Field(j).Interface(), g.Field(j).Interface()) {
						out += " " + f.Type().Field(j).Name
					}
				}
				continue
			}
			if f.CanInterface() && !reflect.DeepEqual(f.Interface(), g.Interface()) {
				out += " " + x.Type().Field(i).Name
			}
		}
		return out
	}
}
