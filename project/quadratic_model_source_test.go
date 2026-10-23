//go:build !onix_release

package project

import (
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/binzume/modelconv/fbx"
	"github.com/binzume/modelconv/geom"
	"github.com/go-gl/mathgl/mgl32"
)

func TestSkinParentIndexKeepsJointOrderAndSkinOffsets(t *testing.T) {
	importer := gltfImporter{document: gltfDocument{
		Nodes: []gltfNode{{Children: []int{1, 4}}, {Children: []int{2}}, {Children: []int{3}}, {}, {}},
		Skins: []gltfSkin{{Joints: []int{3, 2, 0}}, {Joints: []int{4, 0, 1}}},
	}}
	if err := importer.appendSkins(); err != nil {
		t.Fatal(err)
	}
	parents := make([]int32, len(importer.result.Joints))
	for index, joint := range importer.result.Joints {
		parents[index] = joint.Parent
		if joint.InverseBind[0] != 1 || joint.InverseBind[5] != 1 || joint.InverseBind[10] != 1 || joint.InverseBind[15] != 1 {
			t.Fatal("implicit inverse bind matrix changed")
		}
	}
	if !slices.Equal(parents, []int32{1, -1, -1, 4, -1, 4}) {
		t.Fatalf("joint parents = %v", parents)
	}
	for _, joints := range [][]int{{0, 0}, {5}, {}} {
		invalid := gltfImporter{document: gltfDocument{Nodes: importer.document.Nodes, Skins: []gltfSkin{{Joints: joints}}}}
		if err := invalid.appendSkins(); err == nil {
			t.Fatalf("invalid joints accepted: %v", joints)
		}
	}
}

func traversalFBXObject(document *fbx.Document, kind, name string, children ...*fbx.Node) *fbx.Obj {
	object := &fbx.Obj{Node: fbx.NewNode(kind, int64(0), name, "")}
	object.Children = children
	document.AddObject(object)
	return object
}

func TestFBXAnimationConnectionIndexPreservesSelection(t *testing.T) {
	document := fbx.NewDocument()
	first, last := fbx.NewModel("first", "Null"), fbx.NewModel("last", "Null")
	document.AddObject(first)
	document.AddObject(last)
	curve := traversalFBXObject(document, "AnimationCurve", "curve",
		fbx.NewNode("KeyTime", []int64{0, 46186158000}),
		fbx.NewNode("KeyValueFloat", []float32{90, 180}))
	unsupported := traversalFBXObject(document, "AnimationCurveNode", "unsupported")
	rotation := traversalFBXObject(document, "AnimationCurveNode", "rotation")
	translation := traversalFBXObject(document, "AnimationCurveNode", "translation")
	ignored := traversalFBXObject(document, "AnimationCurve", "ignored")
	connections := document.RawNode.FindChild("Connections")
	connections.AddChild(fbx.NewNode("C", "OP", curve.ID(), int64(-1), "D|X"))
	connections.AddChild(fbx.NewNode("C", "OO", curve.ID(), rotation.ID(), "D|X"))
	document.AddPropConnection(unsupported, curve, "D|X")
	document.AddPropConnection(rotation, curve, " d|y ")
	document.AddPropConnection(translation, curve, "D|Z")
	document.AddPropConnection(unsupported, ignored, "D|X")
	// Node targets appear after their curve connections, and the last model
	// connection for a node wins, including an unsupported final property.
	document.AddPropConnection(first, unsupported, "Lcl Translation")
	document.AddPropConnection(last, unsupported, "unsupported")
	document.AddPropConnection(first, rotation, "Lcl Scaling")
	document.AddPropConnection(last, rotation, " Lcl Rotation ")
	document.AddPropConnection(first, translation, "Lcl Translation")
	resource := compiledModelResource{}
	if err := appendFBXAnimations(document, &resource); err != nil {
		t.Fatal(err)
	}
	if len(resource.Animations) != 1 || len(resource.Animations[0].Tracks) != 1 {
		t.Fatalf("unexpected animations: %+v", resource.Animations)
	}
	animation := resource.Animations[0]
	track := animation.Tracks[0]
	if animation.Duration != 1 || track.Path != last.Name() || track.Target != "transform.rotation" || track.Component != 1 || track.Components != 1 || track.Interpolation != "LINEAR" {
		t.Fatalf("wrong connection selected: %+v", animation)
	}
	for index, expected := range []float32{math.Pi / 2, math.Pi} {
		keyframe := track.Keyframes[index]
		if keyframe.Time != float32(index) || math.Abs(float64(keyframe.Value[0]-expected)) > 1e-6 {
			t.Fatalf("keyframe %d = %+v", index, keyframe)
		}
	}
}

func TestFBXWorldTransformsUseSharedParentGraph(t *testing.T) {
	root := fbx.NewModel("walk-root", "Null")
	root.SetTranslation(geom.NewVector3(-100, -100, -100))
	ancestor := fbx.NewModel("ancestor", "Null")
	ancestor.SetTranslation(geom.NewVector3(10, 20, 30))
	ancestor.SetScaling(geom.NewVector3(2, 3, 4))
	geometry := fbx.NewGeometry("triangle", []*geom.Vector3{
		geom.NewVector3(0, 0, 0), geom.NewVector3(1, 0, 0), geom.NewVector3(0, 1, 0),
	}, [][]int{{0, 1, 2}})
	geometry.PolygonVertexCount = 3
	first, second := fbx.NewModel("first", "Mesh"), fbx.NewModel("second", "Mesh")
	first.Parent, second.Parent = ancestor, ancestor
	first.SetTranslation(geom.NewVector3(1, 2, 3))
	first.SetProperty("GeometricTranslation", &fbx.Property{AttributeList: fbx.AttributeList{
		{Value: float32(4)}, {Value: float32(5)}, {Value: float32(6)},
	}})
	first.AddRef(geometry)
	second.AddRef(geometry)
	root.AddRef(first)
	root.AddRef(second)
	resource := compiledModelResource{}
	importer := fbxImporter{result: &resource, visited: make(map[*fbx.Model]uint8)}
	if err := importer.appendModel(root, mgl32.Ident4()); err != nil {
		t.Fatal(err)
	}
	expected := [][3]float32{{20, 41, 66}, {22, 41, 66}, {20, 44, 66}, {10, 20, 30}, {12, 20, 30}, {10, 23, 30}}
	if len(resource.Vertices) != len(expected) {
		t.Fatalf("vertex count = %d", len(resource.Vertices))
	}
	for index, position := range expected {
		if resource.Vertices[index].Position != position {
			t.Fatalf("vertex %d position = %v, want %v", index, resource.Vertices[index].Position, position)
		}
	}
}

func TestFBXWorldTransformsRejectParentAndReferenceCycles(t *testing.T) {
	for _, kind := range []string{"parent", "reference", "duplicate"} {
		t.Run(kind, func(t *testing.T) {
			root, child := fbx.NewModel("root", "Null"), fbx.NewModel("child", "Null")
			root.AddRef(child)
			child.Parent = root
			switch kind {
			case "parent":
				root.Parent = child
			case "reference":
				child.AddRef(root)
			case "duplicate":
				root.AddRef(child)
			}
			resource := compiledModelResource{}
			importer := fbxImporter{result: &resource, visited: make(map[*fbx.Model]uint8)}
			if err := importer.appendModel(root, mgl32.Ident4()); err == nil {
				t.Fatal("invalid graph accepted")
			}
		})
	}
}

func BenchmarkFBXHierarchyTraversal(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			models := make([]*fbx.Model, count)
			for index := range models {
				models[index] = fbx.NewModel(fmt.Sprintf("node%d", index), "Null")
				models[index].SetTranslation(geom.NewVector3(1, 0, 0))
				models[index].GetMatrix()
				if index > 0 {
					models[index].Parent = models[index-1]
					models[index-1].AddRef(models[index])
				}
			}
			b.ReportAllocs()
			for b.Loop() {
				resource := compiledModelResource{}
				importer := fbxImporter{result: &resource, visited: make(map[*fbx.Model]uint8)}
				if err := importer.appendModel(models[0], mgl32.Ident4()); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestFBXAnimationConnectionIndexValidatesKeys(t *testing.T) {
	for _, count := range []int{0, 1} {
		document := fbx.NewDocument()
		model := fbx.NewModel("model", "Null")
		document.AddObject(model)
		curve := traversalFBXObject(document, "AnimationCurve", "invalid",
			fbx.NewNode("KeyTime", []int64{0, 46186158000}),
			fbx.NewNode("KeyValueFloat", make([]float32, count)))
		node := traversalFBXObject(document, "AnimationCurveNode", "node")
		document.AddPropConnection(model, node, "Lcl Translation")
		document.AddPropConnection(node, curve, "D|X")
		resource := compiledModelResource{}
		if err := appendFBXAnimations(document, &resource); err == nil || len(resource.Animations) != 0 {
			t.Fatalf("mismatched keys accepted: count=%d, err=%v", count, err)
		}
	}
}

func BenchmarkFBXAnimationTraversal(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			document := fbx.NewDocument()
			model := fbx.NewModel("model", "Null")
			document.AddObject(model)
			for index := range count {
				curve := traversalFBXObject(document, "AnimationCurve", fmt.Sprintf("curve%d", index),
					fbx.NewNode("KeyTime", []int64{0, 46186158000}),
					fbx.NewNode("KeyValueFloat", []float32{0, 1}))
				node := traversalFBXObject(document, "AnimationCurveNode", fmt.Sprintf("node%d", index))
				document.AddPropConnection(node, curve, "D|X")
				document.AddPropConnection(model, node, "Lcl Translation")
			}
			resource := compiledModelResource{}
			if err := appendFBXAnimations(document, &resource); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for b.Loop() {
				resource.Animations = resource.Animations[:0]
				if err := appendFBXAnimations(document, &resource); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkSkinParentTraversal(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			importer := gltfImporter{document: gltfDocument{Nodes: make([]gltfNode, count), Skins: []gltfSkin{{Joints: make([]int, count)}}}}
			for index := range count {
				importer.document.Nodes[index].Name = fmt.Sprintf("joint%d", index)
				if index+1 < count {
					importer.document.Nodes[index].Children = []int{index + 1}
				}
				importer.document.Skins[0].Joints[index] = index
			}
			if err := importer.appendSkins(); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for b.Loop() {
				importer.result.Joints = importer.result.Joints[:0]
				if err := importer.appendSkins(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
