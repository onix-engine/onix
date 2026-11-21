package scene

import (
	"cmp"
	"slices"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/internal/reuse"
)

const (
	hierarchyActive uint8 = 1 << iota
	hierarchyVisible
	hierarchyEnabled = hierarchyActive | hierarchyVisible
)

const (
	hierarchyResolved uint8 = 1 << iota
	hierarchyVisiting
	hierarchyQueued
	hierarchyDepthResolved
	hierarchyDepthVisiting
	hierarchyOrderQueued
)

type hierarchyNode struct {
	entity, parent, firstChild, nextSibling, previousSibling ecs.Entity
	depth                                                    uint32
	local, inherited, state                                  uint8
}

type hierarchySibling struct {
	entity ecs.Entity
	order  SiblingOrder
}

// Hierarchy is the world-owned parent/child index shared by scene systems.
// It includes suspended scenes and never retains component storage pointers.
type Hierarchy struct {
	missingParents          map[ecs.Entity]int
	changes                 ecs.SystemContext
	entities, queue, path   []ecs.Entity
	entityChanges, ordered  []ecs.Entity
	siblings                []hierarchySibling
	nodes                   []hierarchyNode
	entityPositions         []uint32
	retention               [7]reuse.Retention
	versions, detached      [4]uint64
	version                 uint64
	mutationVersion         uint64
	entityVersion           uint64
	invalidated, recomputed int
	lastRun                 ecs.Tick
	initialized             bool
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

// WalkSubtree visits root and its descendants in sibling order. Returning
// false skips that entity's descendants. The visitor may read the hierarchy;
// collect entities before changing parentage or despawning them.
func (h *Hierarchy) WalkSubtree(root ecs.Entity, visit func(ecs.Entity) bool) {
	if root == 0 || !visit(root) {
		return
	}
	if h.missingParents[root] != 0 && h.node(root) == nil {
		// A removed or reserved root can still be referenced by live children.
		for _, entity := range h.entities {
			if h.Parent(entity) == root && visit(entity) {
				h.walkChildren(entity, root, visit)
			}
		}
		return
	}
	h.walkChildren(root, root, visit)
}

func (h *Hierarchy) walkChildren(entity, root ecs.Entity, visit func(ecs.Entity) bool) {
	for child := h.FirstChild(entity); child != 0; child = h.NextSibling(child) {
		// Each entity has one parent, so any reachable cycle closes at the root.
		if child != root && visit(child) {
			h.walkChildren(child, root, visit)
		}
	}
}

// FirstChild and NextSibling traverse ascending SiblingOrder with entity ties.
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

// Depth returns the number of live ancestors, including ancestors outside the
// current query scope. Missing entities and cycles return -1.
func (h *Hierarchy) Depth(entity ecs.Entity) int {
	h.path = h.path[:0]
	depth, cyclic := -1, false
	for n := h.node(entity); n != nil; n = h.node(n.parent) {
		if n.state&hierarchyDepthResolved != 0 {
			cyclic = n.depth == ^uint32(0)
			depth = int(n.depth)
			break
		}
		if n.state&hierarchyDepthVisiting != 0 {
			cyclic = true
			break
		}
		n.state |= hierarchyDepthVisiting
		h.path = append(h.path, n.entity)
	}
	for i := len(h.path) - 1; i >= 0; i-- {
		n := h.node(h.path[i])
		if cyclic {
			n.depth = ^uint32(0)
		} else {
			depth++
			n.depth = uint32(depth)
		}
		n.state = n.state&^hierarchyDepthVisiting | hierarchyDepthResolved
	}
	if cyclic {
		return -1
	}
	return depth
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
	versions := [4]uint64{componentValueVersion[Parent](world), componentValueVersion[Activity](world), componentValueVersion[Visibility](world), componentValueVersion[SiblingOrder](world)}
	detached := [4]uint64{world.ComponentDetachRevision[Parent](), world.ComponentDetachRevision[Activity](), world.ComponentDetachRevision[Visibility](), world.ComponentDetachRevision[SiblingOrder]()}
	h.invalidated, h.recomputed = 0, 0
	if !h.initialized {
		world.EntityChangesSince(world.EntityVersion(), nil)
		h.rebuild(world)
	} else if h.entityVersion != world.EntityVersion() && !h.syncEntities(world) {
		h.rebuild(world)
	} else {
		// Include the previous tick because callers can read the hierarchy
		// between writes performed by systems sharing the same change tick.
		h.changes = world.ChangesSince(h.lastRun - 1)
		changes := &h.changes
		if h.versions[0] != versions[0] {
			if h.detached[0] != detached[0] {
				for _, entity := range h.entities {
					h.refreshParent(world, entity)
				}
			} else {
				for entity := range world.Query[Parent](ecs.Unscoped(), changes.Changed[Parent]()).All {
					h.refreshParent(world, entity)
				}
			}
		}
		if h.versions[3] != versions[3] {
			if h.detached[3] != detached[3] {
				for _, entity := range h.entities {
					h.queueSiblingOrder(h.node(entity).parent)
				}
			} else {
				for entity := range world.Query[SiblingOrder](ecs.Unscoped(), changes.Changed[SiblingOrder]()).All {
					h.queueSiblingOrder(h.node(entity).parent)
				}
			}
		}
		h.refreshFlags[Activity](world, changes, versions, detached, 1)
		h.refreshFlags[Visibility](world, changes, versions, detached, 2)
		for i := 0; i < len(h.queue); i++ {
			n := h.node(h.queue[i])
			n.state &^= hierarchyResolved | hierarchyDepthResolved
			for child := n.firstChild; child != 0; child = h.NextSibling(child) {
				h.mark(child)
			}
		}
		h.invalidated = len(h.queue)
		for _, entity := range h.queue {
			h.node(entity).state &^= hierarchyQueued
		}
		h.queue = h.queue[:0]
	}
	h.retention[5].Record(len(h.ordered))
	for _, entity := range h.ordered {
		if parent := h.node(entity); parent != nil {
			parent.state &^= hierarchyOrderQueued
			h.sortChildren(world, parent)
		}
	}
	h.ordered = h.ordered[:0]
	h.versions, h.detached = versions, detached
	h.entityVersion, h.mutationVersion = world.EntityVersion(), mutation
	h.lastRun, h.initialized = world.ChangeTick(), true
}

func (h *Hierarchy) syncEntities(world *ecs.World) bool {
	var complete bool
	h.entityChanges, complete = world.EntityChangesSince(h.entityVersion, h.entityChanges[:0])
	if !complete {
		return false
	}
	// An existing child may refer to a previously reserved parent. Resolve
	// these rare batches with the full linker so it can find those children.
	for _, entity := range h.entityChanges {
		if h.missingParents[entity] != 0 && world.IsAlive(entity) {
			return false
		}
	}
	for _, entity := range h.entityChanges {
		if node := h.node(entity); node != nil && !world.IsAlive(entity) {
			h.unlink(node)
			for child := node.firstChild; child != 0; {
				n := h.node(child)
				next := n.nextSibling
				n.previousSibling, n.nextSibling = 0, 0
				h.recordMissingParent(entity)
				if world.IsAlive(child) {
					h.mark(child)
				}
				child = next
			}
			index, _ := entity.Index()
			position := h.entityPositions[index]
			last := len(h.entities) - 1
			moved := h.entities[last]
			movedIndex, _ := moved.Index()
			h.entities[position], h.entityPositions[movedIndex] = moved, position
			h.entities = h.entities[:last]
			*node = hierarchyNode{}
		}
	}
	first := len(h.entities)
	for _, entity := range h.entityChanges {
		if h.node(entity) == nil && world.IsAlive(entity) {
			h.addEntity(world, entity)
		}
	}
	for _, entity := range h.entities[first:] {
		h.link(h.node(entity))
		h.mark(entity)
	}
	h.version++
	return true
}

func (h *Hierarchy) rebuild(world *ecs.World) {
	for _, entity := range h.entities {
		index, _ := entity.Index()
		h.nodes[index] = hierarchyNode{}
	}

	h.entities = h.entities[:0]
	h.ordered = h.ordered[:0]
	clear(h.missingParents)
	for entity := range world.Entities() {
		h.addEntity(world, entity)
	}
	for _, entity := range h.entities {
		h.link(h.node(entity))
	}
	h.version++
	h.invalidated = len(h.entities)
}

func (h *Hierarchy) addEntity(world *ecs.World, entity ecs.Entity) {
	index, _ := entity.Index()
	if int(index) >= len(h.nodes) {
		grow := int(index) + 1 - len(h.nodes)
		h.nodes = append(h.nodes, make([]hierarchyNode, grow)...)
		h.entityPositions = append(h.entityPositions, make([]uint32, grow)...)
	}
	parent, _ := world.Get[Parent](entity)
	h.nodes[index] = hierarchyNode{entity: entity, parent: parent.Entity, local: hierarchyLocalFlags(world, entity)}
	h.entityPositions[index] = uint32(len(h.entities))
	h.entities = append(h.entities, entity)
}

func (h *Hierarchy) link(n *hierarchyNode) {
	if parent := h.node(n.parent); parent != nil {
		n.nextSibling = parent.firstChild
		if next := h.node(n.nextSibling); next != nil {
			next.previousSibling = n.entity
		}
		parent.firstChild = n.entity
		h.queueSiblingOrder(n.parent)
	} else if n.parent != 0 {
		h.recordMissingParent(n.parent)
	}
}

func (h *Hierarchy) queueSiblingOrder(entity ecs.Entity) {
	parent := h.node(entity)
	if parent == nil || parent.state&hierarchyOrderQueued != 0 || h.NextSibling(parent.firstChild) == 0 {
		return
	}
	parent.state |= hierarchyOrderQueued
	h.ordered = append(h.ordered, entity)
}

func (h *Hierarchy) sortChildren(world *ecs.World, parent *hierarchyNode) {
	siblings := h.siblings[:0]
	for child := parent.firstChild; child != 0; child = h.NextSibling(child) {
		order, explicit := world.Get[SiblingOrder](child)
		if !explicit {
			order = SiblingOrder(child)
		}
		siblings = append(siblings, hierarchySibling{entity: child, order: order})
	}
	h.retention[6].Record(len(siblings))
	h.siblings = siblings
	if len(siblings) <= 1 {
		return
	}
	slices.SortFunc(siblings, func(a, b hierarchySibling) int {
		if order := cmp.Compare(a.order, b.order); order != 0 {
			return order
		}
		return cmp.Compare(a.entity, b.entity)
	})
	parent.firstChild = siblings[0].entity
	var previous *hierarchyNode
	for _, sibling := range siblings {
		current := h.node(sibling.entity)
		current.previousSibling, current.nextSibling = 0, 0
		if previous != nil {
			previous.nextSibling, current.previousSibling = current.entity, previous.entity
		}
		previous = current
	}
}

func (h *Hierarchy) recordMissingParent(parent ecs.Entity) {
	if h.missingParents == nil {
		h.missingParents = make(map[ecs.Entity]int)
	}
	h.missingParents[parent]++
}

func (h *Hierarchy) unlink(n *hierarchyNode) {
	if previous := h.node(n.previousSibling); previous != nil {
		previous.nextSibling = n.nextSibling
	} else if old := h.node(n.parent); old != nil {
		old.firstChild = n.nextSibling
	} else if count := h.missingParents[n.parent]; count > 1 {
		h.missingParents[n.parent] = count - 1
	} else if count == 1 {
		delete(h.missingParents, n.parent)
	}
	if next := h.node(n.nextSibling); next != nil {
		next.previousSibling = n.previousSibling
	}
	n.previousSibling, n.nextSibling = 0, 0
}

func (h *Hierarchy) refreshParent(world *ecs.World, entity ecs.Entity) {
	n := h.node(entity)
	parent, _ := world.Get[Parent](entity)
	if n.parent == parent.Entity {
		return
	}
	h.unlink(n)
	n.parent = parent.Entity
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

func (h *Hierarchy) refreshFlags[T any](world *ecs.World, changes *ecs.SystemContext, versions, detached [4]uint64, index int) {
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
	if h.detached[index] != detached[index] {
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
	if n := h.node(entity); n != nil && n.state&hierarchyQueued == 0 {
		n.state |= hierarchyQueued
		h.queue = append(h.queue, entity)
	}
}

func (h *Hierarchy) flags(entity ecs.Entity) uint8 {
	h.path = h.path[:0]
	flags := uint8(hierarchyEnabled)
	for n := h.node(entity); n != nil; n = h.node(n.parent) {
		if n.state&hierarchyResolved != 0 {
			flags = n.inherited
			break
		}
		if n.state&hierarchyVisiting != 0 {
			flags = 0
			break
		}
		n.state |= hierarchyVisiting
		h.path = append(h.path, n.entity)
	}
	for i := len(h.path) - 1; i >= 0; i-- {
		n := h.node(h.path[i])
		flags &= n.local
		n.inherited = flags
		n.state = n.state&^hierarchyVisiting | hierarchyResolved
		h.recomputed++
	}
	return flags
}

func (h *Hierarchy) MaintainCache() {
	h.entities = h.retention[0].Trim(h.entities, 64)
	h.queue = h.retention[1].Trim(h.queue[:0], 64)
	h.path = h.retention[2].Trim(h.path[:0], 64)
	length := len(h.nodes)
	for length > 0 && h.nodes[length-1].entity == 0 {
		length--
	}
	h.nodes = h.retention[3].Trim(h.nodes[:length], 64)
	h.entityPositions = h.retention[4].Trim(h.entityPositions[:length], 64)
	h.ordered = h.retention[5].Trim(h.ordered[:0], 64)
	h.siblings = h.retention[6].Trim(h.siblings[:0], 16)
}
