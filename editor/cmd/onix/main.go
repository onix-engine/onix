package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"

	"github.com/diiyw/onix/editor"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/window/filedialog"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func run(args []string, output io.Writer) error {
	if len(args) > 0 {
		switch args[0] {
		case "new":
			flags := flag.NewFlagSet("onix new", flag.ContinueOnError)
			flags.SetOutput(output)
			width := flags.Int("width", 750, "window and viewport width")
			height := flags.Int("height", 1624, "window and viewport height")
			title := flags.String("title", "", "window title (defaults to directory name)")
			modulePath := flags.String("module", "", "independent project Go module path")
			engineDirectory := flags.String("engine-dir", "", "local Onix checkout for independent projects")
			engineVersion := flags.String("engine-version", "", "published Onix Go module version")
			if err := flags.Parse(args[1:]); err != nil {
				return err
			}
			if flags.NArg() != 1 {
				return fmt.Errorf("usage: onix new [-width N] [-height N] [-title NAME] DIRECTORY")
			}
			plan, err := editor.CreateModuleProject(flags.Arg(0), project.CreateProjectOptions{Width: *width, Height: *height, Title: *title, ModulePath: *modulePath, EngineDirectory: *engineDirectory, EngineVersion: *engineVersion})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(output, "Created project %s\nEntry: %s\n", flags.Arg(0), plan.Spec.Entry)
			return err
		case "check":
			if len(args) != 2 {
				return fmt.Errorf("usage: onix check DIRECTORY")
			}
			plan, err := editor.CheckModuleProject(args[1])
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(output, "Valid project: %d scenes, %d module definitions\nEntry: %s\n", len(plan.Scenes), len(plan.Mods), plan.Spec.Entry)
			return err
		case "generate":
			if len(args) != 2 {
				return fmt.Errorf("usage: onix generate DIRECTORY")
			}
			if _, err := editor.GenerateModuleProject(args[1]); err != nil {
				return err
			}
			_, err := fmt.Fprintln(output, "Generated onix_generated.go without changing main.go")
			return err
		case "build":
			flags := flag.NewFlagSet("onix build", flag.ContinueOnError)
			flags.SetOutput(output)
			platform := flags.String("platform", runtime.GOOS, "windows, macos, linux, web, android, ios or wechat")
			mode := flags.String("mode", "debug", "debug or release")
			format := flags.String("package", "", "platform package format")
			arch := flags.String("arch", "universal", "macOS architecture")
			sdk := flags.String("sdk", "", "Android SDK directory")
			ndk := flags.String("ndk", "", "Android NDK directory")
			if err := flags.Parse(args[1:]); err != nil {
				return err
			}
			if flags.NArg() != 1 {
				return fmt.Errorf("usage: onix build [-platform TARGET] [-mode debug|release] DIRECTORY")
			}
			if *platform == "darwin" {
				*platform = "macos"
			}
			options := editor.DefaultBuildOptions(flags.Arg(0))
			options.Platform = editor.BuildPlatform(*platform)
			options.Mode = editor.BuildMode(*mode)
			options.PackageFormat = editor.BuildPackageFormat(*format)
			options.Architecture = editor.BuildArchitecture(*arch)
			options.AndroidSDK = *sdk
			options.AndroidNDK = *ndk
			options.Progress = func(p editor.BuildProgress) { fmt.Fprintln(output, p.Message) }
			artifact, err := editor.BuildProject(options)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(output, "Built "+artifact)
			return err
		case "migrate":
			flags := flag.NewFlagSet("onix migrate", flag.ContinueOnError)
			flags.SetOutput(output)
			destination := flags.String("output", "", "new draft directory (required)")
			if err := flags.Parse(args[1:]); err != nil {
				return err
			}
			if flags.NArg() != 1 || *destination == "" {
				return fmt.Errorf("usage: onix migrate -output DIRECTORY SOURCE")
			}
			report, err := editor.MigrateModuleProject(flags.Arg(0), *destination)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(output, "Migration draft: %s\nReady: %t; items to rewrite: %d\nReport: resources/migration-report.json\n", report.Output, report.Ready, len(report.Issues))
			return err
		}
	}
	if len(args) > 1 {
		return fmt.Errorf("usage: onix [PROJECT_DIRECTORY] | new [OPTIONS] DIRECTORY | check DIRECTORY | generate DIRECTORY | build [OPTIONS] DIRECTORY")
	}
	filedialog.Register()
	projectDir := ""
	if len(args) == 1 {
		projectDir = args[0]
	}
	return editor.Run(projectDir)
}
