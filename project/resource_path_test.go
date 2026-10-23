package project

import "testing"

func TestNormalizeResourcePath(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "project file", value: " resources/images/../hero.png ", want: "resources/hero.png"},
		{name: "shared repository file", value: "../fonts/ui.otf", want: "../fonts/ui.otf"},
		{name: "empty"},
		{name: "current directory", value: "."},
		{name: "absolute", value: "/tmp/hero.png"},
		{name: "windows drive", value: "C:/game/hero.png"},
		{name: "backslash", value: "resources\\hero.png"},
		{name: "url", value: "https://example.com/hero.png"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := NormalizeResourcePath(test.value)
			if test.want == "" {
				if err == nil {
					t.Fatalf("NormalizeResourcePath(%q) succeeded with %q", test.value, got)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("NormalizeResourcePath(%q) = %q, %v; want %q, nil", test.value, got, err, test.want)
			}
		})
	}
}

func TestResolveResourcePath(t *testing.T) {
	tests := []struct {
		name    string
		baseDir string
		value   string
		want    string
	}{
		{name: "project file", baseDir: "projects/game", value: "resources/hero.png", want: "projects/game/resources/hero.png"},
		{name: "shared repository file", baseDir: "projects/game", value: "../fonts/ui.otf", want: "projects/fonts/ui.otf"},
		{name: "source root file", baseDir: ".", value: "resources/hero.png", want: "resources/hero.png"},
		{name: "escapes source", baseDir: ".", value: "../hero.png"},
		{name: "nested escape", baseDir: "projects/game", value: "../../../hero.png"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolveResourcePath(test.baseDir, test.value)
			if test.want == "" {
				if err == nil {
					t.Fatalf("ResolveResourcePath(%q, %q) succeeded with %q", test.baseDir, test.value, got)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("ResolveResourcePath(%q, %q) = %q, %v; want %q, nil", test.baseDir, test.value, got, err, test.want)
			}
		})
	}
}

func TestResolveLocalResourcePath(t *testing.T) {
	resolved, err := ResolveLocalResourcePath("projects/game/resources", "icons/app.png")
	if err != nil {
		t.Fatalf("resolve local resource: %v", err)
	}
	if resolved != "projects/game/resources/icons/app.png" {
		t.Fatalf("resolved local resource = %q", resolved)
	}
	for _, value := range []string{"../app.png", "/tmp/app.png", "C:/game/app.png", "icons\\app.png"} {
		if _, err := ResolveLocalResourcePath("projects/game/resources", value); err == nil {
			t.Errorf("ResolveLocalResourcePath accepted %q", value)
		}
	}
}
