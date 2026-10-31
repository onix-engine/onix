package style

import (
	"fmt"
	"iter"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/diiyw/onix/html/css"
	"github.com/diiyw/onix/html/dom"
)

type cascadedValue struct {
	value  string
	weight cascadeWeight
	order  cascadePropertyOrder
	parsed *preparedValue
}

type cascadeWeight struct {
	important int
	inline    int
	id        int
	class     int
	tag       int
	order     int
}

type cascadeScratch struct {
	changes    *[]NodeChange
	visited    int
	values     map[string]cascadedValue
	candidates []compiledSelectorRef
	ordered    []specifiedProperty
	matcher    *css.MatchContext
	compiled   *compiledStylesheet
	styles     styleIndex
	motion     motionStyle
	persistent *styleIndex
}

type specifiedProperty struct {
	name  string
	value cascadedValue
	order cascadePropertyOrder
}

var cascadePropertyGroups = [...][]string{
	{"font", "font-size", "font-family", "font-weight"},
	{"display", "appearance", "pointer-events", "visibility"},
	{"table-layout", "border-spacing", "vertical-align"},
	{"flex", "flex-flow", "flex-direction", "flex-wrap", "flex-grow", "flex-shrink", "flex-basis"},
	{"grid-template-columns", "grid-template-rows", "grid-column", "grid-row", "grid-column-start", "grid-column-end", "grid-row-start", "grid-row-end"},
	{"justify-content", "justify-items", "justify-self", "align-items", "align-self", "align-content"},
	{"order"},
	{"gap", "row-gap", "column-gap"},
	{"container", "container-type", "container-name"},
	{"position", "float", "clear", "z-index", "opacity", "overflow", "overflow-x", "overflow-y"},
	{"transform", "transform-origin", "transition", "transition-property", "transition-duration", "transition-timing-function", "transition-delay"},
	{"animation", "animation-name", "animation-duration", "animation-timing-function", "animation-delay", "animation-iteration-count", "animation-direction", "animation-fill-mode", "animation-play-state"},
	{"scrollbar-width", "scrollbar-color", "scrollbar-track-color", "scrollbar-thumb-color", "scrollbar-thumb-hover-color"},
	{"box-sizing"},
	{"width", "height", "min-width", "min-height", "max-width", "max-height"},
	{"margin", "margin-top", "margin-right", "margin-bottom", "margin-left"},
	{"padding", "padding-top", "padding-right", "padding-bottom", "padding-left"},
	{"border", "border-width", "border-style", "border-color", "border-top", "border-right", "border-bottom", "border-left", "border-top-style", "border-right-style", "border-bottom-style", "border-left-style", "border-top-width", "border-right-width", "border-bottom-width", "border-left-width", "border-top-color", "border-right-color", "border-bottom-color", "border-left-color"},
	{"border-radius", "border-top-left-radius", "border-top-right-radius", "border-bottom-right-radius", "border-bottom-left-radius"},
	{"box-shadow"},
	{"filter"},
	{"background", "background-color", "background-image", "background-position", "background-repeat", "background-size", "background-origin", "background-clip", "background-attachment"},
	{"color", "placeholder-color", "text-align", "text-box-trim", "text-box-edge", "text-decoration-line", "text-indent", "letter-spacing", "tab-size", "white-space", "word-break", "overflow-wrap", "word-wrap", "text-overflow", "list-style-type"},
	{"inset", "top", "right", "bottom", "left"},
	{"object-fit", "object-position"},
}

var cascadeScratchPool = sync.Pool{New: func() any {
	return &cascadeScratch{
		values:  make(map[string]cascadedValue, 16),
		matcher: css.NewMatchContext(),
	}
}}

func acquireCascadeScratch() *cascadeScratch {
	scratch := cascadeScratchPool.Get().(*cascadeScratch)
	scratch.visited = 0
	scratch.persistent = nil
	clear(scratch.values)
	scratch.matcher.Reset()
	return scratch
}

func releaseCascadeScratch(scratch *cascadeScratch) {
	clear(scratch.values)
	clear(scratch.candidates)
	scratch.candidates = scratch.candidates[:0]
	clear(scratch.ordered)
	scratch.ordered = scratch.ordered[:0]
	scratch.matcher.Reset()
	scratch.compiled = nil
	scratch.changes = nil
	scratch.persistent = nil
	scratch.styles.reset()
	scratch.motion = motionStyle{}
	cascadeScratchPool.Put(scratch)
}

type filterDefinitions map[string][]Filter

type parseContext struct {
	Viewport       Viewport
	FontSize       float64
	ParentFontSize float64
	RootFontSize   float64
}

type ContainerMetrics struct {
	Width  float64
	Height float64
}

type QueryEnvironment struct {
	Viewport         Viewport
	ContainerMetrics map[*dom.Node]ContainerMetrics
	Styles           Map
}

func Compute(root *dom.Node, stylesheet *css.Stylesheet) (Map, error) {
	return ComputeWithViewportAndStateAndQueries(root, stylesheet, Viewport{}, nil, nil)
}

func ComputeWithState(root *dom.Node, stylesheet *css.Stylesheet, input *css.State) (Map, error) {
	return ComputeWithViewportAndStateAndQueries(root, stylesheet, Viewport{}, input, nil)
}

func ComputeWithViewport(root *dom.Node, stylesheet *css.Stylesheet, viewport Viewport) (Map, error) {
	return ComputeWithViewportAndStateAndQueries(root, stylesheet, viewport, nil, nil)
}

func ComputeWithViewportAndState(root *dom.Node, stylesheet *css.Stylesheet, viewport Viewport, input *css.State) (Map, error) {
	return ComputeWithViewportAndStateAndQueries(root, stylesheet, viewport, input, nil)
}

func ComputeWithViewportAndStateAndQueries(root *dom.Node, stylesheet *css.Stylesheet, viewport Viewport, input *css.State, queries *QueryEnvironment) (Map, error) {
	nodeCount, definitions, err := collectFilterDefinitionsAndCount(root)
	if err != nil {
		return nil, err
	}
	styles := make(Map, nodeCount)
	queryEnv := &QueryEnvironment{Viewport: viewport, Styles: styles}
	if queries != nil {
		queryEnv.ContainerMetrics = queries.ContainerMetrics
	}
	scratch := acquireCascadeScratch()
	scratch.compiled = compiledMatcher(stylesheet)
	defer releaseCascadeScratch(scratch)

	if err := computeStyleSubtree(root, nil, false, 16, stylesheet, input, queryEnv, styles, definitions, scratch, nil); err != nil {
		return nil, err
	}

	return styles, nil
}

// ComputeWithCache replaces the complete node map while retaining its storage
// and a bounded index of immutable styles. Equal values can be shared even
// after DOM nodes are replaced. The caller must invalidate filter definitions
// when their content changes, as for partial recomputation.
func ComputeWithCache(root *dom.Node, stylesheet *css.Stylesheet, viewport Viewport, input *css.State, styles Map, cache *MutationCache) (Map, error) {
	definitions, err := cache.filterDefinitions(root)
	if err != nil {
		return nil, err
	}
	if styles == nil {
		styles = make(Map)
	} else {
		clear(styles) // Removed nodes must not retain detached DOM subtrees.
	}
	queries := QueryEnvironment{Viewport: viewport, Styles: styles, ContainerMetrics: cache.ContainerMetrics}
	scratch := acquireCascadeScratch()
	scratch.compiled = cache.matcher(stylesheet)
	cache.prepareStyleIndex()
	scratch.persistent = &cache.styles
	defer releaseCascadeScratch(scratch)
	if err := computeStyleSubtree(root, nil, false, 16, stylesheet, input, &queries, styles, definitions, scratch, nil); err != nil {
		return nil, err
	}
	return styles, nil
}

func computeStyleSubtree(node *dom.Node, parent *ComputedStyle, hasParent bool, rootFontSize float64, stylesheet *css.Stylesheet, input *css.State, queries *QueryEnvironment, styles Map, definitions filterDefinitions, scratch *cascadeScratch, impact *ChangeImpact) error {
	scratch.visited++
	previous := styles[node]
	// Resolve into a stack local, then keep whichever stored instance already
	// carries these values, so nodes with an identical style share one block.
	computed := defaultStyle(node)
	if err := resolveComputedStyle(node, &computed, parent, hasParent, rootFontSize, stylesheet, input, queries, definitions, scratch); err != nil {
		return err
	}
	if previous == nil {
		if impact != nil {
			*impact = ChangeRebuild
		}
		if scratch.changes != nil {
			*scratch.changes = append(*scratch.changes, NodeChange{Node: node, Impact: ChangeRebuild, Tree: true})
		}
	} else if impact != nil {
		change := computedStyleChangeForNode(node, previous, &computed)
		if change > *impact {
			*impact = change
		}
		if change != ChangeNone && scratch.changes != nil {
			boxTreeChanged := previous.Display != computed.Display || previous.Position != computed.Position
			orderChanged := previous.Order != computed.Order
			*scratch.changes = append(*scratch.changes, NodeChange{Node: node, Impact: change, Tree: boxTreeChanged || orderChanged, Reorder: orderChanged && !boxTreeChanged})
		}
	}
	stored := scratch.internIndex().commit(&computed, previous)
	styles[node] = stored
	return computeStyleChildren(node, stored, rootFontSize, stylesheet, input, queries, styles, definitions, scratch, impact)
}

func (scratch *cascadeScratch) internIndex() *styleIndex {
	if scratch.persistent != nil {
		return scratch.persistent
	}
	return &scratch.styles
}

func computeStyleChildren(node *dom.Node, computed *ComputedStyle, rootFontSize float64, stylesheet *css.Stylesheet, input *css.State, queries *QueryEnvironment, styles Map, definitions filterDefinitions, scratch *cascadeScratch, impact *ChangeImpact) error {
	childRootFontSize := rootFontSize
	if node != nil && node.Type == dom.ElementNode && node.Parent != nil && node.Parent.Type == dom.DocumentNode && computed.FontSize.Kind == ValueLengthPx {
		childRootFontSize = computed.FontSize.LengthPx
	}
	var textStyle *ComputedStyle
	var textChange ChangeImpact
	for _, child := range node.Children {
		if child.Type == dom.TextNode {
			// Text nodes never match rules and carry no form defaults, so the
			// computed style is a pure function of the parent style: every text
			// child of one parent shares the same computed value.
			if textStyle == nil {
				if err := computeStyleSubtree(child, computed, true, childRootFontSize, stylesheet, input, queries, styles, definitions, scratch, impact); err != nil {
					return err
				}
				if scratch.changes != nil && len(*scratch.changes) > 0 {
					last := (*scratch.changes)[len(*scratch.changes)-1]
					if last.Node == child {
						textChange = last.Impact
					}
				}
				textStyle = styles[child]
				continue
			}
			styles[child] = textStyle
			if textChange != ChangeNone && scratch.changes != nil {
				*scratch.changes = append(*scratch.changes, NodeChange{Node: child, Impact: textChange})
			}
			continue
		}
		if err := computeStyleSubtree(child, computed, true, childRootFontSize, stylesheet, input, queries, styles, definitions, scratch, impact); err != nil {
			return err
		}
	}
	return nil
}

func resolveComputedStyle(node *dom.Node, computed *ComputedStyle, parent *ComputedStyle, hasParent bool, rootFontSize float64, stylesheet *css.Stylesheet, input *css.State, queries *QueryEnvironment, definitions filterDefinitions, scratch *cascadeScratch) error {
	scratch.motion = motionStyle{}
	computed.motion = &scratch.motion
	specified, err := collectSpecifiedValuesInto(node, stylesheet, input, queries, scratch)
	if err != nil {
		return err
	}

	if hasParent {
		inherit(parent, computed, specified)
	}
	// The HTML user-agent defaults for list containers establish their own
	// marker type before inherited author values are applied to descendants.
	if node != nil && node.Type == dom.ElementNode {
		switch node.Data {
		case "ol":
			if _, declared := specified["list-style-type"]; !declared {
				computed.ListStyleType = Keyword("decimal")
			}
		case "ul", "menu":
			if _, declared := specified["list-style-type"]; !declared {
				computed.ListStyleType = Keyword("disc")
			}
		case "table":
			if _, declared := specified["border-spacing"]; !declared {
				computed.BorderSpacingX, computed.BorderSpacingY = LengthPx(2), LengthPx(2)
			}
		}
	}
	applyFormControlDefaults(node, computed)
	// Form defaults are reapplied after inheritance, so restore the hidden UA rule before author CSS.
	applyHiddenAttributeDefault(node, computed)
	parentFontSize := computed.FontSize.LengthPx
	if hasParent && parent.FontSize.Kind == ValueLengthPx {
		parentFontSize = parent.FontSize.LengthPx
	}
	ctx := parseContext{
		Viewport:       viewportFromQueries(queries),
		FontSize:       computed.FontSize.LengthPx,
		ParentFontSize: parentFontSize,
		RootFontSize:   rootFontSize,
	}
	if err := applySpecifiedValues(node, computed, parent, specified, definitions, ctx, scratch); err != nil {
		return err
	}
	resolvePaintColors(computed, parent, hasParent, node, specified)
	computed.motion = scratch.internIndex().resolveMotion(computed.motion)
	// A matching zero-duration declaration may replace the configuration of
	// a still-running transition. Keep paint topology until the property stops matching.
	computed.AnimatedOpacity = computed.TransitionFor(1).Enabled
	for i := range AnimatedColorCount {
		transition := computed.TransitionFor(i + 2)
		if transition.Enabled {
			computed.AnimatedPaint |= AnimatedColor << i
		}
	}
	for _, animation := range computed.Animations() {
		if animation.Name == "" || stylesheet == nil {
			continue
		}
		properties := scratch.compiled.keyframes[animation.Name].properties &^ computed.AnimationImportant
		computed.AnimatedOpacity = computed.AnimatedOpacity || properties&AnimatedOpacity != 0
		computed.AnimatedPaint |= properties & AnimatedColors
	}
	if hasParent && computed.ColorInherited {
		computed.AnimatedPaint |= parent.AnimatedPaint & AnimatedColor
	}
	if computed.AnimatedPaint&AnimatedColor != 0 {
		for i, value := range paintColorFields(computed) {
			if value.Keyword == "currentcolor" {
				computed.AnimatedPaint |= AnimatedColor << i
			}
		}
	}
	// Chromium's UA sheet declares [hidden]{display:none !important}, so author
	// rules such as "#content button{display:flex}" must not re-show the box.
	applyHiddenAttributeDefault(node, computed)
	computed.TextOpaqueBackdrop = computed.Visibility == Keyword("visible") && computed.BackgroundColor.Color.A == 255
	computed.TextInOpacityGroup = computed.Opacity.Number < 1 || computed.AnimatedOpacity || computed.AnimatedPaint&AnimatedBackgroundColor != 0 || len(computed.Filters) != 0
	if hasParent {
		computed.TextOpaqueBackdrop = computed.TextOpaqueBackdrop || parent.TextOpaqueBackdrop
		computed.TextInOpacityGroup = computed.TextInOpacityGroup || parent.TextInOpacityGroup
	}
	normalizeOverflow(computed)
	// CSS 2.2 §9.7: absolutely positioned inline boxes are blockified.
	// Resolve this before building boxes and collecting inline text/images,
	// so each element participates in exactly one formatting context.
	if computed.Position == Keyword("absolute") || computed.Float != Keyword("none") {
		switch computed.Display.Keyword {
		case "inline", "inline-block":
			computed.Display = Keyword("block")
		case "inline-flex":
			computed.Display = Keyword("flex")
		case "inline-grid":
			computed.Display = Keyword("grid")
		}
	}
	return nil
}

// CSS Overflow 3 computes a visible axis to auto when its opposite axis is
// clipped or scrollable. The same normalization applies to clip, which loses
// its special no-scroll behavior in that combination.
func normalizeOverflow(computed *ComputedStyle) {
	if computed == nil || computed.OverflowX.Kind != ValueKeyword || computed.OverflowY.Kind != ValueKeyword {
		return
	}
	x, y := computed.OverflowX.Keyword, computed.OverflowY.Keyword
	if x == "visible" && y != "visible" && y != "clip" {
		computed.OverflowX = Keyword("auto")
	}
	if y == "visible" && x != "visible" && x != "clip" {
		computed.OverflowY = Keyword("auto")
	}
	if x == "clip" && y != "visible" && y != "clip" {
		computed.OverflowX = Keyword("hidden")
	}
	if y == "clip" && x != "visible" && x != "clip" {
		computed.OverflowY = Keyword("hidden")
	}
}

func computedStyleChangeForNode(node *dom.Node, previous, current *ComputedStyle) ChangeImpact {
	if previous.Display != current.Display || previous.TableLayout != current.TableLayout ||
		previous.BorderSpacingX != current.BorderSpacingX || previous.BorderSpacingY != current.BorderSpacingY || previous.VerticalAlign != current.VerticalAlign ||
		previous.BoxSizing != current.BoxSizing ||
		previous.Width != current.Width || previous.Height != current.Height ||
		previous.MinWidth != current.MinWidth || previous.MinHeight != current.MinHeight ||
		previous.MaxWidth != current.MaxWidth || previous.MaxHeight != current.MaxHeight ||
		previous.Margin != current.Margin || previous.Padding != current.Padding ||
		previous.BorderWidth != current.BorderWidth || previous.BorderStyle != current.BorderStyle ||
		previous.TextAlign != current.TextAlign ||
		previous.TextBoxTrim != current.TextBoxTrim || previous.TextBoxEdge != current.TextBoxEdge ||
		previous.TextIndent != current.TextIndent || previous.LetterSpacing != current.LetterSpacing || previous.TabSize != current.TabSize ||
		previous.FontSize != current.FontSize ||
		previous.LineHeight != current.LineHeight || previous.LineHeightNormal != current.LineHeightNormal || previous.FontFamily != current.FontFamily ||
		previous.FontWeight != current.FontWeight || previous.WhiteSpace != current.WhiteSpace ||
		previous.ListStyleType != current.ListStyleType ||
		previous.WordBreak != current.WordBreak || previous.OverflowWrap != current.OverflowWrap ||
		previous.TextOverflow != current.TextOverflow ||
		previous.FlexDirection != current.FlexDirection || previous.FlexWrap != current.FlexWrap ||
		previous.GridTemplateColumns != current.GridTemplateColumns || previous.GridTemplateRows != current.GridTemplateRows ||
		previous.GridColumnStart != current.GridColumnStart || previous.GridColumnEnd != current.GridColumnEnd ||
		previous.GridRowStart != current.GridRowStart || previous.GridRowEnd != current.GridRowEnd ||
		previous.JustifyContent != current.JustifyContent || previous.AlignItems != current.AlignItems ||
		previous.JustifyItems != current.JustifyItems || previous.JustifySelf != current.JustifySelf ||
		previous.AlignSelf != current.AlignSelf || previous.AlignContent != current.AlignContent ||
		previous.FlexGrow != current.FlexGrow || previous.FlexShrink != current.FlexShrink ||
		previous.FlexBasis != current.FlexBasis || previous.Order != current.Order ||
		previous.RowGap != current.RowGap || previous.ColumnGap != current.ColumnGap ||
		previous.ContainerType != current.ContainerType || previous.ContainerName != current.ContainerName ||
		previous.Position != current.Position || previous.Float != current.Float || previous.Clear != current.Clear ||
		previous.OverflowX != current.OverflowX || previous.OverflowY != current.OverflowY ||
		previous.ScrollbarWidth != current.ScrollbarWidth ||
		previous.ScrollbarTrackColor != current.ScrollbarTrackColor ||
		previous.ScrollbarThumbColor != current.ScrollbarThumbColor ||
		previous.ScrollbarThumbHoverColor != current.ScrollbarThumbHoverColor ||
		previous.Top != current.Top || previous.Right != current.Right ||
		previous.Bottom != current.Bottom || previous.Left != current.Left ||
		previous.Appearance != current.Appearance {
		return ChangeRebuild
	}
	if previous.Visibility != current.Visibility || previous.TextDecorationLine != current.TextDecorationLine || previous.ZIndex != current.ZIndex || previous.Opacity != current.Opacity || previous.AnimatedOpacity != current.AnimatedOpacity || previous.AnimatedPaint != current.AnimatedPaint ||
		((previous.TextInOpacityGroup || current.TextInOpacityGroup) && (previous.Color != current.Color || previous.BackgroundColor != current.BackgroundColor || previous.BorderRadius != current.BorderRadius || previous.BorderColor != current.BorderColor)) ||
		(previous.BorderColor != current.BorderColor && (!uniformBorderColor(previous.BorderColor) || !uniformBorderColor(current.BorderColor))) ||
		(previous.TextDecorationLine == Keyword("line-through") && transparentColor(previous.Color) != transparentColor(current.Color)) ||
		(!isPaintedHTMLControl(node) && previous.BorderColor != current.BorderColor && (transparentColor(previous.BorderColor.Top) != transparentColor(current.BorderColor.Top) || transparentColor(previous.BorderColor.Right) != transparentColor(current.BorderColor.Right) || transparentColor(previous.BorderColor.Bottom) != transparentColor(current.BorderColor.Bottom) || transparentColor(previous.BorderColor.Left) != transparentColor(current.BorderColor.Left))) ||
		previous.ObjectFit != current.ObjectFit || previous.ObjectPosX != current.ObjectPosX ||
		previous.ObjectPosY != current.ObjectPosY || previous.ObjectPosXOrigin != current.ObjectPosXOrigin ||
		previous.ObjectPosYOrigin != current.ObjectPosYOrigin ||
		backgroundLayersRequireRebuild(previous, current) ||
		(paintedBackground(previous) != paintedBackground(current) && !isPaintedHTMLControl(node)) ||
		!slices.Equal(previous.BoxShadow, current.BoxShadow) || !slices.Equal(previous.Filters, current.Filters) {
		return ChangeVisual
	}
	if previous.TextOpaqueBackdrop != current.TextOpaqueBackdrop || previous.TextInOpacityGroup != current.TextInOpacityGroup ||
		previous.TransformRotation != current.TransformRotation ||
		previous.TransformTranslateX != current.TransformTranslateX ||
		previous.TransformTranslateY != current.TransformTranslateY ||
		previous.TransformScaleX != current.TransformScaleX || previous.TransformScaleY != current.TransformScaleY ||
		previous.TransformSkewX != current.TransformSkewX || previous.TransformTranslateCrossX != current.TransformTranslateCrossX || previous.TransformTranslateCrossY != current.TransformTranslateCrossY ||
		previous.TransformOriginX != current.TransformOriginX || previous.TransformOriginY != current.TransformOriginY ||
		!motionEqual(previous.motion, current.motion) || previous.AnimationImportant != current.AnimationImportant ||
		previous.BackgroundColor != current.BackgroundColor ||
		previous.BorderRadius != current.BorderRadius ||
		// A uniform border is part of the existing rounded-rectangle surface;
		// changing only its color can be applied without rebuilding layout.
		(previous.BorderColor != current.BorderColor && uniformBorderColor(previous.BorderColor) && uniformBorderColor(current.BorderColor)) ||
		previous.ColorInherited != current.ColorInherited || previous.Color != current.Color || previous.PlaceholderColor != current.PlaceholderColor ||
		previous.PointerEvents != current.PointerEvents {
		return ChangePaint
	}
	return ChangeNone
}

// Controls keep a Sprite2D surface even with a transparent background, so a
// hover fill only changes paint. Other elements need a rebuild to add or
// remove their surface when their background becomes visible or transparent.
func isPaintedHTMLControl(node *dom.Node) bool {
	if node == nil || node.Type != dom.ElementNode {
		return false
	}
	switch node.Data {
	case "button", "input", "select", "textarea":
		return true
	default:
		return false
	}
}

func transparentColor(value Value) bool { return value.Kind != ValueColor || value.Color.A == 0 }

func uniformBorderColor(value Edges) bool {
	return value.Top == value.Right && value.Top == value.Bottom && value.Top == value.Left
}

func paintedBackground(value *ComputedStyle) bool {
	return value.BackgroundColor.Kind == ValueColor && value.BackgroundColor.Color.A != 0
}

func backgroundLayersRequireRebuild(previous, current *ComputedStyle) bool {
	if previous.BackgroundImage == "" && current.BackgroundImage == "" {
		return false
	}
	return previous.BackgroundImage != current.BackgroundImage || previous.BackgroundSizeX != current.BackgroundSizeX ||
		previous.BackgroundSizeY != current.BackgroundSizeY || previous.BackgroundPosX != current.BackgroundPosX ||
		previous.BackgroundPosY != current.BackgroundPosY || previous.BackgroundPosXOrigin != current.BackgroundPosXOrigin ||
		previous.BackgroundPosYOrigin != current.BackgroundPosYOrigin || previous.BackgroundRepeatX != current.BackgroundRepeatX ||
		previous.BackgroundRepeatY != current.BackgroundRepeatY || previous.BackgroundOrigin != current.BackgroundOrigin ||
		previous.BackgroundClip != current.BackgroundClip || previous.BackgroundAttachment != current.BackgroundAttachment ||
		!slices.Equal(previous.BackgroundLayers, current.BackgroundLayers)
}

func collectSpecifiedValues(node *dom.Node, stylesheet *css.Stylesheet, input *css.State, queries *QueryEnvironment, matchContext *css.MatchContext) (map[string]cascadedValue, error) {
	scratch := &cascadeScratch{values: make(map[string]cascadedValue, 16), matcher: matchContext}
	if scratch.matcher == nil {
		scratch.matcher = css.NewMatchContext()
	}
	return collectSpecifiedValuesInto(node, stylesheet, input, queries, scratch)
}

func collectSpecifiedValuesInto(node *dom.Node, stylesheet *css.Stylesheet, input *css.State, queries *QueryEnvironment, scratch *cascadeScratch) (map[string]cascadedValue, error) {
	values := scratch.values
	clear(values)
	if node == nil || node.Type != dom.ElementNode {
		return values, nil
	}

	if stylesheet != nil {
		compiled := scratch.compiled
		if compiled == nil {
			compiled = compiledMatcher(stylesheet)
			scratch.compiled = compiled
		}
		candidates := compiled.candidatesInto(node, scratch.candidates)
		scratch.candidates = candidates
		for _, ref := range candidates {
			if !compiledConditionsMatch(compiled.conditions[ref.ruleIndex], node, queries) {
				continue
			}
			if !css.MatchWithStateAndContext(ref.selector, node, input, scratch.matcher) {
				continue
			}

			for declIndex, declaration := range compiled.declarations[ref.ruleIndex] {
				updateCascadedValue(values, declaration.property, declaration.value, declaration.order, declaration.parsed, cascadeWeight{
					important: boolInt(declaration.important),
					id:        ref.specificity.ID,
					class:     ref.specificity.Class,
					tag:       ref.specificity.Tag,
					order:     ref.orderBase + declIndex,
				})
			}
		}
	}

	inlineStyle, ok := node.Attr("style")
	if !ok || strings.TrimSpace(inlineStyle) == "" {
		return values, nil
	}

	if err := applyInlineDeclarations(values, inlineStyle); err != nil {
		return nil, err
	}
	return values, nil
}

func splitTopLevelQueryList(input string) []string {
	var parts []string
	start := 0
	depthParen := 0
	depthBracket := 0
	for i := 0; i < len(input); i++ {
		switch input[i] {
		case '(':
			depthParen++
		case ')':
			if depthParen > 0 {
				depthParen--
			}
		case '[':
			depthBracket++
		case ']':
			if depthBracket > 0 {
				depthBracket--
			}
		case ',':
			if depthParen == 0 && depthBracket == 0 {
				part := strings.TrimSpace(input[start:i])
				if part != "" {
					parts = append(parts, part)
				}
				start = i + 1
			}
		}
	}
	if tail := strings.TrimSpace(input[start:]); tail != "" {
		parts = append(parts, tail)
	}
	return parts
}

func extractQueryFeatureClauses(query string) []string {
	clauses := []string{}
	start := -1
	depth := 0
	for i := 0; i < len(query); i++ {
		switch query[i] {
		case '(':
			if depth == 0 {
				start = i + 1
			}
			depth++
		case ')':
			if depth == 0 {
				continue
			}
			depth--
			if depth == 0 && start >= 0 {
				clause := strings.TrimSpace(query[start:i])
				if clause != "" {
					clauses = append(clauses, clause)
				}
				start = -1
			}
		}
	}
	return clauses
}

type containerRequirement int

const (
	containerRequirementNone containerRequirement = iota
	containerRequirementInline
	containerRequirementSize
)

func splitContainerQueryPrelude(prelude string) (string, string) {
	prelude = strings.TrimSpace(prelude)
	if prelude == "" {
		return "", ""
	}
	if idx := strings.IndexByte(prelude, '('); idx >= 0 {
		name := strings.TrimSpace(prelude[:idx])
		return strings.ToLower(name), strings.TrimSpace(prelude[idx:])
	}
	return strings.ToLower(prelude), ""
}

func supportsContainerQueries(computed *ComputedStyle) bool {
	return containerTypeSupportsQuery(computed.ContainerType.Keyword, containerRequirementInline)
}

func containerNameMatches(names string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	if target == "" {
		return true
	}
	for _, candidate := range splitCSSFields(strings.ToLower(names)) {
		if candidate == target {
			return true
		}
	}
	return false
}

func containerRequirementForFeature(name string) containerRequirement {
	switch name {
	case "width", "min-width", "max-width", "inline-size", "min-inline-size", "max-inline-size":
		return containerRequirementInline
	case "height", "min-height", "max-height", "block-size", "min-block-size", "max-block-size", "aspect-ratio", "min-aspect-ratio", "max-aspect-ratio", "orientation":
		return containerRequirementSize
	default:
		return containerRequirementNone
	}
}

func containerTypeSupportsQuery(containerType string, required containerRequirement) bool {
	containerType = strings.ToLower(strings.TrimSpace(containerType))
	switch required {
	case containerRequirementInline:
		return containerType == "inline-size" || containerType == "size"
	case containerRequirementSize:
		return containerType == "size"
	default:
		return false
	}
}

func splitQueryFeature(feature string) (string, string) {
	before, after, ok := strings.Cut(strings.ToLower(strings.TrimSpace(feature)), ":")
	if !ok {
		return strings.TrimSpace(strings.ToLower(feature)), ""
	}
	return strings.TrimSpace(before), strings.TrimSpace(after)
}

func parseQueryValue(raw string) (float64, bool) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return 0, false
	}
	if strings.Contains(raw, "/") {
		parts := strings.SplitN(raw, "/", 2)
		if len(parts) != 2 {
			return 0, false
		}
		left, errLeft := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		right, errRight := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if errLeft != nil || errRight != nil || right == 0 {
			return 0, false
		}
		return left / right, true
	}
	if strings.HasSuffix(raw, "px") {
		value, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(raw, "px")), 64)
		return value, err == nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	return value, err == nil
}

func aspectRatio(width, height float64) float64 {
	if height == 0 {
		return 0
	}
	return width / height
}

// updateCascadedValue expects pre-normalized property names and values:
// stylesheet declarations are normalized at compile time and inline
// declarations at cache time.
func updateCascadedValue(values map[string]cascadedValue, property, value string, order cascadePropertyOrder, parsed *preparedValue, weight cascadeWeight) {
	if property == "" || value == "" {
		return
	}

	current, ok := values[property]
	if !ok || compareWeights(weight, current.weight) > 0 {
		values[property] = cascadedValue{value: value, weight: weight, order: order, parsed: parsed}
	}
}

func compareWeights(left, right cascadeWeight) int {
	switch {
	case left.important != right.important:
		return left.important - right.important
	case left.inline != right.inline:
		return left.inline - right.inline
	case left.id != right.id:
		return left.id - right.id
	case left.class != right.class:
		return left.class - right.class
	case left.tag != right.tag:
		return left.tag - right.tag
	default:
		return left.order - right.order
	}
}

type inlineDeclaration struct {
	property      string
	value         string
	important     bool
	order         int
	propertyOrder cascadePropertyOrder
	parsed        *preparedValue
}

type cachedInlineStyle struct {
	declarations []inlineDeclaration
	err          error
}

// Parsed inline style attributes are memoized by their exact attribute
// string: elements repeat the same style markup across nodes and frames.
var inlineStyleCache = struct {
	sync.Mutex
	entries map[string]cachedInlineStyle
}{entries: make(map[string]cachedInlineStyle, 64)}

const inlineStyleCacheLimit = 4096

func applyInlineDeclarations(values map[string]cascadedValue, input string) error {
	declarations, err := parseInlineStyleCached(input)
	if err != nil {
		return err
	}
	for i := range declarations {
		declaration := &declarations[i]
		updateCascadedValue(values, declaration.property, declaration.value, declaration.propertyOrder, declaration.parsed, cascadeWeight{important: boolInt(declaration.important), inline: 1, order: declaration.order})
	}
	return nil
}

func parseInlineStyleCached(input string) ([]inlineDeclaration, error) {
	inlineStyleCache.Lock()
	cached, ok := inlineStyleCache.entries[input]
	inlineStyleCache.Unlock()
	if ok {
		return cached.declarations, cached.err
	}
	declarations, err := parseInlineDeclarations(input)
	inlineStyleCache.Lock()
	if len(inlineStyleCache.entries) >= inlineStyleCacheLimit {
		inlineStyleCache.entries = make(map[string]cachedInlineStyle, 64)
	}
	inlineStyleCache.entries[input] = cachedInlineStyle{declarations: declarations, err: err}
	inlineStyleCache.Unlock()
	return declarations, err
}

func parseInlineDeclarations(input string) ([]inlineDeclaration, error) {
	var declarations []inlineDeclaration
	start := 0
	declarationOrder := 0
	parenDepth := 0
	bracketDepth := 0
	var quote byte
	escaped := false
	inComment := false
	flush := func(end int) error {
		part, err := css.StripComments(input[start:end])
		if err != nil {
			return nil
		}
		part = strings.TrimSpace(part)
		if part == "" {
			return nil
		}
		before, after, ok := strings.Cut(part, ":")
		if !ok {
			return nil
		}
		property := strings.ToLower(strings.TrimSpace(before))
		value, important := css.SplitImportant(after)
		if !validateDeclaration(property, value) {
			return nil
		}
		declarations = append(declarations, inlineDeclaration{
			property:      property,
			value:         value,
			important:     important,
			order:         declarationOrder,
			propertyOrder: cascadePropertyIndex[property],
			parsed:        preparePropertyValue(property, value),
		})
		return nil
	}
	for index := 0; index < len(input); index++ {
		ch := input[index]
		if inComment {
			if ch == '*' && index+1 < len(input) && input[index+1] == '/' {
				inComment = false
				index++
			}
			continue
		}
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case quote:
				quote = 0
			}
			continue
		}
		if ch == '/' && index+1 < len(input) && input[index+1] == '*' {
			inComment = true
			index++
			continue
		}
		switch ch {
		case '\'', '"':
			quote = ch
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case ';':
			if parenDepth == 0 && bracketDepth == 0 {
				if err := flush(index); err != nil {
					return nil, err
				}
				declarationOrder++
				start = index + 1
			}
		}
	}
	if inComment {
		return declarations, nil
	}
	if err := flush(len(input)); err != nil {
		return nil, err
	}
	return declarations, nil
}

// cascadePropertyIndex maps each known property to its group and position
// within the group, so applySpecifiedValues can order only the properties a
// node actually declares instead of probing every group.
var cascadePropertyIndex = buildCascadePropertyIndex()

type cascadePropertyOrder struct {
	group    int32
	position int32
	known    bool
}

func buildCascadePropertyIndex() map[string]cascadePropertyOrder {
	index := make(map[string]cascadePropertyOrder, len(cascadePropertyGroups)*2)
	for groupIndex, group := range cascadePropertyGroups {
		for position, property := range group {
			if _, exists := index[property]; !exists {
				index[property] = cascadePropertyOrder{group: int32(groupIndex), position: int32(position), known: true}
			}
		}
	}
	return index
}

func applySpecifiedValues(node *dom.Node, computed, parent *ComputedStyle, specified map[string]cascadedValue, definitions filterDefinitions, ctx parseContext, scratch *cascadeScratch) error {
	ordered := scratch.ordered[:0]
	for name, value := range specified {
		if !value.order.known {
			continue
		}
		ordered = append(ordered, specifiedProperty{name: name, value: value, order: value.order})
	}
	slices.SortFunc(ordered, func(left, right specifiedProperty) int {
		if left.order.group != right.order.group {
			return int(left.order.group - right.order.group)
		}
		if weight := compareWeights(left.value.weight, right.value.weight); weight != 0 {
			return weight
		}
		return int(left.order.position - right.order.position)
	})
	for i := range ordered {
		entry := &ordered[i]
		ctx.FontSize = computed.FontSize.LengthPx
		if isCSSWideKeyword(entry.value.value) {
			applyCSSWideKeyword(computed, parent, node, entry.name, entry.value.value)
			markAnimationImportance(computed, entry)
			continue
		}
		if parsed := entry.value.parsed; parsed != nil {
			if parsed.motion != nil {
				applyPreparedMotion(computed, parsed)
			} else {
				*compiledValueField(computed, parsed.property) = parsed.value
			}
			markAnimationImportance(computed, entry)
			continue
		}
		if err := applyProperty(computed, entry.name, entry.value.value, definitions, ctx); err != nil {
			// Invalid declarations are ignored at property level, including the
			// properties with a narrower implementation-specific value grammar.
			if entry.name == "opacity" || entry.name == "scrollbar-width" || entry.name == "transform" || entry.name == "transform-origin" || strings.HasPrefix(entry.name, "transition") || strings.HasPrefix(entry.name, "animation") {
				continue
			}
			return err
		}
		markAnimationImportance(computed, entry)
	}
	scratch.ordered = ordered
	return applyCascadedLineHeight(node, computed, parent, specified, ctx)
}

func markAnimationImportance(computed *ComputedStyle, entry *specifiedProperty) {
	if entry.value.weight.important == 0 {
		return
	}
	computed.AnimationImportant |= AnimatedProperty(entry.name)
}

func applyCascadedLineHeight(node *dom.Node, computed, parent *ComputedStyle, specified map[string]cascadedValue, ctx parseContext) error {
	lineHeight, hasLineHeight := specified["line-height"]
	font, hasFont := specified["font"]
	if !hasLineHeight && !hasFont {
		return nil
	}
	ctx.FontSize = computed.FontSize.LengthPx
	if hasFont && (!hasLineHeight || compareWeights(font.weight, lineHeight.weight) >= 0) {
		raw := strings.TrimSpace(font.value)
		if isCSSWideKeyword(raw) {
			return nil
		}
		_, linePart, _, err := parseFontShorthandParts(raw, ctx)
		if err != nil {
			return err
		}
		if linePart == "" {
			computed.LineHeight = LengthPx(computed.FontSize.LengthPx * 1.2)
			computed.LineHeightNormal = true
			return nil
		}
		value, err := parseLineHeight(linePart, computed.FontSize, ctx)
		if err != nil {
			return err
		}
		computed.LineHeight = value
		computed.LineHeightNormal = strings.EqualFold(strings.TrimSpace(linePart), "normal")
		return nil
	}
	if isCSSWideKeyword(lineHeight.value) {
		if isWideKeywordOf(lineHeight.value, "inherit") || isWideKeywordOf(lineHeight.value, "unset") {
			if parent != nil {
				computed.LineHeight, computed.LineHeightNormal = parent.LineHeight, parent.LineHeightNormal
			} else {
				defaults := defaultStyle(node)
				computed.LineHeight, computed.LineHeightNormal = defaults.LineHeight, defaults.LineHeightNormal
			}
			return nil
		}
		resetCSSProperty(computed, node, "line-height")
		return nil
	}
	value, err := parseLineHeight(lineHeight.value, computed.FontSize, ctx)
	if err != nil {
		return err
	}
	computed.LineHeight = value
	computed.LineHeightNormal = strings.EqualFold(strings.TrimSpace(lineHeight.value), "normal")
	return nil
}

func applyProperty(style *ComputedStyle, property, raw string, definitions filterDefinitions, ctx parseContext) error {
	if property == "transition" || strings.HasPrefix(property, "transition-") {
		return applyTransitionProperty(style, property, raw)
	}
	if property == "animation" || strings.HasPrefix(property, "animation-") {
		return applyAnimationProperty(style, property, raw)
	}
	switch property {
	case "font":
		return applyFontShorthand(style, raw, ctx)
	case "display":
		value, err := parseKeyword(raw, "none", "contents", "block", "inline", "inline-block", "flow-root", "flex", "inline-flex", "grid", "inline-grid", "list-item", "table", "inline-table", "table-caption", "table-column", "table-column-group", "table-row", "table-row-group", "table-header-group", "table-footer-group", "table-cell")
		if err != nil {
			return err
		}
		style.Display = value
	case "float":
		value, err := parseKeyword(raw, "none", "left", "right")
		if err != nil {
			return err
		}
		style.Float = value
	case "clear":
		value, err := parseKeyword(raw, "none", "left", "right", "both")
		if err != nil {
			return err
		}
		style.Clear = value
	case "table-layout":
		value, err := parseKeyword(raw, "auto", "fixed")
		if err != nil {
			return err
		}
		style.TableLayout = value
	case "border-spacing":
		parts := splitCSSFields(raw)
		if len(parts) < 1 || len(parts) > 2 {
			return fmt.Errorf("invalid border-spacing %q", raw)
		}
		x, err := parseAbsoluteLength(parts[0], ctx)
		if err != nil || x.LengthPx < 0 {
			return fmt.Errorf("invalid border-spacing %q", raw)
		}
		y := x
		if len(parts) == 2 {
			y, err = parseAbsoluteLength(parts[1], ctx)
			if err != nil || y.LengthPx < 0 {
				return fmt.Errorf("invalid border-spacing %q", raw)
			}
		}
		style.BorderSpacingX, style.BorderSpacingY = x, y
	case "vertical-align":
		value, err := parseKeyword(raw, "baseline", "top", "middle", "bottom")
		if err != nil {
			return err
		}
		style.VerticalAlign = value
	case "list-style-type":
		value, err := parseKeyword(raw, "disc", "circle", "square", "decimal", "decimal-leading-zero", "lower-alpha", "upper-alpha", "lower-latin", "upper-latin", "lower-roman", "upper-roman", "none")
		if err != nil {
			return err
		}
		style.ListStyleType = value
	case "grid-template-columns":
		style.GridTemplateColumns = strings.TrimSpace(raw)
	case "grid-template-rows":
		style.GridTemplateRows = strings.TrimSpace(raw)
	case "grid-column":
		start, end, err := parseGridLineShorthand(raw)
		if err != nil {
			return err
		}
		style.GridColumnStart, style.GridColumnEnd = start, end
	case "grid-row":
		start, end, err := parseGridLineShorthand(raw)
		if err != nil {
			return err
		}
		style.GridRowStart, style.GridRowEnd = start, end
	case "grid-column-start":
		value, err := parseGridLine(raw)
		if err != nil {
			return err
		}
		style.GridColumnStart = value
	case "grid-column-end":
		value, err := parseGridLine(raw)
		if err != nil {
			return err
		}
		style.GridColumnEnd = value
	case "grid-row-start":
		value, err := parseGridLine(raw)
		if err != nil {
			return err
		}
		style.GridRowStart = value
	case "grid-row-end":
		value, err := parseGridLine(raw)
		if err != nil {
			return err
		}
		style.GridRowEnd = value
	case "visibility":
		value, err := parseKeyword(raw, "visible", "hidden", "collapse")
		if err != nil {
			return err
		}
		style.Visibility = value
	case "pointer-events":
		value, err := parseKeyword(raw, "auto", "none", "inherit", "initial", "unset")
		if err != nil {
			return err
		}
		switch value.Keyword {
		case "inherit", "unset":
		case "initial":
			style.PointerEvents = Keyword("auto")
		default:
			style.PointerEvents = value
		}
	case "appearance":
		value, err := parseKeyword(raw, "auto", "none")
		if err != nil {
			return err
		}
		style.Appearance = value
	case "box-sizing":
		value, err := parseKeyword(raw, "content-box", "border-box")
		if err != nil {
			return err
		}
		style.BoxSizing = value
	case "flex":
		grow, shrink, basis, err := parseFlex(raw, ctx)
		if err != nil {
			return err
		}
		style.FlexGrow = grow
		style.FlexShrink = shrink
		style.FlexBasis = basis
	case "flex-flow":
		direction, wrap, err := parseFlexFlow(raw)
		if err != nil {
			return err
		}
		style.FlexDirection = direction
		style.FlexWrap = wrap
	case "flex-direction":
		value, err := parseKeyword(raw, "row", "row-reverse", "column", "column-reverse")
		if err != nil {
			return err
		}
		style.FlexDirection = value
	case "flex-wrap":
		value, err := parseKeyword(raw, "nowrap", "wrap", "wrap-reverse")
		if err != nil {
			return err
		}
		style.FlexWrap = value
	case "justify-content":
		value, err := parseKeyword(raw, "flex-start", "flex-end", "start", "end", "center", "space-between", "space-around", "space-evenly")
		if err != nil {
			return err
		}
		style.JustifyContent = value
	case "justify-items":
		value, err := parseKeyword(raw, "stretch", "start", "end", "center")
		if err != nil {
			return err
		}
		style.JustifyItems = value
	case "justify-self":
		value, err := parseKeyword(raw, "auto", "stretch", "start", "end", "center")
		if err != nil {
			return err
		}
		style.JustifySelf = value
	case "align-items":
		value, err := parseKeyword(raw, "stretch", "flex-start", "flex-end", "start", "end", "center", "baseline")
		if err != nil {
			return err
		}
		style.AlignItems = value
	case "align-self":
		value, err := parseKeyword(raw, "auto", "stretch", "flex-start", "flex-end", "start", "end", "center", "baseline")
		if err != nil {
			return err
		}
		style.AlignSelf = value
	case "align-content":
		value, err := parseKeyword(raw, "stretch", "flex-start", "flex-end", "start", "end", "center", "space-between", "space-around", "space-evenly")
		if err != nil {
			return err
		}
		style.AlignContent = value
	case "flex-grow":
		value, err := parseNumber(raw)
		if err != nil {
			return err
		}
		style.FlexGrow = value
	case "flex-shrink":
		value, err := parseNumber(raw)
		if err != nil {
			return err
		}
		style.FlexShrink = value
	case "flex-basis":
		value, err := parseLengthOrAuto(raw, true, ctx)
		if err != nil {
			return err
		}
		style.FlexBasis = value
	case "order":
		value, err := parseNumber(raw)
		if err != nil {
			return err
		}
		style.Order = value
	case "gap":
		rowGap, columnGap, err := parseGap(raw, ctx)
		if err != nil {
			return err
		}
		style.RowGap = rowGap
		style.ColumnGap = columnGap
	case "row-gap":
		value, err := parseLength(raw, ctx)
		if err != nil {
			return err
		}
		style.RowGap = value
	case "column-gap":
		value, err := parseLength(raw, ctx)
		if err != nil {
			return err
		}
		style.ColumnGap = value
	case "container":
		name, containerType, err := parseContainerShorthand(raw)
		if err != nil {
			return err
		}
		style.ContainerName = name
		style.ContainerType = containerType
	case "container-type":
		value, err := parseKeyword(raw, "normal", "inline-size", "size")
		if err != nil {
			return err
		}
		style.ContainerType = value
	case "container-name":
		style.ContainerName = normalizeContainerName(raw)
	case "position":
		style.Position = Keyword(strings.ToLower(strings.TrimSpace(raw)))
	case "inset":
		edges, err := parseBoxValues(raw, true, ctx)
		if err != nil {
			return err
		}
		style.Top, style.Right, style.Bottom, style.Left = edges.Top, edges.Right, edges.Bottom, edges.Left
	case "opacity":
		number, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(raw), "%"), 64)
		if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
			return fmt.Errorf("invalid opacity %q", raw)
		}
		if strings.HasSuffix(strings.TrimSpace(raw), "%") {
			number /= 100
		}
		style.Opacity = Number(max(0, min(1, number)))
	case "transform":
		transform, err := parseTransform(raw, ctx)
		if err != nil {
			return err
		}
		style.TransformRotation = transform.Rotation
		style.TransformTranslateX = transform.TranslateX
		style.TransformTranslateY = transform.TranslateY
		style.TransformScaleX, style.TransformScaleY = transform.ScaleX, transform.ScaleY
		style.TransformSkewX = transform.SkewX
		style.TransformTranslateCrossX, style.TransformTranslateCrossY = transform.TranslateCrossX, transform.TranslateCrossY
	case "transform-origin":
		x, y, err := parseTransformOrigin(raw, ctx)
		if err != nil {
			return err
		}
		style.TransformOriginX, style.TransformOriginY = x, y

	case "z-index":
		value, err := parseZIndex(raw)
		if err != nil {
			return err
		}
		style.ZIndex = value
	case "overflow":
		overflowX, overflowY, err := parseOverflow(raw)
		if err != nil {
			return err
		}
		style.OverflowX = overflowX
		style.OverflowY = overflowY
	case "overflow-x":
		value, err := parseKeyword(raw, "visible", "hidden", "clip", "auto", "scroll")
		if err != nil {
			return err
		}
		style.OverflowX = value
	case "overflow-y":
		value, err := parseKeyword(raw, "visible", "hidden", "clip", "auto", "scroll")
		if err != nil {
			return err
		}
		style.OverflowY = value
	case "scrollbar-width":
		value, err := parseScrollbarWidth(raw, ctx)
		if err != nil {
			return err
		}
		style.ScrollbarWidth = value
	case "scrollbar-color":
		thumb, track, err := parseScrollbarColors(raw)
		if err != nil {
			return err
		}
		style.ScrollbarThumbColor, style.ScrollbarTrackColor = thumb, track
	case "scrollbar-track-color":
		value, err := parseColor(raw)
		if err != nil {
			return err
		}
		style.ScrollbarTrackColor = ColorValue(value)
	case "scrollbar-thumb-color":
		value, err := parseColor(raw)
		if err != nil {
			return err
		}
		style.ScrollbarThumbColor = ColorValue(value)
	case "scrollbar-thumb-hover-color":
		value, err := parseColor(raw)
		if err != nil {
			return err
		}
		style.ScrollbarThumbHoverColor = ColorValue(value)
	case "width":
		value, err := parseLengthOrAuto(raw, true, ctx)
		if err != nil {
			return err
		}
		style.Width = value
	case "height":
		value, err := parseLengthOrAuto(raw, true, ctx)
		if err != nil {
			return err
		}
		style.Height = value
	case "min-width":
		value, err := parseLengthOrAuto(raw, true, ctx)
		if err != nil {
			return err
		}
		style.MinWidth = value
	case "min-height":
		value, err := parseLengthOrAuto(raw, true, ctx)
		if err != nil {
			return err
		}
		style.MinHeight = value
	case "max-width":
		value, err := parseLengthOrAuto(raw, true, ctx)
		if err != nil {
			return err
		}
		style.MaxWidth = value
	case "max-height":
		value, err := parseLengthOrAuto(raw, true, ctx)
		if err != nil {
			return err
		}
		style.MaxHeight = value
	case "margin":
		edges, err := parseBoxValues(raw, true, ctx)
		if err != nil {
			return err
		}
		style.Margin = edges
	case "margin-top":
		value, err := parseLengthOrAuto(raw, true, ctx)
		if err != nil {
			return err
		}
		style.Margin.Top = value
	case "margin-right":
		value, err := parseLengthOrAuto(raw, true, ctx)
		if err != nil {
			return err
		}
		style.Margin.Right = value
	case "margin-bottom":
		value, err := parseLengthOrAuto(raw, true, ctx)
		if err != nil {
			return err
		}
		style.Margin.Bottom = value
	case "margin-left":
		value, err := parseLengthOrAuto(raw, true, ctx)
		if err != nil {
			return err
		}
		style.Margin.Left = value
	case "padding":
		edges, err := parseBoxValues(raw, false, ctx)
		if err != nil {
			return err
		}
		style.Padding = edges
	case "padding-top":
		value, err := parseLength(raw, ctx)
		if err != nil {
			return err
		}
		style.Padding.Top = value
	case "padding-right":
		value, err := parseLength(raw, ctx)
		if err != nil {
			return err
		}
		style.Padding.Right = value
	case "padding-bottom":
		value, err := parseLength(raw, ctx)
		if err != nil {
			return err
		}
		style.Padding.Bottom = value
	case "padding-left":
		value, err := parseLength(raw, ctx)
		if err != nil {
			return err
		}
		style.Padding.Left = value
	case "border":
		widths, borderStyle, colors, err := parseBorder(raw, ctx)
		if err != nil {
			return err
		}
		style.BorderWidth = widths
		style.BorderStyle = borderStyle
		style.BorderColor = colors
	case "border-top", "border-right", "border-bottom", "border-left":
		widths, borderStyle, colors, err := parseBorder(raw, ctx)
		if err != nil {
			return err
		}
		switch property {
		case "border-top":
			style.BorderWidth.Top, style.BorderStyle.Top, style.BorderColor.Top = widths.Top, borderStyle.Top, colors.Top
		case "border-right":
			style.BorderWidth.Right, style.BorderStyle.Right, style.BorderColor.Right = widths.Right, borderStyle.Right, colors.Right
		case "border-bottom":
			style.BorderWidth.Bottom, style.BorderStyle.Bottom, style.BorderColor.Bottom = widths.Bottom, borderStyle.Bottom, colors.Bottom
		case "border-left":
			style.BorderWidth.Left, style.BorderStyle.Left, style.BorderColor.Left = widths.Left, borderStyle.Left, colors.Left
		}
	case "border-width":
		edges, err := parseBoxValues(raw, false, ctx)
		if err != nil {
			return err
		}
		style.BorderWidth = edges
	case "border-style":
		borderStyle, err := parseBorderStyle(raw)
		if err != nil {
			return err
		}
		style.BorderStyle = borderStyle
	case "border-color":
		colors, err := parseColorEdges(raw)
		if err != nil {
			return err
		}
		style.BorderColor = colors
	case "border-top-style", "border-right-style", "border-bottom-style", "border-left-style":
		value, err := parseKeyword(raw, "none", "solid")
		if err != nil {
			return err
		}
		switch property {
		case "border-top-style":
			style.BorderStyle.Top = value
		case "border-right-style":
			style.BorderStyle.Right = value
		case "border-bottom-style":
			style.BorderStyle.Bottom = value
		case "border-left-style":
			style.BorderStyle.Left = value
		}
	case "border-top-width":
		value, err := parseLength(raw, ctx)
		if err != nil {
			return err
		}
		style.BorderWidth.Top = value
	case "border-right-width":
		value, err := parseLength(raw, ctx)
		if err != nil {
			return err
		}
		style.BorderWidth.Right = value
	case "border-bottom-width":
		value, err := parseLength(raw, ctx)
		if err != nil {
			return err
		}
		style.BorderWidth.Bottom = value
	case "border-left-width":
		value, err := parseLength(raw, ctx)
		if err != nil {
			return err
		}
		style.BorderWidth.Left = value
	case "border-top-color":
		color, err := parseColorValue(raw)
		if err != nil {
			return err
		}
		style.BorderColor.Top = color
	case "border-right-color":
		color, err := parseColorValue(raw)
		if err != nil {
			return err
		}
		style.BorderColor.Right = color
	case "border-bottom-color":
		color, err := parseColorValue(raw)
		if err != nil {
			return err
		}
		style.BorderColor.Bottom = color
	case "border-left-color":
		color, err := parseColorValue(raw)
		if err != nil {
			return err
		}
		style.BorderColor.Left = color
	case "border-radius":
		corners, err := parseCornerValues(raw, ctx)
		if err != nil {
			return err
		}
		style.BorderRadius = corners
	case "border-top-left-radius":
		radius, err := parseCornerRadius(raw, ctx)
		if err != nil {
			return err
		}
		style.BorderRadius.TopLeft = radius
	case "border-top-right-radius":
		radius, err := parseCornerRadius(raw, ctx)
		if err != nil {
			return err
		}
		style.BorderRadius.TopRight = radius
	case "border-bottom-right-radius":
		radius, err := parseCornerRadius(raw, ctx)
		if err != nil {
			return err
		}
		style.BorderRadius.BottomRight = radius
	case "border-bottom-left-radius":
		radius, err := parseCornerRadius(raw, ctx)
		if err != nil {
			return err
		}
		style.BorderRadius.BottomLeft = radius
	case "box-shadow":
		shadow, err := parseBoxShadow(raw, ctx)
		if err != nil {
			return err
		}
		style.BoxShadow = shadow
	case "filter":
		filters, err := parseFilter(raw, definitions, ctx)
		if err != nil {
			return err
		}
		style.Filters = filters
	case "background":
		backgrounds, color, hasColor, err := parseBackgroundLayers(raw, ctx)
		if err != nil {
			return err
		}
		if hasColor {
			style.BackgroundColor = color
		}
		style.BackgroundLayers = backgrounds
		syncPrimaryBackgroundFields(style)
	case "background-color":
		color, err := parseColorValue(raw)
		if err != nil {
			return err
		}
		style.BackgroundColor = color
	case "background-image":
		images, err := parseBackgroundImageList(raw)
		if err != nil {
			return err
		}
		ensureBackgroundLayerCount(style, len(images))
		for i, imageURL := range images {
			style.BackgroundLayers[i].Image = imageURL
		}
		syncPrimaryBackgroundFields(style)
	case "background-position":
		positions, err := parseBackgroundPositionList(raw, ctx)
		if err != nil {
			return err
		}
		ensureBackgroundLayerCount(style, len(positions))
		for i, position := range positions {
			style.BackgroundLayers[i].PosX = position.X
			style.BackgroundLayers[i].PosY = position.Y
			style.BackgroundLayers[i].PosXOrigin = position.XOrigin
			style.BackgroundLayers[i].PosYOrigin = position.YOrigin
		}
		syncPrimaryBackgroundFields(style)
	case "background-repeat":
		repeats, err := parseBackgroundRepeatList(raw)
		if err != nil {
			return err
		}
		ensureBackgroundLayerCount(style, len(repeats))
		for i, repeat := range repeats {
			style.BackgroundLayers[i].RepeatX = repeat.X
			style.BackgroundLayers[i].RepeatY = repeat.Y
		}
		syncPrimaryBackgroundFields(style)
	case "background-size":
		sizes, err := parseBackgroundSizeList(raw, ctx)
		if err != nil {
			return err
		}
		ensureBackgroundLayerCount(style, len(sizes))
		for i, size := range sizes {
			style.BackgroundLayers[i].SizeX = size.X
			style.BackgroundLayers[i].SizeY = size.Y
		}
		syncPrimaryBackgroundFields(style)
	case "background-origin":
		origins, err := parseBackgroundBoxList(raw)
		if err != nil {
			return err
		}
		ensureBackgroundLayerCount(style, len(origins))
		for i, origin := range origins {
			style.BackgroundLayers[i].Origin = origin
		}
		style.BackgroundOrigin = origins[0]
		syncPrimaryBackgroundFields(style)
	case "background-clip":
		clips, err := parseBackgroundBoxList(raw)
		if err != nil {
			return err
		}
		ensureBackgroundLayerCount(style, len(clips))
		for i, clip := range clips {
			style.BackgroundLayers[i].Clip = clip
		}
		style.BackgroundClip = clips[0]
		syncPrimaryBackgroundFields(style)
	case "background-attachment":
		attachments, err := parseBackgroundAttachmentList(raw)
		if err != nil {
			return err
		}
		ensureBackgroundLayerCount(style, len(attachments))
		for i, attachment := range attachments {
			style.BackgroundLayers[i].Attachment = attachment
		}
		style.BackgroundAttachment = attachments[0]
		syncPrimaryBackgroundFields(style)
	case "color":
		color, err := parseColorValue(raw)
		if err != nil {
			return err
		}
		style.Color = color
	case "placeholder-color":
		color, err := parseColorValue(raw)
		if err != nil {
			return err
		}
		style.PlaceholderColor = color
	case "text-align":
		value, err := parseKeyword(raw, "left", "right", "center")
		if err != nil {
			return err
		}
		style.TextAlign = value
	case "text-decoration-line":
		value, err := parseKeyword(raw, "none", "line-through")
		if err != nil {
			return err
		}
		style.TextDecorationLine = value
	case "text-indent":
		value, err := parseLength(raw, ctx)
		if err != nil {
			return err
		}
		style.TextIndent = value
	case "word-break":
		value, err := parseKeyword(raw, "normal", "break-all", "keep-all")
		if err != nil {
			return err
		}
		style.WordBreak = value
	case "overflow-wrap", "word-wrap":
		value, err := parseKeyword(raw, "normal", "break-word", "anywhere")
		if err != nil {
			return err
		}
		style.OverflowWrap = value
	case "text-overflow":
		value, err := parseKeyword(raw, "clip", "ellipsis")
		if err != nil {
			return err
		}
		style.TextOverflow = value
	case "letter-spacing":
		if strings.EqualFold(strings.TrimSpace(raw), "normal") {
			style.LetterSpacing = LengthPx(0)
			return nil
		}
		value, err := parseLength(raw, ctx)
		if err != nil {
			return err
		}
		style.LetterSpacing = value
	case "text-box-trim":
		value, err := parseKeyword(raw, "none", "trim-start", "trim-end", "trim-both")
		if err != nil {
			return err
		}
		style.TextBoxTrim = value
	case "text-box-edge":
		value, err := parseTextBoxEdge(raw)
		if err != nil {
			return err
		}
		style.TextBoxEdge = value
	case "tab-size":
		value, err := parseNumber(raw)
		if err == nil {
			if value.Number < 0 || math.IsNaN(value.Number) || math.IsInf(value.Number, 0) {
				return fmt.Errorf("invalid tab-size %q", raw)
			}
			style.TabSize = value
			return nil
		}
		value, err = parseAbsoluteLength(raw, ctx)
		if err != nil || value.Kind == ValueLengthPx && value.LengthPx < 0 || value.Kind == ValueLengthCalc && value.LengthPx < 0 {
			return fmt.Errorf("invalid tab-size %q", raw)
		}
		style.TabSize = value
	case "font-size":
		value, err := parseFontSize(raw, ctx)
		if err != nil {
			return err
		}
		style.FontSize = value
	case "line-height":
		value, err := parseLineHeight(raw, style.FontSize, ctx)
		if err != nil {
			return err
		}
		style.LineHeight = value
		style.LineHeightNormal = strings.EqualFold(strings.TrimSpace(raw), "normal")
	case "font-family":
		style.FontFamily = Keyword(strings.Trim(strings.TrimSpace(raw), `"'`))
	case "font-weight":
		value, err := parseFontWeight(raw)
		if err != nil {
			return err
		}
		style.FontWeight = value
	case "white-space":
		value, err := parseKeyword(raw, "normal", "nowrap", "pre", "pre-wrap", "pre-line")
		if err != nil {
			return err
		}
		style.WhiteSpace = value
	case "top":
		value, err := parseLengthOrAuto(raw, true, ctx)
		if err != nil {
			return err
		}
		style.Top = value
	case "right":
		value, err := parseLengthOrAuto(raw, true, ctx)
		if err != nil {
			return err
		}
		style.Right = value
	case "bottom":
		value, err := parseLengthOrAuto(raw, true, ctx)
		if err != nil {
			return err
		}
		style.Bottom = value
	case "left":
		value, err := parseLengthOrAuto(raw, true, ctx)
		if err != nil {
			return err
		}
		style.Left = value
	case "object-fit":
		value, err := parseKeyword(raw, "fill", "contain", "cover", "none", "scale-down")
		if err != nil {
			return err
		}
		style.ObjectFit = value
	case "object-position":
		x, y, xOrigin, yOrigin, err := parseBackgroundPosition(raw, ctx)
		if err != nil {
			return err
		}
		style.ObjectPosX = x
		style.ObjectPosY = y
		style.ObjectPosXOrigin = xOrigin
		style.ObjectPosYOrigin = yOrigin
	}

	return nil
}

func parseKeyword(input string, allowed ...string) (Value, error) {
	trimmed := strings.ToLower(strings.TrimSpace(input))
	for _, candidate := range allowed {
		if trimmed == candidate {
			return Keyword(trimmed), nil
		}
	}
	return Value{}, fmt.Errorf("unsupported keyword %q", input)
}

func parseNumber(input string) (Value, error) {
	trimmed := strings.TrimSpace(strings.ToLower(input))
	value, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return Value{}, fmt.Errorf("invalid number %q", input)
	}
	return Number(value), nil
}

func parseZIndex(input string) (Value, error) {
	trimmed := strings.ToLower(strings.TrimSpace(input))
	if trimmed == "auto" {
		return Keyword("auto"), nil
	}
	value, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return Value{}, fmt.Errorf("invalid z-index %q", input)
	}
	return Number(float64(value)), nil
}

func parseGridLine(input string) (Value, error) {
	trimmed := strings.ToLower(strings.TrimSpace(input))
	if trimmed == "auto" {
		return Auto(), nil
	}
	line, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || line == 0 {
		return Value{}, fmt.Errorf("invalid grid line %q", input)
	}
	return Number(float64(line)), nil
}

func parseGridLineShorthand(input string) (Value, Value, error) {
	parts := strings.Split(input, "/")
	if len(parts) > 2 {
		return Value{}, Value{}, fmt.Errorf("invalid grid line shorthand %q", input)
	}
	start, err := parseGridLine(parts[0])
	if err != nil {
		return Value{}, Value{}, err
	}
	if len(parts) == 1 {
		return start, Auto(), nil
	}
	end, err := parseGridLine(parts[1])
	if err != nil {
		return Value{}, Value{}, err
	}
	return start, end, nil
}

func parseFlexFlow(input string) (Value, Value, error) {
	direction := Keyword("row")
	wrap := Keyword("nowrap")
	for part := range cssValues(input, true) {
		switch strings.ToLower(part) {
		case "row", "row-reverse", "column", "column-reverse":
			direction = Keyword(strings.ToLower(part))
		case "nowrap", "wrap", "wrap-reverse":
			wrap = Keyword(strings.ToLower(part))
		default:
			return Value{}, Value{}, fmt.Errorf("invalid flex-flow %q", input)
		}
	}
	return direction, wrap, nil
}

func parseFlex(input string, ctx parseContext) (Value, Value, Value, error) {
	trimmed := strings.ToLower(strings.TrimSpace(input))
	switch trimmed {
	case "none":
		return Number(0), Number(0), Auto(), nil
	case "auto":
		return Number(1), Number(1), Auto(), nil
	case "initial":
		return Number(0), Number(1), Auto(), nil
	}

	var tokens [3]string
	count, ok := cssFieldsInto(input, tokens[:])
	if !ok || count == 0 {
		return Value{}, Value{}, Value{}, fmt.Errorf("invalid flex shorthand %q", input)
	}

	grow := Number(0)
	shrink := Number(1)
	basis := LengthPx(0)
	var numbers [2]Value
	numberCount := 0
	basisSet := false

	for _, part := range tokens[:count] {
		part = strings.ToLower(part)
		if part == "auto" || strings.HasPrefix(part, "calc(") || strings.HasSuffix(part, "px") || strings.HasSuffix(part, "em") || strings.HasSuffix(part, "vw") || strings.HasSuffix(part, "vh") || strings.HasSuffix(part, "%") {
			value, err := parseLengthOrAuto(part, true, ctx)
			if err != nil || basisSet {
				return Value{}, Value{}, Value{}, fmt.Errorf("invalid flex shorthand %q", input)
			}
			basis, basisSet = value, true
			continue
		}
		if number, err := parseNumber(part); err == nil {
			if numberCount == len(numbers) {
				// After grow and shrink, a unitless zero denotes the basis.
				if part != "0" || basisSet {
					return Value{}, Value{}, Value{}, fmt.Errorf("invalid flex shorthand %q", input)
				}
				basis, basisSet = LengthPx(0), true
				continue
			}
			numbers[numberCount] = number
			numberCount++
			continue
		}
		return Value{}, Value{}, Value{}, fmt.Errorf("invalid flex shorthand %q", input)
	}

	if numberCount > 0 {
		grow = numbers[0]
	}
	if numberCount > 1 {
		shrink = numbers[1]
	}
	if numberCount == 1 && !basisSet {
		basis = LengthPx(0)
	}

	return grow, shrink, basis, nil
}

func parseGap(input string, ctx parseContext) (Value, Value, error) {
	var tokens [2]string
	count, ok := cssFieldsInto(input, tokens[:])
	if !ok || count == 0 {
		return Value{}, Value{}, fmt.Errorf("invalid gap %q", input)
	}
	first, err := parseLength(tokens[0], ctx)
	if err != nil {
		return Value{}, Value{}, err
	}
	second := first
	if count == 2 {
		second, err = parseLength(tokens[1], ctx)
		if err != nil {
			return Value{}, Value{}, err
		}
	}
	return first, second, nil
}

func parseOverflow(input string) (Value, Value, error) {
	var tokens [2]string
	count, ok := cssFieldsInto(input, tokens[:])
	if !ok || count == 0 {
		return Value{}, Value{}, fmt.Errorf("invalid overflow %q", input)
	}
	first, err := parseKeyword(tokens[0], "visible", "hidden", "clip", "auto", "scroll")
	if err != nil {
		return Value{}, Value{}, err
	}
	second := first
	if count == 2 {
		second, err = parseKeyword(tokens[1], "visible", "hidden", "clip", "auto", "scroll")
		if err != nil {
			return Value{}, Value{}, err
		}
	}
	return first, second, nil
}

func parseScrollbarWidth(input string, ctx parseContext) (Value, error) {
	trimmed := strings.ToLower(strings.TrimSpace(input))
	switch trimmed {
	case "auto", "thin", "none":
		return Keyword(trimmed), nil
	}
	return parseLength(input, ctx)
}

func parseScrollbarColors(input string) (Value, Value, error) {
	trimmed := strings.ToLower(strings.TrimSpace(input))
	if trimmed == "auto" {
		return Keyword("auto"), Keyword("auto"), nil
	}
	parts := splitCSSFields(trimmed)
	if len(parts) != 2 {
		return Value{}, Value{}, fmt.Errorf("invalid scrollbar-color %q", input)
	}
	thumb, err := parseColor(parts[0])
	if err != nil {
		return Value{}, Value{}, err
	}
	track, err := parseColor(parts[1])
	if err != nil {
		return Value{}, Value{}, err
	}
	return ColorValue(thumb), ColorValue(track), nil
}

func parseContainerShorthand(input string) (string, Value, error) {
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" {
		return "", Value{}, fmt.Errorf("invalid container shorthand %q", input)
	}
	if input == "normal" || input == "inline-size" || input == "size" {
		containerType, err := parseKeyword(input, "normal", "inline-size", "size")
		return "", containerType, err
	}
	namePart := input
	typePart := "normal"
	if before, after, ok := strings.Cut(input, "/"); ok {
		namePart = strings.TrimSpace(before)
		typePart = strings.TrimSpace(after)
	}
	if namePart == "none" {
		namePart = ""
	}
	containerType, err := parseKeyword(typePart, "normal", "inline-size", "size")
	if err != nil {
		return "", Value{}, err
	}
	return normalizeContainerName(namePart), containerType, nil
}

func normalizeContainerName(input string) string {
	names := splitCSSFields(strings.ToLower(strings.TrimSpace(input)))
	if len(names) == 0 {
		return ""
	}
	return strings.Join(names, " ")
}

func parseLength(input string, ctx parseContext) (Value, error) {
	input = strings.ToLower(strings.TrimSpace(input))
	if strings.HasPrefix(input, "calc(") {
		return parseCalcLength(input, ctx)
	}
	if input == "0" {
		return LengthPx(0), nil
	}
	for _, unit := range []string{"rem", "em", "px", "vw", "vh", "%"} {
		if !strings.HasSuffix(input, unit) {
			continue
		}
		number := strings.TrimSpace(strings.TrimSuffix(input, unit))
		value, err := strconv.ParseFloat(number, 64)
		if err != nil {
			return Value{}, fmt.Errorf("parse length %q: %w", input, err)
		}
		switch unit {
		case "px":
			return LengthPx(value), nil
		case "%":
			return LengthPercent(value / 100), nil
		case "em":
			fontSize := ctx.FontSize
			if fontSize <= 0 {
				fontSize = 16
			}
			return LengthPx(value * fontSize), nil
		case "rem":
			rootFontSize := ctx.RootFontSize
			if rootFontSize <= 0 {
				rootFontSize = 16
			}
			return LengthPx(value * rootFontSize), nil
		case "vw":
			return LengthPx(ctx.Viewport.Width * value / 100), nil
		case "vh":
			return LengthPx(ctx.Viewport.Height * value / 100), nil
		}
	}
	return Value{}, fmt.Errorf("expected length, got %q", input)
}

func parseAbsoluteLength(input string, ctx parseContext) (Value, error) {
	value, err := parseLength(input, ctx)
	if err != nil {
		return Value{}, err
	}
	if value.Kind == ValueLengthPercent || (value.Kind == ValueLengthCalc && value.Percent != 0) {
		return Value{}, fmt.Errorf("percentage length is not supported here")
	}
	return value, nil
}

func parseLengthOrAuto(input string, allowAuto bool, ctx parseContext) (Value, error) {
	if strings.EqualFold(strings.TrimSpace(input), "auto") {
		if !allowAuto {
			return Value{}, fmt.Errorf("auto is not supported here")
		}
		return Auto(), nil
	}
	return parseLength(input, ctx)
}

func parseFontSize(input string, ctx parseContext) (Value, error) {
	fontCtx := ctx
	fontCtx.FontSize = ctx.ParentFontSize
	value, err := parseLength(input, fontCtx)
	if err != nil {
		return Value{}, err
	}
	if value.Kind == ValueLengthPercent || value.Kind == ValueLengthCalc {
		fontSize := ctx.ParentFontSize
		if fontSize <= 0 {
			fontSize = 16
		}
		return LengthPx(value.LengthPx + fontSize*value.Percent), nil
	}
	return value, nil
}

func parseBoxValues(input string, allowAuto bool, ctx parseContext) (Edges, error) {
	return parseEdgeValues(input, "box shorthand", func(part string) (Value, error) {
		return parseLengthOrAuto(part, allowAuto, ctx)
	})
}

// The four shorthand values stay on the stack; expand them only after validation.
func parseEdgeValues(input, name string, parse func(string) (Value, error)) (Edges, error) {
	var values [4]Value
	count := 0
	for part := range cssValues(input, true) {
		if count == len(values) {
			return Edges{}, fmt.Errorf("invalid %s %q", name, input)
		}
		value, err := parse(part)
		if err != nil {
			return Edges{}, err
		}
		values[count] = value
		count++
	}
	switch count {
	case 0:
		return Edges{}, fmt.Errorf("invalid %s %q", name, input)
	case 1:
		return uniformEdges(values[0]), nil
	case 2:
		return Edges{Top: values[0], Right: values[1], Bottom: values[0], Left: values[1]}, nil
	case 3:
		return Edges{Top: values[0], Right: values[1], Bottom: values[2], Left: values[1]}, nil
	default:
		return Edges{Top: values[0], Right: values[1], Bottom: values[2], Left: values[3]}, nil
	}
}

func parseBorder(input string, ctx parseContext) (Edges, Edges, Edges, error) {
	widths := uniformEdges(LengthPx(0))
	borderStyle := uniformEdges(Keyword("solid"))
	colors := uniformEdges(currentColorValue())

	for part := range cssValues(input, true) {
		part = strings.ToLower(part)
		if part == "none" || part == "solid" {
			borderStyle = uniformEdges(Keyword(part))
			continue
		}
		// Dispatch by token kind instead of allocating errors for each valid
		// style or color before trying the appropriate parser.
		if part[0] >= '0' && part[0] <= '9' || part[0] == '.' || part[0] == '+' || part[0] == '-' || strings.HasPrefix(part, "calc(") {
			width, err := parseLength(part, ctx)
			if err != nil {
				return Edges{}, Edges{}, Edges{}, err
			}
			widths = uniformEdges(width)
			continue
		}
		color, err := parseColorValue(part)
		if err != nil {
			return Edges{}, Edges{}, Edges{}, err
		}
		colors = uniformEdges(color)
	}

	return widths, borderStyle, colors, nil
}

func parseBorderStyle(input string) (Edges, error) {
	return parseEdgeValues(input, "border-style", func(part string) (Value, error) {
		return parseKeyword(part, "none", "solid")
	})
}

func parseFontWeight(input string) (Value, error) {
	trimmed := strings.ToLower(strings.TrimSpace(input))
	switch trimmed {
	case "normal":
		return Number(400), nil
	case "bold":
		return Number(700), nil
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil || value < 100 || value > 900 || value%100 != 0 {
		return Value{}, fmt.Errorf("invalid font-weight %q", input)
	}
	return Number(float64(value)), nil
}

func parseColorEdges(input string) (Edges, error) {
	return parseEdgeValues(input, "color shorthand", parseColorValue)
}

func parseCornerValues(input string, ctx parseContext) (Corners, error) {
	horizontalInput, verticalInput, hasVertical := splitCornerShorthand(input)
	horizontal, err := parseCornerAxis(horizontalInput, ctx)
	if err != nil {
		return Corners{}, err
	}
	vertical := horizontal
	if hasVertical {
		vertical, err = parseCornerAxis(verticalInput, ctx)
		if err != nil {
			return Corners{}, err
		}
	}

	return Corners{
		TopLeft:     Radius{Horizontal: horizontal.Top, Vertical: vertical.Top},
		TopRight:    Radius{Horizontal: horizontal.Right, Vertical: vertical.Right},
		BottomRight: Radius{Horizontal: horizontal.Bottom, Vertical: vertical.Bottom},
		BottomLeft:  Radius{Horizontal: horizontal.Left, Vertical: vertical.Left},
	}, nil
}

func parseCornerRadius(input string, ctx parseContext) (Radius, error) {
	var tokens [2]string
	count, ok := cssFieldsInto(input, tokens[:])
	if !ok || count == 0 {
		return Radius{}, fmt.Errorf("invalid corner radius %q", input)
	}

	horizontal, err := parseLength(tokens[0], ctx)
	if err != nil {
		return Radius{}, err
	}
	vertical := horizontal
	if count == 2 {
		vertical, err = parseLength(tokens[1], ctx)
		if err != nil {
			return Radius{}, err
		}
	}

	return Radius{Horizontal: horizontal, Vertical: vertical}, nil
}

func splitCornerShorthand(input string) (string, string, bool) {
	trimmed := strings.TrimSpace(input)
	before, after, found := strings.Cut(trimmed, "/")
	if !found {
		return trimmed, "", false
	}
	return strings.TrimSpace(before), strings.TrimSpace(after), true
}

func parseCornerAxis(input string, ctx parseContext) (Edges, error) {
	return parseEdgeValues(input, "corner shorthand", func(part string) (Value, error) {
		return parseLength(part, ctx)
	})
}

func parseBoxShadow(input string, ctx parseContext) ([]Shadow, error) {
	trimmed := strings.TrimSpace(strings.ToLower(input))
	if trimmed == "" || trimmed == "none" {
		return nil, nil
	}

	parts := splitCSSList(trimmed)
	shadows := make([]Shadow, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "none" {
			return nil, fmt.Errorf("box-shadow cannot mix none with other values")
		}

		shadow, err := parseSingleBoxShadow(part, ctx)
		if err != nil {
			return nil, err
		}
		shadows = append(shadows, shadow)
	}
	return shadows, nil
}

func parseSingleBoxShadow(input string, ctx parseContext) (Shadow, error) {
	parts := splitCSSFields(input)
	shadow := Shadow{
		BlurRadius:   LengthPx(0),
		SpreadRadius: LengthPx(0),
		Color:        Color{A: 255},
	}
	lengths := make([]Value, 0, 4)

	for _, part := range parts {
		if part == "inset" {
			shadow.Inset = true
			continue
		}

		if color, err := parseColor(part); err == nil {
			shadow.Color = color
			continue
		}

		length, err := parseLength(part, ctx)
		if err != nil {
			return Shadow{}, fmt.Errorf("invalid box-shadow %q", input)
		}
		lengths = append(lengths, length)
	}

	if len(lengths) < 2 || len(lengths) > 4 {
		return Shadow{}, fmt.Errorf("box-shadow expects 2-4 lengths, got %q", input)
	}

	shadow.OffsetX = lengths[0]
	shadow.OffsetY = lengths[1]
	if len(lengths) >= 3 {
		shadow.BlurRadius = lengths[2]
	}
	if len(lengths) == 4 {
		shadow.SpreadRadius = lengths[3]
	}

	return shadow, nil
}

func parseFilter(input string, definitions filterDefinitions, ctx parseContext) ([]Filter, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" || strings.EqualFold(trimmed, "none") {
		return nil, nil
	}

	parts := splitCSSFields(trimmed)
	filters := make([]Filter, 0, len(parts))
	for _, part := range parts {
		resolved, err := parseFilterToken(part, definitions, ctx)
		if err != nil {
			return nil, err
		}
		filters = append(filters, resolved...)
	}
	return filters, nil
}

func parseFilterToken(input string, definitions filterDefinitions, ctx parseContext) ([]Filter, error) {
	trimmed := strings.TrimSpace(input)
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "url(") && strings.HasSuffix(trimmed, ")") {
		reference, err := parseFilterURLReference(trimmed)
		if err != nil {
			return nil, err
		}
		resolved, ok := definitions[reference]
		if !ok {
			return nil, fmt.Errorf("unknown filter reference %q", reference)
		}
		return append([]Filter(nil), resolved...), nil
	}

	filter, err := parseFilterFunction(trimmed, ctx)
	if err != nil {
		return nil, err
	}
	return []Filter{filter}, nil
}

func parseFilterFunction(input string, ctx parseContext) (Filter, error) {
	open := strings.IndexByte(input, '(')
	close := strings.LastIndexByte(input, ')')
	if open <= 0 || close != len(input)-1 {
		return Filter{}, fmt.Errorf("unsupported filter %q", input)
	}

	name := strings.ToLower(strings.TrimSpace(input[:open]))
	arg := strings.TrimSpace(input[open+1 : close])
	if arg == "" {
		switch name {
		case "blur":
			arg = "0px"
		case "hue-rotate":
			arg = "0deg"
		case "brightness", "opacity", "contrast", "grayscale", "saturate", "sepia", "invert":
			arg = "1"
		}
	}
	switch name {
	case "blur":
		value, err := parseAbsoluteLength(arg, ctx)
		if err != nil {
			return Filter{}, err
		}
		if value.LengthPx < 0 || math.IsNaN(value.LengthPx) || math.IsInf(value.LengthPx, 0) {
			return Filter{}, fmt.Errorf("invalid blur length %q", arg)
		}
		return Filter{Kind: FilterBlur, Amount: value.LengthPx}, nil
	case "brightness":
		amount, err := parseFilterMultiplier(arg)
		if err != nil {
			return Filter{}, err
		}
		return Filter{Kind: FilterBrightness, Amount: amount}, nil
	case "opacity":
		amount, err := parseFilterMultiplier(arg)
		if err != nil {
			return Filter{}, err
		}
		return Filter{Kind: FilterOpacity, Amount: amount}, nil
	case "contrast":
		amount, err := parseFilterMultiplier(arg)
		if err != nil {
			return Filter{}, err
		}
		return Filter{Kind: FilterContrast, Amount: amount}, nil
	case "grayscale":
		amount, err := parseFilterMultiplier(arg)
		if err != nil {
			return Filter{}, err
		}
		return Filter{Kind: FilterGrayscale, Amount: amount}, nil
	case "saturate":
		amount, err := parseFilterMultiplier(arg)
		if err != nil {
			return Filter{}, err
		}
		return Filter{Kind: FilterSaturate, Amount: amount}, nil
	case "sepia":
		amount, err := parseFilterMultiplier(arg)
		if err != nil {
			return Filter{}, err
		}
		return Filter{Kind: FilterSepia, Amount: amount}, nil
	case "invert":
		amount, err := parseFilterMultiplier(arg)
		if err != nil {
			return Filter{}, err
		}
		return Filter{Kind: FilterInvert, Amount: amount}, nil
	case "hue-rotate":
		amount, err := parseAngleDegrees(arg)
		if err != nil {
			return Filter{}, err
		}
		return Filter{Kind: FilterHueRotate, Amount: amount}, nil
	case "drop-shadow":
		shadow, err := parseDropShadowFilter(arg, ctx)
		if err != nil {
			return Filter{}, err
		}
		return Filter{Kind: FilterDropShadow, Shadow: shadow}, nil
	default:
		return Filter{}, fmt.Errorf("unsupported filter %q", input)
	}
}

func parseDropShadowFilter(input string, ctx parseContext) (Shadow, error) {
	parts := splitCSSFields(strings.ToLower(strings.TrimSpace(input)))
	shadow := Shadow{
		BlurRadius:   LengthPx(0),
		SpreadRadius: LengthPx(0),
		Color:        Color{R: 0, G: 0, B: 0, A: 255},
	}
	lengths := make([]Value, 0, 3)
	for _, part := range parts {
		if part == "inset" {
			return Shadow{}, fmt.Errorf("drop-shadow does not support inset")
		}
		if color, err := parseColor(part); err == nil {
			shadow.Color = color
			continue
		}
		length, err := parseAbsoluteLength(part, ctx)
		if err != nil {
			return Shadow{}, fmt.Errorf("invalid drop-shadow %q", input)
		}
		if math.IsNaN(length.LengthPx) || math.IsInf(length.LengthPx, 0) {
			return Shadow{}, fmt.Errorf("invalid drop-shadow length %q", part)
		}
		lengths = append(lengths, length)
	}
	if len(lengths) < 2 || len(lengths) > 3 {
		return Shadow{}, fmt.Errorf("drop-shadow expects 2-3 lengths, got %q", input)
	}
	shadow.OffsetX = lengths[0]
	shadow.OffsetY = lengths[1]
	if len(lengths) == 3 {
		if lengths[2].LengthPx < 0 {
			return Shadow{}, fmt.Errorf("drop-shadow blur must be nonnegative")
		}
		shadow.BlurRadius = lengths[2]
	}
	return shadow, nil
}

func parseFilterURLReference(input string) (string, error) {
	inner, err := parseImageURL(input)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(inner, "#") || len(inner) == 1 {
		return "", fmt.Errorf("unsupported filter url %q", input)
	}
	return inner[1:], nil
}

type backgroundValue struct {
	Color           *Value
	Image           string
	PositionX       Value
	PositionY       Value
	PositionXOrigin string
	PositionYOrigin string
	RepeatX         Value
	RepeatY         Value
	SizeX           Value
	SizeY           Value
	Origin          string
	Clip            string
	Attachment      string
}

type backgroundPositionValue struct {
	X       Value
	Y       Value
	XOrigin string
	YOrigin string
}

type backgroundRepeatValue struct {
	X Value
	Y Value
}

type backgroundSizeValue struct {
	X Value
	Y Value
}

func parseBackgroundAttachmentList(input string) ([]string, error) {
	parts := splitCSSList(input)
	if len(parts) == 0 {
		parts = []string{input}
	}
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.ToLower(strings.TrimSpace(part))
		switch value {
		case "scroll", "fixed", "local":
			values = append(values, value)
		default:
			return nil, fmt.Errorf("invalid background attachment %q", input)
		}
	}
	return values, nil
}

func parseBackgroundBoxList(input string) ([]string, error) {
	parts := splitCSSList(input)
	if len(parts) == 0 {
		parts = []string{input}
	}
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.ToLower(strings.TrimSpace(part))
		switch value {
		case "border-box", "padding-box", "content-box":
			values = append(values, value)
		default:
			return nil, fmt.Errorf("invalid background box %q", input)
		}
	}
	return values, nil
}

func parseBackgroundLayers(input string, ctx parseContext) ([]BackgroundLayer, Value, bool, error) {
	trimmed := strings.TrimSpace(input)
	if looksLikeColor(trimmed) {
		color, err := parseColorValue(trimmed)
		if err != nil {
			return nil, Value{}, false, err
		}
		return nil, color, true, nil
	}
	parts := splitCSSList(input)
	if len(parts) == 0 {
		parts = []string{input}
	}
	layers := make([]BackgroundLayer, 0, len(parts))
	var color Value
	hasColor := false
	for _, part := range parts {
		background, err := parseBackground(part, ctx)
		if err != nil {
			return nil, Value{}, false, err
		}
		if background.Color != nil {
			color, hasColor = *background.Color, true
		}
		layers = append(layers, BackgroundLayer{
			Image:      background.Image,
			SizeX:      background.SizeX,
			SizeY:      background.SizeY,
			PosX:       background.PositionX,
			PosY:       background.PositionY,
			PosXOrigin: background.PositionXOrigin,
			PosYOrigin: background.PositionYOrigin,
			RepeatX:    background.RepeatX,
			RepeatY:    background.RepeatY,
			Origin:     background.Origin,
			Clip:       background.Clip,
			Attachment: background.Attachment,
		})
	}
	return layers, color, hasColor, nil
}

func looksLikeColor(input string) bool {
	if input == "" {
		return false
	}
	if input[0] == '#' {
		// Hex colors cannot contain whitespace; a space means this is a
		// shorthand containing another component (usually an image).
		return !strings.ContainsAny(input, " \t\n\r")
	}
	lower := strings.ToLower(input)
	if strings.HasPrefix(lower, "rgb(") || strings.HasPrefix(lower, "rgba(") || isColorMix(lower) {
		// A functional color is a single CSS component.  Checking the
		// closing delimiter keeps a complete background shorthand such as
		// `rgba(...) linear-gradient(...)` on the layer parser path instead
		// of trying to parse the whole shorthand as one color.
		return strings.HasSuffix(lower, ")")
	}
	switch lower {
	case "black", "white", "red", "green", "blue", "transparent", "currentcolor":
		return true
	default:
		return false
	}
}

func parseBackgroundImageList(input string) ([]string, error) {
	parts := splitCSSList(input)
	if len(parts) == 0 {
		parts = []string{input}
	}
	images := make([]string, 0, len(parts))
	for _, part := range parts {
		if isBackgroundImageFunction(part) {
			images = append(images, strings.TrimSpace(part))
			continue
		}
		imageURL, err := parseImageURL(part)
		if err != nil {
			return nil, err
		}
		images = append(images, imageURL)
	}
	return images, nil
}

func parseBackgroundPositionList(input string, ctx parseContext) ([]backgroundPositionValue, error) {
	parts := splitCSSList(input)
	if len(parts) == 0 {
		parts = []string{input}
	}
	positions := make([]backgroundPositionValue, 0, len(parts))
	for _, part := range parts {
		x, y, xOrigin, yOrigin, err := parseBackgroundPosition(part, ctx)
		if err != nil {
			return nil, err
		}
		positions = append(positions, backgroundPositionValue{X: x, Y: y, XOrigin: xOrigin, YOrigin: yOrigin})
	}
	return positions, nil
}

func parseBackgroundRepeatList(input string) ([]backgroundRepeatValue, error) {
	parts := splitCSSList(input)
	if len(parts) == 0 {
		parts = []string{input}
	}
	repeats := make([]backgroundRepeatValue, 0, len(parts))
	for _, part := range parts {
		x, y, err := parseBackgroundRepeat(part)
		if err != nil {
			return nil, err
		}
		repeats = append(repeats, backgroundRepeatValue{X: x, Y: y})
	}
	return repeats, nil
}

func parseBackgroundSizeList(input string, ctx parseContext) ([]backgroundSizeValue, error) {
	parts := splitCSSList(input)
	if len(parts) == 0 {
		parts = []string{input}
	}
	sizes := make([]backgroundSizeValue, 0, len(parts))
	for _, part := range parts {
		x, y, err := parseBackgroundSize(part, ctx)
		if err != nil {
			return nil, err
		}
		sizes = append(sizes, backgroundSizeValue{X: x, Y: y})
	}
	return sizes, nil
}

func ensureBackgroundLayerCount(style *ComputedStyle, count int) {
	if count <= 0 {
		return
	}
	for len(style.BackgroundLayers) < count {
		style.BackgroundLayers = append(style.BackgroundLayers, BackgroundLayer{
			SizeX:      Keyword("auto"),
			SizeY:      Keyword("auto"),
			PosX:       Number(0),
			PosY:       Number(0),
			PosXOrigin: "left",
			PosYOrigin: "top",
			RepeatX:    Keyword("repeat"),
			RepeatY:    Keyword("repeat"),
			Origin:     "padding-box",
			Clip:       "border-box",
			Attachment: "scroll",
		})
	}
}

func syncPrimaryBackgroundFields(style *ComputedStyle) {
	if style == nil {
		return
	}
	if len(style.BackgroundLayers) == 0 {
		style.BackgroundImage = ""
		style.BackgroundSizeX = Keyword("auto")
		style.BackgroundSizeY = Keyword("auto")
		style.BackgroundPosX = Number(0)
		style.BackgroundPosY = Number(0)
		style.BackgroundPosXOrigin = "left"
		style.BackgroundPosYOrigin = "top"
		style.BackgroundRepeatX = Keyword("repeat")
		style.BackgroundRepeatY = Keyword("repeat")
		style.BackgroundOrigin = "padding-box"
		style.BackgroundClip = "border-box"
		style.BackgroundAttachment = "scroll"
		return
	}
	first := style.BackgroundLayers[0]
	style.BackgroundImage = first.Image
	style.BackgroundSizeX = first.SizeX
	style.BackgroundSizeY = first.SizeY
	style.BackgroundPosX = first.PosX
	style.BackgroundPosY = first.PosY
	style.BackgroundPosXOrigin = first.PosXOrigin
	style.BackgroundPosYOrigin = first.PosYOrigin
	style.BackgroundRepeatX = first.RepeatX
	style.BackgroundRepeatY = first.RepeatY
	style.BackgroundOrigin = first.Origin
	style.BackgroundClip = first.Clip
	style.BackgroundAttachment = first.Attachment
}

func parseBackground(input string, ctx parseContext) (backgroundValue, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return backgroundValue{}, fmt.Errorf("invalid background %q", input)
	}
	background := backgroundValue{
		PositionX:       Number(0),
		PositionY:       Number(0),
		PositionXOrigin: "left",
		PositionYOrigin: "top",
		RepeatX:         Keyword("repeat"),
		RepeatY:         Keyword("repeat"),
		SizeX:           Keyword("auto"),
		SizeY:           Keyword("auto"),
		Origin:          "padding-box",
		Clip:            "border-box",
		Attachment:      "scroll",
	}
	if strings.EqualFold(trimmed, "none") {
		return background, nil
	}

	parts := splitBackgroundFields(trimmed)
	positionParts := make([]string, 0, 2)
	sizeParts := make([]string, 0, 2)
	boxParts := make([]string, 0, 2)
	parsingSize := false
	for _, part := range parts {
		lower := strings.ToLower(strings.TrimSpace(part))
		if lower == "/" {
			parsingSize = true
			continue
		}
		if isBackgroundImageFunction(lower) {
			background.Image = strings.TrimSpace(part)
			continue
		}
		if strings.HasPrefix(lower, "url(") && strings.HasSuffix(lower, ")") {
			imageURL, err := parseImageURL(part)
			if err != nil {
				return backgroundValue{}, err
			}
			background.Image = imageURL
			continue
		}
		if repeatX, repeatY, err := parseBackgroundRepeat(part); err == nil {
			background.RepeatX = repeatX
			background.RepeatY = repeatY
			continue
		}
		if attachment, err := parseBackgroundAttachmentList(part); err == nil && len(attachment) == 1 {
			background.Attachment = attachment[0]
			continue
		}
		if boxValues, err := parseBackgroundBoxList(part); err == nil && len(boxValues) == 1 {
			boxParts = append(boxParts, boxValues[0])
			continue
		}
		if color, err := parseColorValue(part); err == nil {
			background.Color = &color
			continue
		}
		if parsingSize {
			sizeParts = append(sizeParts, part)
			continue
		}
		positionParts = append(positionParts, part)
	}
	if len(positionParts) > 0 {
		x, y, xOrigin, yOrigin, err := parseBackgroundPosition(strings.Join(positionParts, " "), ctx)
		if err != nil {
			return backgroundValue{}, err
		}
		background.PositionX = x
		background.PositionY = y
		background.PositionXOrigin = xOrigin
		background.PositionYOrigin = yOrigin
	}
	if len(sizeParts) > 0 {
		x, y, err := parseBackgroundSize(strings.Join(sizeParts, " "), ctx)
		if err != nil {
			return backgroundValue{}, err
		}
		background.SizeX = x
		background.SizeY = y
	}
	if len(boxParts) == 1 {
		background.Origin = boxParts[0]
		background.Clip = boxParts[0]
	}
	if len(boxParts) >= 2 {
		background.Origin = boxParts[0]
		background.Clip = boxParts[1]
	}
	return background, nil
}

func parseImageURL(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" || strings.EqualFold(trimmed, "none") {
		return "", nil
	}
	lower := strings.ToLower(trimmed)
	if !strings.HasPrefix(lower, "url(") || !strings.HasSuffix(trimmed, ")") {
		return "", fmt.Errorf("unsupported image %q", input)
	}
	inner := strings.TrimSpace(trimmed[len("url(") : len(trimmed)-1])
	inner = strings.Trim(inner, `"'`)
	if inner == "" {
		return "", fmt.Errorf("unsupported image %q", input)
	}
	return inner, nil
}

// isBackgroundImageFunction recognizes CSS image functions that can occupy
// the image slot of a background shorthand. Keep the raw function text in
// computed style so the paint layer can select the appropriate image
// implementation without mistaking its arguments for background-position.
func isBackgroundImageFunction(input string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(input))
	open := strings.IndexByte(trimmed, '(')
	if open <= 0 || !strings.HasSuffix(trimmed, ")") || !balancedCSSFunction(trimmed[open:]) {
		return false
	}
	name := trimmed[:open]
	switch {
	case name == "linear-gradient":
		return true
	case name == "repeating-linear-gradient":
		return true
	case name == "radial-gradient":
		return true
	case name == "repeating-radial-gradient":
		return true
	case name == "conic-gradient":
		return true
	case name == "repeating-conic-gradient":
		return true
	default:
		return false
	}
}

func balancedCSSFunction(input string) bool {
	depth := 0
	var quote byte
	comment := false
	escaped := false
	for index := 0; index < len(input); index++ {
		ch := input[index]
		if comment {
			if ch == '*' && index+1 < len(input) && input[index+1] == '/' {
				comment = false
				index++
			}
			continue
		}
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case quote:
				quote = 0
			}
			continue
		}
		if ch == '/' && index+1 < len(input) && input[index+1] == '*' {
			comment = true
			index++
			continue
		}
		switch ch {
		case '\'', '"':
			quote = ch
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return false
			}
			depth--
		}
	}
	return !comment && quote == 0 && depth == 0
}

func parseBackgroundRepeat(input string) (Value, Value, error) {
	parts := splitCSSFields(strings.ToLower(strings.TrimSpace(input)))
	if len(parts) == 0 || len(parts) > 2 {
		return Value{}, Value{}, fmt.Errorf("invalid background-repeat %q", input)
	}
	if len(parts) == 1 {
		switch parts[0] {
		case "repeat-x":
			return Keyword("repeat"), Keyword("no-repeat"), nil
		case "repeat-y":
			return Keyword("no-repeat"), Keyword("repeat"), nil
		case "repeat", "no-repeat":
			return Keyword(parts[0]), Keyword(parts[0]), nil
		default:
			return Value{}, Value{}, fmt.Errorf("invalid background-repeat %q", input)
		}
	}
	x, err := parseKeyword(parts[0], "repeat", "no-repeat")
	if err != nil {
		return Value{}, Value{}, err
	}
	y, err := parseKeyword(parts[1], "repeat", "no-repeat")
	if err != nil {
		return Value{}, Value{}, err
	}
	return x, y, nil
}

func parseBackgroundPosition(input string, ctx parseContext) (Value, Value, string, string, error) {
	parts := splitCSSFields(strings.ToLower(strings.TrimSpace(input)))
	if len(parts) == 0 || len(parts) > 4 {
		return Value{}, Value{}, "", "", fmt.Errorf("invalid background-position %q", input)
	}
	x := LengthPx(0)
	y := LengthPx(0)
	xOrigin := "left"
	yOrigin := "top"
	xSet := false
	ySet := false
	if len(parts) == 1 {
		value, axis, origin, err := parseBackgroundPositionToken(parts[0], ctx)
		if err != nil {
			return Value{}, Value{}, "", "", err
		}
		if axis == "y" {
			return Keyword("center"), value, "center", origin, nil
		}
		if axis == "x" {
			return value, Keyword("center"), origin, "center", nil
		}
		return value, Keyword("center"), origin, "center", nil
	}
	if len(parts) == 2 {
		if parts[0] == "center" {
			if isBackgroundHorizontalKeyword(parts[1]) {
				return LengthPx(0), Keyword("center"), parts[1], "center", nil
			}
			if isBackgroundVerticalKeyword(parts[1]) {
				return Keyword("center"), LengthPx(0), "center", parts[1], nil
			}
		}
		if parts[1] == "center" {
			if isBackgroundHorizontalKeyword(parts[0]) {
				return LengthPx(0), Keyword("center"), parts[0], "center", nil
			}
			if isBackgroundVerticalKeyword(parts[0]) {
				return Keyword("center"), LengthPx(0), "center", parts[0], nil
			}
		}
	}
	for i := 0; i < len(parts); i++ {
		value, axis, origin, err := parseBackgroundPositionToken(parts[i], ctx)
		if err != nil {
			return Value{}, Value{}, "", "", err
		}
		switch axis {
		case "x":
			if xSet {
				return Value{}, Value{}, "", "", fmt.Errorf("invalid background-position %q", input)
			}
			x = value
			xOrigin = origin
			xSet = true
			if i+1 < len(parts) {
				if offset, ok := parseBackgroundPositionOffset(parts[i+1], ctx); ok && !isBackgroundKeyword(parts[i+1]) {
					x = offset
					i++
				}
			}
		case "y":
			if ySet {
				return Value{}, Value{}, "", "", fmt.Errorf("invalid background-position %q", input)
			}
			y = value
			yOrigin = origin
			ySet = true
			if i+1 < len(parts) {
				if offset, ok := parseBackgroundPositionOffset(parts[i+1], ctx); ok && !isBackgroundKeyword(parts[i+1]) {
					y = offset
					i++
				}
			}
		default:
			if !xSet {
				x = value
				xOrigin = origin
				xSet = true
			} else if !ySet {
				y = value
				yOrigin = origin
				ySet = true
			} else {
				return Value{}, Value{}, "", "", fmt.Errorf("invalid background-position %q", input)
			}
		}
	}
	if !xSet {
		x = Keyword("center")
		xOrigin = "center"
	}
	if !ySet {
		y = Keyword("center")
		yOrigin = "center"
	}
	return x, y, xOrigin, yOrigin, nil
}

func parseBackgroundPositionToken(input string, ctx parseContext) (Value, string, string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(input))
	switch trimmed {
	case "left", "right":
		return LengthPx(0), "x", trimmed, nil
	case "top", "bottom":
		return LengthPx(0), "y", trimmed, nil
	case "center":
		return Number(0.5), "both", "center", nil
	}
	if strings.HasSuffix(trimmed, "%") {
		value, err := strconv.ParseFloat(strings.TrimSuffix(trimmed, "%"), 64)
		if err != nil {
			return Value{}, "", "", fmt.Errorf("invalid background-position %q", input)
		}
		return Number(value / 100), "both", "left", nil
	}
	value, err := parseLength(trimmed, ctx)
	if err != nil {
		return Value{}, "", "", fmt.Errorf("invalid background-position %q", input)
	}
	return value, "both", "left", nil
}

func parseBackgroundPositionOffset(input string, ctx parseContext) (Value, bool) {
	trimmed := strings.ToLower(strings.TrimSpace(input))
	if strings.HasSuffix(trimmed, "%") {
		value, err := strconv.ParseFloat(strings.TrimSuffix(trimmed, "%"), 64)
		if err != nil {
			return Value{}, false
		}
		return Number(value / 100), true
	}
	value, err := parseLength(input, ctx)
	if err != nil {
		return Value{}, false
	}
	return value, true
}

func isBackgroundHorizontalKeyword(input string) bool {
	return input == "left" || input == "right"
}

func isBackgroundVerticalKeyword(input string) bool {
	return input == "top" || input == "bottom"
}

func isBackgroundKeyword(input string) bool {
	return isBackgroundHorizontalKeyword(input) || isBackgroundVerticalKeyword(input) || input == "center"
}

func parseBackgroundSize(input string, ctx parseContext) (Value, Value, error) {
	parts := splitCSSFields(strings.ToLower(strings.TrimSpace(input)))
	if len(parts) == 0 || len(parts) > 2 {
		return Value{}, Value{}, fmt.Errorf("invalid background-size %q", input)
	}
	if len(parts) == 1 {
		value, err := parseBackgroundSizeToken(parts[0], ctx)
		if err != nil {
			return Value{}, Value{}, err
		}
		if value.Kind == ValueKeyword && (value.Keyword == "cover" || value.Keyword == "contain") {
			return value, value, nil
		}
		return value, Keyword("auto"), nil
	}
	x, err := parseBackgroundSizeToken(parts[0], ctx)
	if err != nil {
		return Value{}, Value{}, err
	}
	y, err := parseBackgroundSizeToken(parts[1], ctx)
	if err != nil {
		return Value{}, Value{}, err
	}
	return x, y, nil
}

func parseBackgroundSizeToken(input string, ctx parseContext) (Value, error) {
	trimmed := strings.ToLower(strings.TrimSpace(input))
	if trimmed == "auto" || trimmed == "cover" || trimmed == "contain" {
		return Keyword(trimmed), nil
	}
	if strings.HasSuffix(trimmed, "%") {
		value, err := strconv.ParseFloat(strings.TrimSuffix(trimmed, "%"), 64)
		if err != nil {
			return Value{}, fmt.Errorf("invalid background-size %q", input)
		}
		return Number(value / 100), nil
	}
	value, err := parseLength(trimmed, ctx)
	if err != nil {
		return Value{}, fmt.Errorf("invalid background-size %q", input)
	}
	return value, nil
}

func splitBackgroundFields(input string) []string {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	parts := make([]string, 0, 6)
	var builder strings.Builder
	depth := 0
	for _, r := range input {
		switch {
		case r == '(':
			depth++
			builder.WriteRune(r)
		case r == ')':
			if depth > 0 {
				depth--
			}
			builder.WriteRune(r)
		case depth == 0 && (r == ' ' || r == '\t' || r == '\n' || r == '\r'):
			if builder.Len() == 0 {
				continue
			}
			parts = append(parts, builder.String())
			builder.Reset()
		case depth == 0 && r == '/':
			if builder.Len() > 0 {
				parts = append(parts, builder.String())
				builder.Reset()
			}
			parts = append(parts, "/")
		default:
			builder.WriteRune(r)
		}
	}
	if builder.Len() > 0 {
		parts = append(parts, builder.String())
	}
	return parts
}

func parseFilterMultiplier(input string) (float64, error) {
	trimmed := strings.TrimSpace(strings.ToLower(input))
	percentage := strings.HasSuffix(trimmed, "%")
	value, err := strconv.ParseFloat(strings.TrimSuffix(trimmed, "%"), 64)
	if err != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("invalid filter amount %q", input)
	}
	if percentage {
		value /= 100
	}
	return value, nil
}

func parseAngleDegrees(input string) (float64, error) {
	trimmed := strings.TrimSpace(strings.ToLower(input))
	if trimmed == "0" {
		return 0, nil
	}

	for _, suffix := range []string{"deg", "rad", "turn", "grad"} {
		if !strings.HasSuffix(trimmed, suffix) {
			continue
		}
		number := strings.TrimSpace(strings.TrimSuffix(trimmed, suffix))
		value, err := strconv.ParseFloat(number, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid angle %q", input)
		}
		switch suffix {
		case "deg":
			return value, nil
		case "rad":
			return value * 180 / math.Pi, nil
		case "turn":
			return value * 360, nil
		case "grad":
			return value * 0.9, nil
		}
	}

	return 0, fmt.Errorf("invalid angle %q", input)
}

func collectFilterDefinitions(root *dom.Node) (filterDefinitions, error) {
	var definitions filterDefinitions
	if err := collectFilterDefinitionsInto(root, &definitions); err != nil {
		return nil, err
	}
	return definitions, nil
}

func collectFilterDefinitionsAndCount(root *dom.Node) (int, filterDefinitions, error) {
	var definitions filterDefinitions
	nodeCount, err := collectFilterDefinitionsAndCountInto(root, &definitions)
	if err != nil {
		return 0, nil, err
	}
	return nodeCount, definitions, nil
}

func collectFilterDefinitionsAndCountInto(node *dom.Node, definitions *filterDefinitions) (int, error) {
	if node == nil {
		return 0, nil
	}
	nodeCount := 1
	if err := collectFilterDefinition(node, definitions); err != nil {
		return 0, err
	}
	for _, child := range node.Children {
		childCount, err := collectFilterDefinitionsAndCountInto(child, definitions)
		if err != nil {
			return 0, err
		}
		nodeCount += childCount
	}
	return nodeCount, nil
}

func collectFilterDefinitionsInto(node *dom.Node, definitions *filterDefinitions) error {
	if node == nil {
		return nil
	}
	if err := collectFilterDefinition(node, definitions); err != nil {
		return err
	}
	for _, child := range node.Children {
		if err := collectFilterDefinitionsInto(child, definitions); err != nil {
			return err
		}
	}
	return nil
}

func collectFilterDefinition(node *dom.Node, definitions *filterDefinitions) error {
	if node.Type != dom.ElementNode || !strings.EqualFold(node.Data, "filter") {
		return nil
	}
	id, ok := node.Attr("id")
	id = strings.TrimSpace(id)
	if !ok || id == "" {
		return nil
	}
	compiled, err := compileFilterDefinition(node)
	if err != nil {
		return err
	}
	if *definitions == nil {
		*definitions = make(filterDefinitions)
	}
	(*definitions)[id] = compiled
	return nil
}

func compileFilterDefinition(node *dom.Node) ([]Filter, error) {
	filters := make([]Filter, 0, len(node.Children))
	for _, child := range node.Children {
		if child == nil || child.Type != dom.ElementNode {
			continue
		}
		compiled, err := compileFilterPrimitive(child)
		if err != nil {
			return nil, err
		}
		filters = append(filters, compiled...)
	}
	return filters, nil
}

func compileFilterPrimitive(node *dom.Node) ([]Filter, error) {
	switch {
	case strings.EqualFold(node.Data, "fegaussianblur"):
		value, err := parseSVGNumberPair(firstAttr(node, "stdDeviation", "stddeviation"))
		if err != nil {
			return nil, err
		}
		return []Filter{{Kind: FilterBlur, Amount: value}}, nil
	case strings.EqualFold(node.Data, "fecolormatrix"):
		return compileColorMatrixPrimitive(node)
	case strings.EqualFold(node.Data, "fecomponenttransfer"):
		filter, err := compileComponentTransferPrimitive(node)
		if err != nil {
			return nil, err
		}
		return []Filter{filter}, nil
	case strings.EqualFold(node.Data, "fedropshadow"):
		shadow, err := compileDropShadowPrimitive(node)
		if err != nil {
			return nil, err
		}
		return []Filter{{Kind: FilterDropShadow, Shadow: shadow}}, nil
	default:
		return nil, fmt.Errorf("unsupported svg filter primitive <%s>", node.Data)
	}
}

func compileColorMatrixPrimitive(node *dom.Node) ([]Filter, error) {
	typeValue := strings.ToLower(strings.TrimSpace(firstAttr(node, "type")))
	if typeValue == "" || typeValue == "matrix" {
		matrix, err := parseColorMatrixValues(firstAttr(node, "values"))
		if err != nil {
			return nil, err
		}
		return []Filter{{Kind: FilterColorMatrix, Matrix: matrix}}, nil
	}
	if typeValue == "saturate" {
		amount, err := parseSVGNumber(firstAttr(node, "values"))
		if err != nil {
			return nil, err
		}
		return []Filter{{Kind: FilterSaturate, Amount: amount}}, nil
	}
	if typeValue == "huerotate" || typeValue == "hue-rotate" {
		amount, err := parseSVGNumber(firstAttr(node, "values"))
		if err != nil {
			return nil, err
		}
		return []Filter{{Kind: FilterHueRotate, Amount: amount}}, nil
	}
	return nil, fmt.Errorf("unsupported feColorMatrix type %q", typeValue)
}

func compileComponentTransferPrimitive(node *dom.Node) (Filter, error) {
	matrix := identityColorMatrix()
	for _, child := range node.Children {
		if child == nil || child.Type != dom.ElementNode {
			continue
		}
		channel, ok := componentTransferChannel(child.Data)
		if !ok {
			continue
		}
		typeValue := strings.ToLower(strings.TrimSpace(firstAttr(child, "type")))
		if typeValue == "" || typeValue == "identity" {
			continue
		}
		if typeValue != "linear" {
			return Filter{}, fmt.Errorf("unsupported feComponentTransfer type %q", typeValue)
		}
		slope, err := parseSVGNumberOrDefault(firstAttr(child, "slope"), 1)
		if err != nil {
			return Filter{}, err
		}
		intercept, err := parseSVGNumberOrDefault(firstAttr(child, "intercept"), 0)
		if err != nil {
			return Filter{}, err
		}
		for i := channel * 5; i < channel*5+5; i++ {
			matrix[i] = 0
		}
		matrix[channel*5+channel] = slope
		matrix[channel*5+4] = intercept
	}
	return Filter{Kind: FilterColorMatrix, Matrix: matrix}, nil
}

func compileDropShadowPrimitive(node *dom.Node) (Shadow, error) {
	dx, err := parseSVGNumberOrDefault(firstAttr(node, "dx"), 0)
	if err != nil {
		return Shadow{}, err
	}
	dy, err := parseSVGNumberOrDefault(firstAttr(node, "dy"), 0)
	if err != nil {
		return Shadow{}, err
	}
	stdDeviation, err := parseSVGNumberOrDefault(firstAttr(node, "stdDeviation", "stddeviation"), 0)
	if err != nil {
		return Shadow{}, err
	}
	color, err := parseColorOrDefault(firstAttr(node, "flood-color", "floodcolor"), Color{R: 0, G: 0, B: 0, A: 255})
	if err != nil {
		return Shadow{}, err
	}
	if opacityAttr := firstAttr(node, "flood-opacity", "floodopacity"); strings.TrimSpace(opacityAttr) != "" {
		opacity, err := parseFilterMultiplier(opacityAttr)
		if err != nil {
			return Shadow{}, err
		}
		color.A = uint8(math.Round(opacity * 255))
	}
	return Shadow{
		OffsetX:    LengthPx(dx),
		OffsetY:    LengthPx(dy),
		BlurRadius: LengthPx(stdDeviation),
		Color:      color,
	}, nil
}

func parseColorMatrixValues(input string) ([20]float64, error) {
	parts := splitNumericFields(input)
	if len(parts) != 20 {
		return [20]float64{}, fmt.Errorf("feColorMatrix expects 20 values, got %q", input)
	}
	var matrix [20]float64
	for i, part := range parts {
		value, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return [20]float64{}, fmt.Errorf("invalid feColorMatrix value %q", part)
		}
		matrix[i] = value
	}
	return matrix, nil
}

func parseSVGNumberPair(input string) (float64, error) {
	parts := splitNumericFields(input)
	if len(parts) == 0 {
		return 0, nil
	}
	if len(parts) == 1 {
		return parseSVGNumber(parts[0])
	}
	left, err := parseSVGNumber(parts[0])
	if err != nil {
		return 0, err
	}
	right, err := parseSVGNumber(parts[1])
	if err != nil {
		return 0, err
	}
	return (left + right) / 2, nil
}

func parseSVGNumberOrDefault(input string, fallback float64) (float64, error) {
	if strings.TrimSpace(input) == "" {
		return fallback, nil
	}
	return parseSVGNumber(input)
}

func parseSVGNumber(input string) (float64, error) {
	trimmed := strings.TrimSpace(strings.ToLower(input))
	trimmed = strings.TrimSuffix(trimmed, "px")
	if trimmed == "" {
		return 0, fmt.Errorf("invalid svg number %q", input)
	}
	value, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid svg number %q", input)
	}
	return value, nil
}

func parseColorOrDefault(input string, fallback Color) (Color, error) {
	if strings.TrimSpace(input) == "" {
		return fallback, nil
	}
	return parseColor(input)
}

func firstAttr(node *dom.Node, names ...string) string {
	for _, name := range names {
		if value, ok := node.Attr(name); ok {
			return value
		}
	}
	return ""
}

func splitNumericFields(input string) []string {
	replacer := strings.NewReplacer(",", " ", "\n", " ", "\t", " ", "\r", " ")
	return strings.Fields(replacer.Replace(strings.TrimSpace(input)))
}

func identityColorMatrix() [20]float64 {
	return [20]float64{
		1, 0, 0, 0, 0,
		0, 1, 0, 0, 0,
		0, 0, 1, 0, 0,
		0, 0, 0, 1, 0,
	}
}

func componentTransferChannel(name string) (int, bool) {
	switch strings.ToLower(name) {
	case "fefuncr":
		return 0, true
	case "fefuncg":
		return 1, true
	case "fefuncb":
		return 2, true
	case "fefunca":
		return 3, true
	default:
		return 0, false
	}
}

func cssFieldsInto(input string, fields []string) (int, bool) {
	count := 0
	for part := range cssValues(input, true) {
		if count == len(fields) {
			return count, false
		}
		fields[count] = part
		count++
	}
	return count, true
}

func splitCSSList(input string) []string   { return splitCSSValues(input, false) }
func splitCSSFields(input string) []string { return splitCSSValues(input, true) }

// Split only at top-level separators; quoted animation names and function
// arguments keep their original token boundaries without copying strings.
func splitCSSValues(input string, fields bool) []string {
	if strings.TrimSpace(input) == "" {
		return nil
	}
	parts := make([]string, 0, 4)
	for value := range cssValues(input, fields) {
		parts = append(parts, value)
	}
	return parts
}

func cssValues(input string, fields bool) iter.Seq[string] {
	input = strings.TrimSpace(input)
	return func(yield func(string) bool) {
		if input == "" {
			return
		}
		depth, start := 0, 0
		var quote byte
		escaped := false
		for i := 0; i < len(input); i++ {
			c := input[i]
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if quote != 0 {
				if c == quote {
					quote = 0
				}
				continue
			}
			if c == '\'' || c == '"' {
				quote = c
				continue
			}
			if c == '(' {
				depth++
				continue
			}
			if c == ')' {
				if depth > 0 {
					depth--
				}
				continue
			}
			separator := c == ','
			if fields {
				separator = isCSSWhitespace(c)
			}
			if depth == 0 && separator {
				value := strings.TrimSpace(input[start:i])
				if !fields || value != "" {
					if !yield(value) {
						return
					}
				}
				start = i + 1
			}
		}
		value := strings.TrimSpace(input[start:])
		if !fields || value != "" {
			yield(value)
		}
	}
}

func parseLineHeight(input string, fontSize Value, ctx parseContext) (Value, error) {
	trimmed := strings.TrimSpace(strings.ToLower(input))
	if trimmed == "normal" {
		return LengthPx(fontSize.LengthPx * 1.2), nil
	}

	if value, err := parseLength(trimmed, ctx); err == nil {
		if value.Kind == ValueLengthPercent || value.Kind == ValueLengthCalc {
			return LengthPx(value.LengthPx + fontSize.LengthPx*value.Percent), nil
		}
		return value, nil
	}

	multiplier, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return Value{}, fmt.Errorf("invalid line-height %q", input)
	}
	return LengthPx(fontSize.LengthPx * multiplier), nil
}

func applyFontShorthand(computed *ComputedStyle, raw string, ctx parseContext) error {
	if computed == nil {
		return nil
	}
	fontSize, linePart, family, err := parseFontShorthandParts(raw, ctx)
	if err != nil {
		return err
	}
	computed.FontSize = fontSize
	computed.LineHeight = LengthPx(fontSize.LengthPx * 1.2)
	computed.LineHeightNormal = true
	if linePart != "" {
		lineHeight, err := parseLineHeight(linePart, fontSize, ctx)
		if err != nil {
			return err
		}
		computed.LineHeight = lineHeight
		computed.LineHeightNormal = strings.EqualFold(strings.TrimSpace(linePart), "normal")
	}
	if family != "" {
		computed.FontFamily = Keyword(family)
	}
	return nil
}

func parseFontShorthandParts(raw string, ctx parseContext) (Value, string, string, error) {
	parts := splitCSSFields(strings.TrimSpace(raw))
	if len(parts) == 0 {
		return Value{}, "", "", fmt.Errorf("invalid font shorthand %q", raw)
	}
	sizeIndex := -1
	var sizePart, linePart string
	for index, part := range parts {
		candidate, candidateLine, hasLine := strings.Cut(part, "/")
		if value, err := parseFontSize(candidate, ctx); err == nil && value.Kind == ValueLengthPx {
			sizeIndex = index
			sizePart, linePart = candidate, candidateLine
			if !hasLine {
				linePart = ""
			}
			break
		}
	}
	if sizeIndex < 0 {
		return Value{}, "", "", fmt.Errorf("font shorthand is missing a size: %q", raw)
	}
	fontSize, err := parseFontSize(sizePart, ctx)
	if err != nil {
		return Value{}, "", "", err
	}
	family := strings.TrimSpace(strings.Join(parts[sizeIndex+1:], " "))
	family = strings.Trim(strings.TrimSpace(family), `"'`)
	return fontSize, linePart, family, nil
}

func parseColor(input string) (Color, error) {
	input = strings.TrimSpace(input)
	if isColorMix(input) {
		mixed, err := parseMixedColor(input, 0)
		if err != nil {
			return Color{}, err
		}
		if mixed.Current != 0 {
			return Color{}, errCSSColor
		}
		return mixed.Color(), nil
	}
	trimmed := input
	if len(trimmed) >= 4 && strings.EqualFold(trimmed[:4], "rgb(") || len(trimmed) >= 5 && strings.EqualFold(trimmed[:5], "rgba(") {
		return parseFunctionalColor(trimmed)
	}

	switch {
	case strings.EqualFold(trimmed, "black"):
		return Color{R: 0, G: 0, B: 0, A: 255}, nil
	case strings.EqualFold(trimmed, "white"):
		return Color{R: 255, G: 255, B: 255, A: 255}, nil
	case strings.EqualFold(trimmed, "red"):
		return Color{R: 255, G: 0, B: 0, A: 255}, nil
	case strings.EqualFold(trimmed, "green"):
		return Color{R: 0, G: 128, B: 0, A: 255}, nil
	case strings.EqualFold(trimmed, "blue"):
		return Color{R: 0, G: 0, B: 255, A: 255}, nil
	case strings.EqualFold(trimmed, "transparent"):
		return Color{A: 0}, nil
	}

	if !strings.HasPrefix(trimmed, "#") {
		return Color{}, fmt.Errorf("unsupported color %q", input)
	}

	hex := strings.TrimPrefix(trimmed, "#")
	if len(hex) == 3 || len(hex) == 4 {
		value, err := strconv.ParseUint(hex, 16, 16)
		if err != nil {
			return Color{}, err
		}
		a := uint8(255)
		if len(hex) == 4 {
			a = uint8(value&15) * 17
			value >>= 4
		}
		return Color{R: uint8(value>>8) * 17, G: uint8(value>>4&15) * 17, B: uint8(value&15) * 17, A: a}, nil
	}

	if len(hex) == 8 {
		value, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return Color{}, fmt.Errorf("parse color %q: %w", input, err)
		}

		return Color{
			R: uint8(value >> 24),
			G: uint8(value >> 16),
			B: uint8(value >> 8),
			A: uint8(value),
		}, nil
	}

	if len(hex) != 6 {
		return Color{}, fmt.Errorf("unsupported hex color %q", input)
	}

	value, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return Color{}, fmt.Errorf("parse color %q: %w", input, err)
	}

	return Color{
		R: uint8(value >> 16),
		G: uint8(value >> 8),
		B: uint8(value),
		A: 255,
	}, nil
}

func parseFunctionalColor(input string) (Color, error) {
	r, g, b, a, err := parseRGBComponents(input)
	if err != nil {
		return Color{}, err
	}
	return Color{R: uint8(math.Round(r * 255)), G: uint8(math.Round(g * 255)), B: uint8(math.Round(b * 255)), A: uint8(math.Round(a * 255))}, nil
}
