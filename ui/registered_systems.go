package ui

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

// RuntimeFeature is a statically selected UI subsystem. Only feature types
// passed to RegisterRuntimeFeature are retained by the linker.
type RuntimeFeature interface {
	registerRuntime(*runtimeSystems)
}

type runtimeSystems struct {
	features    uint8
	update      [runtimeSystemCapacity]ecs.System
	sync        [runtimeSystemCapacity]ecs.System
	afterLayout [runtimeSystemCapacity]ecs.System
}

const (
	runtimeFeatureAuthoredControls uint8 = 1 << iota
	runtimeFeatureWidgets
	runtimeFeatureHTML
	runtimeSystemCapacity = 4
)

// AuthoredControls enables pointer-state variants for controls whose visuals
// are authored directly in a scene.
type AuthoredControls struct{}

func (AuthoredControls) registerRuntime(systems *runtimeSystems) {
	if systems.features&runtimeFeatureAuthoredControls != 0 {
		return
	}
	systems.features |= runtimeFeatureAuthoredControls
	addRuntimeSystem(&systems.sync, syncAuthoredControls)
	addRuntimeSystem(&systems.afterLayout, syncAuthoredControls)
}

// Widgets enables the reusable control input and visual systems, except
// Markdown rendering and focused text-input dispatch.
type Widgets struct{}

func (Widgets) registerRuntime(systems *runtimeSystems) {
	if systems.features&runtimeFeatureWidgets != 0 {
		return
	}
	systems.features |= runtimeFeatureWidgets
	addRuntimeSystem(&systems.update, update)
	addRuntimeSystem(&systems.sync, syncWidgets)
	addRuntimeSystem(&systems.afterLayout, syncAfterLayout)
}

// HTMLRenderer enables synchronization of html.Surface components into UI
// entities. It is kept separate so tools can opt into HTML without enabling
// Markdown parsing.
type HTMLRenderer struct{}

func (HTMLRenderer) registerRuntime(systems *runtimeSystems) {
	if systems.features&runtimeFeatureHTML != 0 {
		return
	}
	Widgets{}.registerRuntime(systems)
	systems.features |= runtimeFeatureHTML
	addRuntimeSystem(&systems.update, updateHTMLRuntime)
	addRuntimeSystem(&systems.sync, syncHTMLRuntime)
}

// FullRuntime selects HTML and widget systems for editor and tool worlds.
// Optional feature packages can add their own synchronizers with
// RegisterRuntimeExtension. Widgets also synchronize authored control
// compositions.
type FullRuntime struct{}

func (FullRuntime) registerRuntime(systems *runtimeSystems) {
	HTMLRenderer{}.registerRuntime(systems)
	Widgets{}.registerRuntime(systems)
}

// RuntimeExtension adds an optional synchronizer to an existing UI runtime.
// Feature packages use this boundary so unused implementations stay out of
// the core UI binary.
type RuntimeExtension struct {
	Sync ecs.System
}

func RegisterRuntimeExtension(world *ecs.World, extension RuntimeExtension) {
	if world == nil || extension.Sync == nil {
		return
	}
	prependRuntimeSystem(&runtimeSystemsFor(world).sync, extension.Sync)
}

func addRuntimeSystem(systems *[runtimeSystemCapacity]ecs.System, system ecs.System) {
	for index := range systems {
		if systems[index] == nil {
			systems[index] = system
			return
		}
	}
	panic("ui: runtime system capacity exceeded")
}

func prependRuntimeSystem(systems *[runtimeSystemCapacity]ecs.System, system ecs.System) {
	if systems[len(systems)-1] != nil {
		panic("ui: runtime system capacity exceeded")
	}
	for index := len(systems) - 1; index > 0; index-- {
		systems[index] = systems[index-1]
	}
	systems[0] = system
}

func RegisterRuntimeFeature[T RuntimeFeature](world *ecs.World) {
	if world == nil {
		return
	}
	systems := runtimeSystemsFor(world)
	var feature T
	feature.registerRuntime(systems)
}

func runtimeSystemsFor(world *ecs.World) *runtimeSystems {
	if systems, ok := ecs.GetResource[*runtimeSystems](world); ok && systems != nil {
		return systems
	}
	systems := &runtimeSystems{}
	ecs.InsertResource(world, systems)
	return systems
}

func UpdateSystem(name string) ecs.SystemConfig {
	if name == "" {
		name = "ui_update"
	}
	return ecs.SystemConfig{Name: name, System: runRuntimeSystems[runtimeUpdateStage]}
}

func SyncSystem(name string) ecs.SystemConfig {
	if name == "" {
		name = "ui_sync"
	}
	return ecs.SystemConfig{Name: name, System: runRuntimeSystems[runtimeSyncStage]}
}

func SyncAfterLayoutSystem(name string) ecs.SystemConfig {
	if name == "" {
		name = "ui_sync_after_layout"
	}
	return ecs.SystemConfig{Name: name, System: runRuntimeSystems[runtimeAfterLayoutStage]}
}

// Sync runs the registered UI synchronization stages for a world.
func Sync(world *ecs.World) error {
	return SyncSystem("").Run(world)
}

// SyncAfterLayout runs registered UI synchronization after layout.
func SyncAfterLayout(world *ecs.World) error {
	return SyncAfterLayoutSystem("").Run(world)
}

type runtimeStage interface {
	systems(*runtimeSystems) *[runtimeSystemCapacity]ecs.System
}

type runtimeUpdateStage struct{}
type runtimeSyncStage struct{}
type runtimeAfterLayoutStage struct{}

func (runtimeUpdateStage) systems(runtime *runtimeSystems) *[runtimeSystemCapacity]ecs.System {
	return &runtime.update
}

func (runtimeSyncStage) systems(runtime *runtimeSystems) *[runtimeSystemCapacity]ecs.System {
	return &runtime.sync
}

func (runtimeAfterLayoutStage) systems(runtime *runtimeSystems) *[runtimeSystemCapacity]ecs.System {
	return &runtime.afterLayout
}

func runRuntimeSystems[S runtimeStage](context *ecs.SystemContext) error {
	if context == nil || context.World == nil {
		return nil
	}
	systems, ok := ecs.GetResource[*runtimeSystems](context.World)
	if !ok || systems == nil {
		return nil
	}
	return runRuntimeSystemSet[S](context, systems)
}

func runRuntimeSystemSet[S runtimeStage](context *ecs.SystemContext, systems *runtimeSystems) error {
	var stage S
	for _, system := range stage.systems(systems) {
		if system == nil {
			break
		}
		if err := system(context); err != nil {
			return err
		}
	}
	return nil
}

// SyncWith synchronizes a statically selected tool profile without changing
// the features installed in the world's scheduled UI runtime.
func SyncWith[F RuntimeFeature](world *ecs.World) error {
	return runRuntimeStageWith[F, runtimeSyncStage](world)
}

// SyncAfterLayoutWith runs the selected tool profile after UI layout.
func SyncAfterLayoutWith[F RuntimeFeature](world *ecs.World) error {
	return runRuntimeStageWith[F, runtimeAfterLayoutStage](world)
}

func runRuntimeStageWith[F RuntimeFeature, S runtimeStage](world *ecs.World) error {
	if world == nil {
		return nil
	}
	profile := runtimeProfileFor[F](world)
	return runRuntimeSystemSet[S](&profile.context, &profile.systems)
}

type runtimeProfile[F RuntimeFeature] struct {
	systems runtimeSystems
	context ecs.SystemContext
}

func runtimeProfileFor[F RuntimeFeature](world *ecs.World) *runtimeProfile[F] {
	if profile, ok := ecs.GetResource[*runtimeProfile[F]](world); ok && profile != nil {
		return profile
	}
	profile := &runtimeProfile[F]{context: ecs.SystemContext{World: world}}
	var feature F
	feature.registerRuntime(&profile.systems)
	ecs.InsertResource(world, profile)
	return profile
}

func syncAuthoredControls(context *ecs.SystemContext) error {
	if context == nil || context.World == nil {
		return nil
	}
	syncControls(context.World, Skin{})
	syncVariants(context.World, authoredControlVariantMatches)
	return nil
}

func authoredControlVariantMatches(world *ecs.World, entity ecs.Entity, variant ControlVariant) bool {
	pointer := ecs.GetRef[scene.InputState2D](world, entity)
	target := ecs.GetRef[scene.InputTarget2D](world, entity)
	hovered := pointer != nil && pointer.Hovered
	pressed := pointer != nil && pointer.Pressed
	disabled := target != nil && target.Disabled
	switch variant {
	case VariantAlways:
		return true
	case VariantInputNormal, VariantUnfocusedNormal, VariantClosedNormal:
		return !hovered && !pressed
	case VariantHovered, VariantUnfocusedHovered, VariantClosedHovered:
		return hovered && !pressed && !disabled
	case VariantPressed:
		return pressed && !disabled
	default:
		return false
	}
}
