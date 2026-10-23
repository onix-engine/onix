package project

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/diiyw/onix/ecs"
	sceneapi "github.com/diiyw/onix/scene"
)

func TestConfiguredProjectInstantiatesStaticScene(t *testing.T) {
	source := fstest.MapFS{
		"resources/project.json":           {Data: []byte(`{}`)},
		"resources/resources.json":         {Data: []byte(`{}`)},
		"resources/scenes.json":            {Data: []byte(`{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)},
		"resources/scenes/main.scene.json": {Data: []byte(`{"nodes":[{"id":"background","components":{"sprite":{"texture":{"color":"#ffffff"}}},"width":32,"height":32},{"id":"label","components":{"text":{"value":"ready"}},"coordinateSpace":"ui"}]}`)},
	}

	bundle, err := LoadConfiguredWith[ResourceFeature](source, ".", EngineConfig{
		Features:  []RuntimeFeatureID{FeatureRender2D},
		Resources: []ResourceID{ResourceImages, ResourceFonts},
	})
	if err != nil {
		t.Fatalf("load configured project: %v", err)
	}
	defer bundle.Dispose()

	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatalf("instantiate static scene: %v", err)
	}
	if !ecs.Has[sceneapi.Sprite2D](scene.World, scene.MustEntity("background")) {
		t.Fatal("static sprite was not instantiated")
	}
	if !ecs.Has[sceneapi.Text2D](scene.World, scene.MustEntity("label")) {
		t.Fatal("static text was not instantiated")
	}
}

func TestConfiguredProjectRejectsDisabledResources(t *testing.T) {
	source := fstest.MapFS{
		"resources/project.json":           {Data: []byte(`{}`)},
		"resources/resources.json":         {Data: []byte(`{"audio":[{"id":"sound","path":"sound.wav"}]}`)},
		"resources/scenes.json":            {Data: []byte(`{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)},
		"resources/scenes/main.scene.json": {Data: []byte(`{"nodes":[]}`)},
	}

	if _, err := LoadConfiguredWith[ResourceFeature](source, ".", EngineConfig{
		Features:  []RuntimeFeatureID{FeatureRender2D},
		Resources: []ResourceID{ResourceImages, ResourceFonts},
	}); err == nil {
		t.Fatal("configured project accepted an audio resource")
	}
}

func TestBundleValidatesRequiredSceneFeatures(t *testing.T) {
	source := fstest.MapFS{
		"resources/project.json":           {Data: []byte(`{}`)},
		"resources/resources.json":         {Data: []byte(`{}`)},
		"resources/scenes.json":            {Data: []byte(`{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)},
		"resources/scenes/main.scene.json": {Data: []byte(`{"nodes":[{"id":"sprite","components":{"sprite":{"texture":{"color":"#ffffff"}}}}]}`)},
	}
	bundle, err := LoadConfiguredWith[ResourceFeature](source, ".", EngineConfig{
		Features:  []RuntimeFeatureID{FeatureRender2D},
		Resources: []ResourceID{ResourceImages, ResourceFonts},
	})
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	defer bundle.Dispose()
	if err := bundle.ValidateSceneFeatures(bundle.EntrySceneID(), func(RuntimeFeatureID) bool { return false }); err == nil {
		t.Fatal("missing render system was accepted")
	}
	if err := bundle.ValidateSceneFeatures(bundle.EntrySceneID(), func(id RuntimeFeatureID) bool { return id == FeatureRender2D }); err != nil {
		t.Fatalf("render system was rejected: %v", err)
	}
}

func TestBundleValidatesLayoutWidgetRuntimeFeature(t *testing.T) {
	source := fstest.MapFS{
		"resources/project.json":           {Data: []byte(`{}`)},
		"resources/resources.json":         {Data: []byte(`{}`)},
		"resources/scenes.json":            {Data: []byte(`{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)},
		"resources/scenes/main.scene.json": {Data: []byte(`{"nodes":[{"id":"panel","coordinateSpace":"ui","layout":{}}]}`)},
	}
	bundle, err := LoadConfiguredWith[ResourceFeature](source, ".", EngineConfig{
		Features:  []RuntimeFeatureID{FeatureRender2D},
		Resources: []ResourceID{ResourceImages, ResourceFonts},
	})
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	defer bundle.Dispose()
	if err := bundle.ValidateSceneFeatures(bundle.EntrySceneID(), func(id RuntimeFeatureID) bool {
		return id == FeatureRender2D
	}); err == nil || !strings.Contains(err.Error(), `requires runtime feature "ui-widgets"`) {
		t.Fatalf("missing UI Widgets system was accepted: %v", err)
	}
}

func TestLoadRejectsLayoutWidgetOnWorldNode(t *testing.T) {
	source := fstest.MapFS{
		"resources/project.json":           {Data: []byte(`{}`)},
		"resources/resources.json":         {Data: []byte(`{}`)},
		"resources/scenes.json":            {Data: []byte(`{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)},
		"resources/scenes/main.scene.json": {Data: []byte(`{"nodes":[{"id":"panel","layout":{}}]}`)},
	}
	_, err := LoadConfiguredWith[ResourceFeature](source, ".", EngineConfig{
		Features:  []RuntimeFeatureID{FeatureRender2D, FeatureUIWidgets, FeatureInput2D},
		Resources: []ResourceID{ResourceImages, ResourceFonts},
	})
	if err == nil || !strings.Contains(err.Error(), `layout requires UI coordinate space`) {
		t.Fatalf("world-space layout was accepted: %v", err)
	}
}

func TestProjectManifestValidatesEngineFeatures(t *testing.T) {
	source := fstest.MapFS{
		"resources/project.json": {Data: []byte(`{"engine":{"features":["unknown"]}}`)},
	}
	if _, err := LoadConfiguredWith[ResourceFeature](source, ".", EngineConfig{
		Features: []RuntimeFeatureID{FeatureRender2D},
	}); err == nil {
		t.Fatal("project manifest accepted an unknown engine feature")
	}
}

func TestProjectManifestAcceptsConfiguredEngineFeatures(t *testing.T) {
	source := fstest.MapFS{
		"resources/project.json": {Data: []byte(`{"engine":{"features":["render2d"]}}`)},
	}
	if _, _, err := loadGameManifest(source, "."); err != nil {
		t.Fatalf("project manifest rejected configured engine features: %v", err)
	}
}

func TestConfiguredProjectRejectsDisabledResource(t *testing.T) {
	source := fstest.MapFS{
		"resources/project.json":   {Data: []byte(`{"engine":{"features":["render2d"],"resources":["images","fonts"]}}`)},
		"resources/resources.json": {Data: []byte(`{"audio":[{"id":"sound","path":"sound.wav"}]}`)},
	}
	config := EngineConfig{Resources: []ResourceID{ResourceImages, ResourceFonts}, Features: []RuntimeFeatureID{FeatureRender2D}}
	if _, err := LoadConfiguredWith[ResourceFeature](source, ".", config); err == nil {
		t.Fatal("configured project accepted a disabled audio resource")
	}
}

func TestConfiguredProjectBuildsSelectedSceneFeatures(t *testing.T) {
	source := fstest.MapFS{
		"resources/project.json":           {Data: []byte(`{"engine":{"features":["render2d"],"resources":["images","fonts"]}}`)},
		"resources/resources.json":         {Data: []byte(`{}`)},
		"resources/scenes.json":            {Data: []byte(`{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)},
		"resources/scenes/main.scene.json": {Data: []byte(`{"nodes":[{"id":"sprite","components":{"sprite":{"texture":{"color":"#ffffff"}}}}]}`)},
	}
	config := EngineConfig{Resources: []ResourceID{ResourceImages, ResourceFonts}, Features: []RuntimeFeatureID{FeatureRender2D}}
	bundle, err := LoadConfiguredWith[ResourceFeature](source, ".", config)
	if err != nil {
		t.Fatalf("load configured project: %v", err)
	}
	defer bundle.Dispose()
	if _, err := bundle.InstantiateEntryScene(ecs.NewWorld()); err != nil {
		t.Fatalf("instantiate configured scene: %v", err)
	}
}
