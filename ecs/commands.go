package ecs

import (
	"fmt"
	"reflect"
	"slices"
)

type commandKind uint8

const (
	commandInsert commandKind = iota
	commandRemove
	commandDespawn
	commandSpawn
	commandRemovePrepared
)

type commandStream interface {
	apply(*World, Entity, int) error
	truncate(int)
	retain(int)
	trimRetained()
}

type commandValues[T any] struct {
	values   []T
	retained int
}

type typedInsertCommandStream[T any] struct {
	commandValues[T]
}

func (s *typedInsertCommandStream[T]) apply(world *World, entity Entity, index int) error {
	if index < 0 {
		Remove[T](world, entity)
		return nil
	}
	return Insert(world, entity, s.values[index])
}

func (s *commandValues[T]) retain(length int) {
	s.retained = max(s.retained, length)
}

func (s *commandValues[T]) trimRetained() {
	s.truncate(s.retained)
	s.retained = 0
}

func (s *commandValues[T]) truncate(length int) {
	if length < 0 || length > len(s.values) {
		panic("ecs: invalid command stream length")
	}
	clear(s.values[length:])
	s.values = s.values[:length]
}

type queuedCommand struct {
	kind       commandKind
	entity     Entity
	stream     commandStream
	valueIndex int
	spawnStart int
	spawnEnd   int
	removal    *removalPlan
}

type Commands struct {
	world        *World
	queue        []queuedCommand
	spawnEntries []preparedBundleEntry
	streams      map[reflect.Type]commandStream
}

func (c *Commands) checkpoint() int {
	if c == nil {
		panic("ecs: commands is nil")
	}
	return len(c.queue)
}

// Reserve grows command queue storage without changing its contents. It
// avoids queue backing-array growth when a system knows its command burst.
func (c *Commands) Reserve(capacity int) {
	if c == nil {
		panic("ecs: commands is nil")
	}
	if capacity > cap(c.queue) {
		c.queue = slices.Grow(c.queue, capacity-len(c.queue))
	}
}

func (c *Commands) discardFrom(index int) {
	if c == nil {
		panic("ecs: commands is nil")
	}
	if index < 0 || index > len(c.queue) {
		panic("ecs: invalid command checkpoint")
	}
	for _, queued := range c.queue[index:] {
		if queued.kind == commandSpawn {
			c.world.cancelReserved(queued.entity)
		}
	}
	clear(c.queue[index:])
	c.queue = c.queue[:index]
	c.trimStorage()
}

func (c *Commands) trimStorage() {
	if c == nil {
		panic("ecs: commands is nil")
	}
	spawnLength := 0
	for _, queued := range c.queue {
		switch queued.kind {
		case commandInsert:
			queued.stream.retain(queued.valueIndex + 1)
		case commandSpawn:
			spawnLength = max(spawnLength, queued.spawnEnd)
		}
	}
	for _, stream := range c.streams {
		stream.trimRetained()
	}
	clear(c.spawnEntries[spawnLength:])
	c.spawnEntries = c.spawnEntries[:spawnLength]
}

func (w *World) Commands() *Commands {
	requireWorld(w)
	return &w.commands
}

func CommandInsert[T any](commands *Commands, entity Entity, value T) {
	if commands == nil {
		panic("ecs: commands is nil")
	}
	stream := insertCommandStreamFor[T](commands)
	index := len(stream.values)
	stream.values = append(stream.values, value)
	commands.queue = append(commands.queue, queuedCommand{
		kind:       commandInsert,
		entity:     entity,
		stream:     stream,
		valueIndex: index,
	})
}

func CommandRemove[T any](commands *Commands, entity Entity) {
	if commands == nil {
		panic("ecs: commands is nil")
	}
	commands.queue = append(commands.queue, queuedCommand{
		kind:   commandRemove,
		entity: entity,
		stream: insertCommandStreamFor[T](commands),
	})
}

// CommandRemovePrepared queues one batch migration for use during iteration.
func CommandRemovePrepared(commands *Commands, entity Entity, removal PreparedRemoval) {
	if commands == nil {
		panic("ecs: commands is nil")
	}
	plan := removal.requirePlan()
	if plan.world != commands.world {
		panic("ecs: prepared removal belongs to a different world")
	}
	commands.queue = append(commands.queue, queuedCommand{
		kind: commandRemovePrepared, entity: entity, removal: plan,
	})
}

func CommandDespawn(commands *Commands, entity Entity) {
	if commands == nil {
		panic("ecs: commands is nil")
	}
	commands.queue = append(commands.queue, queuedCommand{kind: commandDespawn, entity: entity})
}

func CommandSpawn(commands *Commands, bundle Bundle) Entity {
	if commands == nil {
		panic("ecs: commands is nil")
	}
	if commands.world == nil {
		panic("ecs: commands is not attached to a world")
	}
	start := len(commands.spawnEntries)
	defer func() {
		if value := recover(); value != nil {
			clear(commands.spawnEntries[start:])
			commands.spawnEntries = commands.spawnEntries[:start]
			panic(value)
		}
	}()
	commands.spawnEntries = slices.Grow(commands.spawnEntries, len(bundle.entries))
	commands.spawnEntries = commands.spawnEntries[:start+len(bundle.entries)]
	entries := commands.world.prepareBundle(bundle, commands.spawnEntries[start:])
	entity := commands.world.reserveEntity()
	commands.queue = append(commands.queue, queuedCommand{
		kind:       commandSpawn,
		entity:     entity,
		spawnStart: start,
		spawnEnd:   start + len(entries),
	})
	return entity
}

// CommandSpawnPrepared reserves an entity and queues a previously prepared
// bundle without rebuilding its component entry list.
func CommandSpawnPrepared(commands *Commands, bundle PreparedBundle) Entity {
	if commands == nil {
		panic("ecs: commands is nil")
	}
	if commands.world == nil {
		panic("ecs: commands is not attached to a world")
	}
	if bundle.world != commands.world {
		panic("ecs: prepared bundle belongs to a different world")
	}
	start := len(commands.spawnEntries)
	defer func() {
		if value := recover(); value != nil {
			clear(commands.spawnEntries[start:])
			commands.spawnEntries = commands.spawnEntries[:start]
			panic(value)
		}
	}()
	commands.spawnEntries = append(commands.spawnEntries, bundle.entries...)
	entity := commands.world.reserveEntity()
	commands.queue = append(commands.queue, queuedCommand{
		kind:       commandSpawn,
		entity:     entity,
		spawnStart: start,
		spawnEnd:   len(commands.spawnEntries),
	})
	return entity
}

func insertCommandStreamFor[T any](commands *Commands) *typedInsertCommandStream[T] {
	return commandStreamFor(commands, func() *typedInsertCommandStream[T] { return &typedInsertCommandStream[T]{} })
}

func commandStreamFor[S commandStream](commands *Commands, create func() S) S {
	if commands.streams == nil {
		commands.streams = make(map[reflect.Type]commandStream)
	}
	typ := reflect.TypeFor[S]()
	if stream, ok := commands.streams[typ]; ok {
		return stream.(S)
	}
	stream := create()
	commands.streams[typ] = stream
	return stream
}

func (c queuedCommand) apply(world *World, spawnEntries []preparedBundleEntry) error {
	switch c.kind {
	case commandInsert:
		return c.stream.apply(world, c.entity, c.valueIndex)
	case commandRemove:
		return c.stream.apply(world, c.entity, -1)
	case commandDespawn:
		world.Despawn(c.entity)
		return nil
	case commandSpawn:
		world.spawnReserved(c.entity, spawnEntries[c.spawnStart:c.spawnEnd], world.mutationTick())
		return nil
	case commandRemovePrepared:
		PreparedRemoval{plan: c.removal}.Apply(c.entity)
		return nil
	default:
		panic("ecs: unknown command kind")
	}
}

func (w *World) FlushCommands() error {
	requireWorld(w)
	w.assertStructuralMutationAllowed()
	queue := w.commands.queue
	spawnEntries := w.commands.spawnEntries
	w.commands.queue = nil
	w.commands.spawnEntries = nil
	next := 0
	defer func() {
		clear(queue)
		w.commands.queue = queue[:0]
		w.commands.spawnEntries = spawnEntries
		w.commands.resetStorage()
	}()
	defer func() {
		if value := recover(); value != nil {
			w.commands.queue = queue[next:]
			w.commands.discardFrom(0)
			panic(value)
		}
	}()
	for index, queued := range queue {
		next = index
		if err := queued.apply(w, spawnEntries); err != nil {
			w.commands.queue = queue[index+1:]
			w.commands.discardFrom(0)
			return fmt.Errorf("ecs: apply command %d: %w", index, err)
		}
		next = index + 1
	}
	return nil
}

func (c *Commands) resetStorage() {
	if c == nil {
		panic("ecs: commands is nil")
	}
	for _, stream := range c.streams {
		stream.truncate(0)
	}
	clear(c.spawnEntries)
	c.spawnEntries = c.spawnEntries[:0]
}
