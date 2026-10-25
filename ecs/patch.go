package ecs

import (
	"cmp"
	"fmt"
	"slices"
)

type patchPlan struct {
	world       *World
	writes      []patchWrite // sorted IDs with their caller argument positions
	removed     componentMask
	transitions map[uint32]*patchTransition
}

type patchWrite struct {
	id    componentID
	index uint32
}

type patchTransition struct {
	source, target *archetype
	columns        []removalColumn
	removed        []int // indices into columns
	writes         []patchColumn
}

type patchColumn struct {
	source, target column
}

type patchRow struct {
	transition *patchTransition
	index      uint32
	sourceRow  int
	targetRow  int
	tick       Tick
}

func newPatchPlan(world *World, writes []componentID, removed []ComponentType) *patchPlan {
	requireWorld(world)
	world.assertStructuralMutationAllowed()
	entries := make([]patchWrite, len(writes))
	for i, id := range writes {
		entries[i] = patchWrite{id: id, index: uint32(i)}
	}
	slices.SortFunc(entries, func(a, b patchWrite) int { return cmp.Compare(a.id, b.id) })
	for i := 1; i < len(entries); i++ {
		if entries[i-1].id == entries[i].id {
			panic(fmt.Sprintf("ecs: duplicate component %s in patch", world.componentInfos[entries[i].id].typ))
		}
	}
	mask := componentTypeMask(world, removed)
	for _, entry := range entries {
		mask = mask.without(entry.id)
	}
	return &patchPlan{world: world, writes: entries, removed: mask, transitions: make(map[uint32]*patchTransition)}
}

func (p *patchPlan) reserve(capacity int) {
	p.require()
	p.world.assertStructuralMutationAllowed()
	if capacity <= 0 {
		return
	}
	for _, source := range p.world.archetypes {
		// A source with no allocated rows has never been populated or reserved.
		// Do not create speculative transitions and tables for unused shapes.
		if cap(source.entities) == 0 {
			continue
		}
		if target := p.transition(source).target; target != source {
			target.reserve(capacity)
		}
	}
}

func (p *patchPlan) require() {
	if p == nil || p.world == nil {
		panic("ecs: patch is not initialized")
	}
}

func (p *patchPlan) begin(entity Entity) (patchRow, error) {
	p.require()
	w := p.world
	w.assertStructuralMutationAllowed()
	index, meta, ok := w.entityMeta(entity)
	if !ok {
		return patchRow{}, fmt.Errorf("ecs: entity %d does not exist", entity)
	}
	t := p.transition(w.archetypes[meta.archetypeID])
	row := int(meta.row)
	if t.source.hasCleanup {
		for _, index := range t.removed {
			entry := t.columns[index]
			w.cleanupComponent(entry.id, entity, entry.source, row)
		}
	}
	targetRow := row
	if t.source != t.target {
		targetRow = t.target.appendEntity(entity)
	}
	return patchRow{transition: t, index: index, sourceRow: row, targetRow: targetRow, tick: w.mutationTick()}, nil
}

func writePatchColumn[T any](row patchRow, index int, value T) {
	t := row.transition
	entry := t.writes[index]
	target := entry.target.(*typedColumn[T])
	if t.source == t.target {
		target.replace(row.targetRow, value, row.tick)
		return
	}
	added := row.tick
	if entry.source != nil {
		added = entry.source.ticks(filterAdded)[row.sourceRow]
	}
	target.appendWithTicks(value, added, row.tick)
}

func (p *patchPlan) finish(row patchRow) {
	t := row.transition
	if t.source == t.target {
		return
	}
	moveColumns(t.columns, row.sourceRow)
	w := p.world
	w.removeArchetypeEntity(t.source, row.sourceRow)
	w.commitMigration(row.index, t.target, row.targetRow)
}

func (p *patchPlan) transition(source *archetype) *patchTransition {
	if t := p.transitions[source.id]; t != nil {
		return t
	}
	ids := make([]componentID, 0, len(source.ids)+len(p.writes))
	t := &patchTransition{source: source, columns: make([]removalColumn, len(source.ids)), writes: make([]patchColumn, len(p.writes))}
	writeIndex := 0
	for i, id := range source.ids {
		// Source IDs and write IDs are sorted; each cursor only moves forward.
		for writeIndex < len(p.writes) && p.writes[writeIndex].id < id {
			ids = append(ids, p.writes[writeIndex].id)
			writeIndex++
		}
		if writeIndex < len(p.writes) && p.writes[writeIndex].id == id {
			t.writes[p.writes[writeIndex].index].source = source.columns[i]
			writeIndex++
		}
		entry := removalColumn{id: id, source: source.columns[i]}
		if p.removed.has(id) {
			t.removed = append(t.removed, i)
		} else {
			ids = append(ids, id)
		}
		t.columns[i] = entry
	}
	for _, write := range p.writes[writeIndex:] {
		ids = append(ids, write.id)
	}
	t.target = p.world.archetypeForOwned(ids)
	// Bind targets with forward cursors. Overwritten and removed source
	// columns keep nil targets, while writes retain their caller order.
	writeIndex = 0
	sourceIndex := 0
	for i, id := range t.target.ids {
		if writeIndex < len(p.writes) && p.writes[writeIndex].id == id {
			t.writes[p.writes[writeIndex].index].target = t.target.columns[i]
			writeIndex++
			continue
		}
		for source.ids[sourceIndex] < id {
			sourceIndex++
		}
		t.columns[sourceIndex].target = t.target.columns[i]
		sourceIndex++
	}
	p.transitions[source.id] = t
	return t
}

func queuePatch[T commandValue](commands *Commands, entity Entity, plan *patchPlan, value T) {
	if commands == nil {
		panic("ecs: commands is nil")
	}
	plan.require()
	if commands.world != plan.world {
		panic("ecs: patch belongs to a different world")
	}
	queueCommandValue(commands, entity, value)
}
