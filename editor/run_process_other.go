//go:build !windows

package editor

import "os/exec"

func stopRunCommand(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}
