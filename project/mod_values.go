package project

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"

	"github.com/diiyw/onix/graphics"
)

// ParseColor shares the engine's authored CSS color parser with module loaders.
func ParseColor(value string, alpha float32) (graphics.ColorRGBA, error) {
	return parseColorValue(value, alpha)
}

func isModValueType(kind ValueType) bool {
	switch kind {
	case StringValue, BoolValue, NumberValue, IntValue, ColorValue, Vec2Value, Vec3Value, VisualValue:
		return true
	}
	return false
}

func isModProtocolType(kind ValueType) bool {
	switch kind {
	case "SceneSwitchRequest", "SceneSwitchResult", "SceneStartup", "TextRenderRequest", "TextRenderResult", "Collision2D", "Collision3D":
		return true
	}
	return false
}

func compileSlotValue(kind ValueType, data []byte, types map[ValueType]ValueSchema, depth int) (SlotValue, error) {
	result := SlotValue{Type: kind}
	if depth > 32 {
		return result, fmt.Errorf("cyclic or excessively nested value type %q", kind)
	}
	if kind == "" {
		if len(data) != 0 && !bytes.Equal(bytes.TrimSpace(data), []byte("{}")) {
			return result, fmt.Errorf("an event without a payload type only accepts {}")
		}
		return result, nil
	}
	if kind == VisualValue {
		if len(data) != 0 && !bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
			return result, fmt.Errorf("visual outputs are runtime handles; initial value must be null")
		}
		return result, nil
	}
	if isModValueType(kind) {
		if len(data) == 0 {
			return result, nil
		}
		var err error
		switch kind {
		case StringValue, ColorValue:
			err = decodeModJSON(data, &result.String)
			if err == nil && kind == ColorValue {
				color, colorErr := parseColorValue(result.String, 1)
				err = colorErr
				result.Vector = [4]float32{color.R, color.G, color.B, color.A}
			}
		case BoolValue:
			err = decodeModJSON(data, &result.Bool)
		case NumberValue:
			err = decodeModJSON(data, &result.Number)
			if math.IsNaN(result.Number) || math.IsInf(result.Number, 0) {
				err = fmt.Errorf("number must be finite")
			}
		case IntValue:
			err = decodeModJSON(data, &result.Integer)
		case Vec2Value, Vec3Value:
			var values []float32
			err = decodeModJSON(data, &values)
			count := 2
			if kind == Vec3Value {
				count = 3
			}
			if err == nil && (len(values) != count || !finiteModValues(values)) {
				err = fmt.Errorf("expected %d finite numbers", count)
			}
			copy(result.Vector[:], values)
		}
		if err != nil {
			return result, fmt.Errorf("expected %s: %w", kind, err)
		}
		return result, nil
	}
	schema, exists := types[kind]
	if !exists {
		return result, fmt.Errorf("unknown value type %q", kind)
	}
	var fields map[string]json.RawMessage
	if len(data) != 0 {
		if err := decodeModJSON(data, &fields); err != nil {
			return result, fmt.Errorf("expected %s object: %w", kind, err)
		}
		for name := range fields {
			if _, exists := schema.Fields[name]; !exists {
				return result, fmt.Errorf("%s.%s: unknown field", kind, name)
			}
		}
	}
	for _, name := range sortedModKeys(schema.Fields) {
		if !validModName(name) {
			return result, fmt.Errorf("%s.%s: invalid field name", kind, name)
		}
		fieldType := schema.Fields[name]
		if fieldType == "" {
			return result, fmt.Errorf("%s.%s: field type is required", kind, name)
		}
		field, err := compileSlotValue(fieldType, fields[name], types, depth+1)
		if err != nil {
			return result, fmt.Errorf("%s.%s: %w", kind, name, err)
		}
		result.Fields = append(result.Fields, SlotFieldValue{Name: name, Value: field})
	}
	return result, nil
}

func cloneSlotValue(value SlotValue) SlotValue {
	if len(value.Fields) == 0 {
		return value
	}
	result := value
	result.Fields = make([]SlotFieldValue, len(value.Fields))
	for index, field := range value.Fields {
		result.Fields[index] = SlotFieldValue{Name: field.Name, Value: cloneSlotValue(field.Value)}
	}
	return result
}
