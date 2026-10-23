package css

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/diiyw/onix/html/dom"
)

type MatchContext struct {
	nodes      map[*dom.Node]nodeMatchData
	typeCounts map[string]int32
}

// Keep pointer-bearing fields together and booleans at the end. This cache
// stores one value per visited node, so padding multiplies across the document.
type nodeMatchData struct {
	previousElement   *dom.Node
	focusWithinState  *State
	id                string
	classValue        string
	childIndex        int32
	childReverse      int32
	typeIndex         int32
	typeReverse       int32
	indexFlags        uint8
	previousLoaded    bool
	idLoaded          bool
	classLoaded       bool
	hasClassAttr      bool
	focusWithinLoaded bool
	focusWithin       bool
}

// Reuse ordinary documents without pooling the largest document ever visited.
const maxCachedMatchNodes = 8192

const (
	childIndexLoaded uint8 = 1 << iota
	childReverseLoaded
	typeIndexLoaded
	typeReverseLoaded
)

func NewMatchContext() *MatchContext {
	return &MatchContext{}
}

func (c *MatchContext) Reset() {
	if c == nil {
		return
	}
	if len(c.nodes) > maxCachedMatchNodes {
		c.nodes = nil
		c.typeCounts = nil
		return
	}
	clear(c.nodes)
	clear(c.typeCounts)
}

func Match(selector Selector, node *dom.Node) bool {
	return MatchWithState(selector, node, nil)
}

func MatchWithState(selector Selector, node *dom.Node, state *State) bool {
	return MatchWithStateAndContext(selector, node, state, nil)
}

func MatchWithStateAndContext(selector Selector, node *dom.Node, state *State, ctx *MatchContext) bool {
	if node == nil || node.Type != dom.ElementNode || len(selector.Parts) == 0 {
		return false
	}

	return matchSelectorPart(selector.Parts, len(selector.Parts)-1, node, state, ctx)
}

// Backtrack descendant candidates: a nearer match may fail its left-hand chain.
func matchSelectorPart(parts []CompoundSelector, index int, node *dom.Node, state *State, ctx *MatchContext) bool {
	if !matchesCompound(parts[index], node, state, ctx) {
		return false
	}
	if index == 0 {
		return true
	}
	switch parts[index].combinator {
	case combinatorChild:
		return matchSelectorPart(parts, index-1, node.Parent, state, ctx)
	case combinatorAdjacent:
		return matchSelectorPart(parts, index-1, previousElement(node, ctx), state, ctx)
	case combinatorSibling:
		for sibling := previousElement(node, ctx); sibling != nil; sibling = previousElement(sibling, ctx) {
			if matchSelectorPart(parts, index-1, sibling, state, ctx) {
				return true
			}
		}
		return false
	default:
		for parent := node.Parent; parent != nil; parent = parent.Parent {
			if matchSelectorPart(parts, index-1, parent, state, ctx) {
				return true
			}
		}
		return false
	}
}

func matchesCompound(part CompoundSelector, node *dom.Node, state *State, ctx *MatchContext) bool {
	if node == nil || node.Type != dom.ElementNode {
		return false
	}

	if part.Tag != "" && part.Tag != "*" && node.Data != part.Tag {
		return false
	}

	if part.ID != "" {
		if ctx == nil {
			id, ok := node.Attr("id")
			if !ok || id != part.ID {
				return false
			}
		} else {
			data := ctx.nodes[node]
			if !data.idLoaded {
				id, ok := node.Attr("id")
				data.idLoaded = true
				if ok {
					data.id = id
				}
				ctx.store(node, data)
			}
			if data.id != part.ID {
				return false
			}
		}
	}

	if len(part.Classes) > 0 {
		if ctx == nil {
			classValue, ok := node.Attr("class")
			if !ok {
				return false
			}
			for _, name := range part.Classes {
				if !hasClassToken(classValue, name) {
					return false
				}
			}
		} else {
			data := ctx.nodes[node]
			if !data.classLoaded {
				classValue, ok := node.Attr("class")
				data.classLoaded = true
				data.hasClassAttr = ok
				if ok {
					data.classValue = classValue
				}
				ctx.store(node, data)
			}
			if !data.hasClassAttr {
				return false
			}
			for _, name := range part.Classes {
				if !hasClassToken(data.classValue, name) {
					return false
				}
			}
		}
	}

	for _, attribute := range part.Attributes {
		value, ok := node.Attr(attribute.Name)
		if !ok || !matchesAttribute(attribute, value) {
			return false
		}
	}

	for _, pseudoClass := range part.PseudoClasses {
		if !matchesPseudoClass(pseudoClass, node, state, ctx) {
			return false
		}
	}

	return true
}

func matchesAttribute(attribute AttributeSelector, actual string) bool {
	if attribute.Operator == "" {
		return true
	}
	value := attribute.Value
	if attribute.CaseInsensitive {
		return matchAttributeFold(attribute.Operator, actual, value)
	}
	switch attribute.Operator {
	case "=":
		return actual == value
	case "~=":
		return hasClassToken(actual, value)
	case "|=":
		return actual == value || (len(actual) > len(value) && actual[:len(value)] == value && actual[len(value)] == '-')
	case "^=":
		return strings.HasPrefix(actual, value)
	case "$=":
		return strings.HasSuffix(actual, value)
	case "*=":
		return strings.Contains(actual, value)
	default:
		return false
	}
}

func matchAttributeFold(operator, actual, value string) bool {
	switch operator {
	case "=":
		return strings.EqualFold(actual, value)
	case "~=":
		for start := 0; start < len(actual); {
			for start < len(actual) && isClassWhitespace(actual[start]) {
				start++
			}
			end := start
			for end < len(actual) && !isClassWhitespace(actual[end]) {
				end++
			}
			if start < end && strings.EqualFold(actual[start:end], value) {
				return true
			}
			start = end
		}
		return false
	case "|=":
		return strings.EqualFold(actual, value) || (len(actual) > len(value) && strings.EqualFold(actual[:len(value)], value) && actual[len(value)] == '-')
	case "^=":
		return len(actual) >= len(value) && strings.EqualFold(actual[:len(value)], value)
	case "$=":
		return len(actual) >= len(value) && strings.EqualFold(actual[len(actual)-len(value):], value)
	case "*=":
		if value == "" {
			return true
		}
		for index := 0; index+len(value) <= len(actual); index++ {
			if strings.EqualFold(actual[index:index+len(value)], value) {
				return true
			}
		}
	}
	return false
}

func matchesPseudoClass(pseudoClass PseudoClass, node *dom.Node, state *State, ctx *MatchContext) bool {
	switch pseudoClass.Name {
	case "root":
		return isRootElement(node)
	case "empty":
		return isEmptyElement(node)
	case "disabled":
		return dom.IsDisabled(node)
	case "enabled":
		return dom.SupportsDisabled(node) && !dom.IsDisabled(node)
	case "checked":
		return dom.IsChecked(node)
	case "indeterminate":
		indeterminate, resolved := node.States.Get(dom.StateIndeterminate)
		return resolved && indeterminate
	case "first-child":
		return childIndex(node, false, false, ctx) == 1
	case "last-child":
		return childIndex(node, false, true, ctx) == 1
	case "only-child":
		return childIndex(node, false, false, ctx) == 1 && childIndex(node, false, true, ctx) == 1
	case "first-of-type":
		return childIndex(node, true, false, ctx) == 1
	case "last-of-type":
		return childIndex(node, true, true, ctx) == 1
	case "only-of-type":
		return childIndex(node, true, false, ctx) == 1 && childIndex(node, true, true, ctx) == 1
	case "not":
		return !matchesAnySelector(pseudoClass.Selectors, node, state, ctx)
	case "is", "where":
		return matchesAnySelector(pseudoClass.Selectors, node, state, ctx)
	case "nth-child":
		return matchesNthPseudoClass(node, pseudoClass, false, false, ctx)
	case "nth-of-type":
		return matchesNthPseudoClass(node, pseudoClass, true, false, ctx)
	case "nth-last-child":
		return matchesNthPseudoClass(node, pseudoClass, false, true, ctx)
	case "nth-last-of-type":
		return matchesNthPseudoClass(node, pseudoClass, true, true, ctx)
	case "placeholder-shown":
		return dom.IsPlaceholderShown(node)
	case "focus":
		return isFocused(node, state)
	case "focus-visible":
		return isFocused(node, state) && state != nil && state.FocusVisible
	case "focus-within":
		return hasFocusWithin(node, state, ctx)
	case "hover":
		return isHovered(node, state)
	case "active":
		return isActive(node, state)
	default:
		return false
	}
}

func matchesAnySelector(selectors []Selector, node *dom.Node, state *State, ctx *MatchContext) bool {
	for _, selector := range selectors {
		if MatchWithStateAndContext(selector, node, state, ctx) {
			return true
		}
	}
	return false
}

func hasClassToken(classValue, wanted string) bool {
	for start := 0; start < len(classValue); {
		for start < len(classValue) && isClassWhitespace(classValue[start]) {
			start++
		}
		if start == len(classValue) {
			return false
		}
		end := start
		for end < len(classValue) && !isClassWhitespace(classValue[end]) {
			end++
		}
		if classValue[start:end] == wanted {
			return true
		}
		start = end
	}
	return false
}

func isClassWhitespace(value byte) bool {
	switch value {
	case ' ', '\t', '\n', '\r', '\f':
		return true
	default:
		return false
	}
}

func (c *MatchContext) store(node *dom.Node, data nodeMatchData) {
	if c.nodes == nil {
		c.nodes = make(map[*dom.Node]nodeMatchData)
	}
	c.nodes[node] = data
}

func isRootElement(node *dom.Node) bool {
	return node != nil && node.Parent != nil && node.Parent.Type == dom.DocumentNode
}

func isEmptyElement(node *dom.Node) bool {
	if node == nil {
		return false
	}
	for _, child := range node.Children {
		if child == nil {
			continue
		}
		if child.Type == dom.ElementNode {
			return false
		}
		if child.Type == dom.TextNode {
			return false
		}
	}
	return true
}

func elementParent(node *dom.Node) *dom.Node {
	if node == nil || node.Parent == nil || node.Parent.Type != dom.ElementNode {
		return nil
	}
	return node.Parent
}

func isNthPseudoClass(name string) bool {
	switch name {
	case "nth-child", "nth-of-type", "nth-last-child", "nth-last-of-type":
		return true
	default:
		return false
	}
}

func matchesNthPseudoClass(node *dom.Node, pseudoClass PseudoClass, ofType, fromEnd bool, ctx *MatchContext) bool {
	index := childIndex(node, ofType, fromEnd, ctx)
	if index <= 0 {
		return false
	}
	a, b := pseudoClass.nthA, pseudoClass.nthB
	if !pseudoClass.nthParsed {
		var err error
		a, b, err = parseNthExpression(pseudoClass.Value)
		if err != nil {
			return false
		}
	}
	if a == 0 {
		return index == b
	}
	delta := index - b
	if (delta > 0 && a < 0) || (delta < 0 && a > 0) {
		return false
	}
	return delta%a == 0
}

func childIndex(node *dom.Node, ofType, fromEnd bool, ctx *MatchContext) int {
	parent := elementParent(node)
	if parent == nil {
		return 0
	}
	if ctx != nil {
		data := ctx.nodes[node]
		loaded := data.indexFlags&childIndexLoaded != 0
		index := data.childIndex
		if ofType {
			loaded, index = data.indexFlags&typeIndexLoaded != 0, data.typeIndex
		}
		if fromEnd {
			loaded, index = data.indexFlags&childReverseLoaded != 0, data.childReverse
			if ofType {
				loaded, index = data.indexFlags&typeReverseLoaded != 0, data.typeReverse
			}
		}
		if loaded {
			return int(index)
		}
		cacheChildIndices(parent, ofType, fromEnd, ctx)
		data = ctx.nodes[node]
		if ofType {
			if fromEnd {
				return int(data.typeReverse)
			}
			return int(data.typeIndex)
		}
		if fromEnd {
			return int(data.childReverse)
		}
		return int(data.childIndex)
	}
	return scanChildIndex(parent, node, ofType, fromEnd)
}

func cacheChildIndices(parent *dom.Node, ofType, fromEnd bool, ctx *MatchContext) {
	if ofType {
		cacheTypeIndices(parent, fromEnd, ctx)
		return
	}
	index := 0
	if fromEnd {
		for i := len(parent.Children) - 1; i >= 0; i-- {
			child := parent.Children[i]
			if child == nil || child.Type != dom.ElementNode {
				continue
			}
			index++
			data := ctx.nodes[child]
			data.indexFlags |= childReverseLoaded
			data.childReverse = int32(index)
			ctx.store(child, data)
		}
		return
	}
	for _, child := range parent.Children {
		if child == nil || child.Type != dom.ElementNode {
			continue
		}
		index++
		data := ctx.nodes[child]
		data.indexFlags |= childIndexLoaded
		data.childIndex = int32(index)
		ctx.store(child, data)
	}
}

func cacheTypeIndices(parent *dom.Node, fromEnd bool, ctx *MatchContext) {
	index, step := 0, 1
	if fromEnd {
		index, step = len(parent.Children)-1, -1
	}
	var firstType string
	var firstCount int32
	for ; index >= 0 && index < len(parent.Children); index += step {
		child := parent.Children[index]
		if child == nil || child.Type != dom.ElementNode {
			continue
		}
		if firstCount == 0 {
			firstType = child.Data
		}
		var count int32
		if child.Data == firstType {
			firstCount++
			count = firstCount
		} else {
			if ctx.typeCounts == nil {
				ctx.typeCounts = make(map[string]int32)
			}
			ctx.typeCounts[child.Data]++
			count = ctx.typeCounts[child.Data]
		}
		data := ctx.nodes[child]
		if fromEnd {
			data.typeReverse = count
			data.indexFlags |= typeReverseLoaded
		} else {
			data.typeIndex = count
			data.indexFlags |= typeIndexLoaded
		}
		ctx.store(child, data)
	}
	clear(ctx.typeCounts)
}

func scanChildIndex(parent, node *dom.Node, ofType, fromEnd bool) int {
	index := 0
	visit := func(child *dom.Node) bool {
		if child == nil || child.Type != dom.ElementNode || ofType && child.Data != node.Data {
			return false
		}
		index++
		return child == node
	}
	if fromEnd {
		for i := len(parent.Children) - 1; i >= 0; i-- {
			if visit(parent.Children[i]) {
				return index
			}
		}
		return 0
	}
	for _, child := range parent.Children {
		if visit(child) {
			return index
		}
	}
	return 0
}

func hasFocusWithin(node *dom.Node, state *State, ctx *MatchContext) bool {
	if node == nil {
		return false
	}
	if ctx != nil {
		data := ctx.nodes[node]
		if data.focusWithinLoaded && data.focusWithinState == state {
			return data.focusWithin
		}
		data.focusWithinState = state
		data.focusWithinLoaded = true
		ctx.store(node, data)
	}
	if isFocused(node, state) {
		if ctx != nil {
			data := ctx.nodes[node]
			data.focusWithin = true
			ctx.store(node, data)
		}
		return true
	}
	for _, child := range node.Children {
		if hasFocusWithin(child, state, ctx) {
			if ctx != nil {
				data := ctx.nodes[node]
				data.focusWithin = true
				ctx.store(node, data)
			}
			return true
		}
	}
	if ctx != nil {
		data := ctx.nodes[node]
		data.focusWithin = false
		ctx.store(node, data)
	}
	return false
}

func isFocused(node *dom.Node, state *State) bool {
	return state != nil && state.Focused == node
}

func isHovered(node *dom.Node, state *State) bool {
	return state != nil && isInclusiveAncestor(node, state.Hovered)
}

func isActive(node *dom.Node, state *State) bool {
	return state != nil && isInclusiveAncestor(node, state.Active)
}

func isInclusiveAncestor(ancestor, node *dom.Node) bool {
	if ancestor == nil || ancestor.Type != dom.ElementNode {
		return false
	}
	for current := node; current != nil; current = current.Parent {
		if current == ancestor {
			return true
		}
	}
	return false
}

func parseNthExpression(expr string) (int, int, error) {
	trimmed := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(expr), " ", ""))
	switch trimmed {
	case "odd":
		return 2, 1, nil
	case "even":
		return 2, 0, nil
	}
	if !strings.Contains(trimmed, "n") {
		value, err := strconv.Atoi(trimmed)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid nth expression %q", expr)
		}
		return 0, value, nil
	}
	nIndex := strings.IndexByte(trimmed, 'n')
	aPart := trimmed[:nIndex]
	bPart := trimmed[nIndex+1:]
	switch aPart {
	case "", "+":
		aPart = "1"
	case "-":
		aPart = "-1"
	}
	a, err := strconv.Atoi(aPart)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid nth expression %q", expr)
	}
	b := 0
	if bPart != "" {
		b, err = strconv.Atoi(bPart)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid nth expression %q", expr)
		}
	}
	return a, b, nil
}

func previousElement(node *dom.Node, ctx *MatchContext) *dom.Node {
	if node.Parent == nil {
		return nil
	}
	if ctx != nil {
		if data := ctx.nodes[node]; data.previousLoaded {
			return data.previousElement
		}
	}
	var previous, result *dom.Node
	for _, sibling := range node.Parent.Children {
		if sibling.Type != dom.ElementNode {
			continue
		}
		if sibling == node {
			result = previous
			if ctx == nil {
				return result
			}
		}
		if ctx != nil {
			data := ctx.nodes[sibling]
			data.previousLoaded = true
			data.previousElement = previous
			ctx.store(sibling, data)
		}
		previous = sibling
	}
	return result
}
