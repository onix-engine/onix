package onix

import (
	"sync"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/physics"
	"github.com/diiyw/onix/scene"
	engineui "github.com/diiyw/onix/ui"
	"github.com/diiyw/onix/ui/html"
)

type appIdleState struct {
	sync.Mutex
	enabled       bool
	deadline      time.Time
	caretDeadline time.Time
	version       uint64
	started       bool
}

// SetIdleEnabled allows an on-demand app to suspend its schedules between
// events. Custom time-driven systems must schedule WakeAt or disable idle.
func (app *App) SetIdleEnabled(enabled bool) {
	app.idle.Lock()
	app.idle.enabled = enabled
	app.idle.Unlock()
	app.RequestRender()
}

// WakeAt schedules one update at the earliest requested deadline. It can be
// called from another goroutine; ECS writes must stay on the app's loop thread.
func (app *App) WakeAt(deadline time.Time) {
	if deadline.IsZero() {
		app.RequestRender()
		return
	}
	app.idle.Lock()
	if app.idle.deadline.IsZero() || deadline.Before(app.idle.deadline) {
		app.idle.deadline = deadline
	}
	app.idle.Unlock()
	app.wakeLoop()
}

func (app *App) wakeLoop() {
	if context := app.wakeContext.Load(); context != nil {
		context.Wake()
	}
}

func (app *App) idleEligible() bool {
	if !app.idleEnabled() {
		return false
	}
	if app.moduleRuntime != nil {
		app.moduleRuntime.SyncScopes()
	}
	return (!app.hasSystemSet(systemSetAnimations2D) || scene.AnimationTaskCount(app.World) == 0) &&
		(!app.hasSystemSet(systemSetSpriteTrails2D) || scene.SpriteTrailTaskCount(app.World) == 0) &&
		(!app.hasSystemSet(systemSetParticles2D) || scene.ParticleTaskCount(app.World) == 0) &&
		(!app.hasSystemSet(systemSetSpine2D) || scene.SpineTaskCount(app.World) == 0) &&
		(!app.hasSystemSet(systemSetPhysics2D) || physics.TaskCount2D(app.World) == 0) &&
		(!app.hasSystemSet(systemSetPhysics3D) || physics.TaskCount3D(app.World) == 0) &&
		(!app.hasSystemSet(systemSetWebView) || scene.WebViewTaskCount(app.World) == 0) &&
		(app.moduleRuntime == nil || !app.moduleRuntime.NeedsTick()) &&
		(!app.hasSystemSet(systemSetUIRuntime) || !engineui.LayoutPending(app.World)) &&
		(!app.hasSystemSet(systemSetUIHTML) || !html.NeedsFrame(app.World))
}

func (app *App) idleEnabled() bool {
	app.idle.Lock()
	enabled := app.idle.enabled
	app.idle.Unlock()
	return enabled && app.renderMode == RenderOnDemand
}

func (app *App) pendingFrame() bool {
	request, _ := ecs.GetResource[*scene.RenderRequest](app.World)
	state, _ := ecs.GetResource[*input.State](app.World)
	return app.renderRequested.Load() || request.Pending() || state.HasPendingEvents() ||
		(app.Context != nil && app.Context.FrameRequested()) || app.World.MutationVersion() != app.idle.version
}

func (app *App) shouldIdle(now time.Time) bool {
	if !app.idleEligible() || !app.idle.started || app.pendingFrame() {
		return false
	}
	app.idle.Lock()
	deadline := app.nextIdleDeadline()
	app.idle.Unlock()
	return deadline.IsZero() || now.Before(deadline)
}

func (app *App) takeWakeDeadline(now time.Time) {
	app.idle.Lock()
	if !app.idle.deadline.IsZero() && !now.Before(app.idle.deadline) {
		app.idle.deadline = time.Time{}
		app.renderRequested.Store(true)
	}
	app.idle.Unlock()
}

func (app *App) recordIdleFrame() {
	if app.idleEnabled() && app.hasSystemSet(systemSetUIRuntime) {
		deadline := time.Time{}
		if delay, blinking := engineui.CaretBlinkDelay(app.World); blinking {
			deadline = app.currentTime().Add(delay)
		}
		app.idle.Lock()
		app.idle.caretDeadline = deadline
		app.idle.Unlock()
	}
	app.idle.started = true
	app.idle.version = app.World.MutationVersion()
}

func (app *App) idleDeadline() (time.Time, bool) {
	if !app.idleEligible() || !app.idle.started || app.pendingFrame() {
		return time.Time{}, false
	}
	app.idle.Lock()
	deadline := app.nextIdleDeadline()
	app.idle.Unlock()
	return deadline, true
}

// Caller holds the idle mutex; caret deadlines belong to the loop thread.
func (app *App) nextIdleDeadline() time.Time {
	deadline := app.idle.deadline
	if caret := app.idle.caretDeadline; !caret.IsZero() && (deadline.IsZero() || caret.Before(deadline)) {
		deadline = caret
	}
	return deadline
}
