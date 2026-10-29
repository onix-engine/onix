package module

// ManagedResource belongs to one module and changes activity only at committed
// scene transitions. Heavy platform backends live in opt-in adapter packages.
type ManagedResource interface {
	Activate(bool) error
	Close() error
}

type ownedResource interface {
	ManagedResource
	comparable
}

func (c *Context) Own[T ownedResource](resource T) error {
	if !c.usable() {
		return ErrDisposed
	}
	c.managed = append(c.managed, resource)
	return nil
}

// Disown removes an explicitly closed resource from lifecycle bookkeeping.
func (c *Context) Disown[T ownedResource](resource T) {
	if c == nil {
		return
	}
	for i, current := range c.managed {
		if current == resource {
			copy(c.managed[i:], c.managed[i+1:])
			c.managed[len(c.managed)-1] = nil
			c.managed = c.managed[:len(c.managed)-1]
			return
		}
	}
}

func (c *Context) Alive() bool  { return c.usable() }
func (c *Context) Active() bool { return c.usable() && c.enabled && c.scene.state == SceneActive }

func syncSceneResources(s *Scene, active bool) error {
	var err error
	for i := -1; i < len(s.modules); i++ {
		c := &s.context
		if i >= 0 {
			c = &s.modules[i]
		}
		for _, resource := range c.managed {
			err = joinError(err, resource.Activate(active && c.enabled))
		}
	}
	return err
}
