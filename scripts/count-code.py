#!/usr/bin/env python3
"""Count working-tree source with cloc; see docs/README.md for documentation."""

import argparse
from collections import defaultdict
import fnmatch
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile


ROOT = Path(__file__).resolve().parents[1]
SOURCE_EXTENSIONS = set("""
.go .c .h .cpp .cc .hpp .m .mm .java .swift .js .ts .jsx .tsx
.py .sh .bash .zsh .bat .s .asm .html .css .scss .frag .vert .wgsl
.glsl .inc .gradle .rc
""".split())
EXCLUDED_DIRS = {
    "docs", "doc", "test", "tests", "testdata", "__tests__", "__pycache__",
    "vendor", "node_modules", "dist", ".git", ".gocache", ".gradle",
}
TEST_NAME = re.compile(r"^(?:test_|benchmark_)|(?:_test|_bench|_benchmark|[.]test|[.]spec)[.]")
# Explicit inventory, not a copyright/license inference.
BUNDLED = (
    "internal/brotli/*", "images/webp/*", "images/png/stdlib*.go",
    "render/vulkan/*", "render/vk_video/*",
    "window/*-protocol.c", "window/*-client-protocol.h",
    "editor/templates/*/runtime/wasm_exec.js", "font/lcd.go", "font/lcd_gamma.go",
)
GENERATED = re.compile(
    rb"(?m)^(?://|/\*|#| \*)[ \t]*(?:Code generated .*DO NOT EDIT|[Gg]enerated by|[Aa]utomatically generated)"
)
HEADER = re.compile(rb"\A(?:\s|//[^\n]*(?:\n|$)|/\*[\s\S]*?\*/|\#[^\n]*(?:\n|$))*")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("path", nargs="?", default=".", help="repository-relative file/directory (default: all)")
    parser.add_argument("--exclude-examples", action="store_true")
    parser.add_argument("--exclude-bundled", action="store_true", help="exclude the explicit bundled-source inventory")
    parser.add_argument("--exclude-generated", action="store_true", help="exclude files with a generated header")
    parser.add_argument("--by-file", action="store_true", help="also print each included file")
    parser.add_argument("--cloc", default=shutil.which("cloc") or str(Path.home() / ".local/bin/cloc"))
    args = parser.parse_args()
    target = (ROOT / args.path).resolve()
    if not target.is_relative_to(ROOT) or not target.exists():
        parser.error("path must exist inside the repository")
    if not shutil.which(args.cloc):
        parser.error("cloc is required: install cloc, or pass --cloc /path/to/cloc")

    tracked = subprocess.check_output(
        ["git", "ls-files", "-z", "--cached", "--others", "--exclude-standard"], cwd=ROOT
    )
    files = []
    for raw in sorted(set(tracked.split(b"\0")) - {b""}):
        name = os.fsdecode(raw)
        path = ROOT / name
        relative = Path(name)
        if path.is_symlink() or not path.is_file() or not path.is_relative_to(target):
            continue
        if relative.suffix.lower() not in SOURCE_EXTENSIONS:
            continue
        if EXCLUDED_DIRS.intersection(relative.parts[:-1]) or TEST_NAME.search(relative.name):
            continue
        if args.exclude_examples and relative.parts[0] == "examples":
            continue
        if args.exclude_bundled and any(fnmatch.fnmatchcase(name, pattern) for pattern in BUNDLED):
            continue
        if args.exclude_generated:
            with path.open("rb") as source:
                if GENERATED.search(HEADER.match(source.read(4096)).group()):
                    continue
        if "\n" in name or "\r" in name:
            parser.error(f"cloc list files cannot represent this filename: {name!r}")
        files.append(name)
    if not files:
        print("No source files matched.")
        return

    # The manifest is plain text; JSON is consumed from stdout, never written to disk.
    with tempfile.NamedTemporaryFile(mode="w", encoding="utf-8", suffix=".txt") as manifest:
        manifest.write("\n".join(files) + "\n")
        manifest.flush()
        command = [args.cloc, "--quiet", "--json", "--by-file", "--skip-uniqueness",
                   f"--list-file={manifest.name}"]
        # These shader/include formats use C-style comments. Preserve their names below.
        command += [f"--force-lang=C,{ext}" for ext in ("frag", "vert", "wgsl", "glsl", "inc")]
        command += ["--force-lang=Objective-C,m"]
        report = json.loads(subprocess.check_output(command, cwd=ROOT, text=True))

    rows = {name: values for name, values in report.items() if name not in ("header", "SUM")}
    missing = set(files) - set(rows)
    if missing:
        parser.error("cloc skipped source files (refusing a partial total): " + ", ".join(sorted(missing)))
    print(f"Root: {ROOT}\ncloc: {report['header']['cloc_version']}")
    print("Excludes: docs, tests/benchmarks, fixtures, dependencies/build output, non-source assets/config")
    print(f"Examples: {'excluded' if args.exclude_examples else 'included'}; "
          f"bundled inventory: {'excluded' if args.exclude_bundled else 'included'}; "
          f"generated headers: {'excluded' if args.exclude_generated else 'included'}")
    print("Code = cloc physical source lines excluding blanks and comment-only lines; duplicates count per file.")
    groups = defaultdict(lambda: [0, 0, 0, 0])
    directories = defaultdict(lambda: [0, 0, 0, 0])
    for name, values in rows.items():
        extension = Path(name).suffix
        language = {".frag": "GLSL", ".vert": "GLSL", ".glsl": "GLSL", ".wgsl": "WGSL"}.get(extension, values["language"])
        counts = [1, values["blank"], values["comment"], values["code"]]
        directory = name.split("/")[0] if "/" in name else "(root)"
        for group in (groups[language], directories[directory]):
            for index, count in enumerate(counts):
                group[index] += count
    def table(title, entries):
        print(f"\n{title:<32} {'Files':>10} {'Blank':>10} {'Comment':>10} {'Code':>10} {'Total':>10}")
        for name, counts in entries:
            print(f"{name:<32} " + " ".join(f"{n:>10,}" for n in (*counts, sum(counts[1:]))))
    table("Language", sorted(groups.items(), key=lambda row: (-row[1][3], row[0])))
    table("Directory", sorted(directories.items(), key=lambda row: (-row[1][3], row[0])))
    table("Summary", [("TOTAL", [sum(row[i] for row in groups.values()) for i in range(4)])])
    if args.by_file:
        table("File", [(name, [1, row["blank"], row["comment"], row["code"]]) for name, row in sorted(rows.items())])


if __name__ == "__main__":
    main()
