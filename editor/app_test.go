package editor

import (
	"os"
	"testing"

	engineui "github.com/diiyw/onix/ui"
	"github.com/diiyw/onix/window"
)

func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "onix-editor-test-home-")
	if err != nil {
		panic(err)
	}
	for _, name := range []string{"HOME", "USERPROFILE"} {
		if err := os.Setenv(name, home); err != nil {
			panic(err)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}

func TestEditorHoverAndActiveHighlightsAreDistinctOutsideViewport(t *testing.T) {
	theme := editorUITheme(nil)
	if theme.SurfaceHover != editorTheme.hover || theme.PrimaryHover != editorTheme.hover {
		t.Fatal("expected UI hover highlights to use the low-contrast gray")
	}
	if theme.SurfaceActive != editorTheme.inputFocus || theme.PrimaryActive != editorTheme.inputFocus {
		t.Fatal("expected active UI highlights to remain blue")
	}
	if theme.PrimaryActive == theme.PrimaryHover || theme.SurfaceActive == theme.SurfaceHover {
		t.Fatal("expected hover and active highlights to remain distinct")
	}
	button := engineui.DefaultSkin(theme).Button.Primary.Body
	if fill := button.Resolve(engineui.ControlState{Hovered: true}).Background.Fill; fill != editorTheme.hover {
		t.Fatalf("hovered button background = %v, want %v", fill, editorTheme.hover)
	}
	if fill := button.Resolve(engineui.ControlState{Selected: true}).Background.Fill; fill != editorTheme.inputFocus {
		t.Fatalf("active button background = %v, want %v", fill, editorTheme.inputFocus)
	}
}

func TestEditorWindowUsesFullHeightMacOSTitleBar(t *testing.T) {
	options := editorWindowOptions()
	layout := computeEditorLayout(rect{W: 1600, H: 960}, 0, 0, 0)
	if options.Decorations != window.DecorationNone {
		t.Fatalf("editor window options = %#v", options)
	}
	if center, want := options.TitleBarHeight/2, float64(layout.TopBar.Y+layout.TopBar.H/2); center != want {
		t.Fatalf("native window buttons center = %g, want titlebar center %g", center, want)
	}
}

func TestViewportGridStepFollowsZoom(t *testing.T) {
	if step := viewportGridStep(0.1); step != 256 {
		t.Fatalf("zoomed-out snap step = %g, want 256", step)
	}
}
