package editor

import (
	"strconv"
	"strings"

	"github.com/diiyw/onix/ecs"
	editorresources "github.com/diiyw/onix/editor/resources"
	htmlui "github.com/diiyw/onix/ui/html"
)

func (a *ModuleEditor) layoutWorkspaceWelcome(world *ecs.World) {
	p := a.dialogPanel(760, 640)
	u := &a.dialogUI
	u.htmlDialog(world, "welcome", p, 82)
	registerEditorHTMLDialogTexture(world, "welcome-logo", editorresources.LogoTexture(editorresources.Logo64))
	paths := a.settings.RecentProjects[:min(len(a.settings.RecentProjects), recentProjectLimit)]
	rows := make([]recentProjectRow, len(paths))
	for i, path := range paths {
		row := rect{X: p.X + 24, Y: p.Y + 136 + float32(i)*48, W: p.W - 48, H: 42}
		rows[i] = recentProjectRow{Path: path, Rect: row, RemoveRect: rect{X: row.X + row.W - 34, Y: row.Y + 8, W: editorCompactControlHeight, H: editorCompactControlHeight}}
	}
	open := rect{X: p.X + p.W - 24 - buttonTextWidth("Open Project", true), Y: p.Y + p.H - 48, W: buttonTextWidth("Open Project", true), H: editorCompactControlHeight}
	newWidth := buttonTextWidth("New Project", true)
	htmlui.UpdateComponentProps[welcomeComponentProps, welcomeComponentState](world, u.htmlDialogComponents["welcome"], welcomeComponentProps{
		Root: u.rootRect, Panel: p, CloseRect: panelCloseRect(p), OpenRect: open,
		NewRect: rect{X: open.X - newWidth - 12, Y: open.Y, W: newWidth, H: open.H}, Rows: rows,
	})
}

func (a *ModuleEditor) handleWorkspaceWelcome(id string) {
	switch id {
	case "welcome-close":
		a.closeWorkspaceDialog()
	case "welcome-open":
		a.chromeAction("file.open-project")
	case "welcome-new":
		a.closeWorkspaceDialog()
		a.action("file.new-project")
	default:
		if value, ok := strings.CutPrefix(id, "welcome-recent-"); ok {
			indexText, operation, _ := strings.Cut(value, "-")
			index, err := strconv.Atoi(indexText)
			if err != nil || index < 0 || index >= len(a.settings.RecentProjects) {
				return
			}
			path := a.settings.RecentProjects[index]
			if operation == "remove" {
				a.report(a.settings.forgetRecentProject(path))
				return
			}
			if operation == "button" {
				if err := a.open(path); err != nil {
					a.report(err)
				}
			}
		}
	}
}
