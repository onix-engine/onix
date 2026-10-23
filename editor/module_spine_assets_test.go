package editor

import (
	"testing"

	"github.com/diiyw/onix/project"
)

func TestModuleBuildSpineCollectsDeclaredFilesWithoutExportMetadata(t *testing.T) {
	directory := t.TempDir()
	moduleBuildFile(t, directory, "resources/project.json", `{"entry":"main.json","scenes":["main.json"]}`)
	moduleBuildFile(t, directory, "resources/scenes/main.json", `{"modules":[{"id":"actor","mod":"actor"}]}`)
	moduleBuildFile(t, directory, "actor/mod.json", `{"components":{"spine":{"source":{"id":"actor","skeleton":"exports/actor.json","atlas":"exports/actor.atlas","texture":"exports/actor.png"}}}}`)
	moduleBuildFile(t, directory, "actor/exports/actor.json", `{"skeleton":{"spine":"3.7.90","images":"../authoring/images/","audio":"C:/Spine/examples"},"slots":[{"name":"skin/head.png"}]}`)
	moduleBuildFile(t, directory, "actor/exports/actor.atlas", "actor.png\nsize: 32,32\nregion/head.png\n  size: 32,32\n")
	moduleBuildFile(t, directory, "actor/exports/actor.png", "texture")
	plan, err := project.CompileProjectDirectory(directory)
	if err != nil {
		t.Fatal(err)
	}
	files, err := collectModuleAssets(directory, plan, resourcePackageConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatal("unexpected export dependencies", files)
	}
	for _, name := range []string{"actor/exports/actor.json", "actor/exports/actor.atlas", "actor/exports/actor.png"} {
		if _, ok := files[name]; !ok {
			t.Fatal("missing declared dependency", name)
		}
	}
}
