package module

import (
	"github.com/diiyw/onix/window"
	"time"
)

// Host provides platform capabilities without exposing the application world.
type Host struct {
	Context              *window.Context
	RequestRender        func()
	WakeAt               func(time.Time)
	RequestClose         func()
	Minimize             func()
	ToggleMaximize       func()
	ToggleFullscreen     func()
	SetWindowTitle       func(string)
	SetViewport          func(int, int)
	GetViewport          func() (int, int)
	SetWindowDragRegion  func(func(float64, float64) bool)
	SetWindowInputRegion func([]window.InputRegionRect)
	OnCloseRequested     func(func() bool)
	OpenFolder           func(window.FileDialogOptions) (string, error)
	OpenFile             func(window.FileDialogOptions) (string, error)
	OpenFiles            func(window.FileDialogOptions) ([]string, error)
	SetClipboardText     func(string) bool
	SetMouseCaptured     func(bool) bool
	MouseCaptured        func() bool
	TakeMouseDelta       func() (float64, float64)
}

func (c *Context) Host() *Host { return c.runtime.options.Host }
