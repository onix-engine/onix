package graphics

import (
	"errors"
	"strings"
	"testing"
)

var errTestPNGDecoder = errors.New("PNG decoder called")

type testPNGDecoder struct{}

func (testPNGDecoder) ImageExtension() string { return ".png" }

func (testPNGDecoder) DecodeImage([]byte, TextureOptions) (*Texture, error) {
	return nil, errTestPNGDecoder
}

func TestImageDecoderSetRequiresMatchingExtension(t *testing.T) {
	decoder := ImageDecoderSet[testPNGDecoder, NoImageDecoder, NoImageDecoder, NoImageDecoder, NoImageDecoder]{}

	if _, err := decoder.DecodeImage(nil, "texture.jpg", TextureOptions{}); err == nil || !strings.Contains(err.Error(), `unsupported image format ".jpg"`) {
		t.Fatalf("mismatched extension error = %v", err)
	}
	if _, err := decoder.DecodeImage(nil, "texture.PNG", TextureOptions{}); !errors.Is(err, errTestPNGDecoder) {
		t.Fatalf("matching extension error = %v", err)
	}
}

var errTestContainerDecoder = errors.New("container decoder called")

type testContainerDecoder struct{}

func (testContainerDecoder) ImageExtension() string { return ".ctr" }

func (testContainerDecoder) ImageContainerMagic() []byte { return []byte("CTR1") }

func (testContainerDecoder) DecodeImage([]byte, TextureOptions) (*Texture, error) {
	return nil, errTestContainerDecoder
}

// A build packs a container in place of the source image, so the payload keeps
// the source extension and only the leading bytes identify it.
func TestImageDecoderSetClaimsContainerMagicBeforeExtension(t *testing.T) {
	decoder := ImageDecoderSet[testPNGDecoder, testContainerDecoder, NoImageDecoder, NoImageDecoder, NoImageDecoder]{}

	if _, err := decoder.DecodeImage([]byte("CTR1payload"), "texture.png", TextureOptions{}); !errors.Is(err, errTestContainerDecoder) {
		t.Fatalf("container payload behind a source extension = %v", err)
	}
	if _, err := decoder.DecodeImage([]byte("CTR1payload"), "texture.ctr", TextureOptions{}); !errors.Is(err, errTestContainerDecoder) {
		t.Fatalf("container payload behind its own extension = %v", err)
	}
	if _, err := decoder.DecodeImage([]byte("not a container"), "texture.png", TextureOptions{}); !errors.Is(err, errTestPNGDecoder) {
		t.Fatalf("unclaimed payload fell through to the container decoder: %v", err)
	}
	// A short payload must not be compared past its end.
	if _, err := decoder.DecodeImage([]byte("CT"), "texture.png", TextureOptions{}); !errors.Is(err, errTestPNGDecoder) {
		t.Fatalf("truncated payload error = %v", err)
	}
}

func TestImageDecoderWithPreservesContainerAndFormatDispatch(t *testing.T) {
	type base = ImageDecoderSet[testPNGDecoder, NoImageDecoder, NoImageDecoder, NoImageDecoder, NoImageDecoder]
	decoder := ImageDecoderWith[base, testContainerDecoder]{}
	for _, test := range []struct {
		data, name string
		want       error
	}{{"CTR1payload", "texture.png", errTestContainerDecoder}, {"payload", "texture.CTR", errTestContainerDecoder}, {"CT", "texture.PNG", errTestPNGDecoder}} {
		if _, err := decoder.DecodeImage([]byte(test.data), test.name, TextureOptions{}); !errors.Is(err, test.want) {
			t.Fatalf("%s: %v", test.name, err)
		}
	}
	containerBase := ImageDecoderWith[ImageDecoderSet[testContainerDecoder, NoImageDecoder, NoImageDecoder, NoImageDecoder, NoImageDecoder], testPNGDecoder]{}
	if _, err := containerBase.DecodeImage([]byte("CTR1payload"), "texture.png", TextureOptions{}); !errors.Is(err, errTestContainerDecoder) {
		t.Fatal("extra extension overrode a base container", err)
	}
}
