package __JAVA_PACKAGE__;

import android.view.Surface;
import android.widget.FrameLayout;

public final class NativeBridge {
	private static SystemWebView systemWebView;
    static {
        System.loadLibrary("onix");
        System.loadLibrary("onixbridge");
    }

	private NativeBridge() {}

	public static void initPlatformServices(FrameLayout rootView) {
		systemWebView = new SystemWebView(rootView);
	}

	public static void shutdownPlatformServices() {
		if (systemWebView != null) systemWebView.close();
		systemWebView = null;
	}

    public static void surfaceCreated(Surface surface, int width, int height) {
        nativeSurfaceCreated(surface, width, height);
    }

    public static void surfaceDestroyed() {
        nativeSurfaceDestroyed();
    }

    public static void start(int width, int height) {
        nativeStart(width, height);
    }

    public static void resize(int width, int height) {
        nativeResize(width, height);
    }

    public static void setDevicePixelRatio(float ratio) {
        nativeSetDevicePixelRatio(ratio);
    }

	public static void update() {
		nativeUpdate();
		if (systemWebView != null) systemWebView.sync();
    }

    public static long textInputConfig() {
        return nativeTextInputConfig();
    }

    public static String textInputValue() {
        return nativeTextInputValue();
    }

    public static int textInputSelectionStart() { return nativeTextInputSelectionStart(); }
    public static int textInputSelectionEnd() { return nativeTextInputSelectionEnd(); }
    public static void textSelection(long session, int start, int end) { nativeTextSelection(session, start, end); }

    public static void touchStart(float x, float y) {
        nativeTouchStart(x, y);
    }

    public static void touchMove(float x, float y) {
        nativeTouchMove(x, y);
    }

    public static void touchEnd(float x, float y) {
        nativeTouchEnd(x, y);
    }

    public static void mouseMove(float x, float y) {
        nativeMouseMove(x, y);
    }

    public static void mouseButton(float x, float y, int button, boolean pressed) {
        nativeMouseButton(x, y, button, pressed);
    }

    public static void mouseWheel(float x, float y) {
        nativeMouseWheel(x, y);
    }

    public static void textInput(long session, String text) {
        nativeTextInput(session, text);
    }

    public static void textComposition(long session, String text, boolean active) {
        nativeTextComposition(session, text, active);
    }

	public static void textCommand(long session, int command) {
        nativeTextCommand(session, command);
	}

	static long webViewCommand() { return nativeWebViewCommand(); }
	static long webViewCommandID() { return nativeWebViewCommandID(); }
	static int webViewCommandFlags() { return nativeWebViewCommandFlags(); }
	static String webViewCommandText() { return nativeWebViewCommandText(); }
	static float webViewCommandX() { return nativeWebViewCommandX(); }
	static float webViewCommandY() { return nativeWebViewCommandY(); }
	static float webViewCommandWidth() { return nativeWebViewCommandWidth(); }
	static float webViewCommandHeight() { return nativeWebViewCommandHeight(); }
	static void webViewCommandComplete(long sequence) { nativeWebViewCommandComplete(sequence); }
	static void webViewEvent(long id, int event, String value) { nativeWebViewEvent(id, event, value); }
	static String webViewInvoke(long id, String name, String payload) {
		nativeWebViewInvoke(id, name, payload);
		String error = nativeWebViewInvokeError(id);
		if (!error.isEmpty()) throw new IllegalStateException(error);
		return nativeWebViewInvokeResult(id);
	}

    private static native void nativeSurfaceCreated(Surface surface, int width, int height);

    private static native void nativeSurfaceDestroyed();

    private static native void nativeStart(int width, int height);

    private static native void nativeResize(int width, int height);

    private static native void nativeSetDevicePixelRatio(float ratio);

    private static native void nativeUpdate();

    private static native long nativeTextInputConfig();

    private static native String nativeTextInputValue();

    private static native int nativeTextInputSelectionStart();

    private static native int nativeTextInputSelectionEnd();

    private static native void nativeTextSelection(long session, int start, int end);

    private static native void nativeTouchStart(float x, float y);

    private static native void nativeTouchMove(float x, float y);

    private static native void nativeTouchEnd(float x, float y);

    private static native void nativeMouseMove(float x, float y);

    private static native void nativeMouseButton(float x, float y, int button, boolean pressed);

    private static native void nativeMouseWheel(float x, float y);

    private static native void nativeTextInput(long session, String text);

    private static native void nativeTextComposition(long session, String text, boolean active);

	private static native void nativeTextCommand(long session, int command);

	private static native long nativeWebViewCommand();
	private static native long nativeWebViewCommandID();
	private static native int nativeWebViewCommandFlags();
	private static native String nativeWebViewCommandText();
	private static native float nativeWebViewCommandX();
	private static native float nativeWebViewCommandY();
	private static native float nativeWebViewCommandWidth();
	private static native float nativeWebViewCommandHeight();
	private static native void nativeWebViewCommandComplete(long sequence);
	private static native void nativeWebViewEvent(long id, int event, String value);
	private static native void nativeWebViewInvoke(long id, String name, String payload);
	private static native String nativeWebViewInvokeResult(long id);
	private static native String nativeWebViewInvokeError(long id);
}
