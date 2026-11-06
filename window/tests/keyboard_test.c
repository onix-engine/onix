#include "../linux_native.c"

#include <X11/Xlib.h>
#include <X11/extensions/XTest.h>
#include <X11/keysym.h>
#include <assert.h>
#include <stdio.h>
#include <stdlib.h>

static unsigned int modifiers;
static int last_key;
static unsigned int last_modifiers;

static void drain_keys(OnixNativeWindow *window) {
	OnixEvent event;
	while (onix_window_pop_event(window, &event)) {
		if (event.kind != ONIX_EVENT_KEY_DOWN && event.kind != ONIX_EVENT_KEY_UP) continue;
		if (event.arg0 >= 340 && event.arg0 <= 347) {
			unsigned int bit = 1u << (event.arg0 - 340);
			if (event.kind == ONIX_EVENT_KEY_DOWN) modifiers |= bit;
			else modifiers &= ~bit;
		} else if (event.kind == ONIX_EVENT_KEY_DOWN) {
			last_key = event.arg0;
			last_modifiers = modifiers;
		}
	}
}

static void poll_window(OnixNativeWindow *window) {
	XSync(onix_window_native_display(window), False);
	onix_window_poll_events(window);
	drain_keys(window);
}

static void test_wayland_modifiers(void) {
	OnixNativeWindow *window = onix_alloc_window(320, 240);
	assert(window != NULL);
	struct xkb_context *context = xkb_context_new(XKB_CONTEXT_NO_FLAGS);
	assert(context != NULL);
	struct xkb_keymap *keymap = xkb_keymap_new_from_names(context, NULL, XKB_KEYMAP_COMPILE_NO_FLAGS);
	assert(keymap != NULL);
	window->state.wayland.xkb_state = xkb_state_new(keymap);
	assert(window->state.wayland.xkb_state != NULL);
	uint32_t held = (1u << xkb_keymap_mod_get_index(keymap, XKB_MOD_NAME_CTRL)) |
		(1u << xkb_keymap_mod_get_index(keymap, XKB_MOD_NAME_SHIFT));
	for (int right = 0; right < 2; right++) {
		onix_wayland_keyboard_key(window, NULL, 0, 0, right ? KEY_RIGHTCTRL : KEY_LEFTCTRL, WL_KEYBOARD_KEY_STATE_PRESSED);
		onix_wayland_keyboard_key(window, NULL, 0, 0, right ? KEY_RIGHTSHIFT : KEY_LEFTSHIFT, WL_KEYBOARD_KEY_STATE_PRESSED);
		onix_wayland_keyboard_modifiers(window, NULL, 0, held, 0, 0, 0);
		onix_wayland_keyboard_key(window, NULL, 0, 0, KEY_H, WL_KEYBOARD_KEY_STATE_PRESSED);
		drain_keys(window);
		assert(last_key == 'H' && last_modifiers == (right ? 0x30 : 0x03));
		onix_wayland_keyboard_key(window, NULL, 0, 0, KEY_H, WL_KEYBOARD_KEY_STATE_RELEASED);
		// The compositor's authoritative mask repairs missing releases.
		onix_wayland_keyboard_modifiers(window, NULL, 0, 0, 0, 0, 0);
		onix_wayland_keyboard_key(window, NULL, 0, 0, KEY_H, WL_KEYBOARD_KEY_STATE_PRESSED);
		drain_keys(window);
		assert(last_key == 'H' && last_modifiers == 0);
		onix_wayland_keyboard_key(window, NULL, 0, 0, KEY_H, WL_KEYBOARD_KEY_STATE_RELEASED);
		// Modifiers held when entering are restored; leaving releases them.
		onix_wayland_keyboard_modifiers(window, NULL, 0, held, 0, 0, 0);
		drain_keys(window);
		assert(modifiers == 0x03);
		onix_wayland_keyboard_leave(window, NULL, 0, NULL);
		drain_keys(window);
		assert(modifiers == 0);
		onix_wayland_keyboard_modifiers(window, NULL, 0, 0, 0, 0, 0);
	}
	xkb_state_unref(window->state.wayland.xkb_state);
	xkb_keymap_unref(keymap);
	xkb_context_unref(context);
	free(window);
}

static void key(Display *display, KeySym sym, int pressed) {
	XTestFakeKeyEvent(display, XKeysymToKeycode(display, sym), pressed, CurrentTime);
}

static void send_key(Display *display, Window native, int type, KeySym sym, unsigned int state) {
	XEvent event = {0};
	event.xkey.type = type;
	event.xkey.display = display;
	event.xkey.window = native;
	event.xkey.root = DefaultRootWindow(display);
	event.xkey.keycode = XKeysymToKeycode(display, sym);
	event.xkey.state = state;
	event.xkey.same_screen = True;
	XSendEvent(display, native, False, type == KeyPress ? KeyPressMask : KeyReleaseMask, &event);
}

int main(void) {
	setenv("ONIX_LINUX_BACKEND", "x11", 1);
	OnixNativeWindow *window = onix_window_create(320, 240, "Onix keyboard test", 0, 0);
	assert(window != NULL);
	Display *display = onix_window_native_display(window);
	Window native = onix_window_native_surface(window);
	XSetInputFocus(display, native, RevertToParent, CurrentTime);
	poll_window(window);

	// Genuine shortcuts retain either side of each modifier.
	for (int right = 0; right < 2; right++) {
		KeySym control = right ? XK_Control_R : XK_Control_L;
		KeySym shift = right ? XK_Shift_R : XK_Shift_L;
		key(display, control, True);
		key(display, shift, True);
		key(display, XK_h, True);
		poll_window(window);
		assert(last_key == 'H' && last_modifiers == (right ? 0x30 : 0x03));
		key(display, XK_h, False);
		key(display, shift, False);
		key(display, control, False);
		poll_window(window);
		assert(modifiers == 0);
	}

	// Missing key-up events (e.g. consumed by an IME) cannot poison later typing.
	send_key(display, native, KeyPress, XK_Control_L, 0);
	send_key(display, native, KeyPress, XK_Shift_L, ControlMask);
	poll_window(window);
	assert(modifiers == 0x03);
	send_key(display, native, KeyPress, XK_c, 0);
	send_key(display, native, KeyRelease, XK_c, 0);
	send_key(display, native, KeyPress, XK_h, 0);
	poll_window(window);
	if (last_key != 'H' || last_modifiers != 0) {
		fprintf(stderr, "plain ch inherited stale modifiers: key=%d modifiers=%u\n", last_key, last_modifiers);
		return 1;
	}
	send_key(display, native, KeyRelease, XK_h, 0);
	poll_window(window);

	// Focus loss must release modifiers even when releases go to another window.
	key(display, XK_Control_L, True);
	key(display, XK_Shift_L, True);
	poll_window(window);
	assert(modifiers == 0x03);
	XSetInputFocus(display, DefaultRootWindow(display), RevertToParent, CurrentTime);
	poll_window(window);
	assert(modifiers == 0);
	key(display, XK_Control_L, False);
	key(display, XK_Shift_L, False);
	XSetInputFocus(display, native, RevertToParent, CurrentTime);
	key(display, XK_c, True);
	key(display, XK_c, False);
	key(display, XK_h, True);
	poll_window(window);
	assert(last_key == 'H' && last_modifiers == 0);
	key(display, XK_h, False);
	poll_window(window);
	onix_window_destroy(window);
	free(window);
	test_wayland_modifiers();
	return 0;
}
