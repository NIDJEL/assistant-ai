import importlib.util
import pathlib
import unittest

spec=importlib.util.spec_from_file_location('audit',pathlib.Path(__file__).with_name('security-audit.py'))
audit=importlib.util.module_from_spec(spec)
spec.loader.exec_module(audit)

class AuditTests(unittest.TestCase):
    def test_known_credentials_are_always_detected(self):
        value=b'unit-only-credential-value'
        self.assertIn('known',audit.findings(b'prefix '+value+b' suffix',{value:'known'}))
    def test_empty_example_values_do_not_consume_next_line(self):
        raw=b'\n'.join([b'POSTGRES_PASSWORD=',b'SESSION_SECRET=',b'OTHER_CONFIGURATION=value'])
        self.assertEqual([],audit.findings(raw,{}))
    def test_dynamic_test_expression_is_not_a_password(self):
        self.assertEqual([],audit.findings(b'"password":"`+strings.Repeat("x",9000)',{}))
    def test_jwt_and_private_keys_are_detected(self):
        token=b'eyJ'+b'a'*20+b'.'+b'b'*20+b'.'+b'c'*20
        self.assertIn('JWT/access-token candidate',audit.findings(token,{}))
        self.assertIn('private-key material',audit.findings(b'-----BEGIN '+b'PRIVATE KEY-----',{}))
    def test_old_hash_matches_env_assignment_without_value_output(self):
        value=b'unit-old-credential-value'
        digest=audit.hashlib.sha256(value).hexdigest()
        audit.HASHES[digest]='old credential'
        try:self.assertIn('old credential',audit.findings(b'GIGACHAT_AUTHORIZATION_KEY='+value,{}))
        finally:audit.HASHES.pop(digest,None)

if __name__=='__main__':unittest.main()
