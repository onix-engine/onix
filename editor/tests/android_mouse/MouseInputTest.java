package dev.onix.mousetest;

import android.os.SystemClock;
import android.view.InputDevice;
import android.view.MotionEvent;

public final class MouseInputTest extends android.app.Instrumentation {
    @Override
    public void onCreate(android.os.Bundle arguments) {
        super.onCreate(arguments);
        start();
    }

    @Override
    public void onStart() {
        android.os.Bundle result = new android.os.Bundle();
        runOnMainSync(() -> {
            try {
                runTests();
                result.putString("stream", "PASS: button-free hover through SurfaceView in four directions, mouse buttons, cancel, pause, wheel and touch separation\n");
            } catch (Throwable error) {
                result.putString("stream", android.util.Log.getStackTraceString(error));
                result.putBoolean("failed", true);
            }
        });
        finish(result.getBoolean("failed") ? 0 : android.app.Activity.RESULT_OK, result);
    }

    private static void check(boolean condition, String message) {
        if (!condition) throw new AssertionError(message);
    }

    private static MotionEvent event(int action, int buttons, float x, float y, boolean mouse) {
        MotionEvent.PointerProperties pointer = new MotionEvent.PointerProperties();
        pointer.id = 0;
        pointer.toolType = mouse ? MotionEvent.TOOL_TYPE_MOUSE : MotionEvent.TOOL_TYPE_FINGER;
        MotionEvent.PointerCoords coordinates = new MotionEvent.PointerCoords();
        coordinates.x = x;
        coordinates.y = y;
        coordinates.setAxisValue(MotionEvent.AXIS_HSCROLL, 2);
        coordinates.setAxisValue(MotionEvent.AXIS_VSCROLL, -3);
        long now = SystemClock.uptimeMillis();
        return MotionEvent.obtain(now, now, action, 1,
                new MotionEvent.PointerProperties[]{pointer}, new MotionEvent.PointerCoords[]{coordinates},
                0, buttons, 1, 1, 0, 0, mouse ? InputDevice.SOURCE_MOUSE : InputDevice.SOURCE_TOUCHSCREEN, 0);
    }

    private static void dispatch(OnixMouseInput input, int action, int buttons, float x, float y) {
        MotionEvent event = event(action, buttons, x, y, true);
        try {
            check(input.handle(event), "mouse event was not handled: " + action);
        } finally {
            event.recycle();
        }
    }

    private void runTests() {
        OnixMouseInput input = new OnixMouseInput();
        dispatch(input, MotionEvent.ACTION_HOVER_ENTER, 0, 400, 300);
        float[][] positions = {{420, 300}, {380, 300}, {400, 280}, {400, 320}};
        for (float[] position : positions) {
            int moves = NativeBridge.moves;
            dispatch(input, MotionEvent.ACTION_HOVER_MOVE, 0, position[0], position[1]);
            check(NativeBridge.moves == moves + 1, "hover did not forward mouse motion");
            check(NativeBridge.x == position[0] && NativeBridge.y == position[1], "hover coordinates changed");
            check(NativeBridge.buttons == 0 && NativeBridge.presses == 0, "hover synthesized a button press");
        }

        dispatch(input, MotionEvent.ACTION_DOWN, MotionEvent.BUTTON_PRIMARY, 400, 320);
        dispatch(input, MotionEvent.ACTION_BUTTON_PRESS, MotionEvent.BUTTON_PRIMARY, 400, 320);
        check(NativeBridge.presses == 1, "mouse down and button press were duplicated");
        dispatch(input, MotionEvent.ACTION_MOVE, MotionEvent.BUTTON_PRIMARY, 450, 340);
        check(NativeBridge.buttons == MotionEvent.BUTTON_PRIMARY && NativeBridge.x == 450, "drag lost button or motion");
        dispatch(input, MotionEvent.ACTION_BUTTON_PRESS, MotionEvent.BUTTON_PRIMARY | MotionEvent.BUTTON_SECONDARY, 450, 340);
        check(NativeBridge.presses == 2, "right button was not forwarded independently");
        dispatch(input, MotionEvent.ACTION_UP, MotionEvent.BUTTON_SECONDARY, 450, 340);
        check(NativeBridge.buttons == MotionEvent.BUTTON_SECONDARY, "left release also released right button");
        dispatch(input, MotionEvent.ACTION_BUTTON_RELEASE, 0, 450, 340);
        check(NativeBridge.buttons == 0 && NativeBridge.releases == 2, "mouse release was lost");

        dispatch(input, MotionEvent.ACTION_BUTTON_PRESS, MotionEvent.BUTTON_TERTIARY, 450, 340);
        dispatch(input, MotionEvent.ACTION_CANCEL, 0, 450, 340);
        check(NativeBridge.buttons == 0 && NativeBridge.releases == 3, "cancel left a held button");
        dispatch(input, MotionEvent.ACTION_DOWN, MotionEvent.BUTTON_PRIMARY, 450, 340);
        input.release();
        input.release();
        check(NativeBridge.buttons == 0 && NativeBridge.releases == 4, "pause release was lost or duplicated");

        dispatch(input, MotionEvent.ACTION_SCROLL, 0, 450, 340);
        check(NativeBridge.wheelX == 2 && NativeBridge.wheelY == -3, "wheel axes were not forwarded");
        int moves = NativeBridge.moves;
        MotionEvent touch = event(MotionEvent.ACTION_MOVE, 0, 500, 350, false);
        check(!input.handle(touch) && NativeBridge.moves == moves, "touch was interpreted as mouse motion");
        touch.recycle();

        OnixSurfaceView view = new OnixSurfaceView(getTargetContext());
        view.layout(0, 0, 1280, 800);
        for (float[] position : positions) {
            MotionEvent hover = event(MotionEvent.ACTION_HOVER_MOVE, 0, position[0], position[1], true);
            moves = NativeBridge.moves;
            check(view.dispatchGenericMotionEvent(hover), "view did not handle hover");
            check(NativeBridge.moves == moves + 1, "view swallowed or duplicated button-free mouse motion");
            check(NativeBridge.buttons == 0, "view hover pressed a button");
            hover.recycle();
        }
    }
}
