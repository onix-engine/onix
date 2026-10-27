package style

import (
	"github.com/diiyw/onix/html/css"
	"github.com/diiyw/onix/html/dom"
)

func RecomputeSubtreesInPlaceWithViewportAndStateAndQueries(root *dom.Node, stylesheet *css.Stylesheet, viewport Viewport, input *css.State, queries *QueryEnvironment, previousStyles Map, roots []*dom.Node) (Map, error) {
	if previousStyles == nil {
		return ComputeWithViewportAndStateAndQueries(root, stylesheet, viewport, input, queries)
	}
	return recomputeSubtreesWithStyles(root, stylesheet, viewport, input, queries, previousStyles, roots, nil)
}

func RecomputeSubtreesInPlaceWithImpact(root *dom.Node, stylesheet *css.Stylesheet, viewport Viewport, input *css.State, queries *QueryEnvironment, previousStyles Map, roots []*dom.Node) (Map, ChangeImpact, error) {
	return RecomputeSubtreesInPlaceWithImpactAndQueries(root, stylesheet, viewport, input, queries, previousStyles, roots)
}

func RecomputeSubtreesInPlaceWithImpactAndQueries(root *dom.Node, stylesheet *css.Stylesheet, viewport Viewport, input *css.State, queries *QueryEnvironment, previousStyles Map, roots []*dom.Node) (Map, ChangeImpact, error) {
	if previousStyles == nil {
		styles, err := ComputeWithViewportAndStateAndQueries(root, stylesheet, viewport, input, queries)
		return styles, ChangeRebuild, err
	}
	impact := ChangeNone
	styles, err := recomputeSubtreesWithStyles(root, stylesheet, viewport, input, queries, previousStyles, roots, &impact)
	return styles, impact, err
}

func recomputeSubtreesWithStyles(root *dom.Node, stylesheet *css.Stylesheet, viewport Viewport, input *css.State, queries *QueryEnvironment, styles Map, roots []*dom.Node, impact *ChangeImpact) (Map, error) {
	queryEnv := QueryEnvironment{Viewport: viewport, Styles: styles}
	if queries != nil {
		queryEnv.ContainerMetrics = queries.ContainerMetrics
	}

	definitions, err := collectFilterDefinitions(root)
	if err != nil {
		return nil, err
	}
	scratch := acquireCascadeScratch()
	scratch.compiled = compiledMatcher(stylesheet)
	defer releaseCascadeScratch(scratch)
	for _, subtreeRoot := range roots {
		if subtreeRoot == nil {
			continue
		}
		if err := recomputeStyleSubtree(subtreeRoot, stylesheet, input, &queryEnv, styles, definitions, scratch, impact); err != nil {
			return nil, err
		}
	}
	return styles, nil
}

func recomputeStyleSubtree(node *dom.Node, stylesheet *css.Stylesheet, input *css.State, queries *QueryEnvironment, styles Map, definitions filterDefinitions, scratch *cascadeScratch, impact *ChangeImpact) error {
	var parent *ComputedStyle
	hasParent := false
	if node != nil && node.Parent != nil {
		if parentStyle, ok := styles.Get(node.Parent); ok {
			parent = parentStyle
			hasParent = true
		}
	}
	rootFontSize := rootFontSizeForNode(node, styles)
	return computeStyleSubtree(node, parent, hasParent, rootFontSize, stylesheet, input, queries, styles, definitions, scratch, impact)
}

func rootFontSizeForNode(node *dom.Node, styles Map) float64 {
	for ancestor := node; ancestor != nil; ancestor = ancestor.Parent {
		if ancestor.Type == dom.ElementNode && ancestor.Parent != nil && ancestor.Parent.Type == dom.DocumentNode {
			if computed, ok := styles.Get(ancestor); ok && computed.FontSize.Kind == ValueLengthPx {
				return computed.FontSize.LengthPx
			}
			break
		}
	}
	return 16
}

func viewportFromQueries(queries *QueryEnvironment) Viewport {
	if queries == nil {
		return Viewport{}
	}
	return queries.Viewport
}

// RecomputeInputStateInPlaceWithImpact visits the document once, recomputing
// only branches whose selector matches changed. Recomputing each affected
// branch also propagates inherited values to its descendants.
func RecomputeInputStateInPlaceWithImpact(root *dom.Node, stylesheet *css.Stylesheet, viewport Viewport, previous, current *css.State, styles Map) (Map, ChangeImpact, error) {
	return RecomputeInputStateInPlaceWithImpactAndQueries(root, stylesheet, viewport, previous, current, styles, nil)
}

func RecomputeInputStateInPlaceWithImpactAndQueries(root *dom.Node, stylesheet *css.Stylesheet, viewport Viewport, previous, current *css.State, styles Map, queries *QueryEnvironment) (Map, ChangeImpact, error) {
	return recomputeInputState(root, stylesheet, viewport, previous, current, styles, queries, nil, nil)
}

// RecomputeInputStateWithCache shares immutable styles with full and mutation
// cascades, so toggling back to a previous input state reuses its values.
func RecomputeInputStateWithCache(root *dom.Node, stylesheet *css.Stylesheet, viewport Viewport, previous, current *css.State, styles Map, cache *MutationCache) (Map, ChangeImpact, error) {
	return RecomputeInputStateWithChanges(root, stylesheet, viewport, previous, current, styles, nil, cache)
}

// RecomputeInputStateWithChanges emits changed styles during the input cascade.
// The caller owns and reuses the output slice, as with subtree mutations.
func RecomputeInputStateWithChanges(root *dom.Node, stylesheet *css.Stylesheet, viewport Viewport, previous, current *css.State, styles Map, changes *[]NodeChange, cache *MutationCache) (Map, ChangeImpact, error) {
	return recomputeInputState(root, stylesheet, viewport, previous, current, styles, &QueryEnvironment{ContainerMetrics: cache.ContainerMetrics}, cache, changes)
}

func recomputeInputState(root *dom.Node, stylesheet *css.Stylesheet, viewport Viewport, previous, current *css.State, styles Map, queries *QueryEnvironment, cache *MutationCache, changes *[]NodeChange) (Map, ChangeImpact, error) {
	if styles == nil {
		if cache != nil {
			computed, err := ComputeWithCache(root, stylesheet, viewport, current, nil, cache)
			return computed, ChangeRebuild, err
		}
		computed, err := ComputeWithViewportAndStateAndQueries(root, stylesheet, viewport, current, queries)
		return computed, ChangeRebuild, err
	}
	queryEnv := QueryEnvironment{Viewport: viewport, Styles: styles}
	if queries != nil {
		queryEnv.ContainerMetrics = queries.ContainerMetrics
	}
	var definitions filterDefinitions
	var err error
	if cache != nil {
		definitions, err = cache.filterDefinitions(root)
	} else {
		definitions, err = collectFilterDefinitions(root)
	}
	if err != nil {
		return nil, ChangeNone, err
	}
	scratch := acquireCascadeScratch()
	scratch.changes = changes
	if cache != nil {
		scratch.compiled = cache.matcher(stylesheet)
		cache.prepareStyleIndex()
		scratch.persistent = &cache.styles
	} else {
		scratch.compiled = compiledMatcher(stylesheet)
	}
	defer releaseCascadeScratch(scratch)
	impact := ChangeNone
	err = recomputeInputStateBranch(root, stylesheet, previous, current, &queryEnv, styles, definitions, scratch, &impact)
	return styles, impact, err
}

func recomputeInputStateBranch(node *dom.Node, stylesheet *css.Stylesheet, previous, current *css.State, queries *QueryEnvironment, styles Map, definitions filterDefinitions, scratch *cascadeScratch, impact *ChangeImpact) error {
	if node == nil {
		return nil
	}
	scratch.candidates = scratch.compiled.candidatesInto(node, scratch.candidates)
	for _, ref := range scratch.candidates {
		if !ref.inputDependent {
			continue
		}
		// Conditions can depend on ancestor sizes. Conservatively recompute when
		// a selector changes, even if its conditional rule is currently inactive.
		if css.MatchWithStateAndContext(ref.selector, node, previous, scratch.matcher) != css.MatchWithStateAndContext(ref.selector, node, current, scratch.matcher) {
			return recomputeStyleSubtree(node, stylesheet, current, queries, styles, definitions, scratch, impact)
		}
	}
	for _, child := range node.Children {
		if err := recomputeInputStateBranch(child, stylesheet, previous, current, queries, styles, definitions, scratch, impact); err != nil {
			return err
		}
	}
	return nil
}

// NodeChange records the result of the cascade, rather than the mutation that
// triggered it. The caller owns and reuses the output slice.
type NodeChange struct {
	// Tree changes box membership, order or positioning dependencies.
	Tree bool
	// Reorder means order is the only changed tree property; other non-tree
	// style changes may be present and still require normal layout/paint work.
	Reorder bool
	Node    *dom.Node
	Impact  ChangeImpact
}

// RecomputeSubtreesWithChanges updates retained styles and emits changes during
// the same traversal. Roots must be disjoint and belong to the document.
func RecomputeSubtreesWithChanges(root *dom.Node, stylesheet *css.Stylesheet, viewport Viewport, input *css.State, styles Map, roots []*dom.Node, changes *[]NodeChange, cache *MutationCache) (ChangeImpact, int, error) {
	queries := QueryEnvironment{Viewport: viewport, Styles: styles, ContainerMetrics: cache.ContainerMetrics}
	definitions, err := cache.filterDefinitions(root)
	if err != nil {
		return ChangeNone, 0, err
	}
	scratch := acquireCascadeScratch()
	scratch.compiled = cache.matcher(stylesheet)
	cache.prepareStyleIndex()
	scratch.persistent = &cache.styles
	scratch.changes = changes
	defer releaseCascadeScratch(scratch)
	impact := ChangeNone
	for _, node := range roots {
		if err := recomputeStyleSubtree(node, stylesheet, input, &queries, styles, definitions, scratch, &impact); err != nil {
			return impact, scratch.visited, err
		}
	}
	return impact, scratch.visited, nil
}

// NeedsDocumentStyle reports dependencies which cannot be bounded to a
// changed attribute's descendant branch. Unknown pseudo classes are deliberately
// conservative so extending the selector engine cannot silently stale styles.
func (cache *MutationCache) NeedsDocumentStyle(stylesheet *css.Stylesheet) bool {
	compiled := cache.matcher(stylesheet)
	return compiled != nil && compiled.documentMutation
}

func (cache *MutationCache) NeedsSiblingStyle(stylesheet *css.Stylesheet) bool {
	compiled := cache.matcher(stylesheet)
	return compiled != nil && compiled.siblingMutation
}

func selectorNeedsDocumentMutation(selector css.Selector) bool {
	for _, part := range selector.Parts {
		for _, pseudo := range part.PseudoClasses {
			switch pseudo.Name {
			case "hover", "active", "focus", "focus-visible", "focus-within", "root", "first-child", "last-child", "only-child", "first-of-type", "last-of-type", "only-of-type", "nth-child", "nth-last-child", "nth-of-type", "nth-last-of-type", "empty", "disabled", "enabled", "checked", "indeterminate", "placeholder-shown":
			case "is", "where", "not":
				for _, nested := range pseudo.Selectors {
					if selectorNeedsDocumentMutation(nested) {
						return true
					}
				}
			default:
				return true
			}
		}
	}
	return false
}

func selectorNeedsSiblingMutation(selector css.Selector) bool {
	if selector.HasSiblingCombinator() {
		return true
	}
	for _, part := range selector.Parts {
		for _, pseudo := range part.PseudoClasses {
			switch pseudo.Name {
			case "is", "where", "not":
				for _, nested := range pseudo.Selectors {
					if selectorNeedsSiblingMutation(nested) {
						return true
					}
				}
			}
		}
	}
	return false
}

// MutationCache retains document resources across full and partial updates. DOM
// content/attribute changes other than class/style must invalidate definitions.
type MutationCache struct {
	root             *dom.Node
	definitions      filterDefinitions
	stylesheet       *css.Stylesheet
	compiled         *compiledStylesheet
	styles           styleIndex
	ContainerMetrics map[*dom.Node]ContainerMetrics
}

const maxDocumentStyleInterned = 2048

func (cache *MutationCache) prepareStyleIndex() {
	cache.styles.limit = maxDocumentStyleInterned
	if cache.styles.used >= maxDocumentStyleInterned {
		cache.styles.reset()
	}
}

func (cache *MutationCache) InvalidateDefinitions() { cache.root = nil; cache.definitions = nil }

func (cache *MutationCache) filterDefinitions(root *dom.Node) (filterDefinitions, error) {
	if cache.root != root {
		definitions, err := collectFilterDefinitions(root)
		if err != nil {
			return nil, err
		}
		cache.root, cache.definitions = root, definitions
	}
	return cache.definitions, nil
}

func (cache *MutationCache) matcher(stylesheet *css.Stylesheet) *compiledStylesheet {
	if cache.stylesheet != stylesheet {
		cache.stylesheet, cache.compiled = stylesheet, compiledMatcher(stylesheet)
		cache.styles.reset()
	}
	return cache.compiled
}
