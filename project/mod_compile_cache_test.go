package project

import (
	"encoding/json"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

type compileReadTrackingFS struct {
	fstest.MapFS
	reads map[string]int
}

func (source compileReadTrackingFS) ReadFile(name string) ([]byte, error) {
	source.reads[name]++
	return fs.ReadFile(source.MapFS, name)
}

func TestCompileCacheDoesNotCopyUnchangedJSON(t *testing.T) {
	source := compileReadTrackingFS{MapFS: moduleSystemSource(), reads: make(map[string]int)}
	var cache CompileCache
	if _, err := cache.Compile(source, ".", nil); err != nil {
		t.Fatal(err)
	}
	clear(source.reads)
	if _, err := cache.Compile(source, ".", nil); err != nil {
		t.Fatal(err)
	}
	for name, count := range source.reads {
		if strings.HasSuffix(name, "/mod.json") && count != 0 {
			t.Fatalf("cached module JSON was copied: %s (%d reads)", name, count)
		}
	}
	source.MapFS["empty/mod.json"].Data = []byte(`{"name":"changed"}`)
	if _, err := cache.Compile(source, ".", nil); err != nil {
		t.Fatal(err)
	}
	if source.reads["empty/mod.json"] != 1 || source.reads["button/mod.json"] != 0 {
		t.Fatal("module edit read more than the changed definition")
	}
}

func TestCompileCacheReusesOnlyUnchangedDefinitions(t *testing.T) {
	source := moduleSystemSource()
	source["empty/mod.json"].Data = []byte(`{"elements":[{"id":"part"}]}`)
	var cache CompileCache
	compile := func() *ProjectPlan {
		t.Helper()
		plan, err := cache.Compile(source, ".", nil)
		if err != nil {
			t.Fatal(err)
		}
		full, err := CompileProject(source, ".")
		if err != nil || !reflect.DeepEqual(plan, full) {
			t.Fatalf("cached and full compilation differ: %v", err)
		}
		return plan
	}
	first := compile()
	second := compile()
	for i := range first.Mods {
		a, b := &first.Mods[i], &second.Mods[i]
		if len(a.Slots) > 0 && &a.Slots[0] != &b.Slots[0] || len(a.Elements) > 0 && &a.Elements[0] != &b.Elements[0] {
			t.Fatalf("unchanged module %s was recompiled", a.ID)
		}
	}
	source["empty/mod.json"].Data = []byte(`{"name":"changed","elements":[{"id":"part","transform":{"position":[5,6]}}]}`)
	third := compile()
	for i := range second.Mods {
		a, b := &second.Mods[i], &third.Mods[i]
		if a.ID == "empty" {
			if &a.Elements[0] == &b.Elements[0] || b.Elements[0].Transform.Position[0] != 5 || a.Spec.Name != "" {
				t.Fatal("changed module reused stale content or mutated an earlier plan")
			}
		} else if len(a.Slots) > 0 && &a.Slots[0] != &b.Slots[0] {
			t.Fatalf("editing empty invalidated %s", a.ID)
		}
	}
	delete(source, "empty/mod.json")
	compile()
	if _, exists := cache.mods["empty"]; exists {
		t.Fatal("deleted module remained in the compilation cache")
	}
}

func TestCompileCacheInvalidatesTypeSchemasAndValidatesSceneEdits(t *testing.T) {
	source := moduleSystemSource()
	var cache CompileCache
	first, err := cache.Compile(source, ".", nil)
	if err != nil {
		t.Fatal(err)
	}
	var spec ProjectSpec
	if err := json.Unmarshal(source["resources/project.json"].Data, &spec); err != nil {
		t.Fatal(err)
	}
	spec.Types["Style"].Fields["extra"] = BoolValue
	source["resources/project.json"].Data, err = json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	second, err := cache.Compile(source, ".", nil)
	if err != nil {
		t.Fatal(err)
	}
	full, err := CompileProject(source, ".")
	if err != nil || !reflect.DeepEqual(second, full) {
		t.Fatalf("schema changes reused stale defaults: %v", err)
	}
	if &first.Mods[0].Slots[0] == &second.Mods[0].Slots[0] {
		t.Fatal("schema change kept cached module slots")
	}
	before, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	source["resources/scenes/main.json"].Data = []byte(`{"modules":[{"id":"bad","mod":"button","slots":{"undeclared":true}}]}`)
	if _, err := cache.Compile(source, ".", nil); err == nil || !strings.Contains(err.Error(), "slot is not declared") {
		t.Fatalf("cached module bypassed scene slot validation: %v", err)
	}
	after, err := json.Marshal(second)
	if err != nil || string(before) != string(after) {
		t.Fatal("failed compilation mutated the previously returned plan")
	}
	source["resources/scenes/main.json"].Data = []byte(`{"modules":[{"id":"start","mod":"button","slots":{"text":"Start"}}]}`)
	source["button/mod.json"].Data = []byte(`{"slots":{"text":{"type":"bool"}}}`)
	if _, err := cache.Compile(source, ".", nil); err == nil || !strings.Contains(err.Error(), "expected bool") {
		t.Fatalf("module slot type change bypassed scene validation: %v", err)
	}
	source["button/mod.json"].Data = []byte(`{}`)
	if _, err := cache.Compile(source, ".", nil); err == nil || !strings.Contains(err.Error(), "slot is not declared") {
		t.Fatalf("removed slot bypassed scene validation: %v", err)
	}
}

func TestCompileCacheChecksExternalResourceFiles(t *testing.T) {
	for _, name := range []string{"button/animations/press.json", "font/ui.ttf", "empty/mod.go"} {
		t.Run(name, func(t *testing.T) {
			source := moduleSystemSource()
			source["empty/mod.json"].Data = []byte(`{"behavior":"Example"}`)
			source["empty/mod.go"] = &fstest.MapFile{Data: []byte("package empty"), Mode: 0o644}
			var cache CompileCache
			if _, err := cache.Compile(source, ".", nil); err != nil {
				t.Fatal(err)
			}
			file := source[name]
			delete(source, name)
			if _, err := cache.Compile(source, ".", nil); err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("cached resource removal was not detected: %v", err)
			}
			source[name] = file
			if _, err := cache.Compile(source, ".", nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCompileCacheRevalidatesScenePhysics(t *testing.T) {
	source := moduleSystemSource()
	source["empty/mod.json"].Data = []byte(`{"space":"world","elements":[{"id":"body","components":{"rigidBody3D":{"type":"dynamic"}}}]}`)
	var cache CompileCache
	if _, err := cache.Compile(source, ".", nil); err != nil {
		t.Fatal(err)
	}
	source["resources/scenes/main.json"].Data = []byte(`{"transform":{"scale":[2,2,2]},"modules":[{"id":"body","mod":"empty"}]}`)
	if _, err := cache.Compile(source, ".", nil); err == nil || !strings.Contains(err.Error(), "unit scale") {
		t.Fatalf("scene scale bypassed cached physics validation: %v", err)
	}
	source["resources/scenes/main.json"].Data = []byte(`{"modules":[{"id":"parent","mod":"empty","placement":{"scale":[2,2,2]},"children":[{"id":"child","mod":"empty"}]}]}`)
	if _, err := cache.Compile(source, ".", nil); err == nil || !strings.Contains(err.Error(), "unit scale") {
		t.Fatalf("placement scale bypassed cached physics validation: %v", err)
	}
	source["resources/scenes/main.json"].Data = []byte(`{"modules":[{"id":"body","mod":"empty"}]}`)
	if _, err := cache.Compile(source, ".", nil); err != nil {
		t.Fatal(err)
	}
}
