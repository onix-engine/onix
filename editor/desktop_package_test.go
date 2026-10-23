package editor

import (
	"archive/zip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestWriteMacOSInfoPlist(t *testing.T) {
	name := filepath.Join(t.TempDir(), "Info.plist")
	if err := writeMacOSInfoPlist(name, "Onix Demo"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		if _, err := decoder.Token(); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatalf("generated plist is not valid XML: %v\n%s", err, data)
		}
	}
	for _, value := range []string{"CFBundleExecutable", "Onix Demo", "dev.onix.onix-demo", "CFBundleIconFile", "AppIcon", "NSHighResolutionCapable"} {
		if !strings.Contains(string(data), value) {
			t.Errorf("generated plist does not contain %q", value)
		}
	}
}

func TestMacOSBundleIdentifierPartUsesAppleIdentifierCharacters(t *testing.T) {
	tests := map[string]string{
		"sample_app":  "sample-app",
		" Onix Demo ": "onix-demo",
		"42 / game":   "42-game",
		"---":         "app",
	}
	for value, want := range tests {
		if got := macOSBundleIdentifierPart(value); got != want {
			t.Errorf("macOS bundle identifier part for %q = %q, want %q", value, got, want)
		}
	}
}

func TestBuildMacOSUniversalExecutableBuildsAndCombinesBothArchitectures(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "Demo.app", "Contents", "MacOS", "Demo")
	layout := projectBuildLayout{OverlayPath: filepath.Join(root, "overlay.json"), ProjectImportPath: "example.com/demo"}
	options := BuildOptions{Mode: BuildModeRelease, Architecture: BuildArchitectureUniversal}
	lookPath := func(name string) (string, error) { return "/tools/" + name, nil }
	built := make([]string, 0, 2)
	run := func(_ string, env []string, command string, args ...string) error {
		switch command {
		case "go":
			architecture := ""
			for _, value := range env {
				if strings.HasPrefix(value, "GOARCH=") {
					architecture = strings.TrimPrefix(value, "GOARCH=")
				}
			}
			outputIndex := slices.Index(args, "-o")
			if outputIndex < 0 || outputIndex+1 >= len(args) {
				return fmt.Errorf("go build output is missing: %v", args)
			}
			built = append(built, architecture)
			return os.WriteFile(args[outputIndex+1], []byte(architecture), 0o644)
		case "/tools/lipo":
			if !slices.Contains(args, "-create") || !slices.Contains(args, "-output") {
				return fmt.Errorf("unexpected lipo arguments: %v", args)
			}
			return os.WriteFile(executable, []byte("universal"), 0o644)
		default:
			return fmt.Errorf("unexpected command %q", command)
		}
	}
	if err := buildMacOSExecutableWith("linux", root, layout, options, executable, lookPath, run); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(built, []string{"amd64", "arm64"}) {
		t.Fatalf("built architectures = %v", built)
	}
	if _, err := os.Stat(executable); err != nil {
		t.Fatalf("universal executable is missing: %v", err)
	}
	info, err := os.Stat(executable)
	if err != nil {
		t.Fatalf("stat universal executable: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("universal executable mode = %o, want an executable file", info.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(executable), ".onix-universal")); !os.IsNotExist(err) {
		t.Fatalf("universal staging directory still exists: %v", err)
	}
}

func TestWriteWixSourceIncludesExecutableAndResources(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "bundle")
	executable := filepath.Join(bundle, "Demo.exe")
	writeBuildTestFile(t, executable, "binary")
	writeBuildTestFile(t, filepath.Join(bundle, "resources", "base.dat"), "package")
	for _, schema := range []string{wixV3Namespace, wixV4Namespace} {
		t.Run(schema, func(t *testing.T) {
			output := filepath.Join(root, artifactName(schema, "wix")+".wxs")
			if err := writeWixSource(output, bundle, executable, "Demo", deterministicUUID("demo"), schema); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			decoder := xml.NewDecoder(strings.NewReader(string(data)))
			for {
				if _, err := decoder.Token(); errors.Is(err, io.EOF) {
					break
				} else if err != nil {
					t.Fatalf("generated WiX source is not valid XML: %v\n%s", err, data)
				}
			}
			for _, value := range []string{schema, "Demo.exe", "base.dat", "INSTALLFOLDER", "MainFeature", "ApplicationStartMenuShortcut"} {
				if !strings.Contains(string(data), value) {
					t.Errorf("generated WiX source does not contain %q", value)
				}
			}
		})
	}
}

func TestResolveWindowsInstallerTool(t *testing.T) {
	available := map[string]bool{"wixl": true}
	lookPath := func(name string) (string, error) {
		if available[name] {
			return "/tools/" + name, nil
		}
		return "", os.ErrNotExist
	}
	tool, err := resolveWindowsInstallerTool(lookPath)
	if err != nil || tool.kind != windowsInstallerWixl {
		t.Fatalf("resolved tool = %+v, %v", tool, err)
	}
	available = map[string]bool{"wix": true, "wixl": true}
	tool, err = resolveWindowsInstallerTool(lookPath)
	if err != nil || tool.kind != windowsInstallerWiXV4 {
		t.Fatalf("WiX v4 was not preferred: %+v, %v", tool, err)
	}
	available = nil
	if _, err := resolveWindowsInstallerTool(lookPath); err == nil {
		t.Fatal("missing installer tool did not return an error")
	}
}

func TestWindowsBuildEnvironmentUsesMinGWCrossCompiler(t *testing.T) {
	lookPath := func(name string) (string, error) { return "/toolchain/" + name, nil }
	env, err := windowsBuildEnvironment("linux", "amd64", lookPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=1",
		"CC=/toolchain/x86_64-w64-mingw32-gcc", "CXX=/toolchain/x86_64-w64-mingw32-g++",
	} {
		if !slices.Contains(env, want) {
			t.Errorf("Windows environment does not contain %q: %v", want, env)
		}
	}
	if _, err := windowsBuildEnvironment("linux", "riscv64", lookPath); err == nil {
		t.Fatal("unsupported Windows architecture was accepted")
	}
	missing := func(string) (string, error) { return "", os.ErrNotExist }
	if _, err := windowsBuildEnvironment("linux", "amd64", missing); err == nil || !strings.Contains(err.Error(), "MinGW") {
		t.Fatalf("missing MinGW error = %v", err)
	}
}

func TestVcpkgTargetTripletUsesStaticTargetTriplets(t *testing.T) {
	t.Setenv("ONIX_VCPKG_LINUX_TRIPLET", "")
	t.Setenv("ONIX_VCPKG_WINDOWS_TRIPLET", "")
	t.Setenv("ONIX_VCPKG_MACOS_TRIPLET", "")
	tests := []struct {
		targetOS, architecture, want string
	}{
		{targetOS: "linux", architecture: "amd64", want: "x64-linux"},
		{targetOS: "linux", architecture: "arm64", want: "arm64-linux"},
		{targetOS: "windows", architecture: "amd64", want: "x64-mingw-static"},
		{targetOS: "windows", architecture: "arm64", want: "arm64-mingw-static"},
		{targetOS: "darwin", architecture: "amd64", want: "x64-osx"},
		{targetOS: "darwin", architecture: "arm64", want: "arm64-osx"},
	}
	for _, test := range tests {
		got, err := vcpkgTargetTriplet(test.targetOS, test.architecture)
		if err != nil || got != test.want {
			t.Errorf("vcpkg triplet for %s/%s = %q, %v; want %q", test.targetOS, test.architecture, got, err, test.want)
		}
	}
	t.Setenv("ONIX_VCPKG_WINDOWS_TRIPLET", "custom-windows-static")
	got, err := vcpkgTargetTriplet("windows", "amd64")
	if err != nil || got != "custom-windows-static" {
		t.Fatalf("custom Windows vcpkg triplet = %q, %v", got, err)
	}
	t.Setenv("ONIX_VCPKG_MACOS_TRIPLET", "custom-macos-static")
	got, err = vcpkgTargetTriplet("darwin", "amd64")
	if err != nil || got != "custom-macos-static" {
		t.Fatalf("custom macOS vcpkg triplet = %q, %v", got, err)
	}
}

func TestWindowsBuildEnvironmentAddsTargetVcpkgPaths(t *testing.T) {
	root := t.TempDir()
	includeDir := filepath.Join(root, "installed", "x64-mingw-static", "include")
	libraryDir := filepath.Join(root, "installed", "x64-mingw-static", "lib")
	if err := os.MkdirAll(includeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(libraryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VCPKG_ROOT", root)
	t.Setenv("ONIX_VCPKG_WINDOWS_TRIPLET", "")
	t.Setenv("CGO_CFLAGS", "")
	t.Setenv("CGO_CXXFLAGS", "")
	t.Setenv("CGO_LDFLAGS", "")
	for _, hostOS := range []string{"windows", "linux"} {
		t.Run(hostOS, func(t *testing.T) {
			env, err := windowsBuildEnvironment(hostOS, "amd64", func(name string) (string, error) { return "/toolchain/" + name, nil })
			if err != nil {
				t.Fatal(err)
			}
			values := make(map[string]string, 8)
			for _, entry := range env {
				name, value, ok := strings.Cut(entry, "=")
				if ok {
					values[name] = value
				}
			}
			if values["CGO_CFLAGS"] != "-I"+includeDir || values["CGO_CXXFLAGS"] != "-I"+includeDir {
				t.Fatalf("target vcpkg include flags = %q/%q", values["CGO_CFLAGS"], values["CGO_CXXFLAGS"])
			}
			if values["CGO_LDFLAGS"] != "-L"+libraryDir {
				t.Fatalf("target vcpkg linker flags = %q", values["CGO_LDFLAGS"])
			}
		})
	}
}

func TestWindowsBuildArgsLinksResourceFileDirectly(t *testing.T) {
	resourceFile := filepath.Join("C:\\workspace with spaces", "icon_windows.syso")
	args := windowsBuildArgs(BuildModeRelease, resourceFile)
	ldflagsIndex := slices.Index(args, "-ldflags")
	if ldflagsIndex < 0 || ldflagsIndex+1 >= len(args) {
		t.Fatalf("Windows build arguments do not contain linker flags: %v", args)
	}
	ldflags := args[ldflagsIndex+1]
	for _, want := range []string{"-s", "-w", "-H=windowsgui", "-linkmode=external", `-extldflags="C:/workspace with spaces/icon_windows.syso"`} {
		if !strings.Contains(ldflags, want) {
			t.Errorf("Windows linker flags %q do not contain %q", ldflags, want)
		}
	}
}

func TestWriteZipArchivePreservesFileModificationTime(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "bundle")
	source := filepath.Join(bundle, "resources", "base.dat")
	writeBuildTestFile(t, source, "package")
	modified := time.Date(2026, 8, 5, 12, 34, 56, 0, time.UTC)
	if err := os.Chtimes(source, modified, modified); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(root, "demo.zip")
	if err := writeZipArchive(output, bundle, "Demo"); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(output)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	for _, entry := range archive.File {
		if entry.Name == "Demo/resources/base.dat" {
			if !entry.Modified.Equal(modified) {
				t.Fatalf("ZIP file modification time = %v, want %v", entry.Modified, modified)
			}
			return
		}
	}
	t.Fatal("ZIP archive does not contain Demo/resources/base.dat")
}

func TestCopyProjectThirdPartyNotices(t *testing.T) {
	projectDir := t.TempDir()
	targetDir := filepath.Join(t.TempDir(), "bundle")
	notices := "libvpx BSD license\n"
	writeBuildTestFile(t, filepath.Join(projectDir, "THIRD_PARTY_NOTICES.md"), notices)

	if err := copyProjectThirdPartyNotices(projectDir, targetDir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(targetDir, "THIRD_PARTY_NOTICES.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != notices {
		t.Fatalf("copied notices = %q, want %q", data, notices)
	}
}

func TestCopyProjectThirdPartyNoticesAllowsMissingFile(t *testing.T) {
	targetDir := filepath.Join(t.TempDir(), "bundle")
	if err := copyProjectThirdPartyNotices(t.TempDir(), targetDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(targetDir); !os.IsNotExist(err) {
		t.Fatalf("missing notices created target directory: %v", err)
	}
}

func TestWithTemporaryBuildDirAlwaysCleans(t *testing.T) {
	for _, buildErr := range []error{nil, errors.New("failed")} {
		root := t.TempDir()
		stage := filepath.Join(root, ".stage")
		value, err := withTemporaryBuildDir(root, stage, func() (int, error) {
			writeBuildTestFile(t, filepath.Join(stage, "file"), "data")
			return 7, buildErr
		})
		if !errors.Is(err, buildErr) {
			t.Fatalf("build error = %v, want %v", err, buildErr)
		}
		if buildErr == nil && value != 7 {
			t.Fatalf("build value = %d, want 7", value)
		}
		if _, err := os.Stat(stage); !os.IsNotExist(err) {
			t.Fatalf("temporary build directory still exists: %v", err)
		}
	}
}

func TestRemoveBuildIntermediates(t *testing.T) {
	root := t.TempDir()
	names := []string{
		filepath.Join(root, ".onix-installer.wxs"),
		filepath.Join(root, ".onix-installer.wixobj"),
		filepath.Join(root, "Demo.wixpdb"),
	}
	for _, name := range names {
		writeBuildTestFile(t, name, "intermediate")
	}
	if err := removeBuildIntermediates(names...); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if _, err := os.Stat(name); !os.IsNotExist(err) {
			t.Errorf("build intermediate still exists: %s", name)
		}
	}
}
