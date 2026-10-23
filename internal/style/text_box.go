package style

import (
	"fmt"
	"strings"
)

func parseTextBoxEdge(raw string) (Value, error) {
	parts := strings.Fields(strings.ToLower(raw))
	if len(parts) == 1 && (parts[0] == "auto" || parts[0] == "text") {
		return Keyword(parts[0]), nil
	}
	if len(parts) != 2 {
		return Value{}, fmt.Errorf("invalid text-box-edge %q", raw)
	}
	switch parts[0] {
	case "text", "cap", "ex":
	default:
		return Value{}, fmt.Errorf("unsupported text-box-edge %q", raw)
	}
	under := parts[1]
	if under != "text" && under != "alphabetic" {
		return Value{}, fmt.Errorf("unsupported text-box-edge %q", raw)
	}
	return Keyword(parts[0] + " " + under), nil
}
