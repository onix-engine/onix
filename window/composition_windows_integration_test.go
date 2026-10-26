//go:build windows && cgo && onix_window_integration

package window

import (
	"runtime"
	"syscall"
	"testing"

	"github.com/diiyw/onix/render"
	"github.com/ebitengine/purego"
)

// This opt-in test briefly displays two small test windows. It verifies real
// desktop composition after activation, rather than just alpha capability bits.
func TestRoundedWindowAlphaAfterActivation(t *testing.T) {
	var getPixel func(uintptr, int32, int32) uint32
	var flushComposition func() int32
	var getActiveWindow func() hwnd
	purego.RegisterLibFunc(&getPixel, gdi32, "GetPixel")
	purego.RegisterLibFunc(&getActiveWindow, user32, "GetActiveWindow")
	purego.RegisterLibFunc(&flushComposition, syscall.NewLazyDLL("dwmapi.dll").Handle(), "DwmFlush")
	options := DefaultOptions()
	options.Decorations, options.CornerRadius = DecorationNone, 12
	context := NewContext(96, 96, "Window alpha regression", options)
	defer func() {
		context.Destroy()
		runtime.UnlockOSThread() // NewContext owns this thread until destruction.
	}()
	context.SetDecorations(DecorationNone)
	context.SetBorderVisible(false)
	context.SetResizable(true)

	// SS_BLACKRECT supplies a solid, system-colored backdrop behind all four
	// corners. Read its actual color instead of depending on the desktop theme.
	backdrop := createWindowExW(0x08000000|wsExToolWindow, mustUTF16Ptr("STATIC"), mustUTF16Ptr("Alpha test backdrop"),
		wsPopup|wsVisible|4, 30, 30, int32(context.Width+160), int32(context.Height+160), 0, 0, 0, nil)
	if backdrop == 0 {
		t.Fatal("create composition test backdrop")
	}
	defer destroyWindow(backdrop)
	setWindowPos(backdrop, hwndTopmost, 30, 30, 0, 0, swpNoSize|swpNoActivate)
	setWindowPos(context.window, hwndTopmost, 80, 80, 0, 0, swpNoSize|swpNoActivate)
	frames := 0
	if err := context.LoopGame(func() (bool, error) {
		if frames == 5 {
			flushComposition()
			if getActiveWindow() != context.window {
				t.Fatal("composition test window was not activated")
			}
			origin := point{}
			if !clientToScreen(context.window, &origin) {
				t.Fatal("locate composition test surface")
			}
			dc := getDC(0)
			if dc == 0 {
				t.Fatal("read composition test pixels")
			}
			defer releaseDC(0, dc)
			background := getPixel(dc, 40, 40)
			if background == 0xffffffff {
				t.Fatal("read composition test backdrop")
			}
			for _, corner := range [...]point{
				{}, {X: int32(context.Width - 1)}, {Y: int32(context.Height - 1)},
				{X: int32(context.Width - 1), Y: int32(context.Height - 1)},
			} {
				if color := getPixel(dc, origin.X+corner.X, origin.Y+corner.Y); color != background {
					t.Errorf("activated corner %+v = %#06x, want backdrop %#06x", corner, color, background)
				}
			}
			// Desktop color transforms can change channel intensity. Green must
			// remain visible and distinct from the neutral system backdrop.
			if body := getPixel(dc, origin.X+int32(context.Width/2), origin.Y+int32(context.Height/2)); body&0xff00ff != 0 || body&0xff00 == 0 {
				t.Errorf("surface body = %#06x, want opaque green", body)
			}
			context.shouldClose = true
			return false, nil
		}
		frames++
		return true, context.Device.Driver().BeginFrame(render.Color{G: 1, A: 1})
	}); err != nil {
		t.Fatal(err)
	}
}
