package css

import (
	"fmt"
	"strings"
)

func Parse(input string) (*Stylesheet, error) {
	tokenizer := NewTokenizer(input)
	stylesheet := &Stylesheet{}
	rules, err := parseRuleList(tokenizer, stylesheet, nil, false)
	if err != nil {
		return nil, err
	}
	stylesheet.Rules = rules
	return stylesheet, nil
}

func parseRuleList(tokenizer *Tokenizer, stylesheet *Stylesheet, inherited []RuleCondition, stopOnRBrace bool) ([]Rule, error) {
	rules := []Rule{}
	for {
		token, err := tokenizer.Peek()
		if err != nil {
			return nil, err
		}

		switch token.Type {
		case TokenEOF:
			if stopOnRBrace {
				return nil, fmt.Errorf("unexpected end of input inside at-rule block")
			}
			return rules, nil
		case TokenWhitespace:
			if _, err := tokenizer.Next(); err != nil {
				return nil, err
			}
		case TokenRBrace:
			if !stopOnRBrace {
				return nil, fmt.Errorf("unexpected '}'")
			}
			if _, err := tokenizer.Next(); err != nil {
				return nil, err
			}
			return rules, nil
		case TokenAtKeyword:
			parsed, err := parseAtRule(tokenizer, stylesheet, inherited)
			if err != nil {
				if closed, recoverErr := recoverRule(tokenizer); recoverErr != nil {
					return nil, recoverErr
				} else if closed {
					return rules, nil
				}
				continue
			}
			rules = append(rules, parsed...)
		default:
			rule, err := parseRule(tokenizer, inherited)
			if err != nil {
				if closed, recoverErr := recoverRule(tokenizer); recoverErr != nil {
					return nil, recoverErr
				} else if closed {
					return rules, nil
				}
				continue
			}
			rules = append(rules, rule)
		}
	}
}

// recoverRule discards a malformed qualified rule and resumes after its
// balanced block. CSS syntax errors do not invalidate unrelated later rules.
func recoverRule(tokenizer *Tokenizer) (bool, error) {
	depth, foundBlock := 0, false
	for {
		token, err := tokenizer.Next()
		if err != nil {
			return false, err
		}
		switch token.Type {
		case TokenEOF:
			return true, nil
		case TokenLBrace:
			foundBlock = true
			depth++
		case TokenRBrace:
			if !foundBlock {
				return true, nil
			}
			depth--
			if depth == 0 {
				return false, nil
			}
		case TokenSemicolon:
			if !foundBlock {
				return false, nil
			}
		}
	}
}

func parseRule(tokenizer *Tokenizer, inherited []RuleCondition) (Rule, error) {
	selectors, err := parseSelectorList(tokenizer)
	if err != nil {
		return Rule{}, err
	}

	brace, err := tokenizer.Next()
	if err != nil {
		return Rule{}, err
	}
	if brace.Type != TokenLBrace {
		return Rule{}, fmt.Errorf("expected '{' after selectors")
	}

	declarations, err := parseDeclarations(tokenizer)
	if err != nil {
		return Rule{}, err
	}

	return Rule{Selectors: selectors, Declarations: declarations, Conditions: cloneConditions(inherited)}, nil
}

func parseAtRule(tokenizer *Tokenizer, stylesheet *Stylesheet, inherited []RuleCondition) ([]Rule, error) {
	token, err := tokenizer.Next()
	if err != nil {
		return nil, err
	}
	if token.Type != TokenAtKeyword {
		return nil, fmt.Errorf("expected at-rule, got %q", token.Value)
	}
	prelude, terminator, err := parseAtRulePrelude(tokenizer)
	if err != nil {
		return nil, err
	}

	switch strings.ToLower(token.Value) {
	case "keyframes":
		if terminator != TokenLBrace {
			return nil, nil
		}
		keyframes, err := parseKeyframes(tokenizer, prelude, inherited)
		if err == nil && keyframes.Name != "" {
			stylesheet.Keyframes = append(stylesheet.Keyframes, keyframes)
		}
		return nil, err
	case "media", "container":
		if terminator != TokenLBrace {
			return nil, fmt.Errorf("@%s must use a block", token.Value)
		}
		kind := RuleConditionMedia
		if token.Value == "container" {
			kind = RuleConditionContainer
		}
		return parseRuleList(tokenizer, stylesheet, appendCondition(inherited, RuleCondition{Kind: kind, Prelude: prelude}), true)
	default:
		if terminator == TokenLBrace {
			return skipAtRuleBlock(tokenizer)
		}
		return nil, nil
	}
}

func parseAtRulePrelude(tokenizer *Tokenizer) (string, TokenType, error) {
	var builder strings.Builder
	depthParen := 0
	depthBracket := 0
	for {
		token, err := tokenizer.Next()
		if err != nil {
			return "", TokenEOF, err
		}
		switch token.Type {
		case TokenEOF:
			return "", TokenEOF, fmt.Errorf("unexpected end of input inside at-rule prelude")
		case TokenLBrace:
			if depthParen == 0 && depthBracket == 0 {
				return strings.TrimSpace(builder.String()), TokenLBrace, nil
			}
		case TokenSemicolon:
			if depthParen == 0 && depthBracket == 0 {
				return strings.TrimSpace(builder.String()), TokenSemicolon, nil
			}
		case TokenDelim:
			switch token.Value {
			case "(":
				depthParen++
			case ")":
				if depthParen > 0 {
					depthParen--
				}
			case "[":
				depthBracket++
			case "]":
				if depthBracket > 0 {
					depthBracket--
				}
			}
		case TokenLBracket:
			depthBracket++
		case TokenRBracket:
			if depthBracket > 0 {
				depthBracket--
			}
		}
		if token.Type == TokenString {
			fmt.Fprintf(&builder, "%q", token.Value)
		} else {
			builder.WriteString(token.Value)
		}
	}
}

func skipAtRuleBlock(tokenizer *Tokenizer) ([]Rule, error) {
	depth := 1
	for depth > 0 {
		token, err := tokenizer.Next()
		if err != nil {
			return nil, err
		}
		switch token.Type {
		case TokenEOF:
			return nil, fmt.Errorf("unexpected end of input inside at-rule block")
		case TokenLBrace:
			depth++
		case TokenRBrace:
			depth--
		}
	}
	return nil, nil
}

func appendCondition(existing []RuleCondition, condition RuleCondition) []RuleCondition {
	combined := cloneConditions(existing)
	combined = append(combined, condition)
	return combined
}

func cloneConditions(input []RuleCondition) []RuleCondition {
	if len(input) == 0 {
		return nil
	}
	return append([]RuleCondition(nil), input...)
}

func parseSelectorList(tokenizer *Tokenizer) ([]Selector, error) {
	selectors := []Selector{}
	for {
		if err := skipWhitespace(tokenizer); err != nil {
			return nil, err
		}

		selector, err := parseSelector(tokenizer)
		if err != nil {
			return nil, err
		}
		selectors = append(selectors, selector)

		token, err := tokenizer.Peek()
		if err != nil {
			return nil, err
		}

		if token.Type == TokenWhitespace {
			if _, err := tokenizer.Next(); err != nil {
				return nil, err
			}
			token, err = tokenizer.Peek()
			if err != nil {
				return nil, err
			}
		}

		switch token.Type {
		case TokenComma:
			if _, err := tokenizer.Next(); err != nil {
				return nil, err
			}
		case TokenLBrace:
			return selectors, nil
		default:
			return nil, fmt.Errorf("expected ',' or '{' after selector, got %q", token.Value)
		}
	}
}

func skipWhitespace(tokenizer *Tokenizer) error {
	for {
		token, err := tokenizer.Peek()
		if err != nil {
			return err
		}
		if token.Type != TokenWhitespace {
			return nil
		}
		if _, err := tokenizer.Next(); err != nil {
			return err
		}
	}
}

func parseSelector(tokenizer *Tokenizer) (Selector, error) {
	parts := make([]CompoundSelector, 0, 2)
	part, err := parseCompoundSelector(tokenizer)
	if err != nil {
		return Selector{}, err
	}
	parts = append(parts, part)

	for {
		token, err := tokenizer.Peek()
		if err != nil {
			return Selector{}, err
		}

		relation := combinatorDescendant
		if token.Type == TokenWhitespace {
			if _, err := tokenizer.Next(); err != nil {
				return Selector{}, err
			}
			if err := skipWhitespace(tokenizer); err != nil {
				return Selector{}, err
			}
			token, err = tokenizer.Peek()
			if err != nil {
				return Selector{}, err
			}
			if selectorBoundary(token) {
				return Selector{Parts: parts}, nil
			}
			if token.Type == TokenDelim && (token.Value == ">" || token.Value == "+" || token.Value == "~") {
				if _, err := tokenizer.Next(); err != nil {
					return Selector{}, err
				}
				relation = combinatorChild
				if token.Value == "+" {
					relation = combinatorAdjacent
				} else if token.Value == "~" {
					relation = combinatorSibling
				}
			} else {
				part, err = parseCompoundSelector(tokenizer)
				if err != nil {
					return Selector{}, err
				}
				parts = append(parts, part)
				continue
			}
		} else if token.Type == TokenDelim && (token.Value == ">" || token.Value == "+" || token.Value == "~") {
			if _, err := tokenizer.Next(); err != nil {
				return Selector{}, err
			}
			relation = combinatorChild
			if token.Value == "+" {
				relation = combinatorAdjacent
			} else if token.Value == "~" {
				relation = combinatorSibling
			}
		} else {
			return Selector{Parts: parts}, nil
		}

		if relation != combinatorDescendant {
			if err := skipWhitespace(tokenizer); err != nil {
				return Selector{}, err
			}
		}
		part, err = parseCompoundSelector(tokenizer)
		if err != nil {
			return Selector{}, err
		}
		part.combinator = relation
		parts = append(parts, part)
	}
}

func selectorBoundary(token Token) bool {
	return token.Type == TokenLBrace || token.Type == TokenComma || (token.Type == TokenDelim && token.Value == ")")
}

func parseCompoundSelector(tokenizer *Tokenizer) (CompoundSelector, error) {
	part := CompoundSelector{}
	consumed := false

	for {
		token, err := tokenizer.Peek()
		if err != nil {
			return CompoundSelector{}, err
		}

		switch token.Type {
		case TokenIdent:
			if _, err := tokenizer.Next(); err != nil {
				return CompoundSelector{}, err
			}
			if part.Tag != "" {
				return CompoundSelector{}, fmt.Errorf("duplicate type selector %q", token.Value)
			}
			part.Tag = strings.ToLower(token.Value)
			consumed = true
		case TokenDelim:
			if token.Value != "*" {
				if !consumed {
					return CompoundSelector{}, fmt.Errorf("expected selector, got %q", token.Value)
				}
				return part, nil
			}
			if _, err := tokenizer.Next(); err != nil {
				return CompoundSelector{}, err
			}
			if part.Tag != "" {
				return CompoundSelector{}, fmt.Errorf("duplicate type selector %q", token.Value)
			}
			part.Tag = "*"
			consumed = true
		case TokenHash:
			if _, err := tokenizer.Next(); err != nil {
				return CompoundSelector{}, err
			}
			if part.ID != "" {
				return CompoundSelector{}, fmt.Errorf("duplicate ID selector %q", token.Value)
			}
			part.ID = token.Value
			consumed = true
		case TokenDot:
			if _, err := tokenizer.Next(); err != nil {
				return CompoundSelector{}, err
			}
			classToken, err := tokenizer.Next()
			if err != nil {
				return CompoundSelector{}, err
			}
			if classToken.Type != TokenIdent {
				return CompoundSelector{}, fmt.Errorf("expected class name after '.'")
			}
			part.Classes = append(part.Classes, classToken.Value)
			consumed = true
		case TokenLBracket:
			attribute, err := parseAttributeSelector(tokenizer)
			if err != nil {
				return CompoundSelector{}, err
			}
			part.Attributes = append(part.Attributes, attribute)
			consumed = true
		case TokenColon:
			pseudoClass, err := parsePseudoClass(tokenizer)
			if err != nil {
				return CompoundSelector{}, err
			}
			part.PseudoClasses = append(part.PseudoClasses, pseudoClass)
			consumed = true
		default:
			if !consumed {
				return CompoundSelector{}, fmt.Errorf("expected selector, got %q", token.Value)
			}
			return part, nil
		}
	}
}

func parseAttributeSelector(tokenizer *Tokenizer) (AttributeSelector, error) {
	if _, err := tokenizer.Next(); err != nil {
		return AttributeSelector{}, err
	}
	if err := skipWhitespace(tokenizer); err != nil {
		return AttributeSelector{}, err
	}
	name, err := tokenizer.Next()
	if err != nil {
		return AttributeSelector{}, err
	}
	if name.Type != TokenIdent {
		return AttributeSelector{}, fmt.Errorf("expected attribute name after '['")
	}
	if err := skipWhitespace(tokenizer); err != nil {
		return AttributeSelector{}, err
	}
	selector := AttributeSelector{Name: strings.ToLower(name.Value)}
	operator, err := tokenizer.Peek()
	if err != nil {
		return AttributeSelector{}, err
	}
	if operator.Type == TokenRBracket {
		_, _ = tokenizer.Next()
		return selector, nil
	}
	if operator.Type != TokenDelim || !isAttributeOperatorStart(operator.Value) {
		return AttributeSelector{}, fmt.Errorf("expected attribute operator or ']' after %q", name.Value)
	}
	first, _ := tokenizer.Next()
	selector.Operator = first.Value
	if next, _ := tokenizer.Peek(); next.Type == TokenDelim && next.Value == "=" {
		_, _ = tokenizer.Next()
		selector.Operator += "="
	}
	if selector.Operator != "=" && selector.Operator != "~=" && selector.Operator != "|=" && selector.Operator != "^=" && selector.Operator != "$=" && selector.Operator != "*=" {
		return AttributeSelector{}, fmt.Errorf("unsupported attribute operator %q", selector.Operator)
	}
	if err := skipWhitespace(tokenizer); err != nil {
		return AttributeSelector{}, err
	}
	value, err := tokenizer.Next()
	if err != nil {
		return AttributeSelector{}, err
	}
	if value.Type != TokenIdent && value.Type != TokenString && value.Type != TokenHash {
		return AttributeSelector{}, fmt.Errorf("expected attribute value after %q", selector.Operator)
	}
	selector.Value = value.Value
	if err := skipWhitespace(tokenizer); err != nil {
		return AttributeSelector{}, err
	}
	modifier, err := tokenizer.Peek()
	if err != nil {
		return AttributeSelector{}, err
	}
	if modifier.Type == TokenIdent {
		modifier, _ = tokenizer.Next()
		switch strings.ToLower(modifier.Value) {
		case "i":
			selector.CaseInsensitive = true
		case "s":
		default:
			return AttributeSelector{}, fmt.Errorf("unsupported attribute modifier %q", modifier.Value)
		}
		if err := skipWhitespace(tokenizer); err != nil {
			return AttributeSelector{}, err
		}
	}
	closing, err := tokenizer.Next()
	if err != nil {
		return AttributeSelector{}, err
	}
	if closing.Type != TokenRBracket {
		return AttributeSelector{}, fmt.Errorf("expected ']' after attribute value %q", selector.Value)
	}
	return selector, nil
}

func isAttributeOperatorStart(value string) bool {
	switch value {
	case "=", "~", "|", "^", "$", "*":
		return true
	default:
		return false
	}
}

func parsePseudoClass(tokenizer *Tokenizer) (PseudoClass, error) {
	if _, err := tokenizer.Next(); err != nil {
		return PseudoClass{}, err
	}
	pseudoToken, err := tokenizer.Next()
	if err != nil {
		return PseudoClass{}, err
	}
	if pseudoToken.Type != TokenIdent {
		return PseudoClass{}, fmt.Errorf("expected pseudo-class name after ':'")
	}
	pseudoClass := PseudoClass{Name: strings.ToLower(pseudoToken.Value)}
	next, err := tokenizer.Peek()
	if err != nil {
		return PseudoClass{}, err
	}
	if next.Type != TokenDelim || next.Value != "(" {
		return pseudoClass, nil
	}
	if _, err := tokenizer.Next(); err != nil {
		return PseudoClass{}, err
	}
	if pseudoClass.Name == "not" || pseudoClass.Name == "is" || pseudoClass.Name == "where" {
		selectors, err := parsePseudoSelectorList(tokenizer)
		if err != nil {
			return PseudoClass{}, err
		}
		pseudoClass.Selectors = selectors
		return pseudoClass, nil
	}
	value, err := readPseudoFunctionValue(tokenizer)
	if err != nil {
		return PseudoClass{}, err
	}
	pseudoClass.Value = value
	if isNthPseudoClass(pseudoClass.Name) {
		if a, b, err := parseNthExpression(value); err == nil {
			pseudoClass.nthA = a
			pseudoClass.nthB = b
			pseudoClass.nthParsed = true
		}
	}
	return pseudoClass, nil
}

func parsePseudoSelectorList(tokenizer *Tokenizer) ([]Selector, error) {
	selectors := []Selector{}
	for {
		if err := skipWhitespace(tokenizer); err != nil {
			return nil, err
		}
		selector, err := parseSelector(tokenizer)
		if err != nil {
			return nil, err
		}
		selectors = append(selectors, selector)
		if err := skipWhitespace(tokenizer); err != nil {
			return nil, err
		}
		token, err := tokenizer.Peek()
		if err != nil {
			return nil, err
		}
		if token.Type == TokenComma {
			if _, err := tokenizer.Next(); err != nil {
				return nil, err
			}
			continue
		}
		if token.Type == TokenDelim && token.Value == ")" {
			if _, err := tokenizer.Next(); err != nil {
				return nil, err
			}
			return selectors, nil
		}
		return nil, fmt.Errorf("expected ',' or ')' in pseudo-class, got %q", token.Value)
	}
}

func readPseudoFunctionValue(tokenizer *Tokenizer) (string, error) {
	var builder strings.Builder
	depth := 1
	for {
		token, err := tokenizer.Next()
		if err != nil {
			return "", err
		}
		if token.Type == TokenEOF {
			return "", fmt.Errorf("unexpected end of input inside pseudo-class")
		}
		if token.Type == TokenDelim {
			switch token.Value {
			case "(":
				depth++
			case ")":
				depth--
				if depth == 0 {
					return strings.TrimSpace(builder.String()), nil
				}
			}
		}
		builder.WriteString(token.Value)
	}
}

func parseDeclarations(tokenizer *Tokenizer) ([]Declaration, error) {
	declarations := []Declaration{}

	for {
		token, err := tokenizer.Peek()
		if err != nil {
			return nil, err
		}

		switch token.Type {
		case TokenWhitespace, TokenSemicolon:
			if _, err := tokenizer.Next(); err != nil {
				return nil, err
			}
		case TokenRBrace:
			if _, err := tokenizer.Next(); err != nil {
				return nil, err
			}
			return declarations, nil
		case TokenEOF:
			return declarations, nil
		default:
			declaration, err := parseDeclaration(tokenizer)
			if err != nil {
				closed, recoverErr := recoverDeclaration(tokenizer)
				if recoverErr != nil {
					return nil, recoverErr
				}
				if closed {
					return declarations, nil
				}
				continue
			}
			declarations = append(declarations, declaration)
		}
	}
}

// recoverDeclaration consumes one malformed declaration. CSS error recovery
// resumes at the next semicolon or closes the current block at the next '}'.
func recoverDeclaration(tokenizer *Tokenizer) (bool, error) {
	parenDepth, bracketDepth := 0, 0
	var quote byte
	for {
		token, err := tokenizer.Next()
		if err != nil {
			return false, err
		}
		switch token.Type {
		case TokenEOF:
			return false, fmt.Errorf("unexpected end of input inside declaration block")
		case TokenLBracket:
			bracketDepth++
		case TokenRBracket:
			if bracketDepth > 0 {
				bracketDepth--
			}
		case TokenDelim:
			switch token.Value {
			case "(":
				parenDepth++
			case ")":
				if parenDepth > 0 {
					parenDepth--
				}
			case "'", "\"":
				if quote == 0 {
					quote = token.Value[0]
				} else if quote == token.Value[0] {
					quote = 0
				}
			}
		case TokenSemicolon:
			if parenDepth == 0 && bracketDepth == 0 && quote == 0 {
				return false, nil
			}
		case TokenRBrace:
			if parenDepth == 0 && bracketDepth == 0 && quote == 0 {
				return true, nil
			}
		}
	}
}

func parseDeclaration(tokenizer *Tokenizer) (Declaration, error) {
	name, err := tokenizer.Next()
	if err != nil {
		return Declaration{}, err
	}
	if name.Type != TokenIdent {
		return Declaration{}, fmt.Errorf("expected property name, got %q", name.Value)
	}

	token, err := tokenizer.Peek()
	if err != nil {
		return Declaration{}, err
	}
	if token.Type == TokenWhitespace {
		if _, err := tokenizer.Next(); err != nil {
			return Declaration{}, err
		}
		token, err = tokenizer.Peek()
		if err != nil {
			return Declaration{}, err
		}
	}
	if token.Type != TokenColon {
		return Declaration{}, fmt.Errorf("expected ':' after property %q", name.Value)
	}
	if _, err := tokenizer.Next(); err != nil {
		return Declaration{}, err
	}

	value := tokenizer.ReadDeclarationValue()
	if value == "" {
		return Declaration{}, fmt.Errorf("missing value for property %q", name.Value)
	}

	end, err := tokenizer.Peek()
	if err != nil {
		return Declaration{}, err
	}
	if end.Type == TokenSemicolon {
		if _, err := tokenizer.Next(); err != nil {
			return Declaration{}, err
		}
	}

	value, important := SplitImportant(value)
	if value == "" {
		return Declaration{}, fmt.Errorf("missing value for property %q", name.Value)
	}
	return Declaration{Property: strings.ToLower(name.Value), Value: value, Important: important}, nil
}

// SplitImportant removes the terminal CSS !important flag while preserving
// the authored value. It only recognizes a flag outside a quoted/function
// token because strings such as content:"!important" are ordinary values.
func SplitImportant(input string) (string, bool) {
	value, err := StripComments(input)
	if err != nil {
		return strings.TrimSpace(input), false
	}
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < len("!important") {
		return trimmed, false
	}
	word := len(trimmed) - len("important")
	if !strings.EqualFold(trimmed[word:], "important") {
		return trimmed, false
	}
	prefix := strings.TrimRight(trimmed[:word], " \t\r\n\f")
	if len(prefix) == 0 || prefix[len(prefix)-1] != '!' {
		return trimmed, false
	}
	start := len(prefix) - 1
	depthParen, depthBracket := 0, 0
	var quote byte
	escaped := false
	for index := 0; index < start; index++ {
		ch := trimmed[index]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
			} else if ch == quote {
				quote = 0
			}
			continue
		}
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		switch ch {
		case '\'', '"':
			quote = ch
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
		}
	}
	if quote != 0 || depthParen != 0 || depthBracket != 0 || escaped {
		return trimmed, false
	}
	value = strings.TrimSpace(trimmed[:start])
	if value == "" {
		return trimmed, false
	}
	return value, true
}
