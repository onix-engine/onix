package html

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/html/style"
	"strings"
	"testing"
)

func TestHTMLNativeAttributeStylesStayLocal(t *testing.T) {
	const css = `<style>@media(min-width:100px){#hot span{color:red}[title="hot"] span{font-size:20px}[aria-selected="true"]+p{color:red}#hot+p{background:blue}}</style>`
	const prefix = `<article><div id="target"`
	const suffix = `><span>Child</span></div><p>Sibling</p></article><aside>`
	unchanged := strings.Repeat(`<div>Unchanged</div>`, 250) + `</aside>`
	for _, attribute := range []struct{ name, value, authored string }{
		{"id", "hot", ` id="hot"`},
		{"title", "hot", ` id="target" title="hot"`},
		{"aria-selected", "true", ` id="target" aria-selected="true"`},
		{"hidden", "", ` id="target" hidden=""`},
	} {
		t.Run(attribute.name, func(t *testing.T) {
			world, owner := incrementalHTMLFixture(t, css+prefix+suffix+unchanged)
			target := incrementalHTMLNode(t, world, owner, "target")
			target.SetAttribute(attribute.name, attribute.value)
			incrementalSync(t, world)
			state := ecs.GetRef[htmlRenderState](world, owner)
			if state.SyncStats.Styled > 8 {
				t.Fatalf("attribute restyled unrelated branches: %+v", state.SyncStats)
			}
			assertHTMLPartialMatchesFresh(t, world, owner, css+`<article><div`+attribute.authored+suffix+unchanged)
			target.RemoveAttribute(attribute.name)
			incrementalSync(t, world)
			id := ` id="target"`
			if attribute.name == "id" {
				id = ""
			}
			assertHTMLPartialMatchesFresh(t, world, owner, css+`<article><div`+id+suffix+unchanged)
		})
	}
}

func TestHTMLMediaEnvironmentStillRestylesDocument(t *testing.T) {
	const source = `<style>#target{width:10px}@media(min-width:100px){#target{width:20px}}</style><div id="scope"><div id="target"></div></div>`
	world, owner := incrementalHTMLFixture(t, source)
	node := incrementalHTMLNode(t, world, owner, "target")
	if ecs.GetRef[htmlRenderState](world, owner).Styles[node.node].Width != style.LengthPx(20) {
		t.Fatal("media query did not apply")
	}
	// A local mutation may reuse the environment, but resizing must reevaluate it.
	node.SetAttribute("title", "warm")
	incrementalSync(t, world)
	ecs.GetMut[Surface](world, owner).Width = 80
	incrementalSync(t, world)
	if ecs.GetRef[htmlRenderState](world, owner).Styles[node.node].Width != style.LengthPx(10) {
		t.Fatal("media query stayed stale after resize")
	}
}

func TestHTMLFilterIDAttributeInvalidatesConsumers(t *testing.T) {
	const before = `<svg><filter id="fx"><feGaussianBlur stdDeviation="1"></feGaussianBlur></filter><filter id="other"><feGaussianBlur stdDeviation="2"></feGaussianBlur></filter></svg><div id="target" style="filter:url(#fx)">Text</div>`
	world, owner := incrementalHTMLFixture(t, before)
	filter := incrementalHTMLNode(t, world, owner, "other")
	incrementalHTMLNode(t, world, owner, "target").SetAttribute("class", "warm")
	incrementalSync(t, world)
	filter.SetAttribute("id", "fx")
	incrementalSync(t, world)
	assertHTMLPartialMatchesFresh(t, world, owner, strings.Replace(strings.Replace(before, `id="other"`, `id="fx"`, 1), `id="target"`, `id="target" class="warm"`, 1))
	filter.SetAttribute("id", "other")
	incrementalSync(t, world)
	assertHTMLPartialMatchesFresh(t, world, owner, strings.Replace(before, `id="target"`, `id="target" class="warm"`, 1))
}
