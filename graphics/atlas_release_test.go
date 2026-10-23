//go:build onix_release

package graphics

import (
	"strings"
	"testing"
)

func TestReleaseRuntimeRequiresPreprocessedAtlas(t *testing.T) {
	data, err := MarshalAtlas(&AtlasMetadata{
		Meta:   AtlasMeta{Image: "sheet.png", Size: AtlasSize{W: 32, H: 16}},
		Frames: []AtlasFrame{{Filename: "idle", Bounds: AtlasBounds{W: 8, H: 8}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := ParseAtlas(data)
	if err != nil || metadata.Meta.Image != "sheet.png" || len(metadata.Frames) != 1 {
		t.Fatalf("parse release atlas IR: metadata=%#v err=%v", metadata, err)
	}
	_, err = ParseAtlas([]byte(`{"frames":{},"meta":{}}`))
	if err == nil || !strings.Contains(err.Error(), "require preprocessed atlas IR") {
		t.Fatalf("release Atlas JSON rejection error = %v", err)
	}
}
