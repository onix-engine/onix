package editor

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// buildProgressFeed retains the newest state and a bounded log independently
// of the render rate. A busy compiler cannot block on the UI or lose its newest
// update when many resources finish between frames.
type buildProgressFeed struct {
	mu      sync.Mutex
	state   BuildProgress
	log     runOutputCapture
	dirty   bool
	wake    func()
	started time.Time
}

func newBuildProgressFeed(wake func()) *buildProgressFeed {
	return &buildProgressFeed{wake: wake, started: time.Now()}
}

func (f *buildProgressFeed) report(p BuildProgress) {
	f.mu.Lock()
	p.Value = max(f.state.Value, clamp(p.Value, 0, 1))
	if p.Message != "" && p.Message != f.state.Message {
		_, _ = f.log.Write([]byte(p.Message))
		_, _ = f.log.Write([]byte{'\n'})
	}
	f.state, f.dirty = p, true
	f.mu.Unlock()
	if f.wake != nil {
		f.wake()
	}
}

func (f *buildProgressFeed) snapshot() (BuildProgress, string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.dirty {
		return f.state, "", false
	}
	f.dirty = false
	return f.state, f.log.String(), true
}

func reportBuildActivity(progress func(BuildProgress), message string, value float32) {
	if progress != nil {
		progress(BuildProgress{Message: message, Value: value, Indeterminate: true})
	}
}

// commandProgressWriter reuses its line buffer across writes, including partial
// lines and carriage-return based tool output. The shared stdout/stderr writer
// makes os/exec serialize writes without an additional reader goroutine.
type commandProgressWriter struct {
	destination io.Writer
	progress    func(BuildProgress)
	stage       string
	value       float32
	line        [4096]byte
	size        int
}

func (w *commandProgressWriter) Write(data []byte) (int, error) {
	n, err := w.destination.Write(data)
	for _, b := range data[:n] {
		if b == '\n' || b == '\r' {
			w.flush()
			continue
		}
		if w.size == len(w.line) {
			w.flush()
		}
		w.line[w.size] = b
		w.size++
	}
	return n, err
}

func (w *commandProgressWriter) flush() {
	line := bytes.TrimSpace(w.line[:w.size])
	if len(line) > 0 && w.progress != nil {
		w.progress(BuildProgress{Message: w.stage + ": " + string(line), Value: w.value, Indeterminate: true})
	}
	w.size = 0
}

func runCommand(root string, env []string, name string, args ...string) error {
	return runCommandWithProgress(nil, root, env, name, args...)
}

func runCommandWithProgress(progress func(BuildProgress), root string, env []string, name string, args ...string) error {
	stage, value := "Packaging", float32(.9)
	if name == "go" && len(args) > 0 && args[0] == "build" {
		args = append([]string{"build", "-mod=mod", "-v"}, args[1:]...)
		stage, value = "Compiling", .65
	}
	reportBuildActivity(progress, stage+" ("+name+")...", value)
	cmd := exec.Command(name, args...)
	cmd.Dir, cmd.Env = root, env
	var output runOutputCapture
	writer := commandProgressWriter{destination: captureRunOutput(os.Stdout, &output), progress: progress, stage: stage, value: value}
	cmd.Stdout, cmd.Stderr = &writer, &writer
	err := cmd.Run()
	writer.flush()
	if err != nil {
		return fmt.Errorf("%s %s failed:\n%s", name, strings.Join(args, " "), strings.TrimSpace(output.String()))
	}
	return nil
}

func (a *ModuleEditor) updateBuildProgress() {
	if a.buildProgress == nil {
		return
	}
	if state, log, changed := a.buildProgress.snapshot(); changed {
		a.buildState, a.buildLog, a.status = state, log, state.Message
		a.progressDirty = true
	}
	// Keep activity and elapsed time moving even while a single compiler or
	// resource conversion runs silently. This does not invent a completion %.
	if a.buildState.Indeterminate {
		a.progressDirty = true
		if a.requestWake != nil {
			a.requestWake(time.Now().Add(time.Second / 20))
		}
	}
	if a.progressDirty && a.requestRender != nil {
		a.requestRender()
	}
}
