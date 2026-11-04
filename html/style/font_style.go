package style

// FontStyle is compact because every computed style stores this inherited value.
type FontStyle uint8

const (
	FontNormal FontStyle = iota
	FontItalic
	FontOblique
)

func fontStyleKeyword(keyword string) FontStyle {
	switch keyword {
	case "italic":
		return FontItalic
	case "oblique":
		return FontOblique
	default:
		return FontNormal
	}
}
