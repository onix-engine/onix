package style

import (
	"runtime"
	"testing"
	"weak"

	"github.com/onix-engine/onix/html/css"
)

func abandonedCompiledMatcher(t *testing.T) (weak.Pointer[css.Stylesheet], weak.Pointer[compiledStylesheet]) {
	t.Helper()
	sheet, err := css.Parse(`.unused {color: red} @keyframes unused {from {opacity:0} to {opacity:1}}`)
	if err != nil {
		t.Fatal(err)
	}
	compiled := compiledMatcher(sheet)
	if compiledMatcher(sheet) != compiled {
		t.Fatal("live compiled matcher was not reused")
	}
	return weak.Make(sheet), weak.Make(compiled)
}

func TestCompiledMatcherCacheDoesNotRetainStylesheet(t *testing.T) {
	sheet, compiled := abandonedCompiledMatcher(t)
	for range 10 {
		runtime.GC()
		if sheet.Value() == nil && compiled.Value() == nil {
			return
		}
	}
	t.Fatal("global convenience cache retained an unused stylesheet or matcher")
}
