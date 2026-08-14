import pathlib
import subprocess
import tempfile
import unittest


SCRIPT = pathlib.Path(__file__).parents[1] / "check-release-policy.sh"


class ReleasePolicyTest(unittest.TestCase):
    def run_policy(self, version, tags, compatibility_version=None):
        with tempfile.TemporaryDirectory() as directory:
            repo = pathlib.Path(directory)
            subprocess.run(["git", "init", "-q", "-b", "main"], cwd=repo, check=True)
            subprocess.run(["git", "config", "user.name", "Release Test"], cwd=repo, check=True)
            subprocess.run(["git", "config", "user.email", "release@example.invalid"], cwd=repo, check=True)
            (repo / "VERSION").write_text(f"{version}\n", encoding="utf-8")
            (repo / "docs").mkdir()
            documented = compatibility_version or version
            (repo / "docs" / "COMPATIBILITY.md").write_text(
                f"| Controller | Agent | 状态 | 说明 |\n"
                f"| --- | --- | --- | --- |\n"
                f"| {documented} | v0.6.6 | 支持 | 当前稳定组合 |\n"
                f"| Controller | Agent | Status | Notes |\n"
                f"| --- | --- | --- | --- |\n"
                f"| {documented} | v0.6.6 | Supported | Current stable combination |\n",
                encoding="utf-8",
            )
            subprocess.run(["git", "add", "VERSION", "docs/COMPATIBILITY.md"], cwd=repo, check=True)
            subprocess.run(["git", "commit", "-q", "-m", f"release: {version}"], cwd=repo, check=True)
            for tag in tags:
                subprocess.run(["git", "tag", tag], cwd=repo, check=True)

            return subprocess.run(
                ["bash", str(SCRIPT), version, "HEAD", "main"],
                cwd=repo,
                text=True,
                capture_output=True,
            )

    def test_next_patch_passes_standard_semver_policy(self):
        result = self.run_policy("v1.0.3", ("v0.22.6", "v1.0.2", "v1.0.3"))
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_higher_semver_blocks_release_without_exceptions(self):
        result = self.run_policy("v1.0.3", ("v1.0.3", "v2.0.0"))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("v2.0.0", result.stderr)

    def test_stale_compatibility_version_blocks_stable_release(self):
        result = self.run_policy("v1.0.3", ("v1.0.3",), compatibility_version="v1.0.2")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("COMPATIBILITY.md current stable Controller does not match VERSION", result.stderr)


if __name__ == "__main__":
    unittest.main()
