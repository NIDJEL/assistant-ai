#!/usr/bin/env python3
"""One-command local/reviewer deployment. Python stdlib + Docker Compose v2 only."""
import argparse
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import subprocess
import sys
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[1]


def read_env(path):
    values = {}
    if path.exists():
        for line in path.read_text(encoding='utf-8-sig').splitlines():
            match = re.match(r'^([A-Z][A-Z0-9_]*)=(.*)$', line.strip())
            if match:
                values[match[1]] = match[2].strip().strip('"').strip("'")
    return values


def write_env(path, updates):
    content = path.read_text(encoding='utf-8-sig') if path.exists() else ''
    lines = [line for line in content.splitlines() if line.partition('=')[0].strip() not in updates]
    content = '\n'.join(lines + [key + '=' + value for key, value in updates.items()]) + '\n'
    temporary = path.with_name(path.name + '.new')
    descriptor = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(descriptor, 'w', encoding='utf-8', newline='\n') as stream:
        stream.write(content)
    temporary.chmod(0o600)
    os.replace(temporary, path)


def run(command, *, capture=False):
    result = subprocess.run(command, cwd=ROOT, text=True, encoding='utf-8', errors='replace',
                            stdout=subprocess.PIPE if capture else None,
                            stderr=subprocess.PIPE if capture else None)
    if result.returncode:
        raise RuntimeError('Docker operation failed. Check Docker Desktop/daemon and the preceding build logs.')
    return result.stdout or ''


def prepare(args, path):
    created = not path.exists()
    if created:
        path.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(ROOT / '.env.example', path)
        path.chmod(0o600)
    values = read_env(path)
    updates = {}
    if not values.get('POSTGRES_PASSWORD'):
        updates['POSTGRES_PASSWORD'] = secrets.token_hex(24)
    if not values.get('SESSION_SECRET'):
        updates['SESSION_SECRET'] = secrets.token_hex(32)
    if created:
        port = args.port or 8081
        updates.update(APP_HTTP_PORT=str(port), PUBLIC_URL='http://localhost:' + str(port))
        updates['DATABASE_URL'] = 'postgres://rhythm:' + updates['POSTGRES_PASSWORD'] + '@127.0.0.1:5432/rhythm?sslmode=disable'
    elif args.port and str(args.port) != values.get('APP_HTTP_PORT', '8081'):
        raise ValueError('An existing env file uses another port. Use a new --env-file or edit APP_HTTP_PORT and PUBLIC_URL together.')
    if updates:
        write_env(path, updates)
        values.update(updates)
    mode = args.mode or ('full' if values.get('GENA_NLU_MODE') == 'simple_llm' else 'demo')
    if mode == 'full':
        if not values.get('GIGACHAT_AUTHORIZATION_KEY'):
            raise ValueError('Full mode requires GIGACHAT_AUTHORIZATION_KEY in the env file. See docs/QUICKSTART.md.')
        updates = {'GENA_NLU_MODE': 'simple_llm', 'GENA_SEMANTIC_PROVIDER': 'gigachat'}
        if values.get('GROQ_API_KEY'):
            updates['GENA_STT_PROVIDER'] = 'groq'
        if values.get('AZURE_SPEECH_KEY'):
            updates['GENA_TTS_PROVIDER'] = 'azure'
        write_env(path, updates); values.update(updates)
    elif values.get('GENA_NLU_MODE') != 'semantic_plan_legacy' or values.get('GENA_SEMANTIC_PROVIDER') != 'rules':
        raise ValueError('Existing env selects AI. Use --mode full, or a separate --env-file for the key-free demo.')
    return values, mode


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['up', 'down', 'status', 'logs', 'check'], nargs='?', default='up')
    parser.add_argument('--env-file', default='.env', help='Existing settings are preserved; missing secrets are generated')
    parser.add_argument('--project', default='rhythm-local', help='Separate Compose project and persistent database volume')
    parser.add_argument('--mode', choices=['demo', 'full'])
    parser.add_argument('--port', type=int, help='HTTP port, only when creating a new env file')
    parser.add_argument('--no-build', action='store_true', help='Use the previously built local image')
    args = parser.parse_args()
    if args.port is not None and not 1024 <= args.port <= 65535:
        raise ValueError('--port must be between 1024 and 65535')
    if not re.fullmatch(r'[a-z0-9][a-z0-9_-]*', args.project):
        raise ValueError('Invalid Compose project name')
    if shutil.which('docker') is None:
        raise RuntimeError('Install Docker Desktop (Windows/macOS) or Docker Engine with Compose v2 (Linux).')
    run(['docker', 'compose', 'version'], capture=True)
    run(['docker', 'info'], capture=True)
    path = (ROOT / args.env_file).resolve()
    if args.action == 'up':
        values, mode = prepare(args, path)
    else:
        if not path.exists():
            raise ValueError('Env file not found. Run the up command first.')
        values = read_env(path); mode = 'full' if values.get('GENA_NLU_MODE') == 'simple_llm' else 'demo'
    command = ['docker', 'compose', '--project-name', args.project, '--env-file', str(path), '-f', str(ROOT / 'compose.yaml')]
    ca = values.get('GIGACHAT_CA_FILE_HOST') or values.get('GIGACHAT_CA_FILE', '')
    if mode == 'full' and ca:
        certificate = Path(ca).expanduser().resolve()
        if not certificate.is_file():
            raise ValueError('GIGACHAT_CA_FILE_HOST must point to an existing verified CA bundle.')
        runtime = ROOT / 'var' / 'runtime'; runtime.mkdir(parents=True, exist_ok=True)
        override = runtime / (args.project + '.json')
        override.write_text(json.dumps({'services': {'app': {'environment': {'GIGACHAT_CA_FILE': '/etc/gigachat/ca.pem'},
            'volumes': [{'type': 'bind', 'source': str(certificate), 'target': '/etc/gigachat/ca.pem', 'read_only': True}]}}}), encoding='utf-8')
        command += ['-f', str(override)]
    run(command + ['config', '--quiet'], capture=True)
    if args.action == 'up':
        if not args.no_build:
            run(command + ['build', 'app'])
        if not values.get('WEB_PUSH_VAPID_PUBLIC_KEY') and not values.get('WEB_PUSH_VAPID_PRIVATE_KEY'):
            raw = run(command + ['run', '--rm', '--no-deps', 'app', '-generate-web-push-keys'], capture=True)
            keys = dict(line.split('=', 1) for line in raw.splitlines() if line.startswith('WEB_PUSH_VAPID_'))
            if set(keys) != {'WEB_PUSH_VAPID_PUBLIC_KEY', 'WEB_PUSH_VAPID_PRIVATE_KEY'}:
                raise RuntimeError('The image does not support VAPID key generation. Run again without --no-build.')
            keys['WEB_PUSH_SUBJECT'] = values.get('WEB_PUSH_SUBJECT') or 'https://localhost'
            write_env(path, keys)
        run(command + ['up', '-d', '--wait', '--wait-timeout', '180'])
        check_health(values)
        print('Ready: ' + values.get('PUBLIC_URL', 'http://localhost:8081'))
        print('Mode: ' + mode + '. Create your own account; demo data can be imported from Settings.')
        print('Secrets are saved in ' + str(path) + ' and are never printed. Re-running up preserves them and the database.')
    elif args.action == 'down':
        run(command + ['down'])  # Deliberately never remove database volumes.
    elif args.action == 'logs':
        run(command + ['logs', '--tail', '100', '-f', 'app'])
    elif args.action == 'status':
        run(command + ['ps'])
    else:
        check_health(values)


def check_health(values):
    port = values.get('APP_HTTP_PORT', '8081')
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    with opener.open('http://127.0.0.1:' + port + '/health', timeout=10) as response:
        status = json.load(response)
    if status.get('status') != 'ok' or status.get('storage') != 'postgresql':
        raise RuntimeError('Application health check failed')
    print('Health: ok; storage: PostgreSQL; provider: ' + str(status.get('nlp_provider')))


if __name__ == '__main__':
    try:
        main()
    except (ValueError, RuntimeError, OSError, urllib.error.URLError) as error:
        print('Error: ' + str(error), file=sys.stderr)
        sys.exit(1)
