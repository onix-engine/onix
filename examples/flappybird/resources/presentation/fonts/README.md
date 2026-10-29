# Flappy Bird UI font

`ui.ttf` is a subset of the repository's
`testdata/fonts/Noto_Sans_SC/static/NotoSansSC-Regular.ttf` (Noto Sans SC Regular).
It keeps printable ASCII and the nonbreaking space for this example's English
HUD and numeric score. Glyph outlines, advances and line metrics are preserved;
the original SIL Open Font License remains in `OFL.txt` and in the font metadata.

Regenerate from the repository root with fontTools 4.68.0:

```sh
python3 -m fontTools.subset \
  testdata/fonts/Noto_Sans_SC/static/NotoSansSC-Regular.ttf \
  --unicodes=U+0020-007E,U+00A0 \
  --output-file=examples/flappybird/resources/presentation/fonts/ui.ttf \
  --name-IDs='*' --name-languages='*' \
  --notdef-glyph --notdef-outline --recommended-glyphs
```

Expand the subset when adding non-ASCII translations. fontTools is only needed
to regenerate the asset, not to build or run the game.
