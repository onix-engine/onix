package editor

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/diiyw/onix/internal/brotli"
)

func TestCompressWasmBrotliReplacesRawArtifact(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), wasmFilename)
	sourceData := bytes.Repeat([]byte("\x00asmOnix WebAssembly artifact"), 1024)
	if err := os.WriteFile(sourcePath, sourceData, 0o644); err != nil {
		t.Fatal(err)
	}

	artifact, err := compressWasmBrotli(sourcePath)
	if err != nil {
		t.Fatalf("compress WASM artifact: %v", err)
	}
	if artifact != sourcePath+".br" {
		t.Fatalf("compressed artifact = %q, want %q", artifact, sourcePath+".br")
	}
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Fatalf("raw WASM artifact still exists: %v", err)
	}

	compressed, err := os.ReadFile(artifact)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := io.ReadAll(brotli.NewReader(bytes.NewReader(compressed)))
	if err != nil {
		t.Fatalf("decode WASM artifact: %v", err)
	}
	if !bytes.Equal(decoded, sourceData) {
		t.Fatalf("decoded WASM artifact differs from source")
	}
}
