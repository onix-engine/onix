package project

import (
	"encoding/json"
	"github.com/diiyw/onix/scene"
	engineui "github.com/diiyw/onix/ui"
)

const (
	DefaultSceneFile = "main.json"
	SceneDirectory   = "resources/scenes"
	ModFileName      = "mod.json"
)

type ModID string
type ValueType string

const (
	StringValue ValueType = "string"
	BoolValue   ValueType = "bool"
	NumberValue ValueType = "number"
	IntValue    ValueType = "int"
	ColorValue  ValueType = "color"
	Vec2Value   ValueType = "vec2"
	Vec3Value   ValueType = "vec3"
	VisualValue ValueType = "visual"
)

// ProjectSpec selects the entry and scene order. Scene files live in
// resources/scenes; compilation discovers additional scenes in that directory.
type ProjectSpec struct {
	Types      map[ValueType]ValueSchema `json:"types,omitempty"`
	Entry      SceneID                   `json:"entry"`
	ClearColor string                    `json:"clearColor,omitempty"`
	Engine     EngineConfig              `json:"engine,omitempty"`
	Scenes     []SceneID                 `json:"scenes,omitempty"`
	Viewport   SizeConfig                `json:"viewport,omitempty"`
	Window     WindowConfig              `json:"window,omitempty"`
}

type ValueSchema struct {
	Fields map[string]ValueType `json:"fields"`
}

// SceneExitPolicy controls the departing scene's lifetime. The zero value uses
// the switch request's Retain setting, which defaults to destruction.
type SceneExitPolicy string

const (
	SceneExitDestroy SceneExitPolicy = "destroy"
	SceneExitRetain  SceneExitPolicy = "retain"
)

type SceneSpec struct {
	Behavior string `json:"behavior,omitempty"`
	// ExitPolicy overrides the switch's Retain setting when set. Overlays
	// always retain their background scene until it is explicitly disposed.
	ExitPolicy SceneExitPolicy   `json:"exitPolicy,omitempty"`
	Modules    []ModInstanceSpec `json:"modules"`
	Transform  ModTransformSpec  `json:"transform,omitzero"`
}

type ModSpec struct {
	Components ModComponents              `json:"components,omitzero"`
	Resources  map[string]ModResourceSpec `json:"resources,omitempty"`
	Slots      map[string]SlotSpec        `json:"slots,omitempty"`
	Name       string                     `json:"name,omitempty"`
	Space      CoordinateSpace            `json:"space,omitempty"`
	Behavior   string                     `json:"behavior,omitempty"`
	Transform  ModTransformSpec           `json:"transform,omitzero"`
	Elements   []ModElementSpec           `json:"elements,omitempty"`
}

type ModInstanceSpec struct {
	Slots     map[string]json.RawMessage `json:"slots,omitempty"`
	Enabled   *bool                      `json:"enabled,omitempty"`
	Visible   *bool                      `json:"visible,omitempty"`
	ID        string                     `json:"id"`
	Mod       ModID                      `json:"mod"`
	Placement ModTransformSpec           `json:"placement,omitzero"`
	Children  []ModInstanceSpec          `json:"children,omitempty"`
}

type ModTransformSpec struct {
	Position []float32 `json:"position,omitempty"`
	Rotation []float32 `json:"rotation,omitempty"`
	Scale    []float32 `json:"scale,omitempty"`
}

// Elements are private implementation details of one module. Scene messages and
// placements cannot address them; only the owning behavior can obtain a handle.
type ModElementSpec struct {
	Components ModComponents    `json:"components,omitzero"`
	Visible    *bool            `json:"visible,omitempty"`
	ID         string           `json:"id"`
	Parent     string           `json:"parent,omitempty"`
	Space      CoordinateSpace  `json:"space,omitempty"`
	Transform  ModTransformSpec `json:"transform,omitzero"`
}

type ModComponents struct {
	RigidBody3D            *RigidBody3DSpec            `json:"rigidBody3D,omitempty"`
	AmbientLight           *AmbientLightSpec           `json:"ambientLight,omitempty"`
	Anchor                 *scene.Anchor               `json:"anchor,omitempty"`
	Sprite                 *ModSpriteSpec              `json:"sprite,omitempty"`
	Trail                  *SpriteTrailSpec            `json:"trail,omitempty"`
	Material               *ModMaterial2DSpec          `json:"material,omitempty"`
	Shader                 *ShaderComponentSpec        `json:"shader,omitempty"`
	Input                  *ModInputSpec               `json:"input,omitempty"`
	SegmentCollider2D      *SegmentCollider2DSpec      `json:"segmentCollider2D,omitempty"`
	Animation              *AnimationComponentSpec     `json:"animation,omitempty"`
	HTML                   *ModHTMLSpec                `json:"html,omitempty"`
	Emitter                *EmitterSpec                `json:"emitter,omitempty"`
	Particle               *TextureSpec                `json:"particle,omitempty"`
	Tilemap                *TilemapSpec                `json:"tilemap,omitempty"`
	Layout                 *engineui.Layout            `json:"layout,omitempty"`
	TriangleMeshCollider3D *TriangleMeshCollider3DSpec `json:"triangleMeshCollider3D,omitempty"`
	Text                   *TextComponentSpec          `json:"text,omitempty"`
	Spine                  *ModSpineSpec               `json:"spine,omitempty"`
	ConvexHullCollider3D   *ConvexHullCollider3DSpec   `json:"convexHullCollider3D,omitempty"`
	CompoundCollider3D     *CompoundCollider3DSpec     `json:"compoundCollider3D,omitempty"`
	Camera                 *Camera3DSpec               `json:"camera,omitempty"`
	Camera2D               *ModCamera2DSpec            `json:"camera2D,omitempty"`
	Model                  *ModelSpec                  `json:"model,omitempty"`
	DirectionalLight       *DirectionalLightSpec       `json:"directionalLight,omitempty"`
	PointLight             *PointLightSpec             `json:"pointLight,omitempty"`
	SpotLight              *SpotLightSpec              `json:"spotLight,omitempty"`
	AreaLight              *AreaLightSpec              `json:"areaLight,omitempty"`
	RigidBody2D            *RigidBody2DSpec            `json:"rigidBody2D,omitempty"`
	Collision              *ModCollisionSpec           `json:"collision,omitempty"`
	CircleCollider2D       *CircleCollider2DSpec       `json:"circleCollider2D,omitempty"`
	BoxCollider2D          *BoxCollider2DSpec          `json:"boxCollider2D,omitempty"`
	Size                   []float32                   `json:"size,omitempty"`
}

type ModHTMLSpec struct {
	Resource    string `json:"resource"`
	Font        string `json:"font,omitempty"`
	BoldFont    string `json:"boldFont,omitempty"`
	PassThrough bool   `json:"passThrough,omitempty"`
}

// Collision publishes solver results without exposing either module's ECS IDs.
type ModCollisionSpec struct {
	Emit string `json:"emit"`
}

type ModCamera2DSpec struct {
	Clear      *bool                `json:"clear,omitempty"`
	ClearColor string               `json:"clearColor,omitempty"`
	Order      int                  `json:"order,omitempty"`
	Viewport   scene.CameraViewport `json:"viewport,omitzero"`
	Scale      float32              `json:"scale,omitempty"`
}

type ModSpineSpec struct {
	Source    SpineSpec `json:"source"`
	Animation string    `json:"animation,omitempty"`
	Skin      string    `json:"skin,omitempty"`
	Loop      bool      `json:"loop,omitempty"`
	Speed     float32   `json:"speed,omitempty"`
}

type ModSpriteSpec struct {
	Texture *TextureSpec `json:"texture,omitempty"`
	Alpha   *float32     `json:"alpha,omitempty"`
	Color   string       `json:"color,omitempty"`
	Size    []float32    `json:"size,omitempty"`
}

type ModMaterial2DSpec struct {
	Custom [4][4]float32 `json:"custom,omitzero"`
}

type ModInputSpec struct {
	Pointer  bool `json:"pointer,omitempty"`
	Keyboard bool `json:"keyboard,omitempty"`
	Text     bool `json:"text,omitempty"`
	Files    bool `json:"files,omitempty"`
}

type ModResourceSpec struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
}

type SlotSpec struct {
	Type      ValueType        `json:"type"`
	Default   json.RawMessage  `json:"default,omitempty"`
	Writable  *bool            `json:"writable,omitempty"`
	Placement ModTransformSpec `json:"placement,omitzero"`
}

// Plans contain resolved indices and typed values. JSON bytes are authoring
// data only; dispatch and slot updates do not need to decode JSON or use any.
type ProjectPlan struct {
	Mods   []ModPlan
	Scenes []ScenePlan
	Spec   ProjectSpec
	Entry  uint32
}

type ModPlan struct {
	Spec      ModSpec
	ID        ModID
	Resources []ModResourcePlan
	Slots     []SlotPlan
	Elements  []ModElementPlan
	Transform ModTransform
}

type ModElementPlan struct {
	Spec      ModElementSpec
	Parent    int32
	Transform ModTransform
}

type ModResourcePlan struct {
	Name string
	Kind string
	Path string
}

type ModTransform struct {
	Position [3]float32
	Rotation [3]float32
	Scale    [3]float32
}

type SlotPlan struct {
	Name      string
	Type      ValueType
	Default   SlotValue
	Writable  bool
	Placement ModTransform
}

type SlotValue struct {
	Type    ValueType
	String  string
	Fields  []SlotFieldValue
	Number  float64
	Integer int64
	Vector  [4]float32
	Bool    bool
}

type SlotFieldValue struct {
	Name  string
	Value SlotValue
}

type ScenePlan struct {
	ID        SceneID
	Spec      SceneSpec
	Instances []ModInstancePlan
	Transform ModTransform
}

type ModInstancePlan struct {
	ID        string
	Slots     []SlotValue
	Placement ModTransform
	Mod       uint32
	Parent    int32
	Enabled   bool
	Visible   bool
}
