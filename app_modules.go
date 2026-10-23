package onix

import (
	"fmt"
	"io/fs"
	"path"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/module"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/window"
)

// OpenModuleWindow applies project window metadata. LoadModules stays usable
// without a native window for simulation, build validation and server hosts.
func (app *App) OpenModuleWindow(spec project.ProjectSpec, source fs.FS, decoder window.IconDecoder) error {
	if app == nil {
		return module.ErrInvalid
	}
	var icon []byte
	if spec.Window.Icon != "" {
		if !fs.ValidPath(spec.Window.Icon) || source == nil {
			return fmt.Errorf("invalid project window icon %q", spec.Window.Icon)
		}
		var err error
		icon, err = fs.ReadFile(source, path.Join("resources", spec.Window.Icon))
		if err != nil {
			return err
		}
	}
	return app.configureProjectWindow(project.AppConfig{Window: spec.Window, Viewport: spec.Viewport}, icon, decoder)
}

// LoadModules mounts the entry of the new module format. It does not open a
// native window, so loading and logic tests work independently of rendering.
func (app *App) LoadModules(plan *project.ProjectPlan, registry module.Registry, options module.Options) (*module.Scene, error) {
	if app == nil || plan == nil {
		return nil, module.ErrInvalid
	}
	if app.moduleRuntime != nil {
		return nil, fmt.Errorf("module runtime is already loaded")
	}
	app.ensureECSResources()
	if options.Wake == nil {
		options.Wake = app.RequestRender
	}
	if options.Host == nil {
		options.Host = &module.Host{
			Context: app.Context, RequestRender: app.RequestRender, WakeAt: app.WakeAt,
			RequestClose: func() {
				if app.Context != nil {
					app.RequestClose()
				}
			},
			Minimize: func() {
				if app.Context != nil {
					app.Minimize()
				}
			},
			ToggleMaximize: func() {
				if app.Context != nil {
					app.ToggleMaximize()
				}
			},
			ToggleFullscreen: func() {
				if app.Context != nil {
					app.ToggleFullscreen()
				}
			},
			SetWindowTitle: app.SetWindowTitle,
			SetViewport: func(w, h int) {
				if app.Context != nil {
					app.SetViewport(w, h)
				}
			},
			GetViewport: func() (int, int) {
				if app.Context == nil {
					return plan.Spec.Viewport.Width, plan.Spec.Viewport.Height
				}
				return app.GetViewport()
			},
			SetWindowDragRegion: app.SetWindowDragRegion, SetWindowInputRegion: app.SetWindowInputRegion,
			OnCloseRequested: func(f func() bool) {
				if app.Context != nil {
					app.OnCloseRequested(f)
				}
			},
			OpenFolder: app.OpenFolder, OpenFile: app.OpenFile, OpenFiles: app.OpenFiles,
			SetClipboardText: func(s string) bool { return app.Context != nil && app.SetClipboardText(s) },
			SetMouseCaptured: func(v bool) bool { return app.Context != nil && app.SetMouseCaptured(v) },
			MouseCaptured:    func() bool { return app.Context != nil && app.MouseCaptured() },
			TakeMouseDelta: func() (float64, float64) {
				if app.Context == nil {
					return 0, 0
				}
				return app.TakeMouseDelta()
			},
		}
	}
	runtime, err := module.New(app.World, plan, registry, options)
	if err != nil {
		return nil, err
	}
	entry, err := runtime.Manager().StartEntry()
	if err != nil {
		_ = runtime.Close()
		return nil, err
	}
	if err = app.installModuleSystems(plan); err != nil {
		_ = runtime.Close()
		return nil, err
	}
	app.moduleRuntime = runtime
	if plan.Spec.ClearColor != "" {
		color, err := project.ParseColor(plan.Spec.ClearColor, 1)
		if err != nil {
			_ = runtime.Close()
			app.moduleRuntime = nil
			return nil, err
		}
		ecs.InsertResource(app.World, scene.RenderSettings2D{ClearColor: color})
		if app.hasSystemSet(systemSetRender2D) {
			// Include fallback cameras created after the entry scene established its query scope.
			runtime.SyncScopes()
			app.configureCamera2D()
		}
	}
	return entry, nil
}

func (app *App) Modules() *module.Runtime { return app.moduleRuntime }

func (app *App) installModuleSystems(plan *project.ProjectPlan) error {
	if app.moduleSystemsInstalled {
		return nil
	}
	if err := InstallSystems[Transforms](app); err != nil {
		return err
	}
	var renders, inputs bool
	used := make([]bool, len(plan.Mods))
	for _, s := range plan.Scenes {
		for _, i := range s.Instances {
			if int(i.Mod) >= len(used) {
				return module.ErrInvalid
			}
			used[i.Mod] = true
		}
	}
	for index, mod := range plan.Mods {
		if !used[index] {
			continue
		}
		check := func(c project.ModComponents) {
			renders = renders || c.Sprite != nil || c.Text != nil || c.HTML != nil || c.Emitter != nil || c.Tilemap != nil || c.Camera2D != nil
			inputs = inputs || c.Input != nil && c.Input.Pointer
		}
		check(mod.Spec.Components)
		for _, element := range mod.Elements {
			check(element.Spec.Components)
		}
		for _, slot := range mod.Slots {
			renders = renders || slot.Type == project.VisualValue
		}
	}
	if renders {
		if err := InstallSystems[Render2D](app); err != nil {
			return err
		}
	}
	if inputs {
		if err := InstallSystems[Input2D](app); err != nil {
			return err
		}
	}
	ui := scene.SyncUIGlobalTransformSystem("sync_module_ui_transform")
	if app.hasSystemSet(systemSetInput2D) {
		ui.Before = []string{"dispatch_pointer_events_2d"}
	}
	if err := app.AddSystem(StagePreUpdate, ui); err != nil {
		return err
	}
	uiRender := scene.SyncUIGlobalTransformSystem("sync_module_render_ui_transform")
	uiRender.After = []string{"sync_render_transform"}
	if app.hasSystemSet(systemSetRenderCore) {
		uiRender.Before = []string{"extract_cameras"}
	}
	if err := app.AddSystem(StageRender, uiRender); err != nil {
		return err
	}
	if err := app.AddSystem(StageModuleEvents, ecs.SystemConfig{Name: "dispatch_module_events", System: func(*ecs.SystemContext) error {
		if app.moduleRuntime == nil {
			return nil
		}
		return app.moduleRuntime.Dispatch()
	}}); err != nil {
		return err
	}
	if err := app.AddSystem(StageFixedUpdate, ecs.SystemConfig{Name: "update_modules", System: func(context *ecs.SystemContext) error {
		if app.moduleRuntime == nil {
			return nil
		}
		clock, ok := ecs.GetResource[*scene.Time](context.World)
		if !ok {
			return nil
		}
		return app.moduleRuntime.Update(clock.DeltaSeconds())
	}}); err != nil {
		return err
	}
	if err := app.AddSystem(StageBeforeFixedUpdateLoop, ecs.SystemConfig{Name: "dispatch_module_input", System: app.dispatchModuleInput}); err != nil {
		return err
	}
	if err := app.AddSystem(StageAfterFixedUpdateLoop, ecs.SystemConfig{Name: "update_module_frame", System: func(context *ecs.SystemContext) error {
		if app.moduleRuntime == nil {
			return nil
		}
		clock, _ := ecs.GetResource[*scene.Time](context.World)
		return app.moduleRuntime.UpdateFrame(clock.DeltaSeconds())
	}}); err != nil {
		return err
	}
	if err := app.AddSystem(StageFixedPostUpdate, ecs.SystemConfig{Name: "publish_module_collisions", System: func(*ecs.SystemContext) error {
		if app.moduleRuntime == nil {
			return nil
		}
		return app.moduleRuntime.PublishCollisions()
	}}); err != nil {
		return err
	}
	app.moduleSystemsInstalled = true
	return nil
}

func (app *App) dispatchModuleInput(context *ecs.SystemContext) error {
	if app.moduleRuntime == nil {
		app.moduleInputEvents.Discard(context.World)
		app.moduleHTMLEvents.Discard(context.World)
		return nil
	}
	for event := range app.moduleInputEvents.All(context.World) {
		if err := app.moduleRuntime.HandleInput(*event); err != nil {
			return err
		}
	}
	for event := range app.moduleHTMLEvents.All(context.World) {
		if err := app.moduleRuntime.HandleHTMLEvent(*event); err != nil {
			return err
		}
	}
	return nil
}
