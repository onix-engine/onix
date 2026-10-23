package css

import (
	"fmt"
	"strings"
)

// StripComments preserves quoted text and separates tokens around comments.
func StripComments(input string) (string, error) {
	if !strings.Contains(input, "/*") {
		return input, nil
	}
	var builder strings.Builder
	writeStart := 0
	var quote byte
	escaped := false
	for index := 0; index < len(input); index++ {
		ch := input[index]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			continue
		}
		if ch != '/' || index+1 >= len(input) || input[index+1] != '*' {
			continue
		}
		if builder.Cap() == 0 {
			builder.Grow(len(input))
		}
		builder.WriteString(input[writeStart:index])
		commentEnd := strings.Index(input[index+2:], "*/")
		if commentEnd < 0 {
			return "", fmt.Errorf("unterminated CSS comment")
		}
		builder.WriteByte(' ')
		index += 2 + commentEnd + 1
		writeStart = index + 1
	}
	if builder.Cap() == 0 {
		return input, nil
	}
	builder.WriteString(input[writeStart:])
	return builder.String(), nil
}
