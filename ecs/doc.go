// Package ecs provides typed component storage, queries, structural commands,
// schedules, resources and messages. It is independent of scene, rendering,
// input and UI runtimes; applications supply their own component types.
//
// ComponentRevision tracks storage changes, including archetype migrations,
// and is suitable for caches retaining component references. Caches retaining
// copied values can use ComponentValueRevision to ignore unchanged migrations.
// World.EntityVersion tracks live entities independently of component membership.
package ecs
