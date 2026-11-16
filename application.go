//go:build cgo && (windows || (darwin && !ios) || (linux && !android && !onix_drm))

package onix

import (
	"fmt"

	"github.com/onix-engine/onix/window"
)

// Application owns multiple native windows in one desktop application instance.
// Add and BetweenFrames run on the UI thread, outside individual window frames.
type Application struct {
	BetweenFrames func() error
	loop          window.EventLoop
}

func (a *Application) Add(app *App, closed func()) error {
	if app == nil || app.Context == nil {
		return fmt.Errorf("window context is nil")
	}
	app.wakeContext.Store(app.Context)
	app.Context.ActivateRender()
	app.bindInputEvents()
	app.ensureECSResources()
	app.syncViewportState()
	app.Context.SetIdleDeadline(app.idleDeadline)
	a.loop.Add(app.Context, app.Step, func() {
		app.Destroy()
		if closed != nil {
			closed()
		}
	})
	return nil
}

func (a *Application) Run() error {
	a.loop.BetweenFrames = a.BetweenFrames
	return a.loop.Run()
}

func (a *Application) Close() { a.loop.Close() }
