package font

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAndroidFontConfigurationSelectsRegularFaceAndCollectionIndex(t *testing.T) {
	system, product := t.TempDir(), t.TempDir()
	for _, name := range []string{"Thin.ttf", "Italic.ttf", "Vendor.otc", "Variable.ttf"} {
		if err := os.WriteFile(filepath.Join(product, name), []byte("font fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		name, config, file string
		index              int
	}{
		{"vendor", `<familyset><family name="sans-serif"><font weight="100">Thin.ttf</font><font weight="400" style="italic">Italic.ttf</font><font weight="400" index="3">Vendor.otc</font></family></familyset>`, "Vendor.otc", 3},
		{"variable", `<familyset><family name="sans-serif" supportedAxes="wght,ital"><font>Variable.ttf<axis tag="wght" stylevalue="400"/></font></family></familyset>`, "Variable.ttf", 0},
		{"missing", `<familyset><family name="sans-serif"><font weight="400">Missing.ttf</font><font weight="300">Thin.ttf</font></family></familyset>`, "Thin.ttf", 0},
		{"first-family", `<familyset><family name="custom-ui"><font weight="400">Vendor.otc</font></family><family lang="zh-Hans"><font>Variable.ttf</font></family></familyset>`, "Vendor.otc", 0},
		{"named-default", `<familyset><family name="serif"><font weight="400">Thin.ttf</font></family><family name="sans-serif"><font weight="400">Variable.ttf</font></family></familyset>`, "Variable.ttf", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			ref, ok := androidFontReference(strings.NewReader(test.config), []string{system, product})
			if !ok || ref.path != filepath.Join(product, test.file) || ref.index != test.index {
				t.Fatalf("reference = %+v, found=%v", ref, ok)
			}
		})
	}
}

func TestAndroidFontConfigurationRejectsUnusableEntries(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "Font.ttf"), []byte("font fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{
		`<familyset><family name="sans-serif"><font index="-1">Font.ttf</font></family></familyset>`,
		`<familyset><family name="sans-serif"><font weight="400" style="italic">Font.ttf</font></family></familyset>`,
		`<familyset><family name="sans-serif"><font>Missing.ttf</font></family></familyset>`,
		`<familyset><family name="sans-serif"><font`,
	} {
		if ref, ok := androidFontReference(strings.NewReader(source), []string{directory}); ok {
			t.Fatalf("unusable configuration selected %+v", ref)
		}
	}
}

func TestDefaultFontReferenceDoesNotAllocateAfterWarmup(t *testing.T) {
	if _, err := DefaultFontPath(); err != nil {
		t.Skip(err)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if _, err := DefaultFontPath(); err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatalf("cached font discovery allocated %v times", allocations)
	}
}
