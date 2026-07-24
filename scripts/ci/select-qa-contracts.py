#!/usr/bin/env python3
"""Select enforceable QA contracts from changed paths."""

from __future__ import annotations

import argparse
import fnmatch
import json
import os
from pathlib import Path
import subprocess
import sys


def _matches(path: str, patterns: list[str]) -> bool:
    return any(fnmatch.fnmatchcase(path, pattern) for pattern in patterns)


def _changed_paths(args: argparse.Namespace) -> list[str]:
    if args.changed_file:
        paths = args.changed_file
    elif args.base and args.head:
        result = subprocess.run(
            [
                "git",
                "diff",
                "--name-only",
                "-z",
                "--diff-filter=ACMRTUXB",
                args.base,
                args.head,
            ],
            check=True,
            capture_output=True,
        )
        paths = [os.fsdecode(path) for path in result.stdout.split(b"\0") if path]
    elif args.full:
        return []
    else:
        raise ValueError("provide --full, --changed-file, or both --base and --head")

    normalized: set[str] = set()
    for raw_path in paths:
        path = raw_path.strip().replace("\\", "/")
        if not path:
            continue
        if any(ord(character) < 32 for character in path):
            raise ValueError(f"changed path contains control characters: {raw_path!r}")
        if path.startswith("/") or ".." in Path(path).parts:
            raise ValueError(f"changed path must be repository-relative: {raw_path!r}")
        normalized.add(path)
    return sorted(normalized)


def _load_manifest(path: Path) -> dict:
    with path.open(encoding="utf-8") as handle:
        manifest = json.load(handle)
    if manifest.get("schema_version") != 1:
        raise ValueError("qa contract manifest schema_version must be 1")
    contracts = manifest.get("contracts")
    rules = manifest.get("rules")
    guards = manifest.get("high_risk_patterns")
    if not isinstance(contracts, dict) or not contracts:
        raise ValueError("qa contract manifest must define contracts")
    if not isinstance(rules, list) or not rules:
        raise ValueError("qa contract manifest must define rules")
    if not isinstance(guards, list) or not guards:
        raise ValueError("qa contract manifest must define high_risk_patterns")

    known = set(contracts)
    for contract_id, contract in contracts.items():
        if not isinstance(contract, dict) or not contract.get("reason"):
            raise ValueError(f"contract {contract_id!r} must have a concise reason")
    for rule in rules:
        if not rule.get("id") or not rule.get("reason") or not rule.get("patterns"):
            raise ValueError("every rule must define id, reason, and patterns")
        unknown = set(rule.get("contracts", [])) - known
        if unknown:
            raise ValueError(
                f"rule {rule['id']!r} references unknown contracts: {sorted(unknown)}"
            )
        if not rule.get("contracts"):
            raise ValueError(f"rule {rule['id']!r} must select at least one contract")
    return manifest


def select(manifest: dict, paths: list[str], full: bool) -> tuple[list[str], dict[str, list[str]]]:
    contracts = manifest["contracts"]
    reasons: dict[str, list[str]] = {contract_id: [] for contract_id in contracts}
    if full:
        for contract_id in contracts:
            reasons[contract_id].append("full, tag, or manual run")
        return sorted(contracts), reasons

    matched_high_risk: set[str] = set()
    for rule in manifest["rules"]:
        matched = [path for path in paths if _matches(path, rule["patterns"])]
        if not matched:
            continue
        matched_high_risk.update(matched)
        detail = f"{rule['reason']}: {', '.join(matched[:4])}"
        if len(matched) > 4:
            detail += f" (+{len(matched) - 4} more)"
        for contract_id in rule["contracts"]:
            reasons[contract_id].append(detail)

    guarded = {
        path for path in paths if _matches(path, manifest["high_risk_patterns"])
    }
    unmatched = sorted(guarded - matched_high_risk)
    if unmatched:
        raise ValueError(
            "high-risk changed paths have no QA rule: " + ", ".join(unmatched)
        )
    selected = sorted(contract_id for contract_id, why in reasons.items() if why)
    return selected, reasons


def _write_outputs(path: Path, contract_ids: list[str], selected: set[str]) -> None:
    with path.open("a", encoding="utf-8") as handle:
        for contract_id in sorted(contract_ids):
            output_name = contract_id.replace("-", "_")
            value = "true" if contract_id in selected else "false"
            handle.write(f"{output_name}={value}\n")
        handle.write(f"selected_json={json.dumps(sorted(selected), separators=(',', ':'))}\n")


def main() -> int:
    parser = argparse.ArgumentParser()
    default_manifest = Path(__file__).with_name("qa-contracts.json")
    parser.add_argument("--manifest", type=Path, default=default_manifest)
    parser.add_argument("--base")
    parser.add_argument("--head")
    parser.add_argument("--changed-file", action="append", default=[])
    parser.add_argument("--full", action="store_true")
    parser.add_argument("--github-output", type=Path)
    args = parser.parse_args()

    try:
        manifest = _load_manifest(args.manifest)
        paths = _changed_paths(args)
        selected, reasons = select(manifest, paths, args.full)
    except (OSError, ValueError, subprocess.CalledProcessError, json.JSONDecodeError) as error:
        print(f"QA contract selection failed: {error}", file=sys.stderr)
        return 2

    selected_set = set(selected)
    print("QA contracts: " + (", ".join(selected) if selected else "none"))
    for contract_id in selected:
        print(f"- {contract_id}: {'; '.join(reasons[contract_id])}")

    output_path = args.github_output
    if output_path is None and os.environ.get("GITHUB_OUTPUT"):
        output_path = Path(os.environ["GITHUB_OUTPUT"])
    if output_path is not None:
        _write_outputs(output_path, list(manifest["contracts"]), selected_set)

    if os.environ.get("GITHUB_STEP_SUMMARY"):
        summary_path = Path(os.environ["GITHUB_STEP_SUMMARY"])
        with summary_path.open("a", encoding="utf-8") as summary:
            summary.write("### Selected QA contracts\n\n")
            summary.write(", ".join(f"`{item}`" for item in selected) if selected else "None")
            summary.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
