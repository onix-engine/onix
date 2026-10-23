package editor

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/diiyw/onix/project"
)

// Slot controls display the inherited value without writing an override until
// the user edits it. Unset distinguishes an explicit false or empty string from
// an absent value, even when it matches the displayed default.
func (a *ModuleEditor) slotValueField(b *editorVNodeView, target DocumentID, pointer, label string, kind project.ValueType, raw, fallback json.RawMessage, resetLabel string) {
	unset := len(raw) == 0
	value := raw
	if unset {
		value = fallback
	}
	controlKind, text := string(kind), strings.TrimSpace(string(value))
	var vector [3]float32
	var axes []string
	switch kind {
	case project.StringValue, project.ColorValue:
		text = ""
		if len(value) != 0 {
			_ = json.Unmarshal(value, &text)
		} else if kind == project.ColorValue {
			text = "#00000000"
		}
	case project.BoolValue:
		if text == "" || text == "null" {
			text = "false"
		}
	case project.IntValue, project.NumberValue:
		if text == "" || text == "null" {
			text = "0"
		}
	case project.Vec2Value, project.Vec3Value:
		if len(value) != 0 {
			_ = json.Unmarshal(value, &vector)
		}
		axes = axesXYZ
		if kind == project.Vec2Value {
			axes = axes[:2]
		}
	default:
		controlKind = "json"
	}
	id := workspaceFieldID(target, pointer, "")
	b.open("div", pointer+"-group", "class", "content-property")
	b.open("div", "heading", "class", "slot-value-heading")
	if axes != nil {
		b.text("div", "label", label, "id", id+"-label", "class", "content-field-label")
	} else {
		id, text = a.contentFieldValue(target, pointer, controlKind, text)
		field := a.fields[id]
		field.SlotValue, field.Unset = true, unset
		a.fields[id] = field
		labelFor := id
		if kind == project.ColorValue {
			labelFor = "content-color:" + id
		}
		if kind == project.BoolValue {
			b.open("label", "label", "for", id, "class", "content-check")
			a.contentFieldControl(b, id, label, controlKind, text, "")
			b.text("span", "label", label)
			b.close()
		} else {
			b.text("label", "label", label, "for", labelFor)
		}
	}
	b.button("slot-reset:"+id, resetLabel, "class", "slot-value-reset", "aria-label", resetLabel+" for "+label)
	b.close()
	if axes != nil {
		b.open("div", "vector", "role", "group", "aria-labelledby", id+"-label")
		a.numericVectorFields(b, target, pointer, [4]float64{float64(vector[0]), float64(vector[1]), float64(vector[2])}, axes, 32, workspaceField{SlotValue: true, Unset: unset})
		// The reset action addresses the entire vector, including invalid drafts.
		a.fields[id] = workspaceField{Target: target, Pointer: pointer, Axis: -1, VectorSize: len(axes), SlotValue: true, Unset: unset}
		b.close()
	} else if kind != project.BoolValue {
		a.contentFieldControl(b, id, label, controlKind, text, "")
	}
	b.close()
}

func (a *ModuleEditor) resetSlotField(id string) error {
	field, ok := a.fields[id]
	if !ok || !field.SlotValue {
		return fmt.Errorf("select a slot value")
	}
	delete(a.drafts, id)
	for _, axis := range axesXYZ[:field.VectorSize] {
		delete(a.drafts, workspaceFieldID(field.Target, field.Pointer, axis))
	}
	if field.Unset {
		return nil
	}
	patches, err := a.formPatches()
	if err != nil {
		return err
	}
	patches[field.Target] = append(patches[field.Target], DocumentPatch{Pointer: field.Pointer, Remove: true})
	return a.stagePatches(patches)
}

func slotNumberJSON(kind, value string) (json.RawMessage, error) {
	if kind == "int" {
		number, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil, err
		}
		return json.RawMessage(strconv.FormatInt(number, 10)), nil
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return nil, err
	}
	if math.IsNaN(number) || math.IsInf(number, 0) {
		return nil, fmt.Errorf("number must be finite")
	}
	return json.RawMessage(strconv.FormatFloat(number, 'g', -1, 64)), nil
}
