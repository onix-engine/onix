package onix

import "testing"

func TestInstallRenderSystemsOrdersRenderWorldBefore2DOverlay(t *testing.T) {
	app := NewApp()
	if err := app.InstallSystems[Render2D](); err != nil {
		t.Fatalf("install 2D systems: %v", err)
	}
	if err := app.InstallSystems[Render3D](); err != nil {
		t.Fatalf("install 3D systems: %v", err)
	}
	order, err := app.Schedule(StageRender).Order()
	if err != nil {
		t.Fatalf("compile render schedule: %v", err)
	}
	positions := make(map[string]int, len(order))
	for index, name := range order {
		positions[name] = index
	}
	want := []string{
		"extract_cameras", "extract_render_3d", "prepare_render_resources_3d",
		"sort_render_phases_3d", "render_cameras",
	}
	for index := 1; index < len(want); index++ {
		if positions[want[index-1]] >= positions[want[index]] {
			t.Fatalf("render order %v does not place %q before %q", order, want[index-1], want[index])
		}
	}
}
