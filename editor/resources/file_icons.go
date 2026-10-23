package resources

import (
	_ "embed"
	"encoding/json"
	"path"
	"strings"
	"sync"

	"github.com/diiyw/onix/graphics"
	graphicspng "github.com/diiyw/onix/graphics/codec/png"
)

//go:embed images/file-icons.png
var fileIconsPNG []byte

//go:embed images/file-icons.atlas.json
var fileIconsMetadata []byte

var fileIcons struct {
	sync.Mutex
	atlas *graphics.Atlas
}

// FileIconTexture reuses regions of one shared texture; warmed lookups allocate
// nothing. The retained PNG can recreate the atlas after a device is disposed.
func FileIconTexture(name string) *graphics.Texture {
	fileIcons.Lock()
	defer fileIcons.Unlock()
	if fileIcons.atlas == nil || !fileIcons.atlas.Valid() {
		var metadata graphics.AtlasMetadata
		if err := json.Unmarshal(fileIconsMetadata, &metadata); err != nil {
			panic(err)
		}
		texture, err := graphicspng.Decode(fileIconsPNG, graphics.TextureOptions{Mipmap: true, Usage: graphics.ResourceUsageDefault})
		if err != nil {
			panic(err)
		}
		fileIcons.atlas, err = graphics.NewAtlas(texture, &metadata)
		if err != nil {
			panic(err)
		}
	}
	return fileIcons.atlas.SubTexture(name)
}

// FileIconName identifies formats without opening files. Structured resource
// suffixes take precedence over the generic JSON icon.
func FileIconName(filename string, directory bool) string {
	if directory {
		return "folder"
	}
	name := strings.ToLower(path.Base(filename))
	extension := path.Ext(name)
	if extension == ".json" {
		if name == "mod.json" {
			return "prefab"
		}
		name = strings.TrimSuffix(name, extension)
		extension = path.Ext(name)
		if strings.HasPrefix(filename, "resources/scenes/") || extension == ".scene" {
			return "scene"
		}
		switch extension {
		case ".mat", ".material":
			return "material"
		case ".anim", ".animation":
			return "animation"
		case ".tile", ".tilemap", ".tileset":
			return "tile"
		case ".prefab":
			return "prefab"
		case ".atlas":
			return "image"
		default:
			return "json"
		}
	}
	switch extension {
	case ".ttf", ".otf", ".woff", ".woff2":
		return "font"
	case ".gltf", ".glb", ".obj", ".fbx", ".mqo", ".stl", ".blend":
		return "model"
	case ".wav", ".mp3", ".ogg", ".flac", ".aac", ".m4a":
		return "audio"
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg", ".avif", ".ktx", ".ktx2", ".dds", ".ontex", ".atlas", ".hdr", ".exr":
		return "image"
	case ".mat", ".material":
		return "material"
	case ".wgsl", ".glsl", ".vert", ".frag", ".geom", ".comp", ".hlsl", ".metal", ".spv", ".shader":
		return "shader"
	case ".scene":
		return "scene"
	case ".prefab":
		return "prefab"
	case ".go", ".js", ".ts", ".jsx", ".tsx", ".lua", ".py", ".cs", ".rs", ".c", ".h", ".cpp", ".sh":
		return "script"
	case ".mp4", ".webm", ".mov", ".avi", ".mkv":
		return "video"
	case ".anim", ".animation", ".skel":
		return "animation"
	case ".txt", ".md", ".html", ".css", ".xml", ".yaml", ".yml", ".toml", ".csv", ".mod", ".sum":
		return "text"
	case ".tile", ".tilemap", ".tileset", ".tmx", ".tmj", ".tsj":
		return "tile"
	default:
		return "binary"
	}
}
