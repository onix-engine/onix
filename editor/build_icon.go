package editor

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	editorresources "github.com/diiyw/onix/editor/resources"
	onixsvg "github.com/diiyw/onix/images/svg"
	_ "github.com/diiyw/onix/images/webp"
	"github.com/diiyw/onix/project"
)

type buildIcon struct {
	image  image.Image
	vector *onixsvg.Document
	svg    []byte
}

func loadBuildIcon(options BuildOptions) (buildIcon, error) {
	var (
		data []byte
		err  error
	)
	name := ""
	// The app icon is configured by the project manifest. Keep the explicit
	// option as a fallback for callers that build a project without one.
	if projectDir := strings.TrimSpace(options.ProjectDir); projectDir != "" {
		manifest, manifestErr := readOptionalJSONFile[project.ProjectSpec](filepath.Join(projectDir, "resources", projectFileName))
		if manifestErr != nil {
			return buildIcon{}, fmt.Errorf("read project icon configuration: %w", manifestErr)
		}
		iconPath := strings.TrimSpace(manifest.Window.Icon)
		if iconPath != "" {
			iconPath, err = project.NormalizeLocalResourcePath(iconPath)
			if err != nil {
				return buildIcon{}, fmt.Errorf("project window icon: %w", err)
			}
			name = filepath.Join(projectDir, "resources", filepath.FromSlash(iconPath))
		}
	}
	if name == "" {
		name = strings.TrimSpace(options.Icon)
	}
	if name == "" {
		data, err = editorresources.LogoData()
		name = "editor/resources/images/logo.svg"
	} else {
		if !filepath.IsAbs(name) {
			name = filepath.Join(options.ProjectDir, name)
		}
		data, err = os.ReadFile(name)
	}
	if err != nil {
		return buildIcon{}, fmt.Errorf("read build icon %s: %w", name, err)
	}
	if strings.EqualFold(filepath.Ext(name), ".svg") {
		document, err := onixsvg.Decode(bytes.NewReader(data))
		if err != nil {
			return buildIcon{}, fmt.Errorf("decode build icon %s: %w", name, err)
		}
		return buildIcon{vector: document, svg: data}, nil
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return buildIcon{}, fmt.Errorf("decode build icon %s: %w", name, err)
	}
	if decoded.Bounds().Dx() <= 0 || decoded.Bounds().Dy() <= 0 {
		return buildIcon{}, fmt.Errorf("build icon %s is empty", name)
	}
	return buildIcon{image: decoded}, nil
}

func (icon buildIcon) writePNG(name string, size int) error {
	data, err := icon.png(size)
	if err != nil {
		return err
	}
	return writeBuildIconFile(name, data)
}

func (icon buildIcon) png(size int) ([]byte, error) {
	raster, err := icon.rasterize(size)
	if err != nil {
		return nil, err
	}
	return encodeBuildIconPNG(raster, size)
}

func (icon buildIcon) macOSPNG(size int, background color.NRGBA) ([]byte, error) {
	raster, err := icon.rasterize(size)
	if err != nil {
		return nil, err
	}
	return encodeBuildIconPNG(flattenBuildIcon(raster, background), size)
}

func (icon buildIcon) writeOpaquePNG(name string, size int, background color.NRGBA) error {
	raster, err := icon.rasterize(size)
	if err != nil {
		return err
	}
	data, err := encodeBuildIconPNG(flattenBuildIcon(raster, background), size)
	if err != nil {
		return err
	}
	return writeBuildIconFile(name, data)
}

func (icon buildIcon) rasterize(size int) (*image.NRGBA, error) {
	if size <= 0 {
		return nil, fmt.Errorf("build icon size must be positive")
	}
	if icon.vector != nil {
		raster, err := icon.vector.Rasterize(size, size)
		if err != nil {
			return nil, fmt.Errorf("rasterize %dx%d SVG build icon: %w", size, size, err)
		}
		return raster, nil
	}
	if icon.image == nil {
		return nil, fmt.Errorf("build icon has no image data")
	}
	return resizeBuildIcon(icon.image, size), nil
}

func encodeBuildIconPNG(icon image.Image, size int) ([]byte, error) {
	var data bytes.Buffer
	if err := png.Encode(&data, icon); err != nil {
		return nil, fmt.Errorf("encode %dx%d build icon: %w", size, size, err)
	}
	return data.Bytes(), nil
}

func flattenBuildIcon(source *image.NRGBA, background color.NRGBA) *image.NRGBA {
	target := image.NewNRGBA(source.Bounds())
	for y := source.Bounds().Min.Y; y < source.Bounds().Max.Y; y++ {
		for x := source.Bounds().Min.X; x < source.Bounds().Max.X; x++ {
			pixel := source.NRGBAAt(x, y)
			alpha := uint32(pixel.A)
			blend := func(foreground, background uint8) uint8 {
				return uint8((uint32(foreground)*alpha + uint32(background)*(255-alpha) + 127) / 255)
			}
			target.SetNRGBA(x, y, color.NRGBA{
				R: blend(pixel.R, background.R),
				G: blend(pixel.G, background.G),
				B: blend(pixel.B, background.B),
				A: 255,
			})
		}
	}
	return target
}

// macOS adds a legacy backing plate to transparent app icons, so choose a
// stable opaque color from the artwork before writing the ICNS entries.
func macOSIconBackground(source image.Image) color.NRGBA {
	bounds := source.Bounds()
	var red, green, blue, alpha uint64
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := source.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			red += uint64(r)
			green += uint64(g)
			blue += uint64(b)
			alpha += uint64(a)
		}
	}
	if alpha == 0 {
		return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	}
	toByte := func(value uint64) uint8 {
		average := math.Round(float64(value) * 255 / float64(alpha))
		if average >= 255 {
			return 255
		}
		return uint8(max(0, int(average)))
	}
	return color.NRGBA{R: toByte(red), G: toByte(green), B: toByte(blue), A: 255}
}

func resizeBuildIcon(source image.Image, size int) *image.NRGBA {
	target := image.NewNRGBA(image.Rect(0, 0, size, size))
	bounds := source.Bounds()
	scale := math.Min(float64(size)/float64(bounds.Dx()), float64(size)/float64(bounds.Dy()))
	width := max(1, int(math.Round(float64(bounds.Dx())*scale)))
	height := max(1, int(math.Round(float64(bounds.Dy())*scale)))
	offsetX, offsetY := (size-width)/2, (size-height)/2
	for y := range height {
		sy := (float64(y)+0.5)/scale - 0.5
		for x := range width {
			sx := (float64(x)+0.5)/scale - 0.5
			target.SetNRGBA(offsetX+x, offsetY+y, sampleBuildIcon(source, bounds, sx, sy))
		}
	}
	return target
}

func sampleBuildIcon(source image.Image, bounds image.Rectangle, x, y float64) color.NRGBA {
	x = min(max(x+float64(bounds.Min.X), float64(bounds.Min.X)), float64(bounds.Max.X-1))
	y = min(max(y+float64(bounds.Min.Y), float64(bounds.Min.Y)), float64(bounds.Max.Y-1))
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	x1, y1 := min(x0+1, bounds.Max.X-1), min(y0+1, bounds.Max.Y-1)
	fx, fy := x-float64(x0), y-float64(y0)
	colors := [4]color.NRGBA{
		color.NRGBAModel.Convert(source.At(x0, y0)).(color.NRGBA),
		color.NRGBAModel.Convert(source.At(x1, y0)).(color.NRGBA),
		color.NRGBAModel.Convert(source.At(x0, y1)).(color.NRGBA),
		color.NRGBAModel.Convert(source.At(x1, y1)).(color.NRGBA),
	}
	interpolate := func(component func(color.NRGBA) uint8) uint8 {
		top := float64(component(colors[0]))*(1-fx) + float64(component(colors[1]))*fx
		bottom := float64(component(colors[2]))*(1-fx) + float64(component(colors[3]))*fx
		return uint8(math.Round(top*(1-fy) + bottom*fy))
	}
	return color.NRGBA{
		R: interpolate(func(value color.NRGBA) uint8 { return value.R }),
		G: interpolate(func(value color.NRGBA) uint8 { return value.G }),
		B: interpolate(func(value color.NRGBA) uint8 { return value.B }),
		A: interpolate(func(value color.NRGBA) uint8 { return value.A }),
	}
}

func (icon buildIcon) writeICO(name string) error {
	sizes := [...]int{16, 32, 48, 64, 128, 256}
	images := make([][]byte, len(sizes))
	for index, size := range sizes {
		data, err := icon.png(size)
		if err != nil {
			return err
		}
		images[index] = data
	}
	var data bytes.Buffer
	_ = binary.Write(&data, binary.LittleEndian, uint16(0))
	_ = binary.Write(&data, binary.LittleEndian, uint16(1))
	_ = binary.Write(&data, binary.LittleEndian, uint16(len(images)))
	offset := 6 + len(images)*16
	for index, imageData := range images {
		size := byte(sizes[index])
		data.WriteByte(size)
		data.WriteByte(size)
		data.WriteByte(0)
		data.WriteByte(0)
		_ = binary.Write(&data, binary.LittleEndian, uint16(1))
		_ = binary.Write(&data, binary.LittleEndian, uint16(32))
		_ = binary.Write(&data, binary.LittleEndian, uint32(len(imageData)))
		_ = binary.Write(&data, binary.LittleEndian, uint32(offset))
		offset += len(imageData)
	}
	for _, imageData := range images {
		data.Write(imageData)
	}
	return writeBuildIconFile(name, data.Bytes())
}

func (icon buildIcon) writeICNS(name string) error {
	entries := [...]struct {
		kind string
		size int
	}{
		{kind: "icp4", size: 16},
		{kind: "icp5", size: 32},
		{kind: "icp6", size: 64},
		{kind: "ic07", size: 128},
		{kind: "ic08", size: 256},
		{kind: "ic09", size: 512},
		{kind: "ic10", size: 1024},
	}
	backgroundSource := icon.image
	if icon.vector != nil {
		var err error
		backgroundSource, err = icon.rasterize(128)
		if err != nil {
			return err
		}
	}
	background := macOSIconBackground(backgroundSource)
	var body bytes.Buffer
	for _, entry := range entries {
		imageData, err := icon.macOSPNG(entry.size, background)
		if err != nil {
			return err
		}
		body.WriteString(entry.kind)
		_ = binary.Write(&body, binary.BigEndian, uint32(len(imageData)+8))
		body.Write(imageData)
	}
	var data bytes.Buffer
	data.WriteString("icns")
	_ = binary.Write(&data, binary.BigEndian, uint32(body.Len()+8))
	data.Write(body.Bytes())
	return writeBuildIconFile(name, data.Bytes())
}

func writeBuildIconFile(name string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return fmt.Errorf("create build icon directory: %w", err)
	}
	if err := os.WriteFile(name, data, 0o644); err != nil {
		return fmt.Errorf("write build icon %s: %w", name, err)
	}
	return nil
}

func writeLinuxIconAssets(bundleDir, appName string, icon buildIcon) error {
	if icon.vector != nil {
		if err := writeBuildIconFile(filepath.Join(bundleDir, appName+".svg"), icon.svg); err != nil {
			return err
		}
	} else {
		if err := icon.writePNG(filepath.Join(bundleDir, appName+".png"), 256); err != nil {
			return err
		}
	}
	desktop := fmt.Sprintf("[Desktop Entry]\nType=Application\nName=%s\nExec=./%s\nIcon=%s\nTerminal=false\nCategories=Game;\n", appName, appName, appName)
	return writeBuildIconFile(filepath.Join(bundleDir, appName+".desktop"), []byte(desktop))
}

func writeWindowsIconAssets(root string, layout projectBuildLayout, bundleDir, appName string, icon buildIcon, env []string) (string, error) {
	ico := filepath.Join(bundleDir, appName+".ico")
	if err := icon.writeICO(ico); err != nil {
		return "", err
	}
	rc := filepath.Join(layout.AppDir, "zz_generated_onix_icon.rc")
	syso := filepath.Join(layout.AppDir, "zz_generated_onix_icon_windows.syso")
	if err := writeBuildIconFile(rc, []byte("1 ICON "+strconv.Quote(filepath.ToSlash(ico))+"\n")); err != nil {
		return "", err
	}
	windres, err := windowsResourceCompiler(env)
	if err != nil {
		return "", err
	}
	if err := runCommand(root, env, windres, "--input-format=rc", "--output-format=coff", "-i", rc, "-o", syso); err != nil {
		return "", fmt.Errorf("compile Windows application icon: %w", err)
	}
	return syso, nil
}

func windowsResourceCompiler(env []string) (string, error) {
	compiler := ""
	for _, value := range env {
		if strings.HasPrefix(value, "CC=") {
			compiler = strings.TrimSpace(strings.TrimPrefix(value, "CC="))
			break
		}
	}
	if compiler == "" {
		return "", fmt.Errorf("Windows C compiler is missing from the build environment")
	}
	compiler = strings.Fields(compiler)[0]
	base := filepath.Base(compiler)
	extension := filepath.Ext(base)
	name := strings.TrimSuffix(strings.TrimSuffix(base, extension), "gcc") + "windres" + extension
	path := filepath.Join(filepath.Dir(compiler), name)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("Windows resource compiler %q not found; install it with the MinGW-w64 toolchain", path)
}

func writeAndroidIconAssets(hostDir string, icon buildIcon) error {
	variants := [...]struct {
		density string
		size    int
	}{
		{density: "mdpi", size: 48},
		{density: "hdpi", size: 72},
		{density: "xhdpi", size: 96},
		{density: "xxhdpi", size: 144},
		{density: "xxxhdpi", size: 192},
	}
	for _, variant := range variants {
		if err := icon.writePNG(filepath.Join(hostDir, "app", "src", "main", "res", "mipmap-"+variant.density, "ic_launcher.png"), variant.size); err != nil {
			return err
		}
	}
	return nil
}

func writeIOSIconAssets(hostDir string, icon buildIcon) error {
	assetsDir := filepath.Join(hostDir, "Assets.xcassets")
	dir := filepath.Join(assetsDir, "AppIcon.appiconset")
	type iosIcon struct {
		Filename string `json:"filename"`
		Idiom    string `json:"idiom"`
		Scale    string `json:"scale"`
		Size     string `json:"size"`
		Pixels   int    `json:"-"`
	}
	images := []iosIcon{
		{Filename: "icon-40.png", Idiom: "iphone", Scale: "2x", Size: "20x20", Pixels: 40},
		{Filename: "icon-60.png", Idiom: "iphone", Scale: "3x", Size: "20x20", Pixels: 60},
		{Filename: "icon-58.png", Idiom: "iphone", Scale: "2x", Size: "29x29", Pixels: 58},
		{Filename: "icon-87.png", Idiom: "iphone", Scale: "3x", Size: "29x29", Pixels: 87},
		{Filename: "icon-80.png", Idiom: "iphone", Scale: "2x", Size: "40x40", Pixels: 80},
		{Filename: "icon-120.png", Idiom: "iphone", Scale: "3x", Size: "40x40", Pixels: 120},
		{Filename: "icon-120-app.png", Idiom: "iphone", Scale: "2x", Size: "60x60", Pixels: 120},
		{Filename: "icon-180.png", Idiom: "iphone", Scale: "3x", Size: "60x60", Pixels: 180},
		{Filename: "icon-1024.png", Idiom: "ios-marketing", Scale: "1x", Size: "1024x1024", Pixels: 1024},
	}
	for _, image := range images {
		if err := icon.writeOpaquePNG(filepath.Join(dir, image.Filename), image.Pixels, color.NRGBA{R: 255, G: 255, B: 255, A: 255}); err != nil {
			return err
		}
	}
	contents := struct {
		Images []iosIcon `json:"images"`
		Info   struct {
			Author  string `json:"author"`
			Version int    `json:"version"`
		} `json:"info"`
	}{Images: images}
	contents.Info.Author, contents.Info.Version = "onix", 1
	data, err := json.MarshalIndent(contents, "", "  ")
	if err != nil {
		return err
	}
	if err := writeBuildIconFile(filepath.Join(dir, "Contents.json"), append(data, '\n')); err != nil {
		return err
	}
	return writeBuildIconFile(filepath.Join(assetsDir, "Contents.json"), []byte("{\n  \"info\": {\n    \"author\": \"onix\",\n    \"version\": 1\n  }\n}\n"))
}

func writeWebIconAssets(baseDir string, icon buildIcon) error {
	if icon.vector != nil {
		if err := writeBuildIconFile(filepath.Join(baseDir, "icon.svg"), icon.svg); err != nil {
			return err
		}
		return icon.writePNG(filepath.Join(baseDir, "apple-touch-icon.png"), 192)
	}
	for _, size := range [...]int{192, 512} {
		if err := icon.writePNG(filepath.Join(baseDir, "icon-"+strconv.Itoa(size)+".png"), size); err != nil {
			return err
		}
	}
	return icon.writePNG(filepath.Join(baseDir, "favicon.png"), 32)
}

func writeWeChatIconAssets(baseDir string, icon buildIcon) error {
	return icon.writePNG(filepath.Join(baseDir, "icon.png"), 512)
}
