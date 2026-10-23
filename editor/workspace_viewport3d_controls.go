package editor

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

type preview3DControl uint8

const (
	preview3DBackground preview3DControl = iota
	preview3DGrid
	preview3DQuality
	preview3DProjection
	preview3DGround
	preview3DLighting
	preview3DEnvironment
	preview3DIntensity
	preview3DRotation
	preview3DExposure
	preview3DShadows
	preview3DSnap
	preview3DStep
	preview3DFocus
	preview3DPlane
)

type preview3DControlRow struct {
	label, value string
	action       preview3DControl
}

func (a *ModuleEditor) preview3DControlRows() ([6]preview3DControlRow, int) {
	s := a.settings.Viewport3D
	switch a.viewport3D.view.settingsPage {
	case 1:
		source := s.EnvironmentCamera
		if source == "" {
			source = "Auto (scene camera)"
		}
		return [6]preview3DControlRow{
			{"Lighting", [...]string{"Studio", "Scene"}[boolIndex(s.SceneLighting)], preview3DLighting},
			{"Environment source", source, preview3DEnvironment},
			{"Environment strength", [...]string{"0.25", "0.5", "1", "2", "4"}[max(-2, min(2, s.IntensityEV))+2], preview3DIntensity},
			{"Environment rotation", [...]string{"0°", "45°", "90°", "135°", "180°", "225°", "270°", "315°"}[s.Rotation%8], preview3DRotation},
			{"Exposure", [...]string{"−2 EV", "−1 EV", "0 EV", "+1 EV", "+2 EV"}[max(-2, min(2, s.ExposureEV))+2], preview3DExposure},
			{"Shadows", [...]string{"Off", "1024", "2048"}[s.Shadows%3], preview3DShadows},
		}, 6
	case 2:
		step := "Auto"
		for i, value := range preview3DSnapSteps {
			if s.SnapStep == value {
				step = preview3DSnapLabels[i]
			}
		}
		return [6]preview3DControlRow{
			{"Grid snap", onOffLabel(a.settings.SnapToGrid), preview3DSnap},
			{"Snap step (units)", step, preview3DStep},
			{"Frame selection", "Focus · F", preview3DFocus},
		}, 3
	default:
		return [6]preview3DControlRow{
			{"Background", [...]string{"Gradient sky", "Solid gray", "Environment", "Source default"}[s.Background%4], preview3DBackground},
			{"Grid", onOffLabel(!s.HideGrid), preview3DGrid},
			{"Antialiasing", [...]string{"Standard (1×)", "Supersample (2×)"}[boolIndex(s.Supersample)], preview3DQuality},
			{"Projection", [...]string{"Perspective · O", "Orthographic · O"}[boolIndex(a.viewport3D.view.orthographic)], preview3DProjection},
			{"Preview ground", onOffLabel(s.Ground), preview3DGround},
			{"Grid plane", [...]string{"Auto", "XZ", "XY", "YZ"}[s.GridPlane%4], preview3DPlane},
		}, 6
	}
}

func boolIndex(value bool) int {
	if value {
		return 1
	}
	return 0
}

var preview3DSnapSteps = [...]float32{0, 0.01, 0.1, 0.25, 0.5, 1, 2, 5, 10}
var preview3DSnapLabels = [...]string{"Auto", "0.01", "0.1", "0.25", "0.5", "1", "2", "5", "10"}

func cyclePreview3DValue[T comparable](current T, values []T) T {
	for i, value := range values {
		if value == current {
			return values[(i+1)%len(values)]
		}
	}
	return values[0]
}

func (a *ModuleEditor) preview3DViewButtonRect() rect {
	v := a.canvasBounds()
	return rect{X: v.X + 12, Y: v.Y + 10, W: 62, H: editorCompactControlHeight}
}

func (a *ModuleEditor) preview3DSettingsRect() rect {
	v := a.canvasBounds()
	return rect{X: v.X + 12, Y: v.Y + 44, W: min(344, max(v.W-24, 1)), H: min(272, max(v.H-80, 1))}
}

func (a *ModuleEditor) preview3DSettingsTabRect(index int) rect {
	p := a.preview3DSettingsRect()
	w := (p.W - 16) / 3
	return rect{X: p.X + 8 + float32(index)*w, Y: p.Y + 8, W: w, H: 28}
}

func (a *ModuleEditor) preview3DSettingsRowRect(index int) rect {
	p := a.preview3DSettingsRect()
	return rect{X: p.X + 8, Y: p.Y + 44 + float32(index)*min(32, max((p.H-52)/6, 1)), W: p.W - 16, H: min(28, max((p.H-52)/6-4, 1))}
}

func onOffLabel(value bool) string {
	if value {
		return "On"
	}
	return "Off"
}

func viewport3DContains(bounds rect, x, y float32) bool {
	return bounds.W > 0 && bounds.H > 0 && x >= bounds.X && x < bounds.X+bounds.W && y >= bounds.Y && y < bounds.Y+bounds.H
}

func (a *ModuleEditor) handleViewport3DControls(in *input.State) bool {
	p, v := in.Pointer(), &a.viewport3D.view
	if v.orientation.dragging {
		return false
	}
	if p.Pressed && viewport3DContains(a.preview3DViewButtonRect(), p.PressX, p.PressY) {
		v.settingsOpen = !v.settingsOpen
		a.changedCanvas()
		return true
	}
	if v.settingsOpen {
		if p.Pressed {
			if !viewport3DContains(a.preview3DSettingsRect(), p.PressX, p.PressY) {
				v.settingsOpen = false
				a.changedCanvas()
				return true
			}
			for i := range 3 {
				if viewport3DContains(a.preview3DSettingsTabRect(i), p.PressX, p.PressY) {
					v.settingsPage = i
					a.changedCanvas()
					return true
				}
			}
			rows, count := a.preview3DControlRows()
			for i, row := range rows[:count] {
				if viewport3DContains(a.preview3DSettingsRowRect(i), p.PressX, p.PressY) {
					a.applyViewport3DControl(row.action)
					return true
				}
			}
		}
		if viewport3DContains(a.preview3DSettingsRect(), p.X, p.Y) {
			return true
		}
	}
	return viewport3DContains(a.preview3DViewButtonRect(), p.X, p.Y)
}

func (a *ModuleEditor) applyViewport3DControl(action preview3DControl) {
	s, v := &a.settings.Viewport3D, &a.viewport3D.view
	switch action {
	case preview3DBackground:
		s.Background = (s.Background + 1) % 4
	case preview3DGrid:
		s.HideGrid = !s.HideGrid
	case preview3DPlane:
		s.GridPlane = (s.GridPlane + 1) % 4
	case preview3DQuality:
		s.Supersample = !s.Supersample
	case preview3DProjection:
		v.orthographic = !v.orthographic
		a.syncViewport3DProjection()
	case preview3DGround:
		s.Ground = !s.Ground
	case preview3DLighting:
		s.SceneLighting = !s.SceneLighting
	case preview3DEnvironment:
		next := ""
		for _, row := range a.viewport3D.rows {
			for _, part := range row.parts {
				if part.cameraSpec != nil && part.cameraID > s.EnvironmentCamera && (next == "" || part.cameraID < next) {
					next = part.cameraID
				}
			}
		}
		s.EnvironmentCamera, s.SceneLighting = next, true
	case preview3DIntensity:
		s.IntensityEV = cyclePreview3DValue(s.IntensityEV, []int{0, 1, 2, -2, -1})
	case preview3DExposure:
		s.ExposureEV = cyclePreview3DValue(s.ExposureEV, []int{0, 1, 2, -2, -1})
	case preview3DRotation:
		s.Rotation = (s.Rotation + 1) % 8
	case preview3DShadows:
		s.Shadows = (s.Shadows + 1) % 3
	case preview3DSnap:
		a.settings.SnapToGrid = !a.settings.SnapToGrid
	case preview3DStep:
		s.SnapStep = cyclePreview3DValue(s.SnapStep, preview3DSnapSteps[:])
	case preview3DFocus:
		a.zoom = 1
		a.focusViewport3D()
	}
	a.changed()
	a.report(saveEditorSettings(a.settings))
}

func (a *ModuleEditor) drawViewport3DControls(c *graphics.Canvas2D) {
	v := &a.viewport3D.view
	drawPreview3DOrientationShape(c, a.preview3DViewButtonRect(), 6, editorTheme.panelSoft, editorTheme.panelBorder, 1)
	button := a.preview3DViewButtonRect()
	a.drawViewport3DText(c, "3d/view", "View", mgl32.Vec2{button.X + button.W/2, button.Y + button.H/2}, 12, scene.AnchorCenter, editorTheme.text)
	if !v.settingsOpen {
		return
	}
	drawPreview3DOrientationShape(c, a.preview3DSettingsRect(), 8, editorTheme.panel, editorTheme.panelBorder, 1)
	for i, title := range [...]string{"Display", "Lighting", "Tools"} {
		tab := a.preview3DSettingsTabRect(i)
		fill := editorTheme.panelSoft
		if i == v.settingsPage {
			fill = editorTheme.inputFocus
		}
		drawPreview3DOrientationShape(c, tab, 5, fill, graphics.ColorRGBA{}, 0)
		a.drawViewport3DText(c, [...]string{"3d/tab0", "3d/tab1", "3d/tab2"}[i], title, mgl32.Vec2{tab.X + tab.W/2, tab.Y + tab.H/2}, 12, scene.AnchorCenter, editorTheme.text)
	}
	rows, count := a.preview3DControlRows()
	for i, row := range rows[:count] {
		area := a.preview3DSettingsRowRect(i)
		split := area.W * .46
		a.drawViewport3DText(c, [...]string{"3d/label0", "3d/label1", "3d/label2", "3d/label3", "3d/label4", "3d/label5"}[i], row.label, mgl32.Vec2{area.X + 4, area.Y + area.H/2}, 12, scene.AnchorCenterLeft, editorTheme.textMuted)
		area.X, area.W = area.X+split, area.W-split
		drawPreview3DOrientationShape(c, area, 5, editorTheme.panelSoft, editorTheme.panelBorder, 1)
		a.drawViewport3DText(c, [...]string{"3d/value0", "3d/value1", "3d/value2", "3d/value3", "3d/value4", "3d/value5"}[i], row.value, mgl32.Vec2{area.X + area.W/2, area.Y + area.H/2}, 12, scene.AnchorCenter, editorTheme.text)
	}
}

func (a *ModuleEditor) viewport3DSnapStep() float32 {
	if step := a.settings.Viewport3D.SnapStep; step > 0 {
		return step
	}
	v := &a.viewport3D.view
	projection := ecs.GetRef[scene.Projection](a.viewport3D.world, v.camera)
	step, _ := preview3DGridLOD(preview3DWorldPerPixel(projection, v.distance/max(.05, a.zoom), a.canvasBounds()))
	return step
}
