package module

import (
	"errors"
	"reflect"
	"testing"
	"unsafe"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/project"
)

func TestPayloadCapacityGrowsWithoutReducingQueueLimit(t *testing.T) {
	plan, _ := compileFixture(t, map[string]string{
		"resources/project.json":     `{"entry":"main.json","scenes":["main.json"],"types":{"Pair":{"fields":{"number":"int"}}}}`,
		"resources/scenes/main.json": `{"modules":[{"id":"a","mod":"box"}]}`,
		"box/mod.json":               `{"slots":{"value":{"type":"Pair"}}}`,
	})
	for _, capacity := range []int{0, 2, 32} {
		r := newFixture(t, plan, Registry{}, Options{EventCapacity: 32, PayloadCapacity: capacity})
		s, err := r.MountEntry()
		must(t, err)
		emitter, err := s.Emitter[[1024]byte]("payload")
		must(t, err)
		writer, err := s.SlotWriter("a", "value")
		must(t, err)
		initial := capacity
		if initial == 0 {
			initial = 8
		}
		if cap(emitter.store.values[0]) != initial || len(writer.buffer.values[0]) != initial {
			t.Fatal("payload banks ignored their initial capacity")
		}
		value := project.SlotValue{Type: "Pair", Fields: []project.SlotFieldValue{{Name: "number", Value: project.SlotValue{Type: project.IntValue}}}}
		burst := func(slots bool) {
			for i := range 32 {
				if slots {
					value.Fields[0].Value.Integer = int64(i)
					must(t, writer.Write(value))
				} else {
					must(t, emitter.Emit([1024]byte{byte(i)}))
				}
			}
			if err := emitter.Emit([1024]byte{}); !errors.Is(err, ErrQueueFull) {
				t.Fatalf("queue overflow returned %v", err)
			}
			if slots && (writer.buffer.values[r.buffer()][0].Fields[0].Value.Integer != 0 || writer.buffer.values[r.buffer()][31].Fields[0].Value.Integer != 31) {
				t.Fatal("grown slot entries share mutable fields")
			}
			must(t, r.Dispatch())
		}
		for range 4 {
			burst(false)
			burst(true)
		}
		if allocations := testing.AllocsPerRun(20, func() { burst(false); burst(true) }); allocations != 0 {
			t.Fatalf("warmed bursts allocated %g times", allocations)
		}
	}
	if _, err := New[FullContent](ecs.NewWorld(), plan, Registry{}, Options{EventCapacity: 8, PayloadCapacity: 9}); !errors.Is(err, ErrInvalid) {
		t.Fatal("oversized initial capacity accepted")
	}
}

func BenchmarkPayloadBankReservation(b *testing.B) {
	for _, test := range []struct {
		name     string
		capacity int
	}{{"queue-sized", 256}, {"small", 8}} {
		b.Run(test.name, func(b *testing.B) {
			shape := project.SlotValue{Type: "pair", Fields: []project.SlotFieldValue{{Name: "a", Value: project.SlotValue{Type: project.IntValue}}, {Name: "b", Value: project.SlotValue{Type: project.StringValue}}}}
			b.ReportAllocs()
			var reserved uintptr
			for b.Loop() {
				r := &Runtime{options: Options{EventCapacity: 256, PayloadCapacity: test.capacity}, typedStores: make(map[reflect.Type]valueStore), slotBuffers: make(map[project.ValueType]*slotBuffer)}
				store := r.typedStore[[1024]byte]()
				slots := r.slotBuffer(shape)
				reserved = 0
				for i := range 2 {
					reserved += uintptr(cap(store.values[i])) * 1024
					reserved += uintptr(cap(slots.values[i])) * unsafe.Sizeof(project.SlotValue{})
					for j := range slots.values[i] {
						reserved += uintptr(cap(slots.values[i][j].Fields)) * unsafe.Sizeof(project.SlotFieldValue{})
					}
				}
			}
			b.ReportMetric(float64(reserved), "reserved-B")
		})
	}
}
