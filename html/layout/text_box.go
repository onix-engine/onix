package layout

import (
	"math"
	"strings"

	"github.com/diiyw/onix/font"
	"github.com/diiyw/onix/html/style"
)

// textBoxTrimming resolves the first/last line edges of a horizontal block
// container (CSS Inline Layout 3 §6). Glyphs retain their original line boxes;
// only the container's outer leading is removed, preserving baseline spacing.
func textBoxTrimming(computed *style.ComputedStyle, face *font.Face) (float64, float64) {
	trim := computed.TextBoxTrim.Keyword
	if trim == "none" || trim == "" || face == nil || face.Head.UnitsPerEm == 0 {
		return 0, 0
	}
	size := computed.FontSize.LengthPx
	if size <= 0 {
		size = 16
	}
	scale := size / float64(face.Head.UnitsPerEm)
	ascent, descent := float64(face.Hhea.Ascent)*scale, math.Abs(float64(face.Hhea.Descent))*scale
	height := UsedLineHeight(computed, face)
	baseline := ascent + (height-ascent-descent)/2
	over, under, _ := strings.Cut(computed.TextBoxEdge.Keyword, " ")
	switch over {
	case "cap":
		ascent = float64(face.CapHeight()) * scale
	case "ex":
		ascent = float64(face.XHeight()) * scale
	}
	if under == "alphabetic" {
		descent = 0
	}
	start, end := 0.0, 0.0
	if trim == "trim-start" || trim == "trim-both" {
		start = baseline - ascent
	}
	if trim == "trim-end" || trim == "trim-both" {
		end = height - baseline - descent
	}
	return start, end
}
