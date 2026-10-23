package physics

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/diiyw/onix/internal/reuse"
)

// CollisionMode selects the shape produced by model import baking.
type CollisionMode string

const (
	CollisionModeTriangleMesh        CollisionMode = "trimesh"
	CollisionModeSingleConvex        CollisionMode = "singleConvex"
	CollisionModeConvexDecomposition CollisionMode = "convexDecomposition"
)

func (mode CollisionMode) String() string { return string(mode) }

func ParseCollisionMode(value string) (CollisionMode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "trimesh", "trianglemesh", "triangle_mesh":
		return CollisionModeTriangleMesh, nil
	case "singleconvex", "single_convex", "convex":
		return CollisionModeSingleConvex, nil
	case "convexdecomposition", "convex_decomposition", "compound":
		return CollisionModeConvexDecomposition, nil
	default:
		return "", fmt.Errorf("physics: unsupported collision mode %q", value)
	}
}

// CookedCollision keeps all mode-specific outputs in one immutable resource.
// Only the selected field is populated by CookCollision.
type CookedCollision struct {
	Mode         CollisionMode
	TriangleMesh TriangleMesh
	ConvexHull   ConvexHull
	Compound     Compound
}

// CookCollision performs the importer-side cooking step. Convex decomposition
// accepts a single convex fallback here; asset importers that use CoACD should
// pass all generated pieces to ConvexDecomposition and store the resulting
// Compound directly.
func CookCollision(vertices []Vector3D, indices []uint32, mode CollisionMode) (CookedCollision, error) {
	parsed, err := ParseCollisionMode(string(mode))
	if err != nil {
		return CookedCollision{}, err
	}
	result := CookedCollision{Mode: parsed}
	switch parsed {
	case CollisionModeTriangleMesh:
		result.TriangleMesh, err = NewTriangleMesh(vertices, indices)
	case CollisionModeSingleConvex:
		result.ConvexHull, err = NewConvexHull(vertices)
	case CollisionModeConvexDecomposition:
		var hull ConvexHull
		hull, err = NewConvexHull(vertices)
		if err == nil {
			result.Compound, err = ConvexDecomposition([]ConvexHull{hull})
		}
	}
	return result, err
}

// Bounds3D is an axis-aligned box used by cooked collision resources.
// Bounds are expressed in the resource's local space.
type Bounds3D struct {
	Min Vector3D
	Max Vector3D
}

func (bounds Bounds3D) Valid() bool {
	return bounds.Min.X <= bounds.Max.X && bounds.Min.Y <= bounds.Max.Y && bounds.Min.Z <= bounds.Max.Z
}

func (bounds Bounds3D) Center() Vector3D { return bounds.Min.Add(bounds.Max).Mul(.5) }
func (bounds Bounds3D) Size() Vector3D   { return bounds.Max.Sub(bounds.Min) }

func (bounds Bounds3D) expand(point Vector3D) Bounds3D {
	if !bounds.Valid() {
		return Bounds3D{Min: point, Max: point}
	}
	return Bounds3D{
		Min: V3(min(bounds.Min.X, point.X), min(bounds.Min.Y, point.Y), min(bounds.Min.Z, point.Z)),
		Max: V3(max(bounds.Max.X, point.X), max(bounds.Max.Y, point.Y), max(bounds.Max.Z, point.Z)),
	}
}

func emptyBounds3D() Bounds3D {
	return Bounds3D{Min: V3(math.Inf(1), math.Inf(1), math.Inf(1)), Max: V3(math.Inf(-1), math.Inf(-1), math.Inf(-1))}
}

// TriangleMeshNode is a node in a cooked triangle mesh BVH. A node with Count
// greater than zero is a leaf and indexes TriangleIndices[First:First+Count].
type TriangleMeshNode struct {
	Bounds       Bounds3D
	Left, Right  int32
	First, Count uint32
}

// TriangleMesh is an immutable, cooked static collision resource. The slices
// are owned by the value returned from NewTriangleMesh and must not be edited
// after construction.
type TriangleMesh struct {
	Vertices        []Vector3D
	Indices         []uint32
	TriangleIndices []uint32
	BVH             []TriangleMeshNode
	Bounds          Bounds3D
}

func (mesh TriangleMesh) physicsShape()   {}
func (mesh TriangleMesh) physicsShape3D() {}

func (mesh TriangleMesh) TriangleCount() int { return len(mesh.Indices) / 3 }
func (mesh TriangleMesh) Cooked() bool       { return len(mesh.BVH) != 0 && len(mesh.TriangleIndices) != 0 }

// NewTriangleMesh validates and cooks a triangle mesh once. Invalid index
// references are rejected; repeated vertices and zero-area triangles are
// removed from the cooked copy.
func NewTriangleMesh(vertices []Vector3D, indices []uint32) (TriangleMesh, error) {
	if len(vertices) == 0 {
		return TriangleMesh{}, fmt.Errorf("physics: triangle mesh has no vertices")
	}
	if len(indices) == 0 || len(indices)%3 != 0 {
		return TriangleMesh{}, fmt.Errorf("physics: triangle mesh index count must be a non-zero multiple of three")
	}
	scratch := triangleMeshCookPool.Get().(*triangleMeshCookScratch)
	defer releaseTriangleMeshCookScratch(scratch)
	scratch.remap = reuse.Resize(scratch.remap, len(vertices))
	uniqueVertices := make([]Vector3D, 0, len(vertices))
	// Imported maps contain hundreds of thousands of vertices. Hash exact
	// positions instead of comparing every vertex with every prior vertex.
	if scratch.unique == nil {
		scratch.unique = make(map[Vector3D]uint32, len(vertices))
	}
	for index, vertex := range vertices {
		if !finiteVector3D(vertex) {
			return TriangleMesh{}, fmt.Errorf("physics: triangle mesh vertex %d is not finite", index)
		}
		mapped, found := scratch.unique[vertex]
		if !found {
			mapped = uint32(len(uniqueVertices))
			scratch.unique[vertex] = mapped
			uniqueVertices = append(uniqueVertices, vertex)
		}
		scratch.remap[index] = mapped
	}
	mesh := TriangleMesh{Vertices: uniqueVertices, Bounds: emptyBounds3D()}
	mesh.Indices = make([]uint32, 0, len(indices))
	scratch.refs = reuse.Resize(scratch.refs, len(indices)/3)[:0]
	for triangle := 0; triangle < len(indices); triangle += 3 {
		a, b, c := indices[triangle], indices[triangle+1], indices[triangle+2]
		if int(a) >= len(vertices) || int(b) >= len(vertices) || int(c) >= len(vertices) {
			return TriangleMesh{}, fmt.Errorf("physics: triangle mesh index exceeds %d vertices", len(vertices))
		}
		a, b, c = scratch.remap[a], scratch.remap[b], scratch.remap[c]
		if a == b || b == c || c == a {
			continue
		}
		area := mesh.Vertices[b].Sub(mesh.Vertices[a]).Cross(mesh.Vertices[c].Sub(mesh.Vertices[a])).LengthSquared()
		if area <= 1e-24 {
			continue
		}
		mesh.Indices = append(mesh.Indices, a, b, c)
		bounds := emptyBounds3D().expand(mesh.Vertices[a]).expand(mesh.Vertices[b]).expand(mesh.Vertices[c])
		scratch.refs = append(scratch.refs, triangleMeshRef{triangle: uint32(len(scratch.refs)), bounds: bounds})
		mesh.Bounds = mesh.Bounds.expand(bounds.Min).expand(bounds.Max)
	}
	if len(mesh.Indices) == 0 {
		return TriangleMesh{}, fmt.Errorf("physics: triangle mesh has no non-degenerate triangles")
	}
	mesh.buildBVH(scratch.refs)
	return mesh, nil
}

// CookTriangleMesh is an explicit name for callers that keep authored mesh
// data separate from its immutable runtime representation.
func CookTriangleMesh(vertices []Vector3D, indices []uint32) (TriangleMesh, error) {
	return NewTriangleMesh(vertices, indices)
}

type triangleMeshRef struct {
	triangle uint32
	bounds   Bounds3D
}

// triangleMeshLeafSize is the largest triangle count kept in a BVH leaf.
const triangleMeshLeafSize = 4

// triangleMeshNodeCount reports the exact node count the balanced split
// produces for count triangles. The split shape depends only on the triangle
// count, so the BVH backing slice is allocated once at its final size instead
// of doubling while it grows. Imported maps hold hundreds of thousands of
// triangles, where the growth slack alone costs tens of megabytes.
func triangleMeshNodeCount(count int) int {
	if count <= triangleMeshLeafSize {
		return 1
	}
	middle := count / 2
	return 1 + triangleMeshNodeCount(middle) + triangleMeshNodeCount(count-middle)
}

func (mesh *TriangleMesh) buildBVH(refs []triangleMeshRef) {
	mesh.BVH = make([]TriangleMeshNode, 0, triangleMeshNodeCount(len(refs)))
	mesh.TriangleIndices = make([]uint32, 0, len(refs))
	buildTriangleMeshNode(mesh, refs)
}

func buildTriangleMeshNode(mesh *TriangleMesh, refs []triangleMeshRef) int32 {
	nodeIndex := int32(len(mesh.BVH))
	mesh.BVH = append(mesh.BVH, TriangleMeshNode{})
	bounds := emptyBounds3D()
	for _, ref := range refs {
		bounds = bounds.expand(ref.bounds.Min).expand(ref.bounds.Max)
	}
	if len(refs) <= triangleMeshLeafSize {
		first := uint32(len(mesh.TriangleIndices))
		for _, ref := range refs {
			mesh.TriangleIndices = append(mesh.TriangleIndices, ref.triangle)
		}
		mesh.BVH[nodeIndex] = TriangleMeshNode{Bounds: bounds, First: first, Count: uint32(len(refs))}
		return nodeIndex
	}
	size := bounds.Size()
	axis := 0
	if size.Y > size.X && size.Y >= size.Z {
		axis = 1
	} else if size.Z > size.X && size.Z > size.Y {
		axis = 2
	}
	slices.SortFunc(refs, func(a, b triangleMeshRef) int {
		left, right := a.bounds.Min.component(axis), b.bounds.Min.component(axis)
		if left < right {
			return -1
		}
		if left > right {
			return 1
		}
		return 0
	})
	middle := len(refs) / 2
	left := buildTriangleMeshNode(mesh, refs[:middle])
	right := buildTriangleMeshNode(mesh, refs[middle:])
	mesh.BVH[nodeIndex] = TriangleMeshNode{Bounds: bounds, Left: left, Right: right}
	return nodeIndex
}

// ConvexFace is one outward-oriented triangle of a cooked convex hull.
type ConvexFace struct {
	Indices [3]uint32
	Normal  Vector3D
}

// ConvexHull is an immutable convex collision resource. It contains the
// original support vertices and the baked surface faces used for contacts.
type ConvexHull struct {
	Vertices []Vector3D
	Faces    []ConvexFace
	Edges    [][2]uint32
	Bounds   Bounds3D
}

func (hull ConvexHull) physicsShape()   {}
func (hull ConvexHull) physicsShape3D() {}

func (hull ConvexHull) VertexCount() int { return len(hull.Vertices) }
func (hull ConvexHull) FaceCount() int   { return len(hull.Faces) }
func (hull ConvexHull) Cooked() bool {
	return len(hull.Vertices) >= 4 && len(hull.Faces) != 0 && len(hull.Edges) != 0
}

// NewConvexHull builds a convex hull during asset baking with an incremental
// QuickHull pass. The expensive visibility and horizon work stays out of the
// simulation and scales quadratically for typical imported meshes.
func NewConvexHull(vertices []Vector3D) (ConvexHull, error) {
	if len(vertices) < 4 {
		return ConvexHull{}, fmt.Errorf("physics: convex hull requires at least four vertices")
	}
	unique := make([]Vector3D, 0, len(vertices))
	bounds := emptyBounds3D()
	first, second := 0, 0
	for index, vertex := range vertices {
		if !finiteVector3D(vertex) {
			return ConvexHull{}, fmt.Errorf("physics: convex hull vertex %d is not finite", index)
		}
		duplicate := false
		for _, prior := range unique {
			if prior.Sub(vertex).LengthSquared() <= 1e-24 {
				duplicate = true
				break
			}
		}
		if !duplicate {
			unique = append(unique, vertex)
			bounds = bounds.expand(vertex)
			if vertex.X < unique[first].X {
				first = len(unique) - 1
			}
			if vertex.X > unique[second].X {
				second = len(unique) - 1
			}
		}
	}
	if len(unique) < 4 {
		return ConvexHull{}, fmt.Errorf("physics: convex hull has fewer than four unique vertices")
	}
	hull := ConvexHull{Vertices: unique, Bounds: bounds}
	epsilon := max(hull.Bounds.Size().LengthSquared()*1e-12, 1e-24)
	line := unique[second].Sub(unique[first])
	lineLengthSquared := line.LengthSquared()
	third, maxLineDistance := -1, 0.0
	for index, vertex := range unique {
		if index == first || index == second {
			continue
		}
		distance := line.Cross(vertex.Sub(unique[first])).LengthSquared()
		if distance > maxLineDistance {
			third, maxLineDistance = index, distance
		}
	}
	if lineLengthSquared <= epsilon || third < 0 || maxLineDistance <= epsilon*lineLengthSquared {
		return ConvexHull{}, fmt.Errorf("physics: convex hull vertices are collinear")
	}
	plane := line.Cross(unique[third].Sub(unique[first]))
	fourth, maxPlaneDistance := -1, 0.0
	for index, vertex := range unique {
		if index == first || index == second || index == third {
			continue
		}
		distance := math.Abs(plane.Dot(vertex.Sub(unique[first])))
		if distance > maxPlaneDistance {
			fourth, maxPlaneDistance = index, distance
		}
	}
	if fourth < 0 || maxPlaneDistance <= epsilon {
		return ConvexHull{}, fmt.Errorf("physics: convex hull vertices are coplanar")
	}
	interior := unique[first].Add(unique[second]).Add(unique[third]).Add(unique[fourth]).Mul(.25)
	scratch := convexHullCookPool.Get().(*convexHullCookScratch)
	defer releaseConvexHullCookScratch(scratch)
	if scratch.horizon == nil {
		scratch.horizon = make(map[[2]uint32]horizonEdge)
	}
	edges := scratch.horizon
	addEdge := func(a, b uint32) {
		key := [2]uint32{a, b}
		if key[0] > key[1] {
			key[0], key[1] = key[1], key[0]
		}
		edge := edges[key]
		if edge.count == 0 {
			edge = horizonEdge{a: a, b: b}
		}
		edge.count++
		edges[key] = edge
	}
	faces := growCapacity(scratch.faces, 16)
	appendFace := func(a, b, c int) {
		normal := unique[b].Sub(unique[a]).Cross(unique[c].Sub(unique[a]))
		if normal.LengthSquared() <= epsilon {
			return
		}
		if normal.Dot(interior.Sub(unique[a])) > 0 {
			a, c = c, a
			normal = normal.Mul(-1)
		}
		faces = append(faces, ConvexFace{Indices: [3]uint32{uint32(a), uint32(b), uint32(c)}, Normal: normal.Normalize()})
	}
	appendFace(first, second, third)
	appendFace(first, fourth, second)
	appendFace(first, third, fourth)
	appendFace(second, fourth, third)
	used := [4]int{first, second, third, fourth}
	for pointIndex, point := range unique {
		if pointIndex == used[0] || pointIndex == used[1] || pointIndex == used[2] || pointIndex == used[3] {
			continue
		}
		clear(edges)
		remaining := faces[:0]
		for _, face := range faces {
			a := unique[face.Indices[0]]
			if face.Normal.Dot(point.Sub(a)) > epsilon {
				addEdge(face.Indices[0], face.Indices[1])
				addEdge(face.Indices[1], face.Indices[2])
				addEdge(face.Indices[2], face.Indices[0])
			} else {
				remaining = append(remaining, face)
			}
		}
		faces = remaining
		scratch.horizonPeak = max(scratch.horizonPeak, len(edges))
		for _, edge := range edges {
			if edge.count == 1 {
				appendFace(int(edge.a), int(edge.b), pointIndex)
			}
		}
	}
	scratch.faces = faces
	if len(faces) == 0 {
		return ConvexHull{}, fmt.Errorf("physics: convex hull has no surface faces")
	}
	hull.Faces = slices.Clone(faces)
	hull.buildEdges(scratch)
	return hull, nil
}

func (hull *ConvexHull) buildEdges(scratch *convexHullCookScratch) {
	if scratch.seen == nil {
		scratch.seen = make(map[[2]uint32]struct{}, min(len(hull.Faces)*3/2, maxCachedCookEntries))
	}
	seen := scratch.seen
	// Each edge of a closed triangular hull belongs to two faces.
	hull.Edges = make([][2]uint32, 0, len(hull.Faces)*3/2)
	for _, face := range hull.Faces {
		for _, edge := range [][2]uint32{{face.Indices[0], face.Indices[1]}, {face.Indices[1], face.Indices[2]}, {face.Indices[2], face.Indices[0]}} {
			if edge[0] > edge[1] {
				edge[0], edge[1] = edge[1], edge[0]
			}
			if _, exists := seen[edge]; exists {
				continue
			}
			seen[edge] = struct{}{}
			hull.Edges = append(hull.Edges, edge)
		}
	}
}

// QuickHull is the importer-facing name for convex hull cooking.
func QuickHull(vertices []Vector3D) (ConvexHull, error) { return NewConvexHull(vertices) }

// CompoundChild is a locally transformed, already-cooked convex hull.
type CompoundChild struct {
	Shape    ConvexHull
	Position Vector3D
	Rotation Quaternion
}

// Compound is an immutable collection of convex hulls for dynamic concave
// models. Children are never decomposed or allocated during simulation.
type Compound struct {
	Children []CompoundChild
	Bounds   Bounds3D
}

func (compound Compound) physicsShape()   {}
func (compound Compound) physicsShape3D() {}

func (compound Compound) ChildCount() int { return len(compound.Children) }
func (compound Compound) Cooked() bool    { return len(compound.Children) != 0 && compound.Bounds.Valid() }

func NewCompound(children []CompoundChild) (Compound, error) {
	if len(children) == 0 {
		return Compound{}, fmt.Errorf("physics: compound requires at least one convex hull")
	}
	compound := Compound{}
	compound.Children = append([]CompoundChild(nil), children...)
	compound.Bounds = emptyBounds3D()
	for index := range compound.Children {
		child := &compound.Children[index]
		if len(child.Shape.Vertices) < 4 || len(child.Shape.Faces) == 0 {
			return Compound{}, fmt.Errorf("physics: compound child %d is not a cooked convex hull", index)
		}
		if len(child.Shape.Edges) == 0 {
			scratch := convexHullCookPool.Get().(*convexHullCookScratch)
			child.Shape.buildEdges(scratch)
			releaseConvexHullCookScratch(scratch)
		}
		if !finiteVector3D(child.Position) || !finiteQuaternion(child.Rotation) {
			return Compound{}, fmt.Errorf("physics: compound child %d transform is not finite", index)
		}
		child.Rotation = child.Rotation.Normalize()
		for _, vertex := range child.Shape.Vertices {
			compound.Bounds = compound.Bounds.expand(child.Position.Add(child.Rotation.Rotate(vertex)))
		}
	}
	return compound, nil
}

// ConvexDecomposition creates the runtime compound from importer-produced
// convex pieces. CoACD/V-HACD integration belongs to the asset pipeline; this
// API keeps the runtime format independent of whichever offline tool produced
// the pieces.
func ConvexDecomposition(pieces []ConvexHull) (Compound, error) {
	children := make([]CompoundChild, len(pieces))
	for index, piece := range pieces {
		children[index].Shape = piece
	}
	return NewCompound(children)
}

func NewCompoundFromHulls(pieces []ConvexHull) (Compound, error) {
	return ConvexDecomposition(pieces)
}

func finiteVector3D(value Vector3D) bool {
	return !math.IsNaN(value.X) && !math.IsInf(value.X, 0) &&
		!math.IsNaN(value.Y) && !math.IsInf(value.Y, 0) &&
		!math.IsNaN(value.Z) && !math.IsInf(value.Z, 0)
}

func finiteQuaternion(value Quaternion) bool {
	return !math.IsNaN(value.X) && !math.IsInf(value.X, 0) &&
		!math.IsNaN(value.Y) && !math.IsInf(value.Y, 0) &&
		!math.IsNaN(value.Z) && !math.IsInf(value.Z, 0) &&
		!math.IsNaN(value.W) && !math.IsInf(value.W, 0)
}
