package svg

import (
	"strings"
	"testing"
)

func TestDecodeConfigDimensions(t *testing.T) {
	for _, test := range []struct {
		source        string
		width, height int
	}{
		{`<svg/>`, 300, 150},
		{`<svg viewBox="0 0 32 16"/>`, 32, 16},
		{`<svg width="24" viewBox="0 0 32 16"/>`, 24, 16},
		{`<svg width="2cm" height="12pt"/>`, 76, 16},
		{`<svg width="0" height="-1" viewBox="0 0 31.5 15.5"/>`, 32, 16},
		{`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" width="20" height="10"><g><path d="M0 0 C1 8 8 1 10 10"/></g></svg>`, 20, 10},
	} {
		document, err := Decode(strings.NewReader(test.source))
		if err != nil {
			t.Fatal(err)
		}
		config, err := DecodeConfig(strings.NewReader(test.source))
		if err != nil {
			t.Fatal(err)
		}
		width, height := document.outputSize(Options{})
		if config.Width != test.width || config.Height != test.height || width != test.width || height != test.height {
			t.Fatalf("%s: config=%dx%d, render=%dx%d, want=%dx%d", test.source, config.Width, config.Height, width, height, test.width, test.height)
		}
	}
}

func TestDecodeConfigValidatesXMLWithoutBuildingPaths(t *testing.T) {
	for _, source := range []string{"", `<html/>`, `<svg><g></svg>`, `<svg><path`, `<svg>`} {
		if _, err := DecodeConfig(strings.NewReader(source)); err == nil {
			t.Fatalf("invalid XML accepted: %q", source)
		}
	}
	const source = `<svg width="24" height="16"><path d="invalid geometry"/></svg>`
	if _, err := Decode(strings.NewReader(source)); err == nil {
		t.Fatal("invalid path fixture was accepted by the renderer")
	}
	if config, err := DecodeConfig(strings.NewReader(source)); err != nil || config.Width != 24 || config.Height != 16 {
		t.Fatalf("geometry affected dimensions: %+v, %v", config, err)
	}
}

func BenchmarkDecodeConfigPaths(b *testing.B) {
	source := `<svg viewBox="0 0 24 24">` + strings.Repeat(`<path d="M0 0 C0 24 24 0 24 24 Q12 0 0 0Z"/>`, 128) + `</svg>`
	b.ReportAllocs()
	for b.Loop() {
		if _, err := DecodeConfig(strings.NewReader(source)); err != nil {
			b.Fatal(err)
		}
	}
}
