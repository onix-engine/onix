package icons

import (
	"embed"
	"fmt"
	"strings"
	"sync"

	"github.com/diiyw/onix/graphics"
	graphicssvg "github.com/diiyw/onix/graphics/codec/svg"
	"github.com/diiyw/onix/images/svg"
)

type Name string

const (
	AlignCenterHorizontal           Name = "align-center-horizontal"
	AlignCenterVertical             Name = "align-center-vertical"
	AlignEndHorizontal              Name = "align-end-horizontal"
	AlignEndVertical                Name = "align-end-vertical"
	AlignHorizontalDistributeCenter Name = "align-horizontal-distribute-center"
	AlignVerticalDistributeCenter   Name = "align-vertical-distribute-center"
	AlignStartHorizontal            Name = "align-start-horizontal"
	AlignStartVertical              Name = "align-start-vertical"
	Android                         Name = "android"
	Animation                       Name = "animation"
	AnimationEdit                   Name = "animation-edit"
	Arrow                           Name = "arrow"
	AudioFile                       Name = "audio-file"
	Build                           Name = "build"
	Button                          Name = "button"
	Camera                          Name = "camera"
	Checkbox                        Name = "checkbox"
	Close                           Name = "close"
	CodeFile                        Name = "code-file"
	Component                       Name = "component"
	Copy                            Name = "copy"
	Crosshair                       Name = "crosshair"
	Eye                             Name = "eye"
	EyeOff                          Name = "eye-off"
	Focus                           Name = "focus"
	Folder                          Name = "folder"
	Grid                            Name = "grid"
	Group                           Name = "group"
	ImageFile                       Name = "image-file"
	Info                            Name = "info"
	Input                           Name = "input"
	IOS                             Name = "ios"
	Layers                          Name = "layers"
	Light                           Name = "light"
	Model                           Name = "model"
	MoreHorizontal                  Name = "more-horizontal"
	Maximize                        Name = "maximize"
	MacOS                           Name = "macOS"
	Linux                           Name = "linux"
	Minimize                        Name = "minimize"
	Minus                           Name = "minus"
	MoveUp                          Name = "moveup"
	NumberInput                     Name = "number-input"
	Open                            Name = "open"
	Pause                           Name = "pause"
	Particles                       Name = "particles"
	Play                            Name = "play"
	Plus                            Name = "plus"
	Progress                        Name = "progress"
	Redo                            Name = "redo"
	Reload                          Name = "reload"
	Rotate                          Name = "rotate"
	Resource                        Name = "resource"
	Save                            Name = "save"
	ScrollArea                      Name = "scroll-area"
	ScrollContent                   Name = "scroll-content"
	Selector                        Name = "selector"
	Setting                         Name = "setting"
	Spine                           Name = "spine"
	Sprite                          Name = "sprite"
	Stop                            Name = "stop"
	Trash2                          Name = "trash-2"
	TextFile                        Name = "text-file"
	Textarea                        Name = "textarea"
	TileMap                         Name = "tile-map"
	Tree                            Name = "tree"
	UI                              Name = "ui"
	Undo                            Name = "undo"
	Up                              Name = "up"
	VideoFile                       Name = "video-file"
	Web                             Name = "web"
	WeChat                          Name = "wechat"
	Windows                         Name = "windows"
	WindowTitleBar                  Name = "window-title-bar"
)

//go:embed *.svg
var embedded embed.FS

var (
	cacheMu           sync.Mutex
	cache             = map[textureKey]*graphics.Texture{}
	quarterTurnsCache = map[quarterTurnsTextureKey]*graphics.Texture{}
)

type textureKey struct {
	name   Name
	width  int
	height int
	color  int
}

type quarterTurnsTextureKey struct {
	base  *graphics.Texture
	turns int
}

func Texture(name Name, width, height int, color graphics.ColorRGBA) *graphics.Texture {
	if width <= 0 {
		width = 1
	}
	if height <= 0 {
		height = 1
	}
	key := textureKey{name: name, width: width, height: height, color: color.Int32()}
	cacheMu.Lock()
	if texture, ok := cache[key]; ok && texture.Valid() {
		cacheMu.Unlock()
		return texture
	}
	delete(cache, key)
	cacheMu.Unlock()

	data, err := embedded.ReadFile(string(name) + ".svg")
	if err != nil {
		panic(err)
	}
	tinted := tintCurrentColor(string(data), color)
	texture, err := graphicssvg.Decode(
		[]byte(tinted),
		svg.Options{Width: width, Height: height},
		graphics.TextureOptions{Mipmap: true, Usage: graphics.ResourceUsageGPU},
	)
	if err != nil {
		panic(err)
	}

	cacheMu.Lock()
	cache[key] = texture
	cacheMu.Unlock()
	return texture
}

func QuarterTurnsTexture(name Name, width, height int, color graphics.ColorRGBA, turns int) *graphics.Texture {
	base := Texture(name, width, height, color)
	turns = ((turns % 4) + 4) % 4
	if turns == 0 {
		return base
	}
	key := quarterTurnsTextureKey{base: base, turns: turns}
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if texture := quarterTurnsCache[key]; texture != nil {
		return texture
	}

	u, v, angle := float32(0), float32(0), float32(turns*90)
	switch turns {
	case 1:
		u = 1
	case 2:
		u, v = 1, 1
	case 3:
		v = 1
	}
	rotatedWidth, rotatedHeight := width, height
	if turns%2 != 0 {
		rotatedWidth, rotatedHeight = height, width
	}
	texture := graphics.NewSubTexture(base, rotatedWidth, rotatedHeight, u, v, 1, 1, angle)
	quarterTurnsCache[key] = texture
	return texture
}

func tintCurrentColor(source string, color graphics.ColorRGBA) string {
	hex := colorHex(color)
	source = strings.ReplaceAll(source, "currentColor", hex)
	return strings.ReplaceAll(source, "currentcolor", hex)
}

func colorHex(color graphics.ColorRGBA) string {
	goColor := color.GoColor()
	return fmt.Sprintf("#%02x%02x%02x%02x", goColor.R, goColor.G, goColor.B, goColor.A)
}
