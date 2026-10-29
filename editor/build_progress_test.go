package editor

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	engineui "github.com/diiyw/onix/ui"
	htmlui "github.com/diiyw/onix/ui/html"
)

func TestBuildProgressFeedRetainsEventsBetweenFrames(t *testing.T) {
	feed := newBuildProgressFeed(nil)
	var workers sync.WaitGroup
	for worker := range 2 {
		workers.Go(func() {
			for index := range 100 {
				feed.report(BuildProgress{Message: fmt.Sprintf("module-%d/resource-%d", worker, index), Value: float32(index) / 100})
			}
		})
	}
	workers.Wait()
	feed.report(BuildProgress{Message: "Linking", Value: .65, Indeterminate: true})
	state, log, changed := feed.snapshot()
	if !changed || state.Message != "Linking" || state.Value != .99 || !state.Indeterminate {
		t.Fatalf("newest state missing or progress regressed: %+v", state)
	}
	for worker := range 2 {
		for index := range 100 {
			if !strings.Contains(log, fmt.Sprintf("module-%d/resource-%d\n", worker, index)) {
				t.Fatalf("resource event lost between frames: worker=%d index=%d", worker, index)
			}
		}
	}
	if _, _, changed := feed.snapshot(); changed {
		t.Fatal("idle snapshot still marked dirty")
	}
	if allocations := testing.AllocsPerRun(100, func() { feed.report(state) }); allocations != 0 {
		t.Fatalf("unchanged progress allocates %g times", allocations)
	}
}

func TestCommandProgressHandlesSplitLinesAndCarriageReturns(t *testing.T) {
	var output bytes.Buffer
	var events []BuildProgress
	writer := commandProgressWriter{destination: &output, stage: "Compiling", value: .65, progress: func(p BuildProgress) { events = append(events, p) }}
	for _, data := range []string{"example.com/", "game/a\nexample.com/game/b\r", "linking"} {
		if _, err := writer.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	writer.flush()
	if len(events) != 3 || events[0].Message != "Compiling: example.com/game/a" || events[1].Message != "Compiling: example.com/game/b" || events[2].Message != "Compiling: linking" {
		t.Fatalf("incomplete compiler output: %+v", events)
	}
	if output.String() != "example.com/game/a\nexample.com/game/b\rlinking" {
		t.Fatal("progress altered captured diagnostics")
	}
	for _, event := range events {
		if !event.Indeterminate || event.Value >= 1 {
			t.Fatal("compiler output falsely reports completion")
		}
	}
}

func TestModuleResourceProgressListsPackagesAndCachedRestores(t *testing.T) {
	directory := moduleBuildFixture(t)
	moduleBuildFile(t, directory, "resources/packages.json", `{"packages":[{"name":"secondary","include":["asset/**"]}]}`)
	var events []BuildProgress
	options := resourcePackageOptions{Mode: BuildModeDebug, Platform: BuildPlatformWeb, Progress: func(p BuildProgress) { events = append(events, p) }}
	for attempt := range 2 {
		events = events[:0]
		catalog, _, err := writeModuleResourcePackages(directory, directory, filepath.Join(t.TempDir(), "publish"), options)
		if err != nil {
			t.Fatal(err)
		}
		prefix := "Packed module package "
		if attempt == 1 {
			prefix = "Restoring module package "
		}
		for _, info := range catalog.Packages {
			found := false
			for _, event := range events {
				found = found || strings.HasPrefix(event.Message, prefix+info.Name+" (")
			}
			if !found {
				t.Fatalf("package %s not listed on attempt %d", info.Name, attempt)
			}
		}
		if len(events) == 0 || events[len(events)-1].Value != 1 || events[len(events)-1].Indeterminate {
			t.Fatal("resource work did not report completion")
		}
		if attempt == 0 {
			found := false
			for _, event := range events {
				found = found || strings.Contains(event.Message, "Packing secondary / asset/custom/value.dat")
			}
			if !found {
				t.Fatal("individual private module resource not listed")
			}
		}
	}
}

func TestWorkspaceProgressShowsPlayLogAndKeepsAnimating(t *testing.T) {
	a := workspaceEditorFixture(t)
	a.runBuilding, a.runDone = true, make(chan runProcessResult, 1)
	a.buildProgress = newBuildProgressFeed(nil)
	a.buildProgress.report(BuildProgress{Message: "Compiling example.com/game", Value: .65, Indeterminate: true})
	var wakes int
	a.requestWake = func(time.Time) { wakes++ }
	a.updateBuildProgress()
	if !a.modalVisible() || a.activeWorkspaceDialog() != "progress" || wakes != 1 {
		t.Fatal("Play compilation did not display an animated progress dialog")
	}
	world := ecs.NewWorld()
	syncWorkspaceTest(t, a, world)
	frame := a.dialogUI.htmlDialogFrames["progress"]
	if element := frame.element(htmlDialogFieldID("progress", "log")); element.Value != a.buildLog || !element.ReadOnly {
		t.Fatal("resource and compile log not displayed read-only")
	}
	if _, ok := htmlui.NodeByID(world, a.dialogUI.htmlDialogs["progress"], htmlDialogFieldID("progress", "log")); !ok {
		t.Fatal("progress log field missing from authored HTML")
	}
	before := frame.element("progress-activity").Style
	a.time += .5
	a.updateBuildProgress()
	syncWorkspaceTest(t, a, world)
	if before == frame.element("progress-activity").Style || wakes != 2 {
		t.Fatal("silent compilation froze the activity bar")
	}
	a.closeWorkspaceDialog()
	if a.activeWorkspaceDialog() != "progress" {
		t.Fatal("active Play compilation dialog was dismissed")
	}
}

func TestWorkspaceProgressKeepsNewestLogLineVisible(t *testing.T) {
	a := workspaceEditorFixture(t)
	a.runBuilding = true
	a.buildProgress = newBuildProgressFeed(nil)
	world := ecs.NewWorld()
	engineui.RegisterRuntimeFeature[engineui.FullRuntime](world)
	syncLog := func() ecs.Entity {
		t.Helper()
		a.updateBuildProgress()
		for range 2 {
			a.progressDirty = true
			syncWorkspaceTest(t, a, world)
			if err := engineui.SyncSystem("").Run(world); err != nil {
				t.Fatal(err)
			}
			if err := engineui.SyncAfterLayoutSystem("").Run(world); err != nil {
				t.Fatal(err)
			}
		}
		for entity, access := range world.Query[htmlui.Control]().All {
			if control := access.Ref(); control.Owner == a.dialogUI.htmlDialogs["progress"] && control.ID == htmlDialogFieldID("progress", "log") {
				return entity
			}
		}
		t.Fatal("progress log control missing")
		return 0
	}
	verifyTail := func(control ecs.Entity) {
		t.Helper()
		textarea, _ := world.Get[engineui.Textarea](control)
		field, _ := world.Get[engineui.TextField](control)
		if field.Focused || !textarea.FollowTail || textarea.ScrollY <= 0 {
			t.Fatalf("unfocused log did not follow its tail: %+v", textarea)
		}
		for entity, access := range world.Query2[htmlui.ControlText, scene.Text2D]().All {
			link := access.A.Ref()
			if link.Control != control || !world.Has[engineui.Part[engineui.ValuePart]](entity) {
				continue
			}
			_, height, measured := scene.MeasureText2D(access.B.Ref())
			transform, _ := world.Get[scene.UITransform](entity)
			if !measured || transform.Translation.Y.Value+height > link.Content.Y+link.Content.Height+.01 {
				t.Fatal("newest log line extends below the content viewport")
			}
			return
		}
		t.Fatal("progress log text missing")
	}
	for index := range 40 {
		a.buildProgress.report(BuildProgress{Message: fmt.Sprintf("Compiling module %d", index), Indeterminate: true})
	}
	control := syncLog()
	verifyTail(control)
	previous, _ := world.Get[engineui.Textarea](control)
	world.GetMut[engineui.Textarea](control).ScrollY = 0
	a.buildProgress.report(BuildProgress{Message: "Linking latest module", Indeterminate: true})
	control = syncLog()
	verifyTail(control)
	current, _ := world.Get[engineui.Textarea](control)
	if current.ScrollY <= previous.ScrollY {
		t.Fatal("appended log did not advance the viewport")
	}
}

func TestPlayPreparationStopCleansResourcesWithoutStartingProcess(t *testing.T) {
	a := workspaceEditorFixture(t)
	directory := t.TempDir()
	executable := filepath.Join(directory, "game")
	moduleBuildFile(t, directory, "game", "not an executable")
	staging := filepath.Join(t.TempDir(), "generated")
	moduleBuildFile(t, staging, "source.go", "package main")
	a.runDirectory, a.runExecutable = directory, executable
	a.runBuilding, a.runStopping = true, true
	a.runDone, a.runPrepared = make(chan runProcessResult, 1), make(chan preparedRun, 1)
	a.runPrepared <- preparedRun{build: DevelopmentBuild{layout: projectBuildLayout{AppDir: staging}}}
	a.updateRun()
	if a.runDone != nil || a.runCmd != nil || a.runBuilding || a.runPrepared != nil {
		t.Fatal("cancelled preparation left Play active")
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatal("cancelled preparation left generated files", err)
	}
}

func TestBuildFailureNeverReportsComplete(t *testing.T) {
	var events []BuildProgress
	_, err := BuildProject(BuildOptions{ProjectDir: t.TempDir(), Mode: "invalid", Progress: func(p BuildProgress) { events = append(events, p) }})
	if err == nil {
		t.Fatal("invalid build succeeded")
	}
	for _, event := range events {
		if event.Value == 1 {
			t.Fatal("failed build reported 100%")
		}
	}
}

func TestPlayPreparesResourcesInBackground(t *testing.T) {
	a := workspaceEditorFixture(t)
	if err := a.play(); err != nil {
		t.Fatal(err)
	}
	if a.runCmd != nil || a.runPrepared == nil || !a.runBuilding || a.activeWorkspaceDialog() != "progress" {
		t.Fatal("Play did not return with background resource preparation visible")
	}
	directory := a.runDirectory
	a.action("stop")
	deadline := time.Now().Add(5 * time.Second)
	for a.runDone != nil && time.Now().Before(deadline) {
		a.updateRun()
		time.Sleep(5 * time.Millisecond)
	}
	if a.runDone != nil || a.runCmd != nil || a.runBuilding {
		t.Fatal("background preparation did not stop")
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatal("Play left its temporary executable directory", err)
	}
}

func TestBuildProgressAnimationOnlyDirtiesDialog(t *testing.T) {
	a := &ModuleEditor{buildProgress: newBuildProgressFeed(nil)}
	a.buildProgress.report(BuildProgress{Message: "Linking", Value: .65, Indeterminate: true})
	a.updateBuildProgress()
	if !a.progressDirty || a.dirty || a.canvasDirty {
		t.Fatal("progress refresh requires rebuilding the workspace or preview")
	}
}
