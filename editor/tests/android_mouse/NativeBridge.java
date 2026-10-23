package dev.onix.mousetest;

final class NativeBridge {
    static int moves;
    static int buttons;
    static int presses;
    static int releases;
    static float x, y, wheelX, wheelY;

    static void mouseMove(float nextX, float nextY) {
        moves++;
        x = nextX;
        y = nextY;
    }

    static void mouseButton(float nextX, float nextY, int button, boolean pressed) {
        if (pressed) {
            buttons |= button;
            presses++;
        } else {
            buttons &= ~button;
            releases++;
        }
        x = nextX;
        y = nextY;
    }

    static void mouseWheel(float nextX, float nextY) {
        wheelX = nextX;
        wheelY = nextY;
    }

    static void resize(int width, int height) {}
    static void setDevicePixelRatio(float ratio) {}
    static void start(int width, int height) {}
    static void surfaceCreated(android.view.Surface surface, int width, int height) {}
    static void surfaceDestroyed() {}
    static void update() {}
    static void touchStart(float x, float y) {}
    static void touchMove(float x, float y) {}
    static void touchEnd(float x, float y) {}
    static long textInputConfig() { return 0; }
    static String textInputValue() { return ""; }
    static int textInputSelectionStart() { return 0; }
    static int textInputSelectionEnd() { return 0; }
    static void textCommand(long session, int command) {}
    static void textComposition(long session, String text, boolean active) {}
    static void textInput(long session, String text) {}
    static void textSelection(long session, int start, int end) {}
}
