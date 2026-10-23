package editor

import (
	"runtime"
	"strconv"
	"sync"

	editorresources "github.com/diiyw/onix/editor/resources"
	"github.com/diiyw/onix/ui/html"
)

type titlebarComponentState struct{ Recent editorVNodeView }

var titlebarDefinitionCache struct {
	once       sync.Once
	definition *html.Definition[editorTitlebarView, titlebarComponentState]
	err        error
}

func editorTitlebarDefinition() (*html.Definition[editorTitlebarView, titlebarComponentState], error) {
	d := &titlebarDefinitionCache
	d.once.Do(func() {
		d.definition, d.err = html.Define(html.NewTemplate(editorresources.TitlebarHTML), "", html.Behavior[editorTitlebarView, titlebarComponentState]{Render: renderTitlebarComponent})
	})
	return d.definition, d.err
}
func componentDisabled(view *html.View, ref string, disabled bool) {
	if disabled {
		view.Attr(ref, "disabled", "")
	} else {
		view.RemoveAttr(ref, "disabled")
	}
}
func renderTitlebarComponent(context html.RenderContext[editorTitlebarView, titlebarComponentState], view *html.View) {
	props := context.Props
	class := "titlebar"
	if runtime.GOOS == "darwin" {
		class += " native-controls"
	}
	view.Attr("titlebar", "class", class)
	componentHidden(view, "window-controls", !props.customControls)
	for _, menu := range titlebarMenuDefinitions {
		open := !props.modal && props.menu == menu.key
		componentHidden(view, "popup-"+menu.key, !open)
		class := "menu-trigger"
		if open {
			class += " open"
		}
		view.Attr("menu-"+menu.key, "class", class)
		view.Attr("menu-"+menu.key, "aria-expanded", strconv.FormatBool(open))
		componentDisabled(view, "menu-"+menu.key, props.modal)
	}
	recentOpen := !props.modal && props.menu == "file" && props.submenu == "file.open-recent-project"
	componentHidden(view, "popup-recent", !recentOpen)
	view.Attr("file.open-recent-project", "aria-expanded", strconv.FormatBool(recentOpen))
	for _, id := range [...]string{"top-save", "top-reload", "top-frame", "window-minimize", "window-maximize", "window-close"} {
		componentDisabled(view, id, props.modal)
	}
	componentDisabled(view, "top-run", props.modal || !props.project)
	componentDisabled(view, "top-dimension", props.modal || !props.project)
	dimensionLabel, dimensionAction := "2D", "Switch to 3D view"
	if props.dimension == previewDimension3D {
		dimensionLabel, dimensionAction = "3D", "Switch to 2D view"
	}
	view.Text("top-dimension", dimensionLabel)
	view.Attr("top-dimension", "title", dimensionAction)
	view.Attr("top-dimension", "aria-label", dimensionAction)
	view.Attr("edit.toggle-grid-snap", "aria-pressed", strconv.FormatBool(props.snap))
	class = "tool"
	if props.dirty {
		class += " dirty"
	}
	view.Attr("top-save", "class", class)
	if props.running {
		view.Attr("top-run", "class", "tool running")
		view.Attr("top-run", "aria-label", "Stop project")
		view.Attr("top-run", "title", "Stop project")
		view.Attr("run-icon", "src", "onix-titlebar/stop.svg#danger")
	} else {
		view.Attr("top-run", "class", "tool")
		view.Attr("top-run", "aria-label", "Run project")
		view.Attr("top-run", "title", "Run (F6)")
		view.Attr("run-icon", "src", "onix-titlebar/play.svg")
	}
	componentHidden(view, "recent-empty", len(props.recent) != 0)
	b := &context.State.Recent
	b.reset()
	for index, path := range props.recent {
		b.open("button", path, "id", "recent-"+strconv.Itoa(index), "title", path, "type", "button", "class", "menu-item")
		b.text("img", "icon", "", "src", "onix-titlebar/folder.svg", "alt", "")
		b.text("span", "label", path, "class", "menu-label")
		b.close()
	}
	view.Keyed("popup-recent", b.nodes())
}
