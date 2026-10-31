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
	MutationRoots   []*dom.Node
	MutationRootSet map[*dom.Node]struct{}
	NodeFlags       htmlAncestorCache[*dom.Node, bool]
	NodeAncestors   htmlAncestorCache[*dom.Node, *dom.Node]
	IndexDirty      bool
	Source          string
	SourceDocument  *dom.Node
	EmbeddedCSS     string
	Root            *dom.Node
	Revision        uint64
	EventRevision   uint64
	NodeByID        map[string]*dom.Node
}

type htmlStyleState struct {
	RestyleNodes            []*dom.Node
	StyleCache              style.MutationCache
	StyleChanges            []style.NodeChange
	AnimationBaselines      map[*dom.Node]htmlAnimationBaseline
	Animations              map[*dom.Node]*htmlNodeAnimation
	AnimationTrackHeads     map[string]int
	TransformFrame          uint64
	TransformFrameSet       bool
	Stylesheet              *css.Stylesheet
	StylesheetKey           htmlStylesheetKey
	Styles                  style.Map
	Viewport                style.Viewport
	ContainerMetricsScratch map[*dom.Node]style.ContainerMetrics
}

type htmlLayoutState struct {
	LayoutCache      layout.Cache
	LayoutNodes      []*dom.Node
	LayoutOrderNodes []*dom.Node
	LayoutTopology   bool
	BoxesByNode      map[*dom.Node]*layout.Box
	Layout           Layout
	Boxes            *layout.Box
}

type htmlPaintState struct {
	StickyBindings    map[*dom.Node]*htmlStickyBinding
	StickyRoutes      htmlAncestorCache[*layout.Box, htmlStickyRoute]
	StickyDirty       bool
	BoxRenderings     map[*dom.Node]*htmlBoxRendering
	EmittingBox       *htmlBoxRendering
	RenderQueue       []*htmlBoxRendering
	RemovedBoxes      []*htmlBoxRendering
	LocalReconcile    bool
	PartialValid      bool
	OwnerSpansValid   bool
	BoxFlags          htmlAncestorCache[*layout.Box, bool]
	EntityGroups      map[htmlRenderIdentity]*htmlEntityGroup
	RenderNodes       map[*dom.Node]*htmlNodeRendering
	RenderGeneration  uint64
	Reconciling       bool
	Children          []ecs.Entity
	OverflowExtents   map[*layout.Box]htmlOverflowExtent
	PaintCullOffsets  map[*dom.Node]mgl32.Vec2
	PaintCullRoots    []*htmlBoxRendering
	PaintCullDisabled bool
	Textures          htmlTextureCache
}

type htmlInputState struct {
	ControlStylesChanged bool
	Input                css.State
	PreviousInput        css.State
	StateRoots           [2]*dom.Node
	SnapshotGeneration   uint32
	ControlSnapshots     map[*dom.Node]htmlControlSnapshot
	FocusTargets         []htmlFocusTargetBinding
	FocusByNode          map[*dom.Node]int
	NodeOffsets          []htmlNodeOffset
	NodeScrolls          []htmlNodeScroll
	NodeScrollBindings   map[*dom.Node]struct{}
	NodeBindingLocals    map[*dom.Node]htmlBindingIndices
	NodeBindingRoutes    htmlAncestorCache[*dom.Node, htmlBindingIndices]
	OverflowBindings     map[*dom.Node]htmlOverflowClipBinding
	OverflowEntities     map[ecs.Entity]htmlOverflowClipBinding
	OverflowRoutes       map[*dom.Node]htmlOverflowClipBinding
	NodeEvents           map[*dom.Node]htmlNodeEventBinding
	ComponentEvents      map[*dom.Node][]htmlComponentEventBinding
	EventTargets         map[*dom.Node]ecs.Entity
}

type htmlUpdateState struct {
	Diagnostics              SyncDiagnostics
	PendingLayout            bool
	PendingVisual            bool
	Visible, VisibilityKnown bool
	SyncStats                htmlSyncStats
	Key                      htmlRenderKey
	Rendered                 bool
	ImageProviderChanged     ecs.Tick
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
