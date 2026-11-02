package syntax

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	textmate "github.com/eugenioenko/textmate-go"
	"github.com/eugenioenko/textmate-go/grammars"
)

// Bound compiled grammar retention independently of live documents. Each entry
// owns a registry so evicting a language also releases its compiled includes.
var grammarCache struct {
	sync.Mutex
	entries [4]grammarEntry
	next    int
}

type grammarEntry struct {
	scope   string
	grammar *textmate.Grammar
}

func detectLanguage(filename string) (string, string) {
	if strings.EqualFold(filename, "vimdoc") {
		return "vimdoc", "text.vim-help"
	}
	if strings.EqualFold(filepath.Ext(filename), ".txt") && !strings.EqualFold(filepath.Base(filename), "CMakeLists.txt") {
		return "", ""
	}
	info, ok := grammars.InfoForFilename(filename)
	if !ok {
		info, ok = grammars.InfoForID(filename)
	}
	if !ok {
		info, ok = grammars.InfoForAlias(filename)
	}
	if !ok {
		return "", ""
	}
	return info.ID, info.ScopeName
}

func loadGrammar(scope string) (*textmate.Grammar, error) {
	if scope == "" {
		return nil, nil
	}
	grammarCache.Lock()
	defer grammarCache.Unlock()
	for _, entry := range grammarCache.entries {
		if entry.scope == scope {
			return entry.grammar, nil
		}
	}
	registry := textmate.NewRegistry(textmate.RegistryOptions{LoadGrammar: func(name string) (*textmate.RawGrammar, error) {
		if name == "text.vim-help" {
			data, err := resources.ReadFile("resources/grammars/vimdoc.json")
			if err != nil {
				return nil, err
			}
			return textmate.ParseRawGrammar(data)
		}
		return grammars.Load(name)
	}})
	grammar, err := registry.LoadGrammar(scope)
	if err != nil {
		registry.Dispose()
		return nil, fmt.Errorf("syntax grammar %s: %w", scope, err)
	}
	// The grammar retains the registry for lazy compilation of included rules.
	grammarCache.entries[grammarCache.next] = grammarEntry{scope, grammar}
	grammarCache.next = (grammarCache.next + 1) % len(grammarCache.entries)
	return grammar, nil
}

// ReleaseGrammarCache drops idle compiled grammars. Active documents keep their
// grammar valid and do not lose line states or captures.
func ReleaseGrammarCache() {
	grammarCache.Lock()
	clear(grammarCache.entries[:])
	grammarCache.next = 0
	grammarCache.Unlock()
}
