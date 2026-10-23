package style

import (
	"hash/maphash"
	"slices"
)

// styleSlot is one entry of the interning table; an empty entry holds no style.
type styleSlot struct {
	hash  uint64
	style *ComputedStyle
}

// styleIndex shares immutable resolved values within a pass or a bounded
// document cache, instead of allocating one ~6KB block per node. The table is
// open addressed; storing a new distinct style costs one allocation.
type styleIndex struct {
	seed  maphash.Seed
	slots []styleSlot
	used  int
	limit int // Zero keeps pass-local interning unlimited.
}

// reset empties the table but keeps its capacity for the next pooled pass.
func (index *styleIndex) reset() {
	clear(index.slots)
	index.used = 0
}

// commit returns the instance a node should point at: its previous instance
// when nothing changed, an equal interned instance, or a freshly stored one.
func (index *styleIndex) commit(current, previous *ComputedStyle) *ComputedStyle {
	if previous != nil && styleEqual(previous, current) {
		return previous
	}
	if index.slots == nil {
		index.slots = make([]styleSlot, 64)
	}
	hash := index.hash(current)
	if found := index.find(hash, current); found != nil {
		return found
	}
	stored := *current
	if index.limit > 0 && index.used >= index.limit {
		return &stored
	}
	if (index.used+1)*10 >= len(index.slots)*7 {
		index.grow()
	}
	index.insert(hash, &stored)
	index.used++
	return &stored
}

func (index *styleIndex) find(hash uint64, current *ComputedStyle) *ComputedStyle {
	for offset := uint64(0); offset < uint64(len(index.slots)); offset++ {
		slot := &index.slots[(hash+offset)&uint64(len(index.slots)-1)]
		if slot.style == nil {
			return nil
		}
		if slot.hash == hash && styleEqual(slot.style, current) {
			return slot.style
		}
	}
	return nil
}

func (index *styleIndex) insert(hash uint64, computed *ComputedStyle) {
	for offset := uint64(0); ; offset++ {
		slot := &index.slots[(hash+offset)&uint64(len(index.slots)-1)]
		if slot.style == nil {
			*slot = styleSlot{hash: hash, style: computed}
			return
		}
	}
}

// grow doubles the table and keeps every instance pointer, so nodes interned
// earlier in the pass stay shared.
func (index *styleIndex) grow() {
	previous := index.slots
	index.slots = make([]styleSlot, max(len(previous)*2, 64))
	for _, slot := range previous {
		if slot.style != nil {
			index.insert(slot.hash, slot.style)
		}
	}
}

func (index *styleIndex) hash(computed *ComputedStyle) uint64 {
	if index.seed == (maphash.Seed{}) {
		index.seed = maphash.MakeSeed()
	}
	return maphash.Comparable(index.seed, computed.styleCore) ^
		styleSliceCode(computed.BackgroundLayers)*0x9e3779b97f4a7c15 ^
		styleSliceCode(computed.BoxShadow)*0xbf58476d1ce4e5b9 ^
		styleSliceCode(computed.Filters)*0x94d049bb133111eb ^ styleSliceCode(computed.Animations)*0xd6e8feb86659fd93
}

// styleSliceCode folds a slice's length together with its nil-ness, which the
// comparison keeps distinct.
func styleSliceCode[T comparable](values []T) uint64 {
	if values == nil {
		return 0
	}
	return uint64(len(values)) + 1
}

// styleEqual reports whether two resolved styles hold identical values.
func styleEqual(a, b *ComputedStyle) bool {
	return a.styleCore == b.styleCore &&
		styleSliceEqual(a.BackgroundLayers, b.BackgroundLayers) &&
		styleSliceEqual(a.BoxShadow, b.BoxShadow) &&
		styleSliceEqual(a.Filters, b.Filters) && styleSliceEqual(a.Animations, b.Animations)
}

// styleSliceEqual keeps nil and an empty slice distinct, so an interned
// instance is only reused when it matches exactly what the parser produced.
func styleSliceEqual[T comparable](a, b []T) bool {
	return styleSliceCode(a) == styleSliceCode(b) && slices.Equal(a, b)
}
