package onix

import (
	"fmt"

	"github.com/diiyw/onix/project"
)

func (app *App) LoadScene(bundle *project.Bundle) (*project.SceneInstance, error) {
	if app == nil {
		return nil, fmt.Errorf("app is nil")
	}
	if bundle == nil {
		return nil, fmt.Errorf("project is nil")
	}
	if err := bundle.ValidateSceneFeatures(bundle.EntrySceneID(), app.hasProjectFeature); err != nil {
		return nil, err
	}
	if err := app.applyProjectConfig(bundle); err != nil {
		return nil, err
	}
	app.ensureECSResources()
	scene, err := bundle.InstantiateEntryScene(app.World)
	if err != nil {
		return nil, err
	}
	app.scriptScene = scene
	if !app.scriptRegistry.Empty() {
		if err := app.SetScriptRegistry(app.scriptRegistry); err != nil {
			scene.Dispose()
			app.scriptScene = nil
			return nil, err
		}
	}
	return scene, nil
}
