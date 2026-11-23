package syntax

import (
	"archive/zip"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"weak"

	textmate "github.com/eugenioenko/textmate-go"
	"github.com/eugenioenko/textmate-go/grammars"
)

//go:generate python3 pack_grammars.py

//go:embed grammars.zip
var grammarAssets string

var grammarFiles = sync.OnceValues(func() (*zip.Reader, error) {
	return zip.NewReader(strings.NewReader(grammarAssets), int64(len(grammarAssets)))
})

// Both sets of keys come from the embedded catalog. Open documents own the
// compiled grammars, and their registries own the raw rules for lazy includes.
var grammarCache = struct {
	compiled map[string]weak.Pointer[textmate.Grammar]
	raw      map[string]weak.Pointer[textmate.RawGrammar]
	sync.Mutex
}{
	compiled: make(map[string]weak.Pointer[textmate.Grammar]),
	raw:      make(map[string]weak.Pointer[textmate.RawGrammar]),
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
	if grammar := grammarCache.compiled[scope].Value(); grammar != nil {
		return grammar, nil
	}
	registry := textmate.NewRegistry(textmate.RegistryOptions{LoadGrammar: loadRawGrammar})
	grammar, err := registry.LoadGrammar(scope)
	if err != nil {
		registry.Dispose()
		return nil, fmt.Errorf("syntax grammar %s: %w", scope, err)
	}
	grammarCache.compiled[scope] = weak.Make(grammar)
	return grammar, nil
}

// Called synchronously by Registry.LoadGrammar while grammarCache is locked.
func loadRawGrammar(scope string) (*textmate.RawGrammar, error) {
	if raw := grammarCache.raw[scope].Value(); raw != nil {
		return raw, nil
	}
	var source fs.FS = resources
	name := "resources/grammars/vimdoc.json"
	if scope != "text.vim-help" {
		files, err := grammarFiles()
		if err != nil {
			return nil, err
		}
		source, name = files, scope
	}
	data, err := fs.ReadFile(source, name)
	if errors.Is(err, fs.ErrNotExist) && scope != "text.vim-help" {
		return nil, nil // Optional include outside the embedded catalog.
	}
	if err != nil {
		return nil, err
	}
	raw, err := textmate.ParseRawGrammar(data)
	if err == nil {
		grammarCache.raw[scope] = weak.Make(raw)
	}
	return raw, err
}

// ReleaseGrammarCache forgets shared lookup entries. Active documents keep
// their grammar, line states and captures.
func ReleaseGrammarCache() {
	grammarCache.Lock()
	clear(grammarCache.compiled)
	clear(grammarCache.raw)
	grammarCache.Unlock()
}
