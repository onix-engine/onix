package module

import (
	"errors"
	"reflect"
)

var (
	ErrInvalid        = errors.New("module: invalid argument")
	ErrDisposed       = errors.New("module: instance disposed")
	ErrBusy           = errors.New("module: structural operation requires a tick boundary")
	ErrQueueFull      = errors.New("module: event queue capacity reached")
	ErrPendingFull    = errors.New("module: pending request capacity reached")
	ErrCommandFull    = errors.New("module: structural command capacity reached")
	ErrType           = errors.New("module: protocol type mismatch")
	ErrOwnership      = errors.New("module: entity belongs to another instance")
	ErrReadOnly       = errors.New("module: slot does not allow external writes")
	ErrTimeout        = errors.New("module: request timed out")
	ErrCanceled       = errors.New("module: request canceled")
	ErrAlreadyReplied = errors.New("module: request already replied")
	ErrUnavailable    = errors.New("module: request receiver unavailable")
	ErrStaleRevision  = errors.New("module: request slot revision changed")
)

type RequestID uint64
type Tick struct {
	Number uint64
	Delta  float32
}

type Result[T any] struct {
	ID RequestID
	// Output leases in Value are borrowed until the callback returns. Retain or
	// install them in a slot to keep them. Reply transfers its lease to the queue
	// only on success; on error the provider still owns it.
	Value T
	Err   error
}
type RequestOptions struct {
	// Zero selects the Runtime default; timeouts count fixed logic ticks.
	TimeoutTicks uint64
	Revision     SlotRevision
}

// SceneTarget addresses the sender's scene. All other targets are instance IDs
// in that scene. Receivers need no declaration or subscription.
const SceneTarget = ""
const InputTopic = "input"

// Message is borrowed for the duration of OnMessage. Payloads remain typed in
// preallocated buffers. Use Value, RequestValue or ResultValue to inspect them.
type Message struct {
	Topic    string
	From     string
	ID       RequestID
	Err      error
	kind     eventKind
	envelope envelope
	delivery uint64
}

func (m Message) valid() bool {
	return m.delivery != 0 && m.envelope.target != nil && m.envelope.target.delivery == m.delivery
}

func (m Message) IsRequest() bool { return m.kind == requestEvent }
func (m Message) IsReply() bool   { return m.kind == replyEvent }

func Value[T any](m Message) (T, bool) {
	s, ok := m.envelope.store.(*store[T])
	if !ok || !m.valid() || m.Err != nil {
		var zero T
		return zero, false
	}
	return s.values[m.envelope.buffer][m.envelope.value], true
}

func RequestValue[Q, R any](m Message) (Request[Q, R], bool) {
	if !m.IsRequest() {
		return Request[Q, R]{}, false
	}
	value, ok := Value[Q](m)
	e := m.envelope
	if !ok || e.target == nil {
		return Request[Q, R]{}, false
	}
	r := e.target.runtime
	pending := r.lookupPending(e.pending, e.id)
	if pending == nil {
		return Request[Q, R]{}, false
	}
	response, ok := pending.response.(*store[R])
	if !ok {
		return Request[Q, R]{}, false
	}
	return Request[Q, R]{ID: m.ID, Value: value, runtime: r, pending: e.pending, response: response}, true
}

func ResultValue[T any](m Message) (Result[T], bool) {
	if !m.IsReply() || !m.valid() {
		return Result[T]{}, false
	}
	if _, ok := m.envelope.store.(*store[T]); !ok {
		return Result[T]{}, false
	}
	if m.Err != nil {
		return Result[T]{ID: m.ID, Err: m.Err}, true
	}
	value, ok := Value[T](m)
	return Result[T]{ID: m.ID, Value: value}, ok
}

type valueStore interface {
	deliver(*Context, envelope) error
	forward(*Runtime, envelope) error
	length(uint8) int
	truncate(uint8, int)
}

type store[T any] struct {
	values [4][]T
	drop   func(T)
	retain func(T) (T, error)
}

func newStore[T any](capacity int) *store[T] {
	return &store[T]{values: [4][]T{make([]T, 0, capacity), make([]T, 0, capacity)}}
}
func (s *store[T]) length(buffer uint8) int { return len(s.values[buffer]) }
func (s *store[T]) truncate(buffer uint8, length int) {
	if s.drop != nil {
		for _, v := range s.values[buffer][length:] {
			s.drop(v)
		}
	}
	clear(s.values[buffer][length:])
	s.values[buffer] = s.values[buffer][:length]
}
func (s *store[T]) deliver(c *Context, e envelope) error {
	e.store = s
	e.target = c
	if c.message == nil {
		return nil
	}
	c.runtime.nextDelivery++
	if c.runtime.nextDelivery == 0 {
		panic("module: delivery identity exhausted")
	}
	previous := c.delivery
	c.delivery = c.runtime.nextDelivery
	defer func() { c.delivery = previous }()
	return c.message(Message{Topic: e.topic, From: e.source.name, ID: e.id, Err: e.err, kind: e.kind, envelope: e, delivery: c.delivery})
}
func (s *store[T]) forward(r *Runtime, e envelope) error {
	if !r.hasQueueSpace() {
		return ErrQueueFull
	}
	value := s.values[e.buffer][e.value]
	if s.retain != nil {
		var err error
		value, err = s.retain(value)
		if err != nil {
			return err
		}
	}
	return enqueue(r, typedStore[T](r), value, e)
}

// BindEmitter reserves typed buffers during Install. Emit sends to the scene;
// EmitTo addresses any scene module without a declared receive port.
type Emitter[T any] struct {
	context *Context
	topic   string
	store   *store[T]
}

func typedStore[T any](r *Runtime) *store[T] {
	key := reflect.TypeFor[T]()
	if s, ok := r.typedStores[key]; ok {
		return s.(*store[T])
	}
	s := newStore[T](r.options.PayloadCapacity)
	// Lease policy belongs to the Go payload type, independently of slot aliases.
	switch owned := any(s).(type) {
	case *store[RenderOutputHandle]:
		owned.drop = func(v RenderOutputHandle) { _ = v.Release() }
		owned.retain = func(v RenderOutputHandle) (RenderOutputHandle, error) { return v.Retain() }
	case *store[TextRenderResult]:
		owned.drop = func(v TextRenderResult) { _ = v.Output.Release() }
		owned.retain = func(v TextRenderResult) (TextRenderResult, error) {
			var err error
			v.Output, err = v.Output.Retain()
			return v, err
		}
	}
	r.typedStores[key] = s
	reserveUsedBuffers(&r.usedStores, min(len(r.typedStores), r.options.EventCapacity))
	return s
}
func BindEmitter[T any](c *Context, topic string) (Emitter[T], error) {
	if !c.usable() {
		return Emitter[T]{}, ErrDisposed
	}
	if topic == "" {
		return Emitter[T]{}, ErrInvalid
	}
	return Emitter[T]{context: c, topic: topic, store: typedStore[T](c.runtime)}, nil
}
func (e Emitter[T]) Emit(value T) error { return e.EmitTo(SceneTarget, value) }
func (e Emitter[T]) EmitTo(target string, value T) error {
	if !e.context.usable() {
		return ErrDisposed
	}
	receiver, err := e.context.destination(target)
	if err != nil {
		return err
	}
	return enqueue(e.context.runtime, e.store, value, envelope{kind: notificationEvent, source: e.context, target: receiver, topic: e.topic})
}

// Send is convenient for occasional messages. BindEmitter avoids even the type
// lookup on frequently used send paths.
func Send[T any](c *Context, target, topic string, value T) error {
	e, err := BindEmitter[T](c, topic)
	if err != nil {
		return err
	}
	return e.EmitTo(target, value)
}

func (c *Context) destination(target string) (*Context, error) {
	if !c.usable() {
		return nil, ErrDisposed
	}
	if !c.running() {
		return nil, ErrUnavailable
	}
	receiver := &c.scene.context
	if target != SceneTarget {
		i, ok := c.scene.instances[target]
		if !ok {
			return nil, ErrUnavailable
		}
		receiver = &c.scene.modules[i]
	}
	if !receiver.running() {
		return nil, ErrUnavailable
	}
	return receiver, nil
}

// Forward preserves the sender and payload. Scenes relay to modules within the
// current delivery; module relays enter the next batch. Sends always enqueue.
// Requests have one destination per delivery.
func (c *Context) Forward(target string, m Message) error { return c.ForwardAs(target, m.Topic, m) }

func (c *Context) ForwardAs(target, topic string, m Message) error {
	if !c.usable() {
		return ErrDisposed
	}
	if c.delivery == 0 || c.delivery != m.delivery || m.IsReply() || topic == "" {
		return ErrInvalid
	}
	receiver, err := c.destination(target)
	if err != nil {
		return err
	}
	e := m.envelope
	e.target = receiver
	e.topic = topic
	var pending *pendingRequest
	if m.IsRequest() {
		pending = c.runtime.lookupPending(e.pending, e.id)
		if pending == nil || pending.target != c || pending.replied || pending.failure != nil || pending.forwardedDelivery == m.delivery {
			return ErrCanceled
		}
	}
	if c.definition == nil && receiver != c {
		if pending != nil {
			pending.target = receiver
			pending.forwardedDelivery = m.delivery
		}
		if !receiver.enabled {
			return nil
		}
		return e.store.deliver(receiver, e)
	}
	if err = e.store.forward(c.runtime, e); err != nil {
		return err
	}
	if pending != nil {
		pending.target = receiver
		pending.forwardedDelivery = m.delivery
	}
	return nil
}

type Requester[Q, R any] struct {
	context  *Context
	topic    string
	request  *store[Q]
	response *store[R]
}

func BindRequester[Q, R any](c *Context, topic string) (Requester[Q, R], error) {
	if !c.usable() {
		return Requester[Q, R]{}, ErrDisposed
	}
	if topic == "" {
		return Requester[Q, R]{}, ErrInvalid
	}
	return Requester[Q, R]{context: c, topic: topic, request: typedStore[Q](c.runtime), response: typedStore[R](c.runtime)}, nil
}
func (p Requester[Q, R]) Send(value Q, options RequestOptions) (RequestID, error) {
	return p.SendTo(SceneTarget, value, options)
}

func (p Requester[Q, R]) SendTo(target string, value Q, options RequestOptions) (RequestID, error) {
	c := p.context
	if c == nil || !c.usable() {
		return 0, ErrDisposed
	}
	if !c.running() {
		return 0, ErrUnavailable
	}
	r := c.runtime
	if !r.hasQueueSpace() {
		return 0, ErrQueueFull
	}
	receiver, err := c.destination(target)
	if err != nil {
		return 0, err
	}
	if len(r.freePending) == 0 {
		return 0, ErrPendingFull
	}
	if options.Revision.context != nil && options.Revision.context != c {
		return 0, ErrOwnership
	}
	timeout := options.TimeoutTicks
	if timeout == 0 {
		timeout = r.options.TimeoutTicks
	}
	if timeout > ^uint64(0)-r.tick {
		return 0, ErrInvalid
	}
	index := r.freePending[len(r.freePending)-1]
	r.freePending = r.freePending[:len(r.freePending)-1]
	if r.nextRequest >= RequestID((^uint64(0)-uint64(index))/uint64(len(r.pending))) {
		panic("module: request identity exhausted")
	}
	r.nextRequest++
	node := &r.pending[index]
	// Low bits identify the table slot; the monotonic sequence prevents reuse.
	id := r.nextRequest*RequestID(len(r.pending)) + RequestID(index)
	*node = pendingRequest{id: id, source: c, target: receiver, topic: p.topic, response: p.response, deadline: r.tick + timeout, revision: options.Revision, previous: r.pendingLast, next: -1}
	if r.pendingLast >= 0 {
		r.pending[r.pendingLast].next = index
	} else {
		r.pendingFirst = index
	}
	r.pendingLast = index
	err = enqueue(r, p.request, value, envelope{kind: requestEvent, source: c, target: receiver, topic: p.topic, id: node.id, pending: index})
	if err != nil {
		r.releasePending(index)
		return 0, err
	}
	return node.id, nil
}

func (p Requester[Q, R]) Cancel(id RequestID) error {
	if p.context == nil || !p.context.usable() {
		return ErrDisposed
	}
	r := p.context.runtime
	pending := r.lookupPending(int(id%RequestID(len(r.pending))), id)
	if pending == nil {
		return ErrCanceled
	}
	if pending.source != p.context {
		return ErrOwnership
	}
	return r.Cancel(id)
}

// Request carries an opaque reply address, never the requester's Context/entity.
// Retaining it for deferred work is safe; Reply validates its identity again.
type Request[Q, R any] struct {
	ID       RequestID
	Value    Q
	runtime  *Runtime
	pending  int
	response *store[R]
}

func (q Request[Q, R]) Reply(c *Context, value R) error {
	if q.runtime == nil || c == nil || c.runtime != q.runtime || !c.usable() {
		return ErrCanceled
	}
	p := q.runtime.lookupPending(q.pending, q.ID)
	if p == nil || p.failure != nil || !p.source.usable() {
		return ErrCanceled
	}
	if p.target != c {
		return ErrOwnership
	}
	if p.replied {
		return ErrAlreadyReplied
	}
	if q.runtime.tick >= p.deadline {
		return ErrTimeout
	}
	if err := enqueue(q.runtime, q.response, value, envelope{kind: replyEvent, source: c, target: p.source, topic: p.topic, id: q.ID, pending: q.pending}); err != nil {
		return err
	}
	p.replied = true
	return nil
}

// Fail reports a service failure through Result, with the same reply-address
// validation as Reply. The callback is delivered at the next batch boundary.
func (q Request[Q, R]) Fail(c *Context, err error) error {
	if err == nil {
		return ErrInvalid
	}
	if q.runtime == nil || c == nil || c.runtime != q.runtime || !c.usable() {
		return ErrCanceled
	}
	p := q.runtime.lookupPending(q.pending, q.ID)
	if p == nil || p.failure != nil || !p.source.usable() {
		return ErrCanceled
	}
	if p.target != c {
		return ErrOwnership
	}
	if p.replied {
		return ErrAlreadyReplied
	}
	if q.runtime.tick >= p.deadline {
		return ErrTimeout
	}
	p.failure = err
	p.failureTick = q.runtime.tick + 1
	return nil
}

func enqueue[T any](r *Runtime, s *store[T], value T, e envelope) error {
	if !r.hasQueueSpace() {
		return ErrQueueFull
	}
	e.buffer = r.buffer()
	if s.values[e.buffer] == nil {
		s.values[e.buffer] = make([]T, 0, r.options.PayloadCapacity)
	}
	e.value = len(s.values[e.buffer])
	e.store = s
	if e.value == 0 {
		r.usedStores[e.buffer] = append(r.usedStores[e.buffer], s)
	}
	s.values[e.buffer] = append(s.values[e.buffer], value)
	r.queues[r.write] = append(r.queues[r.write], e)
	if r.options.Wake != nil {
		r.options.Wake()
	}
	return nil
}

type eventKind uint8

const (
	notificationEvent eventKind = iota
	requestEvent
	replyEvent
	slotEvent
	slotChangeEvent
	engineEvent
)

type envelope struct {
	kind           eventKind
	source, target *Context
	topic          string
	store          valueStore
	buffer         uint8
	value          int
	id             RequestID
	pending        int
	err            error
	slot           Slot
	slotBuffer     *slotBuffer
	observer       *slotObserver
}
type pendingRequest struct {
	id                RequestID
	source            *Context
	target            *Context
	topic             string
	response          valueStore
	deadline          uint64
	revision          SlotRevision
	replied           bool
	failure           error
	failureTick       uint64
	forwardedDelivery uint64
	previous, next    int
}
