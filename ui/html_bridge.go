package ui

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	htmlui "github.com/diiyw/onix/ui/html"
)

type htmlBridgeState struct {
	versions     [5]uint64
	scopeVersion uint64
	lastRun      ecs.Tick
	initialized  bool
	context      ecs.SystemContext
}

// The first input may change per entity. Remaining inputs invalidate membership
// or inherited metadata and require checking the complete scoped collection.
func prepareHTMLBridge[Tag any](world *ecs.World, versions [5]uint64) (*htmlBridgeState, bool, bool) {
	state := world.ScopeValue[Tag, htmlBridgeState]()
	_, scopeVersion := world.QueryScopeKey()
	full := !state.initialized || state.scopeVersion != scopeVersion
	for i := 1; i < len(versions); i++ {
		full = full || state.versions[i] != versions[i]
	}
	return state, full || state.versions[0] != versions[0], full
}

func (state *htmlBridgeState) capture(world *ecs.World, versions [5]uint64) {
	_, state.scopeVersion = world.QueryScopeKey()
	state.versions, state.lastRun, state.initialized = versions, world.ChangeTick(), true
}

func (state *htmlBridgeState) changes(world *ecs.World) *ecs.SystemContext {
	// Separate synchronizers can write during the same enclosing ECS tick.
	// Revisit that tick when versions moved, rather than losing those writes.
	state.context = world.ChangesSince(state.lastRun - 1)
	return &state.context
}

func htmlBridgeMembership[T any](world *ecs.World) uint64 {
	_, membership := world.ComponentValueRevision[T]()
	return membership
}

func htmlControlBridgeVersions(world *ecs.World) [5]uint64 {
	values, membership := world.ComponentValueRevision[htmlui.Control]()
	stack, _ := world.ComponentValueRevision[scene.UIStackIndex]()
	return [5]uint64{values, membership, htmlBridgeMembership[htmlControlState](world), stack, htmlBridgeMembership[Selector](world)}
}

func htmlEditableBridgeVersions(world *ecs.World) [5]uint64 {
	values, membership := world.ComponentValueRevision[htmlui.Editable]()
	return [5]uint64{values, membership, htmlBridgeMembership[htmlEditableState](world)}
}

func htmlRelationBridgeVersions[Link, Relation, Owner any](world *ecs.World) [5]uint64 {
	return [5]uint64{htmlBridgeMembership[Link](world), htmlBridgeMembership[Relation](world), htmlBridgeMembership[Owner](world)}
}
