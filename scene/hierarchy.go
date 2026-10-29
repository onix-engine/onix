package scene

import "github.com/diiyw/onix/ecs"

const (
	hierarchyActive uint8 = 1 << iota
	hierarchyVisible
	hierarchyEnabled = hierarchyActive | hierarchyVisible
)

type hierarchyNode struct {
	entity, parent, firstChild, nextSibling, previousSibling ecs.Entity
	local, inherited                                         uint8
	valid, visiting, queued                                  bool
}

// Hierarchy is the world-owned parent/child index shared by scene systems.
// It includes suspended scenes and never retains component storage pointers.
type Hierarchy struct {
	nodes                   []hierarchyNode
	entities, queue, path   []ecs.Entity
	versions, members       [3]uint64
	version, entityVersion  uint64
	mutationVersion         uint64
	lastRun                 ecs.Tick
	changes                 ecs.SystemContext
	initialized             bool
	invalidated, recomputed int
}

// HierarchyFor synchronizes topology and invalidates inherited state only in
// affected subtrees. Borrowed lists remain valid until the next synchronization.
func HierarchyFor(world *ecs.World) *Hierarchy {
	h := world.FrameValue[Hierarchy, Hierarchy]()
	h.sync(world)
	return h
}

// Version changes for live entity membership or an actual parent change.
func (h *Hierarchy) Version() uint64 { return h.version }

// Entities returns the borrowed live entity list, independent of query scopes.
func (h *Hierarchy) Entities() []ecs.Entity { return h.entities }

func (h *Hierarchy) Parent(entity ecs.Entity) ecs.Entity {
	if n := h.node(entity); n != nil {
		return n.parent
	}
	return 0
}

func (h *Hierarchy) FirstChild(entity ecs.Entity) ecs.Entity {
	if n := h.node(entity); n != nil {
		return n.firstChild
	}
	return 0
}

func (h *Hierarchy) NextSibling(entity ecs.Entity) ecs.Entity {
	if n := h.node(entity); n != nil {
		return n.nextSibling
	}
	return 0
}

func (h *Hierarchy) node(entity ecs.Entity) *hierarchyNode {
	index, ok := entity.Index()
	if !ok || int(index) >= len(h.nodes) || h.nodes[index].entity != entity {
		return nil
	}
	return &h.nodes[index]
}

func (h *Hierarchy) sync(world *ecs.World) {
	mutation := world.MutationVersion()
	if h.initialized && h.mutationVersion == mutation {
		return
	}
	var versions, members [3]uint64
	versions[0], members[0] = world.ComponentValueRevision[Parent]()
	versions[1], members[1] = world.ComponentValueRevision[Activity]()
	versions[2], members[2] = world.ComponentValueRevision[Visibility]()
	h.invalidated, h.recomputed = 0, 0
	if !h.initialized || h.entityVersion != world.EntityVersion() {
		h.rebuild(world)
	} else {
		since := h.lastRun
		if since == world.ChangeTick() {
			since--
		}
		h.changes = world.ChangesSince(since)
		changes := &h.changes
		if h.versions[0] != versions[0] {
			if h.members[0] != members[0] {
				for _, entity := range h.entities {
					h.refreshParent(world, entity)
				}
			} else {
				for entity := range world.Query[Parent](ecs.Unscoped(), changes.Changed[Parent]()).All {
					h.refreshParent(world, entity)
				}
			}
		}
		h.refreshFlags[Activity](world, changes, versions, members, 1)
		h.refreshFlags[Visibility](world, changes, versions, members, 2)
		for i := 0; i < len(h.queue); i++ {
			n := h.node(h.queue[i])
			n.valid = false
			for child := n.firstChild; child != 0; child = h.NextSibling(child) {
				h.mark(child)
			}
		}
		h.invalidated = len(h.queue)
		for _, entity := range h.queue {
			h.node(entity).queued = false
		}
		h.queue = h.queue[:0]
	}
	h.versions, h.members = versions, members
	h.entityVersion, h.mutationVersion = world.EntityVersion(), mutation
	h.lastRun, h.initialized = world.ChangeTick(), true
}

func (h *Hierarchy) rebuild(world *ecs.World) {
	for _, entity := range h.entities {
		index, _ := entity.Index()
		h.nodes[index] = hierarchyNode{}
	}
	if len(h.nodes) < world.EntitySlots() {
		h.nodes = append(h.nodes, make([]hierarchyNode, world.EntitySlots()-len(h.nodes))...)
	}
	h.entities = h.entities[:0]
	for entity := range world.Entities() {
		index, _ := entity.Index()
		parent, _ := world.Get[Parent](entity)
		h.nodes[index] = hierarchyNode{entity: entity, parent: parent.Entity, local: hierarchyLocalFlags(world, entity)}
		h.entities = append(h.entities, entity)
	}
	for _, entity := range h.entities {
		h.link(h.node(entity))
	}
	h.version++
	h.invalidated = len(h.entities)
}

func (h *Hierarchy) link(n *hierarchyNode) {
	if parent := h.node(n.parent); parent != nil {
		n.nextSibling = parent.firstChild
		if next := h.node(n.nextSibling); next != nil {
			next.previousSibling = n.entity
		}
		parent.firstChild = n.entity
	}
}

func (h *Hierarchy) refreshParent(world *ecs.World, entity ecs.Entity) {
	n := h.node(entity)
	parent, _ := world.Get[Parent](entity)
	if n.parent == parent.Entity {
		return
	}
	if previous := h.node(n.previousSibling); previous != nil {
		previous.nextSibling = n.nextSibling
	} else if old := h.node(n.parent); old != nil {
		old.firstChild = n.nextSibling
	}
	if next := h.node(n.nextSibling); next != nil {
		next.previousSibling = n.previousSibling
	}
	n.parent, n.previousSibling, n.nextSibling = parent.Entity, 0, 0
	h.link(n)
	h.mark(entity)
	h.version++
}

func hierarchyLocalFlags(world *ecs.World, entity ecs.Entity) uint8 {
	flags := uint8(hierarchyEnabled)
	if value := world.GetRef[Activity](entity); value != nil && !value.Active {
		flags &^= hierarchyActive
	}
	if value := world.GetRef[Visibility](entity); value != nil && !value.Visible {
		flags &^= hierarchyVisible
	}
	return flags
}

func (h *Hierarchy) refreshFlags[T any](world *ecs.World, changes *ecs.SystemContext, versions, members [3]uint64, index int) {
	if h.versions[index] == versions[index] {
		return
	}
	refresh := func(entity ecs.Entity) {
		n := h.node(entity)
		local := hierarchyLocalFlags(world, entity)
		if n.local != local {
			n.local = local
			h.mark(entity)
		}
	}
	if h.members[index] != members[index] {
		for _, entity := range h.entities {
			refresh(entity)
		}
	} else {
		for entity := range world.Query[T](ecs.Unscoped(), changes.Changed[T]()).All {
			refresh(entity)
		}
	}
}

func (h *Hierarchy) mark(entity ecs.Entity) {
	if n := h.node(entity); n != nil && !n.queued {
		n.queued = true
		h.queue = append(h.queue, entity)
	}
}

func (h *Hierarchy) flags(entity ecs.Entity) uint8 {
	h.path = h.path[:0]
	flags := uint8(hierarchyEnabled)
	for n := h.node(entity); n != nil; n = h.node(n.parent) {
		if n.valid {
			flags = n.inherited
			break
		}
		if n.visiting {
			flags = 0
			break
		}
		n.visiting = true
		h.path = append(h.path, n.entity)
	}
	for i := len(h.path) - 1; i >= 0; i-- {
		n := h.node(h.path[i])
		flags &= n.local
		n.inherited, n.valid, n.visiting = flags, true, false
		h.recomputed++
	}
	return flags
}
