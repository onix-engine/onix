package project

import (
	"testing"
	"testing/fstest"
)

func TestEngineConfigValidate(t *testing.T) {
	if err := (EngineConfig{Features: []RuntimeFeatureID{FeatureRender2D, FeatureUIWidgets, FeatureInput2D}}).Validate(); err != nil {
		t.Fatalf("validate supported features: %v", err)
	}
	if err := (EngineConfig{Features: []RuntimeFeatureID{FeatureRender2D, FeatureInput2D, FeatureUIWidgets, FeatureUIHTML}}).Validate(); err != nil {
		t.Fatalf("validate HTML feature: %v", err)
	}
	if err := (EngineConfig{Features: []RuntimeFeatureID{FeaturePhysics3D}}).Validate(); err != nil {
		t.Fatalf("validate 3D physics feature: %v", err)
	}
	if err := (EngineConfig{Features: []RuntimeFeatureID{FeatureWebView}}).Validate(); err != nil {
		t.Fatalf("validate WebView feature: %v", err)
	}
	for _, features := range [][]RuntimeFeatureID{{"unknown"}, {FeatureRender2D, FeatureRender2D}, {FeatureParticles2D}, {FeatureRender2D, FeatureInput2D, FeatureUIHTML}} {
		if err := (EngineConfig{Features: features}).Validate(); err == nil {
			t.Fatalf("invalid features %v were accepted", features)
		}
	}
}

func TestEngineConfigImageDecoders(t *testing.T) {
	for _, decoders := range [][]ImageDecoderID{{ImageDecoderJPEG, ImageDecoderPNG}, {"bmp"}, {ImageDecoderPNG, ImageDecoderPNG}} {
		err := (EngineConfig{ImageDecoders: decoders}).Validate()
		if len(decoders) == 2 && decoders[0] == ImageDecoderJPEG && err != nil {
			t.Fatalf("valid image decoders rejected: %v", err)
		}
		if len(decoders) != 2 || decoders[0] != ImageDecoderJPEG {
			if err == nil {
				t.Fatalf("invalid image decoders %v were accepted", decoders)
			}
		}
	}
}

func TestReadEngineConfigTextureCompressionDefaults(t *testing.T) {
	for _, testCase := range []struct {
		name string
		data []byte
		want bool
	}{
		{name: "engine omitted", data: []byte(`{}`), want: false},
		{name: "engine defaults on", data: []byte(`{"engine":{"features":["render2d"]}}`), want: true},
		{name: "explicitly off", data: []byte(`{"engine":{"textureCompression":false}}`), want: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			config, err := ReadEngineConfig(fstest.MapFS{
				"resources/project.json": {Data: testCase.data},
			}, ".")
			if err != nil {
				t.Fatal(err)
			}
			if config.TextureCompression != testCase.want {
				t.Fatalf("TextureCompression = %v, want %v", config.TextureCompression, testCase.want)
			}
		})
	}
}
