//go:build !onix_release

package project

import (
	"testing"

	"github.com/diiyw/onix/internal/typedbin"
	"github.com/go-gl/mathgl/mgl32"
)

func TestGLTFTangentGenerationMirrorAndBinaryRoundTrip(t *testing.T) {
	var document gltfDocument
	var data []byte
	positions := addTestGLTFFloatAccessor(&document, &data, "VEC3", 3, 0, 0, 0, 1, 0, 0, 0, 1, 0)
	normals := addTestGLTFFloatAccessor(&document, &data, "VEC3", 3, 0, 0, 1, 0, 0, 1, 0, 0, 1)
	uvs := addTestGLTFFloatAccessor(&document, &data, "VEC2", 3, 0, 0, 1, 0, 0, 1)
	tangents := addTestGLTFFloatAccessor(&document, &data, "VEC4", 3, 1, 0, 0, 1, 1, 0, 0, 1, 1, 0, 0, 1)
	morph := addTestGLTFFloatAccessor(&document, &data, "VEC3", 3, 1, 0, 0, 1, 0, 0, 1, 0, 0)
	for _, authored := range []bool{false, true} {
		importer := gltfImporter{document: document, buffers: [][]byte{data}}
		attributes := map[string]int{"POSITION": positions, "NORMAL": normals, "TEXCOORD_0": uvs}
		if authored {
			attributes["TANGENT"] = tangents
		}
		if err := importer.appendPrimitive(gltfPrimitive{Attributes: attributes, Targets: []map[string]int{{"TANGENT": morph}}}, mgl32.Scale3D(-2, 3, 4), -1); err != nil {
			t.Fatal(err)
		}
		encoded, err := typedbin.Marshal(importer.result)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := typedbin.Unmarshal[compiledModelResource](encoded)
		if err != nil {
			t.Fatal(err)
		}
		if err := decoded.validate(); err != nil {
			t.Fatal(err)
		}
		for _, vertex := range decoded.Vertices {
			if vertex.Tangent != [4]float32{-1, 0, 0, -1} {
				t.Fatalf("authored=%t, tangent=%v", authored, vertex.Tangent)
			}
		}
		if decoded.MorphTargets[0].TangentDelta[0] != [3]float32{-2, 0, 0} {
			t.Fatal(decoded.MorphTargets)
		}
	}
}
