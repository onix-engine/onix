package editor

import (
	"fmt"
	"math"
	"strconv"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	engineui "github.com/diiyw/onix/ui"
	htmlui "github.com/diiyw/onix/ui/html"
)

func (a *ModuleEditor) renderAnimationCurvePopup(b *editorVNodeView) {
	s := &a.animation
	if !s.CurveOpen {
		return
	}
	clip, ok := a.animationClip()
	if !ok || !s.Active || !s.Segment {
		s.CurveOpen = false
		return
	}
	height := float32(468)
	title := "Animation transition curve"
	var binding scene.AnimationBinding
	var tween scene.AnimationTween
	switch s.Kind {
	case "float":
		track := clip.FloatTracks[s.Track]
		binding, tween = track.Binding, track.Keyframes[s.Key].Tween
	case "texture":
		track := clip.TextureTracks[s.Track]
		binding, tween = track.Binding, track.TimeTween
		title, height = "Sequence playback curve", 548
	case "bool":
		binding = clip.BoolTracks[s.Track].Binding
		title, height = "Visibility transition", 458
	}
	if tween == scene.AnimationBezier {
		height += 130
	}
	panel := a.dialogPanel(480, height)
	b.open("div", "animation-curve-overlay", "class", "animation-curve-overlay", "style", workspaceRectStyle(rect{W: a.width, H: a.height}))
	b.open("section", "animation-curve-dialog", "id", "animation-curve-dialog", "class", "animation-curve-popup", "role", "dialog", "aria-modal", "true", "aria-labelledby", "animation-curve-title", "style", workspaceRectStyle(panel))
	b.open("header", "header", "class", "animation-curve-header")
	b.text("h2", "title", title, "id", "animation-curve-title")
	b.open("button", "animation:close-curve", "id", "animation:close-curve", "type", "button", "class", "animation-curve-close icon-only", "aria-label", "Close dialog", "title", "Close")
	b.text("img", "icon", "", "src", "onix-titlebar/close.svg", "alt", "")
	b.close()
	b.close()
	b.text("p", "track", animationTrackLabel(binding), "class", "animation-curve-track")
	switch s.Kind {
	case "float":
		a.renderAnimationInterpolation(b, clip.FloatTracks[s.Track].Keyframes)
	case "texture":
		a.renderAnimationSequenceCurve(b, clip.TextureTracks[s.Track])
	case "bool":
		renderAnimationStepTransition(a, b, s, clip.BoolTracks)
	}
	if a.dialogError != "" {
		b.text("p", "error", a.dialogError, "class", "danger", "role", "alert")
	}
	b.open("footer", "actions", "class", "animation-curve-actions")
	b.button("animation:save-curve", "Save", "type", "button", "class", "primary")
	b.close()
	b.close()
	b.close()
}

func animationTrackLabel(binding scene.AnimationBinding) string {
	name := string(binding.Target)
	for _, target := range animationTargets {
		if target.Value == name {
			name = target.Label
			break
		}
	}
	if binding.Path != "" {
		name = binding.Path + " / " + name
	}
	return name
}

func editableAnimationCurve(s *workspaceAnimation, clip *project.AnimationClipSpec) (*scene.AnimationTween, *scene.AnimationCurve, error) {
	if s.Kind == "texture" && s.Track >= 0 && s.Track < len(clip.TextureTracks) {
		track := &clip.TextureTracks[s.Track]
		return &track.TimeTween, &track.TimeCurve, nil
	}
	if s.Kind == "float" {
		keys, err := animationKeys(s, clip.FloatTracks)
		if err == nil && s.Key >= 0 && s.Key < len(keys) {
			return &keys[s.Key].Tween, &keys[s.Key].Curve, nil
		}
	}
	return nil, nil, fmt.Errorf("select a track with an editable curve")
}

func (a *ModuleEditor) renderAnimationSequenceCurve(b *editorVNodeView, track project.AnimationTrackSpec[project.TextureSpec]) {
	keys := track.Keyframes
	b.text("p", "interval", fmt.Sprintf("Whole sequence: %g → %g s · %d frames", keys[0].Time, keys[len(keys)-1].Time, len(keys)))
	b.text("p", "scope", "Controls playback speed across the entire sequence. A steeper curve advances frames faster.", "class", "animation-curve-help")
	tween := track.TimeTween
	if tween == "" {
		tween = scene.AnimationLinear
	}
	a.animationSelect(b, "tween", "Playback curve", string(tween), &animationTimeTweens)
	a.renderAnimationCurve(b, tween, track.TimeCurve, [3]float32{})
	if tween == scene.AnimationBezier {
		a.renderAnimationBezierControls(b, track.TimeCurve)
	} else {
		b.button("animation:custom-curve", "Edit custom Bezier", "class", "animation-curve-custom")
	}
}

func (a *ModuleEditor) renderAnimationBezierControls(b *editorVNodeView, curve scene.AnimationCurve) {
	b.text("p", "drag-help", "Drag the two handles to shape the curve, or enter their coordinates.", "class", "animation-curve-help")
	b.open("div", "handles", "class", "animation-curve-controls")
	a.animationControl(b, "x1", "Handle 1 X", "number", animationNumber(curve.X1))
	a.animationControl(b, "y1", "Handle 1 Y", "number", animationNumber(curve.Y1))
	a.animationControl(b, "x2", "Handle 2 X", "number", animationNumber(curve.X2))
	a.animationControl(b, "y2", "Handle 2 Y", "number", animationNumber(curve.Y2))
	b.close()
}

func renderAnimationStepTransition[T any](a *ModuleEditor, b *editorVNodeView, s *workspaceAnimation, tracks []project.AnimationTrackSpec[T]) {
	keys := tracks[s.Track].Keyframes
	b.text("p", "interval", fmt.Sprintf("Interval: %g → %g s", keys[s.Key].Time, keys[s.Key+1].Time))
	b.text("p", "discrete", "This track switches values at keyframes and uses Step interpolation.")
	a.renderAnimationCurve(b, scene.AnimationStep, scene.AnimationCurve{}, [3]float32{})
}

func animationTweenOptions() workspaceContentProperty {
	choices := [...]struct {
		label string
		tween scene.AnimationTween
	}{
		{"Step", scene.AnimationStep}, {"Linear", scene.AnimationLinear},
		{"Sine / In", scene.AnimationEaseInSine}, {"Sine / Out", scene.AnimationEaseOutSine}, {"Sine / In-out", scene.AnimationEaseInOutSine},
		{"Quad / In", scene.AnimationEaseIn}, {"Quad / Out", scene.AnimationEaseOut}, {"Quad / In-out", scene.AnimationEaseInOut},
		{"Cubic / In", scene.AnimationEaseInCubic}, {"Cubic / Out", scene.AnimationEaseOutCubic}, {"Cubic / In-out", scene.AnimationEaseInOutCubic},
		{"Quart / In", scene.AnimationEaseInQuart}, {"Quart / Out", scene.AnimationEaseOutQuart}, {"Quart / In-out", scene.AnimationEaseInOutQuart},
		{"Quint / In", scene.AnimationEaseInQuint}, {"Quint / Out", scene.AnimationEaseOutQuint}, {"Quint / In-out", scene.AnimationEaseInOutQuint},
		{"Expo / In", scene.AnimationEaseInExpo}, {"Expo / Out", scene.AnimationEaseOutExpo}, {"Expo / In-out", scene.AnimationEaseInOutExpo},
		{"Circ / In", scene.AnimationEaseInCirc}, {"Circ / Out", scene.AnimationEaseOutCirc}, {"Circ / In-out", scene.AnimationEaseInOutCirc},
		{"Back / In", scene.AnimationEaseInBack}, {"Back / Out", scene.AnimationEaseOutBack}, {"Back / In-out", scene.AnimationEaseInOutBack},
		{"Elastic / In", scene.AnimationEaseInElastic}, {"Elastic / Out", scene.AnimationEaseOutElastic}, {"Elastic / In-out", scene.AnimationEaseInOutElastic},
		{"Bounce / In", scene.AnimationEaseInBounce}, {"Bounce / Out", scene.AnimationEaseOutBounce}, {"Bounce / In-out", scene.AnimationEaseInOutBounce},
		{"Custom Bezier", scene.AnimationBezier}, {"Cubic spline", scene.AnimationCubicSpline},
	}
	options := workspaceContentProperty{Label: "Interpolation", Options: make([]string, len(choices)), Values: make([]string, len(choices))}
	for i, choice := range choices {
		options.Options[i], options.Values[i] = choice.label, string(choice.tween)
	}
	return options
}

func animationTimeTweenOptions() workspaceContentProperty {
	options := workspaceContentProperty{Options: make([]string, 0, len(animationTweens.Options)-1), Values: make([]string, 0, len(animationTweens.Values)-1)}
	for i, value := range animationTweens.Values {
		if scene.AnimationTween(value) != scene.AnimationCubicSpline {
			options.Options = append(options.Options, animationTweens.Options[i])
			options.Values = append(options.Values, value)
		}
	}
	return options
}

func (a *ModuleEditor) renderAnimationCurve(b *editorVNodeView, tween scene.AnimationTween, curve scene.AnimationCurve, spline [3]float32) {
	const samples = 128
	var points [samples + 1][2]float32
	low, high := float32(-.25), float32(1.25)
	count := len(points)
	if tween == scene.AnimationStep {
		points[0], points[1], points[2] = [2]float32{0, 0}, [2]float32{1, 0}, [2]float32{1, 1}
		count = 3
	} else {
		for i := range points {
			x := float32(i) / samples
			y := scene.SampleAnimationProgress(tween, curve, x)
			if a.animation.Kind == "texture" {
				y = clamp(y, 0, 1)
			}
			if tween == scene.AnimationCubicSpline {
				x2, x3 := x*x, x*x*x
				y = (x3-2*x2+x)*spline[0] + (-2*x3+3*x2)*spline[2] + (x3-x2)*spline[1]
			}
			points[i] = [2]float32{x, y}
			low, high = min(low, y-.05), max(high, y+.05)
		}
	}
	if tween == scene.AnimationBezier {
		low, high = min(low, curve.Y1-.05, curve.Y2-.05), max(high, curve.Y1+.05, curve.Y2+.05)
	}
	a.animation.CurveMinY, a.animation.CurveMaxY = low, high
	width, height := max(a.dialogPanel(480, 460).W-44, 1), float32(178)
	position := func(point [2]float32) [2]float32 {
		return [2]float32{point[0] * width, (high - point[1]) / (high - low) * height}
	}
	b.open("div", "curve", "id", "animation-curve", "class", "animation-curve", "role", "group", "aria-label", "Interpolation curve: "+string(tween), "style", fmt.Sprintf("width:%gpx;height:%gpx", width+2, height+2))
	for i := 0; i <= 4; i++ {
		y := (high - float32(i)/4) / (high - low) * 100
		b.text("span", "grid:y:"+strconv.Itoa(i), "", "class", "animation-curve-grid", "style", fmt.Sprintf("left:0;top:%g%%;width:100%%;height:1px", y))
		b.text("span", "grid:x:"+strconv.Itoa(i), "", "class", "animation-curve-grid", "style", fmt.Sprintf("left:%g%%;top:0;width:1px;height:100%%", float32(i)*25))
	}
	for _, value := range [...]float32{0, 1} {
		b.text("span", "axis:y:"+animationNumber(value), fmt.Sprintf("%g%%", value*100), "class", "animation-curve-axis-y", "style", fmt.Sprintf("top:%g%%", (high-value)/(high-low)*100))
	}
	for i := 1; i < count; i++ {
		renderAnimationCurveLine(b, strconv.Itoa(i), "animation-curve-line", position(points[i-1]), position(points[i]))
	}
	if tween == scene.AnimationBezier {
		renderAnimationCurveLine(b, "guide:1", "animation-curve-guide", position([2]float32{0, 0}), position([2]float32{curve.X1, curve.Y1}))
		renderAnimationCurveLine(b, "guide:2", "animation-curve-guide", position([2]float32{1, 1}), position([2]float32{curve.X2, curve.Y2}))
		b.button("animation:handle:1", "", "class", "animation-curve-handle", "aria-label", "Bezier handle 1; edit its X and Y values below", "style", fmt.Sprintf("left:%g%%;bottom:%g%%", curve.X1*100, (curve.Y1-low)/(high-low)*100))
		b.button("animation:handle:2", "", "class", "animation-curve-handle", "aria-label", "Bezier handle 2; edit its X and Y values below", "style", fmt.Sprintf("left:%g%%;bottom:%g%%", curve.X2*100, (curve.Y2-low)/(high-low)*100))
	}
	b.close()
	b.open("div", "axis:x", "class", "animation-curve-axis-x")
	b.text("span", "start", "0%")
	b.text("span", "time", "Elapsed time")
	b.text("span", "end", "100%")
	b.close()
	axis := "Value progress"
	if a.animation.Kind == "texture" {
		axis = "Sequence progress"
	}
	b.text("p", "axes", "Y: "+axis+" · X: Elapsed time", "class", "animation-curve-help")
}

func renderAnimationCurveLine(b *editorVNodeView, key, class string, start, end [2]float32) {
	dx, dy := float64(end[0]-start[0]), float64(end[1]-start[1])
	b.text("span", key, "", "class", class, "style", fmt.Sprintf("left:%gpx;top:%gpx;width:%gpx;transform:rotate(%gdeg)", start[0], start[1]-1, math.Hypot(dx, dy)+.5, math.Atan2(dy, dx)*180/math.Pi))
}

func (a *ModuleEditor) closeAnimationCurve() {
	if err := a.flushAnimationDrafts(); err != nil {
		a.report(err)
		return
	}
	a.animation.CurveOpen, a.animation.DragTime, a.animation.DragHandle = false, false, 0
	a.animation.CurveFocus = 0
	a.dialogError = ""
	if a.world != nil {
		engineui.FocusControl(a.world, 0)
	}
	a.changed()
}

func (a *ModuleEditor) saveAnimationCurve() {
	if err := a.applyDrafts(); err != nil {
		a.report(err)
		return
	}
	if a.Document != nil {
		if err := a.Document.Save(); err != nil {
			a.report(err)
			return
		}
	}
	a.status = "Animation curve saved"
	a.closeAnimationCurve()
}

func (a *ModuleEditor) updateAnimationCurveFocus(in *input.State) {
	if a.world == nil {
		return
	}
	s := &a.animation
	focus, _ := ecs.GetResource[*scene.InputFocus](a.world)
	tab := in.KeyPressed(input.KeyTab) && !in.KeyHeld(input.KeyLeftControl) && !in.KeyHeld(input.KeyRightControl) && !in.KeyHeld(input.KeyLeftAlt) && !in.KeyHeld(input.KeyRightAlt)
	if !tab && focus != nil {
		if target := ecs.GetRef[htmlui.Focusable](a.world, focus.Keyboard); target != nil && target.Owner == a.shell {
			switch target.ID {
			case "animation:close-curve", "animation:save-curve", "animation:tween", "animation:custom-curve", "animation:x1", "animation:y1", "animation:x2", "animation:y2", "animation:inTangent", "animation:outTangent", "animation:handle:1", "animation:handle:2":
				s.CurveFocus = focus.Keyboard
				return
			}
		}
	}
	var controls [10]ecs.Entity
	count, current := 0, -1
	for _, id := range [...]string{"animation:close-curve", "animation:tween", "animation:custom-curve", "animation:x1", "animation:y1", "animation:x2", "animation:y2", "animation:outTangent", "animation:inTangent", "animation:save-curve"} {
		if control, ok := htmlui.ControlByID(a.world, a.shell, id); ok {
			if control == s.CurveFocus {
				current = count
			}
			controls[count] = control
			count++
		}
	}
	if count == 0 {
		return
	}
	next := 0
	if tab {
		next = (current + 1) % count
		if in.KeyHeld(input.KeyLeftShift) || in.KeyHeld(input.KeyRightShift) {
			next = (max(current, 0) + count - 1) % count
		}
	}
	s.CurveFocus = controls[next]
	engineui.FocusControl(a.world, s.CurveFocus)
}
