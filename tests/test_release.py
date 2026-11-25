"""Release archive regressions; independent of GitHub or personal credentials."""
import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
import zipfile

spec = importlib.util.spec_from_file_location('release', Path(__file__).resolve().parents[1] / 'scripts/release.py')
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)


class ReleaseTests(unittest.TestCase):
    def test_version(self):
        self.assertEqual(release.version(), '0.1.0')

    def test_registry(self):
        registry = json.loads((release.ROOT / 'registry.json').read_text())
        self.assertEqual(registry['schema_version'], 1)
        self.assertEqual(len(registry['plugins']), 1)
        plugin = registry['plugins'][0]
        self.assertEqual(plugin['id'], release.ID)
        self.assertEqual(plugin['repository'], 'https://github.com/' + release.REPOSITORY)
        for key in ['id', 'name', 'description', 'author', 'repository']:
            self.assertTrue(plugin[key])
        self.assertEqual(plugin['install']['type'], 'github-release')

    def test_layout_and_checksum_rejections(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            archive = root / 'plugin.zip'
            checksums = root / 'checksums.txt'
            elf = b'\x7fELF\x02' + bytes(13) + b'\x3e\x00' + bytes(32)
            with zipfile.ZipFile(archive, 'w') as z:
                z.writestr('quota-first-router.so', elf)
                z.writestr('LICENSE', 'license')
                z.writestr('THIRD_PARTY_NOTICES.md', 'notices')
            checksums.write_text(hashlib.sha256(archive.read_bytes()).hexdigest() + '  plugin.zip\n')
            release.verify_archive(archive, checksums)
            release.validate_package(archive)
            checksums.write_text('0' * 64 + '  plugin.zip\n')
            with self.assertRaises(AssertionError):
                release.verify_archive(archive, checksums)
            with zipfile.ZipFile(archive, 'a') as z:
                z.writestr('../unexpected.so', elf)
            with self.assertRaises(AssertionError):
                release.validate_package(archive)


if __name__ == '__main__':
    unittest.main()
