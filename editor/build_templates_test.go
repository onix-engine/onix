package editor

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestNativeTextInputTemplatesParse(t *testing.T) {
	for _, path := range []string{
		"templates/android/native/main.go",
		"templates/ios/native/main.go",
	} {
		source := readTemplate(t, path)
		source = strings.ReplaceAll(source, "__APP_IMPORT_PATH__", "github.com/diiyw/onix")
		if _, err := parser.ParseFile(token.NewFileSet(), path, source, parser.AllErrors); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
	}
}

func TestNativeTemplatesRestoreHostSurfaceSizeAfterProjectStart(t *testing.T) {
	for _, path := range []string{
		"templates/android/native/main.go",
		"templates/ios/native/main.go",
	} {
		source := readTemplate(t, path)
		if !strings.Contains(source, "window.SetSurfaceSize(int(width), int(height))") {
			t.Fatalf("native template %s does not restore the host surface size", path)
		}
	}
}

func TestAndroidTextInputTemplateContract(t *testing.T) {
	bridge := readTemplate(t, "templates/android/app/src/main/cpp/onixbridge.c")
	for _, expected := range []string{
		"GetStringChars",
		"OnixTextInputConfig",
		"OnixTextInputValue",
		"OnixTextSelection",
		"OnixTextCommand",
		"uint64_t session",
		"ANativeWindow_fromSurface",
	} {
		if !strings.Contains(bridge, expected) {
			t.Fatalf("Android bridge is missing %s", expected)
		}
	}
	if strings.Contains(bridge, "GetStringUTFChars") {
		t.Fatal("Android bridge must not pass JNI modified UTF-8 to Go")
	}

	view := readTemplate(t, "templates/android/app/src/main/java/__JAVA_PACKAGE_PATH__/OnixSurfaceView.java")
	for _, expected := range []string{
		"setTextInputConfig",
		"deleteSurroundingText",
		"deleteSurroundingTextInCodePoints",
		"performEditorAction",
		"performContextMenuAction",
		"long session",
		"Choreographer.FrameCallback",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("Android surface view is missing %s", expected)
		}
	}
}

func TestAndroidGradleTemplateUsesDetectedToolVersions(t *testing.T) {
	build := readTemplate(t, "templates/android/app/build.gradle")
	for _, expected := range []string{
		"onix.android.compileSdk",
		"onix.android.buildToolsVersion",
		"onix.android.cmakeVersion",
		"onix.android.ndkVersion",
		"buildToolsVersion onixBuildToolsVersion",
	} {
		if !strings.Contains(build, expected) {
			t.Fatalf("Android Gradle template is missing %s", expected)
		}
	}
}

func TestAndroidTemplateHidesStatusBar(t *testing.T) {
	styles := readTemplate(t, "templates/android/app/src/main/res/values/styles.xml")
	if !strings.Contains(styles, `<item name="android:windowFullscreen">true</item>`) {
		t.Fatal("Android theme does not hide the status bar")
	}
}

func TestIOSTextInputTemplateContract(t *testing.T) {
	view := readTemplate(t, "templates/ios/host/OnixMetalView.swift")
	for _, expected := range []string{"UITextViewDelegate", "OnixTextInputConfig", "OnixTextSelection", "showTextEditMenu"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("iOS metal view is missing %s", expected)
		}
	}
}

func TestAndroidWebViewTemplateContract(t *testing.T) {
	bridge := readTemplate(t, "templates/android/app/src/main/cpp/onixbridge.c")
	native := readTemplate(t, "templates/android/native/main.go")
	host := readTemplate(t, "templates/android/app/src/main/java/__JAVA_PACKAGE_PATH__/SystemWebView.java")
	activity := readTemplate(t, "templates/android/app/src/main/java/__JAVA_PACKAGE_PATH__/MainActivity.java")
	manifest := readTemplate(t, "templates/android/app/src/main/AndroidManifest.xml")
	for _, expected := range []string{"OnixWebViewCommand", "OnixWebViewEvent", "OnixWebViewInvoke"} {
		if !strings.Contains(bridge, expected) || !strings.Contains(native, expected) {
			t.Fatalf("Android WebView bridge is missing %s", expected)
		}
	}
	for _, expected := range []string{"android.webkit.WebView", "addJavascriptInterface", "loadUrl(content)", "loadDataWithBaseURL", "webViewCommandComplete", "rootView.post(task)"} {
		if !strings.Contains(host, expected) {
			t.Fatalf("Android system WebView is missing %s", expected)
		}
	}
	if !strings.Contains(manifest, "android.permission.INTERNET") {
		t.Fatal("Android WebView cannot load remote URLs without Internet permission")
	}
	if !strings.Contains(activity, "FrameLayout") {
		t.Fatal("Android activity does not provide an overlay container for WebViews")
	}
}

func readTemplate(t *testing.T, path string) string {
	t.Helper()
	content, err := buildTemplates.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}
