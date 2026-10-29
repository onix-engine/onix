//go:build !onix_release

package project

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"
	"testing/fstest"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/internal/typedbin"
	"github.com/diiyw/onix/scene/render3d"
)

func fresnelGLTF() string {
	return fmt.Sprintf(`{"asset":{"version":"2.0"},"extensionsRequired":["KHR_materials_ior","KHR_materials_emissive_strength"],
"buffers":[{"uri":"data:application/octet-stream;base64,%s","byteLength":36}],
"bufferViews":[{"buffer":0,"byteLength":36}],"accessors":[{"bufferView":0,"componentType":5126,"count":3,"type":"VEC3"}],
"materials":[{"emissiveFactor":[0.25,0.5,1],"extensions":{"KHR_materials_ior":{"ior":2.42},"KHR_materials_emissive_strength":{"emissiveStrength":64}}}],
"meshes":[{"primitives":[{"attributes":{"POSITION":0},"material":0}]}]}`, base64.StdEncoding.EncodeToString(testTriangleBuffer()))
}

func TestModelFresnelImportOverridesAndPreprocessing(t *testing.T) {
	for _, override := range []string{``, `,"ior":0,"emissiveStrength":0`, `,"ior":1.33,"emissiveStrength":8`} {
		for _, binary := range []bool{false, true} {
			source := fstest.MapFS{
				"resources/project.json":           {Data: []byte(`{}`)},
				"resources/scenes.json":            {Data: []byte(`{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)},
				"resources/resources.json":         {Data: []byte(fmt.Sprintf(`{"models":[{"id":"surface","path":"resources/model.gltf","material":{"metallic":0%s}}]}`, override))},
				"resources/model.gltf":             {Data: []byte(fresnelGLTF())},
				"resources/scenes/main.scene.json": {Data: []byte(`{"nodes":[{"id":"mesh","components":{"model":{"resource":"surface"}}}]}`)},
			}
			if binary {
				files, err := PreprocessManifests(source, PreprocessOptions{})
				if err != nil {
					t.Fatal(err)
				}
				for _, file := range files {
					source[file.Path] = &fstest.MapFile{Data: file.Data}
				}
			}
			bundle, err := loadTestProject(source, ".")
			if err != nil {
				t.Fatal(err)
			}
			scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
			if err != nil {
				t.Fatal(err)
			}
			material, ok := scene.World.Get[render3d.Material[graphics.StandardMaterial]](scene.MustEntity("mesh"))
			if !ok || material.Material == nil {
				t.Fatal("missing material")
			}
			ior, strength := float32(2.42), float32(64)
			if override == `,"ior":0,"emissiveStrength":0` {
				ior, strength = 0, 0
			}
			if override == `,"ior":1.33,"emissiveStrength":8` {
				ior, strength = 1.33, 8
			}
			if m := material.Material; m.IOR != ior || m.EmissiveStrength != strength || m.Emissive != (graphics.ColorRGBA{R: .25, G: .5, B: 1, A: 1}) {
				t.Fatalf("binary=%t override=%s material=%+v", binary, override, m)
			}
			// Descriptors expose the authored values; project overrides belong
			// to mounted materials rather than changing the imported asset.
			materials, err := bundle.ModelMaterials("surface")
			if err != nil || len(materials) != 1 || materials[0].IOR != 2.42 || materials[0].EmissiveStrength != 64 {
				t.Fatalf("imported descriptors=%+v err=%v", materials, err)
			}
			if err := bundle.Dispose(); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestModelMaterialExplicitZerosSurviveSaveAndBinary(t *testing.T) {
	for _, source := range []string{`{}`, `{"ior":0,"emissiveStrength":0,"metallic":0,"roughness":0}`, `{"ior":2.42,"emissiveStrength":64}`} {
		var spec ModelMaterialSpec
		if err := json.Unmarshal([]byte(source), &spec); err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(spec)
		if err != nil {
			t.Fatal(err)
		}
		var saved ModelMaterialSpec
		if err := json.Unmarshal(encoded, &saved); err != nil {
			t.Fatal(err)
		}
		binary, err := typedbin.Marshal(spec)
		if err != nil {
			t.Fatal(err)
		}
		compiled, err := typedbin.Unmarshal[ModelMaterialSpec](binary)
		if err != nil || saved != spec || compiled != spec {
			t.Fatalf("material override changed after save/preprocess: %s saved=%+v binary=%+v err=%v", source, saved, compiled, err)
		}
	}
}

func TestGLTFFresnelDefaultsAndBoundaryValues(t *testing.T) {
	for _, test := range []struct {
		source        string
		ior, strength float32
	}{
		{`{"extensions":{"KHR_materials_ior":{},"KHR_materials_emissive_strength":{}}}`, 1.5, 1},
		{`{"extensions":{"KHR_materials_ior":{"ior":0},"KHR_materials_emissive_strength":{"emissiveStrength":0}}}`, 0, 0},
		{`{"extensions":{"KHR_materials_ior":{"ior":1},"KHR_materials_emissive_strength":{"emissiveStrength":1e30}}}`, 1, 1e30},
	} {
		var input gltfMaterial
		if err := json.Unmarshal([]byte(test.source), &input); err != nil {
			t.Fatal(err)
		}
		var output compiledModelMaterial
		if err := (&gltfImporter{}).appendMaterialSurface(&output, input); err != nil {
			t.Fatal(err)
		}
		if output.Surface.IOR != test.ior || output.Surface.EmissiveStrength != test.strength {
			t.Fatal(output.Surface)
		}
	}
}
