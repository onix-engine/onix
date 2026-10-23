package editor

import (
	"fmt"
)

func (a *ModuleEditor) startModuleBuild() error {
	if a.Document == nil || a.runDone != nil || a.buildDone != nil {
		return fmt.Errorf("Project is not ready or a build is already running")
	}
	if err := a.applyDrafts(); err != nil {
		return err
	}
	if err := a.Document.Save(); err != nil {
		return err
	}
	options := a.buildOptions
	options.ProjectDir = a.Document.ProjectDir
	a.buildDone = make(chan buildResult, 1)
	a.buildProgress = newBuildProgressFeed(a.requestRender)
	done, wake := a.buildDone, a.requestRender
	options.Progress = a.buildProgress.report
	a.buildLog = ""
	a.buildOpen = false
	a.buildState = BuildProgress{Message: "Building project…", Indeterminate: true}
	a.status = "Building project…"
	a.changed()
	go func() {
		artifact, err := BuildProject(options)
		done <- buildResult{artifact: artifact, outputDir: buildPlatformOutputDir(options.ProjectDir, options.Platform), err: err}
		if wake != nil {
			wake()
		}
	}()
	return nil
}

func (a *ModuleEditor) updateModuleBuild() {
	if a.buildDone == nil {
		return
	}
	a.updateBuildProgress()
	select {
	case result := <-a.buildDone:
		a.updateBuildProgress()
		a.buildDone = nil
		a.buildProgress = nil
		a.status = "Build complete: " + result.artifact
		a.buildArtifact = result.artifact
		a.buildDirectory = result.outputDir
		a.showWorkspaceDialog("build-success")
		if result.err != nil {
			a.status = "Build failed: " + result.err.Error()
			a.buildError = a.buildLog + "\n" + result.err.Error()
			a.showWorkspaceDialog("build-error")
		}
		if a.Document != nil {
			if err := a.Document.Refresh(); err != nil {
				a.status += "\nRefresh generated files: " + err.Error()
			}
		}
		a.changed()
	default:
	}
}
