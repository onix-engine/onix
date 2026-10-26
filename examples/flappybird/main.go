package main

import (
	"github.com/diiyw/onix"
	"github.com/diiyw/onix/module"
	"github.com/diiyw/onix/project"
)

func main() {
	plan, source, err := project.OpenModuleProject()
	if err != nil {
		panic(err)
	}
	registry, err := OnixRegistry()
	if err != nil {
		panic(err)
	}
	app := onix.NewApp()
	defer app.Destroy()
	if err := OnixInstallSystems(app); err != nil {
		panic(err)
	}

	if err := OnixOpenWindow(app, plan.Spec, source); err != nil {
		panic(err)
	}
	if _, err := app.LoadModules(plan, registry, OnixModuleOptions(source)); err != nil {
		panic(err)
	}
	for _, id := range [...]project.SceneID{"result.json"} {
		if _, err := app.Modules().Manager().Preload(module.ScenePreloadRequest{Scene: id}); err != nil {
			panic(err)
		}
	}
	if err := app.Run(); err != nil {
		panic(err)
	}
}
