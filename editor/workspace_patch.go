package editor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

type DocumentPatch struct {
	Pointer string
	Value   json.RawMessage
	Remove  bool
}

// PatchDocuments applies a form containing scene and definition edits as one
// validated transaction. Each history entry still records the owning files.
func (w *WorkspaceDocument) PatchDocuments(label string, patches map[DocumentID][]DocumentPatch) error {
	ids := make([]DocumentID, 0, len(patches))
	for id := range patches {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	command := workspaceCommand{Label: label, Edits: make([]documentEdit, 0, len(ids))}
	for _, id := range ids {
		state := w.states[id]
		if state == nil || state.data == nil {
			return fmt.Errorf("unknown document %q", id)
		}
		data, err := applyDocumentPatches(state.data, patches[id])
		if err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
		command.Edits = append(command.Edits, documentEdit{Target: id, After: data})
	}
	return w.commit(command)
}

func applyDocumentPatches(data []byte, patches []DocumentPatch) ([]byte, error) {
	if len(patches) == 0 {
		return data, nil
	}
	// Expand only edited containers, preserving untouched JSON and number bytes.
	value := documentPatchValue{raw: data}
	for _, patch := range patches {
		if patch.Pointer == "" {
			if patch.Remove || !json.Valid(patch.Value) {
				return nil, fmt.Errorf("invalid document replacement")
			}
			value = documentPatchValue{raw: patch.Value}
			continue
		}
		if !strings.HasPrefix(patch.Pointer, "/") {
			return nil, fmt.Errorf("invalid JSON pointer %q", patch.Pointer)
		}
		if !patch.Remove && !json.Valid(patch.Value) {
			return nil, fmt.Errorf("%s: invalid JSON value", patch.Pointer)
		}
		var err error
		value, err = patchDocumentValue(value, patch.Pointer[1:], patch)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", patch.Pointer, err)
		}
	}
	if value.raw != nil {
		return value.raw, nil
	}
	raw := value.appendJSON(make([]byte, 0, len(data)))
	// Commit and staging own canonical formatting; compilation only needs JSON.
	return append(raw, '\n'), nil
}

type documentPatchValue struct {
	raw    json.RawMessage
	object map[string]documentPatchValue
	array  []documentPatchValue
}

func (value *documentPatchValue) UnmarshalJSON(data []byte) error {
	*value = documentPatchValue{raw: slices.Clone(data)}
	return nil
}

func (value *documentPatchValue) expand() error {
	if value.object != nil || value.array != nil {
		return nil
	}
	raw := bytes.TrimSpace(value.raw)
	if len(raw) != 0 && raw[0] == '[' {
		if err := json.Unmarshal(raw, &value.array); err != nil {
			return err
		}
	} else {
		if len(raw) != 0 {
			if err := json.Unmarshal(raw, &value.object); err != nil {
				return err
			}
		}
		if value.object == nil {
			value.object = make(map[string]documentPatchValue)
		}
	}
	value.raw = nil
	return nil
}

func (value documentPatchValue) appendJSON(data []byte) []byte {
	if value.raw != nil {
		return append(data, value.raw...)
	}
	if value.array != nil {
		data = append(data, '[')
		for i, child := range value.array {
			if i != 0 {
				data = append(data, ',')
			}
			data = child.appendJSON(data)
		}
		return append(data, ']')
	}
	data = append(data, '{')
	for i, key := range sortedWorkspaceKeys(value.object) {
		if i != 0 {
			data = append(data, ',')
		}
		encoded, _ := json.Marshal(key)
		data = append(data, encoded...)
		data = append(data, ':')
		data = value.object[key].appendJSON(data)
	}
	return append(data, '}')
}

func patchDocumentValue(value documentPatchValue, pointer string, patch DocumentPatch) (documentPatchValue, error) {
	if err := value.expand(); err != nil {
		return documentPatchValue{}, err
	}
	key, tail, nested := strings.Cut(pointer, "/")
	key = strings.ReplaceAll(strings.ReplaceAll(key, "~1", "/"), "~0", "~")
	if value.array != nil {
		values := value.array
		index, err := strconv.Atoi(key)
		if err != nil || index < 0 || index >= len(values) {
			return documentPatchValue{}, fmt.Errorf("array index %q is out of range", key)
		}
		if !nested {
			if patch.Remove {
				values = slices.Delete(values, index, index+1)
			} else {
				values[index] = documentPatchValue{raw: patch.Value}
			}
		} else {
			values[index], err = patchDocumentValue(values[index], tail, patch)
			if err != nil {
				return documentPatchValue{}, err
			}
		}
		value.array = values
		return value, nil
	}
	values := value.object
	if !nested {
		if patch.Remove {
			delete(values, key)
		} else {
			values[key] = documentPatchValue{raw: patch.Value}
		}
	} else {
		value, err := patchDocumentValue(values[key], tail, patch)
		if err != nil {
			return documentPatchValue{}, err
		}
		values[key] = value
	}
	return value, nil
}
