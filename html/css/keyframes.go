package css

import (
	"fmt"
	"strconv"
	"strings"
)

func parseKeyframes(tokenizer *Tokenizer, prelude string, conditions []RuleCondition) (Keyframes, error) {
	result := Keyframes{Conditions: cloneConditions(conditions)}
	nameToken := NewTokenizer(prelude)
	name, err := nameToken.Next()
	if err != nil {
		return result, err
	}
	quoted := name.Type == TokenString
	if name.Type == TokenIdent || quoted {
		result.Name = name.Value
	}
	if !quoted {
		switch strings.ToLower(result.Name) {
		case "none", "initial", "inherit", "unset", "revert", "revert-layer":
			result.Name = ""
		}
	}
	if err := skipWhitespace(nameToken); err != nil {
		return result, err
	}
	if end, _ := nameToken.Next(); end.Type != TokenEOF {
		result.Name = ""
	}
	for {
		if err := skipWhitespace(tokenizer); err != nil {
			return result, err
		}
		token, err := tokenizer.Peek()
		if err != nil {
			return result, err
		}
		if token.Type == TokenRBrace {
			_, err = tokenizer.Next()
			return result, err
		}
		if token.Type == TokenEOF {
			return result, fmt.Errorf("unterminated @keyframes block")
		}
		selectors, end, err := parseAtRulePrelude(tokenizer)
		if err != nil {
			return result, err
		}
		if end != TokenLBrace {
			continue
		}
		declarations, err := parseDeclarations(tokenizer)
		if err != nil {
			return result, err
		}
		frame := Keyframe{Declarations: declarations}
		valid := true
		for _, selector := range strings.Split(selectors, ",") {
			selector = strings.TrimSpace(selector)
			var offset float64
			switch strings.ToLower(selector) {
			case "from":
				offset = 0
			case "to":
				offset = 1
			default:
				if !strings.HasSuffix(selector, "%") {
					valid = false
					break
				}
				value, err := strconv.ParseFloat(strings.TrimSuffix(selector, "%"), 64)
				if err != nil || !(value >= 0 && value <= 100) {
					valid = false
					break
				}
				offset = value / 100
			}
			frame.Offsets = append(frame.Offsets, offset)
		}
		if valid && len(frame.Offsets) > 0 {
			result.Frames = append(result.Frames, frame)
		}
	}
}
