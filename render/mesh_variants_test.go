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
		instances []Instance3D
		pass      int
		want      int
	}{
		{name: "basic", pass: MeshPassLit, instances: []Instance3D{base}, want: meshVariantBasic},
		{name: "unlit", pass: MeshPassLit, instances: []Instance3D{unlit}, want: meshVariantUnlit},
		{name: "mixed", pass: MeshPassLit, instances: []Instance3D{base, unlit}, want: meshVariantFull},
		{name: "coat", pass: MeshPassLit, instances: []Instance3D{base, coat}, want: meshVariantFull},
		{name: "sheen", pass: MeshPassLit, instances: []Instance3D{sheen, base}, want: meshVariantFull},
		{name: "anisotropy", pass: MeshPassLit, instances: []Instance3D{anisotropic}, want: meshVariantFull},
		{name: "depth", pass: MeshPassOcclusionDepth, instances: []Instance3D{base, coat}, want: meshVariantDepth},
		{name: "unlit depth", pass: MeshPassShadowDepth, instances: []Instance3D{unlit}, want: meshVariantDepth},
		{name: "SSR", pass: MeshPassReflectionResolve, instances: []Instance3D{base}, want: meshVariantFull},
		{name: "radiance", pass: MeshPassReflectionCapture, instances: []Instance3D{base}, want: meshVariantBasic},
		{name: "AO", pass: MeshPassReflectionOcclusion, instances: []Instance3D{base}, want: meshVariantBasic},
		{name: "FXAA", pass: MeshPassFXAA, instances: []Instance3D{base}, want: meshVariantUnlit},
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
