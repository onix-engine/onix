package projectaudio

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/diiyw/onix/audio/codec"
	"github.com/diiyw/onix/project"
)

func TestFeatureLoadsAudioLazily(t *testing.T) {
	source := fstest.MapFS{
		"resources/project.json":           {Data: []byte(`{}`)},
		"resources/resources.json":         {Data: []byte(`{"audio":[{"id":"broken","path":"broken.wav"}]}`)},
		"resources/scenes.json":            {Data: []byte(`{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)},
		"resources/scenes/main.scene.json": {Data: []byte(`{"nodes":[]}`)},
		"broken.wav":                       {Data: []byte("not a wav")},
	}

	bundle, err := project.LoadConfiguredWith[Feature[codec.DefaultDecoder]](fs.FS(source), ".", project.EngineConfig{
		Resources: []project.ResourceID{project.ResourceAudio},
	})
	if err != nil {
		t.Fatalf("load bundle: %v", err)
	}
	if _, err := Clip(bundle.Resources(), "broken"); err == nil {
		t.Fatal("invalid WAV decoded successfully")
	}
}
