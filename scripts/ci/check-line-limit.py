#!/usr/bin/env python3
"""Enforce LabTether's 500 code-line ceiling with checksum-pinned cloc 2.10."""

import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import urllib.request

LIMIT = 500
CLOC_SHA256 = "bf59272455172108072a0a106379f7509fd4349bdcfd85203bac038ccd286d83"
CLOC_URL = (
    "https://raw.githubusercontent.com/AlDanial/cloc/"
    "eb5cef64db2b2d4380f501568cc584b7cab4ba50/cloc"
)
# Data, prose, lockfiles and project metadata are not executable code. Keep
# markup/style, SQL and executable build/CI configuration in scope.
CODE_SUFFIXES = {
    ".go", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".mts", ".cts",
    ".swift", ".cs", ".fs", ".fsx", ".vb", ".py", ".pyi", ".sh", ".bash",
    ".zsh", ".fish", ".ps1", ".psm1", ".psd1", ".bat", ".cmd", ".c", ".h",
    ".cpp", ".hpp", ".cc", ".cxx", ".m", ".mm", ".rs", ".rb", ".rake",
    ".pl", ".pm", ".lua", ".java", ".kt", ".kts", ".scala", ".dart",
    ".vue", ".svelte", ".html", ".htm", ".css", ".scss", ".sass", ".less",
    ".sql", ".graphql", ".gql", ".proto", ".xaml", ".yml", ".yaml",
    ".cmake", ".mk", ".gradle", ".tf", ".hcl", ".nix", ".ex", ".exs",
    ".csproj", ".fsproj", ".vbproj", ".props", ".targets", ".proj",
    ".dockerfile", ".containerfile", ".makefile",
}
CODE_NAMES = {"makefile", "gnumakefile", "dockerfile", "containerfile", "cmakelists.txt", "justfile"}
XML_BUILD_SUFFIXES = ("xaml", "csproj", "fsproj", "vbproj", "props", "targets", "proj")


def git_files(root):
    result = subprocess.run(
        ["git", "ls-files", "--cached", "--others", "--exclude-standard", "-z"],
        cwd=root, check=True, stdout=subprocess.PIPE,
    )
    return sorted(set(result.stdout.decode().rstrip("\0").split("\0")) - {""})


def exclusions(root):
    path = root / ".line-limit-exclusions.json"
    if not path.exists():
        return []
    entries = json.loads(path.read_text())
    if not isinstance(entries, list):
        raise ValueError(f"{path}: exclusions must be a list")
    for entry in entries:
        name = entry.get("path", "")
        if (not name or name.startswith("/") or ".." in Path(name).parts
                or any(c in name for c in "*?[")
                or entry.get("kind") not in {"generated", "vendor"}
                or not entry.get("reason") or not entry.get("source")):
            raise ValueError(f"{path}: each exclusion needs an exact path, kind, reason and source")
        if not (root / name).exists():
            raise ValueError(f"{path}: stale exclusion: {name}")
        if (root / name).is_dir() and entry["kind"] != "vendor":
            raise ValueError(f"{path}: generated exclusions must name individual files: {name}")
    return entries


def is_code(path):
    if path.suffix.lower() in CODE_SUFFIXES or path.name.lower() in CODE_NAMES:
        return True
    if path.name.startswith(("Dockerfile.", "Containerfile.")):
        return True
    with path.open("rb") as source:
        return source.read(2) == b"#!"


def sources(root, names):
    omitted = exclusions(root)
    selected = []
    for name in names:
        path = root / name
        if path.is_symlink():
            if path.suffix.lower() in CODE_SUFFIXES or path.name.lower() in CODE_NAMES:
                raise ValueError(f"source symlink needs a real file: {path}")
            continue
        if not path.exists() or path.is_dir():
            continue  # Deleted files and Git submodules have no code here.
        if any(name == entry["path"].rstrip("/") or (
                (root / entry["path"]).is_dir()
                and name.startswith(entry["path"].rstrip("/") + "/")) for entry in omitted):
            continue
        if is_code(path):
            if "\n" in str(path) or "\r" in str(path):
                raise ValueError(f"newline in source filename: {path!r}")
            selected.append(path)
    return selected


def pinned_cloc(given, temporary):
    if given:
        path = Path(given).resolve()
        payload = path.read_bytes()
    else:
        path = temporary / "cloc"
        with urllib.request.urlopen(CLOC_URL, timeout=30) as response:
            payload = response.read()
        path.write_bytes(payload)
    if hashlib.sha256(payload).hexdigest() != CLOC_SHA256:
        raise ValueError("cloc checksum mismatch; expected the pinned 2.10 source")
    return path


def count(root, names, cloc, temporary):
    files = sources(root, names)
    if not files:
        return []
    file_list = temporary / "sources.txt"
    file_list.write_text("".join(f"{path}\n" for path in files))
    output = subprocess.run(
        ["perl", str(cloc), "--json", "--by-file", "--quiet", "--skip-uniqueness",
         "--force-lang=TypeScript,mts", "--force-lang=TypeScript,cts",
         *(f"--force-lang=XML,{suffix}" for suffix in XML_BUILD_SUFFIXES),
         f"--list-file={file_list}"],
        cwd=root, check=True, stdout=subprocess.PIPE, text=True,
    )
    result = json.loads(output.stdout)
    counts = []
    for name, values in result.items():
        if name in {"header", "SUM"}:
            continue
        counts.append({"file": str(Path(name).relative_to(root)), **values})
    counted = {row["file"] for row in counts}
    for path in files:
        if str(path.relative_to(root)) not in counted and path.read_text().strip():
            raise ValueError(f"cloc did not recognize source: {path}")
    return sorted(counts, key=lambda row: (-row["code"], row["file"]))


def roots_for(root, workspace):
    if not workspace:
        return [(root, git_files(root))]
    if (root / ".git").exists():
        raise ValueError("--workspace requires the non-Git LabTether workspace root")
    repos = sorted(p for p in root.iterdir() if p.is_dir() and (p / ".git").exists())
    if not repos:
        raise ValueError("no Git repositories found in workspace")
    own = [str(p.relative_to(root)) for folder in ("scripts", "docs")
           for p in (root / folder).rglob("*") if p.is_file()]
    return [(root, own)] + [(repo, git_files(repo)) for repo in repos]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path.cwd())
    parser.add_argument("--workspace", action="store_true", help="check root scripts and every child repo")
    parser.add_argument("--cloc", type=Path, help="reuse a checksum-verified local cloc 2.10")
    parser.add_argument("--report", type=Path, help="save full counts as JSON (violations still fail)")
    args = parser.parse_args()
    root = args.root.resolve()
    try:
        with tempfile.TemporaryDirectory(prefix="labtether-line-limit-") as folder:
            temporary = Path(folder)
            cloc = pinned_cloc(args.cloc, temporary)
            report = []
            for repo, names in roots_for(root, args.workspace):
                counts = count(repo, names, cloc, temporary)
                report.extend({"repo": str(repo.relative_to(root)), **row} for row in counts)
        if args.report:
            args.report.write_text(json.dumps(report, indent=2) + "\n")
        failures = [row for row in report if row["code"] > LIMIT]
        for row in failures:
            path = Path(row["repo"]) / row["file"]
            print(f"{path}: {row['code']} code lines (limit {LIMIT})")
        print(f"Checked {len(report)} handwritten code files; {len(failures)} over {LIMIT} lines.")
        return bool(failures)
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        print(f"Line limit check failed: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
