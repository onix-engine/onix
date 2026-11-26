package html

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/html/css"
	"github.com/onix-engine/onix/html/layout"
	"github.com/onix-engine/onix/html/style"
	"github.com/onix-engine/onix/internal/reuse"
	"github.com/onix-engine/onix/scene"
	uinode "github.com/onix-engine/onix/scene/node"
	"github.com/onix-engine/onix/ui/internal/textfield"
)

type htmlDocumentState struct {
	Mutations         map[*uinode.Node]htmlMutation
	NodeByID          map[string]*uinode.Node
	MutationRootSet   map[*uinode.Node]struct{}
	Root              *uinode.Node
	Source            string
	EmbeddedCSS       []string
	MutationRoots     []*uinode.Node
	DetachedRoots     []*uinode.Node
	NodeAncestors     htmlAncestorCache[*uinode.Node, *uinode.Node]
	NodeFlags         htmlAncestorCache[*uinode.Node, bool]
	DetachedRetention reuse.Retention
	Revision          uint64
	EventRevision     uint64
	IndexDirty        bool
	FromSource        bool
	SourceDirty       bool
	SourceCompacted   bool
}

func (state *htmlDocumentState) matchesSource(value Surface) bool {
	if state.SourceDirty || state.Root == nil || state.Source != value.Value {
		return false
	}
	if value.Document == nil {
		return state.FromSource
	}
	return !state.FromSource && state.Root == value.Document
}

type htmlStyleState struct {
	Stylesheet              *css.Stylesheet
	AnimationTrackHeads     map[string]int
	ContainerMetricsScratch map[*uinode.Node]style.ContainerMetrics
	AnimationBaselines      map[*uinode.Node]htmlAnimationBaseline
	Animations              map[*uinode.Node]*htmlNodeAnimation
	Styles                  style.Map
	StylesheetKey           htmlStylesheetKey
	AnimationRoots          []*uinode.Node
	RestyleNodes            []*uinode.Node
	ActiveAnimations        []*uinode.Node
	StyleChanges            []style.NodeChange
	AnimationComposition    htmlAnimationComposition
	StyleCache              style.MutationCache
	Viewport                style.Viewport
	AnimationTime           float64
	TransformFrame          uint64
	TransformFrameSet       bool
}

type htmlLayoutState struct {
	Boxes            *layout.Box
	LayoutNodes      []*uinode.Node
	LayoutOrderNodes []*uinode.Node
	LayoutCache      layout.Cache
	Layout           Layout
	LayoutTopology   bool
}

type htmlPaintState struct {
	BoxRenderings     map[*uinode.Node]*htmlBoxRendering
	EmittingBox       *htmlBoxRendering
	StickyBindings    map[*uinode.Node]*htmlStickyBinding
	PaintCullOffsets  map[*uinode.Node]mgl32.Vec2
	OverflowExtents   map[*layout.Box]htmlOverflowExtent
	RenderNodes       map[*uinode.Node]*htmlNodeRendering
	EntityGroups      map[htmlRenderIdentity]*htmlEntityGroup
	Textures          htmlTextureCache
	PaintList         *scene.UIPaintList2D
	RenderQueue       []*htmlBoxRendering
	Children          []ecs.Entity
	RemovedBoxes      []*htmlBoxRendering
	PaintCullRoots    []*htmlBoxRendering
	PaintCullGroups   []*htmlEntityGroup
	PaintCullNodes    []*htmlNodeRendering
	InheritedClips    [8]*htmlBoxPaintClip
	StickyRoutes      htmlAncestorCache[*layout.Box, htmlStickyRoute]
	BoxFlags          htmlAncestorCache[*layout.Box, bool]
	RenderGeneration  uint64
	PaintCullEpoch    uint64
	NextInheritedClip uint8
	OwnerSpansValid   bool
	PartialValid      bool
	Reconciling       bool
	LocalReconcile    bool
	PaintCullDisabled bool
	PaintCullSync     bool
	StickyDirty       bool
	PaintListDirty    bool
}

type htmlInputState struct {
	StateRoots           [2]*uinode.Node
	OverflowBindings     map[*uinode.Node]*htmlOverflowClipBinding
	EventTargets         map[*uinode.Node]ecs.Entity
	DetachedFields       map[*uinode.Node]textfield.Field
	NodeEvents           map[*uinode.Node]htmlNodeEventBinding
	FocusByNode          map[*uinode.Node]int
	OverflowRoutes       map[*uinode.Node]*htmlOverflowClipBinding
	OverflowEntities     map[ecs.Entity]*htmlOverflowClipBinding
	NodeBindingLocals    map[*uinode.Node]htmlBindingIndices
	NodeScrollIDs        map[string]int
	NodeOffsetIDs        map[string]int
	PreviousInput        css.State
	Input                css.State
	NodeBindingRoutes    htmlAncestorCache[*uinode.Node, htmlBindingIndices]
	NodeScrolls          []htmlNodeScroll
	NodeOffsets          []htmlNodeOffset
	FocusTargets         []htmlFocusTargetBinding
	NodeBindingKey       htmlNodeBindingKey
	OverflowRetention    [3]reuse.MapRetention
	NodeBindingsPrepared bool
}

type htmlUpdateState struct {
	Diagnostics              SyncDiagnostics
	Key                      htmlRenderKey
	SyncStats                htmlSyncStats
	DocumentRetention        [4]reuse.MapRetention
	ImageProviderChanged     ecs.Tick
	PendingLayout            bool
	PendingVisual            bool
	Visible, VisibilityKnown bool
	Rendered                 bool
	LayoutDirty              bool
}

// SyncPath identifies the completed incremental update path.
type SyncPath string

const (
	SyncCached  SyncPath = "cached"
	SyncEvents  SyncPath = "events"
	SyncStyle   SyncPath = "style"
	SyncPartial SyncPath = "partial"
	SyncFull    SyncPath = "full"
)

// FallbackReason identifies why retained paint could not be updated locally.
type FallbackReason string

const (
	FallbackNone        FallbackReason = ""
	FallbackInitial     FallbackReason = "initial-document"
	FallbackDocument    FallbackReason = "document-replaced"
	FallbackEnvironment FallbackReason = "render-environment"
	FallbackImages      FallbackReason = "image-provider"
	FallbackLayout      FallbackReason = "layout-requested"
	FallbackVisual      FallbackReason = "visual-requested"
	FallbackRetained    FallbackReason = "retained-paint-invalid"
	FallbackVisibility  FallbackReason = "visibility-changed"
	FallbackContract    FallbackReason = "control-or-event-contract"
	FallbackStructure   FallbackReason = "structure-ownership"
	FallbackBox         FallbackReason = "box-ownership"
	FallbackContext     FallbackReason = "context-ownership"
	FallbackStack       FallbackReason = "stacking-ownership"
)

// SyncDiagnostics describes the last sync, including a stable full-update reason.
// Reading diagnostics does not invalidate the surface or allocate.
type SyncDiagnostics struct {
	Path     SyncPath
	Fallback FallbackReason
}

func Diagnostics(world *ecs.World, owner ecs.Entity) SyncDiagnostics {
	if state := world.GetRef[htmlRenderState](owner); state != nil {
		return state.Diagnostics
	}
	return SyncDiagnostics{}
}
func (state *htmlRenderState) fallback(reason FallbackReason) bool {
	state.Diagnostics.Fallback = reason
	return false
}
func (state *htmlUpdateState) requestLayout() { state.PendingLayout = true }
func (state *htmlUpdateState) requestVisual() { state.PendingVisual = true }
func (state *htmlUpdateState) pending() bool  { return state.PendingLayout || state.PendingVisual }
func (state *htmlRenderState) beginSync(key htmlRenderKey, imageTick ecs.Tick) {
	state.SyncStats = htmlSyncStats{}
	state.Diagnostics = SyncDiagnostics{Fallback: FallbackRetained}
	switch {
	case !state.Rendered:
		state.Diagnostics.Fallback = FallbackInitial
	case state.Key.Value != key.Value || state.Key.Document != key.Document:
		state.Diagnostics.Fallback = FallbackDocument
	case state.ImageProviderChanged != imageTick:
		state.Diagnostics.Fallback = FallbackImages
	case !sameHTMLRenderEnvironment(state.Key, key):
		state.Diagnostics.Fallback = FallbackEnvironment
	case state.PendingLayout:
		state.Diagnostics.Fallback = FallbackLayout
	case state.PendingVisual:
		state.Diagnostics.Fallback = FallbackVisual
	}
}

// Commit only after layout/paint/bindings have succeeded. Failed stages retain
// their pending inputs so the next sync retries them.
func (state *htmlRenderState) commitSync(key htmlRenderKey, imageTick ecs.Tick, path SyncPath) {
	state.Key, state.Rendered, state.ImageProviderChanged = key, true, imageTick
	state.PendingLayout, state.PendingVisual = false, false
	if path == SyncFull || path == SyncPartial {
		state.LayoutDirty = false
	}
	state.Diagnostics.Path = path
	if path != SyncFull {
		state.Diagnostics.Fallback = FallbackNone
	}
	clearHTMLMutations(state)
}

func (state *htmlDocumentState) consumeMutations() {
	clear(state.Mutations)
	clear(state.MutationRoots)
	state.MutationRoots = state.MutationRoots[:0]
}
func (state *htmlLayoutState) consumeInvalidations() {
	state.LayoutTopology = false
	clear(state.LayoutOrderNodes)
	state.LayoutOrderNodes = state.LayoutOrderNodes[:0]
	clear(state.LayoutNodes)
	state.LayoutNodes = state.LayoutNodes[:0]
}
func (state *htmlStyleState) consumeChanges() {
	clear(state.StyleChanges)
	state.StyleChanges = state.StyleChanges[:0]
}
