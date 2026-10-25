//go:build windows && cgo

package window

import (
	"os"
	"os/exec"
	"sync"
	"testing"
)

func TestWindowsDialogLibrariesLoadOnDemand(t *testing.T) {
	if os.Getenv("ONIX_TEST_DIALOG_LOAD") != "1" {
		command := exec.Command(os.Args[0], "-test.run=^TestWindowsDialogLibrariesLoadOnDemand$")
		command.Env = append(os.Environ(), "ONIX_TEST_DIALOG_LOAD=1")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("dialog loader subprocess: %v\n%s", err, output)
		}
		return
	}
	if getOpenFileName != nil || coCreateInstance != nil || coTaskMemFree != nil {
		t.Fatal("dialog functions were bound before their first use")
	}
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(loadWindowsFileDialog)
	}
	workers.Wait()
	if getOpenFileName == nil || commDlgExtendedError == nil || coCreateInstance != nil {
		t.Fatal("file dialog loading did not stay independent of folder dialogs")
	}
	for range 8 {
		workers.Go(loadWindowsFolderDialog)
	}
	workers.Wait()
	if coInitializeEx == nil || coUninitialize == nil || coCreateInstance == nil || coTaskMemFree == nil || shCreateItemFromParsingName == nil {
		t.Fatal("folder dialog entry points were not bound")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		loadWindowsFileDialog()
		loadWindowsFolderDialog()
	}); allocations != 0 {
		t.Fatalf("warm dialog loading allocated %g times", allocations)
	}
}
