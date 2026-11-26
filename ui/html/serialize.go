package html

import (
	"fmt"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/html/markup"
)

// Serialize returns the current retained HTML document for page.
//
// The returned string is HTML markup, rather than the diagnostic tree emitted
// by uinode.Node.String. The retained DOM is serialized after all mutations that
// have already been applied to the page, so callers should run Sync after
// changing component state when the serialized output must include that
// change.
func Serialize(world *ecs.World, page ecs.Entity) (string, error) {
	root, err := Root(world, page)
	if err != nil {
		return "", err
	}
	return markup.HTML(root.node), nil
}

// Serialize serializes this retained node and all of its descendants as HTML
// markup. Root is the usual entry point when the whole page is needed.
func (n Node) Serialize() (string, error) {
	if !n.valid() {
		return "", fmt.Errorf("ecs: html node is invalid")
	}
	return markup.HTML(n.node), nil
}
