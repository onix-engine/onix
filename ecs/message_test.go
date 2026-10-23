package ecs

import (
	"slices"
	"testing"
)

func TestMessageReaderReturnAndPanicRetainUnreadMessages(t *testing.T) {
	for _, stop := range []string{"return", "panic"} {
		t.Run(stop, func(t *testing.T) {
			world := NewWorld()
			WriteMessage(world, 1)
			WriteMessage(world, 2)
			AdvanceMessages(world)
			WriteMessage(world, 3)
			var reader MessageReader[int]
			consume := func() {
				for message := range reader.All(world) {
					if *message == 2 {
						if stop == "panic" {
							panic("stop")
						}
						return
					}
				}
			}
			func() {
				var expected any
				if stop == "panic" {
					expected = "stop"
				}
				defer func() {
					if value := recover(); value != expected {
						t.Fatalf("unexpected panic: %v", value)
					}
				}()
				consume()
			}()
			if got := reader.Read(world); !slices.Equal(got, []int{3}) {
				t.Fatalf("unread messages after %s = %v", stop, got)
			}
		})
	}
}

func TestMessageReaderDiscardIsLocalAndPreservesLaterWrites(t *testing.T) {
	world := NewWorld()
	WriteMessage(world, 1)
	AdvanceMessages(world)
	WriteMessage(world, 2)
	var reader, observer MessageReader[int]
	sequence := reader.All(world)
	version := world.MutationVersion()
	reader.Discard(world)
	if world.MutationVersion() != version {
		t.Fatal("discard changed the world")
	}
	WriteMessage(world, 3)
	var messages []int
	for message := range sequence {
		messages = append(messages, *message)
	}
	if !slices.Equal(messages, []int{3}) {
		t.Fatalf("messages after discard = %v", messages)
	}
	if got := observer.Read(world); !slices.Equal(got, []int{1, 2, 3}) {
		t.Fatalf("discard affected another reader: %v", got)
	}
	AdvanceMessages(world)
	AdvanceMessages(world)
	WriteMessage(world, 4)
	reader.Discard(world)
	WriteMessage(world, 5)
	if got := reader.Read(world); !slices.Equal(got, []int{5}) {
		t.Fatalf("messages after expiry and discard = %v", got)
	}
}

func TestMessageReaderAllBorrowsMessagesAcrossConsumers(t *testing.T) {
	world := NewWorld()
	WriteMessage(world, 1)
	AdvanceMessages(world)
	WriteMessage(world, 2)
	version := world.MutationVersion()
	var first, second MessageReader[int]
	for message := range first.All(world) {
		*message += 10
	}
	if got := second.Read(world); !slices.Equal(got, []int{11, 12}) {
		t.Fatalf("borrowed writes were copied: %v", got)
	}
	if world.MutationVersion() != version {
		t.Fatal("message consumption changed the world")
	}
}

func TestMessageReaderAllSkipsExpiredFrames(t *testing.T) {
	world := NewWorld()
	WriteMessage(world, 1)
	AdvanceMessages(world)
	WriteMessage(world, 2)
	AdvanceMessages(world)
	WriteMessage(world, 3)
	var reader MessageReader[int]
	var values []int
	for value := range reader.All(world) {
		values = append(values, *value)
	}
	if !slices.Equal(values, []int{2, 3}) {
		t.Fatalf("expired reader messages = %v", values)
	}
	AdvanceMessages(world)
	AdvanceMessages(world)
	for value := range reader.All(world) {
		t.Fatalf("expired message retained: %v", *value)
	}
	if reader.cursor != 3 {
		t.Fatalf("expired reader cursor = %v, want 3", reader.cursor)
	}
}

func TestMessageReaderAllIncludesWritesDuringIteration(t *testing.T) {
	for _, previousFrame := range []bool{false, true} {
		world := NewWorld()
		WriteMessage(world, 1)
		if previousFrame {
			AdvanceMessages(world)
		}
		var reader MessageReader[int]
		var values []int
		for value := range reader.All(world) {
			values = append(values, *value)
			if *value == 1 {
				for next := 2; next <= 100; next++ {
					WriteMessage(world, next)
				}
			}
		}
		if len(values) != 100 {
			t.Fatalf("iteration lost writes: previousFrame=%v count=%d", previousFrame, len(values))
		}
		for index, value := range values {
			if value != index+1 {
				t.Fatalf("iteration order: previousFrame=%v values=%v", previousFrame, values)
			}
		}
		if got := reader.Read(world); len(got) != 0 {
			t.Fatalf("iteration duplicated writes: %v", got)
		}
	}
}

func TestMessageReaderAllAndDiscardDoNotAllocateAfterWarmup(t *testing.T) {
	world := NewWorld()
	WriteMessage(world, 1)
	AdvanceMessages(world)
	WriteMessage(world, 2)
	consume := func() {
		var reader MessageReader[int]
		total := 0
		for message := range reader.All(world) {
			total += *message
		}
		if total != 3 {
			panic("unexpected messages")
		}
		reader.Discard(world)
	}
	if allocations := testing.AllocsPerRun(100, consume); allocations != 0 {
		t.Fatalf("stack-local message reader allocated %v times", allocations)
	}
}
