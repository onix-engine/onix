package module

import (
	"strings"
	"testing"

	"github.com/diiyw/onix/physics"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/ui"
)

func TestContentProfilesRejectUnsupportedComponents(t *testing.T) {
	for _, components := range []string{
		`"html":{"resource":"page"}`,
		`"animation":{"clip":{"duration":1,"sampleRate":60,"floatTracks":[{"binding":{"target":"transform.position.x"},"keyframes":[{"time":0,"value":0},{"time":1,"value":10}]}]}}`,
		`"layout":{}`,
	} {
		t.Run(components, func(t *testing.T) {
			plan, source := compileFixture(t, map[string]string{
				"resources/scenes/main.json": `{"modules":[{"id":"a","mod":"box"}]}`,
				"box/mod.json":               `{"components":{` + components + `},"resources":{"page":{"kind":"html","path":"page.html"}}}`,
				"box/page.html":              `<button>hello</button>`,
			})
			r := newFixtureWith[Render2D](t, plan, Registry{}, Options{Source: source})
			if _, err := r.Manager().StartEntry(); err == nil || !strings.Contains(err.Error(), "content profile") {
				t.Fatalf("unsupported component was not rejected: %v", err)
			}
			if r.world.EntityCount() != 0 || len(r.contentResources) != 0 {
				t.Fatal("failed mount retained entities or private resources")
			}
		})
	}
}

func TestComposedContentProfilesConstructOnlySelectedCapabilities(t *testing.T) {
	plan, source := compileFixture(t, map[string]string{
		"resources/scenes/main.json": `{"modules":[{"id":"a","mod":"box"},{"id":"b","mod":"body"}]}`,
		"box/mod.json":               `{"components":{"sprite":{"color":"#ffffff","size":[20,30]},"layout":{}}}`,
		"body/mod.json":              `{"space":"world","components":{"rigidBody2D":{},"circleCollider2D":{"radius":10}}}`,
	})
	type Profile = WithContent[WithContent[WithContent[Render2D, LayoutContent], LayoutContent], Physics2DContent]
	r := newFixtureWith[Profile](t, plan, Registry{}, Options{Source: source})
	s, err := r.Manager().StartEntry()
	must(t, err)
	e := s.modules[0].root
	if !r.world.Has[ui.Layout](e) || !r.world.Has[physics.Collider2D[physics.Circle]](s.modules[1].root) || !r.world.Has[scene.Sprite2D](e) {
		t.Fatal("composed profile lost a selected component")
	}
	if r.content.syncHTML != nil || r.content.deform != nil || r.content.release != nil {
		t.Fatal("unused HTML/3D lifecycle was installed")
	}
	must(t, r.Close())
	if r.world.EntityCount() != 0 {
		t.Fatal("composed profile leaked entities")
	}
}

func TestContentAnimationsProfileSharesClipsAndKeepsPlaybackPrivate(t *testing.T) {
	plan, source := compileFixture(t, map[string]string{
		"resources/scenes/main.json": `{"modules":[{"id":"a","mod":"box"},{"id":"b","mod":"box"}]}`,
		"box/mod.json":               `{"components":{"sprite":{"color":"#ffffff","size":[20,30]},"animation":{"clip":{"duration":1,"sampleRate":60,"loop":true,"floatTracks":[{"binding":{"target":"transform.position.x"},"keyframes":[{"time":0,"value":0},{"time":1,"value":10}]}]}}}}`,
	})
	r := newFixtureWith[WithContent[Render2D, AnimationsContent]](t, plan, Registry{}, Options{Source: source})
	s, err := r.Manager().StartEntry()
	must(t, err)
	a := r.world.GetMut[scene.Animator](s.modules[0].root)
	b := r.world.GetMut[scene.Animator](s.modules[1].root)
	if a == nil || b == nil || a == b || a.Clip == nil || a.Clip != b.Clip {
		t.Fatal("animation profile did not share only the immutable clip")
	}
	a.Playing = false
	if !b.Playing {
		t.Fatal("animation playback leaked to another instance")
	}
	must(t, r.Close())
	if r.world.EntityCount() != 0 || len(r.contentResources) != 0 {
		t.Fatal("2D content was not released")
	}
}
