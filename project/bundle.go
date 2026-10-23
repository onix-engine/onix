package project

import (
	"encoding/json"
	"errors"
	"fmt"
	iofs "io/fs"
	"math"
	pathpkg "path"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/internal/sourcejson"
	"github.com/diiyw/onix/internal/typedbin"
	enginephysics "github.com/diiyw/onix/physics"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/resource"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/scene/render3d"
	spineruntime "github.com/diiyw/onix/spine"
	onixtext "github.com/diiyw/onix/text"
	engineui "github.com/diiyw/onix/ui"
	"github.com/diiyw/onix/window"
)

type Bundle struct {
	resources       *Resources
	resourceSpecMu  sync.RWMutex
	clearColor      graphics.ColorRGBA
	hasClear        bool
	appConfig       AppConfig
	engineConfig    EngineConfig
	components      sceneComponentBuilder
	clearComponents sceneComponentClearer
	uiSync          sceneUISync

	windowIcon []byte

	entrySceneID    SceneID
	scenePlansByID  map[SceneID]sceneSpawnPlan
	sceneIDs        []SceneID
	prefabPlansByID map[PrefabID]*prefabPlan
	prefabIDs       []PrefabID

	particleSpecs        map[string]ParticleSpec
	tilemapSpecs         map[string]TilemapSpec
	modelSpecs           map[string]ModelSpec
	modelResources       modelResourceStore
	spineSpecs           map[string]SpineSpec
	inlineAnimationClips *resourceCache[string, *scene.AnimationClip]
}

type modelResourceStore interface {
	resourceDisposer
}

// optionalResourceStore keeps feature-specific resource caches behind the
// resource lifecycle boundary. Narrow profiles leave these fields nil, so the
// concrete video/HTML cache instantiations are not reachable from their build.
type optionalResourceStore interface {
	resourceDisposer
	resourceStatisticsProvider
	ready(string) bool
}

type spineResourceStore = optionalResourceStore

type modelResource struct {
	mesh          *graphics.Mesh
	material      *graphics.StandardMaterial
	materials     []graphics.StandardMaterial
	ownedTextures []*graphics.Texture
	asset         compiledModelResource
	collision     enginephysics.CookedCollision
	hasCollision  bool
}

func (value modelResource) dispose() error {
	var disposeErr error
	if value.mesh != nil {
		disposeErr = value.mesh.Dispose()
	}
	for _, texture := range value.ownedTextures {
		if texture != nil {
			disposeErr = errors.Join(disposeErr, texture.Dispose())
		}
	}
	return disposeErr
}

type resourceManifest struct {
	Prepared bool                 `json:"-"`
	Fonts    []FileResource       `json:"fonts"`
	Audio    []FileResource       `json:"audio"`
	Videos   []FileResource       `json:"videos"`
	Effects  []ParticleSpec       `json:"effects"`
	Tilemaps []TilemapSpec        `json:"tilemaps"`
	Models   []ModelSpec          `json:"models"`
	Spines   []SpineSpec          `json:"spines"`
	Shaders  []ShaderSpec         `json:"shaders"`
	HTML     []FileResource       `json:"html"`
	Prefabs  []PrefabResourceSpec `json:"prefabs"`
}

type gameManifest struct {
	ClearColor string       `json:"clearColor"`
	Window     WindowConfig `json:"window"`
	Viewport   SizeConfig   `json:"viewport"`
	Engine     EngineConfig `json:"engine"`
}

type AppConfig struct {
	Window   WindowConfig
	Viewport SizeConfig
}

type WindowConfig struct {
	Width               int                   `json:"width"`
	Height              int                   `json:"height"`
	Title               string                `json:"title"`
	Icon                string                `json:"icon"`
	TitleBarHeight      float64               `json:"titleBarHeight,omitempty"`
	CornerRadius        int                   `json:"cornerRadius,omitempty"`
	Transparent         bool                  `json:"transparent,omitempty"`
	Decorations         window.DecorationMode `json:"decorations,omitempty"`
	Border              *bool                 `json:"border,omitempty"`
	Shadow              *bool                 `json:"shadow,omitempty"`
	Resizable           *bool                 `json:"resizable,omitempty"`
	ShowInTaskbar       *bool                 `json:"showInTaskbar,omitempty"`
	AlwaysOnTop         *bool                 `json:"alwaysOnTop,omitempty"`
	Vsync               *bool                 `json:"vsync"`
	FPS                 *int                  `json:"fps"`
	TPS                 *int                  `json:"tps"`
	RenderScale         float64               `json:"renderScale,omitempty"`
	MaxDevicePixelRatio float64               `json:"maxDevicePixelRatio,omitempty"`
	Antialias           *bool                 `json:"antialias,omitempty"`
	DRMDevice           string                `json:"drmDevice,omitempty"`
	DRMConnectorID      uint32                `json:"drmConnectorId,omitempty"`
}

type SizeConfig struct {
	Width  int                 `json:"width"`
	Height int                 `json:"height"`
	Mode   window.ViewportMode `json:"mode,omitempty"`
}

type FileResource struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	Stream bool   `json:"stream,omitempty"`
}

type SceneID string

type sceneCatalog struct {
	Entry  SceneID             `json:"entry"`
	Scenes []SceneCatalogEntry `json:"scenes"`
}

type SceneCatalogEntry struct {
	ID   SceneID `json:"id"`
	Path string  `json:"path"`
}

type sceneManifest struct {
	Prepared  bool            `json:"-"`
	SpawnPlan sceneSpawnPlan  `json:"-"`
	Nodes     []SceneNodeSpec `json:"nodes"`
}

type CoordinateSpace string

const (
	CoordinateSpaceWorld CoordinateSpace = "world"
	CoordinateSpaceUI    CoordinateSpace = "ui"
)

type SceneComponent string

const (
	SceneComponentSprite                 SceneComponent = "sprite"
	SceneComponentAnimation              SceneComponent = "animation"
	SceneComponentText                   SceneComponent = "text"
	SceneComponentHTML                   SceneComponent = "html"
	SceneComponentParticle               SceneComponent = "particle"
	SceneComponentTilemap                SceneComponent = "tilemap"
	SceneComponentSpine                  SceneComponent = "spine"
	SceneComponentModel                  SceneComponent = "model"
	SceneComponentCamera                 SceneComponent = "camera"
	SceneComponentAmbientLight           SceneComponent = "ambientlight"
	SceneComponentDirectionalLight       SceneComponent = "directionallight"
	SceneComponentPointLight             SceneComponent = "pointlight"
	SceneComponentSpotLight              SceneComponent = "spotlight"
	SceneComponentAreaLight              SceneComponent = "arealight"
	SceneComponentRigidBody2D            SceneComponent = "rigidbody2d"
	SceneComponentCircleCollider2D       SceneComponent = "circlecollider2d"
	SceneComponentBoxCollider2D          SceneComponent = "boxcollider2d"
	SceneComponentSegmentCollider2D      SceneComponent = "segmentcollider2d"
	SceneComponentRigidBody3D            SceneComponent = "rigidbody3d"
	SceneComponentTriangleMeshCollider3D SceneComponent = "trianglemeshcollider3d"
	SceneComponentConvexHullCollider3D   SceneComponent = "convexhullcollider3d"
	SceneComponentCompoundCollider3D     SceneComponent = "compoundcollider3d"
	SceneComponentLayout                 SceneComponent = "layout"
	SceneComponentWidget                 SceneComponent = "widget"
)

type SceneComponents struct {
	Sprite                 *SpriteComponentSpec        `json:"sprite,omitempty"`
	Animation              *AnimationComponentSpec     `json:"animation,omitempty"`
	Text                   *TextComponentSpec          `json:"text,omitempty"`
	HTML                   *ResourceComponentSpec      `json:"html,omitempty"`
	Particle               *ResourceComponentSpec      `json:"particle,omitempty"`
	Tilemap                *ResourceComponentSpec      `json:"tilemap,omitempty"`
	Spine                  *SpineComponentSpec         `json:"spine,omitempty"`
	Model                  *ResourceComponentSpec      `json:"model,omitempty"`
	Camera                 *Camera3DSpec               `json:"camera,omitempty"`
	AmbientLight           *AmbientLightSpec           `json:"ambientLight,omitempty"`
	DirectionalLight       *DirectionalLightSpec       `json:"directionalLight,omitempty"`
	PointLight             *PointLightSpec             `json:"pointLight,omitempty"`
	SpotLight              *SpotLightSpec              `json:"spotLight,omitempty"`
	AreaLight              *AreaLightSpec              `json:"areaLight,omitempty"`
	RigidBody2D            *RigidBody2DSpec            `json:"rigidBody2D,omitempty"`
	CircleCollider2D       *CircleCollider2DSpec       `json:"circleCollider2D,omitempty"`
	BoxCollider2D          *BoxCollider2DSpec          `json:"boxCollider2D,omitempty"`
	SegmentCollider2D      *SegmentCollider2DSpec      `json:"segmentCollider2D,omitempty"`
	RigidBody3D            *RigidBody3DSpec            `json:"rigidBody3D,omitempty"`
	TriangleMeshCollider3D *TriangleMeshCollider3DSpec `json:"triangleMeshCollider3D,omitempty"`
	ConvexHullCollider3D   *ConvexHullCollider3DSpec   `json:"convexHullCollider3D,omitempty"`
	CompoundCollider3D     *CompoundCollider3DSpec     `json:"compoundCollider3D,omitempty"`
}

// ScriptComponentSpec describes one user-authored script component attached
// to a scene node. Values are kept as JSON so the editor can round-trip
// fields discovered from the user's Go source without coupling the project
// package to that source package.
type ScriptComponentSpec struct {
	Name   string                     `json:"name"`
	Values map[string]json.RawMessage `json:"values,omitempty"`
}

func (c SceneComponents) IsZero() bool {
	return c == (SceneComponents{})
}

// Has reports whether the authored component set contains component.  Scene
// nodes are component compositions; no component is implicitly selected as a
// node kind.
func (c SceneComponents) Has(component SceneComponent) bool {
	switch component {
	case SceneComponentSprite:
		return c.Sprite != nil
	case SceneComponentAnimation:
		return c.Animation != nil
	case SceneComponentText:
		return c.Text != nil
	case SceneComponentHTML:
		return c.HTML != nil
	case SceneComponentParticle:
		return c.Particle != nil
	case SceneComponentTilemap:
		return c.Tilemap != nil
	case SceneComponentSpine:
		return c.Spine != nil
	case SceneComponentModel:
		return c.Model != nil
	case SceneComponentCamera:
		return c.Camera != nil
	case SceneComponentAmbientLight:
		return c.AmbientLight != nil
	case SceneComponentDirectionalLight:
		return c.DirectionalLight != nil
	case SceneComponentPointLight:
		return c.PointLight != nil
	case SceneComponentSpotLight:
		return c.SpotLight != nil
	case SceneComponentAreaLight:
		return c.AreaLight != nil
	case SceneComponentRigidBody2D:
		return c.RigidBody2D != nil
	case SceneComponentCircleCollider2D:
		return c.CircleCollider2D != nil
	case SceneComponentBoxCollider2D:
		return c.BoxCollider2D != nil
	case SceneComponentSegmentCollider2D:
		return c.SegmentCollider2D != nil
	case SceneComponentRigidBody3D:
		return c.RigidBody3D != nil
	case SceneComponentTriangleMeshCollider3D:
		return c.TriangleMeshCollider3D != nil
	case SceneComponentConvexHullCollider3D:
		return c.ConvexHullCollider3D != nil
	case SceneComponentCompoundCollider3D:
		return c.CompoundCollider3D != nil
	default:
		return false
	}
}

func (c SceneComponents) Has2D() bool {
	return c.Sprite != nil || c.Animation != nil || c.Text != nil || c.HTML != nil || c.Particle != nil || c.Tilemap != nil || c.Spine != nil
}

func (c SceneComponents) Has3D() bool {
	return c.Model != nil || c.Camera != nil || c.AmbientLight != nil || c.DirectionalLight != nil || c.PointLight != nil || c.SpotLight != nil || c.AreaLight != nil
}

func (c SceneComponents) HasPhysics2D() bool {
	return c.RigidBody2D != nil || c.CircleCollider2D != nil || c.BoxCollider2D != nil || c.SegmentCollider2D != nil
}

func (c SceneComponents) HasPhysics3D() bool {
	return c.RigidBody3D != nil || c.TriangleMeshCollider3D != nil || c.ConvexHullCollider3D != nil || c.CompoundCollider3D != nil
}

func (c SceneComponents) Empty() bool {
	return c.IsZero()
}

func (c SceneComponents) VisualsEmpty() bool {
	return !c.Has2D() && !c.Has3D()
}

type SpriteComponentSpec struct {
	Texture *TextureSpec     `json:"texture,omitempty"`
	Trail   *SpriteTrailSpec `json:"trail,omitempty"`
}

// AnimationComponentSpec attaches one immutable inline clip to a node.
type AnimationComponentSpec struct {
	Clip  AnimationClipSpec `json:"clip,omitzero"`
	Speed float32           `json:"speed,omitempty"`
}

type TextComponentSpec struct {
	Value         string      `json:"value,omitempty"`
	Font          string      `json:"font,omitempty"`
	FontSize      float64     `json:"fontSize,omitempty"`
	WrapMode      string      `json:"wrapMode,omitempty"`
	WrapWidth     float32     `json:"wrapWidth,omitempty"`
	LetterSpacing float32     `json:"letterSpacing,omitempty"`
	LineHeight    float32     `json:"lineHeight,omitempty"`
	TextAlignment string      `json:"textAlignment,omitempty"`
	Color         string      `json:"color,omitempty"`
	OutlineWidth  float32     `json:"outlineWidth,omitempty"`
	OutlineColor  string      `json:"outlineColor,omitempty"`
	MaxWidth      float32     `json:"maxWidth,omitempty"`
	Shadow        *TextShadow `json:"shadow,omitempty"`
}

type ResourceComponentSpec struct {
	Resource string `json:"resource"`
}

type SpineComponentSpec struct {
	Resource  string  `json:"resource"`
	Animation string  `json:"animation,omitempty"`
	Skin      string  `json:"skin,omitempty"`
	Loop      bool    `json:"loop"`
	Speed     float32 `json:"speed,omitempty"`
}

type ControlSpec struct {
	Text     string  `json:"text,omitempty"`
	Font     string  `json:"font,omitempty"`
	FontSize float64 `json:"fontSize,omitempty"`
}

func Components(values ...SceneComponent) SceneComponents {
	var components SceneComponents
	for _, component := range values {
		components.Add(component)
	}
	return components
}

// Add attaches one component without changing components already attached to
// the node.
func (c *SceneComponents) Add(component SceneComponent) {
	if c == nil {
		return
	}
	switch component {
	case SceneComponentSprite:
		if c.Sprite == nil {
			c.Sprite = &SpriteComponentSpec{}
		}
	case SceneComponentAnimation:
		if c.Animation == nil {
			c.Animation = &AnimationComponentSpec{Speed: 1}
		}
	case SceneComponentText:
		if c.Text == nil {
			c.Text = &TextComponentSpec{}
		}
	case SceneComponentHTML:
		if c.HTML == nil {
			c.HTML = &ResourceComponentSpec{}
		}
	case SceneComponentParticle:
		if c.Particle == nil {
			c.Particle = &ResourceComponentSpec{}
		}
	case SceneComponentTilemap:
		if c.Tilemap == nil {
			c.Tilemap = &ResourceComponentSpec{}
		}
	case SceneComponentSpine:
		if c.Spine == nil {
			c.Spine = &SpineComponentSpec{Loop: true, Speed: 1}
		}
	case SceneComponentModel:
		if c.Model == nil {
			c.Model = &ResourceComponentSpec{}
		}
	case SceneComponentCamera:
		if c.Camera == nil {
			value := NewCamera3DSpec()
			c.Camera = &value
		}
	case SceneComponentAmbientLight:
		if c.AmbientLight == nil {
			value := NewAmbientLightSpec()
			c.AmbientLight = &value
		}
	case SceneComponentDirectionalLight:
		if c.DirectionalLight == nil {
			value := NewDirectionalLightSpec()
			c.DirectionalLight = &value
		}
	case SceneComponentPointLight:
		if c.PointLight == nil {
			value := NewPointLightSpec()
			c.PointLight = &value
		}
	case SceneComponentSpotLight:
		if c.SpotLight == nil {
			value := NewSpotLightSpec()
			c.SpotLight = &value
		}
	case SceneComponentAreaLight:
		if c.AreaLight == nil {
			value := NewAreaLightSpec()
			c.AreaLight = &value
		}
	case SceneComponentRigidBody2D:
		if c.RigidBody2D == nil {
			value := NewRigidBody2DSpec()
			c.RigidBody2D = &value
		}
	case SceneComponentCircleCollider2D:
		if c.CircleCollider2D == nil {
			value := NewCircleCollider2DSpec()
			c.CircleCollider2D = &value
		}
	case SceneComponentBoxCollider2D:
		if c.BoxCollider2D == nil {
			value := NewBoxCollider2DSpec()
			c.BoxCollider2D = &value
		}
	case SceneComponentSegmentCollider2D:
		if c.SegmentCollider2D == nil {
			value := NewSegmentCollider2DSpec()
			c.SegmentCollider2D = &value
		}
	case SceneComponentRigidBody3D:
		if c.RigidBody3D == nil {
			value := NewRigidBody3DSpec()
			c.RigidBody3D = &value
		}
	case SceneComponentTriangleMeshCollider3D:
		if c.TriangleMeshCollider3D == nil {
			value := NewTriangleMeshCollider3DSpec()
			c.TriangleMeshCollider3D = &value
		}
	case SceneComponentConvexHullCollider3D:
		if c.ConvexHullCollider3D == nil {
			value := NewConvexHullCollider3DSpec()
			c.ConvexHullCollider3D = &value
		}
	case SceneComponentCompoundCollider3D:
		if c.CompoundCollider3D == nil {
			value := NewCompoundCollider3DSpec()
			c.CompoundCollider3D = &value
		}
	}
}

// HasComponent reports whether the node contains component.
func (s SceneNodeSpec) HasComponent(component SceneComponent) bool {
	switch component {
	case SceneComponentLayout:
		return s.Layout != nil
	case SceneComponentWidget:
		return s.Widget != nil
	}
	return s.Components.Has(component)
}

func (s SceneNodeSpec) Has3DComponents() bool {
	return s.Components.Has3D()
}

func (s SceneNodeSpec) IsEmpty() bool {
	return s.Components.Empty() && s.Control == nil && s.Layout == nil && s.Widget == nil
}

// SceneNodeSpec is the authoring representation of one entity in a scene.
// Runtime code operates on ecs.Entity values produced from these nodes.
type SceneNodeSpec struct {
	ID              string                `json:"id"`
	Components      SceneComponents       `json:"components,omitzero"`
	Scripts         []ScriptComponentSpec `json:"scripts,omitempty"`
	Control         *ControlSpec          `json:"control,omitempty"`
	Layout          *engineui.Layout      `json:"layout,omitempty"`
	Widget          *engineui.Widget      `json:"widget,omitempty"`
	CoordinateSpace CoordinateSpace       `json:"coordinateSpace,omitempty"`
	Order           int                   `json:"order,omitempty"`
	RenderLayers    []uint                `json:"renderLayers,omitempty"`
	TargetCamera    string                `json:"targetCamera,omitempty"`
	X               float32               `json:"x"`
	Y               float32               `json:"y"`
	Z               float32               `json:"z,omitempty"`
	Width           int                   `json:"width"`
	Height          int                   `json:"height"`
	Anchor          scene.Anchor          `json:"anchor"`
	ScaleX          float32               `json:"scaleX,omitempty"`
	ScaleY          float32               `json:"scaleY,omitempty"`
	Rotation        float32               `json:"rotation,omitempty"`
	RotationX       float32               `json:"rotationX,omitempty"`
	RotationY       float32               `json:"rotationY,omitempty"`
	RotationZ       float32               `json:"rotationZ,omitempty"`
	Alpha           float32               `json:"alpha,omitempty"`
	Visible         *bool                 `json:"visible,omitempty"`
	Disabled        bool                  `json:"disabled,omitempty"`
	PassThrough     bool                  `json:"passThrough,omitempty"`
	BlockInput      bool                  `json:"blockInput,omitempty"`
	ScaleZ          float32               `json:"scaleZ,omitempty"`
	Children        []SceneNodeSpec       `json:"children,omitempty"`
}

type Camera3DSpec struct {
	Fog         *FogSpec             `json:"fog,omitempty"`
	ToneMapping graphics.ToneMapping `json:"toneMapping,omitempty"`
	Quality     render3d.Quality     `json:"quality,omitempty"`
	FXAA        bool                 `json:"fxaa,omitempty"`
	SSAO        *SSAOSpec            `json:"ssao,omitempty"`
	SSR         *SSRSpec             `json:"ssr,omitempty"`
	Exposure    graphics.Exposure    `json:"exposure"`
	Environment *EnvironmentSpec     `json:"environment,omitempty"`
	Fov         float32              `json:"fov"`
	Near        float32              `json:"near"`
	ClearColor  string               `json:"clearColor"`
	Order       int                  `json:"order"`
	Viewport    CameraViewport3DSpec `json:"viewport"`
}

type CameraViewport3DSpec struct {
	X      float32 `json:"x"`
	Y      float32 `json:"y"`
	Width  float32 `json:"width"`
	Height float32 `json:"height"`
}

func NewCamera3DSpec() Camera3DSpec {
	return Camera3DSpec{
		Exposure:   graphics.NewExposure(),
		Fov:        float32(math.Pi / 4),
		Near:       0.1,
		ClearColor: "#06080aff",
	}
}

type DirectionalLightSpec struct {
	Shadows     *ShadowSpec `json:"shadows,omitempty"`
	Color       string      `json:"color"`
	Illuminance float32     `json:"illuminance"`
}

type EnvironmentSpec struct {
	Path       string  `json:"path"`
	Intensity  float32 `json:"intensity"`
	Rotation   float32 `json:"rotation,omitempty"`
	Exposure   float32 `json:"exposure,omitempty"`
	Background bool    `json:"background"`
}

type ShadowSpec struct {
	Resolution   int     `json:"resolution"`
	Radius       float32 `json:"radius"`
	Depth        float32 `json:"depth"`
	Bias         float32 `json:"bias"`
	NormalBias   float32 `json:"normalBias"`
	Cascades     int     `json:"cascades,omitempty"`
	Distance     float32 `json:"distance,omitempty"`
	SplitLambda  float32 `json:"splitLambda,omitempty"`
	CascadeBlend float32 `json:"cascadeBlend,omitempty"`
}

func (s ShadowSpec) Settings() render3d.ShadowSettings {
	return render3d.ShadowSettings{Enabled: true, Resolution: s.Resolution, Radius: s.Radius, Depth: s.Depth, Bias: s.Bias, NormalBias: s.NormalBias, Cascades: s.Cascades, Distance: s.Distance, SplitLambda: s.SplitLambda, CascadeBlend: s.CascadeBlend}
}

func NewShadowSpec() *ShadowSpec {
	s := render3d.NewShadowSettings()
	return &ShadowSpec{Resolution: s.Resolution, Radius: s.Radius, Depth: s.Depth, Bias: s.Bias, NormalBias: s.NormalBias, Cascades: s.Cascades, Distance: s.Distance, SplitLambda: s.SplitLambda, CascadeBlend: s.CascadeBlend}
}

func NewDirectionalLightSpec() DirectionalLightSpec {
	return DirectionalLightSpec{Color: "#ffffffff", Illuminance: 10000}
}

type AmbientLightSpec struct {
	Color      string  `json:"color"`
	Brightness float32 `json:"brightness"`
}

func NewAmbientLightSpec() AmbientLightSpec {
	return AmbientLightSpec{Color: "#ffffffff", Brightness: 800}
}

type TextShadow struct {
	OffsetX float32 `json:"offsetX"`
	OffsetY float32 `json:"offsetY"`
	Color   string  `json:"color"`
}

type Resources struct {
	modShaders        *resourceCache[ShaderSources, *modShaderResource]
	modModels         *resourceCache[string, modelResource]
	modClips          map[*AnimationComponentSpec]*scene.AnimationClip
	modHTMLBases      map[string]string
	modRigs           map[string]*ModelRig
	modParticles      map[string]*TextureSpec
	source            iofs.FS
	baseDir           string
	textures          *resourceCache[textureResourceKey, *graphics.Texture]
	environments      *resourceCache[resource.ContentID, environmentResource]
	atlasMetadata     *resourceCache[resource.ContentID, *graphics.AtlasMetadata]
	atlases           *resourceCache[atlasResourceKey, *graphics.Atlas]
	fonts             *resourceCache[string, fontResource]
	videos            optionalResourceStore
	html              optionalResourceStore
	spines            spineResourceStore
	shaders           *resourceCache[string, shaderResource]
	material2DShaders *resourceCache[string, *graphics.Material2DShader]
	meshShaders       *resourceCache[string, *graphics.MeshShader]
	namedMu           sync.RWMutex
	named             map[string]any
	imageDecoder      ImageDecoder
	fontDecoder       FontDecoder
	iconDecoder       window.IconDecoder
	prefabSpecs       []PrefabResourceSpec
}

type textureResourceKey struct {
	content resource.ContentID
	options graphics.TextureOptions
}

type atlasResourceKey struct {
	metadata resource.ContentID
	texture  textureResourceKey
}

func retryableResourceError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, resource.ErrPackageNotLoaded) {
		return true
	}
	type retryable interface{ Retryable() bool }
	var retryableError retryable
	if errors.As(err, &retryableError) {
		return retryableError.Retryable()
	}
	type temporary interface{ Temporary() bool }
	var temporaryError temporary
	if errors.As(err, &temporaryError) {
		return temporaryError.Temporary()
	}
	return false
}

const (
	projectFileName      = "project.json"
	sceneCatalogFileName = "scenes.json"
	resourceFileName     = "resources.json"
)

type bundleLoader interface {
	loadResources(iofs.FS, string, string, ResourceFeature) (*Resources, map[string]ParticleSpec, map[string]TilemapSpec, map[string]ModelSpec, map[string]SpineSpec, error)
	componentBuilder() sceneComponentBuilder
	componentClearer() sceneComponentClearer
	uiSystems() sceneUISync
}

// LoadProfile is the compile-time boundary for project resource and scene
// support. Profiles intentionally use unexported methods so applications can
// select only engine-owned implementations through generic APIs. Unlike the
// full loader, a profile does not return indexes for unsupported resource
// families, keeping those types out of its static call graph.
type LoadProfile interface {
	loadProfileResources(iofs.FS, string, string, ResourceFeature) (*Resources, error)
	componentBuilder() sceneComponentBuilder
	componentClearer() sceneComponentClearer
	uiSystems() sceneUISync
}

type allFeaturesLoader struct{}

func (allFeaturesLoader) loadResources(source iofs.FS, projectDir, path string, feature ResourceFeature) (*Resources, map[string]ParticleSpec, map[string]TilemapSpec, map[string]ModelSpec, map[string]SpineSpec, error) {
	return loadResources(source, projectDir, path, feature)
}

func (allFeaturesLoader) componentBuilder() sceneComponentBuilder { return buildAllSceneComponents }
func (allFeaturesLoader) componentClearer() sceneComponentClearer {
	return clearAuthoredSceneComponents
}
func (allFeaturesLoader) uiSystems() sceneUISync { return fullSceneUISync() }

// configuredLoader composes resource validation, scene components, and UI
// synchronization from an explicit EngineConfig.
type configuredLoader struct {
	Config  EngineConfig
	Feature ResourceFeature
}

func (loader configuredLoader) loadResources(source iofs.FS, projectDir, path string, _ ResourceFeature) (*Resources, map[string]ParticleSpec, map[string]TilemapSpec, map[string]ModelSpec, map[string]SpineSpec, error) {
	if err := loader.Config.Validate(); err != nil {
		return nil, nil, nil, nil, nil, err
	}
	if err := validateConfiguredResourceManifest(source, path, loader.Config); err != nil {
		return nil, nil, nil, nil, nil, err
	}
	return loadResources(source, projectDir, path, loader.Feature)
}

func (configuredLoader) componentBuilder() sceneComponentBuilder {
	return buildConfiguredSceneComponents
}

func (configuredLoader) componentClearer() sceneComponentClearer { return clearAuthoredSceneComponents }

func (loader configuredLoader) uiSystems() sceneUISync {
	sync := fullSceneUISync()
	switch {
	case loader.Config.HasFeature(FeatureUIMarkdown), loader.Config.HasFeature(FeatureUIHTML):
	case loader.Config.HasFeature(FeatureUIWidgets):
		sync.beforeSync, sync.afterSync = engineui.SyncWith[engineui.Widgets], engineui.SyncAfterLayoutWith[engineui.Widgets]
	case loader.Config.HasFeature(FeatureUIControls):
		sync.beforeSync, sync.afterSync = engineui.SyncWith[engineui.AuthoredControls], engineui.SyncAfterLayoutWith[engineui.AuthoredControls]
	default:
		sync.beforeSync, sync.afterSync = engineui.Sync, engineui.SyncAfterLayout
	}
	return sync
}

func validateConfiguredResourceManifest(source iofs.FS, path string, config EngineConfig) error {
	exists, err := projectFileExists(source, path)
	if err != nil || !exists {
		return err
	}
	manifest, err := readJSON[resourceManifest](source, path)
	if err != nil {
		return err
	}
	unsupported := func(enabled ResourceID, count int, name string) error {
		if count > 0 && !config.HasResource(enabled) {
			return fmt.Errorf("%s contains disabled %s resources", pathpkg.Base(path), name)
		}
		return nil
	}
	checks := [...]struct {
		id    ResourceID
		count int
		name  string
	}{
		{ResourceFonts, len(manifest.Fonts), "font"},
		{ResourceAudio, len(manifest.Audio), "audio"},
		{ResourceVideos, len(manifest.Videos), "video"},
		{ResourceParticles, len(manifest.Effects), "particle"},
		{ResourceTilemaps, len(manifest.Tilemaps), "tilemap"},
		{ResourceModels, len(manifest.Models), "model"},
		{ResourceSpine, len(manifest.Spines), "Spine"},
		{ResourceShaders, len(manifest.Shaders), "shader"},
		{ResourceHTML, len(manifest.HTML), "html"},
	}
	for _, check := range checks {
		if err := unsupported(check.id, check.count, check.name); err != nil {
			return err
		}
	}
	return nil
}

func Load(source iofs.FS, root string) (*Bundle, error) {
	return loadBundle(source, root, allFeaturesLoader{})
}

// LoadConfiguredWith loads a project using explicit resource and runtime
// feature selections.
func LoadConfiguredWith[F ResourceFeature](source iofs.FS, root string, config EngineConfig) (*Bundle, error) {
	var feature F
	return loadBundle(source, root, configuredLoader{Config: config, Feature: feature})
}

func loadProfileBundle[P LoadProfile](source iofs.FS, root string, profile P, feature ResourceFeature) (*Bundle, error) {
	if source == nil {
		return nil, fmt.Errorf("project resource source is nil")
	}
	root = pathpkg.Clean(root)
	if root != "." && (!iofs.ValidPath(root) || strings.ContainsRune(root, '\\')) {
		return nil, fmt.Errorf("invalid project root %q", root)
	}
	manifest, manifestDir, err := loadGameManifest(source, root)
	if err != nil {
		return nil, err
	}
	resources, err := profile.loadProfileResources(source, root, pathpkg.Join(manifestDir, resourceFileName), feature)
	if err != nil {
		return nil, err
	}
	resourcesOwned := true
	defer func() {
		if resourcesOwned {
			_ = resources.Dispose()
		}
	}()
	prefabPlansByID, prefabIDs, err := loadPrefabResources(source, root, resources.prefabSpecs)
	if err != nil {
		return nil, err
	}
	entrySceneID, scenePlansByID, sceneIDs, err := loadSceneCatalog(source, manifestDir)
	if err != nil {
		return nil, err
	}
	project := &Bundle{
		resources:            resources,
		engineConfig:         manifest.Engine,
		components:           profile.componentBuilder(),
		clearComponents:      profile.componentClearer(),
		uiSync:               profile.uiSystems(),
		appConfig:            AppConfig{Window: manifest.Window, Viewport: manifest.Viewport},
		entrySceneID:         entrySceneID,
		scenePlansByID:       scenePlansByID,
		sceneIDs:             sceneIDs,
		prefabPlansByID:      prefabPlansByID,
		prefabIDs:            prefabIDs,
		particleSpecs:        nil,
		tilemapSpecs:         nil,
		modelSpecs:           nil,
		modelResources:       nil,
		spineSpecs:           nil,
		inlineAnimationClips: newResourceCache[string, *scene.AnimationClip](0),
	}
	if err := project.prepareInlineAnimationPlans(); err != nil {
		return nil, err
	}
	if manifest.ClearColor != "" {
		clr, err := resources.ParseColor(manifest.ClearColor, 1)
		if err != nil {
			return nil, fmt.Errorf("%s clearColor: %w", projectFileName, err)
		}
		project.clearColor, project.hasClear = clr, true
	}
	iconPath, err := resolveWindowIconPath(manifestDir, manifest.Window.Icon)
	if err != nil {
		return nil, fmt.Errorf("%s window.icon: %w", projectFileName, err)
	}
	if iconPath != "" {
		project.windowIcon, err = readProjectFile(source, iconPath)
		if err != nil {
			return nil, fmt.Errorf("read window icon %s: %w", iconPath, err)
		}
	}
	resourcesOwned = false
	return project, nil
}

func loadBundle(source iofs.FS, root string, loader bundleLoader) (*Bundle, error) {
	if source == nil {
		return nil, fmt.Errorf("project resource source is nil")
	}
	root = pathpkg.Clean(root)
	if root != "." && (!iofs.ValidPath(root) || strings.ContainsRune(root, '\\')) {
		return nil, fmt.Errorf("invalid project root %q", root)
	}
	manifest, manifestDir, err := loadGameManifest(source, root)
	if err != nil {
		return nil, err
	}

	resources, particleSpecs, tilemapSpecs, modelSpecs, spineSpecs, err := loader.loadResources(source, root, pathpkg.Join(manifestDir, resourceFileName), nil)
	if err != nil {
		return nil, err
	}
	resourcesOwned := true
	defer func() {
		if resourcesOwned {
			_ = resources.Dispose()
		}
	}()
	prefabPlansByID, prefabIDs, err := loadPrefabResources(source, root, resources.prefabSpecs)
	if err != nil {
		return nil, err
	}

	entrySceneID, scenePlansByID, sceneIDs, err := loadSceneCatalog(source, manifestDir)
	if err != nil {
		return nil, err
	}
	project := &Bundle{
		resources:       resources,
		engineConfig:    manifest.Engine,
		components:      loader.componentBuilder(),
		clearComponents: loader.componentClearer(),
		uiSync:          loader.uiSystems(),
		appConfig: AppConfig{
			Window:   manifest.Window,
			Viewport: manifest.Viewport,
		},
		entrySceneID:         entrySceneID,
		scenePlansByID:       scenePlansByID,
		sceneIDs:             sceneIDs,
		prefabPlansByID:      prefabPlansByID,
		prefabIDs:            prefabIDs,
		particleSpecs:        particleSpecs,
		tilemapSpecs:         tilemapSpecs,
		modelSpecs:           modelSpecs,
		modelResources:       newResourceCache[string, modelResource](len(modelSpecs)),
		spineSpecs:           spineSpecs,
		inlineAnimationClips: newResourceCache[string, *scene.AnimationClip](0),
	}
	if configured, ok := loader.(configuredLoader); ok {
		project.engineConfig = configured.Config
	}
	if err := project.prepareInlineAnimationPlans(); err != nil {
		return nil, err
	}

	if manifest.ClearColor != "" {
		clr, err := resources.ParseColor(manifest.ClearColor, 1)
		if err != nil {
			return nil, fmt.Errorf("%s clearColor: %w", projectFileName, err)
		}
		project.clearColor = clr
		project.hasClear = true
	}

	iconPath, err := resolveWindowIconPath(manifestDir, manifest.Window.Icon)
	if err != nil {
		return nil, fmt.Errorf("%s window.icon: %w", projectFileName, err)
	}
	if iconPath != "" {
		project.windowIcon, err = readProjectFile(source, iconPath)
		if err != nil {
			return nil, fmt.Errorf("read window icon %s: %w", iconPath, err)
		}
	}

	resourcesOwned = false
	return project, nil
}

func (b *Bundle) prepareInlineAnimationPlans() error {
	if b == nil || b.resources == nil {
		return nil
	}
	for id, plan := range b.scenePlansByID {
		for index := range plan.Nodes {
			node := &plan.Nodes[index]
			component := node.Spec.Components.Animation
			if component == nil || !component.Clip.HasContent() {
				continue
			}
			key := "scene/" + string(id) + "/" + node.Spec.ID
			if err := component.Clip.Validate(); err != nil {
				return fmt.Errorf("scene %q node %q: %w", id, node.Spec.ID, err)
			}
			spec := component.Clip.Clone()
			if err := b.inlineAnimationClips.register(key, func() (*scene.AnimationClip, error) {
				return b.resources.compileAnimationClipSpec(spec, key)
			}, nil); err != nil {
				return fmt.Errorf("scene %q node %q animation cache: %w", id, node.Spec.ID, err)
			}
			node.AnimationKey = key
		}
		b.scenePlansByID[id] = plan
	}
	for id, prefab := range b.prefabPlansByID {
		if prefab == nil {
			continue
		}
		for index := range prefab.plan.Nodes {
			node := &prefab.plan.Nodes[index]
			component := node.Spec.Components.Animation
			if component == nil || !component.Clip.HasContent() {
				continue
			}
			key := "prefab/" + string(id) + "/" + node.Spec.ID
			if err := component.Clip.Validate(); err != nil {
				return fmt.Errorf("prefab %q node %q: %w", id, node.Spec.ID, err)
			}
			spec := component.Clip.Clone()
			if err := b.inlineAnimationClips.register(key, func() (*scene.AnimationClip, error) {
				return b.resources.compileAnimationClipSpec(spec, key)
			}, nil); err != nil {
				return fmt.Errorf("prefab %q node %q animation cache: %w", id, node.Spec.ID, err)
			}
			node.AnimationKey = key
		}
	}
	return nil
}

func LoadDefault() (*Bundle, error) {
	source, root, ok := resource.Default()
	if !ok {
		return nil, fmt.Errorf("default project resource source is not mounted")
	}
	return Load(source, root)
}

func (b *Bundle) Resources() *Resources {
	return b.resources
}

// CompileAnimationClip compiles an inline authoring clip against this
// bundle's texture resources. The returned clip is independent of any scene
// instance and can be shared by editor previews and prefab instances.
func (b *Bundle) CompileAnimationClip(spec AnimationClipSpec) (*scene.AnimationClip, error) {
	if b == nil || b.resources == nil {
		return nil, fmt.Errorf("project bundle is nil")
	}
	return b.resources.compileAnimationClipSpec(spec, "inline")
}

func (b *Bundle) Dispose() error {
	if b == nil {
		return nil
	}
	errList := make([]error, 0, 2)
	if err := b.inlineAnimationClips.disposeResources(); err != nil {
		errList = append(errList, fmt.Errorf("dispose inline animations: %w", err))
	}
	b.resourceSpecMu.Lock()
	disposeResource("model", b.modelResources, &errList)
	b.resourceSpecMu.Unlock()
	if b.resources != nil {
		if err := b.resources.Dispose(); err != nil {
			errList = append(errList, err)
		}
	}
	return errors.Join(errList...)
}

func (b *Bundle) AppConfig() AppConfig {
	return b.appConfig
}

func (b *Bundle) Has3D() bool {
	if b == nil {
		return false
	}
	for _, plan := range b.scenePlansByID {
		if plan.Has3D {
			return true
		}
	}
	for _, prefab := range b.prefabPlansByID {
		if prefab.plan.Has3D {
			return true
		}
	}
	return false
}

func (b *Bundle) HasUI() bool {
	if b == nil {
		return false
	}
	for _, plan := range b.scenePlansByID {
		if plan.HasUI {
			return true
		}
	}
	for _, prefab := range b.prefabPlansByID {
		if prefab.plan.HasUI {
			return true
		}
	}
	return false
}

func (b *Bundle) EntrySceneID() SceneID {
	if b == nil {
		return ""
	}
	return b.entrySceneID
}

func (b *Bundle) SceneIDs() []SceneID {
	if b == nil {
		return nil
	}
	return slices.Clone(b.sceneIDs)
}

// PrefabIDs returns every prefab registered by resources.json.
func (b *Bundle) PrefabIDs() []PrefabID {
	if b == nil {
		return nil
	}
	return slices.Clone(b.prefabIDs)
}

func (b *Bundle) WindowIcon() []byte {
	return slices.Clone(b.windowIcon)
}

// WindowIconDecoder returns the decoder selected by the project's resource
// feature. A nil value means window icons are not configured.
func (b *Bundle) WindowIconDecoder() window.IconDecoder {
	if b == nil || b.resources == nil {
		return nil
	}
	return b.resources.iconDecoder
}

func (b *Bundle) WindowSize() (int, int, bool) {
	if b.appConfig.Window.Width <= 0 || b.appConfig.Window.Height <= 0 {
		return 0, 0, false
	}
	return b.appConfig.Window.Width, b.appConfig.Window.Height, true
}

func (b *Bundle) ViewportSize() (int, int, bool) {
	if b.appConfig.Viewport.Width > 0 && b.appConfig.Viewport.Height > 0 {
		return b.appConfig.Viewport.Width, b.appConfig.Viewport.Height, true
	}
	if b.appConfig.Window.Width > 0 && b.appConfig.Window.Height > 0 {
		return b.appConfig.Window.Width, b.appConfig.Window.Height, true
	}
	return 0, 0, false
}

func (b *Bundle) ClearColor() (graphics.ColorRGBA, bool) {
	return b.clearColor, b.hasClear
}

func (b *Bundle) textureFor(spec SceneNodeSpec) (*graphics.Texture, error) {
	texture, err := b.resources.texture(spec.Components.Sprite.Texture)
	if err != nil {
		return nil, fmt.Errorf("game object %q texture: %w", spec.ID, err)
	}
	return texture, nil
}

func (b *Bundle) parseColor(value string, alpha float32) (graphics.ColorRGBA, error) {
	return b.resources.ParseColor(value, alpha)
}

func (r *Resources) Spine(id string) (*spineruntime.Resource, error) {
	if r == nil {
		return nil, fmt.Errorf("resources are nil")
	}
	spines, ok := r.spines.(*resourceCache[string, *spineruntime.Resource])
	if !ok {
		return nil, fmt.Errorf("spine resources are not configured")
	}
	return loadNamedResource(id, "spine", spines)
}

func loadResources(source iofs.FS, projectDir, path string, feature ResourceFeature) (*Resources, map[string]ParticleSpec, map[string]TilemapSpec, map[string]ModelSpec, map[string]SpineSpec, error) {
	exists, err := projectFileExists(source, path)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	if !exists {
		store := newResources()
		store.source = source
		store.baseDir = projectDir
		if err := configureResourceFeature(store, source, projectDir, feature, ResourceEntries{}); err != nil {
			return nil, nil, nil, nil, nil, err
		}
		return store, map[string]ParticleSpec{}, map[string]TilemapSpec{}, map[string]ModelSpec{}, map[string]SpineSpec{}, nil
	}

	manifest, err := readJSON[resourceManifest](source, path)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	prefabSpecs, err := normalizePrefabResourceSpecs(manifest.Prefabs)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("%s: %w", pathpkg.Base(path), err)
	}
	particleSpecs, err := indexManifestItems(path, manifest.Effects, "effect", func(spec ParticleSpec) string {
		return spec.ID
	}, func(spec ParticleSpec) error {
		if err := spec.Texture.validate(false); err != nil {
			return fmt.Errorf("%s particle %q texture %w", pathpkg.Base(path), spec.ID, err)
		}
		return nil
	})
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	for i := range manifest.Tilemaps {
		if manifest.Prepared {
			if err := manifest.Tilemaps[i].Expand(); err != nil {
				return nil, nil, nil, nil, nil, fmt.Errorf("%s: %w", pathpkg.Base(path), err)
			}
			continue
		}
		manifest.Tilemaps[i].Normalize()
	}
	tilemapSpecs, err := indexManifestItems(path, manifest.Tilemaps, "tilemap", func(spec TilemapSpec) string {
		return spec.ID
	}, func(spec TilemapSpec) error {
		return spec.validate()
	})
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	modelSpecs, err := indexManifestItems(path, manifest.Models, "model", func(spec ModelSpec) string { return spec.ID }, func(spec ModelSpec) error {
		if err := spec.validate(); err != nil {
			return fmt.Errorf("%s: %w", pathpkg.Base(path), err)
		}
		return nil
	})
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	spineSpecs, err := indexManifestItems(path, manifest.Spines, "spine", func(spec SpineSpec) string { return spec.ID }, func(spec SpineSpec) error {
		if err := spec.validate(); err != nil {
			return fmt.Errorf("%s: %w", pathpkg.Base(path), err)
		}
		return nil
	})
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	shaderSpecs, err := indexManifestItems(path, manifest.Shaders, "shader", func(spec ShaderSpec) string { return spec.ID }, func(spec ShaderSpec) error {
		if err := spec.Validate(); err != nil {
			return fmt.Errorf("%s: %w", pathpkg.Base(path), err)
		}
		return nil
	})
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	for _, model := range manifest.Models {
		id := strings.TrimSpace(model.Material.Shader)
		if id != "" {
			if _, ok := shaderSpecs[id]; !ok {
				return nil, nil, nil, nil, nil, fmt.Errorf("%s: model %q shader resource %q not found", pathpkg.Base(path), model.ID, id)
			}
		}
	}

	store := newResources()
	store.source = source
	store.baseDir = projectDir
	store.fonts = newResourceCache[string, fontResource](len(manifest.Fonts))
	videos := newResourceCache[string, *Video](len(manifest.Videos))
	store.videos = videos
	html := newResourceCache[string, string](len(manifest.HTML))
	store.html = html
	spines := newResourceCache[string, *spineruntime.Resource](len(manifest.Spines))
	store.spines = spines
	store.shaders = newResourceCache[string, shaderResource](len(shaderSpecs))
	store.material2DShaders = newResourceCache[string, *graphics.Material2DShader](len(shaderSpecs))
	store.meshShaders = newResourceCache[string, *graphics.MeshShader](len(shaderSpecs))
	store.prefabSpecs = prefabSpecs
	if len(manifest.Audio) > 0 && feature == nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("%s contains audio resources but no audio resource feature is installed", pathpkg.Base(path))
	}
	if err := configureResourceFeature(store, source, projectDir, feature, ResourceEntries{Audio: manifest.Audio}); err != nil {
		return nil, nil, nil, nil, nil, err
	}

	for _, item := range manifest.Fonts {
		if item.ID == "" || item.Path == "" {
			return nil, nil, nil, nil, nil, fmt.Errorf("resources.json font entry requires id and path")
		}
		resourcePath, err := ResolveResourcePath(projectDir, item.Path)
		if err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("font %q path: %w", item.ID, err)
		}
		item := item
		if err := store.fonts.register(item.ID, func() (fontResource, error) {
			return store.loadFont(resourcePath, item.Path)
		}, fontResource.dispose); err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("register font %q: %w", item.ID, err)
		}
	}

	for _, item := range manifest.Videos {
		if item.ID == "" || item.Path == "" {
			return nil, nil, nil, nil, nil, fmt.Errorf("resources.json video entry requires id and path")
		}
		resourcePath, err := ResolveResourcePath(projectDir, item.Path)
		if err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("video %q path: %w", item.ID, err)
		}
		item := item
		if err := videos.register(item.ID, func() (*Video, error) {
			blob, err := openProjectBlob(source, resourcePath)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", item.Path, err)
			}
			return newVideo(blob), nil
		}, (*Video).Close); err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("register video %q: %w", item.ID, err)
		}
	}
	for _, item := range manifest.HTML {
		if item.ID == "" || item.Path == "" {
			return nil, nil, nil, nil, nil, fmt.Errorf("resources.json html entry requires id and path")
		}
		resourcePath, err := ResolveResourcePath(projectDir, item.Path)
		if err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("html %q path: %w", item.ID, err)
		}
		item := item
		if err := html.register(item.ID, func() (string, error) {
			data, err := readProjectFile(source, resourcePath)
			if err != nil {
				return "", fmt.Errorf("read %s: %w", item.Path, err)
			}
			return string(data), nil
		}, nil); err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("register html %q: %w", item.ID, err)
		}
	}
	for _, spec := range manifest.Spines {
		spec := spec
		if err := spines.register(spec.ID, func() (*spineruntime.Resource, error) {
			return store.loadSpine(spec)
		}, nil); err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("register spine %q: %w", spec.ID, err)
		}
	}
	for id, spec := range shaderSpecs {
		id, spec := id, spec
		if err := store.shaders.register(id, func() (shaderResource, error) {
			return spec.load(source, projectDir)
		}, shaderResource.dispose); err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("register shader %q: %w", id, err)
		}
		if err := store.material2DShaders.register(id, func() (*graphics.Material2DShader, error) {
			descriptor, err := store.Shader(id)
			if err != nil {
				return nil, err
			}
			return graphics.NewMaterial2DShader(descriptor)
		}, (*graphics.Material2DShader).Dispose); err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("register Material2D shader %q: %w", id, err)
		}
		if err := store.meshShaders.register(id, func() (*graphics.MeshShader, error) {
			descriptor, err := store.Shader(id)
			if err != nil {
				return nil, err
			}
			return graphics.NewMeshShader(descriptor)
		}, (*graphics.MeshShader).Dispose); err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("register mesh shader %q: %w", id, err)
		}
	}

	return store, particleSpecs, tilemapSpecs, modelSpecs, spineSpecs, nil
}

func newResources() *Resources {
	return &Resources{
		modShaders:        newResourceCache[ShaderSources, *modShaderResource](0),
		textures:          newResourceCache[textureResourceKey, *graphics.Texture](0),
		environments:      newResourceCache[resource.ContentID, environmentResource](0),
		atlasMetadata:     newResourceCache[resource.ContentID, *graphics.AtlasMetadata](0),
		atlases:           newResourceCache[atlasResourceKey, *graphics.Atlas](0),
		fonts:             newResourceCache[string, fontResource](0),
		shaders:           newResourceCache[string, shaderResource](0),
		material2DShaders: newResourceCache[string, *graphics.Material2DShader](0),
		meshShaders:       newResourceCache[string, *graphics.MeshShader](0),
		named:             make(map[string]any),
		fontDecoder:       builtinFontDecoder{},
	}
}

// Shader returns the descriptor for a named shader resource. The source files
// are read only on first access.
func (r *Resources) Shader(id string) (render.ShaderDescriptor, error) {
	value, err := loadNamedResource(id, "shader", r.shaders)
	return value.descriptor, err
}

func loadNamedResource[T any](id, kind string, resources *resourceCache[string, T]) (T, error) {
	id = strings.TrimSpace(id)
	value, exists, err := resources.get(id)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("load %s resource %q: %w", kind, id, err)
	}
	if !exists {
		var zero T
		return zero, fmt.Errorf("%s resource %q not found", kind, id)
	}
	return value, nil
}

func (r *Resources) Material2DShader(id string) (*graphics.Material2DShader, error) {
	return loadNamedResource(id, "Material2D shader", r.material2DShaders)
}

func (r *Resources) MeshShader(id string) (*graphics.MeshShader, error) {
	if r == nil || r.meshShaders == nil {
		return nil, fmt.Errorf("mesh shader resources are not configured")
	}
	return loadNamedResource(id, "mesh shader", r.meshShaders)
}

func (r *Resources) loadSpine(spec SpineSpec) (*spineruntime.Resource, error) {
	skeletonPath, err := ResolveResourcePath(r.baseDir, spec.Skeleton)
	if err != nil {
		return nil, fmt.Errorf("resolve skeleton: %w", err)
	}
	skeletonBlob, err := openProjectBlob(r.source, skeletonPath)
	if err != nil {
		return nil, fmt.Errorf("load skeleton: %w", err)
	}
	defer skeletonBlob.Close()
	skeletonData, err := skeletonBlob.Bytes()
	if err != nil {
		return nil, fmt.Errorf("load skeleton: %w", err)
	}
	var atlasData []byte
	var atlasBlob *resource.Blob
	if !spineruntime.IsPreprocessed(skeletonData) {
		atlasPath, resolveErr := ResolveResourcePath(r.baseDir, spec.Atlas)
		if resolveErr != nil {
			return nil, fmt.Errorf("resolve atlas: %w", resolveErr)
		}
		atlasBlob, err = openProjectBlob(r.source, atlasPath)
		if err != nil {
			return nil, fmt.Errorf("load atlas: %w", err)
		}
		defer atlasBlob.Close()
		atlasData, err = atlasBlob.Bytes()
		if err != nil {
			return nil, fmt.Errorf("load atlas: %w", err)
		}
	}
	texturePath, err := ResolveResourcePath(r.baseDir, spec.Texture)
	if err != nil {
		return nil, fmt.Errorf("resolve texture: %w", err)
	}
	if r.imageDecoder == nil {
		return nil, fmt.Errorf("spine texture %q requires an injected image decoder", spec.Texture)
	}
	return spineruntime.LoadWithTexture(skeletonData, atlasData, graphics.TextureOptions{
		PremultiplyAlpha: true,
		Usage:            graphics.ResourceUsageDefault,
	}, func(options graphics.TextureOptions) (*graphics.Texture, error) {
		texture, _, err := r.textureFromPath(texturePath, options)
		if err != nil {
			return nil, fmt.Errorf("load texture: %w", err)
		}
		return texture, nil
	})
}

func (r *Resources) texture(spec *TextureSpec) (*graphics.Texture, error) {
	if spec == nil {
		return nil, nil
	}
	if err := spec.validate(true); err != nil {
		return nil, err
	}
	switch {
	case spec.Color != "":
		color, err := r.ParseColor(spec.Color, 1)
		if err != nil {
			return nil, err
		}
		return graphics.CreateTextureByColor(color), nil
	case spec.Image != "":
		options := graphics.TextureOptions{Mipmap: spec.Mipmap, Usage: graphics.ResourceUsageGPU}
		imagePath, err := ResolveResourcePath(r.baseDir, spec.Image)
		if err != nil {
			return nil, fmt.Errorf("resolve image %q: %w", spec.Image, err)
		}
		if r.imageDecoder == nil {
			return nil, fmt.Errorf("image resource %q requires an injected image decoder", spec.Image)
		}
		texture, _, err := r.textureFromPath(imagePath, options)
		if err != nil {
			return nil, fmt.Errorf("load image %q: %w", spec.Image, err)
		}
		return texture, nil
	default:
		atlas, err := r.AtlasWithOptions(spec.Atlas, graphics.TextureOptions{Mipmap: spec.Mipmap, Usage: graphics.ResourceUsageGPU})
		if err != nil {
			return nil, err
		}
		texture := atlas.SubTexture(spec.Frame)
		if texture == nil {
			return nil, fmt.Errorf("atlas frame %q not found in %q", spec.Frame, spec.Atlas)
		}
		return texture, nil
	}
}

func (r *Resources) textureFromPath(path string, options graphics.TextureOptions) (*graphics.Texture, textureResourceKey, error) {
	if r == nil || r.imageDecoder == nil {
		return nil, textureResourceKey{}, fmt.Errorf("image decoder is not configured")
	}
	blob, err := openProjectBlob(r.source, path)
	if err != nil {
		return nil, textureResourceKey{}, err
	}
	defer blob.Close()
	key := textureResourceKey{content: blob.ContentID(), options: options}
	texture, err := r.textures.load(key, func() (*graphics.Texture, error) {
		data, err := blob.Bytes()
		if err != nil {
			return nil, err
		}
		return r.imageDecoder.DecodeImage(data, path, options)
	}, (*graphics.Texture).Dispose)
	return texture, key, err
}

func (r *Resources) ParseColor(value string, alpha float32) (graphics.ColorRGBA, error) {
	return parseColorValue(value, alpha)
}

func (r *Resources) Font(id string) (*onixtext.Font, error) {
	value, err := loadNamedResource(id, "font", r.fonts)
	return value.font, err
}

func (r *Resources) Video(id string) (*Video, error) {
	if r == nil || r.videos == nil {
		return nil, fmt.Errorf("video resources are not configured")
	}
	videos, ok := r.videos.(*resourceCache[string, *Video])
	if !ok {
		return nil, fmt.Errorf("video resources are not configured")
	}
	return loadNamedResource(id, "video", videos)
}

// HTML returns the source of a named HTML resource, loaded lazily on first use.
func (r *Resources) HTML(id string) (string, error) {
	if r == nil || r.html == nil {
		return "", fmt.Errorf("html resources are not configured")
	}
	html, ok := r.html.(*resourceCache[string, string])
	if !ok {
		return "", fmt.Errorf("html resources are not configured")
	}
	return loadNamedResource(id, "html", html)
}

func (r *Resources) Atlas(path string) (*graphics.Atlas, error) {
	return r.AtlasWithOptions(path, graphics.TextureOptions{Usage: graphics.ResourceUsageGPU})
}

func (r *Resources) AtlasWithOptions(path string, options graphics.TextureOptions) (*graphics.Atlas, error) {
	switch strings.ToLower(pathpkg.Ext(path)) {
	case ".atlas", ".json":
	default:
		return nil, fmt.Errorf("atlas path %q must include the .atlas or .json extension", path)
	}
	metaPath, err := ResolveResourcePath(r.baseDir, path)
	if err != nil {
		return nil, fmt.Errorf("resolve atlas metadata %q: %w", path, err)
	}
	options.Usage = graphics.ResourceUsageGPU
	metaBlob, err := openProjectBlob(r.source, metaPath)
	if err != nil {
		return nil, fmt.Errorf("load atlas metadata %q: %w", path, err)
	}
	defer metaBlob.Close()
	metadata, err := r.atlasMetadata.load(metaBlob.ContentID(), func() (*graphics.AtlasMetadata, error) {
		data, err := metaBlob.Bytes()
		if err != nil {
			return nil, err
		}
		return graphics.ParseAtlas(data)
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("decode atlas metadata %q: %w", path, err)
	}
	if strings.TrimSpace(metadata.Meta.Image) == "" {
		return nil, fmt.Errorf("atlas metadata %q requires meta.image", path)
	}
	imagePath, err := ResolveResourcePath(pathpkg.Dir(metaPath), metadata.Meta.Image)
	if err != nil {
		return nil, fmt.Errorf("resolve atlas image for %q: %w", path, err)
	}
	if r.imageDecoder == nil {
		return nil, fmt.Errorf("atlas image %q requires an injected image decoder", metadata.Meta.Image)
	}
	texture, textureKey, err := r.textureFromPath(imagePath, options)
	if err != nil {
		return nil, fmt.Errorf("load atlas image for %q: %w", path, err)
	}
	key := atlasResourceKey{metadata: metaBlob.ContentID(), texture: textureKey}
	atlasResource, err := r.atlases.load(key, func() (*graphics.Atlas, error) {
		return graphics.NewAtlas(texture, metadata)
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("decode atlas %q: %w", path, err)
	}
	return atlasResource, nil
}

func (r *Resources) loadFont(path, displayPath string) (fontResource, error) {
	blob, err := openProjectBlob(r.source, path)
	if err != nil {
		return fontResource{}, fmt.Errorf("read %s: %w", displayPath, err)
	}
	data, err := blob.Bytes()
	if err != nil {
		_ = blob.Close()
		return fontResource{}, fmt.Errorf("read %s: %w", displayPath, err)
	}
	font, err := r.decodeFont(data, path)
	if err != nil {
		_ = blob.Close()
		return fontResource{}, err
	}
	_ = blob.ReleaseResidentPages()
	return fontResource{font: font, source: blob}, nil
}

func (r *Resources) Statistics() ResourceStatistics {
	if r == nil {
		return ResourceStatistics{}
	}
	statistics := ResourceStatistics{}
	addResourceStatistics(&statistics, r.textures)
	addResourceStatistics(&statistics, r.environments)
	addResourceStatistics(&statistics, r.atlasMetadata)
	addResourceStatistics(&statistics, r.atlases)
	addResourceStatistics(&statistics, r.fonts)
	addResourceStatistics(&statistics, r.videos)
	addResourceStatistics(&statistics, r.html)
	addResourceStatistics(&statistics, r.spines)
	addResourceStatistics(&statistics, r.shaders)
	addResourceStatistics(&statistics, r.material2DShaders)
	addResourceStatistics(&statistics, r.meshShaders)
	if r.modShaders != nil {
		addResourceStatistics(&statistics, r.modShaders)
	}
	r.namedMu.RLock()
	for _, table := range r.named {
		if provider, ok := table.(resourceStatisticsProvider); ok {
			addResourceStatistics(&statistics, provider)
		}
	}
	r.namedMu.RUnlock()
	return statistics
}

func (r *Resources) Dispose() error {
	if r == nil {
		return nil
	}
	errList := make([]error, 0)
	if r.modShaders != nil {
		disposeResource("element shader", r.modShaders, &errList)
	}
	if r.modModels != nil {
		disposeResource("module model", r.modModels, &errList)
	}
	clear(r.modClips)
	clear(r.modRigs)
	clear(r.modHTMLBases)
	disposeResource("Material2D shader", r.material2DShaders, &errList)
	disposeResource("mesh shader", r.meshShaders, &errList)
	r.namedMu.Lock()
	for kind, table := range r.named {
		if disposer, ok := table.(namedResourceDisposer); ok {
			if err := disposer.disposeResources(); err != nil {
				errList = append(errList, fmt.Errorf("dispose %s resources: %w", kind, err))
			}
		}
		delete(r.named, kind)
	}
	r.namedMu.Unlock()
	disposeResource("font", r.fonts, &errList)
	disposeResource("video", r.videos, &errList)
	disposeResource("html", r.html, &errList)
	disposeResource("Spine", r.spines, &errList)
	disposeResource("atlas", r.atlases, &errList)
	disposeResource("atlas metadata", r.atlasMetadata, &errList)
	disposeResource("shader source", r.shaders, &errList)
	disposeResource("texture", r.textures, &errList)
	disposeResource("environment", r.environments, &errList)
	r.prefabSpecs = nil
	return errors.Join(errList...)
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

func loadSceneCatalog(source iofs.FS, manifestDir string) (SceneID, map[SceneID]sceneSpawnPlan, []SceneID, error) {
	catalogPath := pathpkg.Join(manifestDir, sceneCatalogFileName)
	catalog, err := readJSON[sceneCatalog](source, catalogPath)
	if err != nil {
		return "", nil, nil, err
	}
	if err := normalizeSceneCatalog(&catalog); err != nil {
		return "", nil, nil, err
	}
	plans := make(map[SceneID]sceneSpawnPlan, len(catalog.Scenes))
	ids := make([]SceneID, 0, len(catalog.Scenes))
	for _, entry := range catalog.Scenes {
		id, path := entry.ID, entry.Path
		manifest, err := readJSON[sceneManifest](source, pathpkg.Join(manifestDir, path))
		if err != nil {
			return "", nil, nil, fmt.Errorf("load scene %q: %w", id, err)
		}
		if !manifest.Prepared {
			if !sourcejson.Enabled {
				return "", nil, nil, fmt.Errorf("scene %q: release builds require a preprocessed spawn plan", id)
			}
			if err := compileSceneManifest(string(id), &manifest); err != nil {
				return "", nil, nil, err
			}
		}
		plans[id], ids = manifest.SpawnPlan, append(ids, id)
	}
	if _, ok := plans[catalog.Entry]; !ok {
		return "", nil, nil, fmt.Errorf("%s entry scene %q is not registered", sceneCatalogFileName, catalog.Entry)
	}
	return catalog.Entry, plans, ids, nil
}

func normalizeSceneCatalog(catalog *sceneCatalog) error {
	if catalog == nil {
		return fmt.Errorf("%s is nil", sceneCatalogFileName)
	}
	catalog.Entry = SceneID(strings.TrimSpace(string(catalog.Entry)))
	if catalog.Entry == "" {
		return fmt.Errorf("%s entry scene is required", sceneCatalogFileName)
	}
	ids := make(map[SceneID]struct{}, len(catalog.Scenes))
	paths := make(map[string]struct{}, len(catalog.Scenes))
	for index := range catalog.Scenes {
		entry := &catalog.Scenes[index]
		entry.ID = SceneID(strings.TrimSpace(string(entry.ID)))
		if entry.ID == "" {
			return fmt.Errorf("%s contains a scene with empty id", sceneCatalogFileName)
		}
		if _, exists := ids[entry.ID]; exists {
			return fmt.Errorf("%s contains duplicate scene id %q", sceneCatalogFileName, entry.ID)
		}
		path, err := normalizeResourceFilePath[sceneResourceFile](entry.Path)
		if err != nil {
			return fmt.Errorf("scene %q path %q must be a relative *.scene.json path", entry.ID, entry.Path)
		}
		entry.Path = path
		if _, exists := paths[entry.Path]; exists {
			return fmt.Errorf("%s contains duplicate scene path %q", sceneCatalogFileName, entry.Path)
		}
		ids[entry.ID], paths[entry.Path] = struct{}{}, struct{}{}
	}
	if _, ok := ids[catalog.Entry]; !ok {
		return fmt.Errorf("%s entry scene %q is not registered", sceneCatalogFileName, catalog.Entry)
	}
	return nil
}

func prepareSceneManifest(name string, manifest *sceneManifest) error {
	seen := make(map[string]struct{})
	if err := validateSceneNodes(name, manifest.Nodes, seen); err != nil {
		return err
	}
	if err := validateGameObjectCoordinateSpaces(name, manifest.Nodes, nil); err != nil {
		return err
	}
	return nil
}

func compileSceneManifest(name string, manifest *sceneManifest) error {
	if err := prepareSceneManifest(name, manifest); err != nil {
		return err
	}
	plan, err := compileSceneSpawnPlan(manifest.Nodes)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	manifest.SpawnPlan = plan
	manifest.Nodes = nil
	manifest.Prepared = true
	return nil
}

func validateSceneNodes(name string, specs []SceneNodeSpec, seen map[string]struct{}) error {
	for _, spec := range specs {
		id := strings.TrimSpace(spec.ID)
		if id == "" {
			return fmt.Errorf("%s contains game object with empty id", name)
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("%s contains duplicate game object id %q", name, id)
		}
		seen[id] = struct{}{}
		// A control is another component. It may coexist with visual and
		// resource components on the same node; behavior is composed from the
		// complete authored component set.
		// Components are independent.  Validate every authored component rather
		// than selecting one as the node's role.
		if err := validateSceneComponents(name, id, spec.Components); err != nil {
			return err
		}
		scriptNames := make(map[string]struct{}, len(spec.Scripts))
		for _, script := range spec.Scripts {
			scriptName := strings.TrimSpace(script.Name)
			if scriptName == "" {
				return fmt.Errorf("%s scene node %q contains a script with empty name", name, id)
			}
			if _, exists := scriptNames[scriptName]; exists {
				return fmt.Errorf("%s scene node %q binds script %q more than once", name, id, scriptName)
			}
			scriptNames[scriptName] = struct{}{}
		}
		if animation := spec.Components.Animation; animation != nil {
			if animation.Speed < 0 || math.IsNaN(float64(animation.Speed)) || math.IsInf(float64(animation.Speed), 0) {
				return fmt.Errorf("%s game object %q animation speed must be finite and non-negative", name, id)
			}
		}
		if spec.Components.Sprite != nil && spec.Components.Sprite.Trail != nil {
			if err := spec.Components.Sprite.Trail.validate(); err != nil {
				return fmt.Errorf("%s game object %q trail %w", name, id, err)
			}
		}
		var texture *TextureSpec
		if spec.Components.Sprite != nil {
			texture = spec.Components.Sprite.Texture
		}
		if err := texture.validate(true); err != nil {
			return fmt.Errorf("%s game object %q texture %w", name, id, err)
		}
		if err := validateSceneNodes(name, spec.Children, seen); err != nil {
			return err
		}
	}
	return nil
}

func validateSceneComponents(name, id string, components SceneComponents) error {
	if components.Camera != nil {
		camera := components.Camera
		if err := camera.ToneMapping.Validate(); err != nil {
			return fmt.Errorf("camera %q: %w", id, err)
		}
		if _, err := camera.Fog.Settings(); err != nil {
			return fmt.Errorf("camera %q: %w", id, err)
		}
		if err := camera.Quality.Validate(); err != nil {
			return fmt.Errorf("camera %q: %w", id, err)
		}
		if err := camera.Exposure.Validate(); err != nil {
			return fmt.Errorf("camera %q: %w", id, err)
		}
		if err := camera.SSAO.Settings().Validate(); err != nil {
			return fmt.Errorf("camera %q: %w", id, err)
		}
		if err := camera.SSR.Settings().Validate(); err != nil {
			return fmt.Errorf("camera %q: %w", id, err)
		}
		if err := validateEnvironmentSpec(camera.Environment); err != nil {
			return fmt.Errorf("%s camera %q: %w", name, id, err)
		}
		if camera.Fov <= 0 || camera.Fov >= math.Pi {
			return fmt.Errorf("%s game object %q camera fov must be between 0 and pi", name, id)
		}
		if camera.Near <= 0 {
			return fmt.Errorf("%s game object %q camera near must be positive", name, id)
		}
		viewport := camera.Viewport
		if viewport.Width < 0 || viewport.Height < 0 {
			return fmt.Errorf("%s game object %q camera viewport size must be non-negative", name, id)
		}
	}
	if light := components.DirectionalLight; light != nil {
		if _, err := compileSceneDirectionalLight(SceneNodeSpec{ID: id, Components: components}); err != nil {
			return err
		}
	}
	if components.AmbientLight != nil && (components.AmbientLight.Brightness < 0 || !finiteFloat32(components.AmbientLight.Brightness)) {
		return fmt.Errorf("ambient brightness must be finite and non-negative")
	}
	if light := components.PointLight; light != nil {
		if _, err := light.Light(); err != nil {
			return fmt.Errorf("%s game object %q pointLight: %w", name, id, err)
		}
	}
	if light := components.SpotLight; light != nil {
		if _, err := light.Light(); err != nil {
			return fmt.Errorf("%s game object %q spotLight: %w", name, id, err)
		}
	}
	if light := components.AreaLight; light != nil {
		if _, err := light.Light(); err != nil {
			return fmt.Errorf("%s game object %q areaLight: %w", name, id, err)
		}
	}
	if err := validatePhysics2DComponents(name, id, components); err != nil {
		return err
	}
	return validatePhysics3DComponents(name, id, components)
}

func validateGameObjectCoordinateSpaces(name string, specs []SceneNodeSpec, parent *SceneNodeSpec) error {
	for i := range specs {
		spec := &specs[i]
		if spec.CoordinateSpace != "" && spec.CoordinateSpace != CoordinateSpaceWorld && spec.CoordinateSpace != CoordinateSpaceUI {
			return fmt.Errorf("%s game object %q has unsupported coordinate space %q", name, spec.ID, spec.CoordinateSpace)
		}
		if spec.Control != nil && !specUsesUI(spec) {
			return fmt.Errorf("%s scene node %q control requires UI coordinate space", name, spec.ID)
		}
		if spec.Layout != nil && !specUsesUI(spec) {
			return fmt.Errorf("%s scene node %q layout requires UI coordinate space", name, spec.ID)
		}
		if spec.Widget != nil && !specUsesUI(spec) {
			return fmt.Errorf("%s scene node %q widget requires UI coordinate space", name, spec.ID)
		}
		if spec.Components.Has3D() && specUsesUI(spec) {
			return fmt.Errorf("%s scene node %q cannot use UI coordinates for 3D components", name, spec.ID)
		}
		if spec.Components.HasPhysics2D() && specUsesUI(spec) {
			return fmt.Errorf("%s scene node %q cannot use UI coordinates for Physics 2D components", name, spec.ID)
		}
		if spec.Components.HasPhysics3D() && specUsesUI(spec) {
			return fmt.Errorf("%s scene node %q cannot use UI coordinates for Physics 3D components", name, spec.ID)
		}
		if strings.TrimSpace(spec.TargetCamera) != "" && !specUsesUI(spec) {
			return fmt.Errorf("%s game object %q targetCamera requires UI coordinate space", name, spec.ID)
		}
		if parent != nil {
			parentUI := specUsesUI(parent)
			childUI := specUsesUI(spec)
			if parentUI != childUI {
				return fmt.Errorf("%s game object %q cannot use %s coordinates under %s parent %q", name, spec.ID, coordinateSpaceName(childUI), coordinateSpaceName(parentUI), parent.ID)
			}
		}
		if err := validateGameObjectCoordinateSpaces(name, spec.Children, spec); err != nil {
			return err
		}
	}
	return nil
}

func specUsesUI(spec *SceneNodeSpec) bool {
	return spec != nil && spec.CoordinateSpace == CoordinateSpaceUI
}

func coordinateSpaceName(ui bool) string {
	if ui {
		return "UI"
	}
	return "world"
}

func countTrue(values ...bool) int {
	count := 0
	for _, value := range values {
		if value {
			count++
		}
	}
	return count
}

func indexManifestItems[T any](path string, items []T, kind string, id func(T) string, validate func(T) error) (map[string]T, error) {
	index := make(map[string]T, len(items))
	for _, item := range items {
		itemID := strings.TrimSpace(id(item))
		if itemID == "" {
			return nil, fmt.Errorf("%s contains %s with empty id", pathpkg.Base(path), kind)
		}
		if _, exists := index[itemID]; exists {
			return nil, fmt.Errorf("%s contains duplicate %s id %q", pathpkg.Base(path), kind, itemID)
		}
		index[itemID] = item
	}
	if validate != nil {
		for _, item := range items {
			if err := validate(item); err != nil {
				return nil, err
			}
		}
	}
	return index, nil
}

func readJSON[T any](source iofs.FS, path string) (T, error) {
	var target T
	data, err := readProjectFile(source, path)
	if err != nil {
		return target, err
	}
	if typedbin.IsEncoded(data) {
		target, err = typedbin.Unmarshal[T](data)
		if err != nil {
			return target, fmt.Errorf("decode preprocessed %s: %w", pathpkg.Base(path), err)
		}
		return target, nil
	}
	return decodeJSON[T](data, path)
}

func loadGameManifest(source iofs.FS, root string) (gameManifest, string, error) {
	manifestDir := pathpkg.Join(root, "resources")
	var manifest gameManifest

	manifestPath := pathpkg.Join(manifestDir, projectFileName)
	exists, err := projectFileExists(source, manifestPath)
	if err != nil {
		return gameManifest{}, "", err
	}
	if !exists {
		return manifest, manifestDir, nil
	}

	loaded, err := readJSON[gameManifest](source, manifestPath)
	if err != nil {
		return gameManifest{}, "", err
	}
	if err := loaded.Engine.Validate(); err != nil {
		return gameManifest{}, "", fmt.Errorf("%s engine: %w", projectFileName, err)
	}
	manifest = loaded

	return manifest, manifestDir, nil
}

func resolveWindowIconPath(manifestDir, value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	return ResolveLocalResourcePath(manifestDir, value)
}

func alphaOrDefault(alpha float32) float32 {
	if alpha <= 0 {
		return 1
	}
	return alpha
}

func parseColorValue(value string, alpha float32) (graphics.ColorRGBA, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "rgb(") || strings.HasPrefix(value, "rgba(") {
		clr, err := parseFunctionalColor(value)
		if err != nil {
			return graphics.ColorRGBA{}, fmt.Errorf("unsupported color %q", value)
		}
		clr.A *= alpha
		return clr, nil
	}
	if !strings.HasPrefix(value, "#") {
		return graphics.ColorRGBA{}, fmt.Errorf("unsupported color %q", value)
	}
	s := strings.TrimPrefix(value, "#")
	if !isHexColor(s) {
		return graphics.ColorRGBA{}, fmt.Errorf("unsupported color %q", value)
	}
	switch len(s) {
	case 3:
		s = strings.Repeat(string(s[0]), 2) + strings.Repeat(string(s[1]), 2) + strings.Repeat(string(s[2]), 2) + "ff"
	case 4:
		s = strings.Repeat(string(s[0]), 2) + strings.Repeat(string(s[1]), 2) + strings.Repeat(string(s[2]), 2) + strings.Repeat(string(s[3]), 2)
	case 6:
		s += "ff"
	case 8:
	default:
		return graphics.ColorRGBA{}, fmt.Errorf("unsupported color %q", value)
	}

	n, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return graphics.ColorRGBA{}, err
	}

	r := uint8(n >> 24)
	g := uint8((n >> 16) & 0xff)
	b := uint8((n >> 8) & 0xff)
	a := uint8(n & 0xff)

	clr := graphics.ColorFromRGBA(r, g, b, a)
	clr.A *= alpha
	return clr, nil
}

func parseFunctionalColor(value string) (graphics.ColorRGBA, error) {
	open := strings.IndexByte(value, '(')
	if open < 0 || !strings.HasSuffix(value, ")") {
		return graphics.ColorRGBA{}, fmt.Errorf("invalid functional color")
	}
	name := strings.TrimSpace(value[:open])
	parts := strings.Split(value[open+1:len(value)-1], ",")
	want := 3
	if name == "rgba" {
		want = 4
	} else if name != "rgb" {
		return graphics.ColorRGBA{}, fmt.Errorf("invalid functional color name")
	}
	if len(parts) != want {
		return graphics.ColorRGBA{}, fmt.Errorf("invalid functional color fields")
	}
	channels := [4]float64{0, 0, 0, 1}
	for index, part := range parts {
		parsed, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return graphics.ColorRGBA{}, err
		}
		maximum := float64(255)
		if index == 3 {
			maximum = 1
		}
		if parsed < 0 || parsed > maximum {
			return graphics.ColorRGBA{}, fmt.Errorf("functional color channel out of range")
		}
		channels[index] = parsed
	}
	return graphics.ColorRGBA{
		R: float32(channels[0] / 255), G: float32(channels[1] / 255),
		B: float32(channels[2] / 255), A: float32(channels[3]),
	}, nil
}

func isHexColor(value string) bool {
	switch len(value) {
	case 3, 4, 6, 8:
	default:
		return false
	}
	for _, ch := range value {
		switch {
		case ch >= '0' && ch <= '9':
		case ch >= 'a' && ch <= 'f':
		case ch >= 'A' && ch <= 'F':
		default:
			return false
		}
	}
	return true
}
