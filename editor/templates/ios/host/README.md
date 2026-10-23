# __APP_NAME__ iOS Host Template

This directory is a lightweight host-side scaffold for integrating the generated Go runtime into an iOS app.

Suggested flow on macOS:

1. Use Debug -> Build in Onix Editor and select iOS to generate the Go entry package, native bridge, and static library.
2. Add the generated `../libonix.a`, matching header, `OnixMetalView.swift`, and `Assets.xcassets` to your Xcode target. Expose the generated header through the target's bridging header and select `AppIcon` as the target's App Icons Source.
3. Use `OnixMetalView` as the game view. It installs its `CAMetalLayer` through `OnixSetMetalLayer` before starting the Go runtime and forwards resize, frame tick, touch, and device scale events.
4. For a custom host, forward lifecycle, resize, frame tick, touch events, and device scale updates to:
   `OnixStart`, `OnixResize`, `OnixSetDevicePixelRatio`, `OnixUpdate`, `OnixTouchStart`, `OnixTouchMove`, `OnixTouchEnd`, `OnixTextInput`, `OnixTextComposition`, `OnixTextCommand`
5. Before calling `OnixStart`/`OnixResize`, install a `CAMetalLayer` with `OnixSetMetalLayer`, then pass the render scale with `OnixSetDevicePixelRatio`.
   A reliable value is `drawableWidth / bounds.width` (or `UIScreen.main.scale` when the drawable matches the view bounds).
6. Poll `OnixTextInputConfig` after each update. A changed non-zero configuration starts or restarts the native text-input session; bit 0 is active, bit 1 is multiline, and bits 2-3 select text, decimal, or integer input. Hide the keyboard when bit 0 clears. Read the initial UTF-8 field value by calling `OnixTextInputValue(NULL, 0)`, allocating that many bytes, and calling it again with the buffer.
7. Keep the current configuration value as the text-input session ID. Send it as the first argument of every event: committed UTF-8 text through `OnixTextInput(session, text)`, composition updates through `OnixTextComposition(session, text, 1)`, composition end through `OnixTextComposition(session, "", 0)`, Backspace as `OnixTextCommand(session, 1)`, and Enter/Done as `OnixTextCommand(session, 2)`. Events from an old session are ignored.

Generated Go entry package:

- `__APP_IMPORT_PATH__`
