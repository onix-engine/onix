package ui

import (
	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/scene"
	htmlui "github.com/onix-engine/onix/ui/html"
)

type htmlBridgeState struct {
	context      ecs.SystemContext
	versions     [5]uint64
	scopeVersion uint64
	lastRun      ecs.Tick
	initialized  bool
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

// Relationships are installed after collecting candidates, since changing
// an archetype while iterating the query would invalidate its rows.
type htmlPartBridgeState struct {
	versions     [htmlPartInputCount][3]uint64
	scopeVersion uint64
	initialized  bool
}

type htmlPartInput uint8

const (
	htmlEditableOwnerInput htmlPartInput = iota
	htmlControlTextInput
	htmlEditableTextInput
	htmlControlVisualInput
	htmlEditableVisualInput
	htmlPartInputCount
)

type htmlPartKind uint8

const (
	htmlBridgeEditable htmlPartKind = iota
	htmlBridgeValue
	htmlBridgeLabel
	htmlBridgeCaret
	htmlBridgeSelection
	htmlBridgeTrackHorizontal
	htmlBridgeTrackVertical
	htmlBridgeThumbHorizontal
	htmlBridgeThumbVertical
	htmlBridgeSurface
)

type htmlPartWork struct {
	entity, owner ecs.Entity
	index         int
	kind          htmlPartKind
}

type htmlPartScratch struct{}

func htmlPartBridgeVersions(world *ecs.World) [htmlPartInputCount][3]uint64 {
	editable := htmlBridgeMembership[htmlui.Editable](world)
	control := htmlBridgeMembership[htmlui.Control](world)
	text := htmlBridgeMembership[Relation[scene.Text2D]](world)
	visual := htmlBridgeMembership[Relation[SurfaceStyle]](world)
	return [htmlPartInputCount][3]uint64{
		{editable, htmlBridgeMembership[Control](world)},
		{htmlBridgeMembership[htmlui.ControlText](world), text, control},
		{htmlBridgeMembership[htmlui.EditableText](world), text, editable},
		{htmlBridgeMembership[htmlui.ControlVisual](world), visual, control},
		{htmlBridgeMembership[htmlui.EditableVisual](world), visual, editable},
	}
}

func enrichHTMLParts(world *ecs.World) {
	state := world.ScopeValue[htmlPartBridgeState, htmlPartBridgeState]()
	versions := htmlPartBridgeVersions(world)
	_, scopeVersion := world.QueryScopeKey()
	full := !state.initialized || state.scopeVersion != scopeVersion
	if !full && state.versions == versions {
		return
	}

	work := world.FrameScratch[htmlPartScratch, htmlPartWork]()
	defer work.ReleaseUnused()
	if full || state.versions[htmlEditableOwnerInput] != versions[htmlEditableOwnerInput] {
		for entity := range world.Query[htmlui.Editable](ecs.Without[Control]()).All {
			work.Values = append(work.Values, htmlPartWork{entity: entity, kind: htmlBridgeEditable})
		}
	}
	if full || state.versions[htmlControlTextInput] != versions[htmlControlTextInput] {
		for entity, access := range world.Query[htmlui.ControlText](ecs.Without[Relation[scene.Text2D]]()).All {
			owner := access.Ref().Control
			if control := world.GetRef[htmlui.Control](owner); control != nil {
				kind := htmlBridgeLabel
				if isHTMLTextControl(*control) {
					kind = htmlBridgeValue
				}
				work.Values = append(work.Values, htmlPartWork{entity: entity, owner: owner, kind: kind})
			}
		}
	}
	if full || state.versions[htmlEditableTextInput] != versions[htmlEditableTextInput] {
		for entity, access := range world.Query[htmlui.EditableText](ecs.Without[Relation[scene.Text2D]]()).All {
			owner := access.Ref().Editable
			if world.IsAlive(owner) {
				work.Values = append(work.Values, htmlPartWork{entity: entity, owner: owner, kind: htmlBridgeValue})
			}
		}
	}
	if full || state.versions[htmlControlVisualInput] != versions[htmlControlVisualInput] {
		for entity, access := range world.Query[htmlui.ControlVisual](ecs.Without[Relation[SurfaceStyle]]()).All {
			link := access.Ref()
			if world.IsAlive(link.Control) {
				kind := htmlControlVisualPart(link.Kind, link.Selection)
				work.Values = append(work.Values, htmlPartWork{entity: entity, owner: link.Control, kind: kind, index: link.Index})
			}
		}
	}
	if full || state.versions[htmlEditableVisualInput] != versions[htmlEditableVisualInput] {
		for entity, access := range world.Query[htmlui.EditableVisual](ecs.Without[Relation[SurfaceStyle]]()).All {
			link := access.Ref()
			if world.IsAlive(link.Editable) {
				kind := htmlControlVisualPart(htmlui.ControlVisualCaret, link.Selection)
				work.Values = append(work.Values, htmlPartWork{entity: entity, owner: link.Editable, kind: kind, index: link.Index})
			}
		}
	}
	for _, part := range work.Values {
		applyHTMLPart(world, part)
	}
	if len(work.Values) != 0 {
		versions = htmlPartBridgeVersions(world)
	}
	state.versions, state.scopeVersion, state.initialized = versions, scopeVersion, true
}

func htmlControlVisualPart(kind htmlui.ControlVisualKind, selection bool) htmlPartKind {
	switch kind {
	case htmlui.ControlVisualScrollTrackHorizontal:
		return htmlBridgeTrackHorizontal
	case htmlui.ControlVisualScrollTrackVertical:
		return htmlBridgeTrackVertical
	case htmlui.ControlVisualScrollThumbHorizontal:
		return htmlBridgeThumbHorizontal
	case htmlui.ControlVisualScrollThumbVertical:
		return htmlBridgeThumbVertical
	case htmlui.ControlVisualCaret:
		if selection {
			return htmlBridgeSelection
		}
		return htmlBridgeCaret
	default:
		return htmlBridgeSurface
	}
}

func applyHTMLPart(world *ecs.World, part htmlPartWork) {
	entity := part.entity
	switch part.kind {
	case htmlBridgeEditable:
		_ = world.Insert(entity, Control{})
	case htmlBridgeValue, htmlBridgeLabel:
		if world.Has[Relation[scene.Text2D]](entity) {
			return
		}
		text := Relation[scene.Text2D]{Owner: part.owner}
		control := Relation[Control]{Owner: part.owner}
		if part.kind == htmlBridgeValue {
			_ = world.Insert3(entity, text, control, Part[ValuePart]{})
		} else {
			_ = world.Insert3(entity, text, control, Part[LabelPart]{})
		}
	default:
		if world.Has[Relation[SurfaceStyle]](entity) {
			return
		}
		relation := Relation[SurfaceStyle]{Owner: part.owner}
		switch part.kind {
		case htmlBridgeCaret:
			_ = world.Insert2(entity, relation, Part[CaretPart]{})
		case htmlBridgeSelection:
			_ = world.Insert3(entity, relation, Part[SelectionPart]{}, Indexed[SelectionPart]{Value: part.index})
		case htmlBridgeTrackHorizontal:
			_ = world.Insert2(entity, relation, Part[ScrollTrackHorizontalPart]{})
		case htmlBridgeTrackVertical:
			_ = world.Insert2(entity, relation, Part[ScrollTrackVerticalPart]{})
		case htmlBridgeThumbHorizontal:
			_ = world.Insert2(entity, relation, Part[ScrollThumbHorizontalPart]{})
		case htmlBridgeThumbVertical:
			_ = world.Insert2(entity, relation, Part[ScrollThumbVerticalPart]{})
		case htmlBridgeSurface:
			_ = world.Insert(entity, relation)
		}
	}
}
