#include <jni.h>
#include <android/native_window_jni.h>
#include <stdint.h>
#include <stdlib.h>

void OnixStart(int width, int height);
void OnixSetNativeWindow(uintptr_t surface, int width, int height);
void OnixClearNativeWindow(void);
void OnixResize(int width, int height);
void OnixSetDevicePixelRatio(float ratio);
void OnixUpdate(void);
uint64_t OnixTextInputConfig(void);
int OnixTextInputValue(uint16_t *buffer, int capacity);
int OnixTextInputSelectionStart(void);
int OnixTextInputSelectionEnd(void);
void OnixTextSelection(uint64_t session, int start, int end);
void OnixTouchStart(float x, float y);
void OnixTouchMove(float x, float y);
void OnixTouchEnd(float x, float y);
void OnixMouseMove(float x, float y);
void OnixMouseButton(float x, float y, int button, int pressed);
void OnixMouseWheel(float x, float y);
void OnixTextInput(uint64_t session, const uint16_t *text, int length);
void OnixTextComposition(uint64_t session, const uint16_t *text, int length, int active);
void OnixTextCommand(uint64_t session, int command);
uint64_t OnixWebViewCommand(void);
uint64_t OnixWebViewCommandID(void);
int OnixWebViewCommandFlags(void);
int OnixWebViewCommandText(uint16_t *buffer, int capacity);
float OnixWebViewCommandX(void);
float OnixWebViewCommandY(void);
float OnixWebViewCommandWidth(void);
float OnixWebViewCommandHeight(void);
void OnixWebViewCommandComplete(uint64_t sequence);
void OnixWebViewEvent(uint64_t id, int event, const uint16_t *value, int length);
void OnixWebViewInvoke(uint64_t id, const uint16_t *name, int name_length, const uint16_t *payload, int payload_length);
int OnixWebViewInvokeResult(uint64_t id, uint16_t *buffer, int capacity);
int OnixWebViewInvokeError(uint64_t id, uint16_t *buffer, int capacity);

static ANativeWindow *onix_window;

static jstring onix_utf16_string(JNIEnv *env, int (*read_value)(uint16_t *, int)) {
    int length = read_value(NULL, 0);
    if (length <= 0) {
        const jchar empty = 0;
        return (*env)->NewString(env, &empty, 0);
    }
    uint16_t *buffer = malloc((size_t)length * sizeof(uint16_t));
    if (buffer == NULL) {
        return NULL;
    }
    read_value(buffer, length);
    jstring value = (*env)->NewString(env, (const jchar *)buffer, (jsize)length);
    free(buffer);
    return value;
}

static jstring onix_utf16_string_for_id(JNIEnv *env, uint64_t id, int (*read_value)(uint64_t, uint16_t *, int)) {
    int length = read_value(id, NULL, 0);
    if (length <= 0) {
        const jchar empty = 0;
        return (*env)->NewString(env, &empty, 0);
    }
    uint16_t *buffer = malloc((size_t)length * sizeof(uint16_t));
    if (buffer == NULL) {
        return NULL;
    }
    read_value(id, buffer, length);
    jstring value = (*env)->NewString(env, (const jchar *)buffer, (jsize)length);
    free(buffer);
    return value;
}

JNIEXPORT void JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeSurfaceCreated(JNIEnv *env, jclass clazz, jobject surface, jint width, jint height) {
    (void)clazz;
    if (onix_window != NULL) {
        OnixClearNativeWindow();
        ANativeWindow_release(onix_window);
        onix_window = NULL;
    }
    onix_window = ANativeWindow_fromSurface(env, surface);
    if (onix_window != NULL) {
        OnixSetNativeWindow((uintptr_t)onix_window, (int)width, (int)height);
    }
}

JNIEXPORT void JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeSurfaceDestroyed(JNIEnv *env, jclass clazz) {
    (void)env;
    (void)clazz;
    if (onix_window == NULL) {
        return;
    }
    OnixClearNativeWindow();
    ANativeWindow_release(onix_window);
    onix_window = NULL;
}

JNIEXPORT void JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeStart(JNIEnv *env, jclass clazz, jint width, jint height) {
    (void)env;
    (void)clazz;
    OnixStart((int)width, (int)height);
}

JNIEXPORT void JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeResize(JNIEnv *env, jclass clazz, jint width, jint height) {
    (void)env;
    (void)clazz;
    OnixResize((int)width, (int)height);
}

JNIEXPORT void JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeSetDevicePixelRatio(JNIEnv *env, jclass clazz, jfloat ratio) {
    (void)env;
    (void)clazz;
    OnixSetDevicePixelRatio((float)ratio);
}

JNIEXPORT void JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeUpdate(JNIEnv *env, jclass clazz) {
    (void)env;
    (void)clazz;
    OnixUpdate();
}

JNIEXPORT jlong JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeTextInputConfig(JNIEnv *env, jclass clazz) {
    (void)env;
    (void)clazz;
    return (jlong)OnixTextInputConfig();
}

JNIEXPORT jstring JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeTextInputValue(JNIEnv *env, jclass clazz) {
	(void)clazz;
	return onix_utf16_string(env, OnixTextInputValue);
}

JNIEXPORT jint JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeTextInputSelectionStart(JNIEnv *env, jclass clazz) {
    (void)env; (void)clazz;
    return (jint)OnixTextInputSelectionStart();
}

JNIEXPORT jint JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeTextInputSelectionEnd(JNIEnv *env, jclass clazz) {
    (void)env; (void)clazz;
    return (jint)OnixTextInputSelectionEnd();
}

JNIEXPORT void JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeTextSelection(JNIEnv *env, jclass clazz, jlong session, jint start, jint end) {
    (void)env; (void)clazz;
    OnixTextSelection((uint64_t)session, (int)start, (int)end);
}

JNIEXPORT void JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeTouchStart(JNIEnv *env, jclass clazz, jfloat x, jfloat y) {
    (void)env;
    (void)clazz;
    OnixTouchStart((float)x, (float)y);
}

JNIEXPORT void JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeTouchMove(JNIEnv *env, jclass clazz, jfloat x, jfloat y) {
    (void)env;
    (void)clazz;
    OnixTouchMove((float)x, (float)y);
}

JNIEXPORT void JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeTouchEnd(JNIEnv *env, jclass clazz, jfloat x, jfloat y) {
    (void)env;
    (void)clazz;
    OnixTouchEnd((float)x, (float)y);
}

JNIEXPORT void JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeMouseMove(JNIEnv *env, jclass clazz, jfloat x, jfloat y) {
    (void)env;
    (void)clazz;
    OnixMouseMove((float)x, (float)y);
}

JNIEXPORT void JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeMouseButton(JNIEnv *env, jclass clazz, jfloat x, jfloat y, jint button, jboolean pressed) {
    (void)env;
    (void)clazz;
    OnixMouseButton((float)x, (float)y, (int)button, pressed == JNI_TRUE);
}

JNIEXPORT void JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeMouseWheel(JNIEnv *env, jclass clazz, jfloat x, jfloat y) {
    (void)env;
    (void)clazz;
    OnixMouseWheel((float)x, (float)y);
}

JNIEXPORT void JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeTextInput(JNIEnv *env, jclass clazz, jlong session, jstring text) {
    (void)clazz;
    if (text == NULL) {
        return;
    }
    jsize length = (*env)->GetStringLength(env, text);
    const jchar *chars = (*env)->GetStringChars(env, text, NULL);
    if (chars != NULL) {
        OnixTextInput((uint64_t)session, (const uint16_t *)chars, (int)length);
        (*env)->ReleaseStringChars(env, text, chars);
    }
}

JNIEXPORT void JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeTextComposition(JNIEnv *env, jclass clazz, jlong session, jstring text, jboolean active) {
    (void)clazz;
    if (text == NULL) {
        return;
    }
    jsize length = (*env)->GetStringLength(env, text);
    const jchar *chars = (*env)->GetStringChars(env, text, NULL);
    if (chars != NULL) {
        OnixTextComposition((uint64_t)session, (const uint16_t *)chars, (int)length, active ? 1 : 0);
        (*env)->ReleaseStringChars(env, text, chars);
    }
}

JNIEXPORT void JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeTextCommand(JNIEnv *env, jclass clazz, jlong session, jint command) {
    (void)env;
    (void)clazz;
	OnixTextCommand((uint64_t)session, (int)command);
}

JNIEXPORT jlong JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeWebViewCommand(JNIEnv *env, jclass clazz) {
	(void)env; (void)clazz;
	return (jlong)OnixWebViewCommand();
}

JNIEXPORT jlong JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeWebViewCommandID(JNIEnv *env, jclass clazz) {
	(void)env; (void)clazz;
	return (jlong)OnixWebViewCommandID();
}

JNIEXPORT jint JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeWebViewCommandFlags(JNIEnv *env, jclass clazz) {
	(void)env; (void)clazz;
	return (jint)OnixWebViewCommandFlags();
}

JNIEXPORT jstring JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeWebViewCommandText(JNIEnv *env, jclass clazz) {
	(void)clazz;
	return onix_utf16_string(env, OnixWebViewCommandText);
}

JNIEXPORT jfloat JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeWebViewCommandX(JNIEnv *env, jclass clazz) {
	(void)env; (void)clazz;
	return (jfloat)OnixWebViewCommandX();
}

JNIEXPORT jfloat JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeWebViewCommandY(JNIEnv *env, jclass clazz) {
	(void)env; (void)clazz;
	return (jfloat)OnixWebViewCommandY();
}

JNIEXPORT jfloat JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeWebViewCommandWidth(JNIEnv *env, jclass clazz) {
	(void)env; (void)clazz;
	return (jfloat)OnixWebViewCommandWidth();
}

JNIEXPORT jfloat JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeWebViewCommandHeight(JNIEnv *env, jclass clazz) {
	(void)env; (void)clazz;
	return (jfloat)OnixWebViewCommandHeight();
}

JNIEXPORT void JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeWebViewCommandComplete(JNIEnv *env, jclass clazz, jlong sequence) {
	(void)env; (void)clazz;
	OnixWebViewCommandComplete((uint64_t)sequence);
}

JNIEXPORT void JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeWebViewEvent(JNIEnv *env, jclass clazz, jlong id, jint event, jstring value) {
	(void)clazz;
	if (value == NULL) {
		OnixWebViewEvent((uint64_t)id, (int)event, NULL, 0);
		return;
	}
	jsize length = (*env)->GetStringLength(env, value);
	const jchar *chars = (*env)->GetStringChars(env, value, NULL);
	if (chars == NULL) return;
	OnixWebViewEvent((uint64_t)id, (int)event, (const uint16_t *)chars, (int)length);
	(*env)->ReleaseStringChars(env, value, chars);
}

JNIEXPORT void JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeWebViewInvoke(JNIEnv *env, jclass clazz, jlong id, jstring name, jstring payload) {
	(void)clazz;
	const jchar *name_chars = name == NULL ? NULL : (*env)->GetStringChars(env, name, NULL);
	const jchar *payload_chars = payload == NULL ? NULL : (*env)->GetStringChars(env, payload, NULL);
	if ((name != NULL && name_chars == NULL) || (payload != NULL && payload_chars == NULL)) {
		if (name_chars != NULL) (*env)->ReleaseStringChars(env, name, name_chars);
		if (payload_chars != NULL) (*env)->ReleaseStringChars(env, payload, payload_chars);
		return;
	}
	jsize name_length = name == NULL ? 0 : (*env)->GetStringLength(env, name);
	jsize payload_length = payload == NULL ? 0 : (*env)->GetStringLength(env, payload);
	OnixWebViewInvoke(
		(uint64_t)id,
		(const uint16_t *)name_chars,
		(int)name_length,
		(const uint16_t *)payload_chars,
		(int)payload_length
	);
	if (name_chars != NULL) (*env)->ReleaseStringChars(env, name, name_chars);
	if (payload_chars != NULL) (*env)->ReleaseStringChars(env, payload, payload_chars);
}

JNIEXPORT jstring JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeWebViewInvokeResult(JNIEnv *env, jclass clazz, jlong id) {
	(void)clazz;
	return onix_utf16_string_for_id(env, (uint64_t)id, OnixWebViewInvokeResult);
}

JNIEXPORT jstring JNICALL
__JNI_NATIVE_BRIDGE_PREFIX___nativeWebViewInvokeError(JNIEnv *env, jclass clazz, jlong id) {
	(void)clazz;
	return onix_utf16_string_for_id(env, (uint64_t)id, OnixWebViewInvokeError);
}
