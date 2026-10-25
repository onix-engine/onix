package module

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	"slices"
)

type sceneScopeNode struct {
	entity                          ecs.Entity
	scene                           *Scene
	owner                           *Context
	parent, firstChild, nextSibling ecs.Entity
	resolved                        bool
	visiting                        bool
}

// SyncScopes rebuilds cached execution lists only after scene transitions,
// hierarchy edits or structural changes. Unowned engine entities are shared.
func (r *Runtime) SyncScopes() {
	if r.queryDepth != 0 {
		return
	}
	w := r.world
	parents := ecs.ComponentVersion[scene.Parent](w)
	if !r.scopeDirty && r.scopeStructure == w.StructureVersion() && r.scopeParents == parents {
		return
	}
	r.scopeDirty = false
	if len(r.scopeNodes) < w.EntitySlots() {
		r.scopeNodes = append(r.scopeNodes, make([]sceneScopeNode, w.EntitySlots()-len(r.scopeNodes))...)
	}
	clear(r.scopeNodes)
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
				r.scopeNodes[index] = sceneScopeNode{entity: root, scene: s, resolved: true}
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
	for e := range w.Entities() {
		s := r.resolveScope(e)
		index, _ := e.Index()
		n := &r.scopeNodes[index]
		if n.owner != nil {
			n.owner.ownedEntities = append(n.owner.ownedEntities, e)
		}
		if parent, ok := ecs.Get[scene.Parent](w, e); ok && w.IsAlive(parent.Entity) {
			pindex, _ := parent.Entity.Index()
			n.parent, n.nextSibling = parent.Entity, r.scopeNodes[pindex].firstChild
			r.scopeNodes[pindex].firstChild = e
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
	}
	r.runningScope.Reset(r.runningEntities)
	r.renderScope.Reset(r.renderEntities)
	r.scopeStructure, r.scopeParents = w.StructureVersion(), parents
	w.SetQueryScope(&r.runningScope)
}

func (r *Runtime) resolveScope(e ecs.Entity) *Scene {
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
	if owner, ok := ecs.Get[entityOwner](r.world, e); ok {
		n.owner = owner.context
		n.scene = owner.context.scene
	} else if parent, ok := ecs.Get[scene.Parent](r.world, e); ok {
		n.scene = r.resolveScope(parent.Entity)
		if index, ok := parent.Entity.Index(); ok && r.world.IsAlive(parent.Entity) {
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
	if owner, ok := ecs.Get[entityOwner](r.world, e); ok {
		return owner.context
	}
	if r.scopeStructure == r.world.StructureVersion() && r.scopeParents == ecs.ComponentVersion[scene.Parent](r.world) {
		index, _ := e.Index()
		if int(index) < len(r.scopeNodes) && r.scopeNodes[index].entity == e {
			return r.scopeNodes[index].owner
		}
	}
	for remaining := r.world.EntityCount(); remaining > 0; remaining-- {
		parent, ok := ecs.Get[scene.Parent](r.world, e)
		if !ok || !r.world.IsAlive(parent.Entity) {
			return nil
		}
		e = parent.Entity
		if owner, ok := ecs.Get[entityOwner](r.world, e); ok {
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
