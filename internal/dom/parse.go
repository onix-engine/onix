package dom

import (
	"strings"

	nethtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func Parse(input string) (*Node, error) {
	return parseHTML5(input)
}

// parseHTML5 delegates malformed and omitted-tag input to the WHATWG tree
// builder already shipped in x/net/html. Fragments stay fragment-shaped so
// existing callers do not gain synthetic document wrappers.
func parseHTML5(input string) (*Node, error) {
	var nodes []*nethtml.Node
	var err error
	if isHTMLDocument(input) {
		var document *nethtml.Node
		document, err = nethtml.Parse(strings.NewReader(input))
		if err == nil {
			nodes = []*nethtml.Node{document}
		}
	} else {
		context := &nethtml.Node{Type: nethtml.ElementNode, Data: "body", DataAtom: atom.Body}
		nodes, err = nethtml.ParseFragment(strings.NewReader(input), context)
	}
	if err != nil {
		return nil, err
	}
	root := NewDocument()
	for _, source := range nodes {
		if source.Type == nethtml.DocumentNode {
			for child := source.FirstChild; child != nil; child = child.NextSibling {
				if child.Type == nethtml.DoctypeNode || child.Type == nethtml.CommentNode {
					continue
				}
				if converted := convertHTML5Node(child); converted != nil {
					root.AppendChild(converted)
				}
			}
			continue
		}
		if converted := convertHTML5Node(source); converted != nil {
			root.AppendChild(converted)
		}
	}
	InitializeElementStates(root)
	return root, nil
}

func isHTMLDocument(input string) bool {
	tokens := nethtml.NewTokenizer(strings.NewReader(input))
	for {
		switch tokens.Next() {
		case nethtml.CommentToken:
			continue
		case nethtml.TextToken:
			if strings.TrimSpace(string(tokens.Text())) == "" {
				continue
			}
			return false
		case nethtml.DoctypeToken:
			return true
		case nethtml.StartTagToken, nethtml.SelfClosingTagToken:
			name, _ := tokens.TagName()
			switch string(name) {
			case "html", "head", "body":
				return true
			}
			return false
		default:
			return false
		}
	}
}

func convertHTML5Node(source *nethtml.Node) *Node {
	if source == nil {
		return nil
	}
	var converted *Node
	switch source.Type {
	case nethtml.ElementNode:
		attributes := make(map[string]string, len(source.Attr))
		for _, attribute := range source.Attr {
			name := strings.ToLower(attribute.Key)
			if _, exists := attributes[name]; !exists {
				attributes[name] = attribute.Val
			}
		}
		if len(attributes) == 0 {
			attributes = nil
		}
		converted = NewElement(strings.ToLower(source.Data), attributes)
	case nethtml.TextNode:
		text := source.Data
		if text == "" {
			return nil
		}
		converted = NewText(text)
	default:
		return nil
	}
	for child := source.FirstChild; child != nil; child = child.NextSibling {
		if nested := convertHTML5Node(child); nested != nil {
			converted.AppendChild(nested)
		}
	}
	return converted
}
