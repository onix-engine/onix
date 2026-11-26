// Package markup builds native Onix documents and serializes HTML fragments.
package markup

import (
	"sync"

	"github.com/onix-engine/onix/scene/node"
)

// Builder constructs an ordered tree in a reusable arena. Its zero value is ready
// to use. Reset invalidates its nodes, but previously serialized strings stay valid.
type Builder struct {
	root, parent *node.Node
	fragments    map[*node.Node]string
	arena        node.Arena
	size         int
}

var builders = sync.Pool{New: func() any { return new(Builder) }}

// Acquire returns a reusable builder for producers that serialize their tree.
func Acquire() *Builder { return builders.Get().(*Builder) }

// Release invalidates the tree. Oversized views do not reserve arena capacity
// for subsequent small updates.
func (b *Builder) Release() {
	if b.size > 256<<10 {
		return
	}
	b.Reset()
	builders.Put(b)
}

func (b *Builder) current() *node.Node {
	if b.root == nil {
		b.root = b.arena.New(node.DocumentNode, "#document")
		b.parent = b.root
	}
	return b.parent
}

// Document returns the native tree. A builder with embedded wire fragments
// can only be serialized, since those fragments have no corresponding nodes.
func (b *Builder) Document() *node.Node {
	if len(b.fragments) != 0 {
		panic("markup: embedded HTML in native document")
	}
	b.current()
	return b.root
}

func (b *Builder) Reset() {
	b.arena.Reset()
	b.root, b.parent = nil, nil
	clear(b.fragments)
	b.size = 0
}

// Element appends a child. Attributes are alternating names and unescaped values.
func (b *Builder) Element(tag string, attributes ...string) *node.Node {
	parent := b.current()
	n := b.arena.New(node.ElementNode, tag)
	parent.AppendChild(n)
	for i := 0; i < len(attributes); i += 2 {
		n.SetAttr(attributes[i], attributes[i+1])
		b.size += len(attributes[i]) + escapedSize(attributes[i+1], true) + 4
	}
	b.size += 2*len(tag) + 5
	return n
}

// Open makes the new element the parent of subsequent children.
func (b *Builder) Open(tag string, attributes ...string) *node.Node {
	b.parent = b.Element(tag, attributes...)
	return b.parent
}

// End returns to the enclosing element.
func (b *Builder) End() {
	if b.parent == nil || b.parent == b.root {
		panic("markup: no open element")
	}
	b.parent = b.parent.Parent
}

func (b *Builder) Text(value string) {
	if value != "" {
		parent := b.current()
		parent.AppendChild(b.arena.New(node.TextNode, value))
		b.size += escapedSize(value, false)
	}
}

func (b *Builder) TextElement(tag, value string, attributes ...string) *node.Node {
	n := b.Open(tag, attributes...)
	b.Text(value)
	b.End()
	return n
}

// Embed inserts a previously serialized, trusted HTML fragment at the wire
// boundary. Native documents should attach nodes directly instead.
func (b *Builder) Embed(fragment string) {
	if fragment == "" {
		return
	}
	parent := b.current()
	n := b.arena.New(node.DocumentNode, "#document")
	parent.AppendChild(n)
	if b.fragments == nil {
		b.fragments = make(map[*node.Node]string)
	}
	b.fragments[n] = fragment
	b.size += len(fragment)
}

// SizeHint estimates the encoded size of authored content, including escaping.
// Direct mutations of the returned nodes do not update the estimate.
func (b *Builder) SizeHint() int { return b.size }

func (b *Builder) String() string {
	document, _ := b.StringWithSubtree(nil)
	return document
}

// StringWithSubtree returns the serialized document and the designated subtree's
// slice of the same string. A missing subtree yields an empty slice.
func (b *Builder) StringWithSubtree(subtree *node.Node) (string, string) {
	e := encoder{fragments: b.fragments, subtree: subtree}
	e.out.Grow(b.size)
	if b.root != nil {
		e.write(b.root)
	}
	document := e.out.String()
	return document, document[e.start:e.end]
}
