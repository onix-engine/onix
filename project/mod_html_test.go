package project

import (
	"io/fs"
	"net/url"
	"path"
	"strings"
	"testing"
)

func referenceHTMLImagePath(base, source string) (string, bool) {
	u, err := url.Parse(source)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Opaque != "" || u.Path == "" ||
		strings.HasPrefix(u.Path, "/") || strings.ContainsAny(u.Path, "\\:") {
		return "", false
	}
	file := path.Join(base, u.Path)
	return file, file != "." && fs.ValidPath(file)
}

func FuzzHTMLImagePath(f *testing.F) {
	for _, s := range []string{
		"tile.png", "../images/tile.png", "../../..", "./", "a+b", "%E6%96%B9.png",
		"sprite%23%3f.png?v=%#frame", "a%2fb", "%2froot", "%5croot", "%3af", "a/%2e%2e/b",
		"", "?x", "#x", "//host", "file:///path", "/root", "scheme:x", "a/b:c",
		"img?%xx", "img#%xx", "img?\x00", "img\x7f", "img#\x00", "%00", "%ff", "a%20b",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		for _, base := range []string{"", "levels/ui"} {
			got, ok := htmlImagePath(base, s)
			want, accepted := referenceHTMLImagePath(base, s)
			if ok != accepted || ok && got != want {
				t.Fatalf("base %q, source %q: got (%q, %t), want (%q, %t)", base, s, got, ok, want, accepted)
			}
		}
	})
}
