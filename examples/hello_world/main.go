package main

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/onix-engine/onix"
	"github.com/onix-engine/onix/graphics"
	"github.com/onix-engine/onix/scene"
)

func main() {
	app := onix.NewApp()
	app.SetRenderMode(onix.RenderOnDemand)
	app.World.InsertResource(scene.RenderSettings2D{ClearColor: graphics.Black})
	if err := app.InstallSystems[onix.Render2D](); err != nil {
		panic(err)
	}
	app.SetWindowSize(320, 160)

	text := scene.NewText2D("hello world")
	spawn := app.World.NewSpawner4[scene.Text2D, scene.UIGlobalTransform, scene.Anchor, scene.Visibility]()
	spawn.Spawn(text, scene.UIGlobalTransform{Matrix: mgl32.Translate3D(24, 24, 0)}, scene.AnchorTopLeft, scene.Visibility{Visible: true})

	if err := app.Run(); err != nil {
		panic(err)
	}
}
