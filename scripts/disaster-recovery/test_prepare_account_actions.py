import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent))
spec = importlib.util.spec_from_file_location('prepare_actions', Path(__file__).with_name('prepare-account-actions.py'))
p = importlib.util.module_from_spec(spec)
spec.loader.exec_module(p)

class PreparationGuards(unittest.TestCase):
    def test_non_root_cannot_prepare(self):
        with patch.object(p.os, 'geteuid', return_value=1000):
            with self.assertRaisesRegex(RuntimeError, 'root'):
                p.main()

    def test_active_and_dangling_markers_cannot_prepare(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(p.r, 'BASE', Path(directory)), patch.object(p.os, 'geteuid', return_value=0):
            marker = Path(directory) / '.account-actions-enabled'
            for symlink in [False, True]:
                if symlink: marker.symlink_to(Path(directory) / 'missing')
                else: marker.write_text('enabled')
                with self.assertRaisesRegex(RuntimeError, 'inactive'):
                    p.main()
                marker.unlink()

    def test_release_without_profile_cannot_prepare(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(p.r, 'BASE', Path(directory)), patch.object(p.os, 'geteuid', return_value=0):
            release = Path(directory) / 'release'; release.mkdir()
            (Path(directory) / 'current').symlink_to(release)
            with self.assertRaisesRegex(RuntimeError, 'release'):
                p.main()
            self.assertFalse((Path(directory) / '.env').exists())
