package project

import (
	"fmt"
	"math"
	"strings"

	enginephysics "github.com/diiyw/onix/physics"
)

// CollisionResourceSpec is the interchange format for collision geometry
// exported by an offline baker such as CoACD. The runtime only consumes this
// deterministic geometry and never invokes the decomposition tool.
//
// Collision resources are kept under resources/ and use a .collision.json
// suffix. A convex decomposition resource stores one convex piece per entry;
// piece indices are optional because the hull is reconstructed from support
// vertices, but are validated when present.
type CollisionResourceSpec struct {
	Mode     enginephysics.CollisionMode    `json:"mode"`
	Vertices [][3]float64                   `json:"vertices,omitempty"`
	Indices  []uint32                       `json:"indices,omitempty"`
	Pieces   []CollisionPieceSpec           `json:"pieces,omitempty"`
	Cooked   *enginephysics.CookedCollision `json:"-"`
}

type CollisionPieceSpec struct {
	Vertices [][3]float64 `json:"vertices"`
	Indices  []uint32     `json:"indices,omitempty"`
	Position [3]float64   `json:"position,omitempty"`
	Rotation [4]float64   `json:"rotation,omitempty"`
}

func normalizeCollisionResourcePath(value string) (string, error) {
	normalized, err := NormalizeResourcePath(value)
	if err != nil {
		return "", err
	}
	lower := strings.ToLower(normalized)
	if !strings.HasSuffix(lower, ".collision.json") {
		return "", fmt.Errorf("collision resource path must use .collision.json")
	}
	return normalized, nil
}

func collisionVector3(value [3]float64) (enginephysics.Vector3D, error) {
	for index, component := range value {
		if math.IsNaN(component) || math.IsInf(component, 0) {
			return enginephysics.Vector3D{}, fmt.Errorf("vertex component %d is not finite", index)
		}
	}
	return enginephysics.V3(value[0], value[1], value[2]), nil
}

func collisionVertices(values [][3]float64) ([]enginephysics.Vector3D, error) {
	vertices := make([]enginephysics.Vector3D, len(values))
	for index, value := range values {
		vertex, err := collisionVector3(value)
		if err != nil {
			return nil, fmt.Errorf("vertex %d: %w", index, err)
		}
		vertices[index] = vertex
	}
	return vertices, nil
}

func validateCollisionIndices(indices []uint32, vertexCount int, required bool) error {
	if required && (len(indices) == 0 || len(indices)%3 != 0) {
		return fmt.Errorf("indices must be a non-zero multiple of three")
	}
	if len(indices)%3 != 0 {
		return fmt.Errorf("indices must contain a multiple of three values")
	}
	for index, value := range indices {
		if int(value) >= vertexCount {
			return fmt.Errorf("index %d exceeds %d vertices", index, vertexCount)
		}
	}
	return nil
}

func (spec CollisionResourceSpec) cook(expected enginephysics.CollisionMode) (enginephysics.CookedCollision, error) {
	if strings.TrimSpace(string(spec.Mode)) == "" {
		return enginephysics.CookedCollision{}, fmt.Errorf("collision resource mode is required")
	}
	mode, err := enginephysics.ParseCollisionMode(string(spec.Mode))
	if err != nil {
		return enginephysics.CookedCollision{}, err
	}
	if strings.TrimSpace(string(expected)) != "" {
		expectedMode, parseErr := enginephysics.ParseCollisionMode(string(expected))
		if parseErr != nil {
			return enginephysics.CookedCollision{}, parseErr
		}
		if mode != expectedMode {
			return enginephysics.CookedCollision{}, fmt.Errorf("collision resource mode %q does not match model mode %q", mode, expectedMode)
		}
	}
	switch mode {
	case enginephysics.CollisionModeTriangleMesh:
		if len(spec.Pieces) != 0 {
			return enginephysics.CookedCollision{}, fmt.Errorf("triangle mesh collision cannot contain pieces")
		}
		vertices, vertexErr := collisionVertices(spec.Vertices)
		if vertexErr != nil {
			return enginephysics.CookedCollision{}, vertexErr
		}
		if indexErr := validateCollisionIndices(spec.Indices, len(vertices), true); indexErr != nil {
			return enginephysics.CookedCollision{}, indexErr
		}
		mesh, meshErr := enginephysics.NewTriangleMesh(vertices, spec.Indices)
		if meshErr != nil {
			return enginephysics.CookedCollision{}, meshErr
		}
		return enginephysics.CookedCollision{Mode: mode, TriangleMesh: mesh}, nil
	case enginephysics.CollisionModeSingleConvex:
		if len(spec.Pieces) != 0 {
			return enginephysics.CookedCollision{}, fmt.Errorf("single convex collision cannot contain pieces")
		}
		vertices, vertexErr := collisionVertices(spec.Vertices)
		if vertexErr != nil {
			return enginephysics.CookedCollision{}, vertexErr
		}
		if indexErr := validateCollisionIndices(spec.Indices, len(vertices), false); indexErr != nil {
			return enginephysics.CookedCollision{}, indexErr
		}
		hull, hullErr := enginephysics.NewConvexHull(vertices)
		if hullErr != nil {
			return enginephysics.CookedCollision{}, hullErr
		}
		return enginephysics.CookedCollision{Mode: mode, ConvexHull: hull}, nil
	case enginephysics.CollisionModeConvexDecomposition:
		if len(spec.Pieces) == 0 {
			return enginephysics.CookedCollision{}, fmt.Errorf("convex decomposition collision requires at least one piece")
		}
		if len(spec.Vertices) != 0 || len(spec.Indices) != 0 {
			return enginephysics.CookedCollision{}, fmt.Errorf("convex decomposition collision must store geometry in pieces")
		}
		children := make([]enginephysics.CompoundChild, len(spec.Pieces))
		for index, pieceSpec := range spec.Pieces {
			vertices, vertexErr := collisionVertices(pieceSpec.Vertices)
			if vertexErr != nil {
				return enginephysics.CookedCollision{}, fmt.Errorf("piece %d: %w", index, vertexErr)
			}
			if indexErr := validateCollisionIndices(pieceSpec.Indices, len(vertices), false); indexErr != nil {
				return enginephysics.CookedCollision{}, fmt.Errorf("piece %d: %w", index, indexErr)
			}
			hull, hullErr := enginephysics.NewConvexHull(vertices)
			if hullErr != nil {
				return enginephysics.CookedCollision{}, fmt.Errorf("piece %d: %w", index, hullErr)
			}
			position := enginephysics.V3(pieceSpec.Position[0], pieceSpec.Position[1], pieceSpec.Position[2])
			rotation := enginephysics.Q(pieceSpec.Rotation[0], pieceSpec.Rotation[1], pieceSpec.Rotation[2], pieceSpec.Rotation[3])
			if rotation == (enginephysics.Quaternion{}) {
				rotation = enginephysics.QuaternionIdentity()
			}
			children[index] = enginephysics.CompoundChild{Shape: hull, Position: position, Rotation: rotation}
		}
		compound, compoundErr := enginephysics.NewCompound(children)
		if compoundErr != nil {
			return enginephysics.CookedCollision{}, compoundErr
		}
		return enginephysics.CookedCollision{Mode: mode, Compound: compound}, nil
	default:
		return enginephysics.CookedCollision{}, fmt.Errorf("unsupported collision resource mode %q", mode)
	}
}

func (r *Resources) loadCollisionResource(path string, expected enginephysics.CollisionMode) (enginephysics.CookedCollision, error) {
	if r == nil {
		return enginephysics.CookedCollision{}, fmt.Errorf("resources are nil")
	}
	normalized, err := normalizeCollisionResourcePath(path)
	if err != nil {
		return enginephysics.CookedCollision{}, err
	}
	resolved, err := ResolveResourcePath(r.baseDir, normalized)
	if err != nil {
		return enginephysics.CookedCollision{}, err
	}
	spec, err := readJSON[CollisionResourceSpec](r.source, resolved)
	if err != nil {
		return enginephysics.CookedCollision{}, fmt.Errorf("read collision resource %q: %w", path, err)
	}
	if spec.Cooked != nil {
		if expected != "" && spec.Cooked.Mode != expected {
			return enginephysics.CookedCollision{}, fmt.Errorf("collision resource mode %q does not match model mode %q", spec.Cooked.Mode, expected)
		}
		return *spec.Cooked, nil
	}
	cooked, err := spec.cook(expected)
	if err != nil {
		return enginephysics.CookedCollision{}, fmt.Errorf("collision resource %q: %w", path, err)
	}
	return cooked, nil
}
