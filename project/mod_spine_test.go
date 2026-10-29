package project

import (
	"image"
	"testing"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/spine"
)

func TestModSpineContentAppliesConfiguredAnimationBeforeFirstTick(t *testing.T) {
	texture := graphics.CreateTextureByImageWithUsage(image.NewRGBA(image.Rect(0, 0, 10, 10)), false, graphics.ResourceUsageCPU)
	t.Cleanup(func() { _ = texture.Dispose() })
	resource, err := spine.Load([]byte(`{
		"bones":[{"name":"root"}],
		"slots":[{"name":"slot","bone":"root","attachment":"region"}],
		"skins":{"default":{"slot":{"region":{"width":10,"height":10}}}},
		"animations":{"idle":{},"move":{"bones":{"root":{"translate":[{"time":0,"x":20,"y":30},{"time":1,"x":100,"y":30}]}}}}
	}`), []byte("page.png\nsize: 10, 10\nregion\n  rotate: false\n  xy: 0, 0\n  size: 10, 10\n  orig: 10, 10\n  offset: 0, 0\n"), texture)
	if err != nil {
		t.Fatal(err)
	}
	resources := newResources()
	cache := newResourceCache[string, *spine.Resource](1)
	if err := cache.register("actor", func() (*spine.Resource, error) { return resource, nil }, nil); err != nil {
		t.Fatal(err)
	}
	resources.spines = cache
	bundle, err := resources.ModContent(ModComponents{Spine: &ModSpineSpec{Source: SpineSpec{ID: "actor"}, Animation: "move", Loop: true}})
	if err != nil {
		t.Fatal(err)
	}
	component, ok := bundle.Value[scene.Spine2D]()
	if !ok || component.Player == nil {
		t.Fatal("Spine content has no player")
	}
	minX, minY, maxX, maxY, valid := component.Player.Bounds()
	if !valid || minX != 15 || minY != 25 || maxX != 25 || maxY != 35 || component.Player.AnimationTime() != 0 {
		t.Fatalf("configured animation was not applied at time zero: (%g,%g)-(%g,%g)", minX, minY, maxX, maxY)
	}
}
