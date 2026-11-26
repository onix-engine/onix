# Android builds

The editor's Build Project panel produces arm64 Android APKs in the project's
`dist/android/app/build/outputs/apk/`. Debug builds use Android debug signing.
Release APKs are currently unsigned and need a production key before publication.

Toolchain discovery works even when the editor is launched outside a terminal.
SDK lookup checks the panel's explicit directory, `ANDROID_HOME`,
`ANDROID_SDK_ROOT`, platform defaults and SDK tools on `PATH`, in that order.
NDK lookup prefers the panel or `ANDROID_NDK_HOME` / `ANDROID_NDK_ROOT`, then the
highest installed side-by-side NDK. Java lookup checks `JAVA_HOME` and `PATH`,
then Android Studio, `~/Android/jdk-17` and system JDK locations.

The Gradle template requires Java 17 or newer and defaults to API 35, Build Tools
35.0.0 and CMake 3.22.1. Available installed versions are selected when defaults
are missing. SDK platforms, Build Tools, CMake, NDK Clang and Java are validated
before building; missing components are reported in the editor.

Android and iOS staging generate a mobile entry point for projects that only
provide standard `main()`. To customize initialization, declare:

```go
func OnixEntryStart(surfaceWidth, surfaceHeight int) error
```

An explicit entry point takes precedence. The mobile host drives each frame;
`App.Run` registers the callback and retains application state.
