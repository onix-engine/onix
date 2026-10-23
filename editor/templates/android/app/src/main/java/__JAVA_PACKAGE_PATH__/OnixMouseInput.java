package __JAVA_PACKAGE__;

import android.view.InputDevice;
import android.view.MotionEvent;

final class OnixMouseInput {
    private int buttons;
    private float x;
    private float y;

    boolean handle(MotionEvent event) {
        if (!event.isFromSource(InputDevice.SOURCE_MOUSE)
                && event.getToolType(0) != MotionEvent.TOOL_TYPE_MOUSE) {
            return false;
        }
        switch (event.getActionMasked()) {
            case MotionEvent.ACTION_CANCEL:
            case MotionEvent.ACTION_HOVER_EXIT:
                release();
                return true;
            case MotionEvent.ACTION_DOWN:
            case MotionEvent.ACTION_UP:
            case MotionEvent.ACTION_MOVE:
            case MotionEvent.ACTION_HOVER_ENTER:
            case MotionEvent.ACTION_HOVER_MOVE:
            case MotionEvent.ACTION_BUTTON_PRESS:
            case MotionEvent.ACTION_BUTTON_RELEASE:
            case MotionEvent.ACTION_SCROLL:
                x = event.getX();
                y = event.getY();
                updateButtons(event.getButtonState());
                // Surface callbacks and frames run on the UI thread. Dispatch directly
                // so mouse motion neither waits for a posted task nor allocates one.
                NativeBridge.mouseMove(x, y);
                if (event.getActionMasked() == MotionEvent.ACTION_SCROLL) {
                    NativeBridge.mouseWheel(event.getAxisValue(MotionEvent.AXIS_HSCROLL),
                            event.getAxisValue(MotionEvent.AXIS_VSCROLL));
                }
                return true;
            default:
                return false;
        }
    }

    void release() {
        updateButtons(0);
    }

    private void updateButtons(int next) {
        int changed = buttons ^ next;
        for (int button = MotionEvent.BUTTON_PRIMARY; button <= MotionEvent.BUTTON_TERTIARY; button <<= 1) {
            if ((changed & button) != 0) {
                NativeBridge.mouseButton(x, y, button, (next & button) != 0);
            }
        }
        buttons = next;
    }
}
