package project

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/diiyw/onix/graphics"
	enginephysics "github.com/diiyw/onix/physics"
	"github.com/diiyw/onix/render"
)

type ModelSpec struct {
	AnimationPath string                      `json:"animationPath,omitempty"`
	ID            string                      `json:"id"`
	Path          string                      `json:"path,omitempty"`
	Mesh          *MeshSpec                   `json:"mesh,omitempty"`
	Material      ModelMaterialSpec           `json:"material,omitempty"`
	Collision     enginephysics.CollisionMode `json:"collision,omitempty"`
	CollisionPath string                      `json:"collisionPath,omitempty"`
}

type MeshSpec struct {
	Primitive string           `json:"primitive,omitempty"`
	Size      float32          `json:"size,omitempty"`
	Radius    float32          `json:"radius,omitempty"`
	Sectors   int              `json:"sectors,omitempty"`
	Stacks    int              `json:"stacks,omitempty"`
	Vertices  []MeshVertexSpec `json:"vertices,omitempty"`
	Indices   []uint32         `json:"indices,omitempty"`
}

type MeshVertexSpec struct {
	Position [3]float32 `json:"position"`
	Normal   [3]float32 `json:"normal"`
	UV       [2]float32 `json:"uv,omitempty"`
}

type ModelMaterialSpec struct {
	Shader              string  `json:"shader,omitempty"`
	BaseColor           string  `json:"baseColor,omitempty"`
	Emissive            string  `json:"emissive,omitempty"`
	IOR                 float32 `json:"ior,omitempty"`
	EmissiveStrength    float32 `json:"emissiveStrength,omitempty"`
	Metallic            float32 `json:"metallic,omitempty"`
	PerceptualRoughness float32 `json:"roughness,omitempty"`
	MetallicSet         bool    `json:"-"`
	RoughnessSet        bool    `json:"-"`
	IORSet              bool    `json:"-"`
	EmissiveStrengthSet bool    `json:"-"`
}

// equalModelSpec keeps model synchronization allocation-free, so an unchanged
// resource does not dispose a mesh that is still mounted by the scene.
func equalModelSpec(left, right ModelSpec) bool {
	return left.ID == right.ID &&
		left.Path == right.Path &&
		left.AnimationPath == right.AnimationPath &&
		left.Collision == right.Collision &&
		left.CollisionPath == right.CollisionPath &&
		left.Material == right.Material &&
		equalMeshSpec(left.Mesh, right.Mesh)
}

func equalMeshSpec(left, right *MeshSpec) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Primitive == right.Primitive &&
		left.Size == right.Size &&
		left.Radius == right.Radius &&
		left.Sectors == right.Sectors &&
		left.Stacks == right.Stacks &&
		slices.Equal(left.Vertices, right.Vertices) &&
		slices.Equal(left.Indices, right.Indices)
}

func (s ModelSpec) validate() error {
	id := strings.TrimSpace(s.ID)
	if id == "" {
		return fmt.Errorf("model id is required")
	}
	if strings.TrimSpace(s.Path) == "" && s.Mesh == nil && strings.TrimSpace(s.CollisionPath) == "" {
		return fmt.Errorf("model %q requires a path, mesh, or collisionPath", id)
	}
	if strings.TrimSpace(s.Path) != "" && s.Mesh != nil {
		return fmt.Errorf("model %q cannot combine path and mesh", id)
	}
	if s.Path != "" {
		path, err := NormalizeResourcePath(s.Path)
		if err != nil {
			return fmt.Errorf("model %q path: %w", id, err)
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".fbx", ".gltf", ".glb":
		default:
			return fmt.Errorf("model %q path must use .fbx, .gltf, or .glb", id)
		}
	} else if s.Mesh != nil {
		if err := s.Mesh.validate(); err != nil {
			return fmt.Errorf("model %q mesh: %w", id, err)
		}
	}
	for name, value := range map[string]string{"baseColor": s.Material.BaseColor, "emissive": s.Material.Emissive} {
		if value != "" {
			if _, err := parseColorValue(value, 1); err != nil {
				return fmt.Errorf("model %q %s: %w", id, name, err)
			}
		}
	}
	if s.Material.Metallic < 0 || s.Material.Metallic > 1 {
		return fmt.Errorf("model %q metallic must be between 0 and 1", id)
	}
	if s.Material.PerceptualRoughness < 0 || s.Material.PerceptualRoughness > 1 {
		return fmt.Errorf("model %q roughness must be between 0 and 1", id)
	}
	if !finiteFloat32(s.Material.IOR) || (s.Material.IOR != 0 && s.Material.IOR < 1) {
		return fmt.Errorf("model %q IOR must be finite and either zero or at least 1", id)
	}
	if !finiteFloat32(s.Material.EmissiveStrength) || s.Material.EmissiveStrength < 0 {
		return fmt.Errorf("model %q emissive strength must be finite and non-negative", id)
	}
	if s.AnimationPath != "" {
		path, err := NormalizeResourcePath(s.AnimationPath)
		if err != nil || !strings.HasSuffix(path, ".json") {
			return fmt.Errorf("model %q animationPath must reference a local JSON file", id)
		}
	}
	if s.Collision != "" {
		if _, err := enginephysics.ParseCollisionMode(string(s.Collision)); err != nil {
			return fmt.Errorf("model %q collision: %w", id, err)
		}
	}
	if strings.TrimSpace(s.CollisionPath) != "" {
		if _, err := normalizeCollisionResourcePath(s.CollisionPath); err != nil {
			return fmt.Errorf("model %q collisionPath: %w", id, err)
		}
	}
	return nil
}

func (s ModelSpec) Validate() error {
	return s.validate()
}

func (s MeshSpec) validate() error {
	primitive := strings.ToLower(strings.TrimSpace(s.Primitive))
	if primitive != "" && (len(s.Vertices) > 0 || len(s.Indices) > 0) {
		return fmt.Errorf("primitive cannot be combined with vertices or indices")
	}
	switch primitive {
	case "cube", "plane":
		return nil
	case "sphere":
		sectors, stacks := s.Sectors, s.Stacks
		if sectors == 0 {
			sectors = 32
		}
		if stacks == 0 {
			stacks = 16
		}
		if sectors < 3 || stacks < 2 {
			return fmt.Errorf("sphere requires sectors >= 3 and stacks >= 2")
		}
		return nil
	case "":
		if len(s.Vertices) == 0 {
			return fmt.Errorf("mesh has no vertices")
		}
		if len(s.Indices) == 0 || len(s.Indices)%3 != 0 {
			return fmt.Errorf("mesh index count must be a non-zero multiple of three")
		}
		for _, index := range s.Indices {
			if int(index) >= len(s.Vertices) {
				return fmt.Errorf("mesh index %d exceeds %d vertices", index, len(s.Vertices))
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported primitive %q", s.Primitive)
	}
}

func (s MeshSpec) build() (*graphics.Mesh, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	primitive := strings.ToLower(strings.TrimSpace(s.Primitive))
	switch primitive {
	case "cube":
		size := s.Size
		if size <= 0 {
			size = 1
		}
		return graphics.CubeMesh(size), nil
	case "plane":
		size := s.Size
		if size <= 0 {
			size = 1
		}
		return graphics.PlaneMesh(size), nil
	case "sphere":
		radius, sectors, stacks := s.Radius, s.Sectors, s.Stacks
		if radius <= 0 {
			radius = 0.5
		}
		if sectors == 0 {
			sectors = 32
		}
		if stacks == 0 {
			stacks = 16
		}
		if sectors < 3 || stacks < 2 {
			return nil, fmt.Errorf("sphere requires sectors >= 3 and stacks >= 2")
		}
		return graphics.UVSphereMesh(radius, sectors, stacks), nil
	case "":
		vertices := make([]render.Vertex3D, len(s.Vertices))
		for i, vertex := range s.Vertices {
			vertices[i] = render.Vertex3D{Position: vertex.Position, Normal: vertex.Normal, UV: vertex.UV}
		}
		return graphics.NewMesh(vertices, s.Indices)
	default:
		return nil, fmt.Errorf("unsupported primitive %q", s.Primitive)
	}
}

// CookCollision converts a render mesh into the immutable collision resource
// selected by the model's import mode. It is intended for the asset-loading
// stage; collider synchronization only receives the returned baked value.
func (s ModelSpec) CookCollision(mesh *graphics.Mesh) (enginephysics.CookedCollision, error) {
	if mesh == nil {
		return enginephysics.CookedCollision{}, fmt.Errorf("model %q collision requires a mesh", s.ID)
	}
	mode := s.Collision
	if mode == "" {
		mode = enginephysics.CollisionModeTriangleMesh
	}
	vertices := make([]enginephysics.Vector3D, len(mesh.Vertices))
	for index, vertex := range mesh.Vertices {
		vertices[index] = enginephysics.V3(float64(vertex.Position[0]), float64(vertex.Position[1]), float64(vertex.Position[2]))
	}
	return enginephysics.CookCollision(vertices, mesh.Indices, mode)
}
