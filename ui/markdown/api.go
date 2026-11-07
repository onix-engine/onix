package markdown

import (
	"github.com/onix-engine/onix/ecs"
	engineui "github.com/onix-engine/onix/ui"
)

func SyncSystem(name string) ecs.SystemConfig { return engineui.SyncSystem(name) }

// MarkdownSystem runs the Markdown-to-HTML conversion stage without invoking
// the complete UI synchronization pipeline. Feature integrations use it when
// registering the optional runtime extension.
func MarkdownSystem(name string) ecs.SystemConfig {
	if name == "" {
		name = "markdown_sync"
	}
	return ecs.SystemConfig{Name: name, System: syncMarkdown}
}
