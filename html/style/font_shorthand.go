package style

import (
	"fmt"
	"strings"
)

type fontShorthand struct {
	line, family string
	size, weight Value
	style        FontStyle
}

func parseFontShorthandParts(raw string, ctx parseContext) (fontShorthand, error) {
	result := fontShorthand{weight: Number(400)}
	styleSeen, weightSeen := false, false
	parts := splitCSSFields(strings.TrimSpace(raw))
	for i, part := range parts {
		part = strings.ToLower(part)
		candidate, line, _ := strings.Cut(part, "/")
		if size, err := parseFontSize(candidate, ctx); err == nil && size.Kind == ValueLengthPx {
			result.size, result.line = size, line
			result.family = strings.Trim(strings.TrimSpace(strings.Join(parts[i+1:], " ")), `"'`)
			if result.family == "" {
				break
			}
			return result, nil
		}
		if part == "normal" {
			continue
		}
		if (part == "italic" || part == "oblique") && !styleSeen {
			result.style, styleSeen = fontStyleKeyword(part), true
			continue
		}
		if weight, err := parseFontWeight(part); err == nil && !weightSeen {
			result.weight, weightSeen = weight, true
			continue
		}
		break
	}
	return fontShorthand{}, fmt.Errorf("invalid font shorthand %q", raw)
}
