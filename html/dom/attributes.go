package dom

import (
	"cmp"
	"iter"
	"maps"
	"slices"
	"sync/atomic"
)

// Attribute is an authored name/value pair. Names retain their exact spelling;
// the HTML parser applies ASCII folding and foreign-content adjustments.
type Attribute struct{ Name, Value string }

const smallAttributeLimit = 8

// Empty nodes need only a nil pointer. Small elements use a compact sorted
// list; larger sets use a map without duplicating names in a second index.
type attributeList struct{ data *attributeStorage }

type attributeStorage struct {
	large  map[string]string
	values []Attribute
	shared uint32 // Atomic so independent clones can share read-only templates.
}

func (a *attributeList) Attr(name string) (string, bool) {
	if a.data == nil {
		return "", false
	}
	if a.data.large != nil {
		value, found := a.data.large[name]
		return value, found
	}
	for index := range a.data.values {
		if a.data.values[index].Name == name {
			return a.data.values[index].Value, true
		}
	}
	return "", false
}

// AttrValue returns an attribute's value, or an empty string if absent.
func (a *attributeList) AttrValue(name string) string {
	value, _ := a.Attr(name)
	return value
}

func (a *attributeList) AttrCount() int {
	if a.data == nil {
		return 0
	}
	if a.data.large != nil {
		return len(a.data.large)
	}
	return len(a.data.values)
}

// Attributes visits attributes without allocating. Order is unspecified.
// Removing or updating the currently yielded attribute is allowed; other
// mutations during iteration are not supported. Pairs are values, never
// writable storage views.
func (a *attributeList) Attributes() iter.Seq2[string, string] {
	return func(yield func(string, string) bool) {
		data := a.data
		if data == nil {
			return
		}
		if data.large != nil {
			for name, value := range data.large {
				if !yield(name, value) {
					return
				}
			}
		} else {
			// Reverse iteration permits removal without skipping compacted items.
			for index := len(data.values) - 1; index >= 0; index-- {
				attribute := data.values[index]
				if !yield(attribute.Name, attribute.Value) {
					return
				}
			}
		}
	}
}

// SortedAttributes visits attributes in name order. Small lists need no extra
// storage. Large maps allocate a sorted name buffer for deterministic output.
// Do not change attributes during iteration.
func (a *attributeList) SortedAttributes() iter.Seq2[string, string] {
	return func(yield func(string, string) bool) {
		if a.data == nil {
			return
		}
		if a.data.large != nil {
			names := make([]string, 0, len(a.data.large))
			for name := range a.data.large {
				names = append(names, name)
			}
			slices.Sort(names)
			for _, name := range names {
				if !yield(name, a.data.large[name]) {
					return
				}
			}
		} else {
			for _, attribute := range a.data.values {
				if !yield(attribute.Name, attribute.Value) {
					return
				}
			}
		}
	}
}

// SetAttr changes an attribute, returning whether its value changed.
func (a *attributeList) SetAttr(name, value string) bool {
	if a.data != nil && a.data.large != nil {
		if previous, found := a.data.large[name]; found && previous == value {
			return false
		}
		a.writable(0).large[name] = value
		return true
	}
	var values []Attribute
	if a.data != nil {
		values = a.data.values
	}
	index, found := attributeIndex(values, name)
	if found && values[index].Value == value {
		return false
	}
	data := a.writable(1)
	if found {
		data.values[index].Value = value
	} else if len(data.values) == smallAttributeLimit {
		data.promote(smallAttributeLimit + 1)
		data.large[name] = value
	} else {
		if cap(data.values) == 0 {
			data.values = make([]Attribute, 0, 4)
		}
		data.values = append(data.values, Attribute{})
		copy(data.values[index+1:], data.values[index:])
		data.values[index] = Attribute{name, value}
	}
	return true
}

// RemoveAttr removes an attribute, returning whether it was present.
func (a *attributeList) RemoveAttr(name string) bool {
	if a.data == nil {
		return false
	}
	if a.data.large != nil {
		if _, found := a.data.large[name]; !found {
			return false
		}
		delete(a.writable(0).large, name)
		return true
	}
	index, found := attributeIndex(a.data.values, name)
	if !found {
		return false
	}
	data := a.writable(0)
	copy(data.values[index:], data.values[index+1:])
	data.values[len(data.values)-1] = Attribute{}
	data.values = data.values[:len(data.values)-1]
	return true
}

// SetAttributes replaces all attributes with an owned copy and reuses capacity.
// Duplicate names keep their first value, matching HTML tokenization.
func (a *attributeList) SetAttributes(attributes ...Attribute) {
	if a.data == nil && len(attributes) == 0 {
		return
	}
	if a.data == nil || atomic.LoadUint32(&a.data.shared) != 0 {
		if len(attributes) == 0 {
			a.data = nil
			return
		}
		a.data = &attributeStorage{}
	}
	data := a.data
	if data.large != nil || len(attributes) > smallAttributeLimit {
		if data.large == nil {
			data.large = make(map[string]string, len(attributes))
		} else {
			clear(data.large)
		}
		data.values = nil
		for _, attribute := range attributes {
			if _, found := data.large[attribute.Name]; !found {
				data.large[attribute.Name] = attribute.Value
			}
		}
		return
	}
	previous := data.values
	data.values = append(data.values[:0], attributes...)
	if len(data.values) < len(previous) {
		clear(previous[len(data.values):])
	}
	slices.SortStableFunc(data.values, func(left, right Attribute) int { return cmp.Compare(left.Name, right.Name) })
	kept := data.values[:0]
	for _, attribute := range data.values {
		if len(kept) == 0 || kept[len(kept)-1].Name != attribute.Name {
			kept = append(kept, attribute)
		}
	}
	clear(data.values[len(kept):])
	data.values = kept
}

func (a *attributeList) writable(extra int) *attributeStorage {
	if a.data == nil {
		a.data = &attributeStorage{}
	} else if atomic.LoadUint32(&a.data.shared) != 0 {
		data := &attributeStorage{}
		if a.data.large != nil {
			data.large = maps.Clone(a.data.large)
		} else {
			data.values = make([]Attribute, len(a.data.values), min(smallAttributeLimit, len(a.data.values)+extra))
			copy(data.values, a.data.values)
		}
		a.data = data
	}
	return a.data
}

func (data *attributeStorage) promote(capacity int) {
	data.large = make(map[string]string, capacity)
	for _, attribute := range data.values {
		data.large[attribute.Name] = attribute.Value
	}
	data.values = nil
}

func (a *attributeList) reset() {
	if a.data == nil {
		return
	}
	if atomic.LoadUint32(&a.data.shared) != 0 {
		a.data = nil
	} else {
		clear(a.data.large)
		clear(a.data.values)
		a.data.values = a.data.values[:0]
	}
}

func (a *attributeList) copyFrom(source *attributeList) {
	if source.data == nil || source.data.large == nil {
		var values []Attribute
		if source.data != nil {
			values = source.data.values
		}
		a.SetAttributes(values...)
		return
	}
	a.reset()
	data := a.writable(0)
	if data.large == nil {
		data.promote(source.AttrCount())
	}
	maps.Copy(data.large, source.data.large)
}

func (a *attributeList) equal(other *attributeList) bool {
	if a.data == other.data {
		return true
	}
	if a.AttrCount() != other.AttrCount() {
		return false
	}
	for name, value := range a.Attributes() {
		if expected, found := other.Attr(name); !found || value != expected {
			return false
		}
	}
	return true
}

func attributeIndex(attributes []Attribute, name string) (int, bool) {
	for index := range attributes {
		if attributes[index].Name == name {
			return index, true
		}
		if attributes[index].Name > name {
			return index, false
		}
	}
	return len(attributes), false
}
