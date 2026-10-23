package editor

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"go/parser"
	"go/token"
	"image"
	"image/color"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	onixpng "github.com/diiyw/onix/images/png"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/resource"
)

func TestResourcePackageConfigRejectsRemovedFields(t *testing.T) {
	for _, field := range []string{"strict", "fontSubsets"} {
		t.Run(field, func(t *testing.T) {
			projectDir := filepath.Join(t.TempDir(), "game")
			writeBuildTestFile(t, filepath.Join(projectDir, "resources", "packages.json"), fmt.Sprintf(`{%q:[]}`, field))

			if _, err := loadResourcePackageConfig(projectDir); err == nil || !strings.Contains(err.Error(), fmt.Sprintf(`unknown field %q`, field)) {
				t.Fatalf("removed %s field error = %v", field, err)
			}
		})
	}
}

func TestGLBJSONExtractsExternalReferences(t *testing.T) {
	jsonData := []byte(`{"buffers":[{"uri":"mesh.bin"}]}`)
	for len(jsonData)%4 != 0 {
		jsonData = append(jsonData, ' ')
	}
	data := make([]byte, 20+len(jsonData))
	copy(data[:4], "glTF")
	binary.LittleEndian.PutUint32(data[4:8], 2)
	binary.LittleEndian.PutUint32(data[8:12], uint32(len(data)))
	binary.LittleEndian.PutUint32(data[12:16], uint32(len(jsonData)))
	binary.LittleEndian.PutUint32(data[16:20], 0x4e4f534a)
	copy(data[20:], jsonData)
	got, ok := glbJSON(data)
	if !ok || !bytes.Contains(got, []byte(`"mesh.bin"`)) {
		t.Fatalf("GLB JSON chunk = %q, ok=%v", got, ok)
	}
}

func TestResourceReferenceVisitors(t *testing.T) {
	var jsonReferences []string
	if err := visitJSONStrings([]byte(`{"uri":"mesh.bin"}`), func(value string) error {
		jsonReferences = append(jsonReferences, value)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"uri", "mesh.bin"}; !slices.Equal(jsonReferences, want) {
		t.Fatalf("JSON references = %v, want %v", jsonReferences, want)
	}
	jsonReferences = jsonReferences[:0]
	if err := visitJSONStrings([]byte(`{"uri":"partial.bin"`), func(value string) error {
		jsonReferences = append(jsonReferences, value)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"uri", "partial.bin"}; !slices.Equal(jsonReferences, want) {
		t.Fatalf("partial JSON references = %v, want %v", jsonReferences, want)
	}

	var textReferences []string
	data := []byte("#include \"common.glsl\"\nmtllib material.mtl\nmap_Kd texture.png\n")
	if err := visitTextReferences(data, func(value string) error {
		textReferences = append(textReferences, value)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"common.glsl", "material.mtl", "texture.png"}; !slices.Equal(textReferences, want) {
		t.Fatalf("text references = %v, want %v", textReferences, want)
	}
}

func TestAndroidShaderTargetIncludesVulkanAndGLES(t *testing.T) {
	if got := shaderTargetForBuildPlatform(BuildPlatformAndroid); got != project.ShaderTargetAndroid {
		t.Fatalf("Android shader target = %v, want Android", got)
	}
}

func TestPreprocessPNGAssetsCompactsOnlyLosslessReleaseImages(t *testing.T) {
	imageData := image.NewNRGBA(image.Rect(0, 0, 96, 96))
	for y := range imageData.Bounds().Dy() {
		for x := range imageData.Bounds().Dx() {
			imageData.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 64, A: 255})
		}
	}
	var source bytes.Buffer
	encoder := onixpng.Encoder{CompressionLevel: onixpng.NoCompression}
	if err := encoder.Encode(&source, imageData); err != nil {
		t.Fatal(err)
	}
	original := source.Bytes()
	groups := map[string]map[string]resourceInput{
		resource.BasePackage: {
			"resources/image.png":     memoryResourceInput(original),
			"resources/not-a-png.png": memoryResourceInput([]byte("source text")),
		},
	}
	if err := preprocessPackageAssets(resourcePreprocessContext{TempDir: t.TempDir(), Options: resourcePackageOptions{Mode: BuildModeRelease}}, groups); err != nil {
		t.Fatal(err)
	}
	compacted, err := groups[resource.BasePackage]["resources/image.png"].bytes()
	if err != nil {
		t.Fatal(err)
	}
	if len(compacted) >= len(original) {
		t.Fatalf("PNG was not compacted: original=%d compacted=%d", len(original), len(compacted))
	}
	decoded, err := onixpng.Decode(bytes.NewReader(compacted))
	if err != nil {
		t.Fatal(err)
	}
	if !sameImagePixels(imageData, decoded) {
		t.Fatal("PNG compaction changed pixels")
	}
	invalid, err := groups[resource.BasePackage]["resources/not-a-png.png"].bytes()
	if err != nil {
		t.Fatal(err)
	}
	if got := string(invalid); got != "source text" {
		t.Fatalf("invalid PNG was changed to %q", got)
	}

	debugGroups := map[string]map[string]resourceInput{resource.BasePackage: {"resources/image.png": memoryResourceInput(original)}}
	if err := preprocessPackageAssets(resourcePreprocessContext{Options: resourcePackageOptions{Mode: BuildModeDebug}}, debugGroups); err != nil {
		t.Fatal(err)
	}
	debugImage, err := debugGroups[resource.BasePackage]["resources/image.png"].bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(debugImage, original) {
		t.Fatal("debug preprocessing changed PNG bytes")
	}
	animated := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0, 'a', 'c', 'T', 'L', 0, 0, 0, 0}
	if !hasPNGAnimation(animated) {
		t.Fatal("animated PNG marker was not detected")
	}
	if compacted, changed, err := compactPNG(animated); err != nil || changed || compacted != nil {
		t.Fatalf("animated PNG was rewritten: changed=%v data=%d err=%v", changed, len(compacted), err)
	}
}

func TestSameImagePixelsPreservesSixteenBitPrecision(t *testing.T) {
	bounds := image.Rect(0, 0, 1, 1)
	left := image.NewNRGBA64(bounds)
	right := image.NewNRGBA64(bounds)
	left.SetNRGBA64(0, 0, color.NRGBA64{R: 1, A: 0xffff})
	right.SetNRGBA64(0, 0, color.NRGBA64{R: 2, A: 0xffff})
	if sameImagePixels(left, right) {
		t.Fatal("16-bit pixel difference was ignored")
	}
	right.SetNRGBA64(0, 0, color.NRGBA64{R: 1, A: 0xffff})
	if !sameImagePixels(left, right) {
		t.Fatal("identical 16-bit pixels were reported as different")
	}
	allocations := testing.AllocsPerRun(100, func() {
		if !sameImagePixels(left, right) {
			t.Fatal("identical pixels were reported as different")
		}
	})
	if allocations != 0 {
		t.Fatalf("same-type pixel comparison allocated %.0f times", allocations)
	}
}

func TestCompressibleTextureSourceIncludesRuntimeRasterFormats(t *testing.T) {
	for _, name := range []string{"image.gif", "image.jpg", "image.jpeg", "image.png", "image.svg", "image.webp"} {
		if !compressibleTextureSource(name) {
			t.Fatalf("%s is not compressible", name)
		}
	}
}

func textureSourcePNG(t *testing.T, width, height int) []byte {
	t.Helper()
	pixels := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			pixels.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 5), G: uint8(y * 7), B: uint8(x ^ y), A: 255})
		}
	}

	var encoded bytes.Buffer
	if err := onixpng.Encode(&encoded, pixels); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func TestCompactHTMLRemovesSourceOverheadWithoutChangingRawText(t *testing.T) {
	source := []byte("<!doctype html>\n<!-- remove -->\n<div   title=\" a  b \" >\n  <span>A</span>\n  <span>B</span>\n</div>\n<style>\n  a > b { content: \"<x>\"; }\n</style>\n<textarea>  a\n  b </textarea>\n")
	compact := compactHTML(source)
	if len(compact) >= len(source) {
		t.Fatalf("HTML was not compacted: %d >= %d", len(compact), len(source))
	}
	if strings.Contains(string(compact), "remove") {
		t.Fatalf("HTML comment was retained: %q", compact)
	}
	for _, want := range []string{`title=" a  b "`, "a > b { content: \"<x>\"; }", "<textarea>  a\n  b </textarea>"} {
		if !strings.Contains(string(compact), want) {
			t.Fatalf("raw HTML content %q was changed or lost: %q", want, compact)
		}
	}
}

func TestGeneratedResourceSourcesParseForEveryPlatform(t *testing.T) {
	catalog := resource.Catalog{Version: resource.CatalogVersion, Packages: []resource.PackageInfo{
		{Name: resource.BasePackage, File: "base.dat", SHA256: [32]byte{1}, Size: 32, Required: true},
		{Name: "chapter", File: "chapter.dat", SHA256: [32]byte{2}, Size: 32, Dependencies: []string{resource.BasePackage}},
	}}
	sources := generatedResourceSources("main", "example.com/onix/resource", "game", "resources", catalog)
	want := []string{"zz_generated_onix_resources_js.go", "zz_generated_onix_resources_mobile.go", "zz_generated_onix_resources_native.go"}
	got := make([]string, 0, len(sources))
	for name, source := range sources {
		got = append(got, name)
		if _, err := parser.ParseFile(token.NewFileSet(), name, source, parser.AllErrors); err != nil {
			t.Errorf("parse %s: %v\n%s", name, err, source)
		}
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("generated source files = %v, want %v", got, want)
	}
	mobile := sources["zz_generated_onix_resources_mobile.go"]
	for _, name := range []string{"catalog.bin", "base.dat", "chapter.dat"} {
		if !strings.Contains(mobile, name) {
			t.Errorf("mobile generated source does not embed %s", name)
		}
	}
	for name, source := range sources {
		if !strings.Contains(source, "onixresource.SetDefaultProject") {
			t.Errorf("%s does not mount project-relative package paths", name)
		}
	}
	jsSource := sources["zz_generated_onix_resources_js.go"]
	for _, name := range []string{"__onixCatalogData", "__onixPackageData"} {
		if !strings.Contains(jsSource, `js.Global().Delete("`+name+`")`) {
			t.Errorf("generated Web source does not release %s", name)
		}
	}
	if !strings.Contains(jsSource, "values.Delete(name)") {
		t.Error("generated Web source does not release each copied package")
	}
	macSources := generatedResourceSources("main", "example.com/onix/resource", "game", "../Resources", catalog)
	if native := macSources["zz_generated_onix_resources_native.go"]; !strings.Contains(native, `"../Resources"`) {
		t.Fatalf("macOS generated source does not use app bundle resources:\n%s", native)
	}
}

func TestProjectGoPackageNameDoesNotUseNumericDirectoryName(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "2048")
	writeBuildTestFile(t, filepath.Join(dir, "game.go"), "package main\n")
	writeBuildTestFile(t, filepath.Join(dir, "game_test.go"), "package game_test\n")
	name, err := projectGoPackageName(dir)
	if err != nil {
		t.Fatal(err)
	}
	if name != "main" {
		t.Fatalf("project package name = %q, want main", name)
	}
}
