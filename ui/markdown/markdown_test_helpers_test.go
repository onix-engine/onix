package markdown

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/scene"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	engineui "github.com/diiyw/onix/ui"
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
