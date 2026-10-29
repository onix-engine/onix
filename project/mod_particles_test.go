package project

import (
	"slices"
	"testing"
	"testing/fstest"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene"
)

func TestModuleEmitterCompilesAndInstantiatesMultipleParticles(t *testing.T) {
	source := fstest.MapFS{
		"resources/project.json":     {Data: []byte(`{"entry":"main.json"}`)},
		"resources/scenes/main.json": {Data: []byte(`{"modules":[{"id":"effect","mod":"effect"}]}`)},
		"effect/mod.json": {Data: []byte(`{
			"components":{"emitter":{"particles":["red","blue"],"count":16,"lifetime":1,"sizeMin":2,"sizeMax":4,"alphaStart":1}},
			"elements":[{"id":"red","components":{"particle":{"color":"#ff0000"}}},{"id":"blue","components":{"particle":{"color":"#0000ff"}}}]
		}`)},
	}
	plan, err := CompileProject(source, ".")
	if err != nil {
		t.Fatal(err)
	}
	mod := &plan.Mods[plan.Scenes[0].Instances[0].Mod]
	if !slices.Equal(mod.Spec.Components.Emitter.Particles, []string{"red", "blue"}) {
		t.Fatal("compiled particle references changed")
	}
	resources, err := OpenModResources(source, mod, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resources.Dispose() })
	bundle, err := resources.ModContent(mod.Spec.Components)
	if err != nil {
		t.Fatal(err)
	}
	emitter, ok := bundle.Value[scene.ParticleEmitter2D]()
	if !ok || len(emitter.Appearances) != 2 || emitter.Appearances[0].Color != graphics.ColorFromRGBA(255, 0, 0, 255) || emitter.Appearances[1].Color != graphics.ColorFromRGBA(0, 0, 255, 255) {
		t.Fatalf("particle appearances did not instantiate: %+v", emitter.Appearances)
	}
	otherBundle, err := resources.ModContent(mod.Spec.Components)
	if err != nil {
		t.Fatal(err)
	}
	other, _ := otherBundle.Value[scene.ParticleEmitter2D]()
	emitter.Appearances[0].Color = graphics.Black
	if other.Appearances[0].Color == graphics.Black {
		t.Fatal("emitter instances share mutable appearances")
	}
}
