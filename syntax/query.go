package syntax

import (
	"strings"
	"sync"
	"unicode"

	"github.com/odvcencio/gotreesitter"
)

var highlightQueries sync.Map

// Contextual captures must follow node-wide defaults: a call's identifier is a
// function, while an identifier elsewhere is a variable. Preserve source order
// within each group so inherited queries and predicate overrides still work.
func highlightQuery(source string, language *gotreesitter.Language) (string, error) {
	if cached, ok := highlightQueries.Load(source); ok {
		return cached.(string), nil
	}
	query, err := gotreesitter.NewQuery(source, language)
	if err != nil {
		return "", err
	}
	var defaults, contextual strings.Builder
	for i := uint32(0); i < uint32(query.PatternCount()); i++ {
		start, _ := query.StartByteForPattern(i)
		end, _ := query.EndByteForPattern(i)
		pattern := source[start:end]
		if nodeWidePattern(pattern) {
			defaults.WriteString(pattern)
			defaults.WriteByte('\n')
		} else {
			contextual.WriteString(pattern)
			contextual.WriteByte('\n')
		}
	}
	defaults.WriteString(contextual.String())
	result := defaults.String()
	highlightQueries.Store(source, result)
	return result, nil
}

func nodeWidePattern(pattern string) bool {
	pattern = strings.TrimSpace(pattern)
	if !strings.HasPrefix(pattern, "(") {
		return false
	}
	end := strings.IndexByte(pattern, ')')
	if end < 0 {
		return false
	}
	for _, c := range strings.TrimSpace(pattern[1:end]) {
		if !(unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_' || c == '/' || c == '-') {
			return false
		}
	}
	comment := false
	for _, c := range pattern[end+1:] {
		if c == ';' {
			comment = true
		} else if c == '\n' {
			comment = false
		} else if !comment && strings.ContainsRune("()[]{}\"", c) {
			return false
		}
	}
	return true
}
