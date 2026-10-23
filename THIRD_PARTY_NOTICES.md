# Third-party notices

The root MIT license applies to original Onix code and documentation. It does
not replace the licenses of bundled third-party code, fonts or assets.

- The WebP decoder derived from the Go image libraries retains its BSD license
  in [images/webp/LICENSE](images/webp/LICENSE).
- The Brotli implementation retains its MIT license in
  [internal/brotli/LICENSE](internal/brotli/LICENSE).
- Skia-derived font code retains its BSD license in
  [font/resources/SKIA_LICENSE](font/resources/SKIA_LICENSE).
- Noto Sans SC test fonts retain the SIL Open Font License and attribution in
  [testdata/fonts/Noto_Sans_SC/OFL.txt](testdata/fonts/Noto_Sans_SC/OFL.txt).
- The Royal Esplanade environment test fixture is by Greg Zaal, distributed by
  Poly Haven under CC0. Its source is recorded in
  [the fixture notice](testdata/resources/environment/README.md).

Other source files and bundled headers retain their embedded copyright and
license notices. Go module dependencies listed in `go.mod` and target platform
libraries remain subject to their respective licenses; preserve applicable
notices when distributing them.
