package font

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSystemFontListCollectionsAndWeights(t *testing.T) {
	regular, err := os.ReadFile("../testdata/fonts/Noto_Sans_SC/static/NotoSansSC-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	bold, err := os.ReadFile("../testdata/fonts/Noto_Sans_SC/static/NotoSansSC-Bold.ttf")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "faces.ttc")
	if err := os.WriteFile(path, testFontCollection(regular, bold), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.ttf"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	// A fontconfig entry and overlapping directory roots must not duplicate faces.
	fonts, err := listSystemFonts([]string{dir, dir}, []string{path})
	if err != nil {
		t.Fatal(err)
	}
	if len(fonts) != 2 {
		t.Fatalf("faces = %d, want 2", len(fonts))
	}
	for index, f := range fonts {
		if f.Family != "Noto Sans SC" || f.Index != index || f.Italic || f.Monospace {
			t.Fatalf("unexpected metadata: %+v", f)
		}
		if want := []int{400, 700}[index]; f.Weight != want {
			t.Fatalf("face %d weight = %d, want %d", index, f.Weight, want)
		}
		face, err := f.Load()
		if err != nil {
			t.Fatal(err)
		}
		if face.GlyphIndex('中') == 0 {
			t.Error("selected collection face lost glyphs")
		}
		if err := face.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSystemFontListEmptyAndCached(t *testing.T) {
	if _, err := listSystemFonts([]string{t.TempDir()}, nil); err == nil {
		t.Fatal("empty directory reported installed fonts")
	}
	if _, err := SystemFonts(); err != nil {
		t.Skip(err)
	}
	if allocations := testing.AllocsPerRun(100, func() { _, _ = SystemFonts() }); allocations != 0 {
		t.Fatalf("cached system font list allocated %g times", allocations)
	}
}
