#!/usr/bin/env python3
from __future__ import annotations

import json
import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest import mock

import install


class InstallHelpersTest(unittest.TestCase):
    def test_parse_args_defaults_scope_to_global(self) -> None:
        with mock.patch("sys.argv", ["install.py", "install", "--target", "opencode"]):
            args = install.parse_args()
        self.assertEqual(args.scope, "global")

    def test_shared_runtime_root_uses_absolute_xdg_data_home(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            data_home = Path(temp_dir) / "xdg-data"
            with mock.patch.dict(os.environ, {"XDG_DATA_HOME": str(data_home)}, clear=False):
                self.assertEqual(install.shared_runtime_root(), data_home / "claude-bash-approve")

    def test_shared_runtime_root_ignores_relative_xdg_data_home(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            home = Path(temp_dir) / "home"
            home.mkdir()
            with mock.patch.dict(os.environ, {"HOME": str(home), "XDG_DATA_HOME": "relative"}, clear=False):
                self.assertEqual(install.shared_runtime_root(), home / ".local" / "share" / "claude-bash-approve")


class InstallCLITestCase(unittest.TestCase):
    maxDiff = None

    def setUp(self) -> None:
        self.temp_dir = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp_dir.cleanup)
        self.temp_path = Path(self.temp_dir.name)
        self.project_root = self.temp_path / "project"
        self.home = self.temp_path / "home"
        self.project_root.mkdir()
        self.home.mkdir()
        self.env = os.environ.copy()
        self.env.update(
            {
                "HOME": str(self.home),
                "XDG_DATA_HOME": str(self.temp_path / "xdg-data"),
                "GOMODCACHE": str(self.temp_path / "gomodcache"),
                "GOCACHE": str(self.temp_path / "gocache"),
            }
        )
        self.install_py = install.repo_root() / "install.py"

    def run_install(self, *args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            ["python3", str(self.install_py), *args, "--project-root", str(self.project_root)],
            env=self.env,
            text=True,
            capture_output=True,
            check=True,
        )

    @property
    def runtime_root(self) -> Path:
        return self.temp_path / "xdg-data" / "claude-bash-approve"


class ClaudeInstallTest(InstallCLITestCase):
    def test_install_and_uninstall_claude(self) -> None:
        self.run_install("install", "--target", "claude")

        settings_path = self.home / ".claude" / "settings.json"
        self.assertTrue((self.runtime_root / "approve-bash").is_file())
        self.assertTrue((self.runtime_root / "run-hook.sh").is_file())
        self.assertFalse((self.home / ".claude" / "hooks" / "bash-approve").exists())

        settings = json.loads(settings_path.read_text())
        pre_tool = settings["hooks"]["PreToolUse"]
        self.assertEqual(len(pre_tool), 2)
        self.assertEqual({entry["matcher"] for entry in pre_tool}, {"Bash", "Read|Grep"})
        for entry in pre_tool:
            self.assertEqual(entry["hooks"][0]["command"], str(self.runtime_root / "run-hook.sh"))

        self.run_install("uninstall", "--target", "claude")
        updated = json.loads(settings_path.read_text())
        self.assertFalse(updated.get("hooks"))


class OpenCodeInstallTest(InstallCLITestCase):
    def test_install_and_uninstall_opencode(self) -> None:
        (self.project_root / "opencode.json").write_text(
            json.dumps(
                {
                    "permission": {
                        "bash": {
                            "*": "deny",
                            "ls *": "ask",
                        }
                    }
                }
            )
        )

        self.run_install("install", "--target", "opencode", "--scope", "both")

        project_plugin = self.project_root / ".opencode" / "plugins" / "bash-approve.ts"
        global_plugin = self.home / ".config" / "opencode" / "plugins" / "bash-approve.ts"
        global_config = self.home / ".config" / "opencode" / "opencode.json"

        self.assertTrue((self.runtime_root / "approve-bash").is_file())
        self.assertTrue(project_plugin.is_file())
        self.assertTrue(global_plugin.is_file())
        self.assertFalse((self.home / ".config" / "opencode" / "bash-approve").exists())
        self.assertIn(str(self.runtime_root / "run-opencode-hook.sh"), project_plugin.read_text())
        self.assertIn(str(self.runtime_root / "run-opencode-hook.sh"), global_plugin.read_text())

        project_config = json.loads((self.project_root / "opencode.json").read_text())
        self.assertEqual(project_config["permission"]["bash"]["*"], "ask")
        self.assertEqual(json.loads(global_config.read_text())["permission"]["bash"]["*"], "ask")

        self.run_install("uninstall", "--target", "opencode", "--scope", "both")
        self.assertFalse(project_plugin.exists())
        self.assertFalse(global_plugin.exists())


class CodexInstallTest(InstallCLITestCase):
    def test_install_and_uninstall_codex(self) -> None:
        codex_root = self.home / ".codex"
        codex_root.mkdir()
        (codex_root / "config.toml").write_text("[features]\nmulti_agent = true\n")

        self.run_install("install", "--target", "codex", "--scope", "both")

        project_config = self.project_root / ".codex" / "config.toml"
        project_hooks = self.project_root / ".codex" / "hooks.json"
        global_config = codex_root / "config.toml"
        global_hooks = codex_root / "hooks.json"

        self.assertTrue((self.runtime_root / "approve-bash").is_file())
        self.assertIn("codex_hooks = true", project_config.read_text())
        self.assertIn("multi_agent = true", global_config.read_text())
        self.assertIn("codex_hooks = true", global_config.read_text())
        self.assertIn(str(self.runtime_root / "run-codex-hook.sh"), project_hooks.read_text())
        self.assertIn(str(self.runtime_root / "run-codex-hook.sh"), global_hooks.read_text())

        self.run_install("uninstall", "--target", "codex", "--scope", "both")
        self.assertNotIn(str(self.runtime_root / "run-codex-hook.sh"), project_hooks.read_text())
        self.assertNotIn(str(self.runtime_root / "run-codex-hook.sh"), global_hooks.read_text())


class AllUninstallTest(InstallCLITestCase):
    def test_uninstall_all_removes_shared_runtime(self) -> None:
        self.run_install("install", "--target", "all")
        self.assertTrue(self.runtime_root.exists())
        self.run_install("uninstall", "--target", "all")
        self.assertFalse(self.runtime_root.exists())


if __name__ == "__main__":
    unittest.main()
