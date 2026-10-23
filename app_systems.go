package onix

import (
	"fmt"

	"github.com/diiyw/onix/ecs"
	enginephysics "github.com/diiyw/onix/physics"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/scene/render3d"
	engineui "github.com/diiyw/onix/ui"
	"github.com/diiyw/onix/window"
)

// SystemSet is a statically selected engine capability. The unexported method
// keeps installation logic inside Onix while allowing a generic public API.
type SystemSet interface {
	install(*App) error
}

// InstallSystems installs one capability and its explicit dependencies. Calls
// are idempotent, and unused capability implementations can be removed by the
// Go linker.
func InstallSystems[T SystemSet](app *App) error {
	if app == nil {
		return fmt.Errorf("app is nil")
	}
	var systems T
	return systems.install(app)
}

const (
	systemSetTransforms uint32 = 1 << iota
	systemSetRenderCore
	systemSetRender2D
	systemSetRender3D
	systemSetInput2D
	systemSetAnimations2D
	systemSetSpriteTrails2D
	systemSetParticles2D
	systemSetTilemaps2D
	systemSetSpine2D
	systemSetPhysics2D
	systemSetPhysics3D
	systemSetUIRuntime
	systemSetUIControls
	systemSetUIWidgets
	systemSetUITextInput
	systemSetUIMarkdown
	systemSetUIAnimator
	systemSetUIHTML
	systemSetWebView
	systemSetPhysicsTransforms
)

type Transforms struct{}
type Render2D struct{}
type Render3D struct{}
type Input2D struct{}

// Animations2D installs the animator update systems for node-bound clips.
type Animations2D struct{}
type SpriteTrails2D struct{}
type Particles2D struct{}
type Tilemaps2D struct{}
type Spine2D struct{}
type Physics2D struct{}
type Physics3D struct{}
type UIControls struct{}
type UIWidgets struct{}
type UITextInput struct{}

// MarkdownRuntime is the optional implementation behind UIMarkdown.
// Keeping the implementation in a feature package lets projects that do not
// enable Markdown leave its parser dependencies out of the final binary.
type MarkdownRuntime interface {
	InstallMarkdown(*App) error
}

type UIMarkdown[F MarkdownRuntime] struct{}

// UIHTML installs the HTML component renderer on top of the UI runtime.
type UIHTML struct{}

// WebView installs the optional browser surface runtime. The platform
// backend is created after the window context exists, so projects can install
// systems before loading their first scene.
type WebView struct{}

func (app *App) hasSystemSet(set uint32) bool {
	return app.installedSystemSets&set != 0
}

func (app *App) hasProjectFeature(id project.RuntimeFeatureID) bool {
	switch id {
	case project.FeatureRender2D:
		return app.hasSystemSet(systemSetRender2D)
	case project.FeatureRender3D:
		return app.hasSystemSet(systemSetRender3D)
	case project.FeatureInput2D:
		return app.hasSystemSet(systemSetInput2D)
	case project.FeatureAnimations2D:
		return app.hasSystemSet(systemSetAnimations2D)
	case project.FeatureSpriteTrails2D:
		return app.hasSystemSet(systemSetSpriteTrails2D)
	case project.FeatureParticles2D:
		return app.hasSystemSet(systemSetParticles2D)
	case project.FeatureTilemaps2D:
		return app.hasSystemSet(systemSetTilemaps2D)
	case project.FeatureSpine2D:
		return app.hasSystemSet(systemSetSpine2D)
	case project.FeaturePhysics2D:
		return app.hasSystemSet(systemSetPhysics2D)
	case project.FeaturePhysics3D:
		return app.hasSystemSet(systemSetPhysics3D)
	case project.FeatureUIControls:
		return app.hasSystemSet(systemSetUIControls)
	case project.FeatureUIWidgets:
		// UIControls and the higher-level UI features install the same generic
		// layout/transform runtime, so they also satisfy Layout/Widget scenes.
		return app.hasSystemSet(systemSetUIWidgets) || app.hasSystemSet(systemSetUIControls)
	case project.FeatureUITextInput:
		return app.hasSystemSet(systemSetUITextInput)
	case project.FeatureUIMarkdown:
		return app.hasSystemSet(systemSetUIMarkdown)
	case project.FeatureUIHTML:
		return app.hasSystemSet(systemSetUIHTML)
	case project.FeatureWebView:
		return app.hasSystemSet(systemSetWebView)
	default:
		return false
	}
}

func (Transforms) install(app *App) error {
	if app.hasSystemSet(systemSetTransforms) {
		return nil
	}
	if err := app.AddSystem(StagePreUpdate, scene.SyncGlobalTransformSystem("sync_global_transform")); err != nil {
		return err
	}
	if err := app.AddSystem(StageRender, scene.SyncGlobalTransformSystem("sync_render_transform")); err != nil {
		return err
	}
	app.installedSystemSets |= systemSetTransforms
	return nil
}

func (Render2D) install(app *App) error {
	if app.hasSystemSet(systemSetRender2D) {
		return nil
	}
	if err := InstallSystems[Transforms](app); err != nil {
		return err
	}
	if err := app.ensureRenderCore(); err != nil {
		return err
	}
	scene.RegisterSpriteRenderer2D(app.World)
	scene.RegisterSpriteLayersRenderer2D(app.World)
	scene.RegisterTextRenderer2D(app.World)
	scene.RegisterUIRenderer2D(app.World)
	scene.RegisterCameraRenderer2D(app.World)
	if err := app.AddSystem(StageRender, ecs.SystemConfig{
		Name: "extract_render_2d", After: []string{"extract_cameras"}, System: scene.ExtractRender2DSystem("").System,
	}); err != nil {
		return err
	}
	app.installedSystemSets |= systemSetRender2D
	app.ensureCamera2D()
	return app.refreshRenderTail()
}

func (Render3D) install(app *App) error {
	if app.hasSystemSet(systemSetRender3D) {
		return nil
	}
	if err := InstallSystems[Transforms](app); err != nil {
		return err
	}
	if err := app.ensureRenderCore(); err != nil {
		return err
	}
	if _, ok := ecs.GetResource[*render3d.AmbientLight](app.World); !ok {
		ecs.InsertResource(app.World, render3d.NewAmbientLight())
	}
	render3d.Register(app.World)
	if err := app.AddSystems(StageRender,
		ecs.SystemConfig{Name: "extract_render_3d", After: []string{"extract_cameras"}, System: render3d.ExtractSystem("").System},
		ecs.SystemConfig{Name: "prepare_render_resources_3d", After: []string{"extract_render_3d"}, System: render3d.PrepareSystem("").System},
		ecs.SystemConfig{Name: "sort_render_phases_3d", After: []string{"prepare_render_resources_3d"}, System: render3d.SortSystem("").System},
	); err != nil {
		return err
	}
	app.installedSystemSets |= systemSetRender3D
	app.configureCamera2D()
	return app.refreshRenderTail()
}

func (app *App) ensureRenderCore() error {
	if app.hasSystemSet(systemSetRenderCore) {
		return nil
	}
	if _, ok := ecs.GetResource[*scene.RenderWorld](app.World); !ok {
		ecs.InsertResource(app.World, scene.NewRenderWorld())
	}
	cameras := scene.ExtractCamerasSystem("extract_cameras")
	cameras.After = []string{"sync_render_transform"}
	if app.hasSystemSet(systemSetUIRuntime) {
		cameras.After = append(cameras.After, "sync_render_ui_transform")
	}
	if err := app.AddSystem(StageRender, cameras); err != nil {
		return err
	}
	app.installedSystemSets |= systemSetRenderCore
	return nil
}

func (app *App) refreshRenderTail() error {
	schedule := app.Schedule(StageRender)
	schedule.Remove("render_debug_info")
	schedule.Remove("render_cameras")
	after := make([]string, 0, 2)
	if app.hasSystemSet(systemSetRender2D) {
		after = append(after, "extract_render_2d")
	}
	if app.hasSystemSet(systemSetRender3D) {
		after = append(after, "sort_render_phases_3d")
	}
	systems := []ecs.SystemConfig{{
		Name: "render_cameras", After: after, System: scene.RenderCamerasSystem("").System,
	}}
	return app.AddSystems(StageRender, appendDefaultDebugRenderSystem(systems)...)
}

func (app *App) ensureCamera2D() {
	var entity ecs.Entity
	for value, _ := range ecs.Query[scene.Camera2D](app.World).All {
		if camera, ok := ecs.Get[scene.Camera](app.World, value); ok && camera.Fallback {
			entity = value
			break
		}
	}
	if entity == 0 {
		entity = scene.SpawnCamera2D(app.World)
		ecs.GetMut[scene.Camera](app.World, entity).Fallback = true
	}
	app.configureCamera2D()
}

func (app *App) configureCamera2D() {
	overlay := app.hasSystemSet(systemSetRender3D)
	settings, hasSettings := ecs.GetResource[scene.RenderSettings2D](app.World)
	for entity, _ := range ecs.Query[scene.Camera2D](app.World).All {
		camera := ecs.GetMut[scene.Camera](app.World, entity)
		if camera != nil && camera.Fallback {
			if hasSettings {
				camera.ClearColor = settings.ClearColor
			}
			if overlay {
				camera.Order, camera.Clear = 1000, false
			} else {
				camera.Order, camera.Clear = 0, true
			}
		}
	}
}

func (Input2D) install(app *App) error {
	if app.hasSystemSet(systemSetInput2D) {
		return nil
	}
	if err := InstallSystems[Transforms](app); err != nil {
		return err
	}
	app.installedSystemSets |= systemSetInput2D
	return app.refreshInput2D()
}

func (app *App) refreshInput2D() error {
	schedule := app.Schedule(StagePreUpdate)
	schedule.Remove("dispatch_pointer_events_2d")
	after := "sync_global_transform"
	if app.hasSystemSet(systemSetSpriteTrails2D) {
		after = "update_sprite_trail_2d"
	}
	return app.AddSystem(StagePreUpdate, ecs.SystemConfig{
		Name: "dispatch_pointer_events_2d", After: []string{after}, System: scene.DispatchInputEvents2DSystem("").System,
	})
}

func (Animations2D) install(app *App) error {
	if app.hasSystemSet(systemSetAnimations2D) {
		return nil
	}
	if err := InstallSystems[Render2D](app); err != nil {
		return err
	}
	// Animation components target authored scene entities and update here before
	// global transform propagation.
	worldAnimation := scene.UpdateWorldAnimatorSystem("update_world_animator")
	worldAnimation.Before = []string{"sync_global_transform"}
	if err := app.AddSystem(StagePreUpdate, worldAnimation); err != nil {
		return err
	}
	app.installedSystemSets |= systemSetAnimations2D
	if err := app.installUIAnimatorSystem(); err != nil {
		return err
	}
	return nil
}

func (SpriteTrails2D) install(app *App) error {
	if app.hasSystemSet(systemSetSpriteTrails2D) {
		return nil
	}
	if err := InstallSystems[Render2D](app); err != nil {
		return err
	}
	scene.RegisterSpriteTrailRenderer2D(app.World)
	if err := app.AddSystem(StagePreUpdate, ecs.SystemConfig{
		Name: "update_sprite_trail_2d", After: []string{"sync_global_transform"}, System: scene.UpdateSpriteTrail2DSystem("").System,
	}); err != nil {
		return err
	}
	app.installedSystemSets |= systemSetSpriteTrails2D
	if app.hasSystemSet(systemSetInput2D) {
		return app.refreshInput2D()
	}
	return nil
}

func (Particles2D) install(app *App) error {
	if app.hasSystemSet(systemSetParticles2D) {
		return nil
	}
	if err := InstallSystems[Render2D](app); err != nil {
		return err
	}
	scene.RegisterParticleRenderer2D(app.World)
	if err := app.AddSystem(StageFixedUpdate, scene.UpdateParticleEmitter2DSystem("update_particle_emitter_2d")); err != nil {
		return err
	}
	app.installedSystemSets |= systemSetParticles2D
	return nil
}

func (Tilemaps2D) install(app *App) error {
	if app.hasSystemSet(systemSetTilemaps2D) {
		return nil
	}
	if err := InstallSystems[Render2D](app); err != nil {
		return err
	}
	scene.RegisterTilemapRenderer2D(app.World)
	app.installedSystemSets |= systemSetTilemaps2D
	return nil
}

func (Spine2D) install(app *App) error {
	if app.hasSystemSet(systemSetSpine2D) {
		return nil
	}
	if err := InstallSystems[Render2D](app); err != nil {
		return err
	}
	scene.RegisterSpineRenderer2D(app.World)
	if err := app.AddSystem(StageUpdate, scene.UpdateSpine2DSystem("update_spine_2d")); err != nil {
		return err
	}
	app.installedSystemSets |= systemSetSpine2D
	return nil
}

// Physics2D installs Onix's fixed-step rigid-body simulation. Only entities
// carrying RigidBody2D and collider components participate in collisions.
func (Physics2D) install(app *App) error {
	if app.hasSystemSet(systemSetPhysics2D) {
		return nil
	}
	if err := InstallSystems[Transforms](app); err != nil {
		return err
	}
	if _, ok := ecs.GetResource[*enginephysics.Runtime2D](app.World); !ok {
		ecs.InsertResource(app.World, enginephysics.NewRuntime2D())
	}
	if err := app.AddSystem(StageFixedUpdate, enginephysics.StepSystem2D("step_physics_2d")); err != nil {
		return err
	}
	if err := app.ensurePhysicsTransforms(); err != nil {
		return err
	}
	app.installedSystemSets |= systemSetPhysics2D
	return nil
}

// Physics3D installs Onix's fixed-step 3D rigid-body simulation.
func (Physics3D) install(app *App) error {
	if app.hasSystemSet(systemSetPhysics3D) {
		return nil
	}
	if err := InstallSystems[Transforms](app); err != nil {
		return err
	}
	if _, ok := ecs.GetResource[*enginephysics.Runtime3D](app.World); !ok {
		ecs.InsertResource(app.World, enginephysics.NewRuntime3D())
	}
	if err := app.AddSystem(StageFixedUpdate, enginephysics.StepSystem3D("step_physics_3d")); err != nil {
		return err
	}
	if err := app.ensurePhysicsTransforms(); err != nil {
		return err
	}
	app.installedSystemSets |= systemSetPhysics3D
	return nil
}

func (app *App) ensurePhysicsTransforms() error {
	if app.hasSystemSet(systemSetPhysicsTransforms) {
		return nil
	}
	if err := app.AddSystem(StageFixedPostUpdate, scene.SyncGlobalTransformSystem("sync_physics_transform")); err != nil {
		return err
	}
	app.installedSystemSets |= systemSetPhysicsTransforms
	return nil
}

func (app *App) installUIAnimatorSystem() error {
	if app == nil || !app.hasSystemSet(systemSetAnimations2D) || app.hasSystemSet(systemSetUIAnimator) {
		return nil
	}
	uiAnimation := scene.UpdateUIAnimatorSystem("update_ui_animator")
	if err := app.AddSystem(StagePostUpdate, uiAnimation); err != nil {
		return err
	}
	animatedTransform := scene.SyncUIGlobalTransformSystem("sync_ui_animated_transform")
	animatedTransform.After = []string{"update_ui_animator"}
	if err := app.AddSystem(StagePostUpdate, animatedTransform); err != nil {
		return err
	}
	app.installedSystemSets |= systemSetUIAnimator
	return nil
}

func (UIControls) install(app *App) error {
	if app.hasSystemSet(systemSetUIControls) {
		return nil
	}
	if err := app.ensureUIRuntime(); err != nil {
		return err
	}
	engineui.RegisterRuntimeFeature[engineui.AuthoredControls](app.World)
	app.installedSystemSets |= systemSetUIControls
	return nil
}

func (UIWidgets) install(app *App) error {
	if app.hasSystemSet(systemSetUIWidgets) {
		return nil
	}
	if err := app.ensureUIRuntime(); err != nil {
		return err
	}
	engineui.RegisterRuntimeFeature[engineui.Widgets](app.World)
	app.installedSystemSets |= systemSetUIWidgets
	return nil
}

func (app *App) ensureUIRuntime() error {
	if app.hasSystemSet(systemSetUIRuntime) {
		return nil
	}
	if err := InstallSystems[Input2D](app); err != nil {
		return err
	}
	if err := app.AddSystem(StageUpdate, engineui.UpdateSystem("update_ui")); err != nil {
		return err
	}
	if err := app.AddSystem(StagePostUpdate, scene.UIScrollSystem("ui_scroll")); err != nil {
		return err
	}
	sync := engineui.SyncSystem("sync_ui")
	sync.After = []string{"ui_scroll"}
	if err := app.AddSystem(StagePostUpdate, sync); err != nil {
		return err
	}
	if err := app.AddSystem(StagePostUpdate, ecs.SystemConfig{
		Name: "sync_ui_global_transform", After: []string{"sync_ui"}, System: scene.SyncUIGlobalTransformSystem("").System,
	}); err != nil {
		return err
	}
	if err := app.AddSystem(StagePostUpdate, ecs.SystemConfig{
		Name: "sync_ui_after_layout", After: []string{"sync_ui_global_transform"}, System: engineui.SyncAfterLayoutSystem("").System,
	}); err != nil {
		return err
	}
	if err := app.AddSystem(StagePostUpdate, ecs.SystemConfig{
		// Generic Layout/Widget constraints are authoritative after legacy UI sync.
		Name: "sync_ui_layout", After: []string{"sync_ui_after_layout"}, System: engineui.SyncLayoutSystem("").System,
	}); err != nil {
		return err
	}
	if err := app.AddSystem(StagePostUpdate, ecs.SystemConfig{
		Name: "sync_ui_global_transform_after_layout", After: []string{"sync_ui_layout"}, System: scene.SyncUIGlobalTransformSystem("").System,
	}); err != nil {
		return err
	}
	final := scene.SyncUIGlobalTransformSystem("sync_render_ui_transform")
	final.After = []string{"sync_render_transform"}
	if app.hasSystemSet(systemSetRenderCore) {
		final.Before = []string{"extract_cameras"}
	}
	if err := app.AddSystem(StageRender, final); err != nil {
		return err
	}
	app.installedSystemSets |= systemSetUIRuntime
	return nil
}

func (UITextInput) install(app *App) error {
	if app.hasSystemSet(systemSetUITextInput) {
		return nil
	}
	if err := InstallSystems[UIWidgets](app); err != nil {
		return err
	}
	if err := app.AddSystem(StageInputGUI, engineui.FocusedInputSystem("dispatch_focused_input")); err != nil {
		return err
	}
	textInput := engineui.SyncTextInputSystem("sync_ui_text_input")
	textInput.After = []string{"sync_ui_global_transform_after_layout"}
	if err := app.AddSystem(StagePostUpdate, textInput); err != nil {
		return err
	}
	app.installedSystemSets |= systemSetUITextInput
	return nil
}

func (UIMarkdown[F]) install(app *App) error {
	if app.hasSystemSet(systemSetUIMarkdown) {
		return nil
	}
	var runtime F
	if err := runtime.InstallMarkdown(app); err != nil {
		return err
	}
	app.installedSystemSets |= systemSetUIMarkdown
	return nil
}

func (UIHTML) install(app *App) error {
	if app.hasSystemSet(systemSetUIHTML) {
		return nil
	}
	if err := InstallSystems[Render2D](app); err != nil {
		return err
	}
	if err := InstallSystems[UITextInput](app); err != nil {
		return err
	}
	engineui.RegisterRuntimeFeature[engineui.HTMLRenderer](app.World)
	app.installedSystemSets |= systemSetUIHTML
	return nil
}

func (WebView) install(app *App) error {
	if app.hasSystemSet(systemSetWebView) {
		return nil
	}
	if err := app.AddSystem(StageUpdate, scene.WebViewSystem("")); err != nil {
		return err
	}
	app.installedSystemSets |= systemSetWebView
	app.webViewSystemInstalled = true
	app.registerWebViewLifecycle()
	if app.Context == nil {
		return nil
	}
	return app.initializeWebViewBackend()
}

func (app *App) registerWebViewLifecycle() {
	app.webViewBeforeResize = app.closeWebViewRuntime
	app.webViewAfterResize = app.initializeWebViewAfterResize
	app.webViewDestroy = app.closeWebViewRuntime
}

func (app *App) closeWebViewRuntime() {
	if app == nil || app.World == nil {
		return
	}
	if runtime, ok := ecs.GetResource[*scene.WebViewRuntime](app.World); ok && runtime != nil {
		_ = runtime.Close()
	}
}

func (app *App) initializeWebViewAfterResize() {
	if app == nil {
		return
	}
	if err := app.initializeWebViewBackend(); err != nil {
		app.windowInitError = err
	}
}

func (app *App) initializeWebViewBackend() error {
	if app == nil || !app.hasSystemSet(systemSetWebView) {
		return nil
	}
	if app.webViewBackendConfigured {
		backend, ok := app.webViewBackend.(scene.WebViewBackend)
		if !ok || backend == nil {
			return scene.ErrWebViewUnavailable
		}
		app.installWebViewBackend(backend)
		return nil
	}
	backend, err := window.NewWebViewBackend(app.Context)
	if err != nil {
		return err
	}
	if backend == nil {
		return scene.ErrWebViewUnavailable
	}
	app.webViewBackend = backend
	app.installWebViewBackend(backend)
	return nil
}
