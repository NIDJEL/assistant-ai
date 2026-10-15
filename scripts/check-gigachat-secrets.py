"""Compare local known secrets to Git-visible files without printing values."""
import base64
import json
import pathlib
import subprocess
import sys

root = pathlib.Path(__file__).resolve().parents[1]
env = dict(line.split('=',1) for line in (root/'.env').read_text(encoding='utf-8-sig').splitlines() if '=' in line and not line.startswith('#'))
names = ['GIGACHAT_AUTHORIZATION_KEY','GIGACHAT_CLIENT_ID','GIGACHAT_CLIENT_SECRET']
secrets = [env[name].encode() for name in names if env.get(name)]
if len(secrets) != 3:
    sys.exit('Missing configured GigaChat credential')
secrets += [base64.b64encode(value) for value in secrets]
tracked = subprocess.check_output(['git','ls-files','-z'],cwd=root).decode().split('\0')
visible = subprocess.check_output(['git','ls-files','-z','--cached','--others','--exclude-standard'],cwd=root).decode().split('\0')
failures = []
for name in visible:
    path = root / name
    if name and path.is_file() and any(secret in path.read_bytes() for secret in secrets):
        failures.append(name)
for folder in ['artifacts/gigachat','artifacts/cache/semantic']:
    for path in (root/folder).rglob('*'):
        if path.is_file() and any(secret in path.read_bytes() for secret in secrets):
            failures.append(str(path.relative_to(root)))
        if path.is_file() and path.suffix == '.json':
            raw = path.read_bytes()
            # Auth envelopes must never enter reports or evaluation caches.
            if b'"access_token"' in raw or b'"authorization"' in raw.lower():
                failures.append(str(path.relative_to(root)))
ignored = subprocess.run(['git','check-ignore','-q','.env'],cwd=root).returncode == 0
report = {'env_ignored':ignored, 'env_tracked':'.env' in tracked, 'credential_matches':len(failures), 'files_with_matches':failures}
print(json.dumps(report))
if not ignored or '.env' in tracked or failures:
    sys.exit(1)
