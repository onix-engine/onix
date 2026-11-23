"""Pack the pinned textmate-go grammar assets by scope for registry-scoped loading."""

import gzip
import json
from pathlib import Path
import subprocess
from zipfile import ZIP_DEFLATED, ZipFile, ZipInfo

package = subprocess.check_output(
    ["go", "list", "-f", "{{.Dir}}", "github.com/eugenioenko/textmate-go/grammars"],
    text=True,
).strip()
assets = sorted((Path(package) / "data").glob("*.json.gz"))
if not assets:
    raise RuntimeError("missing TextMate grammar assets")

with ZipFile("grammars.zip", "w") as archive:
    for asset in assets:
        data = gzip.decompress(asset.read_bytes())
        scope = json.loads(data)["scopeName"]
        if scope in archive.namelist():
            raise ValueError(f"duplicate grammar {scope}")
        archive.writestr(ZipInfo(scope), data, compress_type=ZIP_DEFLATED)
