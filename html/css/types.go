package css

import (
	"fmt"
	"strings"

	"github.com/diiyw/onix/html/dom"
)

type Stylesheet struct {
	Rules     []Rule
	Keyframes []Keyframes
}

type Keyframes struct {
	Name       string
	Frames     []Keyframe
	Conditions []RuleCondition
}

type Keyframe struct {
	Offsets      []float64
	Declarations []Declaration
}

type RuleConditionKind int

const (
	RuleConditionMedia RuleConditionKind = iota
	RuleConditionContainer
)

type Rule struct {
	Selectors    []Selector
	Declarations []Declaration
	Conditions   []RuleCondition
}

type Declaration struct {
	Property  string
	Value     string
	Important bool
}

type RuleCondition struct {
	Kind    RuleConditionKind
	Prelude string
}

type Selector struct {
	Parts []CompoundSelector
}

type combinator uint8

const (
	combinatorDescendant combinator = iota
	combinatorChild
	combinatorAdjacent
	combinatorSibling
)

type PseudoClass struct {
	Name      string
	Value     string
	Selectors []Selector
	nthA      int
	nthB      int
	nthParsed bool
}

// AttributeSelector matches an element that has the named content attribute.
type AttributeSelector struct {
	Name            string
	Operator        string
	Value           string
	CaseInsensitive bool
}

type CompoundSelector struct {
	Tag           string
	ID            string
	Classes       []string
	Attributes    []AttributeSelector
	PseudoClasses []PseudoClass
	combinator    combinator
}

type Specificity struct {
	ID    int
	Class int
	Tag   int
}

type State struct {
	Focused      *dom.Node
	Hovered      *dom.Node
	Active       *dom.Node
	FocusVisible bool
}

func (s Stylesheet) String() string {
	var builder strings.Builder
	builder.WriteString(serializeStylesheet(s.Rules))
	for _, keyframes := range s.Keyframes {
		builder.WriteByte('\n')
		body := keyframes.String()
		for i := len(keyframes.Conditions) - 1; i >= 0; i-- {
			condition := keyframes.Conditions[i]
			name := "media"
			if condition.Kind == RuleConditionContainer {
				name = "container"
			}
			body = fmt.Sprintf("@%s %s { %s }", name, condition.Prelude, body)
		}
		builder.WriteString(body)
	}
	return builder.String()
}

func (k Keyframes) String() string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "@keyframes %q {", k.Name)
	for _, frame := range k.Frames {
		for i, offset := range frame.Offsets {
			if i > 0 {
				builder.WriteString(", ")
			}
			fmt.Fprintf(&builder, "%g%%", offset*100)
		}
		builder.WriteString(" {")
		for _, declaration := range frame.Declarations {
			fmt.Fprintf(&builder, "%s: %s", declaration.Property, declaration.Value)
			if declaration.Important {
				builder.WriteString(" !important")
			}
			builder.WriteByte(';')
		}
		builder.WriteByte('}')
	}
	builder.WriteByte('}')
	return builder.String()
}

func serializeStylesheet(rules []Rule) string {
	var builder strings.Builder
	for i, rule := range rules {
		if i > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(serializeRule(rule))
	}
	return builder.String()
}

func serializeRule(rule Rule) string {
	body := rule.String()
	for index := len(rule.Conditions) - 1; index >= 0; index-- {
		condition := rule.Conditions[index]
		var name string
		switch condition.Kind {
		case RuleConditionMedia:
			name = "media"
		case RuleConditionContainer:
			name = "container"
		default:
			continue
		}
		body = fmt.Sprintf("@%s %s { %s }", name, condition.Prelude, body)
	}
	return body
}

// ScopeStylesheet parses CSS and adds className to each selector subject.
// The resulting stylesheet keeps media and container conditions intact.
func ScopeStylesheet(input, className string) (string, error) {
	stylesheet, err := Parse(input)
	if err != nil {
		return "", err
	}
	for ruleIndex := range stylesheet.Rules {
		for selectorIndex := range stylesheet.Rules[ruleIndex].Selectors {
			scopeSelector(&stylesheet.Rules[ruleIndex].Selectors[selectorIndex], className)
		}
	}
	return stylesheet.String(), nil
}

func scopeSelector(selector *Selector, className string) {
	if selector == nil || className == "" {
		return
	}
	// Every element in a component carries the scope class. Constrain the
	// selector's subject only, so document ancestors such as body/html remain
	// valid and functional pseudo-classes keep their authored meaning.
	part := &selector.Parts[len(selector.Parts)-1]
	for _, existing := range part.Classes {
		if existing == className {
			return
		}
	}
	part.Classes = append(part.Classes, className)
}

func (r Rule) String() string {
	selectorParts := make([]string, 0, len(r.Selectors))
	for _, selector := range r.Selectors {
		selectorParts = append(selectorParts, selector.String())
	}

	declarationParts := make([]string, 0, len(r.Declarations))
	for _, declaration := range r.Declarations {
		value := declaration.Value
		if declaration.Important {
			value += " !important"
		}
		declarationParts = append(declarationParts, fmt.Sprintf("%s: %s", declaration.Property, value))
	}

	return fmt.Sprintf("%s { %s; }", strings.Join(selectorParts, ", "), strings.Join(declarationParts, "; "))
}

func (s Selector) String() string {
	var builder strings.Builder
	for index, part := range s.Parts {
		if index > 0 {
			if part.combinator == combinatorChild {
				builder.WriteString(" > ")
			} else if part.combinator == combinatorAdjacent {
				builder.WriteString(" + ")
			} else if part.combinator == combinatorSibling {
				builder.WriteString(" ~ ")
			} else {
				builder.WriteByte(' ')
			}
		}
		builder.WriteString(part.String())
	}
	return builder.String()
}

func (c CompoundSelector) String() string {
	var builder strings.Builder
	if c.Tag != "" {
		builder.WriteString(c.Tag)
	}
	if c.ID != "" {
		builder.WriteByte('#')
		builder.WriteString(c.ID)
	}
	for _, className := range c.Classes {
		builder.WriteByte('.')
		builder.WriteString(className)
	}
	for _, attribute := range c.Attributes {
		builder.WriteByte('[')
		builder.WriteString(attribute.Name)
		if attribute.Operator != "" {
			builder.WriteString(attribute.Operator)
			builder.WriteByte('"')
			builder.WriteString(attribute.Value)
			builder.WriteByte('"')
			if attribute.CaseInsensitive {
				builder.WriteString(" i")
			}
		}
		builder.WriteByte(']')
	}
	for _, pseudoClass := range c.PseudoClasses {
		builder.WriteByte(':')
		builder.WriteString(pseudoClass.Name)
		if pseudoClass.Value != "" || len(pseudoClass.Selectors) > 0 {
			builder.WriteByte('(')
			if len(pseudoClass.Selectors) > 0 {
				parts := make([]string, 0, len(pseudoClass.Selectors))
				for _, selector := range pseudoClass.Selectors {
					parts = append(parts, selector.String())
				}
				builder.WriteString(strings.Join(parts, ", "))
			} else {
				builder.WriteString(pseudoClass.Value)
			}
			builder.WriteByte(')')
		}
	}
	return builder.String()
}

// HasSiblingCombinator reports whether changing one branch can affect another.
func (s Selector) HasSiblingCombinator() bool {
	for _, part := range s.Parts {
		if part.combinator == combinatorAdjacent || part.combinator == combinatorSibling {
			return true
		}
	}
	return false
}
