package editor

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/diiyw/onix/scene"
	htmlui "github.com/diiyw/onix/ui/html"
)

type workspaceElementPicker struct {
	Field    string
	Trigger  string
	Anchor   rect
	Raw      string
	Values   []string
	Selected map[string]bool
}

func (p *workspaceElementPicker) read(value string) error {
	if p.Raw == value {
		return nil
	}
	if err := json.Unmarshal([]byte(value), &p.Values); err != nil {
		return err
	}
	if p.Selected == nil {
		p.Selected = make(map[string]bool, len(p.Values))
	} else {
		clear(p.Selected)
	}
	for _, name := range p.Values {
		p.Selected[name] = true
	}
	p.Raw = value
	return nil
}

func (a *ModuleEditor) contentElementsField(b *editorVNodeView, target DocumentID, pointer, label, value string) {
	id, value := a.contentFieldValue(target, pointer, "json", value)
	p := &a.elementPicker
	if err := p.read(value); err != nil {
		return
	}
	trigger := "content-elements:" + id
	p.Trigger = trigger
	b.text("div", id+"-label", label, "id", id+"-label", "class", "content-field-label")
	b.open("div", id, "id", id, "class", "content-elements", "role", "group", "aria-labelledby", id+"-label")
	b.open("div", "tags", "class", "content-element-tags")
	for _, name := range p.Values {
		b.open("span", name, "class", "content-element-tag")
		b.text("span", "name", name)
		remove := "content-element-remove:" + id + ":" + name
		a.fields[remove] = workspaceField{Target: target, Pointer: pointer, Kind: "content-element-remove", Value: name, Axis: -1}
		b.open("button", remove, "id", remove, "type", "button", "class", "content-element-remove", "aria-label", "Remove "+name, "title", "Remove "+name)
		b.text("img", "icon", "", "src", "onix-rightbar/close.svg", "alt", "")
		b.close()
		b.close()
	}
	if len(p.Values) == 0 {
		b.button(trigger+":empty", "Select particles", "class", "content-elements-empty", "aria-haspopup", "dialog", "aria-expanded", strconv.FormatBool(p.Field == id), "aria-controls", "content-elements-list")
		a.fields[trigger+":empty"] = workspaceField{Target: target, Pointer: pointer, Kind: "content-elements-open", Axis: -1}
	}
	b.close()
	a.fields[trigger] = workspaceField{Target: target, Pointer: pointer, Kind: "content-elements-open", Axis: -1}
	b.open("button", trigger, "id", trigger, "type", "button", "class", "content-elements-open", "aria-label", "Select "+label, "aria-haspopup", "dialog", "aria-expanded", strconv.FormatBool(p.Field == id), "aria-controls", "content-elements-list")
	arrow := "onix-rightbar/arrow.svg"
	if p.Field == id {
		arrow += "#up"
	}
	b.text("img", "icon", "", "src", arrow, "alt", "")
	b.close()
	b.close()
}

func (a *ModuleEditor) bindElementPicker(root htmlui.Node) {
	trigger := a.elementPicker.Trigger
	if _, exists := a.fields[trigger]; !exists {
		return
	}
	if node, ok := root.FindByID(strings.TrimPrefix(trigger, "content-elements:")); ok {
		node.OnClick(a.clickElementPicker)
	}
}

func (a *ModuleEditor) clickElementPicker(event *htmlui.PointerEvent) {
	if event.Handled {
		return
	}
	a.action(a.elementPicker.Trigger)
	event.Consume()
}

// Render the dropdown outside the inspector's scroll container.
func (a *ModuleEditor) renderElementPicker(b *editorVNodeView) {
	p := &a.elementPicker
	field, exists := a.fields[p.Field]
	if p.Field == "" || !exists {
		p.Field = ""
		b.text("div", "content-elements-list", "", "id", "content-elements-list", "role", "dialog", "aria-label", "Select particles", "hidden", "")
		return
	}
	mod := a.viewMod(a.selectedMod())
	count := 0
	b.open("div", "content-elements-list", "id", "content-elements-list", "class", "content-picker content-elements-picker", "role", "dialog", "aria-labelledby", p.Field+"-label")
	for _, element := range mod.Elements {
		if element.Components.Particle == nil {
			continue
		}
		count++
		id := "content-element-option:" + p.Field + ":" + element.ID
		a.fields[id] = workspaceField{Target: field.Target, Pointer: field.Pointer, Kind: "content-element-option", Value: element.ID, Axis: -1, Selected: p.Selected[element.ID]}
		b.open("label", element.ID, "for", id, "class", "content-element-option")
		b.open("input", id, "id", id, "type", "checkbox")
		if p.Selected[element.ID] {
			b.attr("checked", "")
		}
		b.close()
		b.text("span", "name", element.ID)
		b.close()
	}
	if count == 0 {
		b.text("p", "empty", "Add a Particle component to an element first.", "class", "muted")
	}
	bounds := a.contentPickerBounds(p.Anchor, count)
	bounds.W = min(p.Anchor.W, max(0, a.width-16))
	bounds.X = clamp(p.Anchor.X, 8, max(8, a.width-bounds.W-8))
	b.attr("style", workspaceRectStyle(bounds))
	b.close()
}

// CSS layout boxes exclude the runtime scroll offset. Follow the trigger's
// native parent chain to place the dropdown at its visible border box.
func (a *ModuleEditor) elementPickerAnchor(id string) (rect, bool) {
	node, ok := htmlui.NodeByID(a.world, a.shell, id)
	if !ok {
		return rect{}, false
	}
	box, ok := node.Bounds()
	if !ok {
		return rect{}, false
	}
	anchor := rect{X: box.X, Y: box.Y, W: box.Width, H: box.Height}
	control, ok := htmlui.ControlByID(a.world, a.shell, a.elementPicker.Trigger)
	if !ok {
		return rect{}, false
	}
	for entity := control; entity != 0 && entity != a.shell; {
		if scroll := a.world.GetRef[scene.UIScroll](entity); scroll != nil {
			anchor.X += scroll.Position.X()
			anchor.Y += scroll.Position.Y()
		}
		parent := a.world.GetRef[scene.Parent](entity)
		if parent == nil {
			break
		}
		entity = parent.Entity
	}
	return anchor, true
}

func (a *ModuleEditor) elementPickerAction(field workspaceField) error {
	id := workspaceFieldID(field.Target, field.Pointer, "")
	p := &a.elementPicker
	if field.Kind == "content-elements-open" {
		if p.Field == id {
			p.Field = ""
			return nil
		}
		p.Field = id
		p.Trigger = "content-elements:" + id
		a.componentPickerOpen, a.componentMenu, a.menuOpen, a.contextOpen, a.fileMenuOpen = false, "", "", false, false
		if anchor, ok := a.elementPickerAnchor(id); ok {
			p.Anchor = anchor
			return nil
		}
		panel := a.layout().Inspector
		p.Anchor = rect{X: panel.X + 14, Y: panel.Y + 40, W: panel.W - 28, H: 28}
		return nil
	}
	value := a.fields[id].Value
	if draft, exists := a.drafts[id]; exists {
		value = draft
	}
	if err := p.read(value); err != nil {
		return err
	}
	index := slices.Index(p.Values, field.Value)
	if index >= 0 {
		p.Values = slices.Delete(p.Values, index, index+1)
	} else if field.Kind == "content-element-option" {
		p.Values = append(p.Values, field.Value)
	} else {
		return nil
	}
	// Empty selections remain editable drafts; compilation validates on save.
	if len(p.Values) == 0 {
		a.drafts[id] = "[]"
	} else {
		a.drafts[id] = encodeFormJSON(p.Values)
	}
	p.Raw = ""
	return nil
}
