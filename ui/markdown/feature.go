package markdown

import (
	"github.com/diiyw/onix"
	"github.com/diiyw/onix/ecs"
	engineui "github.com/diiyw/onix/ui"
)

// Feature installs Markdown conversion on top of the HTML UI runtime.
// Keeping it in this package makes Markdown's parser and syntax highlighter a
// link-time optional dependency.
type Feature struct{}

func (Feature) InstallMarkdown(app *onix.App) error {
	if err := onix.InstallSystems[onix.UIHTML](app); err != nil {
		return err
	}
	engineui.RegisterRuntimeExtension(app.World, engineui.RuntimeExtension{Sync: syncMarkdown})
	return nil
}

func syncMarkdown(context *ecs.SystemContext) error {
	if context != nil && context.World != nil {
		syncMarkdownComponents(context.World)
	}
	return nil
}
