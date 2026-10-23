# Onix

A game and application engine written in Go.

Go module: `github.com/diiyw/onix`; root package: `onix`. Requires Go 1.26
or newer and the native development libraries for the target platform.

## Getting started

Run the editor and create an independent project outside the engine checkout:

```sh
go run ./editor/cmd/onix
go run ./editor/cmd/onix new ../my-game
go run ./editor/cmd/onix check ../my-game
```

Game and application projects are maintained separately from this repository.
Project creation generates a Go module and the registration code needed to use
the engine. Project JSON and assets belong in `resources/`.

Projects use `onix_generated.go` and the `OnixRegistry`, `OnixInstallSystems`,
`OnixModuleOptions`, and `OnixOpenWindow` entry points. Platform bridges use
the `Onix` prefix; build tags and environment variables use `onix_` and `ONIX_`.

See [module authoring](docs/module-system-authoring.md),
[desktop builds](docs/desktop-builds.md), [Android builds](docs/android-builds.md),
and [resource packages](docs/resource-packages.md).

## License

Onix is licensed under the [MIT License](LICENSE). You may use, modify, compile,
and distribute the engine, including in commercial games and software, subject
to the license's notice requirements. Third-party code and assets retain their
own licenses; see [third-party notices](THIRD_PARTY_NOTICES.md).
