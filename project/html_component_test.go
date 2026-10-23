package project

import (
	"testing"
	"testing/fstest"

	"github.com/diiyw/onix/ecs"
	htmlui "github.com/diiyw/onix/ui/html"
)

func TestHTMLResourceComponentMaterializesHTMLNode(t *testing.T) {
	source := fstest.MapFS{
		"project.json":                     &fstest.MapFile{Data: []byte(`{"window":{"width":320,"height":180}}`)},
		"resources/resources.json":         &fstest.MapFile{Data: []byte(`{"html":[{"id":"screen","path":"resources/ui/screen.html"}]}`)},
		"resources/ui/screen.html":         &fstest.MapFile{Data: []byte(`<main id="content">Hello</main>`)},
		"resources/scenes.json":            &fstest.MapFile{Data: []byte(`{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)},
		"resources/scenes/main.scene.json": &fstest.MapFile{Data: []byte(`{"nodes":[{"id":"screen","coordinateSpace":"ui","width":320,"height":180,"components":{"html":{"resource":"screen"}}}]}`)},
	}
	bundle, err := Load(source, ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()
	entity := scene.Entities["screen"]
	html, ok := ecs.Get[htmlui.Surface](scene.World, entity)
	if !ok || html.Value != `<main id="content">Hello</main>` {
		t.Fatalf("HTML component = %+v, present=%v", html, ok)
	}
}
