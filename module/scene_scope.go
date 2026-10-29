package module

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	"slices"
)

type sceneScopeNode struct {
	entity             ecs.Entity
	scene              *Scene
	owner              *Context
	resolved, visiting bool
	running, rendered  bool
}

// SyncScopes rebuilds cached execution lists only after scene transitions,
// hierarchy edits, ownership changes or entity lifetimes. Unowned engine
// entities are shared; unrelated component migrations preserve scope versions.
func (r *Runtime) SyncScopes() {
	if r.queryDepth != 0 {
		return
	}
	w := r.world
	hierarchy := scene.HierarchyFor(w)
	owners, _ := w.ComponentValueRevision[entityOwner]()
	if !r.scopeDirty && r.scopeHierarchy == hierarchy.Version() && r.scopeOwners == owners {
		return
	}
	r.scopeDirty = false
	if len(r.scopeNodes) < w.EntitySlots() {
		r.scopeNodes = append(r.scopeNodes, make([]sceneScopeNode, w.EntitySlots()-len(r.scopeNodes))...)
	}
	if r.scopeEntities != w.EntityVersion() {
		for i := range r.scopeNodes {
			if r.scopeNodes[i].entity != 0 && !w.IsAlive(r.scopeNodes[i].entity) {
				r.scopeNodes[i] = sceneScopeNode{}
			}
		}
	}
	for _, entity := range hierarchy.Entities() {
		index, _ := entity.Index()
		r.scopeNodes[index].resolved, r.scopeNodes[index].visiting = false, false
	}
	r.despawnEntities = slices.Grow(r.despawnEntities[:0], w.EntityCount())
	r.renderBackground = false
	previousUpdates, previousFrameUpdates := len(r.runningUpdates), len(r.runningFrameUpdates)
	r.runningUpdates = r.runningUpdates[:0]
	r.runningFrameUpdates = r.runningFrameUpdates[:0]
	for _, s := range r.scenes {
		s.entities = s.entities[:0]
		s.context.ownedEntities = s.context.ownedEntities[:0]
		for i := range s.modules {
			s.modules[i].ownedEntities = s.modules[i].ownedEntities[:0]
		}
		r.renderBackground = r.renderBackground || s.rendered && s.state != SceneActive
		if s.state == SceneActive {
			if s.context.update != nil {
				r.runningUpdates = append(r.runningUpdates, &s.context)
			}
			if s.context.frameUpdate != nil {
				r.runningFrameUpdates = append(r.runningFrameUpdates, &s.context)
			}
			for i := range s.modules {
				c := &s.modules[i]
				if c.live && c.enabled && c.update != nil {
					r.runningUpdates = append(r.runningUpdates, c)
				}
				if c.live && c.enabled && c.frameUpdate != nil {
					r.runningFrameUpdates = append(r.runningFrameUpdates, c)
				}
			}
		}
		for _, root := range [2]ecs.Entity{s.uiRoot, s.worldRoot} {
			index, ok := root.Index()
			if ok && w.IsAlive(root) {
				n := &r.scopeNodes[index]
				n.entity, n.scene, n.owner, n.resolved = root, s, nil, true
			}
		}
	}
	if len(r.runningUpdates) < previousUpdates {
		clear(r.runningUpdates[len(r.runningUpdates):previousUpdates])
	}
	if len(r.runningFrameUpdates) < previousFrameUpdates {
		clear(r.runningFrameUpdates[len(r.runningFrameUpdates):previousFrameUpdates])
	}
	r.runningEntities = r.runningEntities[:0]
	r.renderEntities = r.renderEntities[:0]
	runningChanged, renderedChanged := r.scopeEntities != w.EntityVersion(), r.scopeEntities != w.EntityVersion()
	for _, e := range hierarchy.Entities() {
		index, _ := e.Index()
		n := &r.scopeNodes[index]
		known := n.entity == e
		s := r.resolveScope(hierarchy, e)
		if n.owner != nil {
			n.owner.ownedEntities = append(n.owner.ownedEntities, e)
		}
		running, rendered := s == nil, s == nil
		if s != nil {
			s.entities = append(s.entities, e)
			running = s.state == SceneActive
			rendered = s.rendered
			if r.preparation != nil {
				running = s == r.preparation.scene
				rendered = false
			}
		}
		if running {
			r.runningEntities = append(r.runningEntities, e)
		}
		if rendered {
			r.renderEntities = append(r.renderEntities, e)
		}
		runningChanged = runningChanged || !known || n.running != running
		renderedChanged = renderedChanged || !known || n.rendered != rendered
		n.running, n.rendered = running, rendered
	}
	if runningChanged {
		r.runningScope.Reset(r.runningEntities)
	}
	if renderedChanged {
		r.renderScope.Reset(r.renderEntities)
	}
	r.scopeHierarchy, r.scopeOwners, r.scopeEntities = hierarchy.Version(), owners, w.EntityVersion()
	w.SetQueryScope(&r.runningScope)
}

func (r *Runtime) resolveScope(hierarchy *scene.Hierarchy, e ecs.Entity) *Scene {
	if e == 0 || !r.world.IsAlive(e) {
		return nil
	}
	index, _ := e.Index()
	n := &r.scopeNodes[index]
	if n.entity == e && n.resolved {
		return n.scene
	}
	if n.visiting {
		return nil
	}
	n.entity, n.visiting = e, true
	n.scene, n.owner = nil, nil
	if owner, ok := r.world.Get[entityOwner](e); ok {
		n.owner = owner.context
		n.scene = owner.context.scene
	} else if parent := hierarchy.Parent(e); parent != 0 {
		n.scene = r.resolveScope(hierarchy, parent)
		if index, ok := parent.Index(); ok && r.world.IsAlive(parent) {
			n.owner = r.scopeNodes[index].owner
		}
	}
	n.resolved, n.visiting = true, false
	return n.scene
}

// Direct owners and the compiled descendant cache make steady reads constant
// time. New procedural entities resolve their ancestry until the next boundary.
func (r *Runtime) entityContext(e ecs.Entity) *Context {
	if !r.world.IsAlive(e) {
		return nil
	}
	if owner, ok := r.world.Get[entityOwner](e); ok {
		return owner.context
	}
	hierarchy := scene.HierarchyFor(r.world)
	owners, _ := r.world.ComponentValueRevision[entityOwner]()
	if r.scopeHierarchy == hierarchy.Version() && r.scopeOwners == owners {
		index, _ := e.Index()
		if int(index) < len(r.scopeNodes) && r.scopeNodes[index].entity == e {
			return r.scopeNodes[index].owner
		}
	}
	for remaining := r.world.EntityCount(); remaining > 0; remaining-- {
		parent := hierarchy.Parent(e)
		if !r.world.IsAlive(parent) {
			return nil
		}
		e = parent
		if owner, ok := r.world.Get[entityOwner](e); ok {
			return owner.context
		}
	}
	return nil
}

// RenderScope contains visible scenes, including retained overlay backgrounds.
func (r *Runtime) RenderScope() *ecs.QueryScope {
	if !r.renderBackground && r.preparation == nil {
		return &r.runningScope
	}
	return &r.renderScope
}
