package editor

import (
	"fmt"
	"path/filepath"
	"sync"

	editorresources "github.com/diiyw/onix/editor/resources"
	"github.com/diiyw/onix/ui/html"
)

type welcomeComponentProps struct {
	Root      rect
	Panel     rect
	CloseRect rect
	NewRect   rect
	OpenRect  rect
	Rows      []recentProjectRow
}

type welcomeComponentState struct{}

type welcomeRecentProjectProps struct {
	Index      int
	Panel      rect
	Row        rect
	RemoveRect rect
	Path       string
}

var (
	welcomeDefinitionsOnce sync.Once
	welcomeDefinition      *html.Definition[welcomeComponentProps, welcomeComponentState]
	welcomeRowDefinition   *html.Definition[welcomeRecentProjectProps, struct{}]
	welcomeDefinitionErr   error
)

func editorWelcomeDefinitions() (*html.Definition[welcomeComponentProps, welcomeComponentState], error) {
	welcomeDefinitionsOnce.Do(func() {
		welcomeRowDefinition, welcomeDefinitionErr = html.DefineScoped[welcomeRecentProjectProps, struct{}](
			html.NewTemplate(`<div data-ref="row" class="welcome-recent-row">
  <button data-ref="button" class="welcome-row-button" type="button">
    <img data-ref="icon" class="welcome-row-icon" src="onix-titlebar/folder.svg" alt="">
    <span data-ref="name" class="welcome-row-name"></span>
    <span data-ref="path" class="welcome-row-path"></span>
  </button>
  <button data-ref="remove" class="welcome-row-remove icon-only" type="button" aria-label="Remove">
    <img data-ref="remove-icon" src="onix-titlebar/trash-2.svg" alt="">
  </button>
</div>`), "", html.Behavior[welcomeRecentProjectProps, struct{}]{
				Render: func(context html.RenderContext[welcomeRecentProjectProps, struct{}], view *html.View) {
					props := context.Props
					view.Attr("row", "id", welcomeRecentProjectID(props.Index))
					view.Attr("row", "style", welcomeRelativeRectStyle(props.Row, props.Panel))
					view.Attr("button", "id", welcomeRecentProjectID(props.Index)+"-button")
					view.Attr("icon", "id", welcomeRecentProjectID(props.Index)+"-icon")
					view.Attr("name", "id", welcomeRecentProjectID(props.Index)+"-name")
					view.Attr("path", "id", welcomeRecentProjectID(props.Index)+"-path")
					view.Attr("remove", "id", welcomeRecentProjectID(props.Index)+"-remove")
					view.Attr("remove", "style", welcomeRelativeRectStyle(relativeRect(props.RemoveRect, props.Row), rect{}))
					view.Text("name", filepath.Base(props.Path))
					view.Text("path", props.Path)
				},
			})
		if welcomeDefinitionErr != nil {
			return
		}
		welcomeDefinition, welcomeDefinitionErr = html.DefineScoped[welcomeComponentProps, welcomeComponentState](
			html.NewTemplate(editorresources.DialogHTML("welcome")), "", html.Behavior[welcomeComponentProps, welcomeComponentState]{
				Render: func(context html.RenderContext[welcomeComponentProps, welcomeComponentState], view *html.View) {
					props := context.Props
					view.Attr("root", "style", editorDialogRootStyle(props.Root))
					view.Attr("overlay", "style", fmt.Sprintf("left:0;top:0;width:%gpx;height:%gpx", props.Root.W, props.Root.H))
					view.Attr("panel", "style", fmt.Sprintf("left:%gpx;top:%gpx;width:%gpx;height:%gpx", props.Panel.X, props.Panel.Y, props.Panel.W, props.Panel.H))
					view.Attr("logo", "src", "onix-dialog/welcome-logo")
					view.Attr("close", "style", welcomeRelativeRectStyle(props.CloseRect, props.Panel))
					view.Attr("new", "style", welcomeRelativeRectStyle(props.NewRect, props.Panel))
					view.Attr("open", "style", welcomeRelativeRectStyle(props.OpenRect, props.Panel))
					if len(props.Rows) != 0 {
						view.Attr("empty", "hidden", "")
					} else {
						view.RemoveAttr("empty", "hidden")
					}
					var children [recentProjectLimit]html.ComponentVNode
					count := min(len(props.Rows), len(children))
					for index := 0; index < count; index++ {
						row := props.Rows[index]
						children[index] = html.ComponentChild(uint64(index), welcomeRowDefinition, welcomeRecentProjectProps{
							Index: index, Panel: props.Panel, Row: row.Rect, RemoveRect: row.RemoveRect, Path: row.Path,
						}, struct{}{})
					}
					view.Components("recent", children[:count])
				},
			})
	})
	return welcomeDefinition, welcomeDefinitionErr
}

func welcomeRecentProjectID(index int) string {
	return fmt.Sprintf("welcome-recent-%d", index)
}

func relativeRect(value, origin rect) rect {
	return rect{X: value.X - origin.X, Y: value.Y - origin.Y, W: value.W, H: value.H}
}

func welcomeRelativeRectStyle(value, origin rect) string {
	return fmt.Sprintf("position:absolute;left:%gpx;top:%gpx;width:%gpx;height:%gpx;right:auto;bottom:auto", value.X-origin.X, value.Y-origin.Y, value.W, value.H)
}
