package editor

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

func openSystemDirectory(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("build output directory is empty")
	}

	var name string
	switch runtime.GOOS {
	case "windows":
		name = "explorer.exe"
	case "darwin":
		name = "open"
	case "linux":
		name = "xdg-open"
	default:
		return fmt.Errorf("opening directories is unsupported on %s", runtime.GOOS)
	}

	cmd := exec.Command(name, path)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", name, err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
