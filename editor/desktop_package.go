package editor

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

func buildLinuxPackage(root string, options BuildOptions) (string, error) {
	if options.Progress != nil {
		options.Progress(BuildProgress{Message: "Preparing Linux application...", Value: 0.08, Indeterminate: true})
	}
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("Linux builds require a Linux host")
	}
	layout, err := newProjectBuildLayout(root, options.ProjectDir, BuildPlatformLinux, "")
	if err != nil {
		return "", err
	}
	name := artifactName(options.AppName, layout.PackageName)
	stageDir := filepath.Join(layout.BaseDir, ".linux-stage")
	bundleDir := filepath.Join(stageDir, name)
	archivePath := filepath.Join(layout.BaseDir, fmt.Sprintf("%s-linux-%s.tar.gz", name, runtime.GOARCH))
	return withTemporaryBuildDir(layout.BaseDir, stageDir, func() (string, error) {
		layout.ResourceDir = filepath.Join(bundleDir, "resources")
		layout.NativeResourceDir = "resources"
		icon, err := loadBuildIcon(options)
		if err != nil {
			return "", err
		}
		if err := prepareExternalProjectResources(root, &layout, options.Mode, resourceBuildProgress(options.Progress)); err != nil {
			return "", err
		}
		if err := copyProjectThirdPartyNotices(layout.ProjectDir, bundleDir); err != nil {
			return "", err
		}
		return withResourceOverlay(layout, func() (string, error) {
			if err := writeLinuxIconAssets(bundleDir, name, icon); err != nil {
				return "", err
			}
			executable := filepath.Join(bundleDir, name)
			if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
				return "", fmt.Errorf("create Linux package directory: %w", err)
			}
			args := appendReleaseFlags([]string{"build", "-buildvcs=false", "-trimpath", "-overlay", layout.OverlayPath}, options.Mode)
			args = append(args, "-o", executable, layout.ProjectImportPath)
			env, err := linuxBuildEnvironment()
			if err != nil {
				return "", err
			}
			if err := runCommandWithProgress(options.Progress, root, env, "go", args...); err != nil {
				return "", err
			}
			reportBuildActivity(options.Progress, "Packaging Linux archive...", .9)
			if err := writeTarGzArchive(archivePath, bundleDir, name); err != nil {
				return "", fmt.Errorf("package Linux portable archive: %w", err)
			}
			return archivePath, nil
		})
	})
}

func buildWindowsPackage(root string, options BuildOptions) (string, error) {
	if options.Progress != nil {
		options.Progress(BuildProgress{Message: "Preparing Windows application...", Value: 0.08, Indeterminate: true})
	}
	env, err := windowsBuildEnvironment(runtime.GOOS, runtime.GOARCH, exec.LookPath)
	if err != nil {
		return "", err
	}
	layout, err := newProjectBuildLayout(root, options.ProjectDir, BuildPlatformWindows, "")
	if err != nil {
		return "", err
	}
	name := artifactName(options.AppName, layout.PackageName)
	stageDir := filepath.Join(layout.BaseDir, ".windows-stage")
	bundleDir := filepath.Join(stageDir, name)
	return withTemporaryBuildDir(layout.BaseDir, stageDir, func() (string, error) {
		layout.ResourceDir = filepath.Join(bundleDir, "resources")
		layout.NativeResourceDir = "resources"
		icon, err := loadBuildIcon(options)
		if err != nil {
			return "", err
		}
		if err := prepareExternalProjectResources(root, &layout, options.Mode, resourceBuildProgress(options.Progress)); err != nil {
			return "", err
		}
		if err := copyProjectThirdPartyNotices(layout.ProjectDir, bundleDir); err != nil {
			return "", err
		}
		return withResourceOverlay(layout, func() (string, error) {
			resourceFile, err := writeWindowsIconAssets(root, layout, bundleDir, name, icon, env)
			if err != nil {
				return "", err
			}
			executable := filepath.Join(bundleDir, name+".exe")
			if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
				return "", fmt.Errorf("create Windows package directory: %w", err)
			}
			args := windowsBuildArgs(options.Mode, resourceFile)
			args = append(args, "-overlay", layout.OverlayPath, "-o", executable, layout.ProjectImportPath)
			if err := runCommandWithProgress(options.Progress, root, env, "go", args...); err != nil {
				return "", err
			}
			reportBuildActivity(options.Progress, "Packaging Windows application...", .9)
			switch options.PackageFormat {
			case BuildPackagePortable:
				output := filepath.Join(layout.BaseDir, fmt.Sprintf("%s-windows-%s.zip", name, runtime.GOARCH))
				if err := writeZipArchive(output, bundleDir, name); err != nil {
					return "", fmt.Errorf("package Windows portable archive: %w", err)
				}
				return output, nil
			case BuildPackageMSI:
				output := filepath.Join(layout.BaseDir, name+".msi")
				if err := packageWindowsMSI(layout.BaseDir, bundleDir, executable, output, name, layout.ProjectImportPath, options.Progress); err != nil {
					return "", err
				}
				return output, nil
			default:
				return "", fmt.Errorf("unsupported Windows package format %q", options.PackageFormat)
			}
		})
	})
}

func windowsBuildEnvironment(hostOS, architecture string, lookPath func(string) (string, error)) ([]string, error) {
	ccName, cxxName := "gcc", "g++"
	if hostOS != "windows" {
		prefix := ""
		switch architecture {
		case "amd64":
			prefix = "x86_64-w64-mingw32"
		case "arm64":
			prefix = "aarch64-w64-mingw32"
		default:
			return nil, fmt.Errorf("unsupported Windows architecture %q", architecture)
		}
		ccName, cxxName = prefix+"-gcc", prefix+"-g++"
	}
	cc, err := lookPath(ccName)
	if err != nil {
		return nil, fmt.Errorf("Windows %s C compiler %q not found; install a MinGW-w64 toolchain", architecture, ccName)
	}
	cxx, err := lookPath(cxxName)
	if err != nil {
		return nil, fmt.Errorf("Windows %s C++ compiler %q not found; install a MinGW-w64 toolchain", architecture, cxxName)
	}
	env := append(append([]string{}, os.Environ()...),
		"GOOS=windows", "GOARCH="+architecture, "CGO_ENABLED=1", "CC="+cc, "CXX="+cxx,
	)
	return appendVcpkgEnvironment(env, "windows", hostOS, architecture)
}

func buildMacOSPackage(root string, options BuildOptions) (string, error) {
	if options.Progress != nil {
		options.Progress(BuildProgress{Message: "Preparing macOS application...", Value: 0.08, Indeterminate: true})
	}
	if options.PackageFormat == BuildPackageDMG && runtime.GOOS != "darwin" {
		return "", fmt.Errorf("macOS DMG packaging requires macOS and hdiutil")
	}
	layout, err := newProjectBuildLayout(root, options.ProjectDir, BuildPlatformMacOS, "")
	if err != nil {
		return "", err
	}
	name := artifactName(options.AppName, layout.PackageName)
	appBundle := filepath.Join(layout.BaseDir, name+".app")
	if err := resetArtifactPath(layout.BaseDir, appBundle); err != nil {
		return "", err
	}
	layout.ResourceDir = filepath.Join(appBundle, "Contents", "Resources")
	layout.NativeResourceDir = "../Resources"
	if err := prepareExternalProjectResources(root, &layout, options.Mode, resourceBuildProgress(options.Progress)); err != nil {
		return "", errors.Join(err, removeTemporaryArtifact(appBundle))
	}
	if err := copyProjectThirdPartyNotices(layout.ProjectDir, layout.ResourceDir); err != nil {
		return "", errors.Join(err, cleanupResourceOverlay(layout), removeTemporaryArtifact(appBundle))
	}
	icon, err := loadBuildIcon(options)
	if err != nil {
		return "", errors.Join(err, cleanupResourceOverlay(layout), removeTemporaryArtifact(appBundle))
	}
	if err := icon.writeICNS(filepath.Join(layout.ResourceDir, "AppIcon.icns")); err != nil {
		return "", errors.Join(err, cleanupResourceOverlay(layout), removeTemporaryArtifact(appBundle))
	}
	executable := filepath.Join(appBundle, "Contents", "MacOS", name)
	if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
		return "", errors.Join(fmt.Errorf("create macOS app executable directory: %w", err), cleanupResourceOverlay(layout), removeTemporaryArtifact(appBundle))
	}
	if err := writeMacOSInfoPlist(filepath.Join(appBundle, "Contents", "Info.plist"), name); err != nil {
		return "", errors.Join(err, cleanupResourceOverlay(layout), removeTemporaryArtifact(appBundle))
	}
	appArtifact, err := withResourceOverlay(layout, func() (string, error) {
		if err := buildMacOSExecutable(root, layout, options, executable); err != nil {
			return "", err
		}
		if err := normalizeBundlePermissions(appBundle, executable); err != nil {
			return "", err
		}
		reportBuildActivity(options.Progress, "Signing macOS application...", .9)
		if err := signMacOSAppBundle(root, appBundle, options.Progress); err != nil {
			return "", err
		}
		return appBundle, nil
	})
	if err != nil {
		return "", errors.Join(err, removeTemporaryArtifact(appBundle))
	}
	if options.PackageFormat == BuildPackageApp {
		return appArtifact, nil
	}
	output := filepath.Join(layout.BaseDir, name+".dmg")
	reportBuildActivity(options.Progress, "Packaging macOS disk image...", .95)
	packageErr := packageMacOSDMGWith(layout.BaseDir, appBundle, output, name, exec.LookPath, func(root string, env []string, name string, args ...string) error {
		return runCommandWithProgress(options.Progress, root, env, name, args...)
	})
	cleanupErr := removeTemporaryArtifact(appBundle)
	if err := errors.Join(packageErr, cleanupErr); err != nil {
		return "", err
	}
	return output, nil
}

func buildMacOSExecutable(root string, layout projectBuildLayout, options BuildOptions, executable string) error {
	return buildMacOSExecutableWith(runtime.GOOS, root, layout, options, executable, exec.LookPath, func(root string, env []string, name string, args ...string) error {
		return runCommandWithProgress(options.Progress, root, env, name, args...)
	})
}

func buildMacOSExecutableWith(
	hostOS, root string,
	layout projectBuildLayout,
	options BuildOptions,
	executable string,
	lookPath func(string) (string, error),
	run func(string, []string, string, ...string) error,
) error {
	architectures, err := macOSTargetArchitectures(options.Architecture)
	if err != nil {
		return err
	}
	build := func(architecture BuildArchitecture, output string) error {
		env, err := macOSBuildEnvironment(hostOS, architecture, lookPath)
		if err != nil {
			return err
		}
		reportBuildActivity(options.Progress, "Compiling macOS "+string(architecture)+"...", .65)
		args := macOSBuildArgs(options.Mode)
		args = append(args, "-overlay", layout.OverlayPath, "-o", output, layout.ProjectImportPath)
		return run(root, env, "go", args...)
	}
	if len(architectures) == 1 {
		if err := build(architectures[0], executable); err != nil {
			return err
		}
		return ensureExecutablePermission(executable)
	}
	lipo, err := lookPath("lipo")
	if err != nil {
		return fmt.Errorf("macOS universal build tool \"lipo\" not found: install the Xcode command line tools or osxcross")
	}
	stageDir := filepath.Join(filepath.Dir(executable), ".onix-universal")
	_, err = withTemporaryBuildDir(filepath.Dir(executable), stageDir, func() (struct{}, error) {
		slices := make([]string, len(architectures))
		for index, architecture := range architectures {
			slices[index] = filepath.Join(stageDir, string(architecture))
			if err := build(architecture, slices[index]); err != nil {
				return struct{}{}, fmt.Errorf("build macOS %s executable: %w", architecture, err)
			}
		}
		args := append([]string{"-create"}, slices...)
		args = append(args, "-output", executable)
		if err := run(root, os.Environ(), lipo, args...); err != nil {
			return struct{}{}, fmt.Errorf("combine macOS universal executable: %w", err)
		}
		return struct{}{}, nil
	})
	if err != nil {
		return err
	}
	return ensureExecutablePermission(executable)
}

func ensureExecutablePermission(name string) error {
	if err := os.Chmod(name, 0o755); err != nil {
		return fmt.Errorf("set macOS executable permission for %s: %w", name, err)
	}
	return nil
}

func macOSTargetArchitectures(architecture BuildArchitecture) ([]BuildArchitecture, error) {
	if architecture == BuildArchitectureUniversal {
		configs := macOSArchitectureConfigs()
		architectures := make([]BuildArchitecture, len(configs))
		for index, config := range configs {
			architectures[index] = config.Architecture
		}
		return architectures, nil
	}
	if _, err := macOSArchitectureConfigFor(architecture); err != nil {
		return nil, err
	}
	return []BuildArchitecture{architecture}, nil
}

func signMacOSAppBundle(root, appBundle string, progress func(BuildProgress)) error {
	if runtime.GOOS != "darwin" {
		return nil
	}
	codesign, err := exec.LookPath("codesign")
	if err != nil {
		return fmt.Errorf("macOS code signing tool \"codesign\" not found: install the Xcode command line tools")
	}
	if err := runCommandWithProgress(progress, root, os.Environ(), codesign, "--force", "--sign", "-", appBundle); err != nil {
		return fmt.Errorf("ad-hoc sign macOS app bundle: %w", err)
	}
	return nil
}

func removeTemporaryArtifact(name string) error {
	if err := os.RemoveAll(name); err != nil {
		return fmt.Errorf("remove temporary artifact %s: %w", name, err)
	}
	return nil
}

func copyProjectThirdPartyNotices(projectDir, targetDir string) error {
	source := filepath.Join(projectDir, "THIRD_PARTY_NOTICES.md")
	data, err := os.ReadFile(source)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read third-party notices %s: %w", source, err)
	}
	target := filepath.Join(targetDir, "THIRD_PARTY_NOTICES.md")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create third-party notices directory: %w", err)
	}
	if err := writeAtomicOutput(target, func(writer io.Writer) error {
		_, err := writer.Write(data)
		return err
	}); err != nil {
		return fmt.Errorf("copy third-party notices to %s: %w", target, err)
	}
	return nil
}

func packageMacOSDMG(root, appBundle, output, volumeName string) error {
	return packageMacOSDMGWith(root, appBundle, output, volumeName, exec.LookPath, runCommand)
}

func packageMacOSDMGWith(root, appBundle, output, volumeName string, lookPath func(string) (string, error), run func(string, []string, string, ...string) error) error {
	hdiutil, err := lookPath("hdiutil")
	if err != nil {
		return fmt.Errorf("hdiutil was not found; DMG packaging requires macOS")
	}
	if err := os.Remove(output); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("replace existing DMG: %w", err)
	}
	stageDir := filepath.Join(root, ".dmg-stage")
	_, err = withTemporaryBuildDir(root, stageDir, func() (struct{}, error) {
		stagedApp := filepath.Join(stageDir, filepath.Base(appBundle))
		if err := os.Rename(appBundle, stagedApp); err != nil {
			return struct{}{}, fmt.Errorf("stage macOS app bundle: %w", err)
		}
		if err := os.Symlink("/Applications", filepath.Join(stageDir, "Applications")); err != nil {
			return struct{}{}, fmt.Errorf("create DMG Applications link: %w", err)
		}
		if err := run(root, os.Environ(), hdiutil, "create", "-volname", volumeName, "-srcfolder", stageDir, "-ov", "-format", "UDZO", output); err != nil {
			return struct{}{}, fmt.Errorf("package macOS DMG: %w", err)
		}
		return struct{}{}, nil
	})
	if err != nil {
		return err
	}
	return os.Chmod(output, 0o644)
}

func withTemporaryBuildDir[T any](baseDir, temporaryDir string, build func() (T, error)) (T, error) {
	if err := resetArtifactPath(baseDir, temporaryDir); err != nil {
		var zero T
		return zero, err
	}
	if err := os.MkdirAll(temporaryDir, 0o755); err != nil {
		var zero T
		return zero, fmt.Errorf("create temporary build directory: %w", err)
	}
	result, buildErr := build()
	cleanupErr := os.RemoveAll(temporaryDir)
	if err := errors.Join(buildErr, cleanupErr); err != nil {
		var zero T
		return zero, err
	}
	return result, nil
}

func resetArtifactPath(baseDir, target string) error {
	absoluteBase, err := filepath.Abs(baseDir)
	if err != nil {
		return err
	}
	absoluteTarget, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(absoluteBase, absoluteTarget)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(filepath.ToSlash(relative), "../") {
		return fmt.Errorf("refusing to reset artifact path %s outside %s", absoluteTarget, absoluteBase)
	}
	if err := os.RemoveAll(absoluteTarget); err != nil {
		return fmt.Errorf("reset artifact path %s: %w", absoluteTarget, err)
	}
	return nil
}

func artifactName(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	value = strings.Map(func(char rune) rune {
		if char < 0x20 || strings.ContainsRune(`/\\<>:"|?*`, char) {
			return '_'
		}
		return char
	}, value)
	value = strings.Trim(value, ". ")
	if value == "" {
		return "Onix"
	}
	return value
}

type archiveEntry struct {
	absolute string
	relative string
	info     fs.FileInfo
}

func collectArchiveEntries(sourceDir string) ([]archiveEntry, error) {
	entries := make([]archiveEntry, 0)
	err := filepath.WalkDir(sourceDir, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if name == sourceDir {
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("archive source contains unsupported symlink %s", name)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(sourceDir, name)
		if err != nil {
			return err
		}
		entries = append(entries, archiveEntry{absolute: name, relative: filepath.ToSlash(relative), info: info})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].relative < entries[j].relative })
	return entries, nil
}

func writeTarGzArchive(output, sourceDir, rootName string) error {
	entries, err := collectArchiveEntries(sourceDir)
	if err != nil {
		return err
	}
	return writeAtomicOutput(output, func(writer io.Writer) error {
		compressed, err := gzip.NewWriterLevel(writer, gzip.BestCompression)
		if err != nil {
			return err
		}
		compressed.Header.ModTime = time.Time{}
		archive := tar.NewWriter(compressed)
		for _, entry := range entries {
			header, err := tar.FileInfoHeader(entry.info, "")
			if err != nil {
				return err
			}
			header.Name = path.Join(rootName, entry.relative)
			header.ModTime, header.AccessTime, header.ChangeTime = time.Time{}, time.Time{}, time.Time{}
			header.Mode = int64(distributionMode(entry.info).Perm())
			header.Uid, header.Gid, header.Uname, header.Gname = 0, 0, "", ""
			if err := archive.WriteHeader(header); err != nil {
				return err
			}
			if entry.info.Mode().IsRegular() {
				if err := copyArchiveFile(archive, entry.absolute); err != nil {
					return err
				}
			}
		}
		if err := archive.Close(); err != nil {
			return err
		}
		return compressed.Close()
	})
}

func writeZipArchive(output, sourceDir, rootName string) error {
	entries, err := collectArchiveEntries(sourceDir)
	if err != nil {
		return err
	}
	return writeAtomicOutput(output, func(writer io.Writer) error {
		archive := zip.NewWriter(writer)
		for _, entry := range entries {
			header, err := zip.FileInfoHeader(entry.info)
			if err != nil {
				return err
			}
			header.Name = path.Join(rootName, entry.relative)
			if entry.info.IsDir() {
				header.Name += "/"
			}
			header.SetMode(distributionMode(entry.info))
			header.Method = zip.Deflate
			target, err := archive.CreateHeader(header)
			if err != nil {
				return err
			}
			if entry.info.Mode().IsRegular() {
				if err := copyArchiveFile(target, entry.absolute); err != nil {
					return err
				}
			}
		}
		return archive.Close()
	})
}

func copyArchiveFile(writer io.Writer, name string) error {
	file, err := os.Open(name)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(writer, file)
	return errors.Join(copyErr, file.Close())
}

func distributionMode(info fs.FileInfo) fs.FileMode {
	if info.IsDir() {
		return fs.ModeDir | 0o755
	}
	if info.Mode()&0o111 != 0 {
		return 0o755
	}
	return 0o644
}

func normalizeBundlePermissions(root, executable string) error {
	return filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		mode := fs.FileMode(0o644)
		if entry.IsDir() || name == executable {
			mode = 0o755
		}
		if err := os.Chmod(name, mode); err != nil {
			return fmt.Errorf("set app bundle permissions for %s: %w", name, err)
		}
		return nil
	})
}

func writeAtomicOutput(output string, write func(io.Writer) error) error {
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	temporary := output + ".tmp"
	file, err := os.Create(temporary)
	if err != nil {
		return err
	}
	writeErr := write(file)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	if err := os.Rename(temporary, output); err != nil {
		if removeErr := os.Remove(output); removeErr != nil && !os.IsNotExist(removeErr) {
			_ = os.Remove(temporary)
			return errors.Join(err, removeErr)
		}
		if retryErr := os.Rename(temporary, output); retryErr != nil {
			_ = os.Remove(temporary)
			return retryErr
		}
	}
	return os.Chmod(output, 0o644)
}

func writeMacOSInfoPlist(output, executable string) error {
	var data bytes.Buffer
	data.WriteString(xml.Header)
	data.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	encoder := xml.NewEncoder(&data)
	encoder.Indent("", "  ")
	start := xml.StartElement{Name: xml.Name{Local: "plist"}, Attr: []xml.Attr{{Name: xml.Name{Local: "version"}, Value: "1.0"}}}
	if err := encoder.EncodeToken(start); err != nil {
		return err
	}
	if err := encoder.EncodeToken(xml.StartElement{Name: xml.Name{Local: "dict"}}); err != nil {
		return err
	}
	values := []struct{ key, value string }{
		{key: "CFBundleDevelopmentRegion", value: "en"},
		{key: "CFBundleDisplayName", value: executable},
		{key: "CFBundleExecutable", value: executable},
		{key: "CFBundleIdentifier", value: "dev.onix." + macOSBundleIdentifierPart(executable)},
		{key: "CFBundleIconFile", value: "AppIcon"},
		{key: "CFBundleInfoDictionaryVersion", value: "6.0"},
		{key: "CFBundleName", value: executable},
		{key: "CFBundlePackageType", value: "APPL"},
		{key: "CFBundleShortVersionString", value: "1.0.0"},
		{key: "CFBundleVersion", value: "1"},
		{key: "LSMinimumSystemVersion", value: "11.0"},
	}
	for _, value := range values {
		if err := encoder.EncodeElement(value.key, xml.StartElement{Name: xml.Name{Local: "key"}}); err != nil {
			return err
		}
		if err := encoder.EncodeElement(value.value, xml.StartElement{Name: xml.Name{Local: "string"}}); err != nil {
			return err
		}
	}
	if err := encoder.EncodeElement("NSHighResolutionCapable", xml.StartElement{Name: xml.Name{Local: "key"}}); err != nil {
		return err
	}
	trueElement := xml.StartElement{Name: xml.Name{Local: "true"}}
	if err := encoder.EncodeToken(trueElement); err != nil {
		return err
	}
	if err := encoder.EncodeToken(trueElement.End()); err != nil {
		return err
	}
	if err := encoder.EncodeToken(xml.EndElement{Name: xml.Name{Local: "dict"}}); err != nil {
		return err
	}
	if err := encoder.EncodeToken(start.End()); err != nil {
		return err
	}
	if err := encoder.Flush(); err != nil {
		return err
	}
	data.WriteByte('\n')
	return writeAtomicOutput(output, func(writer io.Writer) error {
		_, err := writer.Write(data.Bytes())
		return err
	})
}

func macOSBundleIdentifierPart(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	var result strings.Builder
	separator := false
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			result.WriteRune(char)
			separator = false
			continue
		}
		if result.Len() != 0 && !separator {
			result.WriteByte('-')
			separator = true
		}
	}
	identifier := strings.Trim(result.String(), "-")
	if identifier == "" {
		return "app"
	}
	return identifier
}
