#!/usr/bin/env python3
"""Installed, root-owned SSH receiver. Reads a revision line and tar.gz from stdin.

The SSH key is restricted to this command. Compose and Dockerfile templates are
installed separately by an administrator; an uploaded archive cannot change them.
"""
import fcntl
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import signal
import shutil
import subprocess
import sys
import tarfile
import tempfile
import time
import urllib.request

BASE = Path('/opt/rhythm')
ENV = Path('/etc/rhythm/production.env')
TEMPLATES = Path('/etc/rhythm/deploy')
ERROR_PAGES = Path('/var/www/rhythm-errors')
MAX_SIZE = 512 * 1024 * 1024


def run(args, timeout=180):
    # Compose output can contain substituted credentials. Never print it.
    result = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=timeout)
    if result.returncode:
        raise RuntimeError('command failed: ' + Path(args[0]).name)
    return result.stdout


def compose(release):
    return ['docker', 'compose', '--env-file', str(ENV),
            '-f', str(release / 'deploy/compose.production.yaml'),
            '-f', str(release / 'deploy/compose.gigachat.example.yaml')]


def allowed(name):
    path = PurePosixPath(name)
    return (not path.is_absolute() and '..' not in path.parts and '\\' not in name
            and (name in {'.release', '.release/server', 'REVISION', 'web', 'web/dist',
                          'docs', 'docs/architecture.html', 'docs/diagrams', 'data'}
                 or name.startswith(('web/dist/', 'docs/diagrams/', 'data/'))))


def extract(archive, destination):
    total = 0
    seen = set()
    with tarfile.open(archive, 'r:gz') as source:
        for member in source:
            name = member.name.rstrip('/')
            total += member.size
            if (not allowed(name) or not (member.isfile() or member.isdir())
                    or name in seen or total > MAX_SIZE or len(seen) >= 20000):
                raise ValueError('invalid release archive')
            seen.add(name)
            target = destination / name
            if member.isdir():
                target.mkdir(parents=True, exist_ok=True)
            else:
                target.parent.mkdir(parents=True, exist_ok=True)
                with source.extractfile(member) as incoming, target.open('xb') as outgoing:
                    shutil.copyfileobj(incoming, outgoing)
                target.chmod(0o755 if name == '.release/server' else 0o644)
    for name in ('.release/server', 'web/dist/index.html', 'REVISION', 'docs/architecture.html'):
        if not (destination / name).is_file():
            raise ValueError('incomplete release archive')


def set_tag(content, tag):
    lines = [line for line in content.splitlines() if not re.match(r'^\s*RELEASE_TAG\s*=', line)]
    return '\n'.join(lines + ['RELEASE_TAG=' + tag]) + '\n'


def write_env(content):
    temporary = ENV.with_name('.production.env.cicd')
    fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, 'w') as stream:
        stream.write(content)
        stream.flush()
        os.fsync(stream.fileno())
    temporary.chmod(0o600)
    os.replace(temporary, ENV)


def point_current(release):
    temporary = BASE / '.current-cicd'
    temporary.unlink(missing_ok=True)
    temporary.symlink_to(release, target_is_directory=True)
    os.replace(temporary, BASE / 'current')


def install_error_pages(release):
    """Nginx must be able to serve errors without the app or its private folder."""
    source = release / 'web/dist/errors'
    if not source.is_dir():
        return  # Older releases predate standalone error pages.
    revision = (release / 'REVISION').read_text().strip()
    if not re.fullmatch('[0-9a-f]{40}', revision):
        raise ValueError('invalid error-page revision')
    versions = ERROR_PAGES.with_name('rhythm-error-releases')
    versions.mkdir(parents=True, exist_ok=True); versions.chmod(0o755)
    target = versions / revision
    target.mkdir(exist_ok=True); target.chmod(0o755)
    for status in range(400, 600):
        incoming = source / (str(status) + '.html')
        if not incoming.is_file():
            raise ValueError('incomplete standalone error pages')
        outgoing = target / incoming.name
        shutil.copyfile(incoming, outgoing); outgoing.chmod(0o644)
    temporary = ERROR_PAGES.with_name('.rhythm-errors-new')
    temporary.unlink(missing_ok=True)
    temporary.symlink_to(target, target_is_directory=True)
    os.replace(temporary, ERROR_PAGES)


def health(release):
    run(compose(release) + ['exec', '-T', 'app', '/app/server', '-healthcheck'])
    config = json.loads(run(compose(release) + ['config', '--format', 'json']))
    public = config['services']['app']['environment']['PUBLIC_URL'].rstrip('/')
    if not public.startswith('https://'):
        raise RuntimeError('production PUBLIC_URL must use HTTPS')
    for path in ('/health', '/auth'):
        with urllib.request.urlopen(public + path, timeout=20) as response:
            body = response.read(2 * 1024 * 1024)
            if response.status != 200 or (path == '/auth' and body != (release / 'web/dist/index.html').read_bytes()):
                raise RuntimeError('public health check failed')


def backup(previous, revision):
    # Use installed code, never a script supplied in the release archive.
    folder = Path('/var/backups/rhythm') / ('cicd-' + time.strftime('%Y%m%dT%H%M%SZ', time.gmtime()) + '-' + revision[:12])
    folder.mkdir(mode=0o700, parents=True)
    shutil.copy2(ENV, folder / 'production.env')
    (folder / 'previous-release').write_text(str(previous))
    dump = folder / 'postgres.dump'
    with dump.open('xb') as output:
        result = subprocess.run(compose(previous) + ['exec', '-T', 'postgres', 'sh', '-c',
            'PGPASSWORD="$POSTGRES_PASSWORD" pg_dump -h 127.0.0.1 -Fc -U "$POSTGRES_USER" -d "$POSTGRES_DB"'],
            stdout=output, stderr=subprocess.PIPE, timeout=180)
    if result.returncode or dump.stat().st_size == 0:
        raise RuntimeError('database backup failed')
    with dump.open('rb') as source:
        result = subprocess.run(compose(previous) + ['exec', '-T', 'postgres', 'pg_restore', '--list'],
                                stdin=source, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=60)
    if result.returncode:
        raise RuntimeError('database backup verification failed')
    return folder


def activate(release, revision):
    previous = (BASE / 'current').resolve(strict=True)
    old_env = ENV.read_text()
    # Image build finishes before the production container is touched.
    run(['docker', 'build', '-f', str(TEMPLATES / 'Dockerfile.release'),
         '-t', 'rhythm-app:sha-' + revision, str(release)], timeout=600)
    saved = backup(previous, revision)
    try:
        write_env(set_tag(old_env, 'sha-' + revision))
        run(compose(release) + ['config', '--quiet'])
        run(compose(release) + ['up', '-d', '--no-deps', '--no-build', '--wait', '--wait-timeout', '120', 'app'])
        health(release)
        install_error_pages(release)
        point_current(release)
    except Exception:
        write_env(old_env)
        point_current(previous)
        run(compose(previous) + ['up', '-d', '--no-deps', '--no-build', '--wait', '--wait-timeout', '120', 'app'])
        health(previous)
        raise RuntimeError('deployment failed; previous application restored; database was not restored') from None
    return {'revision': revision, 'backup': str(saved), 'health': 'passed'}


def receive(stream):
    revision = stream.readline(42).decode('ascii').strip()
    if not re.fullmatch('[0-9a-f]{40}', revision):
        raise ValueError('invalid revision')
    BASE.joinpath('releases').mkdir(exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='.incoming-', dir=BASE / 'releases') as temporary:
        folder = Path(temporary)
        archive = folder / 'release.tar.gz'
        digest = hashlib.sha256()
        size = 0
        with archive.open('xb') as output:
            while True:
                block = stream.read(1024 * 1024)
                if not block:
                    break
                size += len(block)
                if size > MAX_SIZE:
                    raise ValueError('release too large')
                output.write(block)
                digest.update(block)
        release = folder / 'content'
        release.mkdir()
        extract(archive, release)
        if (release / 'REVISION').read_text().strip() != revision:
            raise ValueError('revision mismatch')
        (release / 'deploy').mkdir()
        for name in ('compose.production.yaml', 'compose.gigachat.example.yaml', 'postgres-init.sh'):
            shutil.copy2(TEMPLATES / name, release / 'deploy' / name)
        target = BASE / 'releases' / ('sha-' + revision)
        if target.exists():
            if (BASE / 'current').resolve() == target:
                health(target)
                return {'revision': revision, 'already_deployed': True}
            marker = target / '.archive-sha256'
            if not marker.is_file() or marker.read_text().strip() != digest.hexdigest():
                raise ValueError('release already exists with a different archive')
        else:
            (release / '.archive-sha256').write_text(digest.hexdigest() + '\n')
            release.rename(target)
        result = activate(target, revision)
        result['archive_sha256'] = digest.hexdigest()
        return result


def main():
    os.umask(0o077)
    # Finish activation/rollback if the SSH client disconnects after upload.
    signal.signal(signal.SIGHUP, signal.SIG_IGN)
    with open('/run/lock/rhythm-deploy.lock', 'w') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        print(json.dumps(receive(sys.stdin.buffer)))


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        # Unexpected exception details may contain secrets from external libraries.
        print(json.dumps({'error': str(error) if isinstance(error, (ValueError, RuntimeError)) else type(error).__name__}), file=sys.stderr)
        sys.exit(1)
