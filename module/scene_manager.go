package module

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
)

type SceneState uint8

const (
	SceneLoading SceneState = iota
	SceneReady
	SceneActive
	SceneSuspended
	SceneDisposed
)

var ErrStartupLimit = errors.New("module: scene startup did not settle within its batch/event budget")

type switchCommand struct {
	source  *Scene
	target  *Scene
	request SceneSwitchRequest
}
type SceneManager struct {
	runtime   *Runtime
	active    *Scene
	switches  []switchCommand
	switching bool
	nextLayer uint64
	preload   *ScenePreloader
}

func (r *Runtime) Manager() *SceneManager { return &r.manager }
func (m *SceneManager) Active() *Scene    { return m.active }
func (s *Scene) State() SceneState {
	if s == nil {
		return SceneDisposed
	}
	return s.state
}

func (m *SceneManager) StartEntry() (*Scene, error) {
	if m.runtime.closed {
		return nil, ErrDisposed
	}
	if m.active != nil || m.runtime.busy || m.runtime.loading || m.switching || m.preload != nil {
		return nil, ErrBusy
	}
	m.switching = true
	defer func() { m.switching = false }()
	var s *Scene
	for _, candidate := range m.runtime.scenes {
		if candidate.ID() == m.runtime.plan.Spec.Entry && candidate.state == SceneReady {
			s = candidate
			break
		}
	}
	created := s == nil
	var err error
	if created {
		s, err = m.prepare(m.runtime.plan.Spec.Entry, SceneStartup{})
	}
	if err != nil {
		return nil, err
	}
	if _, err = m.prepareScene(s, s.ID(), s.startup); err != nil {
		if created {
			err = joinError(err, s.Dispose())
		}
		return nil, err
	}
	if err = prepareSceneGPU(s); err != nil {
		if created {
			err = joinError(err, s.Dispose())
		}
		return nil, err
	}
	m.activate(s, false)
	return s, syncSceneResources(s, true)
}

// Preload is the synchronous convenience form of BeginPreload and Advance.
// Interactive loaders use BeginPreload to budget main-thread work per frame.
func (m *SceneManager) Preload(request ScenePreloadRequest) (*Scene, error) {
	p, err := m.BeginPreload(request)
	if err != nil {
		return nil, err
	}
	if err := p.cpu.WaitCPU(context.Background()); err != nil {
		p.Discard()
		return nil, err
	}
	for {
		ready, err := p.Advance(time.Hour)
		if err != nil {
			return nil, err
		}
		if ready {
			return p.scene, nil
		}
	}
}

// Activate queues the exact prepared instance. Disposal before the commit
// produces a failed switch instead of silently loading another instance.
func (m *SceneManager) Activate(target *Scene, options SceneActivationOptions) error {
	if m.runtime.closed {
		return ErrDisposed
	}
	if target == nil || target.runtime != m.runtime {
		return ErrOwnership
	}
	if target.state != SceneReady && target.state != SceneSuspended {
		return ErrUnavailable
	}
	if device := graphics.DeviceFor(m.runtime.world); device.Driver() != nil && !target.GPUReadyFor(device) {
		return ErrUnavailable
	}
	if m.switching || m.runtime.preparation != nil || m.preload != nil {
		return ErrBusy
	}
	if err := m.enqueue(m.active, SceneSwitchRequest{Scene: target.ID(), Retain: options.Retain, Overlay: options.Overlay, Startup: options.Startup}); err != nil {
		return err
	}
	m.switches[len(m.switches)-1].target = target
	return nil
}

// Switch commits at the next event/update structural boundary. A ready target
// activates without repeating installation or startup resource loading.
func (m *SceneManager) Switch(request SceneSwitchRequest) error {
	if m.runtime.closed {
		return ErrDisposed
	}
	if m.switching || m.runtime.preparation != nil || m.preload != nil {
		return ErrBusy
	}
	return m.enqueue(m.active, request)
}
func (m *SceneManager) enqueue(source *Scene, request SceneSwitchRequest) error {
	if m.preload != nil {
		return ErrBusy
	}
	if source != m.active || source != nil && (source.state != SceneActive || !source.Alive()) {
		return ErrUnavailable
	}
	if len(m.switches) >= m.runtime.options.CommandCapacity {
		return ErrCommandFull
	}
	// Startup slices must outlive the batch: copy once at the cold switch edge.
	request.Startup = cloneValue(request.Startup)
	m.switches = append(m.switches, switchCommand{source: source, request: request})
	if m.runtime.options.Wake != nil {
		m.runtime.options.Wake()
	}
	return nil
}

func (m *SceneManager) flush() error {
	if len(m.switches) == 0 {
		return nil
	}
	m.switching = true
	var result error
	for _, command := range m.switches {
		if command.source != m.active || command.source != nil && (command.source.state != SceneActive || !command.source.Alive()) {
			continue
		}
		result = joinError(result, m.apply(command))
	}
	clear(m.switches)
	m.switches = m.switches[:0]
	m.switching = false
	return result
}

func (m *SceneManager) apply(command switchCommand) error {
	r := m.runtime
	old := command.source
	var from project.SceneID
	if old != nil {
		from = old.ID()
	}
	fail := func(err error) error {
		if old == nil {
			return err
		}
		return m.result(old, SceneSwitchResult{From: from, To: command.request.Scene, Err: err})
	}
	request := command.request
	if old != nil {
		switch old.plan.Spec.ExitPolicy {
		case project.SceneExitDestroy:
			request.Retain = false
		case project.SceneExitRetain:
			request.Retain = true
		}
	}
	if request.Overlay {
		request.Retain = true
	}
	target := command.target
	if target != nil && (!target.Alive() || target.state != SceneReady && target.state != SceneSuspended) {
		return fail(ErrUnavailable)
	}
	for _, s := range r.scenes {
		if target != nil {
			break
		}
		if s.ID() == request.Scene && (s.state == SceneReady || s.state == SceneSuspended || s == old) {
			target = s
			break
		}
	}
	var err error
	created := target == nil
	if target == nil {
		target, err = m.prepare(request.Scene, SceneStartup{From: from, Value: request.Startup})
	}
	if err == nil && target != old {
		value := request.Startup
		if value.Type == "" && len(value.Fields) == 0 {
			value = target.startup.Value
		}
		target, err = m.prepareScene(target, target.ID(), SceneStartup{From: from, Value: value})
	}
	if err == nil {
		err = prepareSceneGPU(target)
	}
	if err != nil {
		// Report a business failure on the channel; do not abort the App tick.
		if created && target != nil && target.Alive() {
			err = joinError(err, target.Dispose())
		}
		return fail(err)
	}
	if target == old {
		return m.result(old, SceneSwitchResult{From: from, To: target.ID()})
	}
	// Exit is an explicit foreground lifetime edge; Suspend additionally marks
	// a retained instance. Hooks run before changing the committed foreground.
	if old != nil {
		r.busy = true
		err = m.leave(old, request.Retain)
		r.busy = false
	}
	if err != nil {
		if created {
			err = joinError(err, target.Dispose())
		}
		return fail(err)
	}
	if !request.Overlay {
		for _, s := range r.scenes {
			if s != target && s != old && s.rendered {
				s.rendered = false
				m.visibility(s, false)
			}
		}
	}
	if old != nil && request.Retain {
		old.state = SceneSuspended
		old.rendered = request.Overlay
		if request.Overlay {
			m.activity(old, false)
		} else {
			m.visibility(old, false)
		}
		err = joinError(err, syncSceneResources(old, false))
		for index := r.pendingFirst; index >= 0; {
			p := &r.pending[index]
			next := p.next
			if p.source.scene == old {
				r.releasePending(index)
			} else if p.target.scene == old && !p.replied {
				p.failure = ErrUnavailable
				p.failureTick = r.tick + 1
			}
			index = next
		}
	} else if old != nil {
		r.busy = true
		err = joinError(err, r.disposeScene(old))
		r.busy = false
		err = joinError(err, r.flushStructural())
	}
	m.activate(target, request.Overlay)
	err = joinError(err, syncSceneResources(target, true))
	r.scopeDirty = true
	r.SyncScopes()
	if r.orderDirty {
		err = joinError(err, r.syncDrawOrder())
		r.orderDirty = false
	}
	err = joinError(err, m.result(target, SceneSwitchResult{From: from, To: target.ID()}))
	if old != nil && request.Retain {
		err = joinError(err, m.result(old, SceneSwitchResult{From: from, To: target.ID()}))
	}
	return err
}

func (m *SceneManager) activate(s *Scene, overlay bool) {
	m.nextLayer++
	s.layer.Order = m.nextLayer
	s.layer.Overlay = overlay
	_ = m.runtime.world.Insert(s.uiRoot, s.layer)
	_ = m.runtime.world.Insert(s.worldRoot, s.layer)
	m.visibility(s, true)
	s.state, s.rendered = SceneActive, true
	s.foreground = true
	m.active = s
	m.runtime.scopeDirty = true
	m.runtime.SyncScopes()
}

func (m *SceneManager) leave(s *Scene, retain bool) error {
	var err error
	for i := len(s.modules) - 1; i >= -1; i-- {
		c := &s.context
		if i >= 0 {
			c = &s.modules[i]
		}
		if !c.live || c.disposing {
			continue
		}
		if c.exit != nil {
			err = joinError(err, c.exit())
		}
		if retain && c.suspend != nil {
			err = joinError(err, c.suspend())
		}
	}
	if err == nil {
		s.foreground = false
	}
	return err
}

func (m *SceneManager) activity(s *Scene, active bool) {
	_ = m.runtime.world.Insert(s.uiRoot, scene.Activity{Active: active})
	_ = m.runtime.world.Insert(s.worldRoot, scene.Activity{Active: active})
}

func (m *SceneManager) visibility(s *Scene, active bool) {
	m.activity(s, active)
	for i := range s.modules {
		c := &s.modules[i]
		if !active && c.pausedVisibility == nil {
			c.pausedVisibility = make(map[ecs.Entity]scene.Visibility, len(c.entities))
		}
		for _, e := range c.entities {
			if active {
				if v, ok := c.pausedVisibility[e]; ok {
					_ = m.runtime.world.Insert(e, v)
					delete(c.pausedVisibility, e)
				}
			} else if _, saved := c.pausedVisibility[e]; !saved {
				c.pausedVisibility[e], _ = m.runtime.world.Get[scene.Visibility](e)
				_ = m.runtime.world.Insert(e, scene.Visibility{})
			}
		}
		if active {
			clear(c.pausedVisibility)
		}
	}
}

func (r *Runtime) engineSend[T any](s *Scene, topic string, value T) error {
	return r.enqueue(r.typedStore[T](), value, envelope{kind: engineEvent, source: &s.context, target: &s.context, topic: topic})
}
func (m *SceneManager) startup(s *Scene, value SceneStartup) error {
	return m.runtime.engineSend(s, "scene.startup", value)
}
func (m *SceneManager) result(s *Scene, value SceneSwitchResult) error {
	return m.runtime.engineSend(s, "scene.switch.result", value)
}

// Preparation parks the active queues and pending table. Separate payload banks
// keep startup traffic from overwriting active-scene values or advancing their
// timeouts. All startup requests must settle before activation.
type preparation struct {
	scene   *Scene
	queues  [2][]envelope
	pending []pendingRequest
	free    []int
}

func (m *SceneManager) prepare(id project.SceneID, startup SceneStartup) (target *Scene, err error) {
	return m.prepareScene(nil, id, startup)
}
func (m *SceneManager) prepareScene(target *Scene, id project.SceneID, startup SceneStartup) (_ *Scene, err error) {
	r := m.runtime
	if r.busy || r.loading || r.preparation != nil {
		return nil, ErrBusy
	}
	if target != nil {
		hasHook := false
		for i := -1; i < len(target.modules); i++ {
			c := &target.context
			if i >= 0 {
				c = &target.modules[i]
			}
			if c.enabled && c.usable() && (target.state == SceneSuspended && c.resume != nil || target.state == SceneReady && c.enter != nil) {
				hasHook = true
				break
			}
		}
		if !hasHook {
			return target, nil
		}
	}
	parkedQueues, parkedPending, parkedFree := r.queues, r.pending, r.freePending
	parkedFirst, parkedLast, parkedWrite, parkedTick := r.pendingFirst, r.pendingLast, r.write, r.tick
	p := &r.prepareWorkspace
	if p.pending == nil {
		p.pending = make([]pendingRequest, r.options.RequestCapacity)
		p.free = make([]int, r.options.RequestCapacity)
		for i := range p.queues {
			p.queues[i] = make([]envelope, 0, r.options.EventCapacity)
		}
	}
	p.scene = target
	r.preparation = p
	r.queues, r.pending, r.freePending = p.queues, p.pending, p.free[:r.options.RequestCapacity]
	for i := range r.freePending {
		r.freePending[i] = r.options.RequestCapacity - 1 - i
	}
	r.pendingFirst, r.pendingLast, r.write = -1, -1, 0
	retained := target != nil
	previousState, previousRendered := SceneDisposed, false
	if retained {
		previousState, previousRendered = target.state, target.rendered
	}
	defer func() {
		if err != nil && target != nil {
			if retained && target.Alive() {
				target.state, target.rendered = previousState, previousRendered
				if previousRendered {
					m.visibility(target, true)
					m.activity(target, false)
				} else {
					m.visibility(target, false)
				}
			} else {
				r.busy = true
				err = joinError(err, r.disposeScene(target))
				r.busy = false
				err = joinError(err, r.flushStructural())
			}
		}
		// Failed hooks may leave requests unsettled. Release their references
		// before parking the workspace, rather than at the next preparation.
		for r.pendingFirst >= 0 {
			r.releasePending(r.pendingFirst)
		}
		r.resetBuffer(2)
		r.resetBuffer(3)
		for buffer := range 2 {
			clear(r.queues[buffer])
			r.queues[buffer] = r.queues[buffer][:0]
		}
		p.queues, p.pending, p.free = r.queues, r.pending, r.freePending
		r.queues, r.pending, r.freePending = parkedQueues, parkedPending, parkedFree
		r.pendingFirst, r.pendingLast, r.write, r.tick = parkedFirst, parkedLast, parkedWrite, parkedTick
		r.preparation = nil
		p.scene = nil
		r.scopeDirty = true
		r.SyncScopes()
	}()
	if retained {
		m.visibility(target, true)
		target.state = SceneLoading
	} else {
		target, err = r.mount(id, SceneLoading)
		if err != nil {
			return target, err
		}
	}
	p.scene = target
	r.scopeDirty = true
	r.SyncScopes()
	if !retained {
		target.startup = startup
		err = m.startup(target, startup)
	} else {
		r.busy = true
		for i := -1; i < len(target.modules); i++ {
			c := &target.context
			if i >= 0 {
				c = &target.modules[i]
			}
			if !c.usable() || !c.enabled {
				continue
			}
			hook := c.enter
			if previousState == SceneSuspended {
				hook = c.resume
			}
			if hook != nil {
				err = joinError(err, hook(startup))
			}
		}
		r.busy = false
		err = joinError(err, r.flushStructural())
	}
	if err != nil {
		return target, err
	}
	if !target.Alive() {
		return target, ErrDisposed
	}
	events := 0
	for batches := 0; r.Queued() > 0 || r.Pending() > 0; batches++ {
		events += r.Queued()
		if batches >= r.options.StartupBatches || events > r.options.StartupEvents {
			return target, fmt.Errorf("%s: %w", id, ErrStartupLimit)
		}
		if err = r.Dispatch(); err != nil {
			return target, err
		}
		if !target.Alive() {
			return target, ErrDisposed
		}
	}
	// Complete CPU-side transforms, authored UI and static text while scoped to
	// this scene. Activation only changes lifetime and execution membership.
	if r.content.syncHTML != nil {
		if err = r.content.syncHTML(r.world); err != nil {
			return target, err
		}
	}
	r.SyncScopes()
	if r.content.layout != nil {
		if err = r.content.layout(r.world); err != nil {
			return target, err
		}
	}
	if err = scene.SyncGlobalTransformSystem("").Run(r.world); err != nil {
		return target, err
	}
	if err = scene.SyncUIGlobalTransformSystem("").Run(r.world); err != nil {
		return target, err
	}
	if r.content.deform != nil {
		if err = r.content.deform(r.world); err != nil {
			return target, err
		}
	}
	target.uploads.Reset()
	for _, entity := range target.entities {
		if text := r.world.GetMut[scene.Text2D](entity); text != nil {
			scene.PrepareText2D(text)
		}
		r.content.queueUploads(&target.uploads, r.world, entity)
	}
	target.state, target.rendered = SceneReady, false
	m.visibility(target, false)
	return target, nil
}

func prepareSceneGPU(s *Scene) error {
	if graphics.DeviceFor(s.runtime.world).Driver() == nil {
		return nil
	}
	for {
		ready, err := s.AdvanceGPUFor(graphics.DeviceFor(s.runtime.world), graphics.UploadBudget{Time: time.Hour})
		if ready || err != nil {
			return err
		}
	}
}
