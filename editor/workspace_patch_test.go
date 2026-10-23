package editor

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/diiyw/onix/project"
)

func TestWorkspacePatchBatchOrderAndPrecision(t *testing.T) {
	data := []byte(`{"items":[{"value":1},{"value":2},{"value":3}],"integer":9007199254740993,"a/b":{"~key":0}}`)
	patches := []DocumentPatch{
		{Pointer: "/items/0", Remove: true},
		{Pointer: "/items/0/value", Value: json.RawMessage(`20`)},
		{Pointer: "/items/1", Value: json.RawMessage(`{"value":30}`)},
		{Pointer: "/items/1/value", Value: json.RawMessage(`31`)},
		{Pointer: "/a~1b/~0key", Value: json.RawMessage(`4`)},
	}
	got, err := applyDocumentPatches(data, patches)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := decodeDocument[map[string]json.RawMessage](got)
	if err != nil {
		t.Fatal(err)
	}
	var items []struct{ Value int }
	if err := json.Unmarshal(actual["items"], &items); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(items, []struct{ Value int }{{20}, {31}}) || string(actual["integer"]) != "9007199254740993" {
		t.Fatalf("patches changed order or integer precision: %s", got)
	}
	var escaped map[string]int
	if err := json.Unmarshal(actual["a/b"], &escaped); err != nil || escaped["~key"] != 4 {
		t.Fatalf("escaped pointer changed: %s (%v)", got, err)
	}
	if string(data) != `{"items":[{"value":1},{"value":2},{"value":3}],"integer":9007199254740993,"a/b":{"~key":0}}` {
		t.Fatal("patches mutated the immutable source")
	}
}

func TestWorkspacePatchReplacementAndMissingObjects(t *testing.T) {
	patches := []DocumentPatch{
		{Value: json.RawMessage(`{"items":[null]}`)},
		{Pointer: "/items/0/nested/value", Value: json.RawMessage(`1`)},
		{Pointer: "/items/0/nested/removed", Remove: true},
	}
	got, err := applyDocumentPatches([]byte(`invalid old document`), patches)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string][]map[string]map[string]int
	if err := json.Unmarshal(got, &value); err != nil || value["items"][0]["nested"]["value"] != 1 {
		t.Fatalf("replacement lost subsequent patches: %s (%v)", got, err)
	}
	for _, patch := range []DocumentPatch{
		{Pointer: "items/0", Value: json.RawMessage(`1`)},
		{Pointer: "/items/9", Remove: true},
		{Pointer: "/items/0", Value: json.RawMessage(`invalid`)},
		{Remove: true},
	} {
		if _, err := applyDocumentPatches(got, []DocumentPatch{patch}); err == nil {
			t.Fatalf("accepted invalid patch: %+v", patch)
		}
	}
}

func BenchmarkWorkspacePatchBatch(b *testing.B) {
	mod := project.ModSpec{Elements: make([]project.ModElementSpec, 1000)}
	for i := range mod.Elements {
		mod.Elements[i] = project.ModElementSpec{ID: fmt.Sprintf("part%d", i), Transform: project.ModTransformSpec{Position: []float32{float32(i), 0}}}
	}
	data, err := documentJSON(mod)
	if err != nil {
		b.Fatal(err)
	}
	for _, count := range []int{1, 8, 16} {
		b.Run(fmt.Sprintf("fields_%d", count), func(b *testing.B) {
			patches := make([]DocumentPatch, count)
			for i := range patches {
				patches[i] = DocumentPatch{Pointer: fmt.Sprintf("/elements/%d/transform/position/0", i), Value: json.RawMessage(`1`)}
			}
			b.ReportAllocs()
			for b.Loop() {
				if _, err := applyDocumentPatches(data, patches); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
