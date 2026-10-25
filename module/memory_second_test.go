package module

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/scene"
)

func TestDisposedContentReleasesPrivateState(t *testing.T) {
	_, content, other := boundaryContent(t)
	c := content.context
	ContentStore(content, make([]byte, 1<<20))
	ContentStore(other, "live")
	c.pausedVisibility = map[ecs.Entity]scene.Visibility{c.root: {Visible: true}}
	must(t, c.Dispose())
	if len(content.state) != 0 || content.input != nil || c.content != nil || c.elements != nil || c.pausedVisibility != nil {
		t.Fatal("disposed module retains private content state")
	}
	ContentStore(content, []byte{1})
	if _, ok := ContentResourceMut[[]byte](content); ok || c.Content() != nil {
		t.Fatal("disposed content recreated private storage")
	}
	if value, ok := ContentResource[string](other); !ok || value != "live" {
		t.Fatal("disposing content changed another module")
	}
}

func TestContentInputIsCreatedOnDemand(t *testing.T) {
	r, content, _ := boundaryContent(t)
	if content.input != nil {
		t.Fatal("content eagerly allocates unused input storage")
	}
	state := input.NewState()
	ecs.InsertResource(r.world, state)
	state.PressKey(input.KeyA)
	copy, ok := ContentResource[*input.State](content)
	if !ok || copy == state || !copy.KeyHeld(input.KeyA) {
		t.Fatal("private input snapshot was not initialized")
	}
	if allocations := testing.AllocsPerRun(100, func() { ContentResource[*input.State](content) }); allocations != 0 {
		t.Fatalf("input snapshot allocated %g times after warmup", allocations)
	}
}

func TestDestroyCanReadContentBeforeCleanup(t *testing.T) {
	for _, closeRuntime := range []bool{false, true} {
		plan, _ := compileFixture(t, map[string]string{
			"resources/scenes/main.json": `{"modules":[{"id":"a","mod":"box"}]}`,
			"box/mod.json":               `{}`,
		})
		registry := Registry{}
		calls := 0
		must(t, RegisterMod(&registry, "box", func() Signal { return Signal{} }, Lifecycle[Signal]{
			Install: func(c *Context, _ *Signal) error {
				ContentStore(c.Content(), "private")
				return nil
			},
			Destroy: func(c *Context, _ *Signal) error {
				content := c.Content()
				value, ok := ContentResource[string](content)
				if !ok || value != "private" {
					return ErrInvalid
				}
				ContentStore(content, 7)
				if value, ok := ContentResourceMut[int](content); !ok || *value != 7 {
					return ErrInvalid
				}
				calls++
				return nil
			},
		}))
		r := newFixture(t, plan, registry, Options{})
		s, err := r.MountEntry()
		must(t, err)
		if closeRuntime {
			must(t, r.Close())
		} else {
			must(t, s.modules[0].Dispose())
		}
		if calls != 1 {
			t.Fatal("destroy callback did not run")
		}
	}
}

func BenchmarkContentCreation(b *testing.B) {
	r := &Runtime{}
	b.ReportAllocs()
	for b.Loop() {
		c := &Context{runtime: r, live: true}
		content := c.Content()
		ContentStore(content, 1)
	}
}
