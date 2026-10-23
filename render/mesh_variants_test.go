package render

import "testing"

func TestMeshBuiltinVariants(t *testing.T) {
	base := Instance3D{Material: [4]float32{.5, 1.5, .5, 0}}
	unlit := base
	unlit.Material[3] = 256
	coat := base
	coat.Custom[0][3] = .2
	sheen := base
	sheen.Custom[2][1] = .1
	anisotropic := base
	anisotropic.Custom[3][0] = .3
	tests := []struct {
		name      string
		pass      int
		instances []Instance3D
		want      int
	}{
		{"basic", MeshPassLit, []Instance3D{base}, meshVariantBasic},
		{"unlit", MeshPassLit, []Instance3D{unlit}, meshVariantUnlit},
		{"mixed", MeshPassLit, []Instance3D{base, unlit}, meshVariantFull},
		{"coat", MeshPassLit, []Instance3D{base, coat}, meshVariantFull},
		{"sheen", MeshPassLit, []Instance3D{sheen, base}, meshVariantFull},
		{"anisotropy", MeshPassLit, []Instance3D{anisotropic}, meshVariantFull},
		{"depth", MeshPassOcclusionDepth, []Instance3D{base, coat}, meshVariantDepth},
		{"unlit depth", MeshPassShadowDepth, []Instance3D{unlit}, meshVariantDepth},
		{"SSR", MeshPassReflectionResolve, []Instance3D{base}, meshVariantFull},
		{"radiance", MeshPassReflectionCapture, []Instance3D{base}, meshVariantBasic},
		{"AO", MeshPassReflectionOcclusion, []Instance3D{base}, meshVariantBasic},
		{"FXAA", MeshPassFXAA, []Instance3D{base}, meshVariantUnlit},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			batch := MeshBatch3D{Instances: append([]Instance3D{coat}, test.instances...)}
			batch.Lighting.Params[1] = float32(test.pass)
			draw := MeshDraw3D{FirstInstance: 1, InstanceCount: uint32(len(test.instances))}
			if got := meshBuiltinVariant(&batch, &draw); got != test.want {
				t.Fatalf("variant = %d, want %d", got, test.want)
			}
			if allocations := testing.AllocsPerRun(100, func() { meshBuiltinVariant(&batch, &draw) }); allocations != 0 {
				t.Fatal("variant selection allocated", allocations)
			}
		})
	}
}
