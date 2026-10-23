package scene

import (
	"encoding/json"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

func TestValResolveMatchesBevyUnits(t *testing.T) {
	viewport := mgl32.Vec2{1200, 800}
	tests := []struct {
		value Val
		base  float32
		want  float32
		ok    bool
	}{
		{value: Auto()},
		{value: Px(12), want: 24, ok: true},
		{value: Percent(25), base: 400, want: 100, ok: true},
		{value: Vw(10), want: 120, ok: true},
		{value: Vh(10), want: 80, ok: true},
		{value: VMin(10), want: 80, ok: true},
		{value: VMax(10), want: 120, ok: true},
	}
	for _, test := range tests {
		got, ok := test.value.Resolve(2, test.base, viewport)
		if ok != test.ok || got != test.want {
			t.Fatalf("Resolve(%+v) = %v, %v; want %v, %v", test.value, got, ok, test.want, test.ok)
		}
	}
}

func TestParseValSupportsBevySyntax(t *testing.T) {
	tests := map[string]Val{
		"auto":   Auto(),
		"12px":   Px(12),
		"-2.5 %": Percent(-2.5),
		"10vw":   Vw(10),
		"20vh":   Vh(20),
		"3vmin":  VMin(3),
		"4vmax":  VMax(4),
	}
	for source, want := range tests {
		got, err := ParseVal(source)
		if err != nil || got != want {
			t.Fatalf("ParseVal(%q) = %+v, %v; want %+v", source, got, err, want)
		}
	}
	if _, err := ParseVal("12"); err == nil {
		t.Fatal("ParseVal accepted a value without a unit")
	}
}

func TestValJSONUsesUnitStrings(t *testing.T) {
	data, err := json.Marshal(Percent(25))
	if err != nil || string(data) != `"25%"` {
		t.Fatalf("marshal Val = %s, %v", data, err)
	}
	var value Val
	if err := json.Unmarshal([]byte(`"12.5vmin"`), &value); err != nil || value != VMin(12.5) {
		t.Fatalf("unmarshal Val = %+v, %v", value, err)
	}
}
