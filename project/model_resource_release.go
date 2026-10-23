//go:build onix_release

package project

import (
	"fmt"
	iofs "io/fs"
	pathpkg "path"

	"github.com/diiyw/onix/internal/typedbin"
)

func decodeModelResource(_ iofs.FS, path string, data []byte) (compiledModelResource, error) {
	if !typedbin.IsEncoded(data) {
		return compiledModelResource{}, fmt.Errorf("decode %s: release builds require preprocessed ONIXBIN1 model resources", pathpkg.Base(path))
	}
	return typedbin.Unmarshal[compiledModelResource](data)
}
