# Onix engine development guidelines

- Avoid compatibility code. Prefer generics(supports generic methods) and keep implementations concise.
- Do not launch the editor for screenshot testing; visual acceptance is manual.
- Store project JSON files in the resources directory.
- Aim for zero allocations when adding features, especially on warm paths.
- HTML rendering changes must follow W3C standards and align with Chromium.
- Combine traversals when possible instead of repeating O(n) passes.
- Fix unreasonable designs directly rather than preserving them unnecessarily.

## Mobile testing

Try connecting to the Android device with `adb connect 10.0.0.174:16416`.

## Prohibited actions

- Never run `go test ./...`; it can freeze the machine. Use targeted package tests.
- Do not use subagents.
