"""Exercise discovery, counting, and failure boundaries of the line-limit gate."""

import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SPEC = importlib.util.spec_from_file_location(
    "line_limit", Path(__file__).with_name("check-line-limit.py"))
gate = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(gate)


class LineLimitTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.tool_folder = tempfile.TemporaryDirectory(prefix="line-limit-tool-test-")
        cls.cloc = gate.pinned_cloc(os.environ.get("LABTETHER_CLOC"), Path(cls.tool_folder.name))

    @classmethod
    def tearDownClass(cls):
        cls.tool_folder.cleanup()

    def setUp(self):
        self.folder = tempfile.TemporaryDirectory(prefix="line-limit-test-")
        self.addCleanup(self.folder.cleanup)
        self.root = Path(self.folder.name).resolve()
        subprocess.run(["git", "init", "-q", str(self.root)], check=True)

    def write(self, name, content):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content)
        return path

    def counts(self):
        return gate.count(self.root, gate.git_files(self.root), self.cloc, self.root)

    def test_counts_code_not_comments_and_does_not_deduplicate(self):
        source = "// comment\n\nconst answer = 42;\n" * 500
        self.write("boundary.ts", source)
        self.write("same.ts", source)
        self.write("too_large.ts", source + "const extra = true;\n")
        rows = {row["file"]: row["code"] for row in self.counts()}
        self.assertEqual(rows, {"boundary.ts": 500, "same.ts": 500, "too_large.ts": 501})

    def test_checks_untracked_tests_scripts_and_hidden_workflows(self):
        self.write("test_behavior.py", "assert True\n")
        self.write(".github/workflows/ci.yml", "name: CI\n")
        self.write("bin/run", "#!/bin/sh\necho ok\n")
        self.write("Dockerfile", "FROM scratch\n")
        self.write("view.xaml", '<Window xmlns="urn:test" />\n')
        self.write("README.md", "prose\n" * 501)
        self.assertEqual({row["file"] for row in self.counts()}, {
            "test_behavior.py", ".github/workflows/ci.yml", "bin/run", "Dockerfile", "view.xaml",
        })

    def test_tracked_ignored_code_remains_in_scope(self):
        self.write("tracked.py", "print('hello')\n")
        subprocess.run(["git", "add", "tracked.py"], cwd=self.root, check=True)
        self.write(".gitignore", "*.py\n")
        self.write("cache.py", "print('cache')\n")
        self.assertEqual([row["file"] for row in self.counts()], ["tracked.py"])

    def test_only_declared_generated_or_vendor_exclusions(self):
        self.write("generated/schema.sql", "SELECT 1;\n" * 501)
        self.write("generated/handwritten.sql", "SELECT 2;\n")
        self.write(".line-limit-exclusions.json", json.dumps([{
            "path": "generated/schema.sql", "kind": "generated",
            "reason": "Generated fixture", "source": "scripts/generate.py",
        }]))
        self.assertEqual([row["file"] for row in self.counts()], ["generated/handwritten.sql"])
        self.write(".line-limit-exclusions.json", json.dumps([{
            "path": "generated", "kind": "generated", "reason": "Too broad", "source": "generator",
        }]))
        with self.assertRaisesRegex(ValueError, "individual files"):
            self.counts()

    def test_source_symlink_and_tool_tampering_fail(self):
        self.write("target.py", "print(1)\n")
        (self.root / "linked.py").symlink_to("target.py")
        with self.assertRaisesRegex(ValueError, "source symlink"):
            self.counts()
        fake = self.write("fake-cloc", "not the pinned tool")
        with self.assertRaisesRegex(ValueError, "checksum mismatch"):
            gate.pinned_cloc(fake, self.root)

    def test_cli_returns_failure_for_any_oversized_file(self):
        self.write("old.py", "print(1)\n" * 501)
        subprocess.run(["git", "add", "old.py"], cwd=self.root, check=True)
        result = subprocess.run([
            "python3", str(Path(gate.__file__).resolve()), "--root", str(self.root),
            "--cloc", str(self.cloc),
        ], capture_output=True, text=True)
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertIn("old.py: 501 code lines", result.stdout)

    def test_msbuild_is_counted_and_unknown_script_extensions_fail_closed(self):
        for suffix in ("csproj", "props", "targets", "proj"):
            self.write(f"build.{suffix}", '<Target Name="Check" />\n' * 501)
        rows = {row["file"]: row["code"] for row in self.counts()}
        for suffix in ("csproj", "props", "targets", "proj"):
            self.assertEqual(rows[f"build.{suffix}"], 501)
        self.write("setup.conf", "#!/bin/sh\necho setup\n")
        with self.assertRaisesRegex(ValueError, "cloc did not recognize source"):
            self.counts()


if __name__ == "__main__":
    unittest.main()
