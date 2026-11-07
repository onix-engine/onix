package main

import (
	"github.com/onix-engine/onix"
	"github.com/onix-engine/onix/project"
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
	if err := OnixInstallSystems(app); err != nil {
		panic(err)
	}
	if err := OnixOpenWindow(app, plan.Spec, source); err != nil {
		panic(err)
	}
	if _, err := app.LoadModules[OnixContentProfile](plan, registry, OnixModuleOptions(source)); err != nil {
		panic(err)
	}
	if err := app.Run(); err != nil {
		panic(err)
	}
}
