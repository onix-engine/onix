package editor

import (
	"fmt"

	onixtext "github.com/diiyw/onix/text"
)

func loadSystemUIFont() (*onixtext.Font, error) {
	value := onixtext.DefaultFont()
	if value == nil {
		return nil, fmt.Errorf("load operating system UI font: no usable system font")
	}
	return value, nil
}
