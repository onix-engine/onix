package render3d

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene"
)

func TestShaderComponentPreservesMeshMaterialsAndOverridesEverySlot(t *testing.T) {
	world := ecs.NewWorld()
	WorldFor(world).RegisterMaterial[graphics.StandardMaterial]()
	base := graphics.NewStandardMaterial(graphics.White)
	red := graphics.NewStandardMaterial(graphics.ColorFromRGB(255, 0, 0))
	shader := new(graphics.MeshShader)
	bundle := ecs.NewBundle(2)
	bundle.Put(Material[graphics.StandardMaterial]{Material: base, Materials: []graphics.StandardMaterial{*red}})
	bundle.Put(scene.Shader[graphics.MeshShader]{Program: shader, Custom: [4][4]float32{{.75}}})
	entity := world.Spawn(bundle)
	state := WorldFor(world)
	for _, slot := range []int{-1, 0} {
		got := state.materialFor(world, entity, slot)
		want := base.BaseColor
		if slot == 0 {
			want = red.BaseColor
		}
		if got.Shader != shader || got.Custom[0][0] != .75 || got.BaseColor != want || got.PerceptualRoughness != base.PerceptualRoughness {
			t.Fatal("shader component replaced the mesh surface material", slot)
		}
	}
	if base.Shader != nil || red.Shader != nil {
		t.Fatal("element shader mutated shared materials")
	}
	if allocations := testing.AllocsPerRun(100, func() { _ = state.materialFor(world, entity, 0) }); allocations != 0 {
		t.Fatalf("mesh shader extraction allocated %g", allocations)
	}
	world.Remove[scene.Shader[graphics.MeshShader]](entity)
	if got := state.materialFor(world, entity, 0); got.Shader != nil || got.BaseColor != red.BaseColor {
		t.Fatal("removing Shader did not restore the mesh material")
	}
}
