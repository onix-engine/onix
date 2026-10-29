package editor

import (
	"fmt"
	"strings"
	"sync"

	editorresources "github.com/diiyw/onix/editor/resources"
	engineui "github.com/diiyw/onix/ui"
	"github.com/diiyw/onix/ui/html"
)

type editorDialogComponentProps struct {
	Frame *editorDialogFrame
}
type editorDialogComponentState struct{}

type editorDialogAttr struct {
	Name  string
	Value string
}

type editorDialogElement struct {
	ID          string
	Text        string
	TextSet     bool
	Button      bool
	Authored    bool
	Icon        string
	IconSource  string
	IconRef     string
	LabelRef    string
	Children    []html.VNode
	Style       string
	Class       string
	Source      string
	Name        string
	AriaLabel   string
	AriaPressed string
	Type        string
	Value       string
	ValueSet    bool
	Step        string
	Hidden      bool
	HiddenSet   bool
	Disabled    bool
	DisabledSet bool
	Checked     bool
	CheckedSet  bool
	Selected    string
	SelectedSet bool
	ReadOnly    bool
	ReadOnlySet bool
	FollowTail  bool
	Attrs       []editorDialogAttr
	RemoveAttrs []string
}

type editorDialogDynamic struct {
	Key      uint64
	Tag      string
	Text     string
	Attrs    []editorDialogAttr
	Children []html.VNode
}

type editorDialogSelect struct {
	ID       string
	Options  []string
	Selected string
}

// editorDialogFrame is the retained data model for one dialog render. The UI
// layout code only records values here; HTML component reconciliation owns the
// DOM writes and control lifetime.
type editorDialogFrame struct {
	Root     rect
	Panel    rect
	Elements map[string]*editorDialogElement
	Dynamic  []editorDialogDynamic
	Selects  []editorDialogSelect
	Scratch  []html.VNode
}

func (f *editorDialogFrame) reset(root, panel rect) {
	f.Root, f.Panel = root, panel
	// Authored controls stay in the component tree for the lifetime of the
	// dialog. Mark every previously bound node inactive first, then let the
	// current layout pass re-enable the nodes it uses. Keeping the descriptor
	// means a hidden input retains authored attributes such as type and
	// aria-label instead of being reset to Chromium's default text input.
	for _, element := range f.Elements {
		element.Hidden, element.HiddenSet = true, true
		element.Style = "display:none !important"
	}
	f.Dynamic = f.Dynamic[:0]
	f.Selects = f.Selects[:0]
	f.Scratch = f.Scratch[:0]
}

func (f *editorDialogFrame) element(id string) *editorDialogElement {
	if f.Elements == nil {
		f.Elements = make(map[string]*editorDialogElement)
	}
	if element, ok := f.Elements[id]; ok {
		return element
	}
	element := &editorDialogElement{ID: id}
	f.Elements[id] = element
	return element
}

func (f *editorDialogFrame) dynamic(element editorDialogDynamic) {
	f.Dynamic = append(f.Dynamic, element)
}

func (f *editorDialogFrame) selectElement(id string, options []string, selected string) {
	element := f.element(id)
	element.Selected, element.SelectedSet = selected, true
	for index := range f.Selects {
		if f.Selects[index].ID != id {
			continue
		}
		f.Selects[index].Options = append(f.Selects[index].Options[:0], options...)
		f.Selects[index].Selected = selected
		return
	}
	selectFrame := editorDialogSelect{ID: id, Selected: selected}
	selectFrame.Options = append(selectFrame.Options, options...)
	f.Selects = append(f.Selects, selectFrame)
}

func editorDialogDynamicKey(id string) uint64 {
	value := uint64(14695981039346656037)
	for index := 0; index < len(id); index++ {
		value ^= uint64(id[index])
		value *= 1099511628211
	}
	return value
}

func editorDialogStyle(root, panel rect) string {
	return fmt.Sprintf("position:absolute;left:%gpx;top:%gpx;width:%gpx;height:%gpx", panel.X-root.X, panel.Y-root.Y, panel.W, panel.H)
}

func editorDialogRootStyle(root rect) string {
	return fmt.Sprintf("position:absolute;left:0;top:0;width:%gpx;height:%gpx", root.W, root.H)
}

func renderEditorDialogComponent(context html.RenderContext[editorDialogComponentProps, editorDialogComponentState], view *html.View) {
	frame := context.Props.Frame
	if frame == nil {
		return
	}
	view.Attr("root", "style", editorDialogRootStyle(frame.Root))
	view.Attr("overlay", "style", fmt.Sprintf("left:0;top:0;width:%gpx;height:%gpx", frame.Root.W, frame.Root.H))
	view.Attr("panel", "style", editorDialogStyle(frame.Root, frame.Panel))
	for _, element := range frame.Elements {
		if element.Button && !element.Authored {
			continue
		}
		if element.Style != "" {
			view.Attr(element.ID, "style", element.Style)
		}
		if element.Class != "" {
			view.Attr(element.ID, "class", element.Class)
		}
		if element.Source != "" {
			view.Attr(element.ID, "src", element.Source)
		}
		if element.Name != "" {
			view.Attr(element.ID, "name", element.Name)
		}
		if element.AriaLabel != "" {
			view.Attr(element.ID, "aria-label", element.AriaLabel)
		}
		if element.AriaPressed != "" {
			view.Attr(element.ID, "aria-pressed", element.AriaPressed)
		}
		if element.Type != "" {
			view.Attr(element.ID, "type", element.Type)
		}
		if element.ValueSet {
			view.Attr(element.ID, "value", element.Value)
		}
		if element.Step != "" {
			view.Attr(element.ID, "step", element.Step)
		}
		if element.HiddenSet {
			if element.Hidden {
				view.Attr(element.ID, "hidden", "")
			} else {
				view.RemoveAttr(element.ID, "hidden")
			}
		}
		if element.DisabledSet {
			if element.Disabled {
				view.Attr(element.ID, "disabled", "")
			} else {
				view.RemoveAttr(element.ID, "disabled")
			}
		}
		if element.CheckedSet {
			if element.Checked {
				view.Attr(element.ID, "checked", "")
			} else {
				view.RemoveAttr(element.ID, "checked")
			}
		}
		if element.ReadOnlySet {
			if element.ReadOnly {
				view.Attr(element.ID, "readonly", "")
			} else {
				view.RemoveAttr(element.ID, "readonly")
			}
		}
		for _, attr := range element.Attrs {
			view.Attr(element.ID, attr.Name, attr.Value)
		}
		for _, attr := range element.RemoveAttrs {
			view.RemoveAttr(element.ID, attr)
		}
		if element.Button {
			iconRef, labelRef := element.IconRef, element.LabelRef
			// Static icons belong to the template; only dynamic choices override them.
			if element.IconSource != "" {
				view.Attr(iconRef, "src", element.IconSource)
				view.RemoveAttr(iconRef, "style")
			}
			if strings.Contains(element.Class, "icon-only") {
				view.Attr(labelRef, "style", "display:none")
			} else {
				view.RemoveAttr(labelRef, "style")
			}
			if element.TextSet {
				view.Text(labelRef, element.Text)
			}
		} else if element.TextSet {
			view.Text(element.ID, element.Text)
		}
	}
	if len(frame.Dynamic) > 0 {
		for _, dynamic := range frame.Dynamic {
			frame.Scratch = append(frame.Scratch, editorDialogVNode(dynamic))
		}
	}
	view.Keyed("content", frame.Scratch)
	for _, selectFrame := range frame.Selects {
		frame.Scratch = frame.Scratch[:0]
		for index, option := range selectFrame.Options {
			attrs := []html.VNodeAttribute{{Name: "value", Value: option}}
			if option == selectFrame.Selected {
				attrs = append(attrs, html.VNodeAttribute{Name: "selected", Value: ""})
			}
			frame.Scratch = append(frame.Scratch, html.VNode{Key: uint64(index + 1), Tag: "option", Text: option, Attributes: attrs})
		}
		view.Keyed(selectFrame.ID, frame.Scratch)
	}
}

func updateEditorDialogComponent(context *html.ComponentContext[editorDialogComponentProps, editorDialogComponentState]) {
	frame := context.Props.Frame
	if frame == nil {
		return
	}
	component, ok := context.World.Get[html.Component[editorDialogComponentProps, editorDialogComponentState]](context.Component)
	if !ok {
		return
	}
	for entity, access := range context.World.Query[html.Control]().All {
		control := access.Get()

		if control.Owner != component.Page {
			continue
		}
		element := frame.Elements[control.ID]
		if element == nil || element.Hidden {
			continue
		}
		value, set := element.Value, element.ValueSet
		if control.Tag == "textarea" {
			value, set = element.Text, element.TextSet
			if textarea := context.World.GetMut[engineui.Textarea](entity); textarea != nil {
				textarea.FollowTail = element.FollowTail
			}
		}
		if set && control.Value != value {
			engineui.SetHTMLControlValue(context.World, entity, value)
		}
		if element.CheckedSet && control.Checked != element.Checked {
			engineui.SetHTMLControlChecked(context.World, entity, element.Checked)
		}
		if element.SelectedSet && control.Value != element.Selected {
			for index, option := range control.Options {
				if option.Value != element.Selected {
					continue
				}
				engineui.SetHTMLControlSelected(context.World, entity, index)
				break
			}
		}
	}
}

func editorDialogVNode(dynamic editorDialogDynamic) html.VNode {
	attrs := make([]html.VNodeAttribute, len(dynamic.Attrs))
	for index, attr := range dynamic.Attrs {
		attrs[index] = html.VNodeAttribute{Name: attr.Name, Value: attr.Value}
	}
	children := dynamic.Children
	if dynamic.Text != "" {
		return html.VNode{Key: dynamic.Key, Tag: dynamic.Tag, Text: dynamic.Text, Attributes: attrs}
	}
	return html.VNodeElement(dynamic.Key, dynamic.Tag, attrs, children)
}

type editorDialogDefinitionCache struct {
	once       sync.Once
	definition *html.Definition[editorDialogComponentProps, editorDialogComponentState]
	err        error
}

var editorDialogDefinitions [len(editorDialogNames)]editorDialogDefinitionCache

var editorDialogNames = [...]string{
	"about",
	"build-error",
	"build-success",
	"build",
	"close-confirm",
	"preferences",
	"progress",
	"project-dialog",
	"scene-create",
	"module-create",
	"folder-create",
	"instance-duplicate",
	"project",
	"run-failure",
	"color-picker",
	"nine-slice",
}

func editorDialogDefinition(name string) (*html.Definition[editorDialogComponentProps, editorDialogComponentState], error) {
	for index, dialogName := range editorDialogNames {
		if dialogName != name {
			continue
		}
		cache := &editorDialogDefinitions[index]
		cache.once.Do(func() {
			cache.definition, cache.err = html.DefineScoped[editorDialogComponentProps, editorDialogComponentState](
				html.NewTemplate(editorresources.DialogHTML(name)), "", html.Behavior[editorDialogComponentProps, editorDialogComponentState]{
					Render: renderEditorDialogComponent,
					Update: updateEditorDialogComponent,
				},
			)
			if cache.err != nil {
				cache.err = fmt.Errorf("define editor dialog %q: %w", name, cache.err)
			}
		})
		return cache.definition, cache.err
	}
	return nil, fmt.Errorf("editor dialog %q is not component-backed", name)
}
