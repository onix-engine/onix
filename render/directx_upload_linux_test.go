//go:build linux && !android && cgo

package render

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirectXUploadPool(t *testing.T) {
	compiler, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("requires a C compiler")
	}
	source, err := os.ReadFile("directx_native.c")
	if err != nil {
		t.Fatal(err)
	}
	var sections strings.Builder
	for _, bounds := range [][2]string{
		{"typedef struct {\n    ID3D11Buffer *value;", "typedef struct {\n    ID3D11Texture2D"},
		{"static int dx_upload(", "static ID3D11Buffer *dx_static_buffer("},
		{"static void dx_bind_geometry(", "static void dx_bind_textures("},
	} {
		a := strings.Index(string(source), bounds[0])
		if a < 0 {
			t.Fatalf("missing production DirectX section %q", bounds[0])
		}
		b := strings.Index(string(source[a:]), bounds[1])
		if b < 0 {
			t.Fatalf("missing production DirectX section end %q", bounds[1])
		}
		if sections.Len() == 0 {
			sections.Write(source[a : a+b])
			sections.WriteString("typedef struct { void *device, *context; DxBuffer vertices, indices, instances; } OnixGPUDriver;\n")
		} else {
			sections.Write(source[a : a+b])
		}
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "directx_upload.inc"), []byte(sections.String()), 0600); err != nil {
		t.Fatal(err)
	}
	runNativeUploadTest(t, compiler, []string{"-std=c11", "-O2", "-Wall", "-Wextra", "-Werror", "-Wno-unused-parameter", "-I.", "-I" + dir, "tests/directx_upload_test.c"})
}
