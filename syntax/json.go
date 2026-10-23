package syntax

import (
	"encoding/json"
	"fmt"
)

// decodeJSON accepts the comments and trailing commas allowed in VS Code themes.
func decodeJSON[T any](data []byte, value *T) error {
	out := make([]byte, 0, len(data))
	last := -1
	beforeComma := -1
	for i := 0; i < len(data); {
		c := data[i]
		if c == '"' {
			start := i
			i++
			for i < len(data) {
				if data[i] == '\\' {
					i += 2
					continue
				}
				if data[i] == '"' {
					i++
					break
				}
				i++
			}
			if i > len(data) {
				return fmt.Errorf("unterminated JSON string")
			}
			out = append(out, data[start:i]...)
			last = len(out) - 1
			continue
		}
		if c == '/' && i+1 < len(data) {
			switch data[i+1] {
			case '/':
				i += 2
				for i < len(data) && data[i] != '\n' {
					i++
				}
				out = append(out, ' ')
				continue
			case '*':
				i += 2
				for i+1 < len(data) && (data[i] != '*' || data[i+1] != '/') {
					i++
				}
				if i+1 >= len(data) {
					return fmt.Errorf("unterminated JSON comment")
				}
				i += 2
				out = append(out, ' ')
				continue
			}
		}
		if (c == '}' || c == ']') && last >= 0 && out[last] == ',' && beforeComma >= 0 {
			switch out[beforeComma] {
			case '[', '{', ',', ':':
			default:
				out[last] = ' '
			}
		}
		if c == ',' {
			beforeComma = last
		}
		out = append(out, c)
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			last = len(out) - 1
		}
		i++
	}
	return json.Unmarshal(out, value)
}
