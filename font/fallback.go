package font

import (
	"bytes"
	"cmp"
	"slices"
	"sync"
)

type resolvedGlyph struct {
	face *Face
	id   uint16
}

// Sources and their parsed faces are shared across styles for the process
// lifetime. Coverage is read first so a missing character never loads every
// installed font's outlines and metrics.
type fallbackSource struct {
	font     SystemFont
	cmapOnce sync.Once
	coverage cmap
	faceOnce sync.Once
	face     *Face
}

func (s *fallbackSource) glyph(r rune) resolvedGlyph {
	s.cmapOnce.Do(func() {
		backing, err := loadFontFile(s.font.Path)
		if err != nil {
			return
		}
		defer backing.close()
		data, err := parseSFNT(backing.data, s.font.Index)
		if err != nil {
			return
		}
		table, err := data.table("cmap")
		if err == nil {
			// Format 4 retains table bytes after the temporary mapping closes.
			s.coverage, _ = parseCmap(bytes.Clone(table))
		}
	})
	if s.coverage == nil || s.coverage.GlyphIndex(r) == 0 {
		return resolvedGlyph{}
	}
	s.faceOnce.Do(func() { s.face, _ = s.font.Load() })
	if s.face == nil || s.face.Head.UnitsPerEm == 0 {
		return resolvedGlyph{}
	}
	return resolvedGlyph{s.face, s.face.GlyphIndex(r)}
}

type fallbackStyle struct {
	weight int
	italic bool
}

type fallbackSet struct {
	once    sync.Once
	style   fallbackStyle
	sources []*fallbackSource
	mu      sync.RWMutex
	glyphs  map[rune]resolvedGlyph
}

var fallbackSources = sync.OnceValue(func() []*fallbackSource {
	fonts, _ := SystemFonts()
	sources := make([]*fallbackSource, len(fonts))
	for i, f := range fonts {
		sources[i] = &fallbackSource{font: f}
	}
	return sources
})

var systemFallbacks = struct {
	sync.Mutex
	styles map[fallbackStyle]*fallbackSet
}{styles: make(map[fallbackStyle]*fallbackSet)}

// UseSystemFallback enables lazy, shared system glyph fallback. Configure the
// face before measuring or rendering it; raw glyph-ID APIs remain face-local.
// Closing this face does not close the shared fallback faces.
func (f *Face) UseSystemFallback() {
	if f == nil || f.fallback != nil {
		return
	}
	style := fallbackStyle{weight: f.weight, italic: f.italic}
	if style.weight == 0 {
		style.weight = 400
	}
	systemFallbacks.Lock()
	defer systemFallbacks.Unlock()
	set := systemFallbacks.styles[style]
	if set == nil {
		set = &fallbackSet{style: style}
		systemFallbacks.styles[style] = set
	}
	f.fallback = set
}

// ResolveGlyph returns both the owning face and its local glyph ID. If every
// candidate is missing the character, it returns this face's .notdef glyph.
// System fallback faces are shared; callers must not modify or close them.
func (f *Face) ResolveGlyph(r rune) (*Face, uint16) {
	id := f.GlyphIndex(r)
	if f == nil || id != 0 || f.fallback == nil {
		return f, id
	}
	g := f.fallback.resolve(r)
	if g.face != nil {
		return g.face, g.id
	}
	return f, 0
}

func (s *fallbackSet) resolve(r rune) resolvedGlyph {
	s.once.Do(func() {
		s.sources = slices.Clone(fallbackSources())
		slices.SortStableFunc(s.sources, func(a, b *fallbackSource) int {
			if rank := cmp.Compare(fallbackFamilyRank(a.font.Family), fallbackFamilyRank(b.font.Family)); rank != 0 {
				return rank
			}
			return cmp.Compare(s.style.distance(a.font), s.style.distance(b.font))
		})
		s.glyphs = make(map[rune]resolvedGlyph)
	})
	s.mu.RLock()
	g, ok := s.glyphs[r]
	s.mu.RUnlock()
	if ok {
		return g
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if g, ok = s.glyphs[r]; ok {
		return g
	}
	for _, source := range s.sources {
		if g = source.glyph(r); g.face != nil {
			break
		}
	}
	// Cache misses as well as hits; unsupported characters never rescan fonts.
	s.glyphs[r] = g
	return g
}

func (s fallbackStyle) distance(f SystemFont) int {
	distance := max(f.Weight-s.weight, s.weight-f.Weight)
	if f.Italic != s.italic {
		distance += 1000
	}
	return distance
}

func fallbackFamilyRank(family string) int {
	switch family {
	case "Microsoft YaHei UI", "PingFang SC", "Noto Sans CJK SC", "Noto Sans SC":
		return 0
	case "Microsoft YaHei", "Hiragino Sans GB", "WenQuanYi Micro Hei":
		return 1
	case "Segoe UI", "Arial", "Noto Sans":
		return 2
	default:
		return 3
	}
}
