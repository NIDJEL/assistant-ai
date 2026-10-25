import io
from pathlib import Path
import tarfile
import tempfile
import unittest
from unittest.mock import patch

import release


class ReleaseTests(unittest.TestCase):
    def archive(self, folder, entries):
        path = folder / 'test.tar.gz'
        with tarfile.open(path, 'w:gz') as archive:
            for name, kind, data in entries:
                member = tarfile.TarInfo(name)
                member.type = kind
                member.size = len(data)
                archive.addfile(member, io.BytesIO(data))
        return path

    def test_archive_rejects_escape_links_and_host_configuration(self):
        for name, kind in [('../outside', tarfile.REGTYPE), ('/etc/passwd', tarfile.REGTYPE),
                           ('web/dist/link', tarfile.SYMTYPE), ('data/link', tarfile.LNKTYPE),
                           ('deploy/compose.production.yaml', tarfile.REGTYPE),
                           ('web/dist/../../etc', tarfile.REGTYPE), ('web\\dist\\x', tarfile.REGTYPE)]:
            with self.subTest(name=name), tempfile.TemporaryDirectory() as temporary:
                folder = Path(temporary)
                archive = self.archive(folder, [(name, kind, b'')])
                with self.assertRaises(ValueError):
                    release.extract(archive, folder / 'output')

    def test_archive_requires_binary_frontend_and_revision(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            archive = self.archive(folder, [('data/a.json', tarfile.REGTYPE, b'{}')])
            with self.assertRaisesRegex(ValueError, 'incomplete'):
                release.extract(archive, folder / 'output')

    def test_environment_update_preserves_credentials_and_settings(self):
        before = '# keep comment\nSESSION_SECRET="same value"\nPUBLIC_URL=https://example.test\nRELEASE_TAG=old\n'
        after = release.set_tag(before, 'sha-new')
        self.assertIn('SESSION_SECRET="same value"\n', after)
        self.assertIn('PUBLIC_URL=https://example.test\n', after)
        self.assertNotIn('RELEASE_TAG=old', after)
        self.assertEqual(after.count('RELEASE_TAG='), 1)

    def test_failed_health_restores_previous_application(self):
        with tempfile.TemporaryDirectory() as temporary:
            base = Path(temporary)
            old, new = base / 'old', base / 'new'
            old.mkdir()
            new.mkdir()
            (base / 'current').symlink_to(old, target_is_directory=True)
            env = base / 'production.env'
            env.write_text('SESSION_SECRET=preserved\nRELEASE_TAG=previous\n')
            with patch.object(release, 'BASE', base), patch.object(release, 'ENV', env), \
                    patch.object(release, 'run') as run, patch.object(release, 'backup', return_value=base), \
                    patch.object(release, 'health', side_effect=[RuntimeError('unhealthy'), None]) as health:
                with self.assertRaisesRegex(RuntimeError, 'previous application restored'):
                    release.activate(new, 'a' * 40)
                self.assertEqual((base / 'current').resolve(), old)
                self.assertEqual(env.read_text(), 'SESSION_SECRET=preserved\nRELEASE_TAG=previous\n')
                self.assertEqual(health.call_args_list[-1].args, (old,))
                starts = [call.args[0] for call in run.call_args_list if 'up' in call.args[0]]
                self.assertEqual(len(starts), 2)
                self.assertTrue(all('--no-deps' in args and '--no-build' in args for args in starts))

    def test_build_failure_never_changes_production(self):
        with tempfile.TemporaryDirectory() as temporary:
            base = Path(temporary)
            old = base / 'old'
            old.mkdir()
            (base / 'current').symlink_to(old, target_is_directory=True)
            env = base / 'production.env'
            env.write_text('RELEASE_TAG=previous\n')
            with patch.object(release, 'BASE', base), patch.object(release, 'ENV', env), \
                    patch.object(release, 'run', side_effect=RuntimeError('build failed')), \
                    patch.object(release, 'backup') as backup:
                with self.assertRaisesRegex(RuntimeError, 'build failed'):
                    release.activate(base / 'new', 'a' * 40)
                backup.assert_not_called()
                self.assertEqual(env.read_text(), 'RELEASE_TAG=previous\n')
                self.assertEqual((base / 'current').resolve(), old)


if __name__ == '__main__':
    unittest.main()
