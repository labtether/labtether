import importlib.util
import json
from pathlib import Path
import tempfile
import unittest


SCRIPT_PATH = Path(__file__).with_name("select-qa-contracts.py")
SPEC = importlib.util.spec_from_file_location("qa_selector", SCRIPT_PATH)
assert SPEC and SPEC.loader
SELECTOR = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(SELECTOR)
MANIFEST = SELECTOR._load_manifest(Path(__file__).with_name("qa-contracts.json"))


class QAContractSelectorTests(unittest.TestCase):
    def test_full_mode_selects_every_contract(self):
        selected, _ = SELECTOR.select(MANIFEST, [], True)
        self.assertEqual(selected, sorted(MANIFEST["contracts"]))

    def test_workflow_change_selects_every_contract(self):
        selected, _ = SELECTOR.select(
            MANIFEST, [".github/workflows/codeql.yml"], False
        )
        self.assertEqual(selected, sorted(MANIFEST["contracts"]))

    def test_transport_change_selects_transport_and_installed_contracts(self):
        selected, _ = SELECTOR.select(
            MANIFEST, ["web/console/scripts/runtime-proxy.mjs"], False
        )
        self.assertIn("hub-transport", selected)
        self.assertIn("hub-installed-system", selected)

    def test_schedule_change_selects_behavior_and_installed_contracts(self):
        selected, _ = SELECTOR.select(
            MANIFEST, ["internal/schedules/cron.go"], False
        )
        self.assertIn("schedule-platform", selected)
        self.assertIn("hub-installed-system", selected)

    def test_pbs_ui_selects_restore_and_connector_system_contracts(self):
        selected, _ = SELECTOR.select(
            MANIFEST,
            ["web/console/app/api/pbs/assets/node/snapshots/verify/route.ts"],
            False,
        )
        self.assertIn("connector-system", selected)
        self.assertIn("restore-roundtrip", selected)
        self.assertIn("hub-installed-system", selected)

    def test_proxmox_handler_selects_connector_system_contract(self):
        selected, _ = SELECTOR.select(
            MANIFEST, ["internal/hubapi/proxmox/proxmox_api_backup.go"], False
        )
        self.assertIn("connector-system", selected)
        self.assertIn("hub-installed-system", selected)

    def test_low_risk_documentation_change_selects_nothing(self):
        selected, _ = SELECTOR.select(MANIFEST, ["docs/operator-guide.md"], False)
        self.assertEqual(selected, [])

    def test_unmapped_high_risk_path_fails_closed(self):
        manifest = {
            "schema_version": 1,
            "contracts": {"proof": {"reason": "proof"}},
            "high_risk_patterns": ["secure/**"],
            "rules": [
                {
                    "id": "known",
                    "reason": "known",
                    "patterns": ["secure/known/**"],
                    "contracts": ["proof"],
                }
            ],
        }
        with self.assertRaisesRegex(ValueError, "no QA rule"):
            SELECTOR.select(manifest, ["secure/new-risk/file.go"], False)

    def test_unknown_contract_is_rejected(self):
        manifest = {
            "schema_version": 1,
            "contracts": {"proof": {"reason": "proof"}},
            "high_risk_patterns": ["secure/**"],
            "rules": [
                {
                    "id": "broken",
                    "reason": "broken",
                    "patterns": ["secure/**"],
                    "contracts": ["missing"],
                }
            ],
        }
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "manifest.json"
            path.write_text(json.dumps(manifest), encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "unknown contracts"):
                SELECTOR._load_manifest(path)


if __name__ == "__main__":
    unittest.main()
