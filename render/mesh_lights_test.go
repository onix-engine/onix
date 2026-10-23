package render

import "testing"

func TestCameraFogAndToneUniformsPreserveFullLightingABI(t *testing.T) {
	var values [meshLightUniformFloatCount]float32
	lighting := MeshLighting{FogColor: [3]float32{3, 5, 7}, FogParams: [4]float32{20, 120, .02, 2}, ToneMapping: 1, OcclusionParams: [4]float32{.5, .025, 2, 32}}
	var lights [MaxMeshLights]Light3D
	for index := range lights {
		lights[index] = Light3D{Kind: Light3DPoint, Params: [4]float32{10, float32(index)}}
		lighting.LightShadows[index] = [4]float32{float32(index + 1), 1, .1, 0}
		lighting.Shadows[index].Matrix[15] = float32(index + 1)
	}
	writeMeshLightUniforms(values[:], lights[:])
	writeMeshLighting(values[:], lighting)
	for index := range lights {
		if values[meshLightShadowOffset+index*4] != float32(index+1) || values[meshLightShadowOffset+index*4+1] != 1 || values[meshShadowMatrixOffset+index*16+15] != float32(index+1) {
			t.Fatal("camera effects changed a shadow mapping")
		}
	}
	if meshLightUniformByteCount != 3584 || values[288] != 16 || values[290] != .5 || values[291] != .025 || values[303] != 32 || values[298] != 2 {
		t.Fatal("camera effects changed lights, SSAO or ABI size")
	}
	if values[819] != 3 || values[823] != 5 || values[827] != 7 || values[831] != 20 || values[835] != 120 || values[839] != .02 || values[843] != 2 || values[847] != 1 {
		t.Fatal("camera metadata differs from the shader layout")
	}
	if allocations := testing.AllocsPerRun(100, func() { writeMeshLightUniforms(values[:], lights[:]); writeMeshLighting(values[:], lighting) }); allocations != 0 {
		t.Fatal("camera uniform upload allocated", allocations)
	}
	writeMeshLighting(values[:], MeshLighting{})
	for index := range 8 {
		if values[meshLightShadowOffset+index*4+3] != 0 {
			t.Fatal("disabled camera effects retained metadata")
		}
	}
}

func TestHDREnvironmentEncodingPreservesLightsAndShadows(t *testing.T) {
	var values [meshLightUniformFloatCount]float32
	writeMeshLightUniforms(values[:], []Light3D{{Kind: Light3DAmbient}})
	writeMeshLighting(values[:], MeshLighting{EnvironmentEncoding: EnvironmentRGBE, LightShadows: [MaxMeshLights][4]float32{8: {4, .5, .1, 0}}})
	if values[32+MaxMeshLights*16] != 1 || values[meshLightShadowOffset+8*4+3] != 1 || values[meshLightShadowOffset+8*4] != 4 || values[meshLightShadowOffset+8*4+1] != .5 || values[meshLightShadowOffset+8*4+2] != .1 {
		t.Fatal("HDR encoding overwrote the light count or failed to pack")
	}
	writeMeshLighting(values[:], MeshLighting{})
	if values[meshLightShadowOffset+8*4+3] != 0 {
		t.Fatal("SDR lighting retained previous HDR encoding")
	}
}

func TestWriteMeshLightUniforms(t *testing.T) {
	var values [meshLightUniformFloatCount]float32
	lights := []Light3D{{
		Kind:              Light3DSpot,
		PositionDirection: [4]float32{1, 2, 3, 1},
		Direction:         [4]float32{0, 0, -1, 0},
		Color:             [4]float32{0.2, 0.4, 0.6, 1},
		Params:            [4]float32{10, 2, 0.9, 0.7},
	}}
	if got := writeMeshLightUniforms(values[:], lights); got != 1 {
		t.Fatalf("light count = %d, want 1", got)
	}
	if values[32] != 1 || values[32+MaxMeshLights*4+3] != float32(Light3DSpot) {
		t.Fatalf("light record was not packed: position=%v direction=%v", values[32:36], values[32+MaxMeshLights*4:32+MaxMeshLights*4+4])
	}
	if values[32+MaxMeshLights*16] != 1 {
		t.Fatalf("meta count = %v, want 1", values[32+MaxMeshLights*16])
	}
}

func TestWriteMeshLightUniformsCapsWithoutAllocatingRecords(t *testing.T) {
	lights := make([]Light3D, MaxMeshLights+2)
	var values [meshLightUniformFloatCount]float32
	if got := writeMeshLightUniforms(values[:], lights); got != MaxMeshLights {
		t.Fatalf("light count = %d, want %d", got, MaxMeshLights)
	}
	if values[32+MaxMeshLights*16] != MaxMeshLights {
		t.Fatalf("meta count = %v, want %d", values[32+MaxMeshLights*16], MaxMeshLights)
	}
}

func TestPhotometricUniformShapePackingAndReuse(t *testing.T) {
	var lights [MaxMeshLights]Light3D
	last := MaxMeshLights - 1
	lights[last] = Light3D{Kind: Light3DRectangle, Right: [4]float32{1, 0, 0, 2}, Up: [4]float32{0, 1, 0, 3}, Params: [4]float32{10, 100}}
	var values [meshLightUniformFloatCount]float32
	writeMeshLightUniforms(values[:], lights[:])
	writeMeshLighting(values[:], MeshLighting{Params: [4]float32{.01}})
	if values[meshLightRightOffset+last*4+3] != 2 || values[meshLightUpOffset+last*4+3] != 3 || values[meshLightingOffset+4] != .01 {
		t.Fatal("shape arrays overlap other uniforms")
	}
	if meshLightUniformByteCount != 3584 {
		t.Fatalf("native light ABI is 3584 bytes, Go uses %d", meshLightUniformByteCount)
	}
	writeMeshLightUniforms(values[:], lights[:1])
	if values[meshLightRightOffset+last*4+3] != 0 || values[meshLightUpOffset+last*4+3] != 0 {
		t.Fatal("removed light retained geometry")
	}
	if allocations := testing.AllocsPerRun(100, func() { writeMeshLightUniforms(values[:], lights[:]); writeMeshLighting(values[:], MeshLighting{}) }); allocations != 0 {
		t.Fatal(allocations)
	}
}

func TestSSAOLightingLayoutPreservesLightsAndShadows(t *testing.T) {
	var values [meshLightUniformFloatCount]float32
	lighting := MeshLighting{CameraForward: [4]float32{1, 2, 3}, OcclusionParams: [4]float32{.5, .025, 2, 32}}
	for index := range lighting.OcclusionInverseProjection {
		lighting.OcclusionInverseProjection[index] = float32(index + 1)
	}
	lighting.LightShadows[15] = [4]float32{16, 1, .1, 0}
	lighting.Shadows[15].Matrix[15] = 1
	writeMeshLightUniforms(values[:], []Light3D{{Kind: Light3DDirectional}})
	writeMeshLighting(values[:], lighting)
	if got := [4]float32(values[meshLightingOffset+8 : meshLightRightOffset]); got != ([4]float32{1, 2, 3, 32}) {
		t.Fatal("SSAO overwrote the cascade direction", got)
	}
	if values[meshLightingOffset+6] != 2 || values[290] != .5 || values[291] != .025 || values[meshLightShadowOffset+15*4] != 16 || values[meshShadowMatrixOffset+15*16+15] != 1 {
		t.Fatal("SSAO overlaps light/shadow uniforms")
	}
	for index, want := range lighting.OcclusionInverseProjection {
		if values[meshOcclusionInverseOffset+index] != want {
			t.Fatal("inverse projection was truncated")
		}
	}
	writeMeshLightUniforms(values[:], nil)
	writeMeshLighting(values[:], MeshLighting{})
	if values[290] != 0 || values[291] != 0 || values[meshLightingOffset+6] != 0 || values[meshLightingOffset+11] != 0 || values[meshOcclusionInverseOffset] != 0 {
		t.Fatal("disabled SSAO retained uniforms")
	}
}
