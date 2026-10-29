// Command htmlcompat renders the browser compatibility fixture through Onix.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diiyw/onix"
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/font"
	"github.com/diiyw/onix/graphics"
	graphicspng "github.com/diiyw/onix/graphics/codec/png"
	graphicssvg "github.com/diiyw/onix/graphics/codec/svg"
	onixsvg "github.com/diiyw/onix/images/svg"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	htmlui "github.com/diiyw/onix/ui/html"
)

type fixtureImageProvider struct {
	base     string
	textures map[string]*graphics.Texture
}

func (p *fixtureImageProvider) ResolveHTMLImage(source string) (*graphics.Texture, bool) {
	if p == nil || source == "" || strings.Contains(source, "://") || filepath.IsAbs(source) {
		return nil, false
	}
	if texture, ok := p.textures[source]; ok {
		return texture, texture != nil && texture.Valid()
	}
	data, err := os.ReadFile(filepath.Join(p.base, filepath.FromSlash(source)))
	var texture *graphics.Texture
	if err == nil {
		options := graphics.TextureOptions{Usage: graphics.ResourceUsageGPU, PremultiplyAlpha: true}
		switch strings.ToLower(filepath.Ext(source)) {
		case ".svg":
			texture, err = graphicssvg.Decode(data, onixsvg.Options{}, options)
		case ".png":
			texture, err = graphicspng.Decode(data, options)
		}
	}
	if err != nil {
		texture = nil
	}
	if p.textures == nil {
		p.textures = make(map[string]*graphics.Texture)
	}
	p.textures[source] = texture
	return texture, texture != nil && texture.Valid()
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--raster-info" {
		fmt.Println(font.RasterizerVersion())
		return
	}
	fixture := "tests/htmlcompat/resources/batch1.html"
	if len(os.Args) > 1 {
		fixture = os.Args[1]
	}
	source, err := os.ReadFile(fixture)
	if err != nil {
		panic(err)
	}
	app := onix.NewApp()
	if err := app.InstallSystems[onix.UIHTML](); err != nil {
		panic(err)
	}
	app.SetWindowSize(1200, 650)
	app.SetWindowTitle("Onix HTML compatibility")
	if filepath.Base(fixture) == "workbench.html" {
		app.World.InsertResource[htmlui.ImageProvider](&fixtureImageProvider{base: filepath.Dir(fixture)})
	}
	bundle := ecs.NewBundle(1)
	html := htmlui.New(string(source))
	html.Width, html.Height, html.Font = 1200, 650, notosanssc.NotoSansSCRegular
	bundle.Put(html)
	app.World.Spawn(bundle)
	frames := 0
	if err := app.AddSystem(onix.StageUpdate, ecs.SystemConfig{Name: "compat_ready", System: func(*ecs.SystemContext) error {
		frames++
		if frames == 3 {
			fmt.Println("htmlcompat-ready")
		}
		return nil
	}}); err != nil {
		panic(err)
	}
	if err := app.Run(); err != nil {
		panic(err)
	}
}
