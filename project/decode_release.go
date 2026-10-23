//go:build onix_release

package project

import (
	"fmt"
	pathpkg "path"
)

func decodeJSON[T any](_ []byte, path string) (T, error) {
	var target T
	return target, fmt.Errorf("decode %s: release builds require preprocessed ONIXBIN1 resources", pathpkg.Base(path))
}
