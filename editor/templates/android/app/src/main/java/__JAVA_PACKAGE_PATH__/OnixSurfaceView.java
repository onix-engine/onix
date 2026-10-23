package __JAVA_PACKAGE__;

import android.content.Context;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.text.InputType;
import android.view.Choreographer;
import android.view.ActionMode;
import android.view.KeyEvent;
import android.view.Menu;
import android.view.MenuItem;
import android.view.MotionEvent;
import android.view.SurfaceHolder;
import android.view.SurfaceView;
import android.view.inputmethod.BaseInputConnection;
import android.view.inputmethod.EditorInfo;
import android.view.inputmethod.InputConnection;
import android.view.inputmethod.InputMethodManager;

public final class OnixSurfaceView extends SurfaceView implements SurfaceHolder.Callback, Choreographer.FrameCallback {
    private static final long TEXT_INPUT_ACTIVE = 1L;
    private static final long TEXT_INPUT_MULTILINE = 1L << 1;
    private static final int TEXT_INPUT_KIND_DECIMAL = 1;
    private static final int TEXT_INPUT_KIND_INTEGER = 2;
    private static final int TEXT_COMMAND_BACKSPACE = 1;
    private static final int TEXT_COMMAND_ENTER = 2;

    private boolean started;
    private boolean surfaceReady;
    private boolean resumed;
    private boolean frameScheduled;
    private long textInputConfig = Long.MIN_VALUE;
    private String textInputInitialValue = "";
    private int textSelectionStart;
    private int textSelectionEnd;
    private InputConnection currentInputConnection;
    private final OnixMouseInput mouseInput = new OnixMouseInput();

    public OnixSurfaceView(Context context) {
        super(context);
        setFocusable(true);
        setFocusableInTouchMode(true);
        getHolder().addCallback(this);
		setOnLongClickListener(view -> {
			if (!textInputActive() || currentInputConnection == null) return false;
			startActionMode(new ActionMode.Callback() {
				@Override public boolean onCreateActionMode(ActionMode mode, Menu menu) {
					if (textSelectionStart != textSelectionEnd) {
						menu.add(0, android.R.id.copy, 0, "Copy");
						menu.add(0, android.R.id.cut, 1, "Cut");
					}
					menu.add(0, android.R.id.paste, 2, "Paste");
					return true;
				}
				@Override public boolean onPrepareActionMode(ActionMode mode, Menu menu) { return false; }
				@Override public boolean onActionItemClicked(ActionMode mode, MenuItem item) {
					boolean handled = currentInputConnection.performContextMenuAction(item.getItemId());
					mode.finish();
					return handled;
				}
				@Override public void onDestroyActionMode(ActionMode mode) {}
			});
			return true;
		});
    }

    @Override
    public void surfaceCreated(SurfaceHolder holder) {
        // Surface dimensions are delivered by surfaceChanged.
    }

    @Override
    public void surfaceChanged(SurfaceHolder holder, int format, int width, int height) {
        if (width <= 0 || height <= 0) {
            return;
        }
        NativeBridge.setDevicePixelRatio(resolveDevicePixelRatio(width, height));
        if (!surfaceReady) {
            NativeBridge.surfaceCreated(holder.getSurface(), width, height);
            surfaceReady = true;
        }
        if (!started) {
            NativeBridge.start(width, height);
            started = true;
        } else {
            NativeBridge.resize(width, height);
        }
        scheduleFrame();
    }

    @Override
    public void surfaceDestroyed(SurfaceHolder holder) {
        surfaceReady = false;
        frameScheduled = false;
        Choreographer.getInstance().removeFrameCallback(this);
        NativeBridge.surfaceDestroyed();
    }

    @Override
    public void doFrame(long frameTimeNanos) {
        frameScheduled = false;
        if (!resumed || !surfaceReady || !started) {
            return;
        }
        NativeBridge.update();
        final long nextTextInputConfig = NativeBridge.textInputConfig();
        if (nextTextInputConfig != textInputConfig) {
            textInputConfig = nextTextInputConfig;
            setTextInputConfig(nextTextInputConfig, NativeBridge.textInputValue());
        }
        if (textInputActive()) {
            textSelectionStart = NativeBridge.textInputSelectionStart();
            textSelectionEnd = NativeBridge.textInputSelectionEnd();
        }
        scheduleFrame();
    }

    public void onResume() {
        resumed = true;
        scheduleFrame();
    }

    public void onPause() {
        mouseInput.release();
        resumed = false;
        frameScheduled = false;
        Choreographer.getInstance().removeFrameCallback(this);
    }

    private void scheduleFrame() {
        if (!resumed || !surfaceReady || frameScheduled) {
            return;
        }
        frameScheduled = true;
        Choreographer.getInstance().postFrameCallback(this);
    }

    float resolveDevicePixelRatio(int surfaceWidth, int surfaceHeight) {
        final int viewWidth = Math.max(getWidth(), 0);
        final int viewHeight = Math.max(getHeight(), 0);

        float ratio = 0f;
        if (viewWidth > 0) {
            ratio = (float) surfaceWidth / (float) viewWidth;
        }
        if (ratio <= 0f && viewHeight > 0) {
            ratio = (float) surfaceHeight / (float) viewHeight;
        }
        if (ratio <= 0f) {
            ratio = 1f;
        }
        return ratio;
    }

    @Override
    public boolean onTouchEvent(MotionEvent event) {
        if (mouseInput.handle(event)) {
            return true;
        }
        final float x = event.getX();
        final float y = event.getY();

        switch (event.getActionMasked()) {
            case MotionEvent.ACTION_DOWN:
            case MotionEvent.ACTION_POINTER_DOWN:
                post(() -> NativeBridge.touchStart(x, y));
                return true;
            case MotionEvent.ACTION_MOVE:
                post(() -> NativeBridge.touchMove(x, y));
                return true;
            case MotionEvent.ACTION_UP:
            case MotionEvent.ACTION_POINTER_UP:
            case MotionEvent.ACTION_CANCEL:
                post(() -> NativeBridge.touchEnd(x, y));
                return true;
            default:
                return super.onTouchEvent(event);
        }
    }

    @Override
    public boolean onHoverEvent(MotionEvent event) {
        return mouseInput.handle(event) || super.onHoverEvent(event);
    }

    @Override
    public boolean onGenericMotionEvent(MotionEvent event) {
        return mouseInput.handle(event) || super.onGenericMotionEvent(event);
    }

    @Override
    public boolean onCheckIsTextEditor() {
        return textInputActive();
    }

    @Override
    public InputConnection onCreateInputConnection(EditorInfo outAttrs) {
        if (!textInputActive()) {
            return null;
        }
        final boolean multiline = (textInputConfig & TEXT_INPUT_MULTILINE) != 0;
        final int kind = (int)((textInputConfig >> 2) & 0x3);
        if (kind == TEXT_INPUT_KIND_DECIMAL || kind == TEXT_INPUT_KIND_INTEGER) {
            outAttrs.inputType = InputType.TYPE_CLASS_NUMBER | InputType.TYPE_NUMBER_FLAG_SIGNED;
            if (kind == TEXT_INPUT_KIND_DECIMAL) {
                outAttrs.inputType |= InputType.TYPE_NUMBER_FLAG_DECIMAL;
            }
        } else {
            outAttrs.inputType = InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS;
            if (multiline) {
                outAttrs.inputType |= InputType.TYPE_TEXT_FLAG_MULTI_LINE;
            }
        }
        outAttrs.imeOptions = EditorInfo.IME_FLAG_NO_EXTRACT_UI;
        outAttrs.imeOptions |= multiline ? EditorInfo.IME_FLAG_NO_ENTER_ACTION : EditorInfo.IME_ACTION_DONE;
        final long session = textInputConfig;
        final String initialValue = textInputInitialValue;
		BaseInputConnection connection = new BaseInputConnection(this, true) {
            private final StringBuilder committed = new StringBuilder(initialValue);

            @Override
            public boolean commitText(CharSequence text, int newCursorPosition) {
                if (text != null && text.length() > 0) {
                    final String rawValue = text.toString();
                    if (isLineBreak(rawValue) && !multiline) {
                        post(() -> NativeBridge.textCommand(session, TEXT_COMMAND_ENTER));
                    } else {
                        final String value = sanitizeText(rawValue, multiline);
                        if (!value.isEmpty()) {
							final int selectionStart = textSelectionStart;
							final int selectionEnd = textSelectionEnd;
							final int start = utf16Offset(committed, selectionStart);
							final int end = utf16Offset(committed, selectionEnd);
							post(() -> NativeBridge.textSelection(session, selectionStart, selectionEnd));
							committed.replace(Math.min(start, end), Math.max(start, end), value);
							textSelectionStart = Math.min(textSelectionStart, textSelectionEnd) + value.codePointCount(0, value.length());
							textSelectionEnd = textSelectionStart;
                            post(() -> NativeBridge.textInput(session, value));
                        }
                    }
                }
                post(() -> NativeBridge.textComposition(session, "", false));
                return true;
            }

            @Override
            public boolean setComposingText(CharSequence text, int newCursorPosition) {
                final String value = text == null ? "" : text.toString();
                post(() -> NativeBridge.textComposition(session, value, true));
                return true;
            }

            @Override
            public boolean finishComposingText() {
                post(() -> NativeBridge.textComposition(session, "", false));
                return true;
            }

            @Override
            public boolean deleteSurroundingText(int beforeLength, int afterLength) {
                if (beforeLength > 0) {
					final int start = textSelectionStart;
					final int end = textSelectionEnd;
					post(() -> NativeBridge.textSelection(session, start, end));
					if (start != end) { deleteSelected(); emitBackspaces(1); }
					else { emitBackspaces(Math.max(deleteCodeUnits(beforeLength), 1)); }
                }
                return true;
            }

            @Override
            public boolean deleteSurroundingTextInCodePoints(int beforeLength, int afterLength) {
                if (beforeLength > 0) {
					final int start = textSelectionStart;
					final int end = textSelectionEnd;
					post(() -> NativeBridge.textSelection(session, start, end));
					if (start != end) { deleteSelected(); emitBackspaces(1); }
					else { emitBackspaces(Math.max(deleteCodePoints(beforeLength), 1)); }
                }
                return true;
            }

            @Override
            public boolean sendKeyEvent(KeyEvent event) {
                if (event.getAction() == KeyEvent.ACTION_DOWN) {
                    if (event.getKeyCode() == KeyEvent.KEYCODE_DEL) {
                        deleteLastCodePoint();
                        post(() -> NativeBridge.textCommand(session, TEXT_COMMAND_BACKSPACE));
                        return true;
                    }
                    if (event.getKeyCode() == KeyEvent.KEYCODE_ENTER) {
                        emitEnter();
                        return true;
                    }
                }
                return super.sendKeyEvent(event);
            }

            @Override
            public boolean performEditorAction(int actionCode) {
                emitEnter();
                return true;
            }

            @Override
            public CharSequence getTextBeforeCursor(int length, int flags) {
				final int end = utf16Offset(committed, textSelectionEnd);
                int start = Math.max(0, end - Math.max(length, 0));
                if (start > 0 && start < end && Character.isLowSurrogate(committed.charAt(start))) {
                    start--;
                }
                return committed.substring(start, end);
            }

            @Override
            public CharSequence getTextAfterCursor(int length, int flags) {
				final int start = utf16Offset(committed, textSelectionEnd);
				return committed.substring(start, Math.min(committed.length(), start + Math.max(length, 0)));
            }

			@Override
			public boolean setSelection(int start, int end) {
				textSelectionStart = committed.codePointCount(0, Math.max(0, Math.min(start, committed.length())));
				textSelectionEnd = committed.codePointCount(0, Math.max(0, Math.min(end, committed.length())));
				final int selectionStart = textSelectionStart;
				final int selectionEnd = textSelectionEnd;
				post(() -> NativeBridge.textSelection(session, selectionStart, selectionEnd));
				return true;
			}

			@Override
			public CharSequence getSelectedText(int flags) {
				int start = utf16Offset(committed, textSelectionStart);
				int end = utf16Offset(committed, textSelectionEnd);
				return committed.substring(Math.min(start, end), Math.max(start, end));
			}

			@Override
			public boolean performContextMenuAction(int id) {
				ClipboardManager clipboard = (ClipboardManager)getContext().getSystemService(Context.CLIPBOARD_SERVICE);
				if (id == android.R.id.copy || id == android.R.id.cut) {
					CharSequence selected = getSelectedText(0);
					if (clipboard != null && selected.length() > 0) clipboard.setPrimaryClip(ClipData.newPlainText("text", selected));
					if (id == android.R.id.cut && selected.length() > 0) {
						final int start = textSelectionStart;
						final int end = textSelectionEnd;
						post(() -> NativeBridge.textSelection(session, start, end));
						deleteSelected();
						post(() -> NativeBridge.textCommand(session, TEXT_COMMAND_BACKSPACE));
					}
					return true;
				}
				if (id == android.R.id.paste && clipboard != null && clipboard.hasPrimaryClip()) {
					CharSequence value = clipboard.getPrimaryClip().getItemAt(0).coerceToText(getContext());
					return commitText(value, 1);
				}
				return super.performContextMenuAction(id);
			}

            private void deleteLastCodePoint() {
				if (textSelectionStart != textSelectionEnd) { deleteSelected(); return; }
                if (committed.length() == 0) {
                    return;
                }
				final int end = utf16Offset(committed, textSelectionEnd);
				if (end == 0) return;
                committed.delete(committed.offsetByCodePoints(end, -1), end);
				textSelectionStart = textSelectionEnd = Math.max(0, textSelectionEnd - 1);
            }

			private void deleteSelected() {
				int start = utf16Offset(committed, textSelectionStart);
				int end = utf16Offset(committed, textSelectionEnd);
				committed.delete(Math.min(start, end), Math.max(start, end));
				textSelectionStart = textSelectionEnd = Math.min(textSelectionStart, textSelectionEnd);
			}

            private int deleteCodeUnits(int length) {
				final int end = utf16Offset(committed, textSelectionEnd);
                int start = Math.max(0, end - length);
                if (start > 0 && start < end && Character.isLowSurrogate(committed.charAt(start))) {
                    start--;
                }
                final int count = committed.codePointCount(start, end);
                committed.delete(start, end);
				textSelectionStart = textSelectionEnd = Math.max(0, textSelectionEnd - count);
                return count;
            }

            private int deleteCodePoints(int length) {
				final int end = utf16Offset(committed, textSelectionEnd);
				final int available = textSelectionEnd;
                final int count = Math.min(Math.max(length, 0), available);
                if (count > 0) {
					committed.delete(committed.offsetByCodePoints(end, -count), end);
					textSelectionStart = textSelectionEnd = Math.max(0, textSelectionEnd - count);
                }
                return count;
            }

            private void emitBackspaces(int count) {
                post(() -> {
                    for (int i = 0; i < count; i++) {
                        NativeBridge.textCommand(session, TEXT_COMMAND_BACKSPACE);
                    }
                });
            }

            private void emitEnter() {
                if (multiline) {
                    committed.append('\n');
                    post(() -> NativeBridge.textInput(session, "\n"));
                } else {
                    post(() -> NativeBridge.textCommand(session, TEXT_COMMAND_ENTER));
                }
            }
        };
		currentInputConnection = connection;
		return connection;
    }

    void setTextInputConfig(long config, String initialValue) {
        if (textInputConfig == config && textInputInitialValue.equals(initialValue)) {
            return;
        }
        textInputConfig = config;
        textInputInitialValue = initialValue == null ? "" : initialValue;
        final InputMethodManager manager = (InputMethodManager)getContext().getSystemService(Context.INPUT_METHOD_SERVICE);
        if (manager == null) {
            return;
        }
        if (textInputActive()) {
            requestFocus();
            manager.restartInput(this);
            manager.showSoftInput(this, InputMethodManager.SHOW_IMPLICIT);
        } else {
            manager.hideSoftInputFromWindow(getWindowToken(), 0);
            clearFocus();
            manager.restartInput(this);
        }
    }

    private boolean textInputActive() {
        return (textInputConfig & TEXT_INPUT_ACTIVE) != 0;
    }

    private static boolean isLineBreak(String value) {
        for (int i = 0; i < value.length(); i++) {
            final char ch = value.charAt(i);
            if (ch != '\n' && ch != '\r') {
                return false;
            }
        }
        return !value.isEmpty();
    }

	private static int utf16Offset(CharSequence value, int codePoints) {
		if (value.length() == 0) return 0;
		return Character.offsetByCodePoints(value, 0, Math.max(0, Math.min(codePoints, Character.codePointCount(value, 0, value.length()))));
	}

    private static String sanitizeText(String value, boolean multiline) {
        final String normalized = value.replace("\r\n", "\n").replace('\r', '\n');
        final StringBuilder clean = new StringBuilder(normalized.length());
        for (int offset = 0; offset < normalized.length();) {
            final int codePoint = normalized.codePointAt(offset);
            offset += Character.charCount(codePoint);
            if (codePoint == '\n' || codePoint == '\t') {
                if (multiline) {
                    clean.appendCodePoint(codePoint);
                }
            } else if (!Character.isISOControl(codePoint)) {
                clean.appendCodePoint(codePoint);
            }
        }
        return clean.toString();
    }
}
