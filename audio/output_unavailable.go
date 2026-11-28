//go:build (!js && !linux && !darwin && !windows) || (android && !cgo)

package audio

import (
	"fmt"
	"runtime"
)

type unavailableOutput struct{}

var errOutputUnavailable = fmt.Errorf("audio: native playback is unavailable in this %s build", runtime.GOOS)

func openNativeOutput() (unavailableOutput, error) {
	return unavailableOutput{}, errOutputUnavailable
}

func (unavailableOutput) write([]float32) error { return errOutputUnavailable }
func (unavailableOutput) idle() error           { return nil }
func (unavailableOutput) close(bool) error      { return nil }
