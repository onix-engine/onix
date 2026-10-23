package editor

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func stopRunCommand(cmd *exec.Cmd) error {
	kill := exec.Command("taskkill", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F")
	kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := kill.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
