package main

import (
	"github.com/diiyw/onix"
	"github.com/diiyw/onix/module"
	"github.com/diiyw/onix/project"
)

// Main explicitly routes keyboard and owned HTML input to the game module.
type Main struct{}

func (*Main) OnMessage(c *module.Context, message module.Message) error {
	if message.Topic == module.InputTopic {
		return c.Forward("game", message)
	}
	return nil
}

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
