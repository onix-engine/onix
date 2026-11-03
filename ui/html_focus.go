package ui

import (
	"github.com/diiyw/onix/ecs"
	htmlui "github.com/diiyw/onix/ui/html"
)

type htmlFocusGroup struct {
	owner     ecs.Entity
	tag, kind string
}

type htmlFocusLookupTag struct{}

type htmlFocusLookupState struct {
	version, scopeVersion uint64
	initialized           bool
	ordinals              map[ecs.Entity]int
	groups                map[htmlFocusGroup][]ecs.Entity
	autofocus             []ecs.Entity
}

// Build autofocus candidates and anonymous-element ordinals in one pass.
// Preserve query order, including changes caused by archetype migrations.
func htmlFocusLookup(world *ecs.World) *htmlFocusLookupState {
	lookup := world.ScopeValue[htmlFocusLookupTag, htmlFocusLookupState]()
	version := world.ComponentVersion[htmlui.Focusable]()
	_, scopeVersion := world.QueryScopeKey()
	if lookup.initialized && lookup.version == version && lookup.scopeVersion == scopeVersion {
		return lookup
	}
	lookup.version, lookup.scopeVersion, lookup.initialized = version, scopeVersion, true
	if lookup.ordinals == nil {
		lookup.ordinals = make(map[ecs.Entity]int)
		lookup.groups = make(map[htmlFocusGroup][]ecs.Entity)
	}
	clear(lookup.ordinals)
	for key, entities := range lookup.groups {
		lookup.groups[key] = entities[:0]
	}
	lookup.autofocus = lookup.autofocus[:0]
	for entity, access := range world.Query[htmlui.Focusable]().All {
		value := access.Ref()
		if value.AutoFocus {
			lookup.autofocus = append(lookup.autofocus, entity)
		}
		if value.ID == "" {
			key := htmlFocusGroup{value.Owner, value.Tag, value.Type}
			entities := lookup.groups[key]
			lookup.ordinals[entity] = len(entities)
			lookup.groups[key] = append(entities, entity)
		}
	}
	for key, entities := range lookup.groups {
		if len(entities) == 0 {
			delete(lookup.groups, key)
		}
	}
	return lookup
}
