"""Build bundled fonts with FontTools: python3 tools/subset_fonts.py."""

from pathlib import Path
from shutil import copyfile

from fontTools import subset
from fontTools.ttLib import TTFont


project = Path(__file__).resolve().parents[1]
source = project.parents[1] / "testdata/fonts/Noto_Sans_SC"
destination = project / "resources/game/fonts"
destination.mkdir(parents=True, exist_ok=True)

# Include ASCII for counters and all source characters for static/dynamic UI.
characters = "".join(chr(code) for code in range(32, 127))
for name in ("game.html", "mod.go", "state.go"):
    characters += (project / "resources/game" / name).read_text(encoding="utf-8")

for weight in ("Regular", "Bold"):
    with TTFont(source / "static" / f"NotoSansSC-{weight}.ttf", recalcTimestamp=False) as font:
        options = subset.Options()
        options.notdef_outline = True
        options.recommended_glyphs = True
        subsetter = subset.Subsetter(options=options)
        subsetter.populate(text=characters)
        subsetter.subset(font)
        names = {
            1: "Pelican Sans SC",
            3: f"PelicanSansSC-{weight}",
            4: f"Pelican Sans SC {weight}",
            6: f"PelicanSansSC-{weight}",
            16: "Pelican Sans SC",
        }
        for record in list(font["name"].names):
            if record.nameID in names:
                font["name"].setName(
                    names[record.nameID], record.nameID,
                    record.platformID, record.platEncID, record.langID,
                )
        font.save(destination / f"PelicanSansSC-{weight}.ttf")

copyfile(source / "OFL.txt", destination / "OFL.txt")
