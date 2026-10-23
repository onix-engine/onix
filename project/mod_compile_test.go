package project

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func moduleSystemSource() fstest.MapFS {
	files := map[string]string{
		"resources/project.json":       `{"entry":"main.json","scenes":["main.json","menu.json"],"types":{"Style":{"fields":{"label":"string","size":"int"}}}}`,
		"resources/scenes/main.json":   `{"modules":[{"id":"font","mod":"font"},{"id":"start","mod":"button","placement":{"position":[320,240]},"slots":{"text":"Start"},"children":[{"id":"again","mod":"button","placement":{"position":[10,20]}}]}]}`,
		"resources/scenes/menu.json":   `{"modules":[{"id":"start","mod":"button"},{"id":"font","mod":"font"}]}`,
		"button/mod.json":              `{"space":"ui","transform":{"position":[2,3]},"components":{"sprite":{"color":"#2f7df6","size":[160,48]},"input":{"pointer":true}},"resources":{"press":{"kind":"animation","path":"animations/press.json"}},"slots":{"text":{"type":"string","default":"Button"},"enabled":{"type":"bool","default":true},"label":{"type":"visual"},"style":{"type":"Style","default":{"label":"default","size":12}}}}`,
		"button/animations/press.json": `{}`,
		"font/mod.json":                `{"resources":{"ui":{"kind":"font","path":"ui.ttf"}}}`,
		"font/ui.ttf":                  "test font bytes",
		"empty/mod.json":               `{}`,
	}
	result := make(fstest.MapFS, len(files))
	for name, data := range files {
		result[name] = &fstest.MapFile{Data: []byte(data), Mode: 0o644}
	}
	return result
}

func TestModuleSystemCompilesReusableDefinitionsAndScenePlacement(t *testing.T) {
	source := &moduleCountingFS{FS: moduleSystemSource(), reads: make(map[string]int)}
	plan, err := CompileProject(source, ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Mods) != 3 || len(plan.Scenes) != 2 || plan.Entry != 0 {
		t.Fatalf("unexpected definition counts: %+v", plan)
	}
	scene := &plan.Scenes[0]
	if len(scene.Instances) != 3 || scene.Instances[2].Parent != 1 || scene.Instances[1].Parent != -1 {
		t.Fatalf("hierarchy was not flattened in stable order: %+v", scene.Instances)
	}
	start, again := &scene.Instances[1], &scene.Instances[2]
	if start.Mod != again.Mod || start.Mod != plan.Scenes[1].Instances[0].Mod {
		t.Fatal("instances do not share one compiled module definition")
	}
	definition := &plan.Mods[start.Mod]
	if definition.Transform.Position != [3]float32{2, 3, 0} || start.Placement.Position != [3]float32{320, 240, 0} {
		t.Fatal("scene placement and module transform were combined or overwritten")
	}
	if again.Placement.Scale != [3]float32{1, 1, 1} || !again.Enabled || !again.Visible {
		t.Fatal("instance defaults were not compiled")
	}
	for index, slot := range definition.Slots {
		if slot.Name == "text" {
			if start.Slots[index].String != "Start" || again.Slots[index].String != "Button" {
				t.Fatal("slot override did not stay local to its instance")
			}
		}
		if slot.Name == "style" {
			start.Slots[index].Fields[0].Value.String = "changed"
			if again.Slots[index].Fields[0].Value.String != "default" || slot.Default.Fields[0].Value.String != "default" {
				t.Fatal("structured slot defaults are shared mutable storage")
			}
		}
	}
	if source.reads["button/mod.json"] != 1 || source.reads["font/mod.json"] != 1 {
		t.Fatalf("definitions were reloaded for reuse: %+v", source.reads)
	}

}

type moduleCountingFS struct {
	fs.FS
	reads map[string]int
}

func (source *moduleCountingFS) Open(name string) (fs.File, error) {
	source.reads[name]++
	return source.FS.Open(name)
}

func (source *moduleCountingFS) Stat(name string) (fs.FileInfo, error) {
	return fs.Stat(source.FS, name)
}

func TestModuleSystemStrictDiagnostics(t *testing.T) {
	tests := []struct {
		name string
		file string
		data string
		want string
	}{
		{"old nodes", "resources/scenes/main.json", `{"nodes":[]}`, `main.json: json: unknown field "nodes"`},
		{"null scene", "resources/scenes/main.json", `null`, "main.json: null"},
		{"null modules", "resources/scenes/main.json", `{"modules":null}`, "main.json: modules"},
		{"missing modules", "resources/scenes/main.json", `{}`, "main.json: modules"},
		{"invalid scene exit policy", "resources/scenes/main.json", `{"exitPolicy":"unknown","modules":[]}`, "main.json: exitPolicy: expected destroy or retain"},
		{"extra document", "resources/scenes/main.json", `{"modules":[]} {}`, "main.json: multiple JSON"},
		{"unknown module field", "empty/mod.json", `{"scripts":[]}`, `empty/mod.json: json: unknown field "scripts"`},
		{"nested unknown field", "empty/mod.json", `{"components":{"sprite":{"typo":true}}}`, `empty/mod.json: json: unknown field "typo"`},
		{"duplicate instances", "resources/scenes/main.json", `{"modules":[{"id":"same","mod":"empty","children":[{"id":"same","mod":"empty"}]}]}`, "main.json: modules[0].children[0].id: duplicate"},
		{"missing module", "resources/scenes/main.json", `{"modules":[{"id":"x","mod":"missing"}]}`, "main.json: modules[0].mod: missing/mod.json"},
		{"escaping module", "resources/scenes/main.json", `{"modules":[{"id":"x","mod":"../font"}]}`, "module path"},
		{"unknown slot", "resources/scenes/main.json", `{"modules":[{"id":"x","mod":"empty","slots":{"unknown":1}}]}`, "modules[0].slots.unknown"},
		{"wrong slot type", "resources/scenes/main.json", `{"modules":[{"id":"x","mod":"button","slots":{"text":42}}]}`, "modules[0].slots.text: expected string"},
		{"null scalar", "empty/mod.json", `{"slots":{"x":{"type":"string","default":null}}}`, "slots.x.default: expected string"},
		{"unknown slot type", "empty/mod.json", `{"slots":{"x":{"type":"Unregistered"}}}`, "unknown value type"},
		{"missing slot type", "empty/mod.json", `{"slots":{"x":{}}}`, "slots.x.type: a value type is required"},
		{"visual path reference", "empty/mod.json", `{"slots":{"x":{"type":"visual","default":"font/ui.ttf"}}}`, "visual outputs are runtime handles"},
		{"vector dimensions", "empty/mod.json", `{"slots":{"x":{"type":"vec2","default":[1,2,3]}}}`, "expected 2 finite"},
		{"integer fraction", "empty/mod.json", `{"slots":{"x":{"type":"int","default":1.5}}}`, "expected int"},
		{"resource escape", "empty/mod.json", `{"resources":{"x":{"kind":"font","path":"../font/ui.ttf"}}}`, "resources.x.path: resource path"},
		{"texture escape", "empty/mod.json", `{"components":{"sprite":{"texture":{"image":"../font/ui.ttf"}}}}`, "components: sprite.texture: resource path"},
		{"wrong private font", "empty/mod.json", `{"components":{"text":{"font":"font.ui"}}}`, "must be a font declared by this module"},
		{"missing behavior", "empty/mod.json", `{"behavior":"Mod"}`, "empty/mod.json: behavior: empty/mod.go"},
		{"unexported behavior", "empty/mod.json", `{"behavior":"mod"}`, "behavior must name an exported Go type"},
		{"nested scene path", "resources/project.json", `{"entry":"scenes/main.json","scenes":["scenes/main.json"]}`, "must be a JSON filename"},
		{"unregistered entry", "resources/project.json", `{"entry":"game.json","scenes":["main.json"]}`, "entry scene"},
		{"duplicate scenes", "resources/project.json", `{"entry":"main.json","scenes":["main.json","main.json"]}`, "duplicate scene"},
		{"cyclic type", "resources/project.json", `{"entry":"main.json","scenes":["main.json"],"types":{"Cycle":{"fields":{"next":"Cycle"}}}}`, "cyclic or excessively nested"},
		{"invalid default color", "empty/mod.json", `{"slots":{"x":{"type":"color","default":"bad"}}}`, "expected color"},
		{"unknown structure field", "empty/mod.json", `{"slots":{"x":{"type":"Style","default":{"typo":true}}}}`, "Style.typo: unknown field"},
		{"invalid transform", "empty/mod.json", `{"transform":{"position":[1]}}`, "transform: position must contain"},
		{"missing private resource", "empty/mod.json", `{"resources":{"x":{"kind":"data","path":"missing.json"}}}`, "resources.x.path: empty/missing.json"},
		{"mixed coordinate spaces", "resources/scenes/main.json", `{"modules":[{"id":"x","mod":"empty","children":[{"id":"y","mod":"world"}]}]}`, "parent and child coordinate spaces differ"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := moduleSystemSource()
			source["world/mod.json"] = &fstest.MapFile{Data: []byte(`{"space":"world"}`)}
			source[test.file] = &fstest.MapFile{Data: []byte(test.data)}
			_, err := CompileProject(source, ".")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("want %q, got %v", test.want, err)
			}
		})
	}
}

func TestModuleSystemResourceLayoutsAndEmptyMod(t *testing.T) {
	source := fstest.MapFS{
		"resources/project.json":     {Data: []byte(`{"entry":"main.json","scenes":["main.json"]}`)},
		"resources/scenes/main.json": {Data: []byte(`{"modules":[{"id":"empty","mod":"empty"}]}`)},
		"empty/mod.json":             {Data: []byte(`{}`)},
		"assets/mod.json":            {Data: []byte(`{"resources":{"root":{"kind":"data","path":"data.json"},"nested":{"kind":"image","path":"custom/art/icon.png"}}}`)},
		"assets/data.json":           {Data: []byte(`{}`)},
		"assets/custom/art/icon.png": {Data: []byte("image")},
	}
	plan, err := CompileProject(source, ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Mods) != 2 || plan.Mods[0].Resources[0].Path != "assets/custom/art/icon.png" || plan.Mods[0].Resources[1].Path != "assets/data.json" {
		t.Fatalf("custom resource paths were not resolved: %+v", plan.Mods)
	}
}

func TestModuleSystemSubdirectoryAndDeterministicPlans(t *testing.T) {
	source := moduleSystemSource()
	nested := make(fstest.MapFS, len(source))
	for name, file := range source {
		nested["games/demo/"+name] = file
	}
	plan, err := CompileProject(nested, "games/demo")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mods[0].ID != "button" || plan.Mods[1].ID != "empty" || plan.Mods[2].ID != "font" || plan.Mods[0].Slots[0].Name != "enabled" {
		t.Fatal("definition and slot indices are not deterministic")
	}
	if _, err := CompileProject(nested, "../demo"); err == nil {
		t.Fatal("escaping project root accepted")
	}
}

func TestModuleContentRejectsInvalidParticlesInputAndPhysics(t *testing.T) {
	for _, test := range []struct{ name, mod, scene, want string }{
		{"emitter count", `{"components":{"emitter":{"count":0,"lifetime":1}}}`, "", "emitter requires"},
		{"emitter speed", `{"components":{"emitter":{"count":10,"lifetime":1,"speedMin":2,"speedMax":1}}}`, "", "ordered speed"},
		{"emitter alpha", `{"components":{"emitter":{"count":10,"lifetime":1,"alphaEnd":2}}}`, "", "alpha between"},
		{"emitter missing particle", `{"components":{"emitter":{"count":10,"lifetime":1}}}`, "", "requires a particle element"},
		{"emitter unknown particle", `{"components":{"emitter":{"particles":["ghost"],"count":10,"lifetime":1}}}`, "", "unknown particle element"},
		{"emitter second unknown particle", `{"elements":[{"id":"spark","components":{"particle":{"color":"#ffffff"}}}],"components":{"emitter":{"particles":["spark","ghost"],"count":10,"lifetime":1}}}`, "", "unknown particle element"},
		{"emitter element without particle", `{"elements":[{"id":"plain"}],"components":{"emitter":{"particles":["plain"],"count":10,"lifetime":1}}}`, "", "has no particle component"},
		{"emitter duplicate particle", `{"elements":[{"id":"spark","components":{"particle":{"color":"#ffffff"}}}],"components":{"emitter":{"particles":["spark","spark"],"count":10,"lifetime":1}}}`, "", "repeats particle element"},
		{"emitter slot instead of element", `{"slots":{"spark":{"type":"string","default":"spark"}},"components":{"emitter":{"particles":["spark"],"count":10,"lifetime":1}}}`, "", "unknown particle element"},
		{"orphan collider", `{"space":"world","components":{"circleCollider2D":{"radius":2}}}`, "", "same element"},
		{"mixed bodies", `{"space":"world","components":{"rigidBody2D":{"type":"dynamic"},"rigidBody3D":{"type":"dynamic"}}}`, "", "both 2D and 3D"},
		{"collision body", `{"components":{"collision":{"emit":"hit"}}}`, "", "exactly one rigid body"},
		{"private scale", `{"space":"world","elements":[{"id":"parent","transform":{"scale":[2,2,2]}},{"id":"body","parent":"parent","components":{"rigidBody3D":{"type":"dynamic"}}}]}`, "", "unit scale"},
		{"scene scale", `{"space":"world","components":{"rigidBody2D":{"type":"dynamic"}}}`, `{"transform":{"scale":[2,2,2]},"modules":[{"id":"a","mod":"mod"}]}`, "unit scale"},
		{"placement scale", `{"space":"world","components":{"rigidBody3D":{"type":"dynamic"}}}`, `{"modules":[{"id":"a","mod":"mod","placement":{"scale":[1,2,1]}}]}`, "unit scale"},
		{"2D tilt", `{"space":"world","transform":{"rotation":[0.2,0,0]},"components":{"rigidBody2D":{"type":"dynamic"}}}`, "", "XY plane"},
	} {
		t.Run(test.name, func(t *testing.T) {
			scene := test.scene
			if scene == "" {
				scene = `{"modules":[{"id":"a","mod":"mod"}]}`
			}
			files := fstest.MapFS{"resources/project.json": {Data: []byte(`{"entry":"main.json","scenes":["main.json"]}`)}, "resources/scenes/main.json": {Data: []byte(scene)}, "mod/mod.json": {Data: []byte(test.mod)}}
			if _, err := CompileProject(files, "."); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("wanted %q, got %v", test.want, err)
			}
		})
	}
}

func TestModuleSystemRejectsRemovedMessageConfiguration(t *testing.T) {
	for _, test := range []struct{ file, data, field string }{
		{"empty/mod.json", `{"events":{}}`, "events"},
		{"resources/scenes/main.json", `{"modules":[],"events":{}}`, "events"},
		{"resources/scenes/main.json", `{"modules":[],"routes":[]}`, "routes"},
		{"empty/mod.json", `{"components":{"input":{"pointer":true,"emit":"clicked"}}}`, "emit"},
	} {
		t.Run(test.file+"/"+test.field, func(t *testing.T) {
			source := moduleSystemSource()
			source[test.file].Data = []byte(test.data)
			if _, err := CompileProject(source, "."); err == nil || !strings.Contains(err.Error(), `unknown field "`+test.field+`"`) {
				t.Fatal("removed configuration accepted", err)
			}
		})
	}
}
