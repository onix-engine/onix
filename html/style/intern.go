package style

import (
	"hash/maphash"
	"slices"
)

type valueSlot[T any] struct {
	hash  uint64
	value *T
}

// valueIndex interns immutable values in a bounded document cache or a pooled
// pass. Equal values share one allocation; reset keeps only table storage.
type valueIndex[T any] struct {
	seed  maphash.Seed
	slots []valueSlot[T]
	used  int
	limit int // Zero keeps pass-local interning unlimited.
}

func (index *valueIndex[T]) reset() {
	clear(index.slots)
	index.used = 0
}

func (index *valueIndex[T]) init() {
	if index.slots == nil {
		index.slots = make([]valueSlot[T], 64)
	}
	if index.seed == (maphash.Seed{}) {
		index.seed = maphash.MakeSeed()
	}
}

func (index *valueIndex[T]) store(hash uint64, value *T) {
	if index.limit > 0 && index.used >= index.limit {
		return
	}
	if (index.used+1)*10 >= len(index.slots)*7 {
		previous := index.slots
		index.slots = make([]valueSlot[T], len(previous)*2)
		for _, slot := range previous {
			if slot.value != nil {
				index.insert(slot.hash, slot.value)
			}
		}
	}
	index.insert(hash, value)
	index.used++
}

func (index *valueIndex[T]) insert(hash uint64, value *T) {
	for offset := uint64(0); ; offset++ {
		slot := &index.slots[(hash+offset)&uint64(len(index.slots)-1)]
		if slot.value == nil {
			*slot = valueSlot[T]{hash: hash, value: value}
			return
		}
	}
}

type styleIndex struct {
	valueIndex[ComputedStyle]
	motions valueIndex[motionStyle]
}

func (index *styleIndex) reset() {
	index.valueIndex.reset()
	index.motions.reset()
}

func (index *styleIndex) commit(current, previous *ComputedStyle) *ComputedStyle {
	if previous != nil && styleEqual(previous, current) {
		return previous
	}
	index.init()
	hash := styleHash(index.seed, current)
	for offset := uint64(0); ; offset++ {
		slot := &index.slots[(hash+offset)&uint64(len(index.slots)-1)]
		if slot.value == nil {
			break
		}
		if slot.hash == hash && styleEqual(slot.value, current) {
			return slot.value
		}
	}
	stored := *current
	index.store(hash, &stored)
	return &stored
}

func (index *styleIndex) resolveMotion(current *motionStyle) *motionStyle {
	if current.motionInputs == (motionInputs{}) {
		return nil
	}
	index.motions.limit = index.limit
	index.motions.init()
	hash := motionHash(index.motions.seed, current)
	for offset := uint64(0); ; offset++ {
		slot := &index.motions.slots[(hash+offset)&uint64(len(index.motions.slots)-1)]
		if slot.value == nil {
			break
		}
		if slot.hash == hash && motionEqual(slot.value, current) {
			return slot.value
		}
	}
	stored := *current
	resolveMotion(&stored)
	index.motions.store(hash, &stored)
	return &stored
}

func styleHash(seed maphash.Seed, computed *ComputedStyle) uint64 {
	hash := maphash.Comparable(seed, computed.styleCore) ^
		styleSliceCode(computed.BackgroundLayers)*0x9e3779b97f4a7c15 ^
		styleSliceCode(computed.BoxShadow)*0xbf58476d1ce4e5b9 ^
		styleSliceCode(computed.Filters)*0x94d049bb133111eb
	if computed.motion != nil {
		hash ^= motionHash(seed, computed.motion)
	}
	return hash
}

// styleSliceCode folds a slice's length together with its nil-ness, which the
// comparison keeps distinct.
func styleSliceCode[T comparable](values []T) uint64 {
	if values == nil {
		return 0
	}
	return uint64(len(values)) + 1
}

func styleEqual(a, b *ComputedStyle) bool {
	return a.styleCore == b.styleCore && motionEqual(a.motion, b.motion) &&
		styleSliceEqual(a.BackgroundLayers, b.BackgroundLayers) &&
		styleSliceEqual(a.BoxShadow, b.BoxShadow) && styleSliceEqual(a.Filters, b.Filters)
}

// EqualExceptOpacity identifies changes that can be applied to a composited
// group at draw time without repainting its contents.
func EqualExceptOpacity(a, b *ComputedStyle) bool {
	if a == nil || b == nil {
		return a == b
	}
	left, right := *a, *b
	left.Opacity, right.Opacity = Value{}, Value{}
	return styleEqual(&left, &right)
}

func styleSliceEqual[T comparable](a, b []T) bool {
	return styleSliceCode(a) == styleSliceCode(b) && slices.Equal(a, b)
}
