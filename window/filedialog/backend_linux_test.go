//go:build linux && cgo && !android && !ios && !js && !wasm && !onix_drm

package filedialog

import (
	"errors"
	"net/url"
	"path/filepath"
	"slices"
	"testing"

	"github.com/diiyw/onix/window"
	"github.com/godbus/dbus/v5"
)

func TestPortalResponsePaths(t *testing.T) {
	results := map[string]dbus.Variant{
		"uris": dbus.MakeVariant([]string{"file:///tmp/100%25%20ready.png", "file:///tmp/second.jpg"}),
	}
	for _, test := range []struct {
		name     string
		response portalResponse
		want     []string
		wantErr  error
	}{
		{name: "selected files", response: portalResponse{Results: results}, want: []string{"/tmp/100% ready.png", "/tmp/second.jpg"}},
		{name: "no selection"},
		{name: "cancelled", response: portalResponse{Code: 1}},
		{name: "closed", response: portalResponse{Code: 2}},
		{name: "cancelled ignores results", response: portalResponse{Code: 1, Results: results}},
		{name: "closed ignores results", response: portalResponse{Code: 2, Results: results}},
		{name: "unknown response code", response: portalResponse{Code: 3}, wantErr: window.ErrFileDialogUnavailable},
		{name: "invalid paths", response: portalResponse{Results: map[string]dbus.Variant{"uris": dbus.MakeVariant("file:///tmp/image.png")}}, wantErr: window.ErrFileDialogUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			paths, err := test.response.paths()
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("response error = %v, want %v", err, test.wantErr)
			}
			if !slices.Equal(paths, test.want) {
				t.Fatalf("response paths = %q, want %q", paths, test.want)
			}
		})
	}
}

func TestPortalFileDialogPathUsesNullTerminatedNativePath(t *testing.T) {
	want, err := filepath.Abs(filepath.Join("testdata", "folder"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := portalFileDialogPath(filepath.Join("testdata", "folder"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != filepath.ToSlash(want)+"\x00" {
		t.Fatalf("portal folder = %q, want null-terminated %q", got, want)
	}
}

func TestFileDialogPathFromURIDecodesOnce(t *testing.T) {
	want := "/tmp/100% ready.png"
	raw := (&url.URL{Scheme: "file", Path: want}).String()
	got, ok := fileDialogPathFromURI(raw)
	if !ok || got != filepath.FromSlash(want) {
		t.Fatalf("portal URI %q = %q, %v", raw, got, ok)
	}
}

func TestFileDialogPathFromURIRejectsNonLocalPaths(t *testing.T) {
	for _, raw := range []string{"https://example.com/image.png", "file://server/share/image.png", "file://"} {
		if path, ok := fileDialogPathFromURI(raw); ok {
			t.Fatalf("non-local portal URI %q produced %q", raw, path)
		}
	}
}
