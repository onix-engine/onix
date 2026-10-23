package scene

import (
	"testing"

	onixtext "github.com/diiyw/onix/text"
)

func TestNewText2DUsesSharedSystemFont(t *testing.T) {
	font := onixtext.DefaultFont()
	if font == nil {
		t.Skip("system font unavailable")
	}
	if text := NewText2D("System font"); text.Font != font {
		t.Fatal("default text did not use the shared system font")
	}
}
