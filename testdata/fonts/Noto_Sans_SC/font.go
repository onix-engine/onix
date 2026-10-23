package notosanssc

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	onixtext "github.com/diiyw/onix/text"
)

// NotoSansSCRegular is loaded from the checked-in font file at runtime.
var NotoSansSCRegular = mustLoadRegular()

// NotoSansSCBold is loaded from the checked-in font file at runtime.
var NotoSansSCBold = mustLoadBold()

func mustLoadRegular() *onixtext.Font {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		panic("locate Noto Sans SC font source")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(source), "static", "NotoSansSC-Regular.ttf"))
	if err != nil {
		panic(fmt.Errorf("read Noto Sans SC font: %w", err))
	}
	font, err := onixtext.LoadFont(data)
	if err != nil {
		panic(fmt.Errorf("parse Noto Sans SC font: %w", err))
	}
	return font
}

func mustLoadBold() *onixtext.Font {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		panic("locate Noto Sans SC font source")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(source), "static", "NotoSansSC-Bold.ttf"))
	if err != nil {
		panic(fmt.Errorf("read Noto Sans SC bold font: %w", err))
	}
	font, err := onixtext.LoadFont(data)
	if err != nil {
		panic(fmt.Errorf("parse Noto Sans SC bold font: %w", err))
	}
	return font
}
