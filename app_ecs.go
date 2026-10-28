package onix

import (
	"errors"
	"fmt"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/scene"
)

type Stage string

const (
	StageStartup               Stage = "startup"
	StageInputPreprocess       Stage = "input_preprocess"
	StageInputGlobal           Stage = "input_global"
	StageInputGUI              Stage = "input_gui"
	StageInputShortcut         Stage = "input_shortcut"
	StageInputUnhandled        Stage = "input_unhandled"
	StagePreUpdate             Stage = "pre_update"
	StageBeforeFixedUpdateLoop Stage = "before_fixed_update_loop"
	StageFixedFirst            Stage = "fixed_first"
	StageModuleEvents          Stage = "module_events"
	StageFixedPreUpdate        Stage = "fixed_pre_update"
	StageFixedUpdate           Stage = "fixed_update"
	StageFixedPostUpdate       Stage = "fixed_post_update"
	StageFixedLast             Stage = "fixed_last"
	StageAfterFixedUpdateLoop  Stage = "after_fixed_update_loop"
	StageUpdate                Stage = "update"
	StagePostUpdate            Stage = "post_update"
	StageRender                Stage = "render"
)

var (
	allStages = [...]Stage{
		StageStartup, StageInputPreprocess, StageInputGlobal, StageInputGUI, StageInputShortcut, StageInputUnhandled,
		StagePreUpdate, StageBeforeFixedUpdateLoop, StageFixedFirst, StageModuleEvents, StageFixedPreUpdate, StageFixedUpdate,
		StageFixedPostUpdate, StageFixedLast, StageAfterFixedUpdateLoop, StageUpdate, StagePostUpdate, StageRender,
	}
	preFixedFrameStages  = [...]Stage{StageInputPreprocess, StageInputGlobal, StageInputGUI, StageInputShortcut, StageInputUnhandled, StagePreUpdate, StageBeforeFixedUpdateLoop}
	fixedMainStages      = [...]Stage{StageFixedFirst, StageModuleEvents, StageFixedPreUpdate, StageFixedUpdate, StageFixedPostUpdate, StageFixedLast}
	postFixedFrameStages = [...]Stage{StageAfterFixedUpdateLoop, StageUpdate, StagePostUpdate}
)

func (app *App) AddSystem(stage Stage, config ecs.SystemConfig) error {
	if err := app.schedule(stage).Add(config); err != nil {
		return err
	}
	app.RequestRender()
	return nil
}

func (app *App) AddSystems(stage Stage, configs ...ecs.SystemConfig) error {
	for _, config := range configs {
		if err := app.AddSystem(stage, config); err != nil {
			return err
		}
	}
	return nil
}

func (app *App) Schedule(stage Stage) *ecs.Schedule {
	return app.schedule(stage)
}

func (app *App) Run() error {
	if app.Context == nil {
		return fmt.Errorf("window context is nil; call SetWindowSize before Run")
	}
	if appRunOwnsLifecycle() {
		defer app.Destroy()
	}
	app.wakeContext.Store(app.Context)
	app.Context.ActivateRender()

	app.bindInputEvents()
	app.ensureECSResources()
	app.syncViewportState()
	app.Context.SetIdleDeadline(app.idleDeadline)
	return app.LoopGame(app.Step)
}

func (app *App) Frame() error {
	_, err := app.Step()
	return err
}

func (app *App) Step() (bool, error) {
	if app == nil {
		return false, nil
	}
	if app.Context != nil {
		app.Context.ActivateRender()
		vw, vh := app.Context.GetViewport()
		graphics.BindFrame(app.World, app.Context.Canvas2D(vw, vh))
		if app.Context.TakeFrameRequest() {
			app.renderRequested.Store(true)
		}
	}
	app.ensureECSResources()
	if err := app.runStartupSchedule(); err != nil {
		return false, err
	}
	app.syncViewportState()
	now := app.currentTime()
	if app.shouldIdle(now) {
		return false, nil
	}
	if app.idleEnabled() {
		app.renderRequested.Store(true)
	}
	app.takeWakeDeadline(now)
	if err := app.runMainFrame(now); err != nil {
		return false, err
	}
	if request, ok := ecs.GetResource[*scene.RenderRequest](app.World); ok && request.Take() {
		app.renderRequested.Store(true)
	}
	defer app.recordIdleFrame()
	renders, first := app.renderPacer.steps(now)
	if renders == 0 {
		return false, nil
	}
	if app.renderMode == RenderOnDemand && !first && !app.renderRequested.Load() {
		return false, nil
	}
	app.renderRequested.Swap(false)
	if err := app.renderFrame(); err != nil {
		app.renderRequested.Store(true)
		return false, err
	}
	return true, nil
}

func (app *App) runMainFrame(now time.Time) error {
	clock, ok := ecs.GetResource[*scene.Time](app.World)
	if !ok || clock == nil {
		return nil
	}
	clock.Advance(now)
	frameTime := *clock
	fixedTime := app.ensureFixedTime()

	if err := app.runStages(preFixedFrameStages[:]); err != nil {
		return err
	}
	fixedTime.Accumulate(frameTime.Delta, DefaultMaxFrameDelta)
	for fixedTime.Expend() {
		clock.Set(fixedTime.Delta, fixedTime.Elapsed, fixedTime.Frame)
		if err := app.runFixedMain(); err != nil {
			return err
		}
	}
	*clock = frameTime
	if err := app.runStages(postFixedFrameStages[:]); err != nil {
		return err
	}
	if state := app.ecsInput(); state != nil {
		if app.Context != nil {
			app.Context.SetTextInputConfig(state.TextInputConfig())
		}
		state.EndFrame()
	}
	ecs.AdvanceMessages(app.World)
	return nil
}

func (app *App) runFixedMain() error {
	return app.runStages(fixedMainStages[:])
}

func (app *App) runStages(stages []Stage) error {
	for _, stage := range stages {
		if app.moduleRuntime != nil {
			app.moduleRuntime.SyncScopes()
		}
		if err := app.schedule(stage).Run(app.World); err != nil {
			return err
		}
	}
	return nil
}

func (app *App) renderFrame() error {
	if app.moduleRuntime != nil {
		app.moduleRuntime.SyncScopes()
		previous := app.World.SetQueryScope(app.moduleRuntime.RenderScope())
		defer app.World.SetQueryScope(previous)
	}
	graphics.DeviceFor(app.World).ResetStatistics()
	if ctx := graphics.CanvasFor(app.World); ctx != nil {
		ctx.ResetViewMatrix()
		ctx.ResetProjectionMatrix()
	}
	renderErr := app.schedule(StageRender).Run(app.World)
	if ctx := graphics.CanvasFor(app.World); ctx != nil {
		return errors.Join(renderErr, ctx.FinishFrame())
	}
	return renderErr
}

func (app *App) currentTime() time.Time {
	if app == nil || app.now == nil {
		return time.Now()
	}
	return app.now()
}

func (app *App) schedule(stage Stage) *ecs.Schedule {
	if app == nil {
		return ecs.NewSchedule(string(stage))
	}
	if app.schedules == nil {
		app.schedules = newScheduleMap()
	}
	schedule := app.schedules[stage]
	if schedule == nil {
		schedule = ecs.NewSchedule(string(stage))
		app.schedules[stage] = schedule
	}
	return schedule
}

func (app *App) ensureECSResources() {
	if app == nil {
		return
	}
	if app.World == nil {
		app.World = ecs.NewWorld()
	}
	app.ensureInputSystem()
	if _, ok := ecs.GetResource[*input.State](app.World); !ok {
		ecs.InsertResource(app.World, input.NewState())
	}
	if state, ok := ecs.GetResource[*input.State](app.World); ok && state != nil && app.Context != nil {
		if app.clipboardContext != app.Context || app.clipboardState != state {
			state.SetClipboard(app.Context.SetClipboardText, app.Context.ClipboardText)
			app.clipboardContext = app.Context
			app.clipboardState = state
		}
	}
	if _, ok := ecs.GetResource[*scene.Time](app.World); !ok {
		ecs.InsertResource(app.World, scene.NewTime())
	}
	if _, ok := ecs.GetResource[*scene.RenderRequest](app.World); !ok {
		ecs.InsertResource(app.World, scene.NewRenderRequest(app.wakeLoop))
	}
	if fixedTime, ok := ecs.GetResource[*scene.FixedTime](app.World); ok && fixedTime != nil {
		app.fixedTime = fixedTime
		return
	}
	ecs.InsertResource(app.World, app.ensureFixedTime())
}

func (app *App) ensureInputSystem() {
	if app == nil || app.inputSystemInstalled {
		return
	}
	if err := app.schedule(StageInputPreprocess).Add(ecs.SystemConfig{
		Name:   "process_input_events",
		System: app.processInputEvents,
	}); err == nil {
		app.inputSystemInstalled = true
	}
}

func processInputEvents(context *ecs.SystemContext) error {
	return processInputEventsWithHandlers(context, nil)
}

func (app *App) processInputEvents(context *ecs.SystemContext) error {
	if app == nil {
		return processInputEvents(context)
	}
	return processInputEventsWithHandlers(context, app.inputHandlers)
}

func processInputEventsWithHandlers(context *ecs.SystemContext, handlers map[input.Phase][]func(*input.Dispatch)) error {
	if context == nil || context.World == nil {
		return nil
	}
	state, ok := ecs.GetResource[*input.State](context.World)
	if !ok || state == nil {
		return nil
	}
	for _, event := range state.DrainEvents() {
		state.ApplyEvent(event)
		ecs.WriteMessage(context.World, event)
		dispatch := input.Dispatch{Event: event}
		for _, phase := range []input.Phase{input.PhaseGlobal, input.PhaseGUI, input.PhaseShortcut, input.PhaseUnhandled} {
			dispatch.Phase = phase
			for _, handler := range handlers[phase] {
				handler(&dispatch)
				if dispatch.Handled {
					state.ConsumeEvent(event.Seq)
					break
				}
			}
			if dispatch.Handled {
				break
			}
		}
		if inputEvent, ok := scene.InputEventFromPlatform(event); ok {
			inputEvent.Handled = dispatch.Handled
			ecs.WriteMessage(context.World, inputEvent)
		}
	}
	return nil
}

func (app *App) ensureFixedTime() *scene.FixedTime {
	if app.fixedTime == nil {
		app.fixedTime = scene.NewFixedTime(time.Second / time.Duration(DefaultTargetTPS))
	}
	return app.fixedTime
}

func (app *App) runStartupSchedule() error {
	if app == nil || app.startupRan {
		return nil
	}
	if err := app.schedule(StageStartup).Run(app.World); err != nil {
		return err
	}
	app.startupRan = true
	return nil
}

func (app *App) syncViewportState() {
	if app == nil || app.Context == nil {
		return
	}
	vw, vh := app.GetViewport()
	if app.contextWidth != app.Context.Width || app.contextHeight != app.Context.Height || app.viewportWidth != vw || app.viewportHeight != vh {
		app.contextWidth = app.Context.Width
		app.contextHeight = app.Context.Height
		app.viewportWidth = vw
		app.viewportHeight = vh
		app.RequestRender()
	}
	canvas := app.Context.Canvas2D(vw, vh)
	graphics.BindFrame(app.World, canvas)
	rasterScale := 1.0
	if vw > 0 && vh > 0 {
		rasterScale = min(float64(app.Context.Width)/float64(vw), float64(app.Context.Height)/float64(vh))
	}
	deviceScale := min(app.Context.DevicePixelRatio(), app.windowOptions.MaxDevicePixelRatio) * app.windowOptions.RenderScale
	if rasterScale < deviceScale {
		rasterScale = deviceScale
	}
	if rasterScale <= 0 {
		rasterScale = 1
	}
	canvas.SetRasterScale(rasterScale)
	viewport := scene.ViewportSize{Width: float32(vw), Height: float32(vh), ScaleFactor: 1}
	changed := false
	if previous, ok := ecs.GetResource[scene.ViewportSize](app.World); !ok || previous != viewport {
		ecs.InsertResource(app.World, viewport)
		changed = true
	}
	if changed || app.cameraVersion != ecs.ComponentVersion[scene.Camera](app.World) {
		for _, row := range ecs.Query[scene.Camera](app.World).All {
			access := row

			camera := *access.Ref()
			camera.UpdateViewport(vw, vh)
			if camera != *access.Ref() {
				*access.Mut() = camera
			}
		}
		app.cameraVersion = ecs.ComponentVersion[scene.Camera](app.World)
	}
}

func (app *App) ecsInput() *input.State {
	if app == nil {
		return nil
	}
	app.ensureECSResources()
	state, _ := ecs.GetResource[*input.State](app.World)
	return state
}

func newScheduleMap() map[Stage]*ecs.Schedule {
	schedules := make(map[Stage]*ecs.Schedule, len(allStages))
	for _, stage := range allStages {
		schedules[stage] = ecs.NewSchedule(string(stage))
	}
	return schedules
}
