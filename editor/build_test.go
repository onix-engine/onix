package editor

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestDefaultBuildArchitectureUsesSupportedMacOSTarget(t *testing.T) {
	options := DefaultBuildOptions("demo")
	if options.Architecture != BuildArchitectureUniversal {
		t.Fatalf("default macOS architecture = %q, want universal", options.Architecture)
	}
	if !slices.Contains(buildArchitectures(BuildPlatformMacOS), options.Architecture) {
		t.Fatalf("unsupported default macOS architecture: %q", options.Architecture)
	}
}

func TestAppendReleaseFlagsOnlyAddsFlagsToReleaseBuilds(t *testing.T) {
	debug := appendReleaseFlags([]string{"build"}, BuildModeDebug)
	if slices.Contains(debug, "-ldflags") {
		t.Fatalf("debug build unexpectedly contains ldflags: %v", debug)
	}
	release := appendReleaseFlags([]string{"build"}, BuildModeRelease)
	if !slices.Equal(release, []string{"build", "-tags", "onix_release", "-ldflags", "-s -w"}) {
		t.Fatalf("unexpected release build flags: %v", release)
	}
}

func TestAppendBuildFlagsCombinesPlatformAndReleaseTags(t *testing.T) {
	flags := appendBuildFlags([]string{"build"}, BuildModeRelease, "wechat")
	want := []string{"build", "-tags", "wechat,onix_release", "-ldflags", "-s -w"}
	if !slices.Equal(flags, want) {
		t.Fatalf("combined build flags = %v, want %v", flags, want)
	}
}

func TestWindowsBuildArgsUseGUISubsystemInEveryMode(t *testing.T) {
	for _, mode := range []BuildMode{BuildModeDebug, BuildModeRelease} {
		args := windowsBuildArgs(mode, "")
		index := slices.Index(args, "-ldflags")
		if index < 0 || index+1 >= len(args) || !strings.Contains(args[index+1], "-H=windowsgui") {
			t.Fatalf("Windows %s build does not use GUI subsystem: %v", mode, args)
		}
	}
	if release := windowsBuildArgs(BuildModeRelease, ""); !slices.Contains(release, "onix_release") {
		t.Fatalf("Windows release build is missing onix_release tag: %v", release)
	}
}

func TestLinuxBuildEnvironmentUsesVcpkgDirectories(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux build environment")
	}
	root := t.TempDir()
	var triplet string
	switch runtime.GOARCH {
	case "amd64":
		triplet = "x64-linux"
	case "arm64":
		triplet = "arm64-linux"
	default:
		t.Skip("unsupported Linux architecture")
	}
	includeDir := filepath.Join(root, "installed", triplet, "include")
	libraryDir := filepath.Join(root, "installed", triplet, "lib")
	if err := os.MkdirAll(includeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(libraryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VCPKG_ROOT", root)
	t.Setenv("CGO_CFLAGS", "-O2")
	t.Setenv("CGO_LDFLAGS", "-Wl,--gc-sections")
	env, err := linuxBuildEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	values := make(map[string]string, 5)
	for _, entry := range env {
		name, value, ok := strings.Cut(entry, "=")
		if ok {
			values[name] = value
		}
	}
	if values["CGO_CFLAGS"] != "-O2 -I"+includeDir {
		t.Fatalf("CGO_CFLAGS = %q", values["CGO_CFLAGS"])
	}
	if values["CGO_LDFLAGS"] != "-Wl,--gc-sections -L"+libraryDir {
		t.Fatalf("CGO_LDFLAGS = %q", values["CGO_LDFLAGS"])
	}
}

func TestWithResourceOverlayCleansGeneratedFiles(t *testing.T) {
	for _, test := range []struct {
		name     string
		buildErr error
	}{
		{name: "success"},
		{name: "failure", buildErr: errors.New("compile failed")},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			layout := projectBuildLayout{
				AppDir:      filepath.Join(root, "app"),
				OverlayPath: filepath.Join(root, "overlay.json"),
			}
			writeBuildTestFile(t, filepath.Join(layout.AppDir, "generated.go"), "package main")
			writeBuildTestFile(t, layout.OverlayPath, `{}`)
			value, err := layout.withResourceOverlay(func() (int, error) { return 42, test.buildErr })
			if !errors.Is(err, test.buildErr) {
				t.Fatalf("build error = %v, want %v", err, test.buildErr)
			}
			if test.buildErr == nil && value != 42 {
				t.Fatalf("build value = %d, want 42", value)
			}
			for _, name := range []string{layout.AppDir, layout.OverlayPath} {
				if _, err := os.Stat(name); !os.IsNotExist(err) {
					t.Errorf("generated path still exists after build: %s", name)
				}
			}
		})
	}
}

func TestBuildPlatformsIncludeMacOS(t *testing.T) {
	want := []BuildPlatform{
		BuildPlatformWindows,
		BuildPlatformMacOS,
		BuildPlatformLinux,
		BuildPlatformAndroid,
		BuildPlatformIOS,
		BuildPlatformWeb,
		BuildPlatformWeChat,
	}
	if !slices.Equal(buildPlatforms(), want) {
		t.Fatalf("unexpected build platforms: %v", buildPlatforms())
	}
	if buildPlatformLabel(BuildPlatformMacOS) != "macOS" {
		t.Fatalf("unexpected macOS platform label: %q", buildPlatformLabel(BuildPlatformMacOS))
	}
	for _, platform := range want {
		if buildPlatformIcon(platform) == "" {
			t.Fatalf("build platform %q has no icon", platform)
		}
	}
}

func TestDesktopBuildPackageFormats(t *testing.T) {
	tests := []struct {
		platform BuildPlatform
		want     []BuildPackageFormat
	}{
		{platform: BuildPlatformWindows, want: []BuildPackageFormat{BuildPackagePortable, BuildPackageMSI}},
		{platform: BuildPlatformMacOS, want: []BuildPackageFormat{BuildPackageApp, BuildPackageDMG}},
		{platform: BuildPlatformLinux, want: []BuildPackageFormat{BuildPackagePortable}},
		{platform: BuildPlatformWeb},
	}
	for _, test := range tests {
		if got := buildPackageFormats(test.platform); !slices.Equal(got, test.want) {
			t.Errorf("%s package formats = %v, want %v", test.platform, got, test.want)
		}
	}
	if format, err := normalizedBuildPackageFormat(BuildPlatformMacOS, ""); err != nil || format != BuildPackageApp {
		t.Fatalf("default macOS package format = %q, %v", format, err)
	}
	if _, err := normalizedBuildPackageFormat(BuildPlatformWindows, BuildPackageDMG); err == nil {
		t.Fatal("Windows accepted DMG package format")
	}
}

func TestMacOSBuildArgsApplyReleaseFlags(t *testing.T) {
	debug := macOSBuildArgs(BuildModeDebug)
	if slices.Contains(debug, "-ldflags") {
		t.Fatalf("debug macOS build unexpectedly contains ldflags: %v", debug)
	}
	release := macOSBuildArgs(BuildModeRelease)
	if !slices.Equal(release, []string{"build", "-buildvcs=false", "-trimpath", "-tags", "onix_release", "-ldflags", "-s -w"}) {
		t.Fatalf("unexpected release macOS build flags: %v", release)
	}
}

func TestMacOSBuildEnvironmentUsesSelectedArchitecture(t *testing.T) {
	tests := []struct {
		hostOS       string
		architecture BuildArchitecture
		goArch       string
		clangArch    string
		cc           string
		cxx          string
	}{
		{hostOS: "darwin", architecture: BuildArchitectureAMD64, goArch: "GOARCH=amd64", clangArch: "-arch x86_64", cc: "clang", cxx: "clang++"},
		{hostOS: "darwin", architecture: BuildArchitectureARM64, goArch: "GOARCH=arm64", clangArch: "-arch arm64", cc: "clang", cxx: "clang++"},
		{hostOS: "linux", architecture: BuildArchitectureAMD64, goArch: "GOARCH=amd64", clangArch: "-arch x86_64", cc: "o64-clang", cxx: "o64-clang++"},
		{hostOS: "linux", architecture: BuildArchitectureARM64, goArch: "GOARCH=arm64", clangArch: "-arch arm64", cc: "oa64-clang", cxx: "oa64-clang++"},
	}
	for _, test := range tests {
		t.Run(test.hostOS+"/"+string(test.architecture), func(t *testing.T) {
			lookPath := func(name string) (string, error) { return "/toolchain/" + name, nil }
			env, err := macOSBuildEnvironment(test.hostOS, test.architecture, lookPath)
			if err != nil {
				t.Fatalf("macOS build environment: %v", err)
			}
			if !slices.Contains(env, test.goArch) {
				t.Fatalf("missing %q in environment: %v", test.goArch, env)
			}
			want := map[string]string{
				"CC=":           "/toolchain/" + test.cc + " " + test.clangArch,
				"CXX=":          "/toolchain/" + test.cxx + " " + test.clangArch,
				"CGO_CFLAGS=":   test.clangArch,
				"CGO_CXXFLAGS=": test.clangArch,
				"CGO_LDFLAGS=":  test.clangArch,
			}
			for name, expected := range want {
				found := false
				for _, value := range env {
					if value == name+expected {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("missing %s%q in environment: %v", name, expected, env)
				}
			}
		})
	}
	lookPath := func(name string) (string, error) { return name, nil }
	if _, err := macOSBuildEnvironment("linux", "riscv64", lookPath); err == nil {
		t.Fatal("unsupported macOS architecture should fail")
	}
	if _, err := macOSBuildEnvironment("windows", BuildArchitectureARM64, lookPath); err == nil {
		t.Fatal("unsupported macOS build host should fail")
	}
}

func TestMacOSBuildEnvironmentReportsMissingOSXCrossCompiler(t *testing.T) {
	lookPath := func(name string) (string, error) { return "", os.ErrNotExist }
	_, err := macOSBuildEnvironment("linux", BuildArchitectureAMD64, lookPath)
	if err == nil || !strings.Contains(err.Error(), "osxcross") || !strings.Contains(err.Error(), "o64-clang") {
		t.Fatalf("unexpected missing osxcross error: %v", err)
	}
}

func TestMacOSBuildEnvironmentIncludesOSXCrossLibraries(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	libDir := filepath.Join(root, "lib")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatal(err)
	}
	lookPath := func(name string) (string, error) { return filepath.Join(binDir, name), nil }
	env, err := macOSBuildEnvironment("linux", BuildArchitectureAMD64, lookPath)
	if err != nil {
		t.Fatal(err)
	}
	want := "LD_LIBRARY_PATH=" + libDir
	if current := os.Getenv("LD_LIBRARY_PATH"); current != "" {
		want += string(os.PathListSeparator) + current
	}
	if !slices.Contains(env, want) {
		t.Fatalf("osxcross library path is missing from environment: %v", env)
	}
}

func TestMacOSBuildEnvironmentAddsTargetVcpkgPaths(t *testing.T) {
	root := t.TempDir()
	includeDir := filepath.Join(root, "installed", "x64-osx", "include")
	libraryDir := filepath.Join(root, "installed", "x64-osx", "lib")
	if err := os.MkdirAll(includeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(libraryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VCPKG_ROOT", root)
	env, err := macOSBuildEnvironment("linux", BuildArchitectureAMD64, func(name string) (string, error) { return "/toolchain/" + name, nil })
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
	if values["CGO_CFLAGS"] != "-arch x86_64 -I"+includeDir || values["CGO_CXXFLAGS"] != "-arch x86_64 -I"+includeDir {
		t.Fatalf("target macOS vcpkg include flags = %q/%q", values["CGO_CFLAGS"], values["CGO_CXXFLAGS"])
	}
	if values["CGO_LDFLAGS"] != "-arch x86_64 -L"+libraryDir {
		t.Fatalf("target macOS vcpkg linker flags = %q", values["CGO_LDFLAGS"])
	}
}

func TestAndroidEnvironmentIsDetectedWithoutShellVariables(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux Android environment layout")
	}
	home := t.TempDir()
	sdk := filepath.Join(home, "Android", "Sdk")
	ndk := filepath.Join(sdk, "ndk", "27.2.12479018")
	javaHome := filepath.Join(home, "Android", "jdk-17")
	for _, key := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT", "ANDROID_NDK_HOME", "ANDROID_NDK_ROOT", "JAVA_HOME"} {
		t.Setenv(key, "")
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", "")

	writeBuildTestFile(t, filepath.Join(sdk, "platforms", "android-35", "android.jar"), "")
	writeBuildTestFile(t, filepath.Join(sdk, "build-tools", "35.0.0", "aapt2"), "")
	writeBuildTestFile(t, filepath.Join(sdk, "build-tools", "35.0.0", "zipalign"), "")
	writeBuildTestFile(t, filepath.Join(sdk, "cmake", "3.22.1", "bin", "cmake"), "")
	writeBuildTestFile(t, filepath.Join(ndk, "source.properties"), "Pkg.Revision = 27.2.12479018\n")
	hostTag, err := androidHostTag()
	if err != nil {
		t.Fatal(err)
	}
	toolchain := filepath.Join(ndk, "toolchains", "llvm", "prebuilt", hostTag, "bin")
	writeBuildTestFile(t, filepath.Join(toolchain, "aarch64-linux-android24-clang"), "")
	writeBuildTestFile(t, filepath.Join(toolchain, "aarch64-linux-android24-clang++"), "")
	writeBuildTestFile(t, filepath.Join(javaHome, "bin", "java"), "")
	writeBuildTestFile(t, filepath.Join(javaHome, "bin", "javac"), "")
	writeBuildTestFile(t, filepath.Join(javaHome, "release"), "JAVA_VERSION=\"17.0.20\"\n")

	environment, err := resolveAndroidBuildEnvironment(home, 24, "", "")
	if err != nil {
		t.Fatalf("resolve Android build environment: %v", err)
	}
	if environment.SDKPath != sdk || environment.NDKPath != ndk || environment.JavaHome != javaHome {
		t.Fatalf("unexpected Android paths: %+v", environment)
	}
	if environment.CompileSDK != 35 || environment.BuildTools != "35.0.0" || environment.CMakeVersion != "3.22.1" || environment.NDKVersion != "27.2.12479018" {
		t.Fatalf("unexpected Android versions: %+v", environment)
	}
}

func TestResolveNDKRootSelectsNewestSemanticVersion(t *testing.T) {
	root := t.TempDir()
	for _, version := range []string{"9.0.0", "27.2.12479018", "10.0.0"} {
		writeBuildTestFile(t, filepath.Join(root, "ndk", version, "source.properties"), "Pkg.Revision = "+version+"\n")
	}
	for _, key := range []string{"ANDROID_NDK_HOME", "ANDROID_NDK_ROOT"} {
		t.Setenv(key, "")
	}
	ndk, err := resolveNDKRoot(root, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "ndk", "27.2.12479018"); ndk != want {
		t.Fatalf("NDK path = %s, want %s", ndk, want)
	}
}

func TestResolveAndroidJavaHomeRejectsOldExplicitJDK(t *testing.T) {
	home := t.TempDir()
	writeBuildTestFile(t, filepath.Join(home, "bin", androidExecutableName("java")), "")
	writeBuildTestFile(t, filepath.Join(home, "bin", androidExecutableName("javac")), "")
	writeBuildTestFile(t, filepath.Join(home, "release"), "JAVA_VERSION=\"11.0.29\"\n")
	t.Setenv("JAVA_HOME", home)
	_, err := resolveAndroidJavaHome()
	if err == nil || !strings.Contains(err.Error(), "require Java 17") {
		t.Fatalf("unexpected old JDK error: %v", err)
	}
}

func TestAndroidCommandEnvironmentOverridesSDKAndJava(t *testing.T) {
	config := androidProjectConfig{SDKPath: "/android/sdk", JavaHome: "/java/home"}
	env := androidCommandEnvironment([]string{"PATH=/bin", "ANDROID_HOME=/old/sdk", "JAVA_HOME=/old/java"}, config)
	want := map[string]string{"ANDROID_HOME": config.SDKPath, "JAVA_HOME": config.JavaHome}
	for name, value := range want {
		matches := 0
		for _, entry := range env {
			if entry == name+"="+value {
				matches++
			}
		}
		if matches != 1 {
			t.Fatalf("%s environment entries = %v", name, env)
		}
	}
}

func TestProjectBuildLayoutUsesPlatformDirectoryUnderProjectDist(t *testing.T) {
	root := t.TempDir()
	writeBuildTestFile(t, filepath.Join(root, "go.mod"), "module example.com/game\n")
	projectDir := filepath.Join(root, "apps", "demo")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	layout, err := newProjectBuildLayout(root, projectDir, BuildPlatformMacOS, "main")
	if err != nil {
		t.Fatalf("new project build layout: %v", err)
	}
	want := filepath.Join(projectDir, "dist", "macos")
	if layout.BaseDir != want {
		t.Fatalf("unexpected output directory: got %s want %s", layout.BaseDir, want)
	}
	if layout.ProjectRelPath != filepath.Join("apps", "demo") {
		t.Fatalf("project source path changed with output path: %s", layout.ProjectRelPath)
	}
	if layout.OutputRelPath != filepath.Join("apps", "demo", "dist", "macos") {
		t.Fatalf("unexpected relative output directory: %s", layout.OutputRelPath)
	}
	if !strings.HasPrefix(layout.BaseDir, filepath.Join(projectDir, "dist")+string(filepath.Separator)) {
		t.Fatalf("output directory is outside project dist: %s", layout.BaseDir)
	}
}

func TestProjectBuildLayoutResetOnlyRemovesPlatformDirectory(t *testing.T) {
	root := t.TempDir()
	writeBuildTestFile(t, filepath.Join(root, "go.mod"), "module example.com/game\n")
	projectDir := filepath.Join(root, "apps", "demo")
	layout, err := newProjectBuildLayout(root, projectDir, BuildPlatformWeb, "main")
	if err != nil {
		t.Fatal(err)
	}
	writeBuildTestFile(t, filepath.Join(layout.BaseDir, "stale.txt"), "stale")
	keep := filepath.Join(projectDir, "resources", "project.json")
	writeBuildTestFile(t, keep, `{}`)
	if err := layout.resetOutput(); err != nil {
		t.Fatalf("reset project build output: %v", err)
	}
	if _, err := os.Stat(layout.BaseDir); !os.IsNotExist(err) {
		t.Fatalf("platform output still exists after reset: %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("reset removed project source: %v", err)
	}

	layout.BaseDir = filepath.Join(projectDir, "resources")
	if err := layout.resetOutput(); err == nil {
		t.Fatal("reset accepted a directory outside project dist")
	}
}

func TestEmbeddedTemplatesApplyPlatformConfig(t *testing.T) {
	dir := t.TempDir()
	options := DefaultBuildOptions(dir)
	options.AppName = `Pocket "Onix"`
	options.WebTitle = "Pocket Onix Web"
	options.WeChatAppID = "wx-test-app"
	wechatOptions := options
	wechatOptions.Platform = BuildPlatformWeChat
	wechatOptions.Mode = BuildModeDebug
	if err := copyTemplateDir("templates/wechat", dir, wasmTemplateVars(wechatOptions, false), true); err != nil {
		t.Fatalf("copy embedded WeChat templates: %v", err)
	}
	config, err := os.ReadFile(filepath.Join(dir, "project.config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var generated struct {
		AppID       string `json:"appid"`
		ProjectName string `json:"projectname"`
	}
	if err := json.Unmarshal(config, &generated); err != nil {
		t.Fatalf("decode generated WeChat config: %v", err)
	}
	if generated.AppID != options.WeChatAppID || generated.ProjectName != options.AppName {
		t.Fatalf("unexpected generated WeChat config: %+v", generated)
	}
	if strings.Contains(string(config), "__WECHAT_") {
		t.Fatalf("generated WeChat config contains unresolved placeholders: %s", config)
	}
	webDir := t.TempDir()
	webOptions := options
	webOptions.Platform = BuildPlatformWeb
	webOptions.Mode = BuildModeDebug
	if err := copyTemplateDir("templates/web", webDir, wasmTemplateVars(webOptions, false), true); err != nil {
		t.Fatalf("copy embedded Web templates: %v", err)
	}
	manifestData, err := os.ReadFile(filepath.Join(webDir, "manifest.webmanifest"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Name      string `json:"name"`
		ShortName string `json:"short_name"`
		Icons     []struct {
			Source string `json:"src"`
		} `json:"icons"`
	}
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("decode generated Web manifest: %v", err)
	}
	if manifest.Name != options.WebTitle || manifest.ShortName != options.WebTitle || len(manifest.Icons) != 2 {
		t.Fatalf("unexpected generated Web manifest: %+v", manifest)
	}
	game, err := os.ReadFile(filepath.Join(dir, "game.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(game), "resources/catalog.bin") || !strings.Contains(string(game), "resources/main.wasm.br") || strings.Contains(string(game), "JSON.parse") {
		t.Fatalf("generated WeChat loader does not use the binary catalog")
	}
	web, err := os.ReadFile(filepath.Join(webDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(web), "resources/catalog.bin") || !strings.Contains(string(web), "resources/main.wasm") || !strings.Contains(string(web), "runtime/webgpu.js") || strings.Contains(string(web), "resources/main.wasm.br") || strings.Contains(string(web), "JSON.parse") {
		t.Fatalf("generated Web debug loader does not use the binary catalog and raw WASM")
	}
	if _, err := os.Stat(filepath.Join(webDir, "runtime", "webgpu.js")); err != nil {
		t.Fatalf("generated Web build is missing its WebGPU frame bridge: %v", err)
	}
	vectorWebDir := t.TempDir()
	if err := copyTemplateDir("templates/web", vectorWebDir, wasmTemplateVars(webOptions, true), true); err != nil {
		t.Fatalf("copy SVG Web templates: %v", err)
	}
	vectorManifestData, err := os.ReadFile(filepath.Join(vectorWebDir, "manifest.webmanifest"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(vectorManifestData), `"src": "icon.svg"`) || !strings.Contains(string(vectorManifestData), `"sizes": "any"`) {
		t.Fatalf("SVG Web manifest does not retain the vector icon: %s", vectorManifestData)
	}
	vectorIndexData, err := os.ReadFile(filepath.Join(vectorWebDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(vectorIndexData), `type="image/svg+xml" href="icon.svg"`) || !strings.Contains(string(vectorIndexData), `href="apple-touch-icon.png"`) {
		t.Fatalf("SVG Web index has incorrect icon links: %s", vectorIndexData)
	}
}

func TestWASMArtifactCompressionFollowsPlatformAndMode(t *testing.T) {
	tests := []struct {
		name       string
		platform   BuildPlatform
		mode       BuildMode
		compressed bool
		filename   string
	}{
		{name: "web debug", platform: BuildPlatformWeb, mode: BuildModeDebug, filename: wasmFilename},
		{name: "web release", platform: BuildPlatformWeb, mode: BuildModeRelease, compressed: true, filename: wasmBrotliFilename},
		{name: "wechat debug", platform: BuildPlatformWeChat, mode: BuildModeDebug, compressed: true, filename: wasmBrotliFilename},
		{name: "wechat release", platform: BuildPlatformWeChat, mode: BuildModeRelease, compressed: true, filename: wasmBrotliFilename},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := BuildOptions{Platform: test.platform, Mode: test.mode}
			if got := shouldCompressWASM(options); got != test.compressed {
				t.Fatalf("shouldCompressWASM = %v, want %v", got, test.compressed)
			}
			if got := wasmTemplateVars(options, false)["__WASM_FILENAME__"]; got != test.filename {
				t.Fatalf("WASM template filename = %q, want %q", got, test.filename)
			}
		})
	}
}

func TestSyncProjectAppPackageRequiresCanonicalEntry(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "game")
	writeBuildTestFile(t, filepath.Join(projectDir, "main.go"), "package main\nfunc OnixEntryStart(surfaceWidth, surfaceHeight int) error { return nil }\n")
	layout := projectBuildLayout{ProjectDir: projectDir, AppDir: filepath.Join(root, "dist", "game", "ios", "app"), PackageName: "game"}
	if err := syncProjectAppPackage(layout); err != nil {
		t.Fatalf("sync iOS app package: %v", err)
	}
	entry, err := os.ReadFile(filepath.Join(layout.AppDir, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(entry), "package game") || !strings.Contains(string(entry), "func OnixEntryStart") {
		t.Fatalf("unexpected staged entry:\n%s", entry)
	}
}

func TestSyncProjectAppPackageSkipsProjectDistDirectory(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "game")
	writeBuildTestFile(t, filepath.Join(projectDir, "main.go"), "package main\nfunc OnixEntryStart(surfaceWidth, surfaceHeight int) error { return nil }\n")
	writeBuildTestFile(t, filepath.Join(projectDir, "dist", "web", "stale.go"), "package stale\n")
	layout := projectBuildLayout{
		ProjectDir:  projectDir,
		AppDir:      filepath.Join(projectDir, "dist", "android", "onixapp"),
		PackageName: "game",
	}
	if err := syncProjectAppPackage(layout); err != nil {
		t.Fatalf("sync Android app package: %v", err)
	}
	if _, err := os.Stat(filepath.Join(layout.AppDir, "dist")); !os.IsNotExist(err) {
		t.Fatalf("project dist was copied into staged app: %v", err)
	}
}

func TestSyncProjectAppPackageGeneratesMobileEntryForMain(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "game")
	writeBuildTestFile(t, filepath.Join(projectDir, "main.go"), "package main\nfunc main() {}\n")
	layout := projectBuildLayout{ProjectDir: projectDir, AppDir: filepath.Join(root, "dist", "game", "android", "app"), PackageName: "game"}
	if err := syncProjectAppPackage(layout); err != nil {
		t.Fatalf("sync project with standard main: %v", err)
	}
	entry, err := os.ReadFile(filepath.Join(layout.AppDir, "onix_mobile_entry.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(entry), "func OnixEntryStart(_, _ int) error") || !strings.Contains(string(entry), "main()") {
		t.Fatalf("unexpected generated mobile entry:\n%s", entry)
	}
}

func TestSyncProjectAppPackageRejectsInvalidExplicitMobileEntry(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "game")
	writeBuildTestFile(t, filepath.Join(projectDir, "main.go"), "package main\nfunc main() {}\nfunc OnixEntryStart() {}\n")
	layout := projectBuildLayout{ProjectDir: projectDir, AppDir: filepath.Join(root, "dist", "game", "android", "app"), PackageName: "game"}
	err := syncProjectAppPackage(layout)
	if err == nil || !strings.Contains(err.Error(), "must define OnixEntryStart") {
		t.Fatalf("expected invalid mobile entry error, got %v", err)
	}
}

func writeBuildTestFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatalf("create test directory: %v", err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
}
