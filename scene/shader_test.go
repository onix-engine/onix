package scene

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
)

func TestShaderComponentOverridesMaterialAndRemovalRestoresIt(t *testing.T) {
	world := ecs.NewWorld()
	RenderWorldFor(world).RegisterMaterial2D[customMaterial2DTest]()
	base, attached := new(graphics.Material2DShader), new(graphics.Material2DShader)
	material := customMaterial2DTest{shader: base, value: .25}
	bundle := ecs.NewBundle(2)
	bundle.Put(MeshMaterial2D[customMaterial2DTest]{Material: &material})
	bundle.Put(Shader[graphics.Material2DShader]{Program: attached, Custom: [4][4]float32{{.75}}})
	entity := world.Spawn(bundle)
	if got := material2DForEntity(world, entity); got.Shader != attached || got.Custom[0][0] != .75 {
		t.Fatal("element shader did not override the material pipeline and uniforms")
	}
	if allocations := testing.AllocsPerRun(100, func() { _ = material2DForEntity(world, entity) }); allocations != 0 {
		t.Fatalf("shader extraction allocated %g", allocations)
	}
	world.Remove[Shader[graphics.Material2DShader]](entity)
	if got := material2DForEntity(world, entity); got.Shader != base || got.Custom[0][0] != .25 {
		t.Fatal("removing Shader did not restore the material pipeline")
	}
}
