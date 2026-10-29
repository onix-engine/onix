package onix

import (
	"testing"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func TestAppStepSkipsOnDemandRenderUntilRequested(t *testing.T) {
	app := NewApp()
	app.SetRenderMode(RenderOnDemand)
	app.SetTargetFPS(30)

	now := time.Unix(0, 0)
	app.now = func() time.Time {
		return now
	}

	renders := 0
	if err := app.AddSystem(StageRender, ecs.SystemConfig{
		Name: "render",
		System: func(context *ecs.SystemContext) error {
			renders++
			return nil
		},
	}); err != nil {
		t.Fatalf("add render: %v", err)
	}

	rendered, err := app.Step()
	if err != nil {
		t.Fatalf("first step: %v", err)
	}
	if !rendered || renders != 1 {
		t.Fatalf("expected first step to render once, rendered=%v renders=%d", rendered, renders)
	}

	now = now.Add(app.renderPacer.interval)
	rendered, err = app.Step()
	if err != nil {
		t.Fatalf("idle step: %v", err)
	}
	if rendered || renders != 1 {
		t.Fatalf("expected idle on-demand step to skip render, rendered=%v renders=%d", rendered, renders)
	}

	app.RequestRender()
	now = now.Add(app.renderPacer.interval)
	rendered, err = app.Step()
	if err != nil {
		t.Fatalf("requested step: %v", err)
	}
	if !rendered || renders != 2 {
		t.Fatalf("expected requested step to render, rendered=%v renders=%d", rendered, renders)
	}
}

func TestAppStepConsumesWorldRenderRequest(t *testing.T) {
	app := NewApp()
	app.SetRenderMode(RenderOnDemand)
	app.SetTargetFPS(30)

	now := time.Unix(0, 0)
	app.now = func() time.Time { return now }
	renders := 0
	if err := app.AddSystem(StageRender, ecs.SystemConfig{
		Name: "render",
		System: func(context *ecs.SystemContext) error {
			renders++
			return nil
		},
	}); err != nil {
		t.Fatalf("add render: %v", err)
	}

	if _, err := app.Step(); err != nil {
		t.Fatalf("first step: %v", err)
	}
	now = now.Add(app.renderPacer.interval)
	request, _ := app.World.GetResource[*scene.RenderRequest]()
	request.Request()
	rendered, err := app.Step()
	if err != nil {
		t.Fatalf("requested step: %v", err)
	}
	if !rendered || renders != 2 {
		t.Fatalf("expected world request to render, rendered=%v renders=%d", rendered, renders)
	}
}
