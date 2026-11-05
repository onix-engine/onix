package html

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/html/css"
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/layout"
	"github.com/diiyw/onix/html/style"
	"github.com/go-gl/mathgl/mgl32"
)

type htmlDocumentState struct {
	Mutations       map[*dom.Node]htmlMutation
	NodeByID        map[string]*dom.Node
	MutationRootSet map[*dom.Node]struct{}
	Root            *dom.Node
	SourceDocument  *dom.Node
	Source          string
	EmbeddedCSS     string
	NodeAncestors   htmlAncestorCache[*dom.Node, *dom.Node]
	NodeFlags       htmlAncestorCache[*dom.Node, bool]
	MutationRoots   []*dom.Node
	Revision        uint64
	EventRevision   uint64
	IndexDirty      bool
}

type htmlStyleState struct {
	Stylesheet              *css.Stylesheet
	AnimationTrackHeads     map[string]int
	ContainerMetricsScratch map[*dom.Node]style.ContainerMetrics
	AnimationBaselines      map[*dom.Node]htmlAnimationBaseline
	Animations              map[*dom.Node]*htmlNodeAnimation
	Styles                  style.Map
	StylesheetKey           htmlStylesheetKey
	AnimationRoots          []*dom.Node
	RestyleNodes            []*dom.Node
	ActiveAnimations        []*dom.Node
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
	LayoutNodes      []*dom.Node
	LayoutOrderNodes []*dom.Node
	LayoutCache      layout.Cache
	Layout           Layout
	LayoutTopology   bool
}

type htmlPaintState struct {
	Textures          htmlTextureCache
	BoxRenderings     map[*dom.Node]*htmlBoxRendering
	EmittingBox       *htmlBoxRendering
	StickyBindings    map[*dom.Node]*htmlStickyBinding
	PaintCullOffsets  map[*dom.Node]mgl32.Vec2
	OverflowExtents   map[*layout.Box]htmlOverflowExtent
	RenderNodes       map[*dom.Node]*htmlNodeRendering
	EntityGroups      map[htmlRenderIdentity]*htmlEntityGroup
	BoxFlags          htmlAncestorCache[*layout.Box, bool]
	StickyRoutes      htmlAncestorCache[*layout.Box, htmlStickyRoute]
	RenderQueue       []*htmlBoxRendering
	Children          []ecs.Entity
	RemovedBoxes      []*htmlBoxRendering
	PaintCullRoots    []*htmlBoxRendering
	RenderGeneration  uint64
	OwnerSpansValid   bool
	PartialValid      bool
	Reconciling       bool
	LocalReconcile    bool
	PaintCullDisabled bool
	StickyDirty       bool
}

type htmlInputState struct {
	StateRoots           [2]*dom.Node
	NodeScrollBindings   map[*dom.Node]struct{}
	OverflowBindings     map[*dom.Node]*htmlOverflowClipBinding
	EventTargets         map[*dom.Node]ecs.Entity
	ComponentEvents      map[*dom.Node][]htmlComponentEventBinding
	ControlSnapshots     map[*dom.Node]htmlControlSnapshot
	NodeEvents           map[*dom.Node]htmlNodeEventBinding
	FocusByNode          map[*dom.Node]int
	OverflowRoutes       map[*dom.Node]*htmlOverflowClipBinding
	OverflowEntities     map[ecs.Entity]*htmlOverflowClipBinding
	NodeBindingLocals    map[*dom.Node]htmlBindingIndices
	NodeScrollIDs        map[string]int
	NodeOffsetIDs        map[string]int
	PreviousInput        css.State
	Input                css.State
	NodeBindingRoutes    htmlAncestorCache[*dom.Node, htmlBindingIndices]
	NodeScrolls          []htmlNodeScroll
	NodeOffsets          []htmlNodeOffset
	FocusTargets         []htmlFocusTargetBinding
	NodeBindingKey       htmlNodeBindingKey
	SnapshotGeneration   uint32
	ControlStylesChanged bool
	NodeBindingsPrepared bool
}

type htmlUpdateState struct {
	Diagnostics              SyncDiagnostics
	Key                      htmlRenderKey
	SyncStats                htmlSyncStats
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
