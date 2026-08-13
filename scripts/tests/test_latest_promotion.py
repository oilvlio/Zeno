import importlib.util
import pathlib
import unittest


PATH = pathlib.Path(__file__).parents[1] / "latest-promotion.py"
SPEC = importlib.util.spec_from_file_location("latest_promotion", PATH)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class LatestPromotionTest(unittest.TestCase):
    def test_highest_stable_promotes(self):
        self.assertEqual(MODULE.decision("v2.0.0", ["v1.9.9", "v2.0.0"]), (True, "v2.0.0"))

    def test_next_patch_promotes_under_standard_semver(self):
        self.assertEqual(
            MODULE.decision("v1.0.3", ["v0.22.6", "v1.0.2", "v1.0.3"]),
            (True, "v1.0.3"),
        )

    def test_no_release_line_exception_can_override_higher_semver(self):
        self.assertEqual(
            MODULE.decision("v1.0.3", ["v1.0.3", "v2.0.0"]),
            (False, "v2.0.0"),
        )

    def test_older_workflow_cannot_roll_back_latest(self):
        self.assertEqual(MODULE.decision("v1.9.9", ["v1.9.9", "v2.0.0"]), (False, "v2.0.0"))

    def test_prerelease_never_promotes(self):
        self.assertEqual(MODULE.decision("v3.0.0-rc.1", ["v2.0.0", "v3.0.0-rc.1"]), (False, "v2.0.0"))

    def test_semver_numeric_order_not_lexical(self):
        self.assertEqual(MODULE.decision("v1.10.0", ["v1.9.99", "v1.10.0"]), (True, "v1.10.0"))


    def test_build_metadata_and_malformed_tags_do_not_promote(self):
        self.assertEqual(MODULE.decision("v2.0.0+build", ["v1.0.0", "v2.0.0+build"]), (False, "v1.0.0"))
