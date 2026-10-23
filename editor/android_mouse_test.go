package editor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Run the template against real MotionEvents without starting the editor or a window.
func TestAndroidMouseInputOnDevice(t *testing.T) {
	serial := os.Getenv("ONIX_ANDROID_TEST_SERIAL")
	if serial == "" {
		t.Skip("set ONIX_ANDROID_TEST_SERIAL to run Android mouse input tests")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	sdk, err := resolveAndroidSDKRoot(root, "")
	if err != nil {
		t.Fatal(err)
	}
	api, err := resolveAndroidCompileSDK(sdk, defaultAndroidCompileSDK)
	if err != nil {
		t.Fatal(err)
	}
	buildTools, err := resolveAndroidBuildToolsVersion(sdk, defaultAndroidBuildToolsVersion)
	if err != nil {
		t.Fatal(err)
	}
	java, err := resolveAndroidJavaHome()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	run := func(name string, args ...string) string {
		t.Helper()
		command := exec.CommandContext(ctx, name, args...)
		command.Env = append(os.Environ(), "JAVA_HOME="+java)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", name, err, output)
		}
		return string(output)
	}
	dir := t.TempDir()
	source := strings.ReplaceAll(readTemplate(t, "templates/android/app/src/main/java/__JAVA_PACKAGE_PATH__/OnixMouseInput.java"), "__JAVA_PACKAGE__", "dev.onix.mousetest")
	helper := filepath.Join(dir, "OnixMouseInput.java")
	if err := os.WriteFile(helper, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	viewSource := strings.ReplaceAll(readTemplate(t, "templates/android/app/src/main/java/__JAVA_PACKAGE_PATH__/OnixSurfaceView.java"), "__JAVA_PACKAGE__", "dev.onix.mousetest")
	view := filepath.Join(dir, "OnixSurfaceView.java")
	if err := os.WriteFile(view, []byte(viewSource), 0600); err != nil {
		t.Fatal(err)
	}
	classes := filepath.Join(dir, "classes")
	if err := os.Mkdir(classes, 0700); err != nil {
		t.Fatal(err)
	}
	androidJar := filepath.Join(sdk, "platforms", fmt.Sprintf("android-%d", api), "android.jar")
	run(filepath.Join(java, "bin", "javac"), "-source", "8", "-target", "8", "-cp", androidJar, "-d", classes,
		helper, view, "tests/android_mouse/NativeBridge.java", "tests/android_mouse/MouseInputTest.java")
	inputJar := filepath.Join(dir, "input.jar")
	run(filepath.Join(java, "bin", "jar"), "cf", inputJar, "-C", classes, ".")
	toolDir := filepath.Join(sdk, "build-tools", buildTools)
	run(filepath.Join(toolDir, "d8"), "--lib", androidJar, "--min-api", "26", "--output", dir, inputJar)
	apk := filepath.Join(dir, "mouse-test.apk")
	run(filepath.Join(toolDir, "aapt2"), "link", "-I", androidJar, "--manifest", "tests/android_mouse/AndroidManifest.xml", "-o", apk)
	run(filepath.Join(java, "bin", "jar"), "uf", apk, "-C", dir, "classes.dex")
	key := filepath.Join(dir, "test.keystore")
	run(filepath.Join(java, "bin", "keytool"), "-genkeypair", "-keystore", key, "-alias", "test", "-storepass", "android",
		"-keypass", "android", "-keyalg", "RSA", "-keysize", "2048", "-validity", "1", "-dname", "CN=Onix Input Test")
	run(filepath.Join(toolDir, "apksigner"), "sign", "--ks", key, "--ks-pass", "pass:android", apk)
	if installed := run("adb", "-s", serial, "shell", "pm", "list", "packages", "dev.onix.mousetest"); strings.TrimSpace(installed) != "" {
		t.Fatal("remove the existing dev.onix.mousetest fixture before running this test")
	}
	run("adb", "-s", serial, "install", "-t", apk)
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = exec.CommandContext(cleanup, "adb", "-s", serial, "uninstall", "dev.onix.mousetest").Run()
	}()
	output := run("adb", "-s", serial, "shell", "am", "instrument", "-w", "-r", "dev.onix.mousetest/.MouseInputTest")
	if !strings.Contains(output, "PASS: button-free hover") || !strings.Contains(output, "INSTRUMENTATION_CODE: -1") {
		t.Fatalf("Android mouse input: %s", output)
	}
	t.Log(strings.TrimSpace(output))
}
