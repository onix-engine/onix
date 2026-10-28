package onix

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/module"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/scene/render3d"
	"github.com/diiyw/onix/ui/html"
	"github.com/diiyw/onix/window"
)

type App struct {
	World *ecs.World
	*window.Context
	schedules                         map[Stage]*ecs.Schedule
	renderPacer                       ratePacer
	renderMode                        RenderMode
	renderRequested                   atomic.Bool
	idle                              appIdleState
	wakeContext                       atomic.Pointer[window.Context]
	fixedTime                         *scene.FixedTime
	now                               func() time.Time
	contextWidth                      int
	contextHeight                     int
	viewportWidth                     int
	viewportHeight                    int
	cameraVersion                     uint64
	initialWindowWidth                int
	initialWindowHeight               int
	startupRan                        bool
	inputBound                        bool
	clipboardContext                  *window.Context
	clipboardState                    *input.State
	inputSystemInstalled              bool
	webViewSystemInstalled            bool
	webViewBackendConfigured          bool
	webViewBackend                    any
	webViewBeforeResize               func()
	webViewAfterResize                func()
	webViewDestroy                    func()
	windowInitError                   error
	inputHandlers                     map[input.Phase][]func(*input.Dispatch)
	installedSystemSets               uint32
	windowDragRegion                  func(x, y float64) bool
	windowDragClick                   windowDragClick
	windowOptions                     window.Options
	scriptRegistry                    project.ScriptRegistry
	scriptScene                       *project.SceneInstance
	scriptInputEvents                 ecs.MessageReader[scene.InputEvent]
	scriptSystemInstalled             bool
	scriptGlobalInputSystemInstalled  bool
	scriptPointerInputSystemInstalled bool
	moduleRuntime                     *module.Runtime
	moduleInputEvents                 ecs.MessageReader[scene.InputEvent]
	moduleHTMLEvents                  ecs.MessageReader[html.Event]
	moduleSystemsInstalled            bool
}

// SetScriptRegistry installs generated scene script bindings. The registry is
// applied to the next scene loaded by the app and to the current scene, if
// one has already been loaded.
func (app *App) SetScriptRegistry(registry project.ScriptRegistry) error {
	if app == nil {
		return fmt.Errorf("app is nil")
	}
	app.scriptRegistry = registry
	if app.scriptScene != nil {
		if err := app.scriptScene.SetScriptRegistry(registry); err != nil {
			return err
		}
	}
	if registry.Empty() {
		return nil
	}
	if !app.scriptSystemInstalled {
		if err := app.AddSystem(StageUpdate, ecs.SystemConfig{
			Name: "onix_script_components",
			System: func(context *ecs.SystemContext) error {
				if app.scriptScene == nil {
					return nil
				}
				return app.scriptScene.UpdateScripts(context)
			},
		}); err != nil {
			return err
		}
		app.scriptSystemInstalled = true
	}
	inputMask := registry.InputMask()
	installedGlobal := false
	if inputMask.Intersects(scene.InputMaskKeyboard|scene.InputMaskText|scene.InputMaskFiles) && !app.scriptGlobalInputSystemInstalled {
		if err := app.AddSystem(StageInputGlobal, ecs.SystemConfig{Name: "onix_script_global_input", System: app.dispatchScriptInput}); err != nil {
			return err
		}
		app.scriptGlobalInputSystemInstalled = true
		installedGlobal = true
	}
	if inputMask.Intersects(scene.InputMaskPointer) && !app.scriptPointerInputSystemInstalled {
		if err := app.AddSystem(StageBeforeFixedUpdateLoop, ecs.SystemConfig{Name: "onix_script_pointer_input", System: app.dispatchScriptInput}); err != nil {
			if installedGlobal {
				app.Schedule(StageInputGlobal).Remove("onix_script_global_input")
				app.scriptGlobalInputSystemInstalled = false
			}
			return err
		}
		app.scriptPointerInputSystemInstalled = true
	}
	return nil
}

func (app *App) dispatchScriptInput(context *ecs.SystemContext) error {
	if app.scriptScene == nil {
		app.scriptInputEvents.Discard(context.World)
		return nil
	}
	return app.scriptScene.DispatchInputScripts(context, &app.scriptInputEvents)
}

type RenderMode uint8

const (
	RenderAlways RenderMode = iota
	RenderOnDemand
)

func NewApp() *App {
	app := &App{
		World:         ecs.NewWorld(),
		schedules:     newScheduleMap(),
		renderPacer:   newRatePacer(DefaultTargetFPS, DefaultMaxRendersPerRun),
		fixedTime:     scene.NewFixedTime(time.Second / time.Duration(DefaultTargetTPS)),
		now:           time.Now,
		windowOptions: window.DefaultOptions(),
		inputHandlers: map[input.Phase][]func(*input.Dispatch){},
	}
	// Loaders may start before the first frame. Give their workers a stable
	// wake request immediately rather than creating it during the first Step.
	ecs.InsertResource(app.World, scene.NewRenderRequest(app.wakeLoop))
	app.renderRequested.Store(true)
	return app
}

// AddInputHandler registers a synchronous handler for one input phase.
// Handlers run in registration order and may call Dispatch.Consume.
func (app *App) AddInputHandler(phase input.Phase, handler func(*input.Dispatch)) {
	if app == nil || handler == nil {
		return
	}
	if app.inputHandlers == nil {
		app.inputHandlers = map[input.Phase][]func(*input.Dispatch){}
	}
	app.inputHandlers[phase] = append(app.inputHandlers[phase], handler)
}

func (app *App) SetWindowSize(width, height int) {
	if app.webViewBeforeResize != nil {
		app.webViewBeforeResize()
	}
	previous := app.Context
	app.Context = window.NewContext(width, height, "Onix", app.windowOptions)
	app.wakeContext.Store(app.Context)
	if previous != nil && previous != app.Context {
		previous.Destroy()
	}
	vw, vh := app.Context.GetViewport()
	graphics.BindFrame(app.World, app.Context.Canvas2D(vw, vh))
	app.inputBound = false
	app.Context.SetVsyncEnabled(true)
	app.windowInitError = nil
	if app.webViewAfterResize != nil {
		app.webViewAfterResize()
	}
	app.configureLoopRate()
}

// SetWebViewBackend installs the platform browser backend used by ECS
// WebView components. It is useful on Linux and mobile, where the host can
// provide a WebKitGTK or native Android/iOS implementation.
func (app *App) SetWebViewBackend(backend scene.WebViewBackend) {
	if app == nil {
		return
	}
	app.webViewBackendConfigured = true
	app.webViewBackend = backend
	app.windowInitError = nil
	if app.Context != nil {
		app.installWebViewBackend(backend)
	}
}

func (app *App) installWebViewBackend(backend scene.WebViewBackend) {
	if app == nil {
		return
	}
	if app.World == nil {
		app.World = ecs.NewWorld()
	}
	if previous, ok := ecs.GetResource[*scene.WebViewRuntime](app.World); ok && previous != nil {
		_ = previous.Close()
	}
	ecs.InsertResource(app.World, scene.NewWebViewRuntime(backend))
	if app.webViewSystemInstalled {
		return
	}
	if err := app.AddSystem(StageUpdate, scene.WebViewSystem("")); err == nil {
		app.webViewSystemInstalled = true
		app.installedSystemSets |= systemSetWebView
		app.registerWebViewLifecycle()
	}
}

// SetInitialWindowSize overrides the project window size before the native
// window is created. Project window options are still applied by LoadScene.
func (app *App) SetInitialWindowSize(width, height int) {
	if app == nil || app.Context != nil || width <= 0 || height <= 0 {
		return
	}
	app.initialWindowWidth = width
	app.initialWindowHeight = height
}

func (app *App) SetWindowOptions(options window.Options) {
	if app == nil {
		return
	}
	app.windowOptions = options
}

func (app *App) SetWindowTaskbarVisible(visible bool) {
	if app == nil || app.Context == nil {
		return
	}
	app.Context.SetTaskbarVisible(visible)
}

func (app *App) SetWindowAlwaysOnTop(enabled bool) {
	if app == nil || app.Context == nil {
		return
	}
	app.Context.SetAlwaysOnTop(enabled)
}

// ToggleFullscreen switches the native window between fullscreen and its
// previous windowed bounds.
func (app *App) ToggleFullscreen() {
	if app == nil || app.Context == nil {
		return
	}
	app.Context.ToggleFullscreen()
}

func (app *App) SetWindowTitle(title string) {
	app.SetTitle(title)
}

func (app *App) SetTargetFPS(fps int) {
	if app == nil {
		return
	}
	app.renderPacer.setTargetRate(fps)
	app.RequestRender()
	app.configureLoopRate()
}

func (app *App) SetRenderMode(mode RenderMode) {
	if app == nil {
		return
	}
	app.renderMode = mode
	app.RequestRender()
}

func (app *App) RequestRender() {
	if app == nil {
		return
	}
	app.renderRequested.Store(true)
	app.wakeLoop()
}

func (app *App) SetTargetTPS(tps int) {
	if app == nil {
		return
	}
	if tps <= 0 {
		return
	}
	app.ensureFixedTime().SetTimestep(time.Second / time.Duration(tps))
}

func (app *App) configureLoopRate() {
	if app == nil || app.Context == nil {
		return
	}
	app.Context.SetTargetFPS(app.loopRate())
}

func (app *App) loopRate() int {
	if app == nil {
		return 0
	}
	return app.renderPacer.targetRate
}

func (app *App) SetWindowIcon(data []byte) error {
	return app.SetIcon(data)
}

func (app *App) SetWindowDragRegion(region func(x, y float64) bool) {
	app.windowDragRegion = region
	app.windowDragClick = windowDragClick{}
}

// Destroy releases browser surfaces before destroying the native window.
func (app *App) Destroy() {
	if app == nil {
		return
	}
	app.wakeContext.Store(nil)
	if app.moduleRuntime != nil {
		_ = app.moduleRuntime.Close()
		app.moduleRuntime = nil
	}
	if cleanup := app.webViewDestroy; cleanup != nil {
		cleanup()
	}
	if app.World != nil {
		html.Dispose(app.World)
		if state, ok := ecs.GetResource[*scene.RenderWorld](app.World); ok {
			_ = state.Dispose()
		}
		if state, ok := ecs.GetResource[*render3d.World](app.World); ok {
			_ = state.Dispose()
		}
	}
	if app.Context != nil {
		app.Context.Destroy()
	}
	if frame := graphics.FrameFor(app.World); frame != nil {
		frame.Uploads.Reset()
		frame.Canvas = nil
	}
}

// OpenFileDialog opens a modal native file dialog owned by the engine window.
// An empty result means that the user cancelled the dialog.
func (app *App) OpenFileDialog(options window.FileDialogOptions) ([]string, error) {
	if app == nil || app.Context == nil {
		return nil, window.ErrFileDialogUnavailable
	}
	return app.Context.OpenFileDialog(options)
}

// OpenFile opens a native dialog for one file owned by the engine window.
func (app *App) OpenFile(options window.FileDialogOptions) (string, error) {
	if app == nil || app.Context == nil {
		return "", window.ErrFileDialogUnavailable
	}
	return app.Context.OpenFile(options)
}

// OpenFolder opens a native dialog for one folder owned by the engine window.
func (app *App) OpenFolder(options window.FileDialogOptions) (string, error) {
	if app == nil || app.Context == nil {
		return "", window.ErrFileDialogUnavailable
	}
	return app.Context.OpenFolder(options)
}

// OpenFiles opens a native multi-file dialog owned by the engine window.
func (app *App) OpenFiles(options window.FileDialogOptions) ([]string, error) {
	if app == nil || app.Context == nil {
		return nil, window.ErrFileDialogUnavailable
	}
	return app.Context.OpenFiles(options)
}

// SetWindowInputRegion limits pointer input to logical viewport rectangles.
// A nil slice restores the full window; an empty non-nil slice passes all pointer input through.
func (app *App) SetWindowInputRegion(rects []window.InputRegionRect) {
	if app == nil || app.Context == nil {
		return
	}
	app.Context.SetInputRegion(rects)
}

func (app *App) bindInputEvents() {
	if app == nil || app.inputBound {
		return
	}
	app.inputBound = true

	app.OnKeydown(func(key input.KeyCode) {
		app.enqueueInput(input.Event{Kind: input.EventKeyDown, Key: key})
	})

	app.OnKeyup(func(key input.KeyCode) {
		app.enqueueInput(input.Event{Kind: input.EventKeyUp, Key: key})
	})

	app.OnTextInput(func(text string) {
		app.enqueueInput(input.Event{Kind: input.EventTextInput, Text: text})
	})

	app.OnTextComposition(func(event input.TextComposition) {
		app.enqueueInput(input.Event{Kind: input.EventTextComposition, Composition: event})
	})

	app.OnTextCommand(func(command input.TextCommand) {
		app.enqueueInput(input.Event{Kind: input.EventTextCommand, Command: command})
	})

	app.OnTextSelection(func(selection input.TextSelection) {
		app.enqueueInput(input.Event{Kind: input.EventTextSelection, Selection: selection})
	})

	app.OnMouseDown(func(x, y float64, button input.MouseButton) {
		if routeMouseDown(nil, button, x, y, app.windowDragRegion) {
			app.RequestRender()
			if app.windowDragClick.press(x, y, app.currentTime()) {
				app.ToggleMaximize()
			} else {
				app.StartWindowDrag()
			}
			return
		}
		app.windowDragClick = windowDragClick{}
		app.enqueueInput(input.Event{Kind: input.EventMouseDown, Button: button, X: x, Y: y})
	})

	app.OnMouseUp(func(x, y float64, button input.MouseButton) {
		app.windowDragClick.move(x, y)
		app.enqueueInput(input.Event{Kind: input.EventMouseUp, Button: button, X: x, Y: y})
	})

	app.OnMouseMove(func(x, y float64) {
		app.windowDragClick.move(x, y)
		app.enqueueInput(input.Event{Kind: input.EventMouseMove, X: x, Y: y})
	})

	app.OnMouseWheel(func(xoff, yoff float64) {
		app.enqueueInput(input.Event{Kind: input.EventMouseWheel, WheelX: xoff, WheelY: yoff})
	})

	app.OnTouchStart(func(x, y float64) {
		app.enqueueInput(input.Event{Kind: input.EventTouchStart, X: x, Y: y})
	})

	app.OnTouchEnd(func(x, y float64) {
		app.enqueueInput(input.Event{Kind: input.EventTouchEnd, X: x, Y: y})
	})

	app.OnTouchMove(func(x, y float64) {
		app.enqueueInput(input.Event{Kind: input.EventTouchMove, X: x, Y: y})
	})

	app.OnFilesDropped(func(x, y float64, files []string) {
		for _, file := range files {
			if file != "" {
				app.enqueueInput(input.Event{Kind: input.EventFilesDropped, X: x, Y: y, File: file})
			}
		}
	})
}

func (app *App) enqueueInput(event input.Event) {
	app.RequestRender()
	if state := app.ecsInput(); state != nil {
		state.Enqueue(event)
	}
}

func routeMouseDown(state *input.State, button input.MouseButton, x, y float64, windowDragRegion func(float64, float64) bool) bool {
	if button == input.MouseLeft && windowDragRegion != nil && windowDragRegion(x, y) {
		return true
	}
	if state != nil {
		state.PressMouse(button, x, y)
	}
	return false
}

func (app *App) applyProjectConfig(bundle *project.Bundle) error {
	return app.configureProjectWindow(bundle.AppConfig(), bundle.WindowIcon(), bundle.WindowIconDecoder())
}

func (app *App) configureProjectWindow(config project.AppConfig, icon []byte, decoder window.IconDecoder) error {
	app.windowOptions = window.DefaultOptions()
	if config.Window.RenderScale > 0 {
		app.windowOptions.RenderScale = config.Window.RenderScale
	}
	if config.Window.MaxDevicePixelRatio > 0 {
		app.windowOptions.MaxDevicePixelRatio = config.Window.MaxDevicePixelRatio
	}
	if config.Window.Antialias != nil {
		app.windowOptions.Antialias = *config.Window.Antialias
	}
	app.windowOptions.DRMDevice = config.Window.DRMDevice
	app.windowOptions.DRMConnectorID = config.Window.DRMConnectorID
	app.windowOptions.TitleBarHeight = config.Window.TitleBarHeight
	if config.Window.CornerRadius > 0 {
		app.windowOptions.CornerRadius = config.Window.CornerRadius
	}
	if !config.Window.Decorations.Valid() {
		return fmt.Errorf("invalid window decorations %q", config.Window.Decorations)
	}
	app.windowOptions.Transparent = config.Window.Transparent
	app.windowOptions.Decorations = config.Window.Decorations
	app.windowOptions.Border = config.Window.Border == nil || *config.Window.Border
	app.windowOptions.Shadow = config.Window.Shadow == nil || *config.Window.Shadow
	app.windowOptions.Resizable = config.Window.Resizable == nil || *config.Window.Resizable
	contextCreated := false
	if app.Context == nil {
		width, height := app.initialWindowWidth, app.initialWindowHeight
		if width <= 0 || height <= 0 {
			width = config.Window.Width
			height = config.Window.Height
		}
		if width <= 0 || height <= 0 {
			width, height = config.Viewport.Width, config.Viewport.Height
		}
		if width <= 0 || height <= 0 {
			return fmt.Errorf("window size is missing; set window.width/height or viewport.width/height in project.json")
		}
		app.SetWindowSize(width, height)
		contextCreated = true
	}
	if app.windowInitError != nil {
		err := app.windowInitError
		app.windowInitError = nil
		return fmt.Errorf("initialize webview backend: %w", err)
	}

	if !contextCreated {
		app.Context.SetDecorations(app.windowOptions.Decorations)
		app.Context.SetBorderVisible(app.windowOptions.Border)
		app.Context.SetShadowVisible(app.windowOptions.Shadow)
		app.Context.SetResizable(app.windowOptions.Resizable)
	}
	if config.Window.ShowInTaskbar != nil && (!contextCreated || !*config.Window.ShowInTaskbar) {
		app.SetWindowTaskbarVisible(*config.Window.ShowInTaskbar)
	}
	if config.Window.AlwaysOnTop != nil {
		app.SetWindowAlwaysOnTop(*config.Window.AlwaysOnTop)
	}
	if config.Window.Title != "" {
		app.SetWindowTitle(config.Window.Title)
	}
	if decoder != nil {
		app.Context.SetIconDecoder(decoder)
	}
	if len(icon) != 0 {
		if err := app.SetWindowIcon(icon); err != nil {
			return fmt.Errorf("set window icon: %w", err)
		}
	}
	if config.Window.Vsync != nil {
		app.SetVsyncEnabled(*config.Window.Vsync)
	}
	if config.Window.FPS != nil {
		app.SetTargetFPS(*config.Window.FPS)
	}
	if config.Window.TPS != nil {
		app.SetTargetTPS(*config.Window.TPS)
	}
	app.SetViewportMode(config.Viewport.Mode)
	viewportWidth := config.Viewport.Width
	viewportHeight := config.Viewport.Height
	if viewportWidth <= 0 || viewportHeight <= 0 {
		viewportWidth = config.Window.Width
		viewportHeight = config.Window.Height
	}
	if viewportWidth <= 0 || viewportHeight <= 0 {
		if contextCreated {
			viewportWidth = app.Context.Width
			viewportHeight = app.Context.Height
		}
	}
	if viewportWidth > 0 && viewportHeight > 0 {
		app.SetViewport(viewportWidth, viewportHeight)
	}

	return nil
}
