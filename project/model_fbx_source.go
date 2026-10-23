//go:build !onix_release

package project

import (
	"bytes"
	"fmt"
	"math"
	"strings"

	"github.com/binzume/modelconv/fbx"
	"github.com/binzume/modelconv/geom"
	"github.com/diiyw/onix/render"
	"github.com/go-gl/mathgl/mgl32"
)

type fbxLayer[T any] struct {
	values    []T
	indices   []int32
	mapping   string
	reference string
	present   bool
}

func importFBX(data []byte) (resource compiledModelResource, err error) {
	defer func() {
		if value := recover(); value != nil {
			resource = compiledModelResource{}
			err = fmt.Errorf("decode FBX: %v", value)
		}
	}()
	document, err := fbx.Parse(bytes.NewReader(data))
	if err != nil {
		return compiledModelResource{}, fmt.Errorf("decode FBX: %w", err)
	}
	if document == nil || document.Scene == nil || document.GlobalSettings == nil {
		return compiledModelResource{}, fmt.Errorf("FBX scene is incomplete")
	}
	unitScale := document.GlobalSettings.GetProperty("UnitScaleFactor").ToFloat32(1) * 0.01
	if !finiteFloat32(unitScale) || unitScale <= 0 {
		return compiledModelResource{}, fmt.Errorf("FBX UnitScaleFactor is invalid")
	}
	coordinate := mgl32.Mat4(*document.CoordMatrix())
	rootTransform := mgl32.Scale3D(unitScale, unitScale, unitScale).Mul4(coordinate)
	importer := fbxImporter{result: &resource, visited: make(map[*fbx.Model]uint8), jointIndices: make(map[*fbx.Model]int32), jointVisiting: make(map[*fbx.Model]bool)}
	for _, model := range document.Scene.GetChildModels() {
		if err := importer.appendModel(model, rootTransform); err != nil {
			return compiledModelResource{}, err
		}
	}
	if err := resource.validate(); err != nil {
		return compiledModelResource{}, fmt.Errorf("FBX geometry: %w", err)
	}
	if err := appendFBXAnimations(document, &resource); err != nil {
		return compiledModelResource{}, err
	}
	return resource, nil
}

type fbxAnimationCurveTarget struct {
	axis string
	fbxAnimationNodeTarget
}

type fbxAnimationNodeTarget struct {
	target string
	model  *fbx.Model
}

func appendFBXAnimations(document *fbx.Document, resource *compiledModelResource) error {
	if document == nil || resource == nil {
		return nil
	}
	curves := make(map[int64]fbxAnimationCurveTarget)
	nodes := make(map[int64]fbxAnimationNodeTarget)
	animation := compiledModelAnimation{Name: "fbx-animation"}
	connections := document.RawNode.FindChild("Connections")
	for _, connection := range connections.GetChildren() {
		if connection.Name != "C" || connection.Attr(0).ToString() != "OP" {
			continue
		}
		fromID, toID := connection.Attr(1).ToInt64(0), connection.Attr(2).ToInt64(0)
		from, to := document.ObjectByID[fromID], document.ObjectByID[toID]
		if from == nil || to == nil {
			continue
		}
		if from.NodeName() == "AnimationCurveNode" {
			if model, ok := to.(*fbx.Model); ok {
				var target string
				switch strings.TrimSpace(connection.Attr(3).ToString()) {
				case "Lcl Translation":
					target = "transform.translation"
				case "Lcl Rotation":
					target = "transform.rotation"
				case "Lcl Scaling":
					target = "transform.scale"
				}
				nodes[fromID] = fbxAnimationNodeTarget{target: target, model: model}
			}
		}
	}
	// Resolve after all node targets are known, preserving the first supported
	// curve connection even when its node-to-model connection appears later.
	for _, connection := range connections.GetChildren() {
		if connection.Name != "C" || connection.Attr(0).ToString() != "OP" {
			continue
		}
		fromID, toID := connection.Attr(1).ToInt64(0), connection.Attr(2).ToInt64(0)
		if _, exists := curves[fromID]; exists {
			continue
		}
		from := document.ObjectByID[fromID]
		nodeTarget, exists := nodes[toID]
		if from == nil || from.NodeName() != "AnimationCurve" || !exists || nodeTarget.target == "" {
			continue
		}
		curves[fromID] = fbxAnimationCurveTarget{axis: connection.Attr(3).ToString(), fbxAnimationNodeTarget: nodeTarget}
	}
	for _, curveObject := range document.ObjectByID {
		if curveObject.NodeName() != "AnimationCurve" {
			continue
		}
		curveTarget, ok := curves[curveObject.ID()]
		if !ok {
			continue
		}
		times := fbxAnimationTimes(curveObject.GetNode().FindChild("KeyTime"))
		values := curveObject.GetNode().FindChild("KeyValueFloat").GetFloat32Array()
		if len(times) == 0 || len(times) != len(values) {
			return fmt.Errorf("FBX animation curve %q has mismatched key times and values", curveObject.Name())
		}
		component := uint8(0)
		switch strings.ToUpper(strings.TrimSpace(curveTarget.axis)) {
		case "D|Y":
			component = 1
		case "D|Z":
			component = 2
		}
		track := compiledModelAnimationTrack{Path: curveTarget.model.Name(), Target: curveTarget.target, Components: 1, Component: component, Interpolation: "LINEAR", Keyframes: make([]compiledModelAnimationKeyframe, len(times))}
		for index, tick := range times {
			value := values[index]
			if curveTarget.target == "transform.rotation" {
				value *= float32(math.Pi / 180)
			}
			track.Keyframes[index] = compiledModelAnimationKeyframe{Time: float32(float64(tick) / 46186158000), Value: [4]float32{value}}
		}
		animation.Duration = max(animation.Duration, track.Keyframes[len(track.Keyframes)-1].Time)
		animation.Tracks = append(animation.Tracks, track)
	}
	if len(animation.Tracks) != 0 {
		resource.Animations = append(resource.Animations, animation)
	}
	return nil
}

func fbxAnimationTimes(node *fbx.Node) []int64 {
	if node == nil || node.Attr(0) == nil {
		return nil
	}
	switch values := node.Attr(0).Value.(type) {
	case []int64:
		return values
	case []int32:
		result := make([]int64, len(values))
		for index, value := range values {
			result[index] = int64(value)
		}
		return result
	default:
		return nil
	}
}

type fbxImporter struct {
	result        *compiledModelResource
	visited       map[*fbx.Model]uint8
	worlds        map[*fbx.Model]fbxWorldTransform
	jointIndices  map[*fbx.Model]int32
	jointVisiting map[*fbx.Model]bool
}

type fbxWorldTransform struct {
	matrix mgl32.Mat4
	state  uint8
}

func (importer *fbxImporter) appendModel(model *fbx.Model, coordinate mgl32.Mat4) error {
	if model == nil {
		return fmt.Errorf("FBX scene contains a nil model")
	}
	switch importer.visited[model] {
	case 1:
		return fmt.Errorf("FBX model graph contains a cycle at %q", model.Name())
	case 2:
		return fmt.Errorf("FBX model %q is referenced more than once", model.Name())
	}
	importer.visited[model] = 1
	world, err := importer.worldMatrix(model)
	if err != nil {
		return err
	}
	transform := coordinate.Mul4(world).Mul4(fbxGeometricMatrix(model))
	if geometry := model.GetGeometry(); geometry != nil {
		if err := importer.appendGeometry(model.Name(), geometry, transform); err != nil {
			return err
		}
	}
	for _, child := range model.GetChildModels() {
		if err := importer.appendModel(child, coordinate); err != nil {
			return err
		}
	}
	importer.visited[model] = 2
	return nil
}

func (importer *fbxImporter) worldMatrix(model *fbx.Model) (mgl32.Mat4, error) {
	if model == nil {
		return mgl32.Ident4(), nil
	}
	entry := importer.worlds[model]
	switch entry.state {
	case 1:
		return mgl32.Mat4{}, fmt.Errorf("FBX model parent graph contains a cycle at %q", model.Name())
	case 2:
		return entry.matrix, nil
	}
	if importer.worlds == nil {
		importer.worlds = make(map[*fbx.Model]fbxWorldTransform)
	}
	importer.worlds[model] = fbxWorldTransform{state: 1}
	parent, err := importer.worldMatrix(model.Parent)
	if err != nil {
		return mgl32.Mat4{}, err
	}
	world := parent.Mul4(mgl32.Mat4(*model.GetMatrix()))
	importer.worlds[model] = fbxWorldTransform{matrix: world, state: 2}
	return world, nil
}

func fbxGeometricMatrix(model *fbx.Model) mgl32.Mat4 {
	translation := model.GetProperty("GeometricTranslation").ToVector3(0, 0, 0)
	rotation := model.GetProperty("GeometricRotation").ToVector3(0, 0, 0)
	scale := model.GetProperty("GeometricScaling").ToVector3(1, 1, 1)
	const radians = float32(math.Pi / 180)
	rx := mgl32.HomogRotate3DX(rotation.X * radians)
	ry := mgl32.HomogRotate3DY(rotation.Y * radians)
	rz := mgl32.HomogRotate3DZ(rotation.Z * radians)
	return mgl32.Translate3D(translation.X, translation.Y, translation.Z).
		Mul4(rz).Mul4(ry).Mul4(rx).
		Mul4(mgl32.Scale3D(scale.X, scale.Y, scale.Z))
}

func (importer *fbxImporter) appendGeometry(name string, geometry *fbx.Geometry, transform mgl32.Mat4) error {
	if len(geometry.Vertices) == 0 {
		return fmt.Errorf("FBX model %q geometry has no vertices", name)
	}
	if len(geometry.Polygons) == 0 {
		return fmt.Errorf("FBX model %q geometry has no polygons", name)
	}
	normals, err := readFBXLayer(geometry, "LayerElementNormal", "Normals", "NormalsIndex", (*fbx.Node).GetVec3Array)
	if err != nil {
		return fmt.Errorf("FBX model %q normals: %w", name, err)
	}
	uvs, err := readFBXLayer(geometry, "LayerElementUV", "UV", "UVIndex", (*fbx.Node).GetVec2Array)
	if err != nil {
		return fmt.Errorf("FBX model %q UVs: %w", name, err)
	}
	cornerCount, triangleCount := 0, 0
	for polygonIndex, polygon := range geometry.Polygons {
		if len(polygon) < 3 {
			return fmt.Errorf("FBX model %q polygon %d has fewer than three vertices", name, polygonIndex)
		}
		if !fbxPolygonConvex(geometry.Vertices, polygon) {
			return fmt.Errorf("FBX model %q polygon %d is degenerate or non-convex; triangulate it before import", name, polygonIndex)
		}
		cornerCount += len(polygon)
		triangleCount += len(polygon) - 2
	}
	if geometry.PolygonVertexCount != cornerCount {
		return fmt.Errorf("FBX model %q polygon vertex stream is unterminated", name)
	}
	skin, hasSkin, err := importer.readSkin(geometry)
	if err != nil {
		return fmt.Errorf("FBX model %q skin: %w", name, err)
	}
	if uint64(len(importer.result.Vertices))+uint64(cornerCount) > uint64(^uint32(0))+1 {
		return fmt.Errorf("FBX model %q exceeds uint32 index capacity", name)
	}
	base := len(importer.result.Vertices)
	indexStart := len(importer.result.Indices)
	importer.result.Vertices = append(importer.result.Vertices, make([]render.Vertex3D, cornerCount)...)
	importer.result.Indices = append(importer.result.Indices, make([]uint32, triangleCount*3)...)
	if len(importer.result.Skin) != 0 || hasSkin {
		if len(importer.result.Skin) == 0 {
			importer.result.Skin = make([]compiledModelSkinVertex, base)
		}
		importer.result.Skin = append(importer.result.Skin, make([]compiledModelSkinVertex, cornerCount)...)
	}
	corner, indexOffset := 0, indexStart
	for polygonIndex, polygon := range geometry.Polygons {
		polygonBase := uint32(base + corner)
		for _, controlPoint := range polygon {
			if controlPoint < 0 || controlPoint >= len(geometry.Vertices) || geometry.Vertices[controlPoint] == nil {
				return fmt.Errorf("FBX model %q polygon %d control point %d is out of range", name, polygonIndex, controlPoint)
			}
			position := geometry.Vertices[controlPoint]
			vertex := &importer.result.Vertices[base+corner]
			vertex.Position = [3]float32{position.X, position.Y, position.Z}
			if hasSkin {
				importer.result.Skin[base+corner] = skin[controlPoint]
			}
			if normals.present {
				normal, valueErr := normals.value(controlPoint, corner, polygonIndex)
				if valueErr != nil {
					return fmt.Errorf("FBX model %q polygon %d normal: %w", name, polygonIndex, valueErr)
				}
				if normal == nil {
					return fmt.Errorf("FBX model %q polygon %d normal is nil", name, polygonIndex)
				}
				vertex.Normal = [3]float32{normal.X, normal.Y, normal.Z}
			}
			if uvs.present {
				uv, valueErr := uvs.value(controlPoint, corner, polygonIndex)
				if valueErr != nil {
					return fmt.Errorf("FBX model %q polygon %d UV: %w", name, polygonIndex, valueErr)
				}
				if uv == nil {
					return fmt.Errorf("FBX model %q polygon %d UV is nil", name, polygonIndex)
				}
				vertex.UV = [2]float32{uv.X, 1 - uv.Y}
			}
			corner++
		}
		for triangle := 1; triangle < len(polygon)-1; triangle++ {
			importer.result.Indices[indexOffset] = polygonBase
			importer.result.Indices[indexOffset+1] = polygonBase + uint32(triangle)
			importer.result.Indices[indexOffset+2] = polygonBase + uint32(triangle+1)
			indexOffset += 3
		}
	}
	if err := transformPrimitive(importer.result.Vertices[base:], importer.result.Indices[indexStart:], uint32(base), transform, normals.present); err != nil {
		return fmt.Errorf("FBX model %q: %w", name, err)
	}
	return nil
}

func (importer *fbxImporter) ensureJoint(model *fbx.Model, bind *geom.Matrix4) (int32, error) {
	if model == nil {
		return -1, fmt.Errorf("skin joint has no model")
	}
	if index, ok := importer.jointIndices[model]; ok {
		return index, nil
	}
	if importer.jointVisiting[model] {
		return -1, fmt.Errorf("FBX joint hierarchy contains a cycle at %q", model.Name())
	}
	importer.jointVisiting[model] = true
	parent := int32(-1)
	var err error
	if model.Parent != nil {
		parent, err = importer.ensureJoint(model.Parent, nil)
		if err != nil {
			return -1, err
		}
	}
	index := int32(len(importer.result.Joints))
	importer.jointIndices[model] = index
	value := compiledModelJoint{Name: model.Name(), Parent: parent}
	if bind != nil {
		value.InverseBind = [16]float32(*bind.Inverse())
	} else {
		value.InverseBind[0], value.InverseBind[5], value.InverseBind[10], value.InverseBind[15] = 1, 1, 1, 1
	}
	importer.result.Joints = append(importer.result.Joints, value)
	delete(importer.jointVisiting, model)
	return index, nil
}

func (importer *fbxImporter) readSkin(geometry *fbx.Geometry) ([]compiledModelSkinVertex, bool, error) {
	deformers := geometry.GetDeformers()
	if len(deformers) == 0 {
		return nil, false, nil
	}
	result := make([]compiledModelSkinVertex, len(geometry.Vertices))
	hasSkin := false
	for _, deformer := range deformers {
		if deformer == nil {
			continue
		}
		model := deformer.GetTarget()
		if model == nil {
			continue
		}
		joint, err := importer.ensureJoint(model, deformer.GetTransformLink())
		if err != nil {
			return nil, false, err
		}
		indexes, weights := deformer.GetIndexes(), deformer.GetWeights()
		if len(indexes) != len(weights) {
			return nil, false, fmt.Errorf("joint %q has %d indexes and %d weights", model.Name(), len(indexes), len(weights))
		}
		for influence, controlPoint := range indexes {
			if controlPoint < 0 || int(controlPoint) >= len(result) {
				return nil, false, fmt.Errorf("joint %q control point %d is out of range", model.Name(), controlPoint)
			}
			weight := weights[influence]
			if !finiteFloat32(weight) || weight < 0 {
				return nil, false, fmt.Errorf("joint %q has invalid weight %g", model.Name(), weight)
			}
			value := &result[controlPoint]
			value.Active = true
			replace := -1
			minimum := float32(math.Inf(1))
			for slot, current := range value.Weights {
				if current < minimum {
					minimum, replace = current, slot
				}
			}
			if weight > minimum {
				value.Joints[replace], value.Weights[replace] = uint32(joint), weight
			}
			hasSkin = true
		}
	}
	for index := range result {
		if !result[index].Active {
			continue
		}
		total := result[index].Weights[0] + result[index].Weights[1] + result[index].Weights[2] + result[index].Weights[3]
		if total <= 1e-8 {
			result[index].Active = false
			continue
		}
		for influence := range result[index].Weights {
			result[index].Weights[influence] /= total
		}
	}
	return result, hasSkin, nil
}

func readFBXLayer[T any](geometry *fbx.Geometry, layerName, valueName, indexName string, decode func(*fbx.Node) []T) (fbxLayer[T], error) {
	node := geometry.FindChild(layerName)
	if node == nil {
		return fbxLayer[T]{}, nil
	}
	layer := fbxLayer[T]{
		values:    decode(node.FindChild(valueName)),
		indices:   node.FindChild(indexName).GetInt32Array(),
		mapping:   node.FindChild("MappingInformationType").GetString(),
		reference: node.FindChild("ReferenceInformationType").GetString(),
		present:   true,
	}
	if len(layer.values) == 0 {
		return fbxLayer[T]{}, fmt.Errorf("%s contains no values", layerName)
	}
	switch layer.mapping {
	case "ByControlPoint", "ByPolygonVertex", "ByPolygon", "AllSame":
	default:
		return fbxLayer[T]{}, fmt.Errorf("unsupported mapping %q", layer.mapping)
	}
	switch layer.reference {
	case "Direct":
	case "IndexToDirect":
		if len(layer.indices) == 0 {
			return fbxLayer[T]{}, fmt.Errorf("IndexToDirect layer has no indices")
		}
	default:
		return fbxLayer[T]{}, fmt.Errorf("unsupported reference %q", layer.reference)
	}
	return layer, nil
}

func (layer fbxLayer[T]) value(controlPoint, polygonVertex, polygon int) (T, error) {
	var zero T
	index := 0
	switch layer.mapping {
	case "ByControlPoint":
		index = controlPoint
	case "ByPolygonVertex":
		index = polygonVertex
	case "ByPolygon":
		index = polygon
	case "AllSame":
	default:
		return zero, fmt.Errorf("unsupported mapping %q", layer.mapping)
	}
	if layer.reference == "IndexToDirect" {
		if index < 0 || index >= len(layer.indices) {
			return zero, fmt.Errorf("layer index %d exceeds %d indices", index, len(layer.indices))
		}
		index = int(layer.indices[index])
	}
	if index < 0 || index >= len(layer.values) {
		return zero, fmt.Errorf("layer value %d exceeds %d values", index, len(layer.values))
	}
	return layer.values[index], nil
}

func fbxPolygonConvex(vertices []*geom.Vector3, polygon []int) bool {
	if len(polygon) == 3 {
		return fbxTriangleArea(vertices, polygon[0], polygon[1], polygon[2]) > 1e-12
	}
	normal := mgl32.Vec3{}
	for index, currentIndex := range polygon {
		nextIndex := polygon[(index+1)%len(polygon)]
		if currentIndex < 0 || currentIndex >= len(vertices) || nextIndex < 0 || nextIndex >= len(vertices) || vertices[currentIndex] == nil || vertices[nextIndex] == nil {
			return false
		}
		current, next := vertices[currentIndex], vertices[nextIndex]
		normal[0] += (current.Y - next.Y) * (current.Z + next.Z)
		normal[1] += (current.Z - next.Z) * (current.X + next.X)
		normal[2] += (current.X - next.X) * (current.Y + next.Y)
	}
	axis := 0
	if math.Abs(float64(normal[1])) > math.Abs(float64(normal[axis])) {
		axis = 1
	}
	if math.Abs(float64(normal[2])) > math.Abs(float64(normal[axis])) {
		axis = 2
	}
	if math.Abs(float64(normal[axis])) < 1e-8 {
		return false
	}
	sign := float32(0)
	for index := range polygon {
		a := fbxProjectedPoint(vertices[polygon[index]], axis)
		b := fbxProjectedPoint(vertices[polygon[(index+1)%len(polygon)]], axis)
		c := fbxProjectedPoint(vertices[polygon[(index+2)%len(polygon)]], axis)
		cross := (b[0]-a[0])*(c[1]-b[1]) - (b[1]-a[1])*(c[0]-b[0])
		if math.Abs(float64(cross)) < 1e-8 {
			continue
		}
		if sign == 0 {
			sign = cross
		} else if sign*cross < 0 {
			return false
		}
	}
	return sign != 0
}

func fbxTriangleArea(vertices []*geom.Vector3, a, b, c int) float32 {
	if a < 0 || a >= len(vertices) || b < 0 || b >= len(vertices) || c < 0 || c >= len(vertices) || vertices[a] == nil || vertices[b] == nil || vertices[c] == nil {
		return 0
	}
	left := mgl32.Vec3{vertices[b].X - vertices[a].X, vertices[b].Y - vertices[a].Y, vertices[b].Z - vertices[a].Z}
	right := mgl32.Vec3{vertices[c].X - vertices[a].X, vertices[c].Y - vertices[a].Y, vertices[c].Z - vertices[a].Z}
	return left.Cross(right).Len()
}

func fbxProjectedPoint(value *geom.Vector3, droppedAxis int) mgl32.Vec2 {
	switch droppedAxis {
	case 0:
		return mgl32.Vec2{value.Y, value.Z}
	case 1:
		return mgl32.Vec2{value.X, value.Z}
	default:
		return mgl32.Vec2{value.X, value.Y}
	}
}
