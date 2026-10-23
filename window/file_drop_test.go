package window

import "testing"

func TestNormalizeDroppedPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "local uri", in: "file:///tmp/hello%20world.txt", want: "/tmp/hello world.txt"},
		{name: "localhost uri", in: "file://localhost/tmp/item.txt", want: "/tmp/item.txt"},
		{name: "browser name", in: "item.txt", want: "item.txt"},
		{name: "space-preserving name", in: " item.txt ", want: " item.txt "},
		{name: "remote uri", in: "file://server/share/item.txt", want: ""},
		{name: "web uri", in: "https://example.com/item.txt", want: ""},
		{name: "hash-prefixed name", in: "#dropped-file", want: "#dropped-file"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeDroppedPath(test.in); got != test.want {
				t.Fatalf("normalizeDroppedPath(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}
