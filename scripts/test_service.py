import importlib.util
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
from types import SimpleNamespace

spec = importlib.util.spec_from_file_location('service', Path(__file__).with_name('service.py'))
service = importlib.util.module_from_spec(spec); spec.loader.exec_module(service)


class LauncherTests(unittest.TestCase):
    def test_first_start_generates_secrets_and_repeat_preserves_configuration(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / '.env.example').write_text('\n'.join([
                'POSTGRES_PASSWORD=', 'SESSION_SECRET=',
                'GENA_NLU_MODE=semantic_plan_legacy', 'GENA_SEMANTIC_PROVIDER=rules', ''
            ]))
            args = SimpleNamespace(port=18086, mode=None)
            with patch.object(service, 'ROOT', root):
                first, mode = service.prepare(args, root / '.env')
                second, again = service.prepare(args, root / '.env')
            self.assertEqual(first, second)
            self.assertEqual((mode, again), ('demo', 'demo'))
            self.assertGreaterEqual(len(first['SESSION_SECRET']), 32)
            self.assertNotEqual(first['SESSION_SECRET'], first['POSTGRES_PASSWORD'])
            self.assertEqual(first['PUBLIC_URL'], 'http://localhost:18086')

    def test_full_mode_requires_credentials_without_replacing_existing_secrets(self):
        with tempfile.TemporaryDirectory() as temporary:
            env = Path(temporary) / '.env'
            env.write_text('POSTGRES_PASSWORD=unit-preserved-password\nSESSION_SECRET=' + 'x' * 40 + '\nGENA_NLU_MODE=semantic_plan_legacy\nGENA_SEMANTIC_PROVIDER=rules\n')
            args = SimpleNamespace(port=None, mode='full')
            before = env.read_text()
            with self.assertRaisesRegex(ValueError, 'GIGACHAT_AUTHORIZATION_KEY'):
                service.prepare(args, env)
            self.assertEqual(env.read_text(), before)
            service.write_env(env, {'GIGACHAT_AUTHORIZATION_KEY': 'fixture-only', 'GROQ_API_KEY': 'fixture-only'})
            values, mode = service.prepare(args, env)
            self.assertEqual(mode, 'full')
            self.assertEqual(values['SESSION_SECRET'], 'x' * 40)
            self.assertEqual(values['POSTGRES_PASSWORD'], 'unit-preserved-password')
            self.assertEqual(values['GENA_NLU_MODE'], 'simple_llm')
            self.assertEqual(values['GENA_STT_PROVIDER'], 'groq')

    def test_existing_ai_configuration_cannot_be_silently_downgraded(self):
        with tempfile.TemporaryDirectory() as temporary:
            env = Path(temporary) / '.env'
            env.write_text('POSTGRES_PASSWORD=unit-preserved\nSESSION_SECRET=' + 'x' * 40 + '\nGENA_NLU_MODE=simple_llm\nGENA_SEMANTIC_PROVIDER=gigachat\n')
            before = env.read_text()
            with self.assertRaisesRegex(ValueError, 'Existing env selects AI'):
                service.prepare(SimpleNamespace(port=None, mode='demo'), env)
            self.assertEqual(env.read_text(), before)


if __name__ == '__main__':
    unittest.main()
