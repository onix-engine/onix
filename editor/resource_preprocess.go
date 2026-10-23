package editor

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/diiyw/onix/graphics"
	onixpng "github.com/diiyw/onix/images/png"
	"github.com/diiyw/onix/project"
)

type resourcePackageOptions struct {
	Mode           BuildMode
	Platform       BuildPlatform
	ShaderCompiler project.ShaderCompiler
	Progress       func(BuildProgress)
}

type resourcePreprocessContext struct {
	Root       string
	ProjectDir string
	TempDir    string
	Options    resourcePackageOptions
	Config     resourcePackageConfig
}

func shaderTargetForBuildPlatform(platform BuildPlatform) project.ShaderTarget {
	switch platform {
	case BuildPlatformWindows, BuildPlatformLinux:
		return project.ShaderTargetVulkan
	case BuildPlatformAndroid:
		return project.ShaderTargetAndroid
	case BuildPlatformMacOS, BuildPlatformIOS:
		return project.ShaderTargetMetal
	case BuildPlatformWeChat:
		return project.ShaderTargetWebGL2
	case BuildPlatformWeb:
		return project.ShaderTargetWebGPU
	default:
		return project.ShaderTargetAll
	}
}

// store keeps transformed blobs file-backed so a build retains metadata instead
// of every optimized image, compiled model and manifest until packaging ends.
func (context resourcePreprocessContext) store(name string, data []byte) (resourceInput, error) {
	if context.TempDir == "" || !filepath.IsLocal(filepath.FromSlash(name)) {
		return resourceInput{}, fmt.Errorf("invalid resource staging path %q in %q", name, context.TempDir)
	}
	source := filepath.Join(context.TempDir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		return resourceInput{}, fmt.Errorf("stage resource %s: %w", name, err)
	}
	if err := os.WriteFile(source, data, 0o600); err != nil {
		return resourceInput{}, fmt.Errorf("stage resource %s: %w", name, err)
	}
	return resourceInput{source: source}, nil
}

func textureCompressionExcluded(excludes []string, resourceName string) bool {
	resourceName = path.Clean(filepath.ToSlash(resourceName))
	for _, exclude := range excludes {
		exclude = path.Clean(filepath.ToSlash(strings.TrimSpace(exclude)))
		if exclude == resourceName || strings.TrimPrefix(exclude, "resources/") == strings.TrimPrefix(resourceName, "resources/") {
			return true
		}
	}
	return false
}

type packageAssetKind uint8

const (
	packageAssetOther packageAssetKind = iota
	packageAssetAtlas
	packageAssetPNG
	packageAssetHTML
)

func packageAssetKindFor(extension string) packageAssetKind {
	switch {
	case strings.EqualFold(extension, ".atlas"):
		return packageAssetAtlas
	case strings.EqualFold(extension, ".png"):
		return packageAssetPNG
	case strings.EqualFold(extension, ".html"):
		return packageAssetHTML
	}
	return packageAssetOther
}

// preprocessPackageAssets rewrites atlas metadata, release PNGs and HTML in one
// pass; disjoint extensions mean no kind ever observes another's output.
func preprocessPackageAssets(context resourcePreprocessContext, groups map[string]map[string]resourceInput) error {
	release := context.Options.Mode == BuildModeRelease
	for _, group := range groups {
		for name, input := range group {
			kind := packageAssetKindFor(path.Ext(name))
			if kind == packageAssetOther || (kind == packageAssetPNG && !release) {
				continue
			}
			if context.Options.Progress != nil {
				reportBuildActivity(context.Options.Progress, "Processing resource "+name, .55)
			}
			data, err := input.bytes()
			if err != nil {
				return fmt.Errorf("read package resource %s: %w", name, err)
			}
			compacted, store, err := compactPackageAsset(kind, data)
			if err != nil {
				return fmt.Errorf("preprocess package resource %s: %w", name, err)
			}
			if !store {
				continue
			}
			group[name], err = context.store(name, compacted)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// compactPackageAsset rewrites one asset kind, reporting whether the result
// replaces the source. PNG normalization is release-only.
func compactPackageAsset(kind packageAssetKind, data []byte) ([]byte, bool, error) {
	switch kind {
	case packageAssetAtlas:
		compiled, atlas, err := graphics.PreprocessAtlas(data)
		if err != nil {
			return nil, false, err
		}
		if trimmed := bytes.TrimSpace(data); !atlas && len(trimmed) > 0 && trimmed[0] == '{' {
			return nil, false, errors.New("source is not TexturePacker JSON")
		}
		return compiled, atlas, nil
	case packageAssetPNG:
		if len(data) == 0 {
			return nil, false, nil
		}
		// The decoder/encoder pair drops ancillary chunks, so an invalid or
		// unsupported PNG is published untouched rather than failing the build.
		compacted, changed, err := compactPNG(data)
		if err != nil {
			return nil, false, nil
		}
		return compacted, changed, nil
	case packageAssetHTML:
		// compactHTML keeps quoted attributes and style/script/pre/textarea raw
		// text, dropping only comments and redundant whitespace.
		compacted := compactHTML(data)
		return compacted, len(compacted) < len(data), nil
	}
	return nil, false, nil
}

func compactHTML(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	output := make([]byte, 0, len(data))
	rawTag := ""
	for offset := 0; offset < len(data); {
		if rawTag != "" {
			end := indexHTMLClosingTag(data, offset, rawTag)
			if end < 0 {
				output = append(output, data[offset:]...)
				break
			}
			output = append(output, data[offset:end]...)
			offset = end
			rawTag = ""
		}
		if offset >= len(data) {
			break
		}
		if data[offset] != '<' {
			end := offset
			for end < len(data) && data[end] != '<' {
				end++
			}
			appendHTMLText(&output, data[offset:end])
			offset = end
			continue
		}
		if bytes.HasPrefix(data[offset:], []byte("<!--")) {
			end := bytes.Index(data[offset+4:], []byte("-->"))
			if end < 0 {
				output = append(output, data[offset:]...)
				break
			}
			offset += end + 7
			continue
		}
		end := htmlTagEnd(data, offset+1)
		if end < 0 {
			output = append(output, data[offset:]...)
			break
		}
		tag := data[offset : end+1]
		output = appendHTMLTag(output, tag)
		if name, closing := htmlTagName(tag); !closing && (name == "style" || name == "script" || name == "pre" || name == "textarea") {
			rawTag = name
		}
		offset = end + 1
	}
	return bytes.TrimRight(output, " \t\r\n\f")
}

func appendHTMLText(output *[]byte, text []byte) {
	space := false
	for _, value := range text {
		if isHTMLSpace(value) {
			space = true
			continue
		}
		if space && len(*output) > 0 {
			*output = append(*output, ' ')
		}
		*output = append(*output, value)
		space = false
	}
	if space && len(*output) > 0 {
		// Keep a single separator between inline elements; the document trim removes it at EOF.
		*output = append(*output, ' ')
	}
}

func appendHTMLTag(output, tag []byte) []byte {
	quote := byte(0)
	space := false
	for index, value := range tag {
		if quote != 0 {
			output = append(output, value)
			if value == quote {
				quote = 0
			}
			continue
		}
		if value == '\'' || value == '"' {
			quote = value
			output = append(output, value)
			space = false
			continue
		}
		if isHTMLSpace(value) {
			space = true
			continue
		}
		if space && index > 0 && value != '>' && value != '/' {
			output = append(output, ' ')
		}
		output = append(output, value)
		space = false
	}
	return output
}

func htmlTagEnd(data []byte, offset int) int {
	quote := byte(0)
	for ; offset < len(data); offset++ {
		value := data[offset]
		if quote != 0 {
			if value == quote {
				quote = 0
			}
			continue
		}
		if value == '\'' || value == '"' {
			quote = value
		} else if value == '>' {
			return offset
		}
	}
	return -1
}

func htmlTagName(tag []byte) (string, bool) {
	start := 1
	closing := start < len(tag) && tag[start] == '/'
	if closing {
		start++
	}
	end := start
	for end < len(tag) && ((tag[end] >= 'a' && tag[end] <= 'z') || (tag[end] >= 'A' && tag[end] <= 'Z')) {
		end++
	}
	return strings.ToLower(string(tag[start:end])), closing
}

func indexHTMLClosingTag(data []byte, offset int, name string) int {
	for index := offset; index+2+len(name) <= len(data); index++ {
		if data[index] != '<' || data[index+1] != '/' {
			continue
		}
		matches := true
		for nameIndex := range len(name) {
			left, right := data[index+2+nameIndex], name[nameIndex]
			if left >= 'A' && left <= 'Z' {
				left += 'a' - 'A'
			}
			if left != right {
				matches = false
				break
			}
		}
		end := index + 2 + len(name)
		if matches && end < len(data) && (isHTMLSpace(data[end]) || data[end] == '>') {
			return index
		}
	}
	return -1
}

func isHTMLSpace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\n' || value == '\r' || value == '\f'
}

func compactPNG(data []byte) ([]byte, bool, error) {
	if hasPNGAnimation(data) {
		return nil, false, nil
	}
	original, err := onixpng.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, false, err
	}
	var encoded bytes.Buffer
	encoder := onixpng.Encoder{CompressionLevel: onixpng.BestCompression}
	if err := encoder.Encode(&encoded, original); err != nil {
		return nil, false, err
	}
	if encoded.Len() >= len(data) {
		return nil, false, nil
	}
	decoded, err := onixpng.Decode(bytes.NewReader(encoded.Bytes()))
	if err != nil || !sameImagePixels(original, decoded) {
		if err == nil {
			err = fmt.Errorf("re-encoded PNG pixels differ")
		}
		return nil, false, err
	}
	return encoded.Bytes(), true, nil
}

func hasPNGAnimation(data []byte) bool {
	const signatureSize = 8
	if len(data) < signatureSize || !bytes.Equal(data[:signatureSize], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		return false
	}
	for offset := signatureSize; offset+12 <= len(data); {
		length := uint64(binary.BigEndian.Uint32(data[offset : offset+4]))
		if length > uint64(len(data)-offset-12) {
			return false
		}
		kind := data[offset+4 : offset+8]
		if bytes.Equal(kind, []byte("acTL")) || bytes.Equal(kind, []byte("fcTL")) || bytes.Equal(kind, []byte("fdAT")) {
			return true
		}
		offset += 12 + int(length)
	}
	return false
}

type pixelFormat uint8

const (
	pixelAlpha pixelFormat = iota
	pixelAlpha16
	pixelGray
	pixelGray16
	pixelNRGBA
	pixelNRGBA64
	pixelPaletted
	pixelRGBA
	pixelRGBA64
)

type pixelView struct {
	format   pixelFormat
	pixels   []byte
	palette  color.Palette
	stride   int
	rowBytes int
}

func imagePixelView(source image.Image) (pixelView, bool) {
	width := source.Bounds().Dx()
	switch source := source.(type) {
	case *image.Alpha:
		return pixelView{format: pixelAlpha, pixels: source.Pix, stride: source.Stride, rowBytes: width}, true
	case *image.Alpha16:
		return pixelView{format: pixelAlpha16, pixels: source.Pix, stride: source.Stride, rowBytes: width * 2}, true
	case *image.Gray:
		return pixelView{format: pixelGray, pixels: source.Pix, stride: source.Stride, rowBytes: width}, true
	case *image.Gray16:
		return pixelView{format: pixelGray16, pixels: source.Pix, stride: source.Stride, rowBytes: width * 2}, true
	case *image.NRGBA:
		return pixelView{format: pixelNRGBA, pixels: source.Pix, stride: source.Stride, rowBytes: width * 4}, true
	case *image.NRGBA64:
		return pixelView{format: pixelNRGBA64, pixels: source.Pix, stride: source.Stride, rowBytes: width * 8}, true
	case *image.Paletted:
		return pixelView{format: pixelPaletted, pixels: source.Pix, palette: source.Palette, stride: source.Stride, rowBytes: width}, true
	case *image.RGBA:
		return pixelView{format: pixelRGBA, pixels: source.Pix, stride: source.Stride, rowBytes: width * 4}, true
	case *image.RGBA64:
		return pixelView{format: pixelRGBA64, pixels: source.Pix, stride: source.Stride, rowBytes: width * 8}, true
	default:
		return pixelView{}, false
	}
}

func (v pixelView) equal(other pixelView, height int) bool {
	if v.format != other.format || v.rowBytes != other.rowBytes {
		return false
	}
	if v.format == pixelPaletted && !samePalette(v.palette, other.palette) {
		return false
	}
	return samePixelRows(v.pixels, v.stride, other.pixels, other.stride, v.rowBytes, height)
}

func sameImagePixels(left, right image.Image) bool {
	if left == nil || right == nil || left.Bounds() != right.Bounds() {
		return false
	}
	bounds := left.Bounds()
	leftView, leftOK := imagePixelView(left)
	rightView, rightOK := imagePixelView(right)
	if leftOK && rightOK && leftView.equal(rightView, bounds.Dy()) {
		return true
	}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			leftR, leftG, leftB, leftA := left.At(x, y).RGBA()
			rightR, rightG, rightB, rightA := right.At(x, y).RGBA()
			if leftR != rightR || leftG != rightG || leftB != rightB || leftA != rightA {
				return false
			}
		}
	}
	return true
}

func samePalette(left, right color.Palette) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		leftR, leftG, leftB, leftA := left[index].RGBA()
		rightR, rightG, rightB, rightA := right[index].RGBA()
		if leftR != rightR || leftG != rightG || leftB != rightB || leftA != rightA {
			return false
		}
	}
	return true
}

func samePixelRows[T comparable](left []T, leftStride int, right []T, rightStride, width, height int) bool {
	for row := range height {
		leftOffset := row * leftStride
		rightOffset := row * rightStride
		if !slices.Equal(left[leftOffset:leftOffset+width], right[rightOffset:rightOffset+width]) {
			return false
		}
	}
	return true
}

func updateGroupedResource(groups map[string]map[string]resourceInput, path string, input resourceInput, remove bool) bool {
	for _, group := range groups {
		if _, ok := group[path]; !ok {
			continue
		}
		if remove {
			delete(group, path)
		} else {
			group[path] = input
		}
		return true
	}
	return false
}

func addGeneratedResource(groups map[string]map[string]resourceInput, sourcePath, generatedPath string, input resourceInput) bool {
	for _, group := range groups {
		if _, ok := group[sourcePath]; !ok {
			continue
		}
		group[generatedPath] = input
		return true
	}
	return false
}
