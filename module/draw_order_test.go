package module

import (
	"fmt"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/ui"
)

func TestProceduralPainterOrderSurvivesModuleChanges(t *testing.T) {
	r, content, _ := boundaryContent(t)
	bundle := ecs.NewBundle(2)
	bundle.Put(scene.UIStackIndex(1_000_000))
	bundle.Put(scene.RenderOrder2D(500))
	visual := content.Spawn(bundle)
	node := content.SpawnNode(ui.NodeOptions{StackIndex: 700}, boundaryValue{})
	var queued ecs.Entity
	for range content.Query[boundaryValue]() {
		queued = content.SpawnNode(ui.NodeOptions{StackIndex: 900}, boundaryExtra{})
	}
	must(t, r.flushStructural())
	// Applications can move an existing visual without surrendering its order.
	content.SetIfChanged(visual, scene.UIStackIndex(1_000_001))
	content.Spawn(ecs.NewBundle(0))
	must(t, r.flushStructural())
	for entity, want := range map[ecs.Entity]scene.UIStackIndex{visual: 1_000_001, node: 700, queued: 900} {
		if got, _ := content.Get[scene.UIStackIndex](entity); got != want {
			t.Fatalf("procedural paint order=%d, want %d", got, want)
		}
	}
	if got, _ := content.Get[scene.RenderOrder2D](visual); got != 500 {
		t.Fatalf("world paint order=%d, want 500", got)
	}
	if allocs := testing.AllocsPerRun(100, func() {
		content.context.dirtyDrawOrder()
		if err := r.syncDrawOrder(); err != nil {
			panic(err)
		}
	}); allocs != 0 {
		t.Fatalf("warm paint order synchronization allocated %g times", allocs)
	}
}

func drawOrderFixture(tb testing.TB) (*Runtime, *Scene) {
	tb.Helper()
	var spec strings.Builder
	spec.WriteString(`{"modules":[`)
	for i := range 64 {
		if i != 0 {
			spec.WriteByte(',')
		}
		fmt.Fprintf(&spec, `{"id":"m%d","mod":"box"}`, i)
	}
	spec.WriteString(`]}`)
	plan, _ := compileFixture(tb, map[string]string{"resources/scenes/main.json": spec.String(), "box/mod.json": `{}`})
	r := newFixture(tb, plan, Registry{}, Options{})
	s, err := r.MountEntry()
	must(tb, err)
	for i := range s.modules {
		c := &s.modules[i]
		for range 127 {
			_, err := c.Spawn(Entity{})
			must(tb, err)
		}
	}
	must(tb, r.flushStructural())
	return r, s
}

func TestDrawOrderOnlyWritesChangedModule(t *testing.T) {
	r, s := drawOrderFixture(t)
	c := &s.modules[0]
	last := &s.modules[len(s.modules)-1]
	otherOrder, _ := r.world.Get[scene.UIStackIndex](last.entities[len(last.entities)-1])
	version := r.world.ComponentVersion[scene.UIStackIndex]()
	_, err := c.Spawn(Entity{})
	must(t, err)
	must(t, r.flushStructural())
	if delta := r.world.ComponentVersion[scene.UIStackIndex]() - version; delta != 1 {
		t.Fatalf("appending one node wrote %d stack records", delta)
	}
	if now, _ := r.world.Get[scene.UIStackIndex](last.entities[len(last.entities)-1]); now != otherOrder {
		t.Fatal("appending content changed another module")
	}
	version = r.world.ComponentVersion[scene.UIStackIndex]()
	c.dirtyDrawOrder()
	must(t, r.syncDrawOrder())
	if r.world.ComponentVersion[scene.UIStackIndex]() != version {
		t.Fatal("unchanged local order advanced component versions")
	}
}

func BenchmarkDrawOrderSync(b *testing.B) {
	for _, global := range []bool{true, false} {
		name := "module"
		if global {
			name = "global"
		}
		b.Run(name, func(b *testing.B) {
			r, s := drawOrderFixture(b)
			c := &s.modules[0]
			entity := c.entities[len(c.entities)-1]
			b.ReportAllocs()
			for b.Loop() {
				r.world.SetIfChanged(entity, scene.UIStackIndex(-1))
				if global {
					order := 0
					for i := range s.modules {
						for _, e := range s.modules[i].entities {
							order++
							if err := r.world.Insert(e, scene.UIStackIndex(order)); err != nil {
								b.Fatal(err)
							}
						}
					}
				} else {
					c.dirtyDrawOrder()
					if err := r.syncDrawOrder(); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}
