package module

import (
	"fmt"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

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
	otherOrder, _ := ecs.Get[scene.UIStackIndex](r.world, last.entities[len(last.entities)-1])
	version := ecs.ComponentVersion[scene.UIStackIndex](r.world)
	_, err := c.Spawn(Entity{})
	must(t, err)
	must(t, r.flushStructural())
	if delta := ecs.ComponentVersion[scene.UIStackIndex](r.world) - version; delta != 1 {
		t.Fatalf("appending one node wrote %d stack records", delta)
	}
	if now, _ := ecs.Get[scene.UIStackIndex](r.world, last.entities[len(last.entities)-1]); now != otherOrder {
		t.Fatal("appending content changed another module")
	}
	version = ecs.ComponentVersion[scene.UIStackIndex](r.world)
	c.dirtyDrawOrder()
	must(t, r.syncDrawOrder())
	if ecs.ComponentVersion[scene.UIStackIndex](r.world) != version {
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
				ecs.SetIfChanged(r.world, entity, scene.UIStackIndex(-1))
				if global {
					order := 0
					for i := range s.modules {
						for _, e := range s.modules[i].entities {
							order++
							if err := ecs.Insert(r.world, e, scene.UIStackIndex(order)); err != nil {
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
