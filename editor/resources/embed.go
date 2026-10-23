package resources

import (
	"embed"
	"sync"

	"github.com/diiyw/onix/graphics"
	graphicssvg "github.com/diiyw/onix/graphics/codec/svg"
	"github.com/diiyw/onix/images/svg"
)

type LogoSize uint16

const (
	Logo16   LogoSize = 16
	Logo32   LogoSize = 32
	Logo48   LogoSize = 48
	Logo64   LogoSize = 64
	Logo128  LogoSize = 128
	Logo512  LogoSize = 512
	Logo1024 LogoSize = 1024
)

//go:embed ui/tilebar.html
var TitlebarHTML string

//go:embed ui/workspace.html
var WorkspaceHTML string

//go:embed ui/workspace-scenes.html
var WorkspaceScenesHTML string

//go:embed ui/workspace-files.html
var WorkspaceFilesHTML string

//go:embed ui/workspace-inspector.html
var WorkspaceInspectorHTML string

//go:embed ui/workspace.css
var WorkspaceCSS string

//go:embed environment/studio.png
var StudioEnvironment []byte

//go:embed images/logo.svg
var images embed.FS

var (
	logoMu       sync.Mutex
	logoTextures = map[LogoSize]*graphics.Texture{}
)

func LogoData() ([]byte, error) {
	return images.ReadFile("images/logo.svg")
}

func LogoTexture(size LogoSize) *graphics.Texture {
	logoMu.Lock()
	defer logoMu.Unlock()
	if texture := logoTextures[size]; texture != nil && texture.Valid() {
		return texture
	}
	delete(logoTextures, size)

	data, err := LogoData()
	if err != nil {
		panic(err)
	}
	texture, err := graphicssvg.Decode(data, svg.Options{Width: int(size), Height: int(size)}, graphics.TextureOptions{
		Mipmap:           true,
		PremultiplyAlpha: true,
		Usage:            graphics.ResourceUsageGPU,
	})
	if err != nil {
		panic(err)
	}

	logoTextures[size] = texture
	return texture
}

// dialogHTML contains the HTML component templates used by editor modal
// dialogs. Each dialog owns its markup, styles and native form controls.
//
//go:embed dialog/*.html
var dialogHTML embed.FS

func DialogHTML(name string) string {
	data, err := dialogHTML.ReadFile("dialog/" + name + ".html")
	if err != nil {
		panic(err)
	}
	return string(data)
}
