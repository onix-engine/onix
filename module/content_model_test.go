//go:build !onix_release

package module

import (
	"os"
	"testing"

	"github.com/onix-engine/onix/graphics"
	"github.com/onix-engine/onix/project"
	"github.com/onix-engine/onix/scene/render3d"
)

func TestModuleImportedModelSharesRigAndIsolatesPoseAndMaterials(t *testing.T) {
	data, err := os.ReadFile("../project/resources/fixtures/gltf/embedded_cubic.gltf")
	must(t, err)
	plan, source := compileFixture(t, map[string]string{
		"resources/scenes/main.json": `{"modules":[{"id":"a","mod":"model"},{"id":"b","mod":"model"}]}`,
		"model/mod.json":             `{"space":"world","components":{"model":{"id":"model","path":"model.gltf"}}}`,
		"model/model.gltf":           string(data),
	})
	r := newFixture(t, plan, Registry{}, Options{Source: source, Resources: project.ImageFeature[contentPNGDecoder]{}})
	s, err := r.Manager().StartEntry()
	must(t, err)
	a, b := &s.modules[0], &s.modules[1]
	rigA, okA := a.Get[*project.ModelRig](a.Root())
	rigB, okB := b.Get[*project.ModelRig](b.Root())
	poseA, _ := a.Get[*project.ModelPose](a.Root())
	poseB, _ := b.Get[*project.ModelPose](b.Root())
	if !okA || !okB || rigA != rigB || poseA == nil || poseB == nil || poseA == poseB {
		t.Fatal("model rig or pose sharing is incorrect")
	}
	materialA, _ := a.Get[render3d.Material[graphics.StandardMaterial]](a.Root())
	materialB, _ := b.Get[render3d.Material[graphics.StandardMaterial]](b.Root())
	if materialA.Material == nil || materialB.Material == nil || materialA.Material == materialB.Material {
		t.Fatal("model material crossed instances")
	}
	materialA.Material.PerceptualRoughness = .123
	if materialB.Material.PerceptualRoughness == .123 {
		t.Fatal("mutable material crossed instances")
	}
	meshA, _ := a.Get[render3d.Mesh](a.Root())
	meshB, _ := b.Get[render3d.Mesh](b.Root())
	if meshA.Mesh == nil || meshA.Mesh != meshB.Mesh {
		t.Fatal("immutable mesh was not shared")
	}
	clip, err := rigA.Animation("cubic-move")
	must(t, err)
	poseA.Sample(clip, .5, false)
	if poseA.NodeMatrix(0) == poseB.NodeMatrix(0) {
		t.Fatal("animation pose crossed instances")
	}
	if allocs := testing.AllocsPerRun(100, func() { poseA.Sample(clip, .5, false) }); allocs != 0 {
		t.Fatalf("warm model sampling allocated %g", allocs)
	}
	must(t, a.Dispose())
	poseB.Sample(clip, .5, false)
	if len(r.contentResources) != 1 {
		t.Fatal("first instance released shared model")
	}
	must(t, b.Dispose())
	if len(r.contentResources) != 0 {
		t.Fatal("model content store leaked")
	}
}
