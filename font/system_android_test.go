package font

import (
	"strings"
	"testing"
)

func TestAndroidLoadsConfiguredSystemFontWithoutBundledAssets(t *testing.T) {
	ref, ok := platformDefaultFont()
	if !ok {
		t.Fatal("system font configuration did not resolve a default font")
	}
	if !strings.HasPrefix(ref.path, "/system/fonts/") && !strings.HasPrefix(ref.path, "/product/fonts/") && !strings.HasPrefix(ref.path, "/vendor/fonts/") && !strings.HasPrefix(ref.path, "/data/fonts/") {
		t.Fatalf("font did not come from a system directory: %s", ref.path)
	}
	face, err := LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	defer face.Close()
	if face.GlyphIndex('A') == 0 {
		t.Fatal("default system font does not contain a Latin glyph")
	}
	t.Logf("system font: %s, collection index: %d", ref.path, ref.index)
}
