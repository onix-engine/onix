package editor

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/editor/resources/images/icons"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	engineui "github.com/diiyw/onix/ui"
	htmlui "github.com/diiyw/onix/ui/html"
)

// Authoring state stays outside runtime clips; idle playback reuses the bound animator.
type workspaceAnimation struct {
	Open, Active, Playing   bool
	Mod                     project.ModID
	Element, Instance, Kind string
	Track, Key              int
	Segment                 bool
	CurveOpen               bool
	CurveFocus              ecs.Entity
	LastSegment             string
	LastSegmentClick        float64
	Time                    float32
	Target                  string
	PreviewEntity           ecs.Entity
	PreviewWorld            *ecs.World
	DragHandle              int
	CurveBounds             rect
	CurveMinY, CurveMaxY    float32
	UITime                  float32
	DragTime                bool
	Playheads               []htmlui.Node
	TrackCounts             [3]int
	ElementIndex            int
	Space                   project.CoordinateSpace
	Detail                  workspaceAnimationDetail
	Zoom                    float32
	RangeStart, RangeEnd    float32
	ScrollPosition          [2]float32
	ScrollPending           bool
}

type workspaceAnimationDetail struct {
	Target                                  DocumentID
	Pointer, ValuePointer, AnimationPointer string
	Value                                   string
	Texture                                 project.TextureSpec
	Animation                               *project.AnimationComponentSpec
	Axis                                    int
	Time                                    float32
}

func selectedAnimationKey[T any](s *workspaceAnimation, tracks []project.AnimationTrackSpec[T]) (scene.AnimationBinding, project.AnimationKeyframeSpec[T], bool) {
	if s.Track < 0 || s.Track >= len(tracks) || s.Key < 0 || s.Key >= len(tracks[s.Track].Keyframes) {
		return scene.AnimationBinding{}, project.AnimationKeyframeSpec[T]{}, false
	}
	return tracks[s.Track].Binding, tracks[s.Track].Keyframes[s.Key], true
}

// Details edits the selected key's value through the target component's controls.
// The authored component remains untouched, including its unanimated vector axes.
func (a *ModuleEditor) animationInspectorIndex(mod project.ModSpec, index int) int {
	s := &a.animation
	s.Detail = workspaceAnimationDetail{}
	if !s.Active || !s.Open || s.Mod != a.selectedMod() || s.Element != a.selectedElement {
		return index
	}
	animation := mod.Elements[index].Components.Animation
	if animation == nil {
		return index
	}
	s.ElementIndex, s.Space = index, mod.Space
	clip := &animation.Clip
	normalizeAnimationSelection(s, clip)
	var binding scene.AnimationBinding
	var valid bool
	detail := workspaceAnimationDetail{Target: modDocumentID(s.Mod), Axis: -1}
	switch s.Kind {
	case "float":
		var key project.AnimationKeyframeSpec[float32]
		binding, key, valid = selectedAnimationKey(s, clip.FloatTracks)
		detail.Value = animationNumber(key.Value)
		detail.Time = key.Time
	case "texture":
		var key project.AnimationKeyframeSpec[project.TextureSpec]
		binding, key, valid = selectedAnimationKey(s, clip.TextureTracks)
		detail.Texture = key.Value
		detail.Time = key.Time
	case "bool":
		var key project.AnimationKeyframeSpec[bool]
		binding, key, valid = selectedAnimationKey(s, clip.BoolTracks)
		detail.Value = strconv.FormatBool(key.Value)
		detail.Time = key.Time
	}
	if !valid {
		return index
	}
	detail.Animation, detail.AnimationPointer = animation, "/elements/"+strconv.Itoa(index)+"/components/animation"
	detail.ValuePointer = detail.AnimationPointer + fmt.Sprintf("/clip/%sTracks/%d/keyframes/%d/value", s.Kind, s.Track, s.Key)
	if binding.Path != "" && binding.Path != s.Element {
		index = slices.IndexFunc(mod.Elements, func(element project.ModElementSpec) bool { return element.ID == binding.Path })
		if index < 0 {
			return s.ElementIndex
		}
	}
	base := "/elements/" + strconv.Itoa(index)
	target := string(binding.Target)
	switch binding.Target {
	case scene.AnimationTargetSpriteTexture:
		detail.Pointer = base + "/components/sprite/texture"
	case scene.AnimationTargetSpriteAlpha:
		detail.Pointer = base + "/components/sprite/alpha"
	case scene.AnimationTargetVisibility:
		detail.Pointer = base + "/visible"
	default:
		if !strings.HasPrefix(target, "transform.") {
			return index
		}
		detail.Axis = strings.IndexByte("xyz", target[len(target)-1])
		detail.Pointer = base + "/" + strings.ReplaceAll(target[:len(target)-2], ".", "/")
	}
	s.Detail = detail
	return index
}

func (a *ModuleEditor) animationDetailField(field workspaceField) workspaceField {
	detail := &a.animation.Detail
	if detail.ValuePointer == "" || field.Target != detail.Target {
		return field
	}
	if field.Pointer == detail.Pointer && field.Axis == detail.Axis {
		field.Pointer, field.Value, field.Axis = detail.ValuePointer, detail.Value, -1
		field.Animation = true
	} else if suffix, ok := strings.CutPrefix(field.Pointer, detail.ValuePointer); ok && strings.HasPrefix(suffix, "/") {
		field.Animation = true
	}
	return field
}

var animationTargets = [...]struct {
	Value, Label string
	World        bool
}{
	{"transform.position.x", "Position X", false}, {"transform.position.y", "Position Y", false}, {"transform.position.z", "Position Z", true},
	{"transform.scale.x", "Scale X", false}, {"transform.scale.y", "Scale Y", false}, {"transform.scale.z", "Scale Z", true},
	{"transform.rotation.x", "Rotation X", true}, {"transform.rotation.y", "Rotation Y", true}, {"transform.rotation.z", "Rotation Z", false},
	{"sprite.alpha", "Opacity", false}, {"sprite.texture", "Sprite frames", false}, {"visibility.visible", "Visibility", false},
}
var animationTweens = animationTweenOptions()
var animationTimeTweens = animationTimeTweenOptions()

func (a *ModuleEditor) animationElement(mod project.ModSpec) (*project.ModElementSpec, int) {
	for i := range mod.Elements {
		if mod.Elements[i].ID == a.animation.Element {
			return &mod.Elements[i], i
		}
	}
	return nil, -1
}
func (a *ModuleEditor) animationClip() (*project.AnimationClipSpec, bool) {
	s := &a.animation
	if !s.Open || a.Document == nil || a.Document.Mods[s.Mod] == nil || a.selectedMod() != s.Mod || a.selectedElement != s.Element {
		return nil, false
	}
	mod := a.viewMod(s.Mod)
	element, index := a.animationElement(mod)
	if element == nil || element.Components.Animation == nil {
		return nil, false
	}
	s.ElementIndex, s.Space = index, mod.Space
	return &element.Components.Animation.Clip, true
}

func (a *ModuleEditor) updateResourceTabKeys(in *input.State) {
	if !in.KeyPressed(input.KeyLeft) && !in.KeyPressed(input.KeyRight) && !in.KeyPressed(input.KeyHome) && !in.KeyPressed(input.KeyEnd) {
		return
	}
	focus, _ := ecs.GetResource[*scene.InputFocus](a.world)
	if focus == nil {
		return
	}
	target := ecs.GetRef[htmlui.Focusable](a.world, focus.Keyboard)
	if target == nil || target.Owner != a.shell || target.ID != "resources-tab" && target.ID != "animation-tab" {
		return
	}
	if _, ok := a.animationClip(); !ok {
		return
	}
	active := !a.animation.Active
	if in.KeyPressed(input.KeyHome) {
		active = false
	}
	if in.KeyPressed(input.KeyEnd) {
		active = true
	}
	if err := a.stageAnimationInspectorDrafts(); err != nil {
		a.report(err)
		return
	}
	a.animation.Active = active
	a.changed()
	id := "resources-tab"
	if active {
		id = "animation-tab"
	}
	for entity, access := range ecs.Query[htmlui.Focusable](a.world).All {
		value := access.Ref()
		if value.Owner == a.shell && value.ID == id {
			engineui.FocusControl(a.world, entity)
			break
		}
	}
}
func (a *ModuleEditor) openAnimation() {
	mod := a.selectedMod()
	if a.Document == nil || a.Document.Mods[mod] == nil {
		return
	}
	spec := a.viewMod(mod)
	index, ok := a.selectedElementIndex(spec.Elements)
	if !ok || spec.Elements[index].Components.Animation == nil {
		return
	}
	if err := a.stageAnimationInspectorDrafts(); err != nil {
		a.report(err)
		return
	}
	spec = a.viewMod(mod)
	a.releaseAnimationPreview()
	a.animation = workspaceAnimation{Open: true, Active: true, Mod: mod, Element: spec.Elements[index].ID, Instance: a.Selected, Kind: "float", Target: "transform.position.x", Zoom: 1, CurveMinY: -.25, CurveMaxY: 1.25}
	clip := spec.Elements[index].Components.Animation.Clip
	if len(clip.FloatTracks) == 0 {
		a.animation.Kind = "texture"
		if len(clip.TextureTracks) == 0 {
			a.animation.Kind = "bool"
		}
	}
	a.lastRow = workspaceModuleKey{}
	a.changed()
}
func (a *ModuleEditor) releaseAnimationPreview() {
	s := &a.animation
	if s.PreviewWorld != nil {
		if animator := ecs.GetMut[scene.Animator](s.PreviewWorld, s.PreviewEntity); animator != nil {
			animator.Play()
		}
	}
	s.PreviewWorld, s.PreviewEntity = nil, 0
}

// Preview controls are applied before both world and UI animation systems.
func (a *ModuleEditor) syncAnimationPreview(delta float32) {
	s, v := &a.animation, &a.viewport2D
	if !s.Active || !s.Open || s.Instance != a.Selected || s.Element != a.selectedElement || s.Mod != a.selectedMod() {
		a.releaseAnimationPreview()
		return
	}
	var entity ecs.Entity
	for i := range v.rows {
		row := &v.rows[i]
		if row.id == s.Instance || a.Local != "" && row.mod == s.Mod {
			entity = row.targets[s.Element]
			break
		}
	}
	animator := ecs.GetMut[scene.Animator](v.world, entity)
	if animator == nil || animator.Clip == nil {
		a.releaseAnimationPreview()
		return
	}
	if s.PreviewEntity != entity || s.PreviewWorld != v.world {
		a.releaseAnimationPreview()
		s.PreviewEntity, s.PreviewWorld = entity, v.world
	}
	if s.Playing {
		s.Time += delta * animator.Speed
		if s.Time >= animator.Clip.Duration {
			if animator.Loop {
				s.Time = float32(math.Mod(float64(s.Time), float64(animator.Clip.Duration)))
			} else {
				s.Time = animator.Clip.Duration
				s.Playing = false
			}
		}
	}
	s.Time = min(max(s.Time, 0), animator.Clip.Duration)
	animator.Pause()
	animator.Seek(s.Time)
}

func (a *ModuleEditor) bindAnimation(root htmlui.Node) {
	s := &a.animation
	clear(s.Playheads)
	s.Playheads = s.Playheads[:0]
	s.UITime = -1
	if !s.Active {
		return
	}
	if node, ok := root.FindByID("animation:playhead:ruler"); ok {
		s.Playheads = append(s.Playheads, node)
	}
	for kind, count := range s.TrackCounts {
		name := [...]string{"float", "texture", "bool"}[kind]
		for i := 0; i < count; i++ {
			if node, ok := root.FindByID("animation:playhead:" + name + ":" + strconv.Itoa(i)); ok {
				s.Playheads = append(s.Playheads, node)
			}
		}
	}
}

// Refresh only retained playhead nodes, without rebuilding the component tree.
func (a *ModuleEditor) updateAnimationPlaybackUI() {
	s := &a.animation
	if !s.Active || s.PreviewWorld == nil {
		return
	}
	if s.Time == s.UITime {
		return
	}
	animator := ecs.GetRef[scene.Animator](s.PreviewWorld, s.PreviewEntity)
	if animator == nil || animator.Clip == nil {
		return
	}
	s.UITime = s.Time
	start, end := s.timelineRange(animator.Clip.Duration)
	style := animationPlayheadStyle(s.Time, start, end)
	for _, node := range s.Playheads {
		node.SetAttribute("style", style)
	}
}

func (a *ModuleEditor) animationTimelineScroll(world *ecs.World) *scene.UIScroll {
	control, ok := htmlui.ControlByID(world, a.shell, "animation:seek-start")
	if !ok {
		return nil
	}
	content, ok := ecs.Get[htmlui.ScrollContent](world, control)
	if !ok {
		return nil
	}
	return ecs.GetMut[scene.UIScroll](world, content.Area)
}

func (a *ModuleEditor) captureAnimationScroll(world *ecs.World) {
	s := &a.animation
	if !s.Active || s.ScrollPending {
		return
	}
	if scroll := a.animationTimelineScroll(world); scroll != nil {
		s.ScrollPosition = scroll.Position
	}
	s.ScrollPending = true
}

func (a *ModuleEditor) restoreAnimationScroll(context *ecs.SystemContext) error {
	s := &a.animation
	if !s.Active || !s.ScrollPending {
		return nil
	}
	s.ScrollPending = false
	if scroll := a.animationTimelineScroll(context.World); scroll != nil {
		for axis := range 2 {
			scroll.Position[axis] = clamp(s.ScrollPosition[axis], -max(0, scroll.ContentSize[axis]-scroll.ViewportSize[axis]), 0)
		}
	}
	return nil
}

func (a *ModuleEditor) animationTimelineBounds() (rect, bool) {
	node, ok := htmlui.NodeByID(a.world, a.shell, "animation-timeline")
	if !ok {
		return rect{}, false
	}
	box, ok := node.Bounds()
	return rect{X: box.X, Y: box.Y, W: box.Width, H: box.Height}, ok
}

func (a *ModuleEditor) animationRulerBounds() (rect, bool) {
	node, ok := htmlui.NodeByID(a.world, a.shell, "animation-ruler")
	if !ok {
		return rect{}, false
	}
	box, ok := node.Bounds()
	bounds := rect{X: box.X, Y: box.Y, W: box.Width, H: box.Height}
	if scroll := a.animationTimelineScroll(a.world); scroll != nil {
		bounds.X += scroll.Position[0]
		bounds.Y += scroll.Position[1]
	}
	return bounds, ok
}

func (a *ModuleEditor) updateAnimationWheel(in *input.State) bool {
	s, pointer := &a.animation, in.Pointer()
	if !s.Active || a.modalVisible() || a.workspaceMenuOpen() || pointer.WheelY == 0 {
		return false
	}
	bounds, ok := a.animationTimelineBounds()
	if !ok || pointer.X < bounds.X || pointer.X >= bounds.X+bounds.W || pointer.Y < bounds.Y || pointer.Y >= bounds.Y+bounds.H {
		return false
	}
	ruler, ok := a.animationRulerBounds()
	if !ok {
		return false
	}
	zoom := clamp(s.Zoom*float32(math.Pow(1.12, float64(pointer.WheelY))), 1, 64)
	if zoom != s.Zoom {
		// Keep the time under the pointer fixed while the lane changes width.
		fraction := clamp((pointer.X-ruler.X)/ruler.W, 0, 1)
		width := max(180, (bounds.W-148)*zoom)
		if scroll := a.animationTimelineScroll(a.world); scroll != nil {
			s.ScrollPosition = scroll.Position
			scroll.Momentum = [2]float32{}
		}
		s.ScrollPosition[0] += ruler.W*fraction - width*fraction
		s.ScrollPending, s.Zoom = true, zoom
		a.changed()
	}
	in.ConsumePointerWheel()
	return true
}

func (a *ModuleEditor) updateAnimationPointer(pointer input.Pointer) bool {
	s := &a.animation
	if !s.Active || a.modalVisible() && !s.CurveOpen || a.workspaceMenuOpen() {
		s.DragHandle = 0
		s.DragTime = false
		return false
	}
	if pointer.Pressed {
		for handle := 1; handle <= 2; handle++ {
			node, ok := htmlui.NodeByID(a.world, a.shell, "animation:handle:"+strconv.Itoa(handle))
			if !ok {
				continue
			}
			box, ok := node.Bounds()
			// Handles translate(-50%, 50%) so their centers sit on the curve points.
			if !ok || !viewportContains(rect{X: box.X - box.Width/2, Y: box.Y + box.Height/2, W: box.Width, H: box.Height}, pointer.X, pointer.Y) {
				continue
			}
			graph, ok := htmlui.NodeByID(a.world, a.shell, "animation-curve")
			if !ok {
				continue
			}
			bounds, ok := graph.Bounds()
			if !ok {
				continue
			}
			s.DragHandle, s.CurveBounds = handle, rect{X: bounds.X + 1, Y: bounds.Y + 1, W: bounds.Width - 2, H: bounds.Height - 2}
			break
		}
		if s.DragHandle == 0 && !s.CurveOpen {
			if timeline, ok := a.animationTimelineBounds(); ok && viewportContains(timeline, pointer.X, pointer.Y) {
				if ruler, ok := a.animationRulerBounds(); ok && viewportContains(ruler, pointer.X, pointer.Y) {
					s.DragTime, s.CurveBounds = true, ruler
				}
			}
		}
	}
	if s.DragTime {
		if clip, ok := a.animationClip(); ok {
			start, end := s.timelineRange(clip.Duration)
			s.Time = start + clamp((pointer.X-s.CurveBounds.X)/max(s.CurveBounds.W, 1), 0, 1)*(end-start)
			s.Playing = false
			a.changed()
		}
		if pointer.Released || !pointer.Down {
			s.DragTime = false
		}
		return true
	}
	if s.DragHandle == 0 {
		return false
	}
	if pointer.Down && (pointer.DeltaX != 0 || pointer.DeltaY != 0) {
		bounds, handle := s.CurveBounds, s.DragHandle
		x := clamp((pointer.X-bounds.X)/max(bounds.W, 1), 0, 1)
		low, high := s.CurveMinY, s.CurveMaxY
		y := high - (high-low)*(pointer.Y-bounds.Y)/max(bounds.H, 1)
		err := a.editAnimation(func(clip *project.AnimationClipSpec, _ *project.ModElementSpec) error {
			_, curve, err := editableAnimationCurve(s, clip)
			if err != nil {
				return err
			}
			if handle == 1 {
				curve.X1, curve.Y1 = x, y
			} else {
				curve.X2, curve.Y2 = x, y
			}
			return nil
		})
		a.report(err)
	}
	if pointer.Released || !pointer.Down {
		s.DragHandle = 0
	}
	return true
}

func animationNumber(value float32) string { return strconv.FormatFloat(float64(value), 'f', -1, 32) }

// A zero end follows the full clip; an invalidated view falls back to that range.
func (s *workspaceAnimation) timelineRange(duration float32) (float32, float32) {
	end := s.RangeEnd
	if end == 0 {
		end = duration
	}
	end = min(end, duration)
	if s.RangeStart >= end {
		return 0, duration
	}
	return s.RangeStart, end
}

func animationPlayheadStyle(time, start, end float32) string {
	if time < start || time > end {
		return "display:none"
	}
	return fmt.Sprintf("left:%g%%", (time-start)/(end-start)*100)
}

func animationTickStep(duration, zoom float32) float32 {
	step := float64(duration) / (5 * float64(zoom))
	unit := math.Pow(10, math.Floor(math.Log10(step)))
	for _, multiple := range [...]float64{1, 2, 5, 10} {
		if unit*multiple >= step {
			return max(math.SmallestNonzeroFloat32, float32(unit*multiple))
		}
	}
	return float32(unit * 10)
}

func (a *ModuleEditor) animationFieldValue(name, kind, value string) string {
	s := &a.animation
	pointer := fmt.Sprintf("/elements/%d/components/animation/clip", s.ElementIndex)
	switch name {
	case "key-time", "x1", "y1", "x2", "y2", "inTangent", "outTangent", "tween":
		if s.Kind == "texture" && name != "key-time" {
			property := "timeTween"
			if name != "tween" {
				property = "timeCurve/" + name
			}
			pointer += fmt.Sprintf("/textureTracks/%d/%s", s.Track, property)
			break
		}
		key, property := s.Key, name
		switch name {
		case "key-time":
			property = "time"
		case "x1", "y1", "x2", "y2":
			property = "curve/" + name
		case "inTangent":
			key++
		}
		pointer += fmt.Sprintf("/%sTracks/%d/keyframes/%d/%s", s.Kind, s.Track, key, property)
	default:
		return value
	}
	id := "animation:" + name
	a.fields[id] = workspaceField{Target: modDocumentID(s.Mod), Pointer: pointer, Kind: kind, Value: value, Axis: -1, Animation: true}
	if draft, ok := a.drafts[id]; ok {
		return draft
	}
	return value
}

func (a *ModuleEditor) animationControl(b *editorVNodeView, name, label, kind, value string) {
	id := "animation:" + name
	value = a.animationFieldValue(name, kind, value)
	a.controlValues[id] = value
	b.open("div", id, "class", "animation-control")
	b.text("label", "label", label, "for", id)
	b.text("input", "input", "", "id", id, "type", kind, "value", value, "step", "any")
	b.close()
}
func (a *ModuleEditor) animationSelect(b *editorVNodeView, name, label, value string, options *workspaceContentProperty) {
	id := "animation:" + name
	value = a.animationFieldValue(name, "string", value)
	a.controlValues[id] = value
	b.open("div", id, "class", "animation-control")
	b.text("label", "label", label, "for", id)
	b.open("select", "select", "id", id)
	for i, option := range options.Options {
		b.open("option", options.Values[i], "value", options.Values[i])
		if value == options.Values[i] {
			b.attr("selected", "")
		}
		b.content(option)
		b.close()
	}
	b.close()
	b.close()
}

func animationIconButton(b *editorVNodeView, id, label string, icon icons.Name, attrs ...string) {
	class := "animation-icon-button"
	caption := id == "animation:add-key" || id == "animation:delete-key" || id == "animation:delete-track"
	if caption {
		class = "animation-action-button"
	}
	iconSource := "onix-rightbar/" + string(icon) + ".svg"
	if strings.HasPrefix(id, "animation:delete") {
		class += " danger"
		iconSource += "#danger"
	}
	b.open("button", id, "id", id, "type", "button", "class", class, "aria-label", label, "title", label)
	for i := 0; i < len(attrs); i += 2 {
		b.attr(attrs[i], attrs[i+1])
	}
	b.text("img", "icon", "", "src", iconSource, "alt", "")
	if caption {
		b.text("span", "label", label)
	}
	b.close()
}

func (a *ModuleEditor) renderAnimation(b *editorVNodeView) bool {
	clip, ok := a.animationClip()
	if !ok {
		return false
	}
	s := &a.animation
	normalizeAnimationSelection(s, clip)
	s.TrackCounts = [3]int{len(clip.FloatTracks), len(clip.TextureTracks), len(clip.BoolTracks)}
	a.controlValues["animation:target"] = s.Target
	b.open("div", "animation-editor", "class", "animation-editor")
	b.open("div", "toolbar", "class", "animation-toolbar")
	b.text("strong", "name", s.Element)
	playLabel := "Play"
	playIcon := icons.Play
	if s.Playing {
		playLabel = "Pause"
		playIcon = icons.Pause
	}
	animationIconButton(b, "animation:play", playLabel, playIcon, "aria-pressed", strconv.FormatBool(s.Playing))
	b.open("select", "target", "id", "animation:target", "aria-label", "New animation track")
	world := s.Space == project.CoordinateSpaceWorld
	for _, target := range animationTargets {
		if target.World && !world {
			continue
		}
		b.open("option", target.Value, "value", target.Value)
		if s.Target == target.Value {
			b.attr("selected", "")
		}
		b.content(target.Label)
		b.close()
	}
	b.close()
	animationIconButton(b, "animation:add-track", "Add track", icons.Plus)
	animationIconButton(b, "animation:add-key", "Insert key", icons.Plus)
	animationIconButton(b, "animation:delete-key", "Delete key", icons.Trash2)
	animationIconButton(b, "animation:delete-track", "Delete track", icons.Trash2)
	start, end := s.timelineRange(clip.Duration)
	span := end - start
	b.open("div", "range", "class", "animation-range", "role", "group", "aria-label", "Timeline display range")
	b.text("label", "label", "View (s)", "for", "animation:range-start")
	for i, value := range [...]float32{start, end} {
		id := [...]string{"animation:range-start", "animation:range-end"}[i]
		label := [...]string{"View start (seconds)", "View end (seconds)"}[i]
		if i == 1 {
			b.text("span", "separator", "–", "aria-hidden", "true")
		}
		text := animationNumber(value)
		a.controlValues[id] = text
		b.text("input", id, "", "id", id, "type", "number", "value", text, "step", "any", "aria-label", label, "title", label)
	}
	b.button("animation:range-reset", "Show all", "type", "button", "title", "Show the full animation timeline")
	b.close()
	b.close()
	b.open("div", "body", "class", "animation-body")
	b.open("div", "timeline", "id", "animation-timeline", "class", "animation-timeline", "title", "Scroll to zoom the timeline")
	b.open("div", "timeline-content", "class", "animation-timeline-content", "style", fmt.Sprintf("width:calc(148px + (100%% - 148px) * %g)", s.Zoom))
	b.open("div", "ruler-row", "class", "animation-track-row")
	b.text("span", "label", "Tracks / seconds", "class", "animation-track-label")
	b.open("div", "ruler", "id", "animation-ruler", "class", "animation-lane animation-ruler")
	b.button("animation:seek-start", animationNumber(start), "style", "position:absolute;left:0;top:0;transform:none")
	step := animationTickStep(span, s.Zoom)
	lastTick := start
	for i := 1; float32(i)*step < span; i++ {
		time := start + float32(i)*step
		if time >= end {
			break
		}
		if time <= lastTick {
			continue
		}
		lastTick = time
		style := fmt.Sprintf("position:absolute;left:%g%%;top:0", (time-start)/span*100)
		b.button("animation:seek:"+animationNumber(time), animationNumber(time), "style", style)
	}
	b.button("animation:seek:"+animationNumber(end), animationNumber(end), "style", "position:absolute;right:0;top:0;transform:none")
	b.text("div", "cursor", "", "id", "animation:playhead:ruler", "class", "animation-playhead", "style", animationPlayheadStyle(s.Time, start, end))
	b.close()
	b.close()
	renderAnimationTracks(b, s, "float", clip.FloatTracks, start, end)
	renderAnimationTracks(b, s, "texture", clip.TextureTracks, start, end)
	renderAnimationTracks(b, s, "bool", clip.BoolTracks, start, end)
	b.close()
	b.close()
	b.close()
	b.close()
	return true
}

func normalizeAnimationSelection(s *workspaceAnimation, clip *project.AnimationClipSpec) {
	count := 0
	switch s.Kind {
	case "float":
		count = len(clip.FloatTracks)
	case "texture":
		count = len(clip.TextureTracks)
	case "bool":
		count = len(clip.BoolTracks)
	}
	if count == 0 {
		s.Track, s.Key, s.Segment = 0, 0, false
		switch {
		case len(clip.FloatTracks) > 0:
			s.Kind = "float"
			count = len(clip.FloatTracks)
		case len(clip.TextureTracks) > 0:
			s.Kind = "texture"
			count = len(clip.TextureTracks)
		case len(clip.BoolTracks) > 0:
			s.Kind = "bool"
			count = len(clip.BoolTracks)
		}
	}
	s.Track = max(0, min(s.Track, count-1))
	keys := 0
	switch s.Kind {
	case "float":
		if count > 0 {
			keys = len(clip.FloatTracks[s.Track].Keyframes)
		}
	case "texture":
		if count > 0 {
			keys = len(clip.TextureTracks[s.Track].Keyframes)
		}
	case "bool":
		if count > 0 {
			keys = len(clip.BoolTracks[s.Track].Keyframes)
		}
	}
	s.Key = max(0, min(s.Key, keys-1))
	s.Segment = s.Segment && s.Key+1 < keys
	s.CurveOpen = s.CurveOpen && s.Segment
	s.Time = min(max(s.Time, 0), clip.Duration)
}
func renderAnimationTracks[T any](b *editorVNodeView, s *workspaceAnimation, kind string, tracks []project.AnimationTrackSpec[T], start, end float32) {
	span := end - start
	title := "Double-click to edit transition"
	if kind == "texture" {
		title = "Double-click to edit sequence playback speed"
	}
	for i, track := range tracks {
		prefix := kind + ":" + strconv.Itoa(i)
		b.open("div", prefix, "class", "animation-track-row")
		name := animationTrackLabel(track.Binding)
		b.button("animation:track:"+prefix, name, "class", "animation-track-label", "aria-pressed", strconv.FormatBool(s.Kind == kind && s.Track == i))
		b.open("div", "lane", "class", "animation-lane")
		for j, key := range track.Keyframes {
			if key.Time > end {
				break
			}
			if key.Time < start && (j+1 == len(track.Keyframes) || track.Keyframes[j+1].Time <= start) {
				continue
			}
			id := prefix + ":" + strconv.Itoa(j)
			if j+1 < len(track.Keyframes) && key.Time < end && track.Keyframes[j+1].Time > start {
				next := track.Keyframes[j+1].Time
				left, right := max(key.Time, start), min(next, end)
				b.open("button", "animation:segment:"+id, "id", "animation:segment:"+id, "type", "button", "class", "animation-segment", "title", title, "aria-haspopup", "dialog", "aria-label", fmt.Sprintf("%s interval %g to %g seconds", name, key.Time, next), "aria-pressed", strconv.FormatBool(s.Kind == kind && s.Track == i && s.Key == j && s.Segment), "style", fmt.Sprintf("left:%g%%;width:%g%%", (left-start)/span*100, (right-left)/span*100))
				b.text("span", "line", "", "class", "animation-segment-line")
				b.close()
			}
			if key.Time >= start && key.Time <= end {
				b.button("animation:key:"+id, "◆", "class", "animation-key", "aria-label", fmt.Sprintf("%s keyframe at %g seconds", name, key.Time), "aria-pressed", strconv.FormatBool(s.Kind == kind && s.Track == i && s.Key == j && !s.Segment), "style", fmt.Sprintf("left:%g%%", (key.Time-start)/span*100))
			}
		}
		b.text("div", "cursor", "", "id", "animation:playhead:"+prefix, "class", "animation-playhead", "style", animationPlayheadStyle(s.Time, start, end))
		b.close()
		b.close()
	}
}
func (a *ModuleEditor) renderAnimationInterpolation(b *editorVNodeView, keys []project.AnimationKeyframeSpec[float32]) {
	s := &a.animation
	key := keys[s.Key]
	b.text("p", "interval", fmt.Sprintf("Interval: %g → %g s", key.Time, keys[s.Key+1].Time))
	tween := project.EffectiveAnimationTween(key, scene.AnimationLinear)
	a.animationSelect(b, "tween", "Interpolation", string(tween), &animationTweens)
	var spline [3]float32
	if tween == scene.AnimationCubicSpline {
		next := keys[s.Key+1]
		span := next.Time - key.Time
		difference := next.Value - key.Value
		scale := difference
		if scale == 0 {
			scale = 1
		}
		spline = [3]float32{key.OutTangent * span / scale, next.InTangent * span / scale, difference / scale}
	}
	a.renderAnimationCurve(b, tween, key.Curve, spline)
	if tween == scene.AnimationBezier {
		a.renderAnimationBezierControls(b, key.Curve)
	}
	if tween == scene.AnimationCubicSpline {
		a.animationControl(b, "outTangent", "Start tangent", "number", animationNumber(key.OutTangent))
		a.animationControl(b, "inTangent", "End tangent", "number", animationNumber(keys[s.Key+1].InTangent))
	}
}

// Merge outstanding inspector fields before structural edits, so indices never
// leave drafts attached to the wrong keyframe after an insertion or removal.
func (a *ModuleEditor) editAnimation(edit func(*project.AnimationClipSpec, *project.ModElementSpec) error) error {
	s := &a.animation
	patches, err := a.formPatches()
	if err != nil {
		return err
	}
	target := modDocumentID(s.Mod)
	data, err := applyDocumentPatches(a.Document.states[target].data, patches[target])
	if err != nil {
		return err
	}
	mod, err := decodeDocument[project.ModSpec](data)
	if err != nil {
		return err
	}
	element, index := a.animationElement(mod)
	if element == nil || element.Components.Animation == nil {
		return fmt.Errorf("animation element no longer exists")
	}
	clip := element.Components.Animation.Clip
	if err = a.finishAnimationDrafts(&clip); err != nil {
		return err
	}
	if err = edit(&clip, element); err != nil {
		return err
	}
	if err = clip.Validate(); err != nil {
		return err
	}
	encoded, err := json.Marshal(clip)
	if err != nil {
		return err
	}
	patches[target] = append(patches[target], DocumentPatch{Pointer: "/elements/" + strconv.Itoa(index) + "/components/animation/clip", Value: encoded})
	return a.stagePatches(patches)
}

func (a *ModuleEditor) finishAnimationDrafts(clip *project.AnimationClipSpec) error {
	s := &a.animation
	if text, ok := a.drafts["animation:key-time"]; ok {
		value, err := strconv.ParseFloat(text, 32)
		if err != nil {
			return err
		}
		switch s.Kind {
		case "float":
			err = mutateAnimationKeys(s, clip.FloatTracks, "key-time", float32(value))
		case "texture":
			err = mutateAnimationKeys(s, clip.TextureTracks, "key-time", float32(value))
		case "bool":
			err = mutateAnimationKeys(s, clip.BoolTracks, "key-time", float32(value))
		}
		if err != nil {
			return err
		}
	}
	if a.drafts["animation:tween"] == "bezier" {
		_, curve, err := editableAnimationCurve(s, clip)
		if err != nil {
			return err
		}
		if *curve == (scene.AnimationCurve{}) {
			*curve = scene.EaseInOutAnimationCurve()
		}
	}
	return nil
}

func (a *ModuleEditor) flushAnimationDrafts() error {
	for id, value := range a.drafts {
		if field, ok := a.fields[id]; ok && (field.Animation || strings.HasPrefix(id, "animation:")) && field.modified(value) {
			previous := a.animation
			err := a.editAnimation(func(*project.AnimationClipSpec, *project.ModElementSpec) error { return nil })
			if err != nil {
				a.animation = previous
			}
			return err
		}
	}
	return nil
}

func (a *ModuleEditor) stageAnimationInspectorDrafts() error {
	if len(a.drafts) == 0 {
		return nil
	}
	if _, ok := a.animationClip(); ok {
		previous := a.animation
		err := a.editAnimation(func(*project.AnimationClipSpec, *project.ModElementSpec) error { return nil })
		if err != nil {
			a.animation = previous
		}
		return err
	}
	patches, err := a.formPatches()
	if err != nil {
		return err
	}
	return a.stagePatches(patches)
}
func animationKeys[T any](s *workspaceAnimation, tracks []project.AnimationTrackSpec[T]) ([]project.AnimationKeyframeSpec[T], error) {
	if s.Track < 0 || s.Track >= len(tracks) {
		return nil, fmt.Errorf("select an animation track")
	}
	return tracks[s.Track].Keyframes, nil
}
func mutateAnimationKeys[T any](s *workspaceAnimation, tracks []project.AnimationTrackSpec[T], action string, number float32) error {
	keys, err := animationKeys(s, tracks)
	if err != nil {
		return err
	}
	switch action {
	case "add-key":
		i, exists := slices.BinarySearchFunc(keys, s.Time, func(k project.AnimationKeyframeSpec[T], time float32) int {
			if k.Time < time {
				return -1
			}
			if k.Time > time {
				return 1
			}
			return 0
		})
		if !exists {
			key := keys[max(0, i-1)]
			key.Time = s.Time
			if _, numeric := any(key.Value).(float32); numeric && i > 0 && i < len(keys) {
				var sample [2]scene.AnimationKeyframe[float32]
				for j := range sample {
					source := keys[i-1+j]
					sample[j] = scene.AnimationKeyframe[float32]{Time: source.Time, Value: any(source.Value).(float32), InTangent: any(source.InTangent).(float32), OutTangent: any(source.OutTangent).(float32), Tween: project.EffectiveAnimationTween(source, scene.AnimationLinear), Curve: source.Curve}
				}
				value, _ := scene.SampleAnimationTrack(scene.AnimationTrack[float32]{Keyframes: sample[:]}, s.Time, func(a, b, t float32) float32 { return a + (b-a)*t })
				key.Value = any(value).(T)
			}
			keys = slices.Insert(keys, i, key)
		}
		s.Key = i
		s.Segment = false
	case "delete-key":
		if len(keys) <= 1 {
			return fmt.Errorf("a track needs at least one keyframe")
		}
		if s.Key < 0 || s.Key >= len(keys) {
			return fmt.Errorf("select a keyframe")
		}
		keys = slices.Delete(keys, s.Key, s.Key+1)
		s.Key = min(s.Key, len(keys)-1)
		s.Segment = false
	case "key-time":
		if s.Key < 0 || s.Key >= len(keys) {
			return fmt.Errorf("select a keyframe")
		}
		for i, key := range keys {
			if i != s.Key && key.Time == number {
				return fmt.Errorf("a keyframe already exists at this time")
			}
		}
		keys[s.Key].Time = number
		slices.SortFunc(keys, func(a, b project.AnimationKeyframeSpec[T]) int {
			if a.Time < b.Time {
				return -1
			}
			if a.Time > b.Time {
				return 1
			}
			return 0
		})
		for i, key := range keys {
			if key.Time == number {
				s.Key = i
				break
			}
		}
		s.Time = number
		s.Segment = false
	}
	tracks[s.Track].Keyframes = keys
	return nil
}
func (a *ModuleEditor) animationEvent(event htmlui.Event) bool {
	name, ok := strings.CutPrefix(event.ID, "animation:")
	if !ok {
		return false
	}
	s := &a.animation
	if s.CurveOpen {
		switch name {
		case "save-curve":
			if event.Kind == htmlui.EventClick {
				a.saveAnimationCurve()
			}
			return true
		case "close-curve":
			if event.Kind == htmlui.EventClick {
				a.closeAnimationCurve()
			}
			return true
		case "tween", "custom-curve", "x1", "y1", "x2", "y2", "inTangent", "outTangent", "handle:1", "handle:2":
		default:
			return true
		}
	}
	if name == "custom-curve" && event.Kind == htmlui.EventClick {
		name, event.Kind, event.Value = "tween", htmlui.EventChange, string(scene.AnimationBezier)
	}
	if name == "open" && event.Kind == htmlui.EventClick {
		a.openAnimation()
		return true
	}
	clip, valid := a.animationClip()
	if !valid {
		return true
	}
	if name == "range-start" || name == "range-end" || name == "range-reset" {
		if name == "range-reset" && event.Kind == htmlui.EventClick {
			s.RangeStart, s.RangeEnd = 0, 0
		} else if name != "range-reset" && (event.Kind == htmlui.EventChange || event.Kind == htmlui.EventSubmit) {
			value, err := strconv.ParseFloat(event.Value, 32)
			if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
				a.report(fmt.Errorf("enter a finite timeline time"))
				return true
			}
			start, end := s.timelineRange(clip.Duration)
			if name == "range-start" {
				start = float32(value)
			} else {
				end = float32(value)
			}
			if start < 0 || start >= end || end > clip.Duration {
				a.report(fmt.Errorf("timeline range must satisfy 0 ≤ start < end ≤ %g seconds", clip.Duration))
				return true
			}
			s.RangeStart, s.RangeEnd = start, end
		} else {
			return true
		}
		s.Zoom, s.DragTime = 1, false
		if scroll := a.animationTimelineScroll(a.world); scroll != nil {
			s.ScrollPosition = scroll.Position
			scroll.Momentum = [2]float32{}
		}
		s.ScrollPosition[0], s.ScrollPending = 0, true
		a.changed()
		return true
	}
	if event.Kind == htmlui.EventInput {
		if _, ok := a.fields[event.ID]; ok {
			a.drafts[event.ID] = event.Value
			a.changed()
		}
		return true
	}
	if event.Kind != htmlui.EventChange && event.Kind != htmlui.EventClick {
		return true
	}
	if event.Kind == htmlui.EventClick {
		if !strings.HasPrefix(name, "segment:") {
			s.LastSegment = ""
		}
		switch {
		case strings.HasPrefix(name, "handle:"):
			return true
		case name == "play":
			s.Playing = !s.Playing
			if s.Time >= clip.Duration {
				s.Time = 0
			}
			a.changed()
			return true
		case name == "seek-start":
			s.Time, _ = s.timelineRange(clip.Duration)
			s.Playing = false
			a.changed()
			return true
		case strings.HasPrefix(name, "seek:"):
			value, _ := strconv.ParseFloat(strings.TrimPrefix(name, "seek:"), 32)
			s.Time = float32(value)
			s.Playing = false
			a.changed()
			return true
		case strings.HasPrefix(name, "track:") || strings.HasPrefix(name, "key:") || strings.HasPrefix(name, "segment:"):
			if len(a.drafts) != 0 {
				previous := *s
				if err := a.editAnimation(func(*project.AnimationClipSpec, *project.ModElementSpec) error { return nil }); err != nil {
					*s = previous
					a.report(err)
					return true
				}
				clip, _ = a.animationClip()
			}
			parts := strings.Split(name, ":")
			if len(parts) < 3 {
				return true
			}
			track, err := strconv.Atoi(parts[2])
			if err != nil {
				return true
			}
			key := 0
			if len(parts) == 4 {
				key, err = strconv.Atoi(parts[3])
				if err != nil {
					return true
				}
			}
			s.Kind, s.Track, s.Key, s.Segment = parts[1], track, key, parts[0] == "segment"
			if s.Segment {
				elapsed := a.time - s.LastSegmentClick
				double := s.LastSegment == event.ID && elapsed >= 0 && elapsed < .5
				s.LastSegment, s.LastSegmentClick = event.ID, a.time
				if double {
					s.CurveOpen, s.Playing, s.DragTime, s.DragHandle = true, false, false, 0
					s.CurveFocus = 0
					s.LastSegment = ""
					a.dialogError = ""
					if a.world != nil {
						engineui.FocusControl(a.world, 0)
					}
				}
			}
			switch s.Kind {
			case "float":
				if keys, err := animationKeys(s, clip.FloatTracks); err == nil && key >= 0 && key < len(keys) {
					s.Time = keys[key].Time
				}
			case "texture":
				if keys, err := animationKeys(s, clip.TextureTracks); err == nil && key >= 0 && key < len(keys) {
					s.Time = keys[key].Time
				}
			case "bool":
				if keys, err := animationKeys(s, clip.BoolTracks); err == nil && key >= 0 && key < len(keys) {
					s.Time = keys[key].Time
				}
			}
			a.changed()
			return true
		}
		if name != "add-key" && name != "delete-key" && name != "add-track" && name != "delete-track" {
			return true
		}
	}
	if name == "target" {
		s.Target = event.Value
		return true
	}
	var number float32
	switch name {
	case "key-time", "x1", "y1", "x2", "y2", "inTangent", "outTangent":
		value, err := strconv.ParseFloat(event.Value, 32)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			a.report(fmt.Errorf("enter a finite number"))
			return true
		}
		number = float32(value)
	case "tween", "add-key", "delete-key", "add-track", "delete-track":
	default:
		return true
	}
	previous := *s
	err := a.editAnimation(func(clip *project.AnimationClipSpec, element *project.ModElementSpec) error {
		switch name {
		case "add-track":
			return a.addAnimationTrack(clip, element)
		case "delete-track":
			if len(clip.FloatTracks)+len(clip.TextureTracks)+len(clip.BoolTracks) <= 1 {
				return fmt.Errorf("an animation needs at least one track")
			}
			switch s.Kind {
			case "float":
				if s.Track >= 0 && s.Track < len(clip.FloatTracks) {
					clip.FloatTracks = slices.Delete(clip.FloatTracks, s.Track, s.Track+1)
				}
			case "texture":
				if s.Track >= 0 && s.Track < len(clip.TextureTracks) {
					clip.TextureTracks = slices.Delete(clip.TextureTracks, s.Track, s.Track+1)
				}
			case "bool":
				if s.Track >= 0 && s.Track < len(clip.BoolTracks) {
					clip.BoolTracks = slices.Delete(clip.BoolTracks, s.Track, s.Track+1)
				}
			}
			s.Track, s.Key = 0, 0
			return nil
		case "add-key", "delete-key", "key-time":
			switch s.Kind {
			case "float":
				return mutateAnimationKeys(s, clip.FloatTracks, name, number)
			case "texture":
				return mutateAnimationKeys(s, clip.TextureTracks, name, number)
			case "bool":
				return mutateAnimationKeys(s, clip.BoolTracks, name, number)
			}
		}
		switch name {
		case "tween", "x1", "y1", "x2", "y2":
			tween, curve, err := editableAnimationCurve(s, clip)
			if err != nil {
				return err
			}
			switch name {
			case "tween":
				*tween = scene.AnimationTween(event.Value)
				if *tween == scene.AnimationBezier && *curve == (scene.AnimationCurve{}) {
					*curve = scene.EaseInOutAnimationCurve()
				}
			case "x1":
				curve.X1 = number
			case "y1":
				curve.Y1 = number
			case "x2":
				curve.X2 = number
			case "y2":
				curve.Y2 = number
			}
			return nil
		}
		switch s.Kind {
		case "float":
			keys, err := animationKeys(s, clip.FloatTracks)
			if err != nil {
				return err
			}
			if s.Key < 0 || s.Key >= len(keys) {
				return fmt.Errorf("select a keyframe")
			}
			key := &keys[s.Key]
			switch name {
			case "outTangent":
				key.OutTangent = number
			case "inTangent":
				if s.Key+1 < len(keys) {
					keys[s.Key+1].InTangent = number
				}
			}
		}
		return nil
	})
	if err != nil {
		*s = previous
	} else {
		if updated, ok := a.animationClip(); ok {
			normalizeAnimationSelection(s, updated)
		}
	}
	a.report(err)
	return true
}
func (a *ModuleEditor) addAnimationTrack(clip *project.AnimationClipSpec, element *project.ModElementSpec) error {
	s := &a.animation
	target := scene.AnimationTarget(s.Target)
	known := false
	for _, option := range animationTargets {
		if option.Value == s.Target {
			known = true
			if option.World && s.Space != project.CoordinateSpaceWorld {
				return fmt.Errorf("this track requires world space")
			}
			break
		}
	}
	if !known {
		return fmt.Errorf("select a supported animation target")
	}
	binding := scene.AnimationBinding{Target: target}
	for _, track := range clip.FloatTracks {
		if track.Binding == binding {
			return fmt.Errorf("track already exists")
		}
	}
	for _, track := range clip.TextureTracks {
		if track.Binding == binding {
			return fmt.Errorf("track already exists")
		}
	}
	for _, track := range clip.BoolTracks {
		if track.Binding == binding {
			return fmt.Errorf("track already exists")
		}
	}
	switch target {
	case scene.AnimationTargetSpriteTexture:
		value := project.TextureSpec{Color: "#ffffff"}
		if sprite := element.Components.Sprite; sprite != nil {
			if sprite.Color != "" {
				value.Color = sprite.Color
			}
			if sprite.Texture != nil {
				value = *sprite.Texture
			}
		}
		s.Kind, s.Track = "texture", len(clip.TextureTracks)
		clip.TextureTracks = append(clip.TextureTracks, project.AnimationTrackSpec[project.TextureSpec]{Binding: binding, Keyframes: []project.AnimationKeyframeSpec[project.TextureSpec]{{Time: s.Time, Value: value, Tween: scene.AnimationStep}}})
	case scene.AnimationTargetVisibility:
		s.Kind, s.Track = "bool", len(clip.BoolTracks)
		clip.BoolTracks = append(clip.BoolTracks, project.AnimationTrackSpec[bool]{Binding: binding, Keyframes: []project.AnimationKeyframeSpec[bool]{{Time: s.Time, Value: element.Visible == nil || *element.Visible, Tween: scene.AnimationStep}}})
	default:
		value := float32(0)
		transform := element.Transform
		axis := strings.LastIndex("xyz", s.Target[len(s.Target)-1:])
		var values []float32
		switch {
		case strings.HasPrefix(s.Target, "transform.position."):
			values = transform.Position
		case strings.HasPrefix(s.Target, "transform.scale."):
			values = transform.Scale
			value = 1
		case strings.HasPrefix(s.Target, "transform.rotation."):
			values = transform.Rotation
			if len(values) == 1 {
				if axis == 2 {
					value = values[0]
				}
				values = nil
			}
		case target == scene.AnimationTargetSpriteAlpha:
			if element.Components.Sprite == nil {
				return fmt.Errorf("opacity requires a sprite")
			}
			value = 1
			if element.Components.Sprite.Alpha != nil {
				value = *element.Components.Sprite.Alpha
			}
		}
		if axis >= 0 && axis < len(values) {
			value = values[axis]
		}
		s.Kind, s.Track = "float", len(clip.FloatTracks)
		clip.FloatTracks = append(clip.FloatTracks, project.AnimationTrackSpec[float32]{Binding: binding, Keyframes: []project.AnimationKeyframeSpec[float32]{{Time: s.Time, Value: value, Tween: scene.AnimationLinear}}})
	}
	s.Key, s.Segment = 0, false
	return nil
}
