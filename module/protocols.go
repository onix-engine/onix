package module

import (
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
)

// Engine protocols contain data only; consumers need no provider imports.
type TextRenderRequest struct {
	Text          string
	Size          float64
	Color         [4]float32
	Width, Height float32
	Alignment     scene.TextAlignment
}
type TextRenderResult struct {
	Output        RenderOutputHandle
	Width, Height float32
}
type SceneSwitchRequest struct {
	Scene   project.SceneID
	Startup project.SlotValue
	// Retain applies when the departing scene has no explicit ExitPolicy.
	Retain bool
	// Overlay keeps retained scenes visible beneath the new foreground scene.
	// The background scenes are paused and cannot receive input.
	Overlay bool
}

type ScenePreloadRequest struct {
	Scene   project.SceneID
	Startup project.SlotValue
}

type SceneActivationOptions struct {
	Startup         project.SlotValue
	Retain, Overlay bool
}
type SceneStartup struct {
	From  project.SceneID
	Value project.SlotValue
}
type SceneSwitchResult struct {
	Err      error
	From, To project.SceneID
}
