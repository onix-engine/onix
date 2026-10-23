package spine

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/diiyw/onix/graphics"
	graphicspng "github.com/diiyw/onix/graphics/codec/png"
	"github.com/diiyw/onix/internal/typedbin"
	"github.com/go-gl/mathgl/mgl32"
)

var bundledSpineSources = []struct {
	skeleton string
	atlas    string
}{
	{"coin-pro.json", "coin.atlas"},
	{"goblins-pro.json", "goblins.atlas"},
	{"raptor-pro.json", "raptor.atlas"},
	{"spineboy-pro.json", "spineboy.atlas"},
	{"stretchyman-pro.json", "stretchyman.atlas"},
	{"stretchyman-stretchy-ik-pro.json", "stretchyman.atlas"},
	{"tank-pro.json", "tank.atlas"},
	{"vine-pro.json", "vine.atlas"},
}

func TestPreprocessedBundledSpinesMatchSource(t *testing.T) {
	texture := graphics.CreateTextureByImageWithUsage(image.NewRGBA(image.Rect(0, 0, 1024, 1024)), false, graphics.ResourceUsageCPU)
	defer texture.Dispose()
	var sourceBytes, compiledBytes int
	for _, test := range bundledSpineSources {
		t.Run(test.skeleton, func(t *testing.T) {
			skeleton, atlas := readSpineSource(t, test.skeleton, test.atlas)
			compiled, err := Preprocess(skeleton, atlas)
			if err != nil {
				t.Fatal(err)
			}
			again, err := Preprocess(skeleton, atlas)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(compiled, again) {
				t.Fatal("equivalent preprocessing runs produced different data")
			}
			if !IsPreprocessed(compiled) {
				t.Fatal("compiled skeleton does not have the typed binary marker")
			}

			source, err := Load(skeleton, atlas, texture)
			if err != nil {
				t.Fatal(err)
			}
			restored, err := Load(compiled, nil, texture)
			if err != nil {
				t.Fatal(err)
			}
			if source.Width != restored.Width || source.Height != restored.Height {
				t.Fatalf("dimensions = %fx%f, want %fx%f", restored.Width, restored.Height, source.Width, source.Height)
			}
			if !slices.Equal(source.Animations(), restored.Animations()) {
				t.Fatalf("animations = %v, want %v", restored.Animations(), source.Animations())
			}
			if !slices.Equal(source.Skins(), restored.Skins()) {
				t.Fatalf("skins = %v, want %v", restored.Skins(), source.Skins())
			}
			for _, skin := range source.Skins() {
				for _, animation := range source.Animations() {
					assertCompiledPose(t, source, restored, skin, animation)
				}
			}
			sourceBytes += len(skeleton) + len(atlas)
			compiledBytes += len(compiled)
		})
	}
	if compiledBytes >= sourceBytes {
		t.Fatalf("compiled Spine data = %d bytes, source JSON and atlas = %d bytes", compiledBytes, sourceBytes)
	}
	t.Logf("bundled Spine data: source=%d bytes compiled=%d bytes", sourceBytes, compiledBytes)
}

func TestDecodePreprocessedSpineUsesCompiledMipmapFlag(t *testing.T) {
	skeleton := []byte(`{
		"skeleton":{"width":1,"height":1},
		"bones":[{"name":"root"}],
		"slots":[{"name":"slot","bone":"root","attachment":"region"}],
		"skins":{"default":{"slot":{"region":{"width":1,"height":1}}}}
	}`)
	atlas := []byte("page.png\nsize: 1,1\nformat: RGBA8888\nfilter: MipMapLinearLinear,Linear\nrepeat: none\nregion\nrotate: false\nxy: 0,0\nsize: 1,1\norig: 1,1\noffset: 0,0\nindex: -1\n")
	compiled, err := Preprocess(skeleton, atlas)
	if err != nil {
		t.Fatal(err)
	}
	var texture bytes.Buffer
	if err := png.Encode(&texture, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	resource, err := Decode(compiled, nil, texture.Bytes(), "page.png", graphics.TextureOptions{PremultiplyAlpha: true, Usage: graphics.ResourceUsageCPU}, graphics.ImageDecoderSet[graphics.NoImageDecoder, graphics.NoImageDecoder, graphicspng.Decoder, graphics.NoImageDecoder, graphics.NoImageDecoder]{})
	if err != nil {
		t.Fatal(err)
	}
	defer resource.Texture.Dispose()
	if !resource.Texture.Mipmapped() {
		t.Fatal("compiled atlas mipmap flag was not applied to the texture")
	}
	if !resource.Texture.PremultipliedAlpha() {
		t.Fatal("texture options were not preserved")
	}
}

func TestLoadRejectsCorruptPreprocessedSpine(t *testing.T) {
	skeleton, atlas := readSpineSource(t, "coin-pro.json", "coin.atlas")
	compiled, err := Preprocess(skeleton, atlas)
	if err != nil {
		t.Fatal(err)
	}
	texture := graphics.CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	defer texture.Dispose()
	if _, err := Load(compiled[:len(compiled)-1], nil, texture); err == nil || !strings.Contains(err.Error(), "decode compiled skeleton") {
		t.Fatalf("truncated compiled skeleton error = %v", err)
	}
	wrongSchema, err := typedbin.Marshal(struct{ Value string }{Value: "not Spine"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Load(wrongSchema, nil, texture); err == nil || !strings.Contains(err.Error(), "schema mismatch") {
		t.Fatalf("wrong compiled schema error = %v", err)
	}
}

func BenchmarkLoadPreprocessedSpine(b *testing.B) {
	skeleton, atlas := readSpineSource(b, "spineboy-pro.json", "spineboy.atlas")
	compiled, err := Preprocess(skeleton, atlas)
	if err != nil {
		b.Fatal(err)
	}
	texture := graphics.CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	defer texture.Dispose()
	b.ReportMetric(float64(len(skeleton)+len(atlas)), "source-bytes")
	b.ReportMetric(float64(len(compiled)), "compiled-bytes")
	b.Run("JSON+atlas", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			if _, err := Load(skeleton, atlas, texture); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("compiled", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			if _, err := Load(compiled, nil, texture); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func assertCompiledPose(t *testing.T, source, restored *Resource, skin, animation string) {
	t.Helper()
	want := NewPlayer(source)
	got := NewPlayer(restored)
	for _, player := range []*Player{want, got} {
		if err := player.SetSkin(skin); err != nil {
			t.Fatal(err)
		}
		if err := player.SetAnimation(animation, false); err != nil {
			t.Fatal(err)
		}
	}
	want.time = source.animations[animation].duration * .37
	got.time = want.time
	want.evaluate()
	got.evaluate()
	if !reflect.DeepEqual(got.pose, want.pose) {
		t.Fatalf("compiled pose differs for skin %q animation %q", skin, animation)
	}
	projection := mgl32.Ident4()
	if gotBatch, wantBatch := got.MeshBatch(projection, projection), want.MeshBatch(projection, projection); !reflect.DeepEqual(gotBatch, wantBatch) {
		t.Fatalf("compiled mesh differs for skin %q animation %q", skin, animation)
	}
}

func readSpineSource(t testing.TB, skeletonName, atlasName string) ([]byte, []byte) {
	t.Helper()
	base := bundledSpineExportsDir()
	skeleton, err := os.ReadFile(filepath.Join(base, skeletonName))
	if err != nil {
		t.Fatal(err)
	}
	atlas, err := os.ReadFile(filepath.Join(base, atlasName))
	if err != nil {
		t.Fatal(err)
	}
	return skeleton, atlas
}

func bundledSpineExportsDir() string {
	return filepath.Join("..", "projects", "spine", "gallery", "exports")
}
