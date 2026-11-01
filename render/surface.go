package render

import "unsafe"

type NativeSurfaceKind int

const (
	NativeSurfaceXlib NativeSurfaceKind = iota + 1
	NativeSurfaceWayland
	NativeSurfaceAndroid
	NativeSurfaceWin32
	NativeSurfaceDRM
	NativeSurfaceMetal
)

type NativeSurface struct {
	Kind           NativeSurfaceKind
	Display        unsafe.Pointer
	Window         uintptr
	Width          int
	Height         int
	Transparent    bool
	DRMFD          int
	DRMConnectorID uint32
	DRMRefreshRate uint32 // Millihertz; zero selects the highest rate at Width x Height.
}
