package html

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/html/css"
	"github.com/onix-engine/onix/html/element"
	"github.com/onix-engine/onix/html/parse"
	uinode "github.com/onix-engine/onix/scene/node"
)

func loadHTMLDocument(world *ecs.World, owner ecs.Entity, state *htmlRenderState, value *Surface) error {
	root, err := readHTMLTree(*value)
	if err != nil {
		return err
	}
	compact := value.CompactDOM && value.Document == nil
	state.resetNodeIndex()
	root, embeddedCSS := initializeHTMLDocument(root, compact, state.indexNode)
	if !slices.Equal(embeddedCSS, state.EmbeddedCSS) {
		state.EmbeddedCSS = embeddedCSS
		state.Stylesheet = nil
	}
	if compact {
		value.Value = ""
		world.GetMut[Surface](owner).Value = ""
	}
	state.Root, state.Source = root, value.Value
	state.SourceDirty, state.IndexDirty, state.FromSource = false, false, value.Document == nil
	state.SourceCompacted = compact
	state.Textures.dispose()
	clear(state.Animations)
	clear(state.ActiveAnimations)
	state.ActiveAnimations = state.ActiveAnimations[:0]
	state.AnimationTime = 0
	clear(state.AnimationBaselines)
	state.TransformFrameSet = false
	state.StyleCache.InvalidateDefinitions()
	state.Input = css.State{}
	clear(state.StateRoots[:])
	clear(state.NodeOffsets)
	state.NodeOffsets = state.NodeOffsets[:0]
	clear(state.NodeScrolls)
	state.NodeScrolls = state.NodeScrolls[:0]
	clear(state.NodeEvents)
	clear(state.EventTargets)
	clear(state.DetachedFields)
	state.PaintCullDisabled, state.PaintCullOffsets = false, nil
	clear(state.InheritedClips[:])
	state.NextInheritedClip = 0
	state.Revision++
	return nil
}

func prepareHTMLDocument(value Surface, visit func(*uinode.Node)) (*uinode.Node, []string, error) {
	root, err := readHTMLTree(value)
	if err != nil {
		return nil, nil, err
	}
	root, embeddedCSS := initializeHTMLDocument(root, false, visit)
	return root, embeddedCSS, nil
}

func readHTMLTree(value Surface) (*uinode.Node, error) {
	if value.Document == nil {
		return parse.ParseTree(value.Value)
	}
	if value.Document.Type != uinode.DocumentNode {
		return nil, fmt.Errorf("html: supplied document root must be a document node")
	}
	return value.Document, nil
}

func initializeHTMLDocument(root *uinode.Node, compact bool, visit func(*uinode.Node)) (*uinode.Node, []string) {
	var embeddedCSS []string
	keep := func(child *uinode.Node) bool {
		if child.Type != uinode.ElementNode || !isHTMLMetadataElement(child.Data) {
			return true
		}
		collectHTMLStyles(child, &embeddedCSS)
		return false
	}
	if compact {
		element.InitializeDocument(root, nil, keep)
		var arena uinode.Arena
		root = arena.CloneCompact(root)
		walkHTMLDocument(root, visit)
	} else {
		element.InitializeDocument(root, visit, keep)
	}
	return root, embeddedCSS
}

func walkHTMLDocument(root *uinode.Node, visit func(*uinode.Node)) {
	if visit == nil {
		return
	}
	visit(root)
	for _, child := range root.Children {
		walkHTMLDocument(child, visit)
	}
}

// Metadata such as head can contain multiple independently parsed sheets.
func collectHTMLStyles(node *uinode.Node, parts *[]string) {
	if node.Type == uinode.ElementNode && node.Data == "style" {
		var text string
		var joined strings.Builder
		for _, child := range node.Children {
			if child.Type != uinode.TextNode || child.Data == "" {
				continue
			}
			if text == "" {
				text = child.Data
			} else {
				if joined.Len() == 0 {
					joined.WriteString(text)
				}
				joined.WriteString(child.Data)
			}
		}
		if joined.Len() != 0 {
			*parts = append(*parts, joined.String())
		} else if text != "" {
			// The sheet can survive replacement of an otherwise unrelated DOM.
			*parts = append(*parts, strings.Clone(text))
		}
		return
	}
	for _, child := range node.Children {
		if child != nil {
			collectHTMLStyles(child, parts)
		}
	}
}

func (state *htmlDocumentState) resetNodeIndex() {
	if state.NodeByID == nil {
		state.NodeByID = make(map[string]*uinode.Node)
	} else {
		clear(state.NodeByID)
	}
}

func (state *htmlDocumentState) indexNode(node *uinode.Node) {
	if id := node.AttrValue("id"); id != "" && state.NodeByID[id] == nil {
		state.NodeByID[id] = node
	}
}

func htmlStylesheet(cssText string, embeddedCSS []string, fontSize float64) (*css.Stylesheet, error) {
	var defaults [128]byte
	input := append(defaults[:0], ":root { font-size:"...)
	input = strconv.AppendFloat(input, fontSize, 'f', -1, 64)
	input = append(input, "px; }p { margin-top: 1em; }"...)
	var inline [8]string
	parts := append(inline[:0], string(input), cssText)
	parts = append(parts, embeddedCSS...)
	return css.ParseParts(parts...)
}
