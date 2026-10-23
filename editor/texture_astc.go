package editor

import "github.com/diiyw/onix/internal/texcompress"

func encodeASTCChain(pixels []byte, width, height int, _ BuildMode, srgb bool) ([]byte, int, error) {
	return texcompress.EncodeASTCChain(pixels, width, height, srgb)
}
