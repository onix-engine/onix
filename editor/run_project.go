package editor

import (
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
)

const (
	maxRunOutputBytes        = 64 << 10
	truncatedRunOutputPrefix = "[... earlier output truncated ...]\n"
)

type preparedRun struct {
	cmd   *exec.Cmd
	build DevelopmentBuild
	err   error
}

type runProcessResult struct {
	err      error
	exitCode int
	stdout   string
	stderr   string
}

type runOutputCapture struct {
	data      []byte
	start     int
	size      int
	truncated bool
}

func (c *runOutputCapture) Write(data []byte) (int, error) {
	written := len(data)
	if written == 0 {
		return 0, nil
	}
	if c.data == nil {
		c.data = make([]byte, maxRunOutputBytes)
	}
	capacity := len(c.data)
	if written >= capacity {
		c.truncated = c.truncated || c.size > 0 || written > capacity
		copy(c.data, data[written-capacity:])
		c.start = 0
		c.size = capacity
		return written, nil
	}
	if overflow := c.size + written - capacity; overflow > 0 {
		c.start = (c.start + overflow) % capacity
		c.size -= overflow
		c.truncated = true
	}
	end := (c.start + c.size) % capacity
	first := min(written, capacity-end)
	copy(c.data[end:end+first], data[:first])
	copy(c.data, data[first:])
	c.size += written
	return written, nil
}

func (c *runOutputCapture) String() string {
	if c.size == 0 {
		return ""
	}
	var output strings.Builder
	output.Grow(c.size + len(truncatedRunOutputPrefix))
	if c.truncated {
		output.WriteString(truncatedRunOutputPrefix)
	}
	first := min(c.size, len(c.data)-c.start)
	output.Write(c.data[c.start : c.start+first])
	output.Write(c.data[:c.size-first])
	return output.String()
}

func captureRunOutput(destination io.Writer, captured *runOutputCapture) io.Writer {
	if destination == nil {
		return captured
	}
	return io.MultiWriter(destination, captured)
}

func cachedProjectBuildCommand() ([]string, []string, error) {
	switch runtime.GOOS {
	case "linux":
		args := appendReleaseFlags([]string{"build", "-buildvcs=false", "-trimpath"}, BuildModeDebug)
		env, err := linuxBuildEnvironment()
		return args, env, err
	case "darwin":
		env, err := macOSBuildEnvironment(runtime.GOOS, BuildArchitecture(runtime.GOARCH), exec.LookPath)
		return macOSBuildArgs(BuildModeDebug), env, err
	case "windows":
		env, err := windowsBuildEnvironment(runtime.GOOS, runtime.GOARCH, exec.LookPath)
		return windowsBuildArgs(BuildModeDebug, ""), env, err
	default:
		return nil, nil, fmt.Errorf("running desktop projects is unsupported on %s", runtime.GOOS)
	}
}
