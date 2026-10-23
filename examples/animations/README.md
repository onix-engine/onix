# CSS animations

An Onix project displaying the English HTML fixture from
`tests/htmlcompat/resources/animations.html` in a 1280 x 900 window.
The project uses the engine checkout's Go module.

Run from this directory:

```sh
go run .
```

From the engine repository root, validate or open the project in the editor:

```sh
go run ./editor/cmd/onix check ./examples/animations
go run ./editor/cmd/onix ./examples/animations
```

The page is stored at `resources/animations/animations.html` as a private module
resource. It is a copy of the compatibility fixture, so the example can be built
and packaged without reading files outside its project directory.

Compare the page with Chromium manually. Hover over the pause and transition
cases to check their interactive behavior.
