package style

import (
	"math"
	"strings"

	"github.com/diiyw/onix/html/css"
	"github.com/diiyw/onix/html/dom"
)

// compiledCondition is a rule condition (media or container query) parsed
// once per stylesheet instead of re-parsing the prelude string for every
// matched node.
type compiledCondition struct {
	kind css.RuleConditionKind
	// never marks conditions with an unknown kind: the original matcher
	// rejects them, so the rule can never match.
	never bool
	// media alternatives are evaluated as OR.
	media []compiledQueryAlternative
	// container name filter plus the AND-ed feature clauses.
	containerName    string
	containerClauses []compiledQueryClause
}

// HasContainerQueries reports whether a stylesheet contains rules whose
// result depends on a laid-out ancestor. Callers can use it to avoid a second
// style/layout pass for ordinary media-only documents.
func HasContainerQueries(stylesheet *css.Stylesheet) bool {
	compiled := compiledMatcher(stylesheet)
	if compiled == nil {
		return false
	}
	for _, conditions := range compiled.conditions {
		for _, condition := range conditions {
			if condition.kind == css.RuleConditionContainer {
				return true
			}
		}
	}
	return false
}

type compiledQueryAlternative struct {
	negated bool
	// matchesAll marks bodies that match unconditionally ("", "all", "screen").
	matchesAll bool
	clauses    []compiledQueryClause
}

type queryFeatureKind uint8

const (
	queryFeatureUnknown queryFeatureKind = iota
	queryFeatureWidth
	queryFeatureHeight
	queryFeatureAspectRatio
	queryFeatureOrientation
)

type queryCompareOp uint8

const (
	queryCompareEqual queryCompareOp = iota
	queryCompareMin
	queryCompareMax
)

// compiledQueryClause is one parenthesized feature such as "(min-width: 768px)"
// with the numeric value already parsed. The zero kind is an always-false
// clause, which is how unknown or unparseable features evaluated before.
type compiledQueryClause struct {
	kind        queryFeatureKind
	op          queryCompareOp
	value       float64
	portrait    bool
	requirement containerRequirement
}

func (c *compiledQueryClause) matches(width, height float64) bool {
	switch c.kind {
	case queryFeatureWidth:
		return c.compare(width)
	case queryFeatureHeight:
		return c.compare(height)
	case queryFeatureAspectRatio:
		return c.compare(aspectRatio(width, height))
	case queryFeatureOrientation:
		if c.portrait {
			return height >= width
		}
		return width >= height
	default:
		return false
	}
}

func (c *compiledQueryClause) compare(actual float64) bool {
	switch c.op {
	case queryCompareMin:
		return actual+0.01 >= c.value
	case queryCompareMax:
		return actual <= c.value+0.01
	default:
		return math.Abs(actual-c.value) < 0.01
	}
}

func compileRuleConditions(conditions []css.RuleCondition) []compiledCondition {
	if len(conditions) == 0 {
		return nil
	}
	compiled := make([]compiledCondition, 0, len(conditions))
	for _, condition := range conditions {
		compiled = append(compiled, compileQueryCondition(condition))
	}
	return compiled
}

func compileQueryCondition(condition css.RuleCondition) compiledCondition {
	switch condition.Kind {
	case css.RuleConditionMedia:
		compiled := compiledCondition{kind: css.RuleConditionMedia}
		for _, alternative := range splitTopLevelQueryList(condition.Prelude) {
			compiled.media = append(compiled.media, compileQueryAlternative(alternative))
		}
		return compiled
	case css.RuleConditionContainer:
		name, body := splitContainerQueryPrelude(condition.Prelude)
		compiled := compiledCondition{kind: css.RuleConditionContainer, containerName: name}
		compiled.containerClauses = compileQueryClauses(body)
		return compiled
	default:
		return compiledCondition{kind: condition.Kind, never: true}
	}
}

func compileQueryAlternative(alternative string) compiledQueryAlternative {
	lowered := strings.TrimSpace(strings.ToLower(alternative))
	compiled := compiledQueryAlternative{}
	if lowered == "" || lowered == "all" || lowered == "screen" {
		compiled.matchesAll = true
		return compiled
	}
	body := lowered
	if strings.HasPrefix(lowered, "not ") {
		compiled.negated = true
		body = strings.TrimSpace(strings.TrimPrefix(lowered, "not"))
	}
	compiled.clauses = compileQueryClauses(body)
	return compiled
}

func compileQueryClauses(query string) []compiledQueryClause {
	features := extractQueryFeatureClauses(query)
	compiled := make([]compiledQueryClause, 0, len(features))
	for _, feature := range features {
		if clause, ok := compileQueryClause(feature); ok {
			compiled = append(compiled, clause)
		} else {
			compiled = append(compiled, compiledQueryClause{kind: queryFeatureUnknown})
		}
	}
	return compiled
}

func compileQueryClause(feature string) (compiledQueryClause, bool) {
	name, rawValue := splitQueryFeature(feature)
	clause := compiledQueryClause{requirement: containerRequirementForFeature(name)}
	switch name {
	case "width", "inline-size":
		clause.kind = queryFeatureWidth
	case "min-width", "min-inline-size":
		clause.kind, clause.op = queryFeatureWidth, queryCompareMin
	case "max-width", "max-inline-size":
		clause.kind, clause.op = queryFeatureWidth, queryCompareMax
	case "height", "block-size":
		clause.kind = queryFeatureHeight
	case "min-height", "min-block-size":
		clause.kind, clause.op = queryFeatureHeight, queryCompareMin
	case "max-height", "max-block-size":
		clause.kind, clause.op = queryFeatureHeight, queryCompareMax
	case "aspect-ratio":
		clause.kind = queryFeatureAspectRatio
	case "min-aspect-ratio":
		clause.kind, clause.op = queryFeatureAspectRatio, queryCompareMin
	case "max-aspect-ratio":
		clause.kind, clause.op = queryFeatureAspectRatio, queryCompareMax
	case "orientation":
		switch strings.ToLower(strings.TrimSpace(rawValue)) {
		case "portrait":
			return compiledQueryClause{kind: queryFeatureOrientation, portrait: true, requirement: clause.requirement}, true
		case "landscape":
			return compiledQueryClause{kind: queryFeatureOrientation, portrait: false, requirement: clause.requirement}, true
		default:
			return compiledQueryClause{}, false
		}
	default:
		return compiledQueryClause{}, false
	}
	value, ok := parseQueryValue(rawValue)
	if !ok {
		return compiledQueryClause{}, false
	}
	clause.value = value
	return clause, true
}

func compiledConditionsMatch(conditions []compiledCondition, node *dom.Node, queries *QueryEnvironment) bool {
	for i := range conditions {
		if !conditions[i].matches(node, queries) {
			return false
		}
	}
	return true
}

func (c *compiledCondition) matches(node *dom.Node, queries *QueryEnvironment) bool {
	if c.never {
		return false
	}
	switch c.kind {
	case css.RuleConditionMedia:
		if queries == nil {
			return false
		}
		return c.matchesMedia(queries.Viewport)
	case css.RuleConditionContainer:
		if queries == nil || node == nil {
			return false
		}
		return c.matchesContainer(node, queries)
	default:
		return false
	}
}

func (c *compiledCondition) matchesMedia(viewport Viewport) bool {
	for i := range c.media {
		if c.media[i].matches(viewport) {
			return true
		}
	}
	return false
}

func (a *compiledQueryAlternative) matches(viewport Viewport) bool {
	result := a.matchesAll
	if !result {
		all := true
		for i := range a.clauses {
			if !a.clauses[i].matches(viewport.Width, viewport.Height) {
				all = false
				break
			}
		}
		result = all
	}
	if a.negated {
		return !result
	}
	return result
}

func (c *compiledCondition) matchesContainer(node *dom.Node, queries *QueryEnvironment) bool {
	for ancestor := node.Parent; ancestor != nil; ancestor = ancestor.Parent {
		ancestorStyle, ok := queries.Styles.Get(ancestor)
		if !ok || !supportsContainerQueries(ancestorStyle) {
			continue
		}
		if c.containerName != "" && !containerNameMatches(ancestorStyle.ContainerName, c.containerName) {
			continue
		}
		metrics, ok := queries.ContainerMetrics[ancestor]
		if !ok {
			continue
		}
		for i := range c.containerClauses {
			clause := &c.containerClauses[i]
			if !containerTypeSupportsQuery(ancestorStyle.ContainerType.Keyword, clause.requirement) {
				return false
			}
			if !clause.matches(metrics.Width, metrics.Height) {
				return false
			}
		}
		return true
	}
	return false
}
