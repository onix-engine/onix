package markdown

import (
	"testing"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/input"
	"github.com/onix-engine/onix/scene"
	notosanssc "github.com/onix-engine/onix/testdata/fonts/Noto_Sans_SC"
	engineui "github.com/onix-engine/onix/ui"
)

func newUIWorld(t *testing.T) *ecs.World {
	t.Helper()
	world := ecs.NewWorld()
	engineui.RegisterRuntimeFeature[engineui.FullRuntime](world)
	world.InsertResource(input.NewState())
	camera := scene.SpawnCamera2D(world)
	component := world.GetMut[scene.Camera](camera)
	component.UpdateViewport(1920, 1080)
	if err := scene.SyncGlobalTransformSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	theme := engineui.DefaultTheme()
	theme.Font = notosanssc.NotoSansSCRegular
	world.InsertResource(theme)
	return world
}
