package editor

import (
	"path/filepath"

	editoricons "github.com/diiyw/onix/editor/resources/images/icons"
)

const (
	buildAppNameField       = "build.app-name"
	buildIconField          = "build.icon"
	buildJavaPackageField   = "build.java-package"
	buildAndroidMinSDKField = "build.android-min-sdk"
	buildAndroidSDKField    = "build.android-sdk"
	buildAndroidNDKField    = "build.android-ndk"
	buildWebTitleField      = "build.web-title"
	buildWeChatAppIDField   = "build.wechat-app-id"
)

type buildResult struct {
	artifact  string
	outputDir string
	err       error
}

func buildPlatforms() []BuildPlatform {
	return []BuildPlatform{BuildPlatformWindows, BuildPlatformMacOS, BuildPlatformLinux, BuildPlatformAndroid, BuildPlatformIOS, BuildPlatformWeb, BuildPlatformWeChat}
}

func buildPlatformLabel(platform BuildPlatform) string {
	switch platform {
	case BuildPlatformWindows:
		return "Windows"
	case BuildPlatformMacOS:
		return "macOS"
	case BuildPlatformLinux:
		return "Linux"
	case BuildPlatformAndroid:
		return "Android"
	case BuildPlatformIOS:
		return "iOS"
	case BuildPlatformWeb:
		return "Web"
	case BuildPlatformWeChat:
		return "WeChat"
	default:
		return string(platform)
	}
}

func buildPlatformIcon(platform BuildPlatform) editoricons.Name {
	switch platform {
	case BuildPlatformWindows:
		return editoricons.Windows
	case BuildPlatformMacOS:
		return editoricons.MacOS
	case BuildPlatformLinux:
		return editoricons.Linux
	case BuildPlatformAndroid:
		return editoricons.Android
	case BuildPlatformIOS:
		return editoricons.IOS
	case BuildPlatformWeb:
		return editoricons.Web
	case BuildPlatformWeChat:
		return editoricons.WeChat
	default:
		return ""
	}
}

func buildPackageFormatLabel(format BuildPackageFormat) string {
	switch format {
	case BuildPackagePortable:
		return "Portable"
	case BuildPackageApp:
		return "App Bundle"
	case BuildPackageDMG:
		return "DMG"
	case BuildPackageMSI:
		return "MSI"
	default:
		return string(format)
	}
}

func buildModeLabel(mode BuildMode) string {
	if mode == BuildModeRelease {
		return "Release"
	}
	return "Debug"
}

func buildArchitectureLabel(architecture BuildArchitecture) string {
	switch architecture {
	case BuildArchitectureUniversal:
		return "Universal"
	case BuildArchitectureAMD64:
		return "Intel"
	case BuildArchitectureARM64:
		return "Apple Silicon"
	default:
		return string(architecture)
	}
}

func buildPlatformOutputDir(projectDir string, platform BuildPlatform) string {
	return filepath.Join(projectDir, buildOutputDirName, string(platform))
}
