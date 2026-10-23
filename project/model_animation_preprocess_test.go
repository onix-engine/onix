//go:build !onix_release

package project

import (
	"github.com/diiyw/onix/internal/typedbin"
	"testing"
	"testing/fstest"
)

func TestPreprocessExternalModelAnimations(t *testing.T) {
	source := fstest.MapFS{
		"resources/animation.json": {Data: []byte(`[{"name":"move","duration":1,"blendMode":2500,"tracks":[{"name":"root.position","type":"vector","times":[0,1],"values":[0,0,0,2,0,0]}]}]`)},
	}
	// A procedural mesh is enough to exercise packaging of the independent file.
	files, err := preprocessModelResources(source, ".", []ModelSpec{{ID: "model", Mesh: &MeshSpec{Primitive: "cube"}, AnimationPath: "resources/animation.json"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path != "resources/animation.json" || !typedbin.IsEncoded(files[0].Data) {
		t.Fatalf("animation was not packaged: %+v", files)
	}
	source[files[0].Path] = &fstest.MapFile{Data: files[0].Data}
	resources := &Resources{source: source, baseDir: "."}
	clips, err := resources.loadModelAnimations("resources/animation.json", []ModelNode{{Name: "root", Path: "root", Parent: -1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(clips) != 1 || clips[0].Tracks[0].Keyframes[1].Value[0] != 2 {
		t.Fatal("compiled external animation lost keyframes")
	}
}
