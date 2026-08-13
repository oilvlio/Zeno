import pathlib
import subprocess
import tempfile
import unittest


SCRIPT = pathlib.Path(__file__).parents[1] / "check-release-policy.sh"


class ReleasePolicyTest(unittest.TestCase):
    def test_canonical_v1_patch_ignores_mistaken_higher_release_lines(self):
        with tempfile.TemporaryDirectory() as directory:
            repo = pathlib.Path(directory)
            subprocess.run(["git", "init", "-q", "-b", "main"], cwd=repo, check=True)
            subprocess.run(["git", "config", "user.name", "Release Test"], cwd=repo, check=True)
            subprocess.run(["git", "config", "user.email", "release@example.invalid"], cwd=repo, check=True)
            (repo / "VERSION").write_text("v1.0.2\n", encoding="utf-8")
            subprocess.run(["git", "add", "VERSION"], cwd=repo, check=True)
            subprocess.run(["git", "commit", "-q", "-m", "release: v1.0.2"], cwd=repo, check=True)
            for tag in ("v1.0.1", "v1.0.2", "v1.9.10", "v2.0.0"):
                subprocess.run(["git", "tag", tag], cwd=repo, check=True)

            result = subprocess.run(
                ["bash", str(SCRIPT), "v1.0.2", "HEAD", "main"],
                cwd=repo,
                text=True,
                capture_output=True,
            )

            self.assertEqual(result.returncode, 0, result.stderr)


if __name__ == "__main__":
    unittest.main()