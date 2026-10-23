package editor

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestBuildCommandStreamsOutputAndBoundsFailure(t *testing.T) {
	log, err := os.CreateTemp(t.TempDir(), "build-output-")
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = log
	defer func() {
		os.Stdout = original
		log.Close()
	}()
	err = runCommand(t.TempDir(), append(os.Environ(), "ONIX_BUILD_COMMAND_LARGE_OUTPUT=1"), os.Args[0], "-test.run=^TestBuildCommandLargeOutputHelper$")
	if err == nil {
		t.Fatal("expected command failure")
	}
	_, output, ok := strings.Cut(err.Error(), "\n")
	if !ok || !strings.HasPrefix(output, truncatedRunOutputPrefix) {
		t.Fatal("large build output is missing its truncation marker")
	}
	if len(output) > maxRunOutputBytes+len(truncatedRunOutputPrefix) {
		t.Fatalf("build failure retained %d bytes of output", len(output))
	}
	if !strings.HasSuffix(output, "fatal: final failure") {
		t.Fatal("build failure lost its final diagnostic")
	}
	info, err := log.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if want := int64(8<<20 + len("fatal: final failure\n")); info.Size() != want {
		t.Fatalf("streamed output = %d bytes, want %d", info.Size(), want)
	}
}

func TestBuildCommandLargeOutputHelper(t *testing.T) {
	if os.Getenv("ONIX_BUILD_COMMAND_LARGE_OUTPUT") == "" {
		return
	}
	chunk := bytes.Repeat([]byte{'x'}, 32<<10)
	for range 256 {
		if _, err := os.Stdout.Write(chunk); err != nil {
			os.Exit(7)
		}
	}
	fmt.Fprintln(os.Stdout, "fatal: final failure")
	os.Exit(6)
}
