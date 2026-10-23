package editor

import "github.com/diiyw/onix/ui/html"

// editorVNodeView retains VNode slices for each group. It builds typed nodes,
// never markup; user values reach the DOM as text or attribute values.
// A group's key is independent of its position in the parent's child list.
type editorVNodeView struct {
	groups []editorVNodeGroup
	stack  []int
	used   int
}
type editorVNodeGroup struct {
	node     html.VNode
	children []html.VNode
	attrs    []html.VNodeAttribute
}

func editorVNodeKey(tag, value string) uint64 {
	key := uint64(14695981039346656037)
	for i := range len(tag) {
		key = (key ^ uint64(tag[i])) * 1099511628211
	}
	key = (key ^ uint64(':')) * 1099511628211
	for i := range len(value) {
		key = (key ^ uint64(value[i])) * 1099511628211
	}
	return key
}
func (b *editorVNodeView) reset() {
	b.used = 0
	b.stack = b.stack[:0]
	b.open("", "root")
}
func (b *editorVNodeView) open(tag, key string, attrs ...string) {
	index := b.used
	b.used++
	if index == len(b.groups) {
		b.groups = append(b.groups, editorVNodeGroup{})
	}
	group := &b.groups[index]
	clear(group.children)
	group.children = group.children[:0]
	clear(group.attrs)
	group.attrs = group.attrs[:0]
	group.node = html.VNode{Key: editorVNodeKey(tag, key), Tag: tag}
	for i := 0; i < len(attrs); i += 2 {
		group.attrs = append(group.attrs, html.VNodeAttribute{Name: attrs[i], Value: attrs[i+1]})
	}
	b.stack = append(b.stack, index)
}
func (b *editorVNodeView) attr(name, value string) {
	group := &b.groups[b.stack[len(b.stack)-1]]
	group.attrs = append(group.attrs, html.VNodeAttribute{Name: name, Value: value})
}
func (b *editorVNodeView) content(value string) {
	b.groups[b.stack[len(b.stack)-1]].node.Text = value
}
func (b *editorVNodeView) close() {
	index := b.stack[len(b.stack)-1]
	b.stack = b.stack[:len(b.stack)-1]
	group := &b.groups[index]
	group.node.Attributes, group.node.Children = group.attrs, group.children
	parent := &b.groups[b.stack[len(b.stack)-1]]
	parent.children = append(parent.children, group.node)
}
func (b *editorVNodeView) text(tag, key, value string, attrs ...string) {
	b.open(tag, key, attrs...)
	b.content(value)
	b.close()
}
func (b *editorVNodeView) nodes() []html.VNode {
	clear(b.groups[b.used:])
	b.groups = b.groups[:b.used]
	return b.groups[0].children
}
func (b *editorVNodeView) button(id, label string, attrs ...string) {
	b.open("button", id, "id", id, "type", "button")
	for i := 0; i < len(attrs); i += 2 {
		b.attr(attrs[i], attrs[i+1])
	}
	b.content(label)
	b.close()
}
func (b *editorVNodeView) input(id, label, value string) {
	b.text("label", id, label, "for", id)
	b.text("input", id, "", "id", id, "type", "text", "value", value)
}

func (b *editorVNodeView) append(nodes []html.VNode) {
	parent := &b.groups[b.stack[len(b.stack)-1]]
	parent.children = append(parent.children, nodes...)
}
