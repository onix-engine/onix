//go:build onix_release

package spine

import (
	"strings"
	"testing"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/internal/typedbin"
)

func TestReleaseRuntimeRequiresPreprocessedSpine(t *testing.T) {
	texture := graphics.CreateTextureByColor(graphics.White)
	defer texture.Dispose()
	compiled, err := typedbin.Marshal(compiledResource{
		Bones:    []compiledBone{{Name: "root", Parent: -1, ScaleX: 1, ScaleY: 1}},
		Children: [][]int{nil},
	})
	if err != nil {
		t.Fatal(err)
	}
	resource, err := Load(compiled, nil, texture)
	if err != nil || len(resource.bones) != 1 {
		t.Fatalf("load release Spine IR: resource=%#v err=%v", resource, err)
	}
	_, err = Load([]byte(`{"bones":[{"name":"root"}]}`), nil, texture)
	if err == nil || !strings.Contains(err.Error(), "require preprocessed Spine IR") {
		t.Fatalf("release Spine JSON rejection error = %v", err)
	}
}
