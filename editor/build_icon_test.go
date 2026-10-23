package editor

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	onixsvg "github.com/diiyw/onix/images/svg"
)

func TestLoadBuildIconUsesDefaultLogoAndCustomImage(t *testing.T) {
	defaultIcon, err := loadBuildIcon(DefaultBuildOptions(t.TempDir()))
	if err != nil || defaultIcon.vector == nil {
		t.Fatalf("load default SVG build icon: vector=%v err=%v", defaultIcon.vector != nil, err)
	}
	defaultRaster, err := defaultIcon.rasterize(128)
	if err != nil || defaultRaster.Bounds().Dx() != 128 || defaultRaster.Bounds().Dy() != 128 {
		t.Fatalf("rasterize default SVG build icon: bounds=%v err=%v", defaultRaster.Bounds(), err)
	}

	dir := t.TempDir()
	custom := filepath.Join(dir, "custom.png")
	writeBuildIconTestPNG(t, custom, 80, 40, color.NRGBA{R: 10, G: 20, B: 30, A: 255})
	icon, err := loadBuildIcon(BuildOptions{ProjectDir: dir, Icon: "custom.png"})
	if err != nil {
		t.Fatal(err)
	}
	resized := resizeBuildIcon(icon.image, 100)
	if got := resized.NRGBAAt(50, 50); got.R != 10 || got.G != 20 || got.B != 30 || got.A != 255 {
		t.Fatalf("resized icon center = %#v", got)
	}
	if got := resized.NRGBAAt(50, 0); got.A != 0 {
		t.Fatalf("non-square icon does not have transparent padding: %#v", got)
	}
}

func TestLoadBuildIconRasterizesSVGAtTargetResolution(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "custom.svg")
	writeBuildTestFile(t, name, `<svg width="1" height="1" viewBox="0 0 100 100"><rect x="40" width="20" height="100" fill="red"/></svg>`)
	icon, err := loadBuildIcon(BuildOptions{ProjectDir: dir, Icon: "custom.svg"})
	if err != nil {
		t.Fatal(err)
	}
	if icon.vector == nil {
		t.Fatal("SVG build icon was decoded as a raster image")
	}
	raster, err := icon.rasterize(100)
	if err != nil {
		t.Fatal(err)
	}
	if raster.NRGBAAt(10, 50).A != 0 || raster.NRGBAAt(50, 50).A != 255 || raster.NRGBAAt(90, 50).A != 0 {
		t.Fatal("SVG details were not rendered at the requested target resolution")
	}
}

func TestLoadBuildIconUsesProjectWindowIcon(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "resources"), 0o755); err != nil {
		t.Fatal(err)
	}
	iconPath := filepath.Join(dir, "resources", "app.png")
	writeBuildIconTestPNG(t, iconPath, 32, 32, color.NRGBA{R: 40, G: 90, B: 140, A: 255})
	writeBuildTestFile(t, filepath.Join(dir, "resources", "project.json"), `{"window":{"icon":"app.png"}}`)

	icon, err := loadBuildIcon(BuildOptions{ProjectDir: dir, Icon: "ignored.png"})
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, a := icon.image.At(16, 16).RGBA()
	if r>>8 != 40 || g>>8 != 90 || b>>8 != 140 || a>>8 != 255 {
		t.Fatalf("project icon pixel = %#v", icon.image.At(16, 16))
	}
}

func TestBuildIconWritesICOAndICNSContainers(t *testing.T) {
	dir := t.TempDir()
	source := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for index := range source.Pix {
		source.Pix[index] = 255
	}
	icon := buildIcon{image: source}
	ico := filepath.Join(dir, "app.ico")
	if err := icon.writeICO(ico); err != nil {
		t.Fatal(err)
	}
	icoData, err := os.ReadFile(ico)
	if err != nil {
		t.Fatal(err)
	}
	if len(icoData) < 6 || binary.LittleEndian.Uint16(icoData[2:4]) != 1 || binary.LittleEndian.Uint16(icoData[4:6]) != 6 {
		t.Fatalf("invalid ICO header: %x", icoData[:min(len(icoData), 16)])
	}

	icns := filepath.Join(dir, "AppIcon.icns")
	if err := icon.writeICNS(icns); err != nil {
		t.Fatal(err)
	}
	icnsData, err := os.ReadFile(icns)
	if err != nil {
		t.Fatal(err)
	}
	if len(icnsData) < 8 || string(icnsData[:4]) != "icns" || int(binary.BigEndian.Uint32(icnsData[4:8])) != len(icnsData) {
		t.Fatalf("invalid ICNS header: %x", icnsData[:min(len(icnsData), 16)])
	}
}

func TestMacOSICNSEntriesAreOpaque(t *testing.T) {
	dir := t.TempDir()
	source := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := range 4 {
		for x := range 4 {
			source.SetNRGBA(x, y, color.NRGBA{R: 20, G: 80, B: 160, A: 255})
		}
	}
	source.SetNRGBA(0, 0, color.NRGBA{})
	if err := (buildIcon{image: source}).writeICNS(filepath.Join(dir, "AppIcon.icns")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "AppIcon.icns"))
	if err != nil {
		t.Fatal(err)
	}
	for offset := 8; offset < len(data); {
		if offset+8 > len(data) {
			t.Fatalf("truncated ICNS entry at offset %d", offset)
		}
		entrySize := int(binary.BigEndian.Uint32(data[offset+4 : offset+8]))
		if entrySize < 8 || offset+entrySize > len(data) {
			t.Fatalf("invalid ICNS entry size %d at offset %d", entrySize, offset)
		}
		decoded, err := png.Decode(bytes.NewReader(data[offset+8 : offset+entrySize]))
		if err != nil {
			t.Fatalf("decode ICNS entry %q: %v", data[offset:offset+4], err)
		}
		for _, point := range [][2]int{{0, 0}, {decoded.Bounds().Max.X - 1, 0}, {0, decoded.Bounds().Max.Y - 1}, {decoded.Bounds().Max.X - 1, decoded.Bounds().Max.Y - 1}} {
			r, g, b, alpha := decoded.At(point[0], point[1]).RGBA()
			if alpha != 0xffff {
				t.Fatalf("ICNS entry %q has transparent pixel at %v", data[offset:offset+4], point)
			}
			if r>>8 != 20 || g>>8 != 80 || b>>8 != 160 {
				t.Fatalf("ICNS entry %q background at %v = %d,%d,%d, want 20,80,160", data[offset:offset+4], point, r>>8, g>>8, b>>8)
			}
		}
		offset += entrySize
	}
}

func TestBuildIconWritesEveryPlatformAssetSet(t *testing.T) {
	dir := t.TempDir()
	source := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := range 4 {
		for x := range 4 {
			source.SetNRGBA(x, y, color.NRGBA{R: 20, G: 80, B: 160, A: 255})
		}
	}
	source.SetNRGBA(0, 0, color.NRGBA{})
	icon := buildIcon{image: source}
	if err := writeLinuxIconAssets(filepath.Join(dir, "linux"), "Demo", icon); err != nil {
		t.Fatal(err)
	}
	if err := writeAndroidIconAssets(filepath.Join(dir, "android"), icon); err != nil {
		t.Fatal(err)
	}
	if err := writeIOSIconAssets(filepath.Join(dir, "ios"), icon); err != nil {
		t.Fatal(err)
	}
	if err := writeWebIconAssets(filepath.Join(dir, "web"), icon); err != nil {
		t.Fatal(err)
	}
	if err := writeWeChatIconAssets(filepath.Join(dir, "wechat"), icon); err != nil {
		t.Fatal(err)
	}
	want := map[string]int{
		filepath.Join(dir, "linux", "Demo.png"):                                                         256,
		filepath.Join(dir, "android", "app", "src", "main", "res", "mipmap-xxxhdpi", "ic_launcher.png"): 192,
		filepath.Join(dir, "ios", "Assets.xcassets", "AppIcon.appiconset", "icon-1024.png"):             1024,
		filepath.Join(dir, "web", "icon-512.png"):                                                       512,
		filepath.Join(dir, "wechat", "icon.png"):                                                        512,
	}
	for name, size := range want {
		file, err := os.Open(name)
		if err != nil {
			t.Errorf("open platform icon %s: %v", name, err)
			continue
		}
		config, err := png.DecodeConfig(file)
		_ = file.Close()
		if err != nil || config.Width != size || config.Height != size {
			t.Errorf("platform icon %s = %dx%d, %v; want %dx%d", name, config.Width, config.Height, err, size, size)
		}
	}
	for _, name := range []string{
		filepath.Join(dir, "linux", "Demo.desktop"),
		filepath.Join(dir, "ios", "Assets.xcassets", "Contents.json"),
		filepath.Join(dir, "ios", "Assets.xcassets", "AppIcon.appiconset", "Contents.json"),
	} {
		if _, err := os.Stat(name); err != nil {
			t.Errorf("platform icon metadata is missing: %s: %v", name, err)
		}
	}
	iosData, err := os.ReadFile(filepath.Join(dir, "ios", "Assets.xcassets", "AppIcon.appiconset", "icon-1024.png"))
	if err != nil {
		t.Fatal(err)
	}
	if len(iosData) < 26 {
		t.Fatalf("iOS marketing icon PNG is truncated: %d bytes", len(iosData))
	}
	if iosData[25] != 2 {
		t.Fatalf("iOS marketing icon PNG color type = %d, want opaque truecolor (2)", iosData[25])
	}
}

func TestVectorBuildIconStaysSVGOnSupportedPlatforms(t *testing.T) {
	dir := t.TempDir()
	source := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><rect width="16" height="16" fill="red"/></svg>`)
	document, err := onixsvg.Decode(bytes.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	icon := buildIcon{vector: document, svg: source}
	linuxDir := filepath.Join(dir, "linux")
	if err := writeLinuxIconAssets(linuxDir, "Demo", icon); err != nil {
		t.Fatal(err)
	}
	webDir := filepath.Join(dir, "web")
	if err := writeWebIconAssets(webDir, icon); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{filepath.Join(linuxDir, "Demo.svg"), filepath.Join(webDir, "icon.svg")} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, source) {
			t.Errorf("vector icon %s was modified", name)
		}
	}
	file, err := os.Open(filepath.Join(webDir, "apple-touch-icon.png"))
	if err != nil {
		t.Fatal(err)
	}
	config, err := png.DecodeConfig(file)
	_ = file.Close()
	if err != nil || config.Width != 192 || config.Height != 192 {
		t.Fatalf("Apple touch icon = %dx%d, %v; want 192x192", config.Width, config.Height, err)
	}
	for _, name := range []string{filepath.Join(linuxDir, "Demo.png"), filepath.Join(webDir, "favicon.png"), filepath.Join(webDir, "icon-512.png")} {
		if _, err := os.Stat(name); !os.IsNotExist(err) {
			t.Errorf("SVG-capable platform generated unnecessary raster icon %s: %v", name, err)
		}
	}
}

func writeBuildIconTestPNG(t *testing.T, name string, width, height int, fill color.NRGBA) {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			im.SetNRGBA(x, y, fill)
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, im); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, data.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}
