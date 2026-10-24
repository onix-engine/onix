//go:build !onix_release

package graphics

import (
	"bytes"
	"reflect"
	"testing"
)

func TestParseAtlasIncludesFrameDimensions(t *testing.T) {
	metadata, err := ParseAtlas([]byte(`{
  "frames": [
    {"filename":"idle","frame":{"x":1,"y":2,"w":37,"h":19}},
    {"filename":"run","frame":{"x":3,"y":4,"w":41,"h":23}}
  ],
  "meta":{"size":{"w":128,"h":64}}
}`))
	if err != nil {
		t.Fatalf("parse metadata: %v", err)
	}
	frame, ok := metadata.Frame("run")
	if !ok || frame.Bounds.W != 41 || frame.Bounds.H != 23 {
		t.Fatalf("unexpected atlas frame: %#v, found=%v", frame, ok)
	}
}

func TestParseAtlasUsesMapKeysAsFrameNames(t *testing.T) {
	metadata, err := ParseAtlas([]byte(`{
  "frames": {"idle":{"frame":{"x":0,"y":0,"w":17,"h":13}}},
  "meta":{"size":{"w":32,"h":32}}
}`))
	if err != nil {
		t.Fatalf("parse metadata: %v", err)
	}
	frame, ok := metadata.Frame("idle")
	if !ok || frame.Bounds.W != 17 || frame.Bounds.H != 13 {
		t.Fatalf("unexpected atlas frame: %#v, found=%v", frame, ok)
	}
}

func TestNewAtlasPreservesTrimmedSourceRect(t *testing.T) {
	texture := CreateTextureByColor(White)
	metadata, err := ParseAtlas([]byte(`{
  "frames": {"idle":{"frame":{"x":0,"y":0,"w":17,"h":13},"sourceSize":{"w":24,"h":20},"spriteSourceSize":{"x":3,"y":4,"w":17,"h":13}}},
  "meta":{"size":{"w":32,"h":32}}
}`))
	if err != nil {
		t.Fatalf("parse metadata: %v", err)
	}
	atlas, err := NewAtlas(texture, metadata)
	if err != nil {
		t.Fatalf("new atlas: %v", err)
	}
	frame := atlas.SubTexture("idle")
	if frame == nil || !frame.Trimmed() || frame.SourceWidth != 24 || frame.SourceHeight != 20 || frame.OffsetX != 3 || frame.OffsetY != 4 {
		t.Fatalf("unexpected trimmed frame: %#v", frame)
	}
	if frame.LogicalWidth() != 24 || frame.LogicalHeight() != 20 {
		t.Fatalf("unexpected logical dimensions: %d x %d", frame.LogicalWidth(), frame.LogicalHeight())
	}
}

func TestNewAtlasPreservesRotatedTrimmedSourceRect(t *testing.T) {
	texture := newTexture(64, 64, false, ResourceUsageCPU, make([]byte, 64*64*4))
	metadata, err := ParseAtlas([]byte(`{
  "frames": {"turn":{"frame":{"x":5,"y":7,"w":17,"h":13},"sourceSize":{"w":24,"h":20},"spriteSourceSize":{"x":3,"y":4,"w":17,"h":13},"rotated":true}},
  "meta":{"size":{"w":64,"h":64}}
}`))
	if err != nil {
		t.Fatalf("parse metadata: %v", err)
	}
	atlas, err := NewAtlas(texture, metadata)
	if err != nil {
		t.Fatalf("new atlas: %v", err)
	}
	frame := atlas.SubTexture("turn")
	if frame == nil || frame.Angle != 90 || frame.Width != 17 || frame.Height != 13 ||
		frame.SourceWidth != 24 || frame.SourceHeight != 20 || frame.OffsetX != 3 || frame.OffsetY != 4 {
		t.Fatalf("unexpected rotated trimmed frame: %#v", frame)
	}
}

func TestParseAtlasRejectsInvalidFrames(t *testing.T) {
	if _, err := ParseAtlas([]byte(`{"frames":true}`)); err == nil {
		t.Fatal("expected invalid frames to fail")
	}
}

func TestAtlasIRRoundTripIsDeterministic(t *testing.T) {
	source := []byte(`{
      "frames":{"turn":{"frame":{"x":5,"y":7,"w":17,"h":13},"sourceSize":{"w":24,"h":20},"spriteSourceSize":{"x":3,"y":4,"w":17,"h":13},"rotated":true}},
      "meta":{"format":"RGBA8888","image":"sheet.png","size":{"w":64,"h":64}}
    }`)
	want, err := ParseAtlas(source)
	if err != nil {
		t.Fatal(err)
	}
	first, atlas, err := PreprocessAtlas(source)
	if err != nil || !atlas {
		t.Fatalf("preprocess atlas: atlas=%t err=%v", atlas, err)
	}
	second, _, err := PreprocessAtlas(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("atlas IR is not deterministic")
	}
	if !IsPreprocessedAtlas(first) {
		t.Fatal("compiled atlas marker is missing")
	}
	got, err := ParseAtlas(first)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded atlas = %#v, want %#v", got, want)
	}
	for length := range len(first) {
		if _, err := ParseAtlas(first[:length]); err == nil {
			t.Fatalf("truncated atlas IR length %d was accepted", length)
		}
	}
}
