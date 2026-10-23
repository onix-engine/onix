package project

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func TestModuleSystemDiscoversSceneDirectoryWithoutRegistration(t *testing.T) {
	source := fstest.MapFS{
		"resources/project.json":            {Data: []byte(`{"entry":"main.json"}`)},
		"resources/scenes/main.json":        {Data: []byte(`{"modules":[]}`)},
		"resources/scenes/game.json":        {Data: []byte(`{"modules":[{"id":"box","mod":"box","children":[{"id":"child","mod":"box"}]}]}`)},
		"resources/scenes/readme.txt":       {Data: []byte("not a scene")},
		"resources/scenes/assets/data.json": {Data: []byte(`{"not":"a scene"}`)},
		"box/mod.json":                      {Data: []byte(`{}`)},
		"ignored.json":                      {Data: []byte(`{"not":"a scene"}`)},
	}
	plan, err := CompileProject(source, ".")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(plan.Spec.Scenes, []SceneID{"game.json", "main.json"}) || plan.Entry != 1 || len(plan.Scenes[0].Instances) != 2 {
		t.Fatalf("directory scenes were not discovered: %+v", plan)
	}
	source["resources/scenes/new.json"] = &fstest.MapFile{Data: []byte(`{"modules":null}`)}
	if _, err := CompileProject(source, "."); err == nil || !strings.Contains(err.Error(), "new.json") {
		t.Fatalf("unregistered invalid scene was ignored: %v", err)
	}
}

func TestModuleSystemRejectsSceneOutsideSceneDirectory(t *testing.T) {
	source := fstest.MapFS{
		"resources/project.json": {Data: []byte(`{"entry":"main.json","scenes":["main.json"]}`)},
		"main.json":              {Data: []byte(`{"modules":[]}`)},
	}
	if _, err := CompileProject(source, "."); err == nil || !strings.Contains(err.Error(), SceneDirectory) {
		t.Fatalf("root scene was accepted: %v", err)
	}
}
