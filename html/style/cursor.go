package style

import "strings"

// Cursor stores a computed CSS keyword in one byte rather than a full Value.
// The first keyword is the initial value, auto.
type Cursor uint8

var cursorKeywords = [...]string{
	"auto", "default", "text", "pointer", "crosshair", "move",
	"ew-resize", "ns-resize", "nesw-resize", "nwse-resize", "grab", "grabbing",
	"not-allowed", "wait", "progress", "help", "none", "vertical-text",
	"all-scroll", "context-menu", "cell", "copy", "alias", "zoom-in", "zoom-out",
	"col-resize", "row-resize", "e-resize", "w-resize", "n-resize", "s-resize",
	"ne-resize", "nw-resize", "se-resize", "sw-resize", "no-drop",
}

func (c Cursor) String() string {
	if int(c) >= len(cursorKeywords) {
		return "auto"
	}
	return cursorKeywords[c]
}

func parseCursor(raw string) (Cursor, bool) {
	raw = strings.TrimSpace(raw)
	for cursor, keyword := range cursorKeywords {
		if strings.EqualFold(raw, keyword) {
			return Cursor(cursor), true
		}
	}
	return 0, false
}
