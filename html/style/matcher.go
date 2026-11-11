package style

import (
	"iter"
	"strings"
	"sync"
	"weak"

	"github.com/onix-engine/onix/html/css"
	"github.com/onix-engine/onix/html/dom"
)

type compiledStylesheet struct {
	byID      map[string][]compiledSelectorRef
	byClass   map[string][]compiledSelectorRef
	byTag     map[string][]compiledSelectorRef
	keyframes map[string]compiledKeyframeGroup
	universal []compiledSelectorRef
	// Parallel to stylesheet.Rules: declarations with normalized property
	// names and pre-parsed rule conditions, prepared once per stylesheet
	// instead of per matched node.
	declarations     [][]preparedDeclaration
	conditions       [][]compiledCondition
	documentMutation bool
	siblingMutation  bool
}

type compiledKeyframeGroup struct {
	definitions []compiledKeyframes
	properties  AnimatedProperties
}

type compiledKeyframes struct {
	frames     *css.Keyframes
	conditions []compiledCondition
}

type preparedDeclaration struct {
	parsed    *preparedValue
	value     string
	order     cascadePropertyOrder
	important bool
}

func prepareDeclarations(declarations []css.Declaration) []preparedDeclaration {
	prepared := make([]preparedDeclaration, 0, len(declarations))
	for _, declaration := range declarations {
		property := strings.ToLower(strings.TrimSpace(declaration.Property))
		value := strings.TrimSpace(declaration.Value)
		if !validateDeclaration(property, value) {
			continue
		}
		prepared = append(prepared, preparedDeclaration{
			value:     value,
			important: declaration.Important,
			order:     cascadePropertyIndex[property],
			parsed:    preparePropertyValue(property, value),
		})
	}
	return prepared
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// validateDeclaration parses a property against a context-free default. Values
// whose grammar depends on layout or font metrics remain deferred to cascade.
func validateDeclaration(property, value string) bool {
	if property == "" || value == "" || strings.HasPrefix(property, "--") {
		return false
	}
	if isCSSWideKeyword(value) {
		return true
	}
	if property == "line-height" {
		_, err := parseLineHeight(value, LengthPx(16), parseContext{FontSize: 16, ParentFontSize: 16, RootFontSize: 16})
		return err == nil
	}
	if property == "font" {
		ctx := parseContext{FontSize: 16, ParentFontSize: 16, RootFontSize: 16}
		parts, err := parseFontShorthandParts(value, ctx)
		size, line := parts.size, parts.line
		if err != nil {
			return false
		}
		if line != "" {
			_, err = parseLineHeight(line, size, ctx)
		}
		return err == nil
	}
	if _, known := cascadePropertyIndex[property]; !known {
		return true
	}
	if property == "font-size" {
		_, err := parseFontSize(value, parseContext{FontSize: 16, ParentFontSize: 16, RootFontSize: 16})
		return err == nil
	}
	if property == "font-family" || property == "grid-template-columns" || property == "grid-template-rows" || property == "container-name" {
		return true
	}
	if property == "filter" {
		_, err := parseFilter(value, nil, parseContext{FontSize: 16, ParentFontSize: 16, RootFontSize: 16})
		return err == nil || strings.Contains(strings.ToLower(value), "url(")
	}
	computed := defaultStyle(nil)
	return applyProperty(&computed, property, value, nil, parseContext{FontSize: 16, ParentFontSize: 16, RootFontSize: 16}) == nil
}

type compiledSelectorRef struct {
	selector       css.Selector
	specificity    css.Specificity
	ruleIndex      int
	orderBase      int
	inputDependent bool
}

var compiledStylesheetCache struct {
	stylesheet weak.Pointer[css.Stylesheet]
	compiled   weak.Pointer[compiledStylesheet]
	sync.Mutex
}

// Stylesheets are immutable after parsing, so pointer identity is enough to
// cache the compiled matcher. This convenience cache owns neither object;
// retained documents keep their matcher alive through MutationCache.compiled.
func compiledMatcher(stylesheet *css.Stylesheet) *compiledStylesheet {
	if stylesheet == nil {
		return nil
	}
	compiledStylesheetCache.Lock()
	defer compiledStylesheetCache.Unlock()
	if compiledStylesheetCache.stylesheet.Value() == stylesheet {
		if compiled := compiledStylesheetCache.compiled.Value(); compiled != nil {
			return compiled
		}
	}
	compiled := compileStylesheet(stylesheet)
	compiledStylesheetCache.stylesheet = weak.Make(stylesheet)
	compiledStylesheetCache.compiled = weak.Make(compiled)
	return compiled
}

func compileStylesheet(stylesheet *css.Stylesheet) *compiledStylesheet {
	if stylesheet == nil {
		return nil
	}
	compiled := &compiledStylesheet{
		byID:    map[string][]compiledSelectorRef{},
		byClass: map[string][]compiledSelectorRef{},
		byTag:   map[string][]compiledSelectorRef{},
	}
	orderBase := 0
	for ruleIndex, rule := range stylesheet.Rules {
		for _, condition := range rule.Conditions {
			// Media queries depend on the render environment, which already
			// invalidates the whole document when it changes. Container queries
			// can depend on geometry outside the mutated descendant branch.
			if condition.Kind != css.RuleConditionMedia {
				compiled.documentMutation = true
			}
		}
		compiled.declarations = append(compiled.declarations, prepareDeclarations(rule.Declarations))
		compiled.conditions = append(compiled.conditions, compileRuleConditions(rule.Conditions))
		for _, selector := range rule.Selectors {
			compiled.documentMutation = compiled.documentMutation || selectorNeedsDocumentMutation(selector)
			compiled.siblingMutation = compiled.siblingMutation || selectorNeedsSiblingMutation(selector)
			ref := compiledSelectorRef{
				ruleIndex:      ruleIndex,
				selector:       selector,
				specificity:    css.CalculateSpecificity(selector),
				orderBase:      orderBase,
				inputDependent: selectorDependsOnInput(selector),
			}
			compiled.add(selector, ref)
			orderBase += len(rule.Declarations)
		}
	}
	if len(stylesheet.Keyframes) != 0 {
		compiled.keyframes = make(map[string]compiledKeyframeGroup, len(stylesheet.Keyframes))
		for index := range stylesheet.Keyframes {
			frames := &stylesheet.Keyframes[index]
			group := compiled.keyframes[frames.Name]
			group.definitions = append(group.definitions, compiledKeyframes{frames: frames, conditions: compileRuleConditions(frames.Conditions)})
			for _, frame := range frames.Frames {
				for _, declaration := range frame.Declarations {
					if !declaration.Important {
						group.properties |= AnimatedProperty(declaration.Property)
					}
				}
			}
			compiled.keyframes[frames.Name] = group
		}
	}
	return compiled
}

func (c *compiledStylesheet) add(selector css.Selector, ref compiledSelectorRef) {
	kind, key := selectorBucket(selector)
	switch kind {
	case selectorBucketID:
		c.byID[key] = append(c.byID[key], ref)
	case selectorBucketClass:
		c.byClass[key] = append(c.byClass[key], ref)
	case selectorBucketTag:
		c.byTag[key] = append(c.byTag[key], ref)
	default:
		c.universal = append(c.universal, ref)
	}
}

// Candidate order is irrelevant: cascadeWeight includes source order. Stream
// the indexed buckets without a per-node copy, sort or retained candidate slice.
func (c *compiledStylesheet) candidates(node *dom.Node) iter.Seq[compiledSelectorRef] {
	return func(yield func(compiledSelectorRef) bool) {
		if c == nil || node == nil || node.Type != dom.ElementNode {
			return
		}
		if id, ok := node.Attr("id"); ok {
			for _, ref := range c.byID[id] {
				if !yield(ref) {
					return
				}
			}
		}
		classValue, _ := node.Attr("class")
		classes := classTokenIter{value: classValue}
		for name, ok := classes.next(); ok; name, ok = classes.next() {
			for _, ref := range c.byClass[name] {
				if !yield(ref) {
					return
				}
			}
		}
		for _, ref := range c.byTag[node.Data] {
			if !yield(ref) {
				return
			}
		}
		for _, ref := range c.universal {
			if !yield(ref) {
				return
			}
		}
	}
}

type classTokenIter struct {
	value  string
	offset int
}

func (iter *classTokenIter) next() (string, bool) {
	for iter.offset < len(iter.value) && isClassWhitespace(iter.value[iter.offset]) {
		iter.offset++
	}
	start := iter.offset
	for iter.offset < len(iter.value) && !isClassWhitespace(iter.value[iter.offset]) {
		iter.offset++
	}
	return iter.value[start:iter.offset], start < iter.offset
}

func isClassWhitespace(value byte) bool {
	switch value {
	case ' ', '\t', '\n', '\r', '\f':
		return true
	default:
		return false
	}
}

type selectorBucketKind uint8

const (
	selectorBucketUniversal selectorBucketKind = iota
	selectorBucketID
	selectorBucketClass
	selectorBucketTag
)

func selectorBucket(selector css.Selector) (selectorBucketKind, string) {
	if len(selector.Parts) == 0 {
		return selectorBucketUniversal, ""
	}
	part := selector.Parts[len(selector.Parts)-1]
	if part.ID != "" {
		return selectorBucketID, part.ID
	}
	if len(part.Classes) > 0 {
		return selectorBucketClass, part.Classes[0]
	}
	if part.Tag != "" && part.Tag != "*" {
		return selectorBucketTag, part.Tag
	}
	return selectorBucketUniversal, ""
}

// Input-dependent selectors may nest inside functional pseudo-classes.
func selectorDependsOnInput(selector css.Selector) bool {
	for _, part := range selector.Parts {
		for _, pseudo := range part.PseudoClasses {
			switch pseudo.Name {
			case "hover", "active", "focus", "focus-visible", "focus-within":
				return true
			}
			for _, nested := range pseudo.Selectors {
				if selectorDependsOnInput(nested) {
					return true
				}
			}
		}
	}
	return false
}
