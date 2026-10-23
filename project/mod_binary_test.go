package project

import (
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/diiyw/onix/internal/typedbin"
)

func TestModuleBinaryPlanRoundTripAndRejectsInvalidData(t *testing.T) {
	source := fstest.MapFS{
		"resources/project.json":     {Data: []byte(`{"entry":"main.json","scenes":["main.json"]}`)},
		"resources/scenes/main.json": {Data: []byte(`{"modules":[{"id":"first","mod":"empty"},{"id":"second","mod":"empty"}]}`)},
		"empty/mod.json":             {Data: []byte(`{"slots":{"value":{"type":"number","default":12}}}`)},
	}
	plan, err := CompileProject(source, ".")
	if err != nil {
		t.Fatal(err)
	}
	data, err := MarshalModulePlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	if !typedbin.IsEncoded(data) {
		t.Fatal("plan is not typed binary")
	}
	decoded, err := ReadModulePlan(fstest.MapFS{ModulePlanFile: {Data: data}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan, decoded) {
		t.Fatal("resolved plan changed")
	}
	for _, bad := range [][]byte{data[:8], append(append([]byte(nil), data...), 1), []byte(`{"modules":[]}`)} {
		if _, err := ReadModulePlan(fstest.MapFS{ModulePlanFile: {Data: bad}}); err == nil {
			t.Fatal("invalid binary accepted")
		}
	}
	plan.Entry = 4
	bad, err := MarshalModulePlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadModulePlan(fstest.MapFS{ModulePlanFile: {Data: bad}}); err == nil {
		t.Fatal("invalid entry index accepted")
	}
}
