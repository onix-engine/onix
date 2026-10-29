package editor

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"html"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"

	"github.com/diiyw/onix/resource"
)

const buildOutputDirName = "dist"

const (
	wasmFilename       = "main.wasm"
	wasmBrotliFilename = wasmFilename + ".br"
)

const (
	defaultAndroidAPI               = 24
	defaultAndroidCompileSDK        = 35
	defaultAndroidBuildToolsVersion = "35.0.0"
	defaultAndroidCMakeVersion      = "3.22.1"
	minimumAndroidJavaVersion       = 17
	maximumAndroidJavaVersion       = 22 // Gradle 8.9's supported JVM range.
)

// Enumerated instead of all:templates so templates/android/.gradle (gradle
// build state with locked .lock files) stays out of the embed.
//
//go:embed all:templates/android/app all:templates/android/gradle all:templates/android/native
//go:embed templates/android/build.gradle templates/android/settings.gradle templates/android/gradle.properties templates/android/gradlew templates/android/gradlew.bat
//go:embed all:templates/ios all:templates/web all:templates/wechat
var buildTemplates embed.FS

type BuildPlatform string

const (
	BuildPlatformWindows BuildPlatform = "windows"
	BuildPlatformMacOS   BuildPlatform = "macos"
	BuildPlatformLinux   BuildPlatform = "linux"
	BuildPlatformAndroid BuildPlatform = "android"
	BuildPlatformIOS     BuildPlatform = "ios"
	BuildPlatformWeb     BuildPlatform = "web"
	BuildPlatformWeChat  BuildPlatform = "wechat"
)

type BuildMode string

const (
	BuildModeDebug   BuildMode = "debug"
	BuildModeRelease BuildMode = "release"
)

type BuildPackageFormat string

const (
	BuildPackagePortable BuildPackageFormat = "portable"
	BuildPackageApp      BuildPackageFormat = "app"
	BuildPackageDMG      BuildPackageFormat = "dmg"
	BuildPackageMSI      BuildPackageFormat = "msi"
)

type BuildArchitecture string

const (
	BuildArchitectureUniversal BuildArchitecture = "universal"
	BuildArchitectureAMD64     BuildArchitecture = "amd64"
	BuildArchitectureARM64     BuildArchitecture = "arm64"
)

type macOSArchitectureConfig struct {
	Architecture   BuildArchitecture
	ClangName      string
	OSXCrossPrefix string
}

type BuildOptions struct {
	ProjectDir    string
	Platform      BuildPlatform
	Mode          BuildMode
	PackageFormat BuildPackageFormat
	Architecture  BuildArchitecture
	AppName       string
	// Icon is only used when a project does not declare window.icon.
	Icon          string
	JavaPackage   string
	AndroidMinSDK int
	AndroidSDK    string
	AndroidNDK    string
	WebTitle      string
	WeChatAppID   string
	Progress      func(BuildProgress)
}

// BuildProgress reports completed work within build phases. Indeterminate
// marks work whose remaining amount the tool cannot measure.
type BuildProgress struct {
	Message       string
	Value         float32
	Indeterminate bool
}

// resourceBuildProgress maps resource work into the resource phase of a build.
func resourceBuildProgress(progress func(BuildProgress)) func(BuildProgress) {
	if progress == nil {
		return nil
	}
	return func(p BuildProgress) {
		p.Value = .1 + .55*p.Value
		progress(p)
	}
}

func DefaultBuildOptions(projectDir string) BuildOptions {
	name := filepath.Base(filepath.Clean(projectDir))
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = "Onix"
	}
	return BuildOptions{
		ProjectDir:    projectDir,
		Platform:      BuildPlatformAndroid,
		Mode:          BuildModeDebug,
		Architecture:  defaultBuildArchitecture(),
		AppName:       name,
		JavaPackage:   "dev.onix." + sanitizeJavaPackagePart(name),
		AndroidMinSDK: defaultAndroidAPI,
		WebTitle:      name,
		WeChatAppID:   "touristappid",
	}
}

func buildArchitectures(platform BuildPlatform) []BuildArchitecture {
	if platform != BuildPlatformMacOS {
		return nil
	}
	configs := macOSArchitectureConfigs()
	architectures := make([]BuildArchitecture, len(configs)+1)
	architectures[0] = BuildArchitectureUniversal
	for index, config := range configs {
		architectures[index+1] = config.Architecture
	}
	return architectures
}

func defaultBuildArchitecture() BuildArchitecture {
	return BuildArchitectureUniversal
}

func linuxBuildEnvironment() ([]string, error) {
	env := append(append([]string{}, os.Environ()...), "GOOS=linux", "GOARCH="+runtime.GOARCH, "CGO_ENABLED=1")
	return appendVcpkgEnvironment(env, "linux", runtime.GOARCH)
}

func appendVcpkgEnvironment(env []string, targetOS, architecture string) ([]string, error) {
	vcpkgRoot := strings.TrimSpace(os.Getenv("VCPKG_ROOT"))
	if vcpkgRoot == "" {
		return env, nil
	}
	triplet, err := vcpkgTargetTriplet(targetOS, architecture)
	if err != nil {
		return nil, err
	}
	includeDir := filepath.Join(vcpkgRoot, "installed", triplet, "include")
	libraryDir := filepath.Join(vcpkgRoot, "installed", triplet, "lib")
	for _, directory := range []string{includeDir, libraryDir} {
		info, err := os.Stat(directory)
		if err != nil {
			return nil, fmt.Errorf("vcpkg %s directory is unavailable: %w", directory, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("vcpkg path is not a directory: %s", directory)
		}
	}
	env = appendEnvironmentFlag(env, "CGO_CFLAGS", "-I"+includeDir)
	env = appendEnvironmentFlag(env, "CGO_CXXFLAGS", "-I"+includeDir)
	// The codec bindings use static vcpkg triplets so desktop artifacts do not
	// depend on a separately installed libvpx/libaom/libyuv runtime.
	env = appendEnvironmentFlag(env, "CGO_LDFLAGS", "-L"+libraryDir)
	return env, nil
}

func vcpkgTargetTriplet(targetOS, architecture string) (string, error) {
	envOS := strings.ToUpper(targetOS)
	if targetOS == "darwin" {
		envOS = "MACOS"
	}
	if override := strings.TrimSpace(os.Getenv("ONIX_VCPKG_" + envOS + "_TRIPLET")); override != "" {
		return override, nil
	}
	switch targetOS {
	case "linux":
		switch architecture {
		case "amd64":
			return "x64-linux", nil
		case "arm64":
			return "arm64-linux", nil
		}
	case "windows":
		// Windows CGO builds use MinGW on both native and cross hosts.
		switch architecture {
		case "amd64":
			return "x64-mingw-static", nil
		case "arm64":
			return "arm64-mingw-static", nil
		}
	case "darwin":
		switch architecture {
		case "amd64":
			return "x64-osx", nil
		case "arm64":
			return "arm64-osx", nil
		}
	}
	return "", fmt.Errorf("unsupported %s architecture %q for vcpkg", targetOS, architecture)
}

func appendEnvironmentFlag(env []string, name, flag string) []string {
	prefix := name + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return setEnvironmentValue(env, name, strings.TrimSpace(entry[len(prefix):]+" "+flag))
		}
	}
	return append(env, prefix+flag)
}

func macOSArchitectureConfigs() []macOSArchitectureConfig {
	return []macOSArchitectureConfig{
		{Architecture: BuildArchitectureAMD64, ClangName: "x86_64", OSXCrossPrefix: "o64"},
		{Architecture: BuildArchitectureARM64, ClangName: "arm64", OSXCrossPrefix: "oa64"},
	}
}

func macOSArchitectureConfigFor(architecture BuildArchitecture) (macOSArchitectureConfig, error) {
	for _, config := range macOSArchitectureConfigs() {
		if config.Architecture == architecture {
			return config, nil
		}
	}
	return macOSArchitectureConfig{}, fmt.Errorf("unsupported macOS architecture %q", architecture)
}

type androidProjectConfig struct {
	AppName         string
	JavaPackage     string
	GoPackage       string
	GoPackageName   string
	HostProjectName string
	MinSDK          int
	CompileSDK      int
	TargetSDK       int
	BuildTools      string
	NDKVersion      string
	NDKPath         string
	SDKPath         string
	CMakeVersion    string
	JavaHome        string
	AppDir          string
	HostDir         string
	NativePkgDir    string
	JavaPackagePath string
	JNIPrefix       string
}

type androidBuildEnvironment struct {
	SDKPath      string
	NDKPath      string
	JavaHome     string
	CompileSDK   int
	BuildTools   string
	CMakeVersion string
	NDKVersion   string
}

type iosProjectConfig struct {
	AppName       string
	GoPackage     string
	GoPackageName string
	AppDir        string
	HostDir       string
	NativePkgDir  string
}

type projectBuildLayout struct {
	ProjectDir        string
	ProjectImportPath string
	ProjectRelPath    string
	Platform          BuildPlatform
	OutputRelPath     string
	BaseDir           string
	AppDir            string
	AppPkg            string
	HostDir           string
	NativePkg         string
	PackageName       string
	OverlayPath       string
	ResourceDir       string
	NativeResourceDir string
}

func BuildProject(options BuildOptions) (artifact string, resultErr error) {
	defer func() {
		if resultErr == nil && options.Progress != nil {
			options.Progress(BuildProgress{Message: "Build complete: " + artifact, Value: 1})
		}
	}()
	// Build workers can leave large image and compression buffers in the heap.
	// Reclaim them before reporting either success or failure to the editor.
	defer debug.FreeOSMemory()
	if strings.TrimSpace(options.ProjectDir) == "" {
		return "", fmt.Errorf("project directory cannot be empty")
	}
	if options.Progress != nil {
		options.Progress(BuildProgress{Message: "Preparing build...", Value: 0, Indeterminate: true})
	}
	if options.Mode != BuildModeDebug && options.Mode != BuildModeRelease {
		return "", fmt.Errorf("unsupported build mode %q", options.Mode)
	}
	if _, err := GenerateModuleProject(options.ProjectDir); err != nil {
		return "", fmt.Errorf("generate module project: %w", err)
	}
	packageFormat, err := normalizedBuildPackageFormat(options.Platform, options.PackageFormat)
	if err != nil {
		return "", err
	}
	options.PackageFormat = packageFormat
	root, err := findRepoRoot(options.ProjectDir)
	if err != nil {
		return "", err
	}
	if options.Progress != nil {
		options.Progress(BuildProgress{Message: "Preparing project files...", Value: 0.08, Indeterminate: true})
	}
	switch options.Platform {
	case BuildPlatformWindows:
		return buildWindows(root, options)
	case BuildPlatformMacOS:
		return buildMacOS(root, options)
	case BuildPlatformLinux:
		return buildLinux(root, options)
	case BuildPlatformAndroid:
		return buildAndroid(root, options)
	case BuildPlatformIOS:
		return buildIOS(root, options)
	case BuildPlatformWeb, BuildPlatformWeChat:
		return buildWASM(root, options)
	default:
		return "", fmt.Errorf("unsupported build platform %q", options.Platform)
	}
}

func buildPackageFormats(platform BuildPlatform) []BuildPackageFormat {
	switch platform {
	case BuildPlatformWindows:
		return []BuildPackageFormat{BuildPackagePortable, BuildPackageMSI}
	case BuildPlatformMacOS:
		return []BuildPackageFormat{BuildPackageApp, BuildPackageDMG}
	case BuildPlatformLinux:
		return []BuildPackageFormat{BuildPackagePortable}
	default:
		return nil
	}
}

func normalizedBuildPackageFormat(platform BuildPlatform, format BuildPackageFormat) (BuildPackageFormat, error) {
	formats := buildPackageFormats(platform)
	if len(formats) == 0 {
		if format != "" {
			return "", fmt.Errorf("platform %q does not support package format %q", platform, format)
		}
		return "", nil
	}
	if format == "" {
		return formats[0], nil
	}
	for _, supported := range formats {
		if format == supported {
			return format, nil
		}
	}
	return "", fmt.Errorf("platform %q does not support package format %q", platform, format)
}

func buildWindows(root string, options BuildOptions) (string, error) {
	return buildWindowsPackage(root, options)
}

func buildMacOS(root string, options BuildOptions) (string, error) {
	return buildMacOSPackage(root, options)
}

func buildLinux(root string, options BuildOptions) (string, error) {
	return buildLinuxPackage(root, options)
}

func macOSBuildEnvironment(hostOS string, architecture BuildArchitecture, lookPath func(string) (string, error)) ([]string, error) {
	config, err := macOSArchitectureConfigFor(architecture)
	if err != nil {
		return nil, err
	}
	ccName, cxxName := "clang", "clang++"
	switch hostOS {
	case "darwin":
	case "linux":
		ccName = config.OSXCrossPrefix + "-clang"
		cxxName = config.OSXCrossPrefix + "-clang++"
	default:
		return nil, fmt.Errorf("macOS builds are unsupported on %s", hostOS)
	}
	cc, err := lookPath(ccName)
	if err != nil {
		return nil, macOSCompilerNotFoundError(hostOS, architecture, ccName)
	}
	cxx, err := lookPath(cxxName)
	if err != nil {
		return nil, macOSCompilerNotFoundError(hostOS, architecture, cxxName)
	}
	archFlag := "-arch " + config.ClangName
	env := append([]string{}, os.Environ()...)
	if hostOS == "linux" {
		libraryDir := filepath.Join(filepath.Dir(filepath.Dir(cc)), "lib")
		if info, err := os.Stat(libraryDir); err == nil && info.IsDir() {
			libraryPath := libraryDir
			if current := os.Getenv("LD_LIBRARY_PATH"); current != "" {
				libraryPath += string(os.PathListSeparator) + current
			}
			env = setEnvironmentValue(env, "LD_LIBRARY_PATH", libraryPath)
		}
	}
	env = append(env,
		"GOOS=darwin",
		"GOARCH="+string(architecture),
		"CGO_ENABLED=1",
		"CC="+cc+" "+archFlag,
		"CXX="+cxx+" "+archFlag,
		"CGO_CFLAGS="+archFlag,
		"CGO_CXXFLAGS="+archFlag,
		"CGO_LDFLAGS="+archFlag,
	)
	return appendVcpkgEnvironment(env, "darwin", string(architecture))
}

func setEnvironmentValue(env []string, name, value string) []string {
	prefix := name + "="
	result := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if !strings.HasPrefix(entry, prefix) {
			result = append(result, entry)
		}
	}
	return append(result, prefix+value)
}

func macOSCompilerNotFoundError(hostOS string, architecture BuildArchitecture, compiler string) error {
	if hostOS == "linux" {
		return fmt.Errorf("macOS %s compiler %q not found: install osxcross and add its target/bin directory to PATH", architecture, compiler)
	}
	return fmt.Errorf("macOS compiler %q not found: install the Xcode command line tools", compiler)
}

func macOSBuildArgs(mode BuildMode) []string {
	return appendReleaseFlags([]string{"build", "-buildvcs=false", "-trimpath"}, mode)
}

func windowsBuildArgs(mode BuildMode, resourceFile string) []string {
	ldflags := []string{"-H=windowsgui"}
	if mode == BuildModeRelease {
		ldflags = append([]string{"-s", "-w"}, ldflags...)
	}
	if resourceFile != "" {
		resourcePath := strings.ReplaceAll(filepath.ToSlash(resourceFile), `\`, "/")
		ldflags = append(ldflags, "-linkmode=external", "-extldflags="+strconv.Quote(resourcePath))
	}
	return appendBuildTags([]string{"build", "-buildvcs=false", "-trimpath", "-ldflags", strings.Join(ldflags, " ")}, mode)
}

func buildWASM(root string, options BuildOptions) (string, error) {
	if options.Progress != nil {
		options.Progress(BuildProgress{Message: "Preparing WebAssembly...", Value: 0.08, Indeterminate: true})
	}
	icon, err := loadBuildIcon(options)
	if err != nil {
		return "", err
	}
	layout, err := stageWASMProject(root, options.ProjectDir, options, icon.vector != nil)
	if err != nil {
		return "", err
	}
	if options.Platform == BuildPlatformWeChat {
		err = writeWeChatIconAssets(layout.BaseDir, icon)
	} else {
		err = writeWebIconAssets(layout.BaseDir, icon)
	}
	if err != nil {
		return "", err
	}
	return layout.withResourceOverlay(func() (string, error) {
		output := filepath.Join(layout.BaseDir, "resources", wasmFilename)
		if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
			return "", fmt.Errorf("create output directory: %w", err)
		}
		args := []string{"build", "-buildvcs=false", "-overlay", layout.OverlayPath}
		tags := []string(nil)
		if options.Platform == BuildPlatformWeChat {
			tags = append(tags, "wechat")
		}
		args = appendBuildFlags(args, options.Mode, tags...)
		args = append(args, "-o", output, layout.ProjectImportPath)
		env := append(append([]string{}, os.Environ()...), "GOOS=js", "GOARCH=wasm")
		if err := runCommandWithProgress(options.Progress, root, env, "go", args...); err != nil {
			return "", err
		}
		if !shouldCompressWASM(options) {
			return output, nil
		}
		reportBuildActivity(options.Progress, "Compressing WebAssembly...", .9)
		return compressWasmBrotli(output)
	})
}

func (layout projectBuildLayout) withResourceOverlay[T any](build func() (T, error)) (T, error) {
	result, buildErr := build()
	cleanupErr := cleanupResourceOverlay(layout)
	if err := errors.Join(buildErr, cleanupErr); err != nil {
		var zero T
		return zero, err
	}
	return result, nil
}

func cleanupResourceOverlay(layout projectBuildLayout) error {
	var result error
	if layout.AppDir != "" {
		if err := os.RemoveAll(layout.AppDir); err != nil {
			result = errors.Join(result, fmt.Errorf("remove generated resource source directory: %w", err))
		}
	}
	if layout.OverlayPath != "" {
		if err := os.Remove(layout.OverlayPath); err != nil && !os.IsNotExist(err) {
			result = errors.Join(result, fmt.Errorf("remove generated resource overlay: %w", err))
		}
	}
	return result
}

// DevelopmentBuild contains the generated resource package and Go overlay
// used to compile a project directly from the repository.
type DevelopmentBuild struct {
	ProjectImportPath string
	OverlayPath       string
	ExecutableDir     string
	ResourceDir       string
	layout            projectBuildLayout
}

// PrepareDevelopmentBuild preprocesses and packages a project for the
// command-line builder. The caller owns the returned build and must call
// Cleanup when the child process exits. Pass nil to disable resource progress.
func PrepareDevelopmentBuild(projectDir string, progress func(BuildProgress)) (DevelopmentBuild, error) {
	reportBuildActivity(progress, "Preparing development resources...", .08)
	root, err := findRepoRoot(projectDir)
	if err != nil {
		return DevelopmentBuild{}, err
	}
	if _, err := GenerateModuleProject(projectDir); err != nil {
		return DevelopmentBuild{}, fmt.Errorf("generate module project: %w", err)
	}
	layout, err := newCachedProjectBuildLayout(root, projectDir)
	if err != nil {
		return DevelopmentBuild{}, err
	}
	if err := layout.resetOutput(); err != nil {
		return DevelopmentBuild{}, err
	}
	if err := prepareExternalProjectResources(root, &layout, BuildModeDebug, resourceBuildProgress(progress)); err != nil {
		return DevelopmentBuild{}, err
	}
	return DevelopmentBuild{
		ProjectImportPath: layout.ProjectImportPath,
		OverlayPath:       layout.OverlayPath,
		ExecutableDir:     layout.BaseDir,
		ResourceDir:       layout.ResourceDir,
		layout:            layout,
	}, nil
}

// Cleanup removes generated development resources and the Go overlay.
func (build DevelopmentBuild) Cleanup() error {
	return cleanupResourceOverlay(build.layout)
}

func buildAndroid(root string, options BuildOptions) (string, error) {
	if options.Progress != nil {
		options.Progress(BuildProgress{Message: "Preparing Android native library...", Value: 0.08, Indeterminate: true})
	}
	androidEnvironment, err := resolveAndroidBuildEnvironment(root, options.AndroidMinSDK, options.AndroidSDK, options.AndroidNDK)
	if err != nil {
		return "", err
	}
	layout, err := stageAndroidProject(root, options.ProjectDir, options.Mode, resourceBuildProgress(options.Progress))
	if err != nil {
		return "", err
	}
	config, err := newAndroidProjectConfig(root, layout.BaseDir, options.AppName, options.JavaPackage, layout.AppPkg, layout.NativePkg, options.AndroidMinSDK, androidEnvironment)
	if err != nil {
		return "", err
	}
	if err := generateAndroidProject(config, true); err != nil {
		return "", err
	}
	icon, err := loadBuildIcon(options)
	if err != nil {
		return "", err
	}
	if err := writeAndroidIconAssets(config.HostDir, icon); err != nil {
		return "", err
	}
	if err := writeAndroidHostBuildFiles(config, true); err != nil {
		return "", err
	}
	libPath := filepath.Join(layout.BaseDir, "app", "src", "main", "jniLibs", "arm64-v8a", "libonix.so")
	if err := os.MkdirAll(filepath.Dir(libPath), 0o755); err != nil {
		return "", fmt.Errorf("create native output directory: %w", err)
	}
	cc, cxx, err := resolveAndroidToolchain(options.AndroidMinSDK, config.NDKPath)
	if err != nil {
		return "", err
	}
	env := androidCommandEnvironment(os.Environ(), config)
	env = append(env, "GOOS=android", "GOARCH=arm64", "CGO_ENABLED=1", "CC="+cc, "CXX="+cxx)
	args := []string{"build", "-buildvcs=false", "-buildmode=c-shared", "-trimpath"}
	args = appendReleaseFlags(args, options.Mode)
	args = append(args, "-o", libPath, layout.NativePkg)
	if err := runCommandWithProgress(options.Progress, root, env, "go", args...); err != nil {
		return "", err
	}
	if options.Progress != nil {
		options.Progress(BuildProgress{Message: "Packaging Android application...", Value: 0.9, Indeterminate: true})
	}
	task := "assembleDebug"
	if options.Mode == BuildModeRelease {
		task = "assembleRelease"
	}
	gradleCmd, err := resolveGradleCommand(layout.BaseDir)
	if err != nil {
		return "", err
	}
	if err := runCommandWithProgress(options.Progress, layout.BaseDir, androidCommandEnvironment(os.Environ(), config), gradleCmd[0], append(gradleCmd[1:], "--console=plain", task)...); err != nil {
		return "", err
	}
	return resolveAPKOutput(layout.BaseDir, task)
}

func buildIOS(root string, options BuildOptions) (string, error) {
	if options.Progress != nil {
		options.Progress(BuildProgress{Message: "Preparing iOS application...", Value: 0.08, Indeterminate: true})
	}
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("iOS builds require macOS")
	}
	layout, err := stageIOSProject(root, options.ProjectDir, options.Mode, resourceBuildProgress(options.Progress))
	if err != nil {
		return "", err
	}
	config, err := newIOSProjectConfig(root, layout.HostDir, options.AppName, layout.AppPkg, layout.NativePkg)
	if err != nil {
		return "", err
	}
	if err := generateIOSProject(config, true); err != nil {
		return "", err
	}
	icon, err := loadBuildIcon(options)
	if err != nil {
		return "", err
	}
	if err := writeIOSIconAssets(config.HostDir, icon); err != nil {
		return "", err
	}
	output := filepath.Join(layout.BaseDir, "libonix.a")
	args := []string{"build", "-buildvcs=false", "-buildmode=c-archive", "-trimpath"}
	args = appendReleaseFlags(args, options.Mode)
	args = append(args, "-o", output, layout.NativePkg)
	env := append(append([]string{}, os.Environ()...), "GOOS=ios", "GOARCH=arm64", "CGO_ENABLED=1")
	if err := runCommandWithProgress(options.Progress, root, env, "go", args...); err != nil {
		return "", err
	}
	return output, nil
}

func appendReleaseFlags(args []string, mode BuildMode) []string {
	return appendBuildFlags(args, mode)
}

func appendBuildFlags(args []string, mode BuildMode, tags ...string) []string {
	args = appendBuildTags(args, mode, tags...)
	if mode == BuildModeRelease {
		return append(args, "-ldflags", "-s -w")
	}
	return args
}

func appendBuildTags(args []string, mode BuildMode, tags ...string) []string {
	if mode == BuildModeRelease {
		tags = append(tags, "onix_release")
	}
	if len(tags) == 0 {
		return args
	}
	return append(args, "-tags", strings.Join(tags, ","))
}

func newAndroidProjectConfig(root, hostDir, appName, javaPkg, appPkg, nativePkg string, minSDK int, environment androidBuildEnvironment) (androidProjectConfig, error) {
	if strings.TrimSpace(appName) == "" {
		return androidProjectConfig{}, fmt.Errorf("android app name cannot be empty")
	}
	if strings.TrimSpace(javaPkg) == "" {
		return androidProjectConfig{}, fmt.Errorf("android java package cannot be empty")
	}
	if !isValidJavaPackage(javaPkg) {
		return androidProjectConfig{}, fmt.Errorf("invalid Java package %q", javaPkg)
	}
	if strings.TrimSpace(appPkg) == "" {
		return androidProjectConfig{}, fmt.Errorf("android app Go package cannot be empty")
	}
	if strings.TrimSpace(nativePkg) == "" {
		return androidProjectConfig{}, fmt.Errorf("android native Go package cannot be empty")
	}

	absoluteAppDir, appImportPath, err := resolveGoPackagePath(root, appPkg)
	if err != nil {
		return androidProjectConfig{}, err
	}
	absoluteNativeDir, _, err := resolveGoPackagePath(root, nativePkg)
	if err != nil {
		return androidProjectConfig{}, err
	}

	goPackageName := filepath.Base(appImportPath)
	if !isValidGoIdentifier(goPackageName) {
		return androidProjectConfig{}, fmt.Errorf("invalid Go package name %q derived from %q", goPackageName, appImportPath)
	}

	return androidProjectConfig{
		AppName:         appName,
		JavaPackage:     javaPkg,
		GoPackage:       appImportPath,
		GoPackageName:   goPackageName,
		HostProjectName: sanitizeGradleProjectName(appName),
		MinSDK:          minSDK,
		CompileSDK:      environment.CompileSDK,
		TargetSDK:       environment.CompileSDK,
		BuildTools:      environment.BuildTools,
		NDKVersion:      environment.NDKVersion,
		NDKPath:         environment.NDKPath,
		SDKPath:         environment.SDKPath,
		CMakeVersion:    environment.CMakeVersion,
		JavaHome:        environment.JavaHome,
		AppDir:          absoluteAppDir,
		HostDir:         hostDir,
		NativePkgDir:    absoluteNativeDir,
		JavaPackagePath: strings.ReplaceAll(javaPkg, ".", string(filepath.Separator)),
		JNIPrefix:       "Java_" + strings.NewReplacer("_", "_1", ".", "_").Replace(javaPkg) + "_NativeBridge",
	}, nil
}

func newIOSProjectConfig(root, hostDir, appName, appPkg, nativePkg string) (iosProjectConfig, error) {
	if strings.TrimSpace(appName) == "" {
		return iosProjectConfig{}, fmt.Errorf("ios app name cannot be empty")
	}
	if strings.TrimSpace(appPkg) == "" {
		return iosProjectConfig{}, fmt.Errorf("ios app Go package cannot be empty")
	}
	if strings.TrimSpace(nativePkg) == "" {
		return iosProjectConfig{}, fmt.Errorf("ios native Go package cannot be empty")
	}

	absoluteAppDir, appImportPath, err := resolveGoPackagePath(root, appPkg)
	if err != nil {
		return iosProjectConfig{}, err
	}
	absoluteNativeDir, _, err := resolveGoPackagePath(root, nativePkg)
	if err != nil {
		return iosProjectConfig{}, err
	}

	goPackageName := filepath.Base(appImportPath)
	if !isValidGoIdentifier(goPackageName) {
		return iosProjectConfig{}, fmt.Errorf("invalid Go package name %q derived from %q", goPackageName, appImportPath)
	}

	return iosProjectConfig{
		AppName:       appName,
		GoPackage:     appImportPath,
		GoPackageName: goPackageName,
		AppDir:        absoluteAppDir,
		HostDir:       hostDir,
		NativePkgDir:  absoluteNativeDir,
	}, nil
}

func generateAndroidProject(config androidProjectConfig, force bool) error {
	vars := androidTemplateVars(config)
	if err := copyTemplateDirSkipping("templates/android", config.HostDir, vars, force, map[string]struct{}{
		"native": {},
	}); err != nil {
		return err
	}
	return copyTemplateDir("templates/android/native", config.NativePkgDir, vars, true)
}

func generateIOSProject(config iosProjectConfig, force bool) error {
	vars := iosTemplateVars(config)
	if err := copyTemplateDir("templates/ios/host", config.HostDir, vars, force); err != nil {
		return err
	}
	return copyTemplateDir("templates/ios/native", config.NativePkgDir, vars, true)
}

func stageAndroidProject(root, project string, mode BuildMode, progress func(BuildProgress)) (projectBuildLayout, error) {
	layout, err := newProjectBuildLayout(root, project, BuildPlatformAndroid, "")
	if err != nil {
		return projectBuildLayout{}, err
	}
	layout.HostDir = layout.BaseDir
	layout.AppDir = filepath.Join(layout.BaseDir, "onixapp")
	layout.AppPkg = "./" + filepath.ToSlash(filepath.Join(layout.OutputRelPath, "onixapp"))
	layout.NativePkg = "./" + filepath.ToSlash(filepath.Join(layout.OutputRelPath, "native"))
	if err := layout.resetOutput(); err != nil {
		return projectBuildLayout{}, err
	}
	if err := syncProjectAppPackage(layout); err != nil {
		return projectBuildLayout{}, err
	}
	if err := prepareEmbeddedProjectResources(root, &layout, mode, progress); err != nil {
		return projectBuildLayout{}, err
	}
	return layout, nil
}

func stageIOSProject(root, project string, mode BuildMode, progress func(BuildProgress)) (projectBuildLayout, error) {
	layout, err := newProjectBuildLayout(root, project, BuildPlatformIOS, "")
	if err != nil {
		return projectBuildLayout{}, err
	}
	layout.AppDir = filepath.Join(layout.BaseDir, "app")
	layout.AppPkg = "./" + filepath.ToSlash(filepath.Join(layout.OutputRelPath, "app"))
	layout.HostDir = filepath.Join(layout.BaseDir, "host")
	layout.NativePkg = "./" + filepath.ToSlash(filepath.Join(layout.OutputRelPath, "native"))
	if err := layout.resetOutput(); err != nil {
		return projectBuildLayout{}, err
	}
	if err := syncProjectAppPackage(layout); err != nil {
		return projectBuildLayout{}, err
	}
	if err := prepareEmbeddedProjectResources(root, &layout, mode, progress); err != nil {
		return projectBuildLayout{}, err
	}
	return layout, nil
}

func stageWASMProject(root, project string, options BuildOptions, vectorIcon bool) (projectBuildLayout, error) {
	platform := options.Platform
	layout, err := newProjectBuildLayout(root, project, platform, "main")
	if err != nil {
		return projectBuildLayout{}, err
	}
	if err := layout.resetOutput(); err != nil {
		return projectBuildLayout{}, err
	}
	vars := wasmTemplateVars(options, vectorIcon && platform == BuildPlatformWeb)
	if err := copyTemplateDir("templates/"+string(platform), layout.BaseDir, vars, true); err != nil {
		return projectBuildLayout{}, err
	}
	if err := prepareExternalProjectResources(root, &layout, options.Mode, resourceBuildProgress(options.Progress)); err != nil {
		return projectBuildLayout{}, err
	}
	return layout, nil
}

func generateProjectResourceOverlay(root string, layout *projectBuildLayout, catalog resource.Catalog) error {
	packageName, err := projectGoPackageName(layout.ProjectDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(layout.AppDir, 0o755); err != nil {
		return fmt.Errorf("create resource overlay directory: %w", err)
	}
	sources := generatedResourceSources(packageName, "github.com/diiyw/onix/resource", layout.ProjectRelPath, layout.NativeResourceDir, catalog)
	replacements := make(map[string]string, len(sources))
	for name, source := range sources {
		generatedPath := filepath.Join(layout.AppDir, name)
		if err := os.WriteFile(generatedPath, []byte(source), 0o644); err != nil {
			return fmt.Errorf("write project resource source %s: %w", name, err)
		}
		virtualPath := filepath.Join(layout.ProjectDir, name)
		if _, err := os.Stat(virtualPath); err == nil {
			return fmt.Errorf("reserved generated file already exists: %s", virtualPath)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat reserved generated file: %w", err)
		}
		replacements[virtualPath] = generatedPath
	}
	overlay := struct {
		Replace map[string]string
	}{Replace: replacements}
	overlayData, err := json.MarshalIndent(overlay, "", "  ")
	if err != nil {
		return fmt.Errorf("encode resource build overlay: %w", err)
	}
	layout.OverlayPath = filepath.Join(layout.BaseDir, "overlay.json")
	if err := os.WriteFile(layout.OverlayPath, overlayData, 0o644); err != nil {
		return fmt.Errorf("write resource build overlay: %w", err)
	}
	return nil
}

func prepareExternalProjectResources(root string, layout *projectBuildLayout, mode BuildMode, progress func(BuildProgress)) (resultErr error) {
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, cleanupResourceOverlay(*layout))
		}
	}()
	catalog, _, err := writeModuleResourcePackages(root, layout.ProjectDir, layout.ResourceDir, resourcePackageOptions{Mode: mode, Platform: layout.Platform, Progress: progress})
	if err != nil {
		return err
	}
	return generateProjectResourceOverlay(root, layout, catalog)
}

func prepareEmbeddedProjectResources(root string, layout *projectBuildLayout, mode BuildMode, progress func(BuildProgress)) error {
	catalog, _, err := writeModuleResourcePackages(root, layout.ProjectDir, filepath.Join(layout.AppDir, "zz_generated_resources"), resourcePackageOptions{Mode: mode, Platform: layout.Platform, Progress: progress})
	if err != nil {
		return err
	}
	for name, source := range generatedResourceSources(layout.PackageName, "github.com/diiyw/onix/resource", layout.ProjectRelPath, layout.NativeResourceDir, catalog) {
		if err := os.WriteFile(filepath.Join(layout.AppDir, name), []byte(source), 0o644); err != nil {
			return fmt.Errorf("write embedded project resource source %s: %w", name, err)
		}
	}
	return nil
}

func generatedResourceSources(packageName, resourceImportPath, projectRoot, nativeResourceDir string, catalog resource.Catalog) map[string]string {
	quotedImport := strconv.Quote(resourceImportPath)
	quotedRoot := strconv.Quote(filepath.ToSlash(projectRoot))
	quotedNativeResourceDir := strconv.Quote(filepath.ToSlash(nativeResourceDir))
	var mobileEmbeds, mobilePackages strings.Builder
	for index, info := range catalog.Packages {
		variable := fmt.Sprintf("onixPackageData%d", index)
		fmt.Fprintf(&mobileEmbeds, "//go:embed zz_generated_resources/%s\nvar %s []byte\n\n", info.File, variable)
		fmt.Fprintf(&mobilePackages, "\t\t%s: %s,\n", strconv.Quote(info.File), variable)
	}
	return map[string]string{
		"zz_generated_onix_resources_js.go": fmt.Sprintf(`// Code generated by Onix Editor; DO NOT EDIT.
//go:build js

package %s

import (
	"syscall/js"

	onixresource %s
)

func init() {
	catalogData := onixResourceBytes(js.Global().Get("__onixCatalogData"))
	js.Global().Delete("__onixCatalogData")
	values := js.Global().Get("__onixPackageData")
	if values.IsUndefined() || values.IsNull() {
		panic("Onix resource packages were not preloaded")
	}
	keys := js.Global().Get("Object").Call("keys", values)
	packages := make(map[string][]byte, keys.Length())
	for index := 0; index < keys.Length(); index++ {
		name := keys.Index(index).String()
		packages[name] = onixResourceBytes(values.Get(name))
		values.Delete(name)
	}
	js.Global().Delete("__onixPackageData")
	source, err := onixresource.NewMemoryManager(catalogData, packages)
	if err != nil {
		panic(err)
	}
	if err := onixresource.SetDefaultProject(source, %s); err != nil {
		panic(err)
	}
}

func onixResourceBytes(value js.Value) []byte {
	if value.IsUndefined() || value.IsNull() {
		panic("Onix resource data was not preloaded")
	}
	data := make([]byte, value.Get("byteLength").Int())
	js.CopyBytesToGo(data, value)
	return data
}
`, packageName, quotedImport, quotedRoot),
		"zz_generated_onix_resources_native.go": fmt.Sprintf(`// Code generated by Onix Editor; DO NOT EDIT.
//go:build !js && !android && !ios

package %s

import (
	"os"
	"path/filepath"

	onixresource %s
)

func init() {
	executable, err := os.Executable()
	if err != nil {
		panic(err)
	}
	resourceDir := os.Getenv("ONIX_RESOURCE_DIR")
	if resourceDir == "" {
		resourceDir = filepath.Clean(filepath.Join(filepath.Dir(executable), filepath.FromSlash(%s)))
	}
	source, err := onixresource.OpenManager(resourceDir)
	if err != nil {
		panic(err)
	}
	if err := onixresource.SetDefaultProject(source, %s); err != nil {
		panic(err)
	}
}
`, packageName, quotedImport, quotedNativeResourceDir, quotedRoot),
		"zz_generated_onix_resources_mobile.go": fmt.Sprintf(`// Code generated by Onix Editor; DO NOT EDIT.
//go:build android || ios

package %s

import (
	_ "embed"

	onixresource %s
)

//go:embed zz_generated_resources/catalog.bin
var onixCatalogData []byte

%s

func init() {
	source, err := onixresource.NewMemoryManager(onixCatalogData, map[string][]byte{
%s	})
	if err != nil {
		panic(err)
	}
	if err := onixresource.SetDefaultProject(source, %s); err != nil {
		panic(err)
	}
}
`, packageName, quotedImport, mobileEmbeds.String(), mobilePackages.String(), quotedRoot),
	}
}

func newProjectBuildLayout(root, project string, platform BuildPlatform, packageName string) (projectBuildLayout, error) {
	layout, err := newProjectBuildLayoutBase(root, project, packageName)
	if err != nil {
		return projectBuildLayout{}, err
	}
	layout.Platform = platform
	return layout.withProjectOutput(string(platform)), nil
}

func newCachedProjectBuildLayout(root, project string) (projectBuildLayout, error) {
	layout, err := newProjectBuildLayoutBase(root, project, "")
	if err != nil {
		return projectBuildLayout{}, err
	}
	layout.Platform = hostBuildPlatform(runtime.GOOS)
	return layout.withProjectOutput("cache"), nil
}

func hostBuildPlatform(goos string) BuildPlatform {
	switch goos {
	case "windows":
		return BuildPlatformWindows
	case "darwin":
		return BuildPlatformMacOS
	case "linux":
		return BuildPlatformLinux
	default:
		return ""
	}
}

func newProjectBuildLayoutBase(root, project, packageName string) (projectBuildLayout, error) {
	projectDir, projectImportPath, err := resolveGoPackagePath(root, project)
	if err != nil {
		return projectBuildLayout{}, err
	}

	projectRelPath, err := filepath.Rel(root, projectDir)
	if err != nil {
		return projectBuildLayout{}, fmt.Errorf("resolve project path %q: %w", project, err)
	}
	projectRelPath = filepath.Clean(projectRelPath)
	if strings.HasPrefix(filepath.ToSlash(projectRelPath), "../") {
		return projectBuildLayout{}, fmt.Errorf("project path %q must stay within repository root", project)
	}

	if packageName == "" {
		packageName = sanitizeGoPackageName(filepath.Base(projectDir))
	}
	return projectBuildLayout{
		ProjectDir:        projectDir,
		ProjectImportPath: projectImportPath,
		ProjectRelPath:    projectRelPath,
		PackageName:       packageName,
	}, nil
}

func (layout projectBuildLayout) withProjectOutput(path ...string) projectBuildLayout {
	outputPath := filepath.Join(append([]string{buildOutputDirName}, path...)...)
	stageRelPath := filepath.Join(layout.ProjectRelPath, outputPath)
	appPkg := "./" + filepath.ToSlash(filepath.Join(stageRelPath, "app"))
	nativePkg := "./" + filepath.ToSlash(filepath.Join(stageRelPath, "native"))
	layout.OutputRelPath = stageRelPath
	layout.BaseDir = filepath.Join(layout.ProjectDir, outputPath)
	layout.AppDir = filepath.Join(layout.BaseDir, "app")
	layout.AppPkg = appPkg
	layout.HostDir = filepath.Join(layout.BaseDir, "host")
	layout.NativePkg = nativePkg
	layout.ResourceDir = filepath.Join(layout.BaseDir, "resources")
	layout.NativeResourceDir = "resources"
	return layout
}

func sanitizeGoPackageName(s string) string {
	if s == "" {
		return "app"
	}

	var b strings.Builder
	for i, r := range s {
		valid := r == '_' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z'
		if !valid {
			b.WriteByte('_')
			continue
		}
		if i == 0 && r >= '0' && r <= '9' {
			b.WriteByte('_')
		}
		b.WriteRune(r)
	}

	result := b.String()
	if !isValidGoIdentifier(result) {
		return "app"
	}
	return result
}

func projectGoPackageName(projectDir string) (string, error) {
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return "", fmt.Errorf("read project Go package %s: %w", projectDir, err)
	}
	packageName := ""
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		name := filepath.Join(projectDir, entry.Name())
		parsed, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.PackageClauseOnly)
		if err != nil {
			return "", fmt.Errorf("parse project package %s: %w", name, err)
		}
		if packageName == "" {
			packageName = parsed.Name.Name
			continue
		}
		if packageName != parsed.Name.Name {
			return "", fmt.Errorf("project %s contains packages %q and %q", projectDir, packageName, parsed.Name.Name)
		}
	}
	if packageName == "" {
		return "", fmt.Errorf("project %s does not contain any Go source files", projectDir)
	}
	return packageName, nil
}

func (layout projectBuildLayout) resetOutput() error {
	distDir := filepath.Join(layout.ProjectDir, buildOutputDirName)
	absoluteStageDir, err := filepath.Abs(layout.BaseDir)
	if err != nil {
		return fmt.Errorf("resolve stage directory %q: %w", layout.BaseDir, err)
	}
	absoluteDistRoot, err := filepath.Abs(distDir)
	if err != nil {
		return fmt.Errorf("resolve dist directory %q: %w", distDir, err)
	}
	rel, err := filepath.Rel(absoluteDistRoot, absoluteStageDir)
	if err != nil {
		return fmt.Errorf("resolve generated stage %q: %w", layout.BaseDir, err)
	}
	if rel == "." || strings.HasPrefix(filepath.ToSlash(rel), "../") {
		return fmt.Errorf("refusing to reset stage outside %s: %s", absoluteDistRoot, absoluteStageDir)
	}
	if err := os.RemoveAll(absoluteStageDir); err != nil {
		return fmt.Errorf("reset generated stage %s: %w", absoluteStageDir, err)
	}
	return nil
}

func syncProjectAppPackage(layout projectBuildLayout) error {
	if err := os.MkdirAll(layout.AppDir, 0o755); err != nil {
		return fmt.Errorf("create staged app directory %s: %w", layout.AppDir, err)
	}

	var hasGoFiles bool
	var hasEntryStart bool
	var entryStartDeclared bool
	var hasMain bool

	err := filepath.WalkDir(layout.ProjectDir, func(srcPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk project path %s: %w", srcPath, walkErr)
		}
		if srcPath == layout.ProjectDir {
			return nil
		}

		relPath, err := filepath.Rel(layout.ProjectDir, srcPath)
		if err != nil {
			return fmt.Errorf("resolve project relative path %s: %w", srcPath, err)
		}
		if d.IsDir() && relPath == buildOutputDirName {
			return filepath.SkipDir
		}
		dstPath := filepath.Join(layout.AppDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(dstPath, 0o755)
		}

		if strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		if d.Name() == "go.mod" || d.Name() == "go.sum" || strings.Contains(filepath.ToSlash(relPath), "/.editor-trash/") {
			return nil
		}

		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return fmt.Errorf("create staged directory %s: %w", filepath.Dir(dstPath), err)
		}

		if filepath.Dir(srcPath) == layout.ProjectDir && strings.HasSuffix(d.Name(), ".go") {
			hasGoFiles = true

			data, meta, err := rewriteProjectGoFile(srcPath, layout.PackageName)
			if err != nil {
				return err
			}
			hasEntryStart = hasEntryStart || meta.hasEntryStart
			entryStartDeclared = entryStartDeclared || meta.entryStartDeclared
			hasMain = hasMain || meta.hasMain
			if err := os.WriteFile(dstPath, data, 0o644); err != nil {
				return fmt.Errorf("write staged Go file %s: %w", dstPath, err)
			}
			return nil
		}

		data, err := os.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("read project file %s: %w", srcPath, err)
		}
		if err := os.WriteFile(dstPath, data, templateFileMode(srcPath)); err != nil {
			return fmt.Errorf("write staged project file %s: %w", dstPath, err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	if !hasGoFiles {
		return fmt.Errorf("project %s does not contain any Go source files", layout.ProjectDir)
	}
	if entryStartDeclared && !hasEntryStart {
		return fmt.Errorf("project %s must define OnixEntryStart(surfaceWidth, surfaceHeight int) error", layout.ProjectDir)
	}
	if hasEntryStart {
		return nil
	}
	if !hasMain {
		return fmt.Errorf("project %s must define main() or OnixEntryStart(surfaceWidth, surfaceHeight int) error", layout.ProjectDir)
	}
	return writeGeneratedMobileEntry(layout.AppDir, layout.PackageName)
}

type projectGoFileMeta struct {
	hasEntryStart      bool
	entryStartDeclared bool
	hasMain            bool
}

func rewriteProjectGoFile(path, packageName string) ([]byte, projectGoFileMeta, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, projectGoFileMeta{}, fmt.Errorf("parse Go file %s: %w", path, err)
	}

	meta := projectGoFileMeta{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil {
			continue
		}
		if fn.Name.Name == "OnixEntryStart" {
			meta.entryStartDeclared = true
			meta.hasEntryStart = isProjectEntryStartSignature(fn)
		}
		if fn.Name.Name == "main" {
			meta.hasMain = isProjectMainSignature(fn)
		}
	}

	file.Name = ast.NewIdent(packageName)

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		return nil, projectGoFileMeta{}, fmt.Errorf("format staged Go file %s: %w", path, err)
	}
	return buf.Bytes(), meta, nil
}

func writeGeneratedMobileEntry(directory, packageName string) error {
	source := fmt.Sprintf(`// Code generated by Onix Editor; DO NOT EDIT.

package %s

func OnixEntryStart(_, _ int) error {
	main()
	return nil
}
`, packageName)
	formatted, err := format.Source([]byte(source))
	if err != nil {
		return fmt.Errorf("format generated mobile entry: %w", err)
	}
	path := filepath.Join(directory, "onix_mobile_entry.go")
	if err := os.WriteFile(path, formatted, 0o644); err != nil {
		return fmt.Errorf("write generated mobile entry %s: %w", path, err)
	}
	return nil
}

func isProjectMainSignature(fn *ast.FuncDecl) bool {
	return fn.Type.Params != nil && len(fn.Type.Params.List) == 0 && fn.Type.Results == nil
}

func isProjectEntryStartSignature(fn *ast.FuncDecl) bool {
	if fn.Type.Params == nil || len(fn.Type.Params.List) != 1 || len(fn.Type.Params.List[0].Names) != 2 {
		return false
	}
	paramsType, ok := fn.Type.Params.List[0].Type.(*ast.Ident)
	if !ok || paramsType.Name != "int" {
		return false
	}
	if fn.Type.Results == nil || len(fn.Type.Results.List) != 1 {
		return false
	}
	resultType, ok := fn.Type.Results.List[0].Type.(*ast.Ident)
	return ok && resultType.Name == "error"
}

func wasmTemplateVars(options BuildOptions, vectorIcon bool) map[string]string {
	vars := map[string]string{
		"__WEB_FAVICON_TYPE__":     "image/png",
		"__WEB_FAVICON__":          "favicon.png",
		"__WEB_APPLE_TOUCH_ICON__": "icon-192.png",
		"__WEB_MANIFEST_ICONS__":   `[{ "src": "icon-192.png", "sizes": "192x192", "type": "image/png" }, { "src": "icon-512.png", "sizes": "512x512", "type": "image/png" }]`,
		"__WEB_TITLE__":            html.EscapeString(options.WebTitle),
		"__WEB_TITLE_JSON__":       jsonStringContent(options.WebTitle),
		"__WECHAT_APP_ID__":        jsonStringContent(options.WeChatAppID),
		"__WECHAT_PROJECT_NAME__":  jsonStringContent(options.AppName),
		"__WASM_FILENAME__":        wasmArtifactFilename(options),
	}
	if vectorIcon {
		vars["__WEB_FAVICON_TYPE__"] = "image/svg+xml"
		vars["__WEB_FAVICON__"] = "icon.svg"
		vars["__WEB_APPLE_TOUCH_ICON__"] = "apple-touch-icon.png"
		vars["__WEB_MANIFEST_ICONS__"] = `[{ "src": "icon.svg", "sizes": "any", "type": "image/svg+xml" }]`
	}
	return vars
}

func shouldCompressWASM(options BuildOptions) bool {
	switch options.Platform {
	case BuildPlatformWeb:
		return options.Mode == BuildModeRelease
	case BuildPlatformWeChat:
		return true
	default:
		return false
	}
}

func wasmArtifactFilename(options BuildOptions) string {
	if shouldCompressWASM(options) {
		return wasmBrotliFilename
	}
	return wasmFilename
}

func androidTemplateVars(config androidProjectConfig) map[string]string {
	return map[string]string{
		"__APP_NAME__":                 html.EscapeString(config.AppName),
		"__ANDROID_BUILD_TOOLS__":      config.BuildTools,
		"__ANDROID_CMAKE_VERSION__":    config.CMakeVersion,
		"__ANDROID_COMPILE_SDK__":      strconv.Itoa(config.CompileSDK),
		"__JAVA_PACKAGE__":             config.JavaPackage,
		"__JAVA_PACKAGE_PATH__":        config.JavaPackagePath,
		"__APP_IMPORT_PATH__":          config.GoPackage,
		"__GO_PACKAGE_NAME__":          config.GoPackageName,
		"__HOST_PROJECT_NAME__":        config.HostProjectName,
		"__ANDROID_MIN_SDK__":          strconv.Itoa(config.MinSDK),
		"__ANDROID_TARGET_SDK__":       strconv.Itoa(config.TargetSDK),
		"__JNI_NATIVE_BRIDGE_PREFIX__": config.JNIPrefix,
	}
}

func jsonStringContent(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded[1 : len(encoded)-1])
}

func iosTemplateVars(config iosProjectConfig) map[string]string {
	return map[string]string{
		"__APP_NAME__":        config.AppName,
		"__APP_IMPORT_PATH__": config.GoPackage,
		"__GO_PACKAGE_NAME__": config.GoPackageName,
	}
}

func copyTemplateDir(srcDir, dstDir string, vars map[string]string, force bool) error {
	return copyTemplateDirSkipping(srcDir, dstDir, vars, force, nil)
}

func copyTemplateDirSkipping(srcDir, dstDir string, vars map[string]string, force bool, skipTopLevel map[string]struct{}) error {
	templates, err := fs.Sub(buildTemplates, srcDir)
	if err != nil {
		return fmt.Errorf("open embedded template dir %s: %w", srcDir, err)
	}
	entries := make([]string, 0, 32)
	err = fs.WalkDir(templates, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if len(skipTopLevel) != 0 && path != "." {
			topLevel := strings.Split(filepath.ToSlash(path), "/")[0]
			if _, ok := skipTopLevel[topLevel]; ok {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		entries = append(entries, path)
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk template dir %s: %w", srcDir, err)
	}
	sort.Strings(entries)

	for _, srcPath := range entries {
		rel := applyTemplateVars(srcPath, vars)
		if rel == "." {
			if err := os.MkdirAll(dstDir, 0o755); err != nil {
				return fmt.Errorf("create directory %s: %w", dstDir, err)
			}
			continue
		}

		dstPath := filepath.Join(dstDir, rel)
		info, err := fs.Stat(templates, srcPath)
		if err != nil {
			return fmt.Errorf("stat template path %s: %w", srcPath, err)
		}

		if info.IsDir() {
			if err := os.MkdirAll(dstPath, 0o755); err != nil {
				return fmt.Errorf("create directory %s: %w", dstPath, err)
			}
			continue
		}

		if !force {
			if _, err := os.Stat(dstPath); err == nil {
				continue
			}
		}

		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", filepath.Dir(dstPath), err)
		}

		data, err := fs.ReadFile(templates, srcPath)
		if err != nil {
			return fmt.Errorf("read template file %s: %w", srcPath, err)
		}
		content := applyTemplateVars(string(data), vars)
		if err := os.WriteFile(dstPath, []byte(content), templateFileMode(srcPath)); err != nil {
			return fmt.Errorf("write generated file %s: %w", dstPath, err)
		}
	}

	return nil
}

func templateFileMode(path string) os.FileMode {
	if filepath.Base(path) == "gradlew" || strings.HasSuffix(path, ".sh") {
		return 0o755
	}
	return 0o644
}

func applyTemplateVars(s string, vars map[string]string) string {
	for old, newValue := range vars {
		s = strings.ReplaceAll(s, old, newValue)
	}
	return s
}

func resolveGoPackagePath(root, pkg string) (string, string, error) {
	modulePath, err := modulePath(root)
	if err != nil {
		return "", "", err
	}

	pkg = filepath.Clean(pkg)
	if filepath.IsAbs(pkg) {
		rel, err := filepath.Rel(root, pkg)
		if err != nil {
			return "", "", fmt.Errorf("resolve Go package path %q: %w", pkg, err)
		}
		pkg = rel
	}

	cleanSlash := filepath.ToSlash(pkg)
	if cleanSlash == "." {
		return root, modulePath, nil
	}
	if strings.HasPrefix(cleanSlash, "../") {
		return "", "", fmt.Errorf("go package path %q escapes repository root", pkg)
	}
	if strings.Contains(cleanSlash, "//") {
		cleanSlash = strings.ReplaceAll(cleanSlash, "//", "/")
	}
	cleanSlash = strings.TrimPrefix(cleanSlash, "./")

	abs := filepath.Join(root, filepath.FromSlash(cleanSlash))
	importPath := modulePath + "/" + strings.TrimPrefix(cleanSlash, "/")
	importPath = strings.TrimSuffix(importPath, "/")
	return abs, importPath, nil
}

func modulePath(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	return "", fmt.Errorf("module path not found in go.mod")
}

func sanitizeGradleProjectName(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		isAlnum := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if isAlnum {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		return "onix-android"
	}
	return result
}

func sanitizeJavaPackagePart(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	result := strings.Trim(b.String(), "_")
	if result == "" || result[0] >= '0' && result[0] <= '9' {
		return "app"
	}
	return result
}

func isValidGoIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 {
			if !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z') {
				return false
			}
			continue
		}
		if !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func isValidJavaPackage(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) == 0 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for i, r := range part {
			if i == 0 {
				if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
					return false
				}
				continue
			}
			if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
				return false
			}
		}
	}
	return true
}

func writeAndroidHostBuildFiles(config androidProjectConfig, strict bool) error {
	if err := writeAndroidLocalProperties(config, strict); err != nil {
		return err
	}
	if err := writeAndroidOnixLocalProperties(config); err != nil {
		return err
	}
	return nil
}

func writeAndroidLocalProperties(config androidProjectConfig, strict bool) error {
	if config.SDKPath == "" {
		if strict {
			return fmt.Errorf("android SDK not found: configure Android SDK in Build, or set ANDROID_HOME / ANDROID_SDK_ROOT")
		}
		return nil
	}

	path := filepath.Join(config.HostDir, "local.properties")
	content := "sdk.dir=" + escapeJavaPropertiesPath(config.SDKPath) + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func writeAndroidOnixLocalProperties(config androidProjectConfig) error {
	lines := []string{
		"# Generated by Onix Editor; machine-specific Android build settings.",
		"onix.android.compileSdk=" + strconv.Itoa(config.CompileSDK),
		"onix.android.targetSdk=" + strconv.Itoa(config.TargetSDK),
		"onix.android.buildToolsVersion=" + config.BuildTools,
		"onix.android.cmakeVersion=" + config.CMakeVersion,
	}
	if config.NDKVersion != "" {
		lines = append(lines, "onix.android.ndkVersion="+config.NDKVersion)
	}
	if config.NDKPath != "" {
		lines = append(lines, "onix.android.ndkPath="+filepath.ToSlash(config.NDKPath))
	}

	path := filepath.Join(config.HostDir, "onix.local.properties")
	data := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func escapeJavaPropertiesPath(path string) string {
	replacer := strings.NewReplacer(
		`\\`, `\\\\`,
		`\`, `\\`,
		":", `\:`,
		"=", `\=`,
	)
	return replacer.Replace(path)
}

func resolveAndroidSDKRoot(root, override string) (string, error) {
	if strings.TrimSpace(override) != "" {
		return resolveOverrideDir(root, override, "Android SDK")
	}
	for _, key := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return resolveOverrideDir(root, value, key)
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory for Android SDK: %w", err)
	}
	for _, candidate := range defaultAndroidSDKPaths(runtime.GOOS, home, os.Getenv("LOCALAPPDATA")) {
		if directory := existingDirectory(candidate); directory != "" {
			return directory, nil
		}
	}
	for _, tool := range []string{"sdkmanager", "android"} {
		path, err := exec.LookPath(tool)
		if err != nil {
			continue
		}
		if sdkRoot := androidSDKRootFromTool(path); sdkRoot != "" {
			return sdkRoot, nil
		}
	}
	return "", fmt.Errorf("android SDK not found: install it in the platform default location, configure Android SDK in Build, or set ANDROID_HOME")
}

func defaultAndroidSDKPaths(goos, home, localAppData string) []string {
	switch goos {
	case "darwin":
		return []string{filepath.Join(home, "Library", "Android", "sdk")}
	case "windows":
		return []string{filepath.Join(localAppData, "Android", "Sdk")}
	default:
		return []string{
			filepath.Join(home, "Android", "Sdk"),
			filepath.Join(home, "Android", "sdk"),
			"/opt/android-sdk",
			"/usr/lib/android-sdk",
			"/usr/local/lib/android/sdk",
		}
	}
}

func androidSDKRootFromTool(tool string) string {
	path, err := filepath.Abs(tool)
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	for directory := filepath.Dir(path); ; directory = filepath.Dir(directory) {
		if isAndroidSDKRoot(directory) {
			return filepath.Clean(directory)
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return ""
		}
	}
}

func isAndroidSDKRoot(directory string) bool {
	for _, name := range []string{"platforms", "build-tools", "cmdline-tools", "platform-tools"} {
		if info, err := os.Stat(filepath.Join(directory, name)); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

func resolveAndroidCompileSDK(sdkRoot string, preferred int) (int, error) {
	platformsDir := filepath.Join(sdkRoot, "platforms")
	preferredDir := filepath.Join(platformsDir, fmt.Sprintf("android-%d", preferred))
	if isRegularFile(filepath.Join(preferredDir, "android.jar")) {
		return preferred, nil
	}

	entries, err := os.ReadDir(platformsDir)
	if err != nil {
		return 0, fmt.Errorf("read Android platforms: %w", err)
	}

	best := 0
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "android-") {
			continue
		}
		version, ok := parseAndroidPlatformVersion(strings.TrimPrefix(entry.Name(), "android-"))
		if !isRegularFile(filepath.Join(platformsDir, entry.Name(), "android.jar")) {
			continue
		}
		if ok && version > best {
			best = version
		}
	}
	if best == 0 {
		return 0, fmt.Errorf("no Android platforms found under %s", platformsDir)
	}
	return best, nil
}

func resolveAndroidBuildToolsVersion(sdkRoot, preferred string) (string, error) {
	return resolveAndroidPackageVersion(filepath.Join(sdkRoot, "build-tools"), preferred, "Android Build Tools", func(directory string) bool {
		return isRegularFile(filepath.Join(directory, androidExecutableName("aapt2"))) &&
			isRegularFile(filepath.Join(directory, androidExecutableName("zipalign")))
	})
}

func parseAndroidPlatformVersion(s string) (int, bool) {
	var digits strings.Builder
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		digits.WriteRune(r)
	}
	if digits.Len() == 0 {
		return 0, false
	}
	value, err := strconv.Atoi(digits.String())
	return value, err == nil
}

func resolveAndroidCMakeVersion(sdkRoot, preferred string) (string, error) {
	return resolveAndroidPackageVersion(filepath.Join(sdkRoot, "cmake"), preferred, "Android CMake", func(directory string) bool {
		return isRegularFile(filepath.Join(directory, "bin", androidExecutableName("cmake")))
	})
}

func resolveAndroidPackageVersion(directory, preferred, label string, valid func(string) bool) (string, error) {
	preferredDir := filepath.Join(directory, preferred)
	if preferred != "" && valid(preferredDir) {
		return preferred, nil
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return "", fmt.Errorf("read %s directory: %w", label, err)
	}
	best := ""
	for _, entry := range entries {
		if !entry.IsDir() || !valid(filepath.Join(directory, entry.Name())) {
			continue
		}
		if best == "" || compareVersionStrings(entry.Name(), best) > 0 {
			best = entry.Name()
		}
	}
	if best == "" {
		return "", fmt.Errorf("no usable %s versions found under %s", label, directory)
	}
	return best, nil
}

func compareVersionStrings(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	maxLen := len(as)
	if len(bs) > maxLen {
		maxLen = len(bs)
	}
	for i := 0; i < maxLen; i++ {
		av := 0
		if i < len(as) {
			av, _ = strconv.Atoi(as[i])
		}
		bv := 0
		if i < len(bs) {
			bv, _ = strconv.Atoi(bs[i])
		}
		switch {
		case av > bv:
			return 1
		case av < bv:
			return -1
		}
	}
	return 0
}

func readPackageRevision(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Pkg.Revision") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1]), nil
			}
		}
	}
	return "", fmt.Errorf("Pkg.Revision not found in %s", path)
}

func resolveAndroidBuildEnvironment(root string, minSDK int, sdkOverride, ndkOverride string) (androidBuildEnvironment, error) {
	if minSDK <= 0 {
		return androidBuildEnvironment{}, fmt.Errorf("minimum Android SDK must be a positive integer")
	}
	sdkPath, err := resolveAndroidSDKRoot(root, sdkOverride)
	if err != nil {
		return androidBuildEnvironment{}, err
	}
	compileSDK, err := resolveAndroidCompileSDK(sdkPath, defaultAndroidCompileSDK)
	if err != nil {
		return androidBuildEnvironment{}, err
	}
	if minSDK > compileSDK {
		return androidBuildEnvironment{}, fmt.Errorf("minimum Android SDK %d exceeds installed compile SDK %d", minSDK, compileSDK)
	}
	buildTools, err := resolveAndroidBuildToolsVersion(sdkPath, defaultAndroidBuildToolsVersion)
	if err != nil {
		return androidBuildEnvironment{}, err
	}
	cmakeVersion, err := resolveAndroidCMakeVersion(sdkPath, defaultAndroidCMakeVersion)
	if err != nil {
		return androidBuildEnvironment{}, err
	}
	ndkPath, err := resolveNDKRoot(root, sdkPath, ndkOverride)
	if err != nil {
		return androidBuildEnvironment{}, err
	}
	ndkVersion, err := readPackageRevision(filepath.Join(ndkPath, "source.properties"))
	if err != nil {
		return androidBuildEnvironment{}, err
	}
	if _, _, err := resolveAndroidToolchain(minSDK, ndkPath); err != nil {
		return androidBuildEnvironment{}, err
	}
	javaHome, err := resolveAndroidJavaHome()
	if err != nil {
		return androidBuildEnvironment{}, err
	}
	return androidBuildEnvironment{
		SDKPath:      sdkPath,
		NDKPath:      ndkPath,
		JavaHome:     javaHome,
		CompileSDK:   compileSDK,
		BuildTools:   buildTools,
		CMakeVersion: cmakeVersion,
		NDKVersion:   ndkVersion,
	}, nil
}

func resolveAndroidToolchain(api int, ndkRoot string) (string, string, error) {
	if ndkRoot == "" {
		return "", "", fmt.Errorf("android NDK not found: configure Android NDK in Build or set ANDROID_NDK_HOME / ANDROID_NDK_ROOT")
	}

	hostTag, err := androidHostTag()
	if err != nil {
		return "", "", err
	}

	binDir := filepath.Join(ndkRoot, "toolchains", "llvm", "prebuilt", hostTag, "bin")
	cc := filepath.Join(binDir, fmt.Sprintf("aarch64-linux-android%d-clang", api))
	cxx := filepath.Join(binDir, fmt.Sprintf("aarch64-linux-android%d-clang++", api))

	if _, err := os.Stat(cc); err != nil {
		return "", "", fmt.Errorf("android clang not found at %s", cc)
	}
	if _, err := os.Stat(cxx); err != nil {
		return "", "", fmt.Errorf("android clang++ not found at %s", cxx)
	}
	return cc, cxx, nil
}

func resolveNDKRoot(root, sdkRoot, override string) (string, error) {
	if strings.TrimSpace(override) != "" {
		return resolveOverrideDir(root, override, "Android NDK")
	}

	for _, key := range []string{"ANDROID_NDK_HOME", "ANDROID_NDK_ROOT"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return resolveOverrideDir(root, value, key)
		}
	}

	if sdkRoot == "" {
		var err error
		sdkRoot, err = resolveAndroidSDKRoot(root, "")
		if err != nil {
			return "", fmt.Errorf("android NDK not found: configure Android NDK in Build or set ANDROID_NDK_HOME / ANDROID_NDK_ROOT")
		}
	}

	best := ""
	if entries, err := os.ReadDir(filepath.Join(sdkRoot, "ndk")); err == nil {
		for _, entry := range entries {
			path := filepath.Join(sdkRoot, "ndk", entry.Name())
			if entry.IsDir() && isRegularFile(filepath.Join(path, "source.properties")) && (best == "" || compareVersionStrings(entry.Name(), filepath.Base(best)) > 0) {
				best = path
			}
		}
	}
	if best != "" {
		return best, nil
	}
	legacy := filepath.Join(sdkRoot, "ndk-bundle")
	if isRegularFile(filepath.Join(legacy, "source.properties")) {
		return legacy, nil
	}
	return "", fmt.Errorf("android NDK not found under %s", sdkRoot)
}

func resolveAndroidJavaHome() (string, error) {
	if value := strings.TrimSpace(os.Getenv("JAVA_HOME")); value != "" {
		return validateAndroidJavaHome(value, "JAVA_HOME")
	}
	if compiler, err := exec.LookPath(androidExecutableName("javac")); err == nil {
		if resolved, err := filepath.EvalSymlinks(compiler); err == nil {
			compiler = resolved
		}
		if home, err := validateAndroidJavaHome(filepath.Dir(filepath.Dir(compiler)), "Java on PATH"); err == nil {
			return home, nil
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory for Java: %w", err)
	}
	for _, candidate := range defaultAndroidJavaHomes(runtime.GOOS, home, os.Getenv("LOCALAPPDATA"), os.Getenv("ProgramFiles")) {
		if javaHome, err := validateAndroidJavaHome(candidate, "Java"); err == nil {
			return javaHome, nil
		}
	}
	return "", fmt.Errorf("Java %d or newer not found: install a JDK, set JAVA_HOME, or install Android Studio", minimumAndroidJavaVersion)
}

func defaultAndroidJavaHomes(goos, home, localAppData, programFiles string) []string {
	var candidates []string
	switch goos {
	case "darwin":
		candidates = append(candidates,
			filepath.Join("/Applications", "Android Studio.app", "Contents", "jbr", "Contents", "Home"),
			filepath.Join(home, "Applications", "Android Studio.app", "Contents", "jbr", "Contents", "Home"),
		)
	case "windows":
		candidates = append(candidates,
			filepath.Join(programFiles, "Android", "Android Studio", "jbr"),
			filepath.Join(localAppData, "Programs", "Android Studio", "jbr"),
		)
	default:
		candidates = append(candidates,
			filepath.Join(home, "android-studio", "jbr"),
			filepath.Join("/opt", "android-studio", "jbr"),
			filepath.Join("/usr/local", "android-studio", "jbr"),
			filepath.Join("/usr/lib", "jvm", "default-java"),
		)
	}
	candidates = append(candidates, filepath.Join(home, "Android", "jdk"), filepath.Join(home, "Android", "jdk-17"))
	patterns := []string{filepath.Join(home, "Android", "jdk-*")}
	if goos == "darwin" {
		patterns = append(patterns, filepath.Join("/Library", "Java", "JavaVirtualMachines", "*", "Contents", "Home"))
	} else if goos != "windows" {
		patterns = append(patterns, filepath.Join("/usr", "lib", "jvm", "*"))
	}
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		sort.Slice(matches, func(i, j int) bool {
			return compareVersionStrings(versionSuffix(filepath.Base(matches[i])), versionSuffix(filepath.Base(matches[j]))) > 0
		})
		candidates = append(candidates, matches...)
	}
	return candidates
}

func validateAndroidJavaHome(home, label string) (string, error) {
	directory := existingDirectory(home)
	if directory == "" {
		return "", fmt.Errorf("%s directory not found: %s", label, home)
	}
	for _, tool := range []string{"java", "javac"} {
		if !isRegularFile(filepath.Join(directory, "bin", androidExecutableName(tool))) {
			return "", fmt.Errorf("%s is not a JDK: %s is missing", label, tool)
		}
	}
	version, err := readJavaMajorVersion(filepath.Join(directory, "release"))
	if err != nil {
		return "", err
	}
	if version < minimumAndroidJavaVersion {
		return "", fmt.Errorf("%s uses Java %d; Android builds require Java %d or newer", label, version, minimumAndroidJavaVersion)
	}
	if version > maximumAndroidJavaVersion {
		return "", fmt.Errorf("%s uses Java %d; Gradle 8.9 requires Java %d through %d", label, version, minimumAndroidJavaVersion, maximumAndroidJavaVersion)
	}
	return directory, nil
}

func readJavaMajorVersion(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read Java release metadata %s: %w", path, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		name, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(name) != "JAVA_VERSION" {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"`)
		parts := strings.Split(value, ".")
		majorIndex := 0
		if len(parts) > 1 && parts[0] == "1" {
			majorIndex = 1
		}
		major, err := strconv.Atoi(versionDigits(parts[majorIndex]))
		if err == nil && major > 0 {
			return major, nil
		}
		break
	}
	return 0, fmt.Errorf("JAVA_VERSION not found in %s", path)
}

func versionSuffix(value string) string {
	for index, r := range value {
		if r >= '0' && r <= '9' {
			return value[index:]
		}
	}
	return value
}

func versionDigits(value string) string {
	for index, r := range value {
		if r < '0' || r > '9' {
			return value[:index]
		}
	}
	return value
}

func androidExecutableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func androidCommandEnvironment(env []string, config androidProjectConfig) []string {
	env = append([]string{}, env...)
	env = setEnvironmentValue(env, "ANDROID_HOME", config.SDKPath)
	env = setEnvironmentValue(env, "JAVA_HOME", config.JavaHome)
	return env
}

func resolveOverrideDir(root, value, label string) (string, error) {
	path := strings.TrimSpace(value)
	if path == "" {
		return "", fmt.Errorf("%s directory cannot be empty", label)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path = filepath.Clean(path)
	stat, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("%s directory not found: %s", label, path)
	}
	if !stat.IsDir() {
		return "", fmt.Errorf("%s path is not a directory: %s", label, path)
	}
	return path, nil
}

func androidHostTag() (string, error) {
	switch runtime.GOOS {
	case "linux":
		return "linux-x86_64", nil
	case "windows":
		return "windows-x86_64", nil
	case "darwin":
		if runtime.GOARCH == "arm64" {
			return "darwin-arm64", nil
		}
		return "darwin-x86_64", nil
	default:
		return "", fmt.Errorf("unsupported host platform for Android NDK: %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

func resolveGradleCommand(projectDir string) ([]string, error) {
	wrapperCandidates := []string{"gradlew"}
	if runtime.GOOS == "windows" {
		wrapperCandidates = []string{"gradlew.bat", "gradlew"}
	}
	for _, candidate := range wrapperCandidates {
		wrapper := filepath.Join(projectDir, candidate)
		if stat, err := os.Stat(wrapper); err == nil && !stat.IsDir() {
			return []string{wrapper}, nil
		}
	}

	gradlePath, err := exec.LookPath("gradle")
	if err == nil {
		return []string{gradlePath}, nil
	}
	return nil, fmt.Errorf("gradle not found: install gradle or add a gradle wrapper under %s", projectDir)
}

func resolveAPKOutput(projectDir string, task string) (string, error) {
	switch {
	case strings.EqualFold(task, "assembleDebug"):
		return filepath.Join(projectDir, "app", "build", "outputs", "apk", "debug", "app-debug.apk"), nil
	case strings.EqualFold(task, "assembleRelease"):
		return filepath.Join(projectDir, "app", "build", "outputs", "apk", "release", "app-release-unsigned.apk"), nil
	default:
		return "", fmt.Errorf("unable to infer APK path for gradle task %q", task)
	}
}

func findRepoRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("unable to locate go.mod from %s", start)
		}
		dir = parent
	}
}
