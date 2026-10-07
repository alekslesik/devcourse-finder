import contextlib
import io
import pathlib
import sys
import tempfile
import unittest
from unittest.mock import patch

import release


class ReleaseTests(unittest.TestCase):
    def test_rejects_ambiguous_versions(self):
        for value in ["v1.2.3", "01.2.3", "1.2", "1.2.3-rc1", "1.2.3\n"]:
            with self.subTest(value=value), self.assertRaises(ValueError):
                release.parse_version(value)

    def test_extracts_only_selected_notes(self):
        text = "# Changelog\n\n## [1.2.3] - 2026-10-07\n\n- New.\n\n## [1.2.2] - 2026-10-06\n\n- Old.\n"
        self.assertEqual(release.release_notes(text, "1.2.3"), "- New.")

    def test_rejects_empty_duplicate_or_placeholder_notes(self):
        for suffix in ["", "- TODO fix", "- Ready.\n\n## [1.2.3] - 2026-10-07\n- Duplicate."]:
            with self.subTest(suffix=suffix), self.assertRaises(ValueError):
                release.release_notes("## [1.2.3] - 2026-10-07\n" + suffix, "1.2.3")

    def test_minor_bump_updates_version_and_preserves_previous_history(self):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            (root / "VERSION").write_text("1.2.3\n")
            old = "# Changelog\n\n## [1.2.3] - 2026-10-07\n\n- Existing.\n"
            (root / "CHANGELOG.md").write_text(old)
            with patch.object(release, "ROOT", root), patch.object(sys, "argv", ["release.py", "bump", "minor", "--note", "Manual deployment."]), contextlib.redirect_stdout(io.StringIO()):
                release.main()
            self.assertEqual((root / "VERSION").read_text(), "1.3.0\n")
            notes = (root / "CHANGELOG.md").read_text()
            self.assertEqual(release.release_notes(notes, "1.3.0"), "- Manual deployment.")
            self.assertIn("## [1.2.3] - 2026-10-07\n\n- Existing.", notes)


if __name__ == "__main__":
    unittest.main()
