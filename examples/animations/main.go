package main

import (
	"github.com/diiyw/onix"
	"github.com/diiyw/onix/project"
)

func main() {
	plan, source, err := project.OpenModuleProject()
	if err != nil {
		panic(err)
	}
	app := onix.NewApp()
	registry, err := OnixRegistry()
	if err != nil {
		panic(err)
	}
	if err := OnixInstallSystems(app); err != nil {
		panic(err)
	}
	if err := OnixOpenWindow(app, plan.Spec, source); err != nil {
		panic(err)
	}
	if _, err := app.LoadModules(plan, registry, OnixModuleOptions(source)); err != nil {
		panic(err)
	}
	if err := app.Run(); err != nil {
		panic(err)
	}
}
