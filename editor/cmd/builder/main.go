// Command builder preprocesses a project resource package before compiling it.
//
//	go run ./editor/cmd/builder run ../my-game
//	go run ./editor/cmd/builder test ../my-game
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/diiyw/onix/editor"
)

func main() {
	if len(os.Args) < 3 {
		fatalf("usage: builder <run|test> <project-dir> [args]")
	}
	command, projectArg := os.Args[1], os.Args[2]
	projectDir, err := filepath.Abs(projectArg)
	if err != nil {
		fatalf("resolve project directory: %v", err)
	}
	build, err := editor.PrepareDevelopmentBuild(projectDir, func(p editor.BuildProgress) { fmt.Fprintln(os.Stdout, p.Message) })
	if err != nil {
		fatalf("prepare project: %v", err)
	}
	defer func() {
		if err := build.Cleanup(); err != nil {
			fmt.Fprintln(os.Stderr, "cleanup development build:", err)
		}
	}()

	root, err := repositoryRoot(projectDir)
	if err != nil {
		fatalf("resolve repository root: %v", err)
	}
	env := append(os.Environ(), "ONIX_RESOURCE_DIR="+build.ResourceDir)
	var args []string
	var runArgs []string
	var output string
	switch command {
	case "run":
		runArgs = append([]string(nil), os.Args[3:]...)
		output = filepath.Join(build.ExecutableDir, "project")
		if runtime.GOOS == "windows" {
			output += ".exe"
		}
		args = []string{"build", "-v", "-mod=mod", "-buildvcs=false", "-trimpath", "-overlay", build.OverlayPath, "-o", output, build.ProjectImportPath}
	case "test":
		compileArgs, testArgs := splitTestArgs(os.Args[3:])
		runArgs = testArgs
		output = filepath.Join(build.ExecutableDir, "project.test")
		args = []string{"test", "-mod=mod", "-buildvcs=false", "-overlay", build.OverlayPath, "-c", "-o", output}
		args = append(args, compileArgs...)
		args = append(args, build.ProjectImportPath)
	default:
		fatalf("unknown command %q; want run or test", command)
	}

	fmt.Fprintln(os.Stdout, "Compiling project...")
	compile := exec.Command("go", args...)
	compile.Dir = root
	compile.Env = env
	compile.Stdout = os.Stdout
	compile.Stderr = os.Stderr
	if err := compile.Run(); err != nil {
		fatalf("go %s: %v", command, err)
	}
	if command == "test" {
		if _, err := os.Stat(output); err != nil {
			if os.IsNotExist(err) {
				fatalf("project has no test files")
			}
			fatalf("inspect test binary: %v", err)
		}
	}

	fmt.Fprintln(os.Stdout, "Compilation complete; starting project...")
	process := exec.Command(output, runArgs...)
	process.Dir = build.ExecutableDir
	process.Env = env
	process.Stdin = os.Stdin
	process.Stdout = os.Stdout
	process.Stderr = os.Stderr
	if err := process.Run(); err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			os.Exit(exitError.ExitCode())
		}
		fatalf("run %s: %v", command, err)
	}
}

// splitTestArgs keeps go command flags for compilation and translates the
// standard test-selection flags to the names accepted by a compiled test
// binary. Arguments after -- are passed to the binary unchanged.
func splitTestArgs(args []string) (compileArgs, testArgs []string) {
	const testPrefix = "-test."
	runOnly := false
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" {
			runOnly = true
			continue
		}
		if runOnly {
			testArgs = append(testArgs, arg)
			continue
		}
		if strings.HasPrefix(arg, testPrefix) {
			testArgs = append(testArgs, arg)
			name := strings.TrimPrefix(arg, testPrefix)
			if !strings.Contains(arg, "=") && testFlagTakesValue(name) && index+1 < len(args) {
				testArgs = append(testArgs, args[index+1])
				index++
			}
			continue
		}
		if name, ok := testFlagName(arg); ok {
			translated := testPrefix + name
			if equals := strings.IndexByte(arg, '='); equals >= 0 {
				translated += arg[equals:]
			}
			testArgs = append(testArgs, translated)
			if !strings.Contains(arg, "=") && testFlagTakesValue(name) && index+1 < len(args) {
				testArgs = append(testArgs, args[index+1])
				index++
			}
			continue
		}
		compileArgs = append(compileArgs, arg)
	}
	return compileArgs, testArgs
}

func testFlagTakesValue(name string) bool {
	switch name {
	case "bench", "benchtime", "count", "list", "parallel", "run", "shuffle", "skip", "timeout":
		return true
	default:
		return false
	}
}

func testFlagName(arg string) (string, bool) {
	name := strings.TrimPrefix(arg, "-")
	if equals := strings.IndexByte(name, '='); equals >= 0 {
		name = name[:equals]
	}
	switch name {
	case "bench", "benchmem", "benchtime", "count", "failfast", "fullpath", "list", "parallel", "run", "short", "shuffle", "skip", "timeout", "v":
		return name, true
	default:
		return "", false
	}
}

func repositoryRoot(start string) (string, error) {
	for dir := filepath.Clean(start); ; dir = filepath.Dir(dir) {
		info, err := os.Stat(filepath.Join(dir, "go.mod"))
		if err == nil && !info.IsDir() {
			return dir, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found from %s", start)
		}
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "builder: %s\n", strings.TrimSpace(fmt.Sprintf(format, args...)))
	os.Exit(1)
}
