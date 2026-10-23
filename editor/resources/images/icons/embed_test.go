package icons

import (
	"strings"
	"testing"

	"github.com/diiyw/onix/graphics"
)

func TestSpineIconSupportsTint(t *testing.T) {
	data, err := embedded.ReadFile("spine.svg")
	if err != nil {
		t.Fatalf("read spine icon: %v", err)
	}
	tinted := tintCurrentColor(string(data), graphics.ColorFromRGBA(10, 90, 240, 255))
	if tinted == string(data) {
		t.Fatal("spine icon does not contain a tintable currentColor value")
	}
	if strings.Contains(strings.ToLower(tinted), "currentcolor") {
		t.Fatal("spine icon still contains currentColor after tinting")
	}
}

func TestComponentIconSupportsTint(t *testing.T) {
	data, err := embedded.ReadFile("component.svg")
	if err != nil {
		t.Fatalf("read component icon: %v", err)
	}
	tinted := tintCurrentColor(string(data), graphics.ColorFromRGBA(10, 90, 240, 255))
	if tinted == string(data) {
		t.Fatal("component icon does not contain a tintable currentColor value")
	}
	if strings.Contains(strings.ToLower(tinted), "currentcolor") {
		t.Fatal("component icon still contains currentColor after tinting")
	}
}
