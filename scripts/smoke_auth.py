"""PostgreSQL container smoke: auth, isolation, CRUD and session after restart."""
import argparse
import json
import os
import secrets
from pathlib import Path
from urllib.error import HTTPError
from urllib.request import Request, urlopen

parser = argparse.ArgumentParser()
parser.add_argument('--phase', choices=['prepare', 'verify'], required=True)
parser.add_argument('--base', default='http://localhost:8081')
parser.add_argument('--state', default='tmp/p12-smoke.json')
args = parser.parse_args()
state_path = Path(args.state)
state = {}
if args.phase == 'verify':
    state = json.loads(state_path.read_text(encoding='utf-8'))


def request(method, path, body=None, authenticated=True, status=200):
    headers = {'Content-Type': 'application/json'}
    if authenticated and state.get('cookie'):
        headers['Cookie'] = state['cookie']
    req = Request(args.base + path, data=json.dumps(body).encode() if body is not None else None, headers=headers, method=method)
    try:
        response = urlopen(req, timeout=20)
    except HTTPError as exc:
        response = exc
    assert response.status == status, (method, path, response.status, response.read().decode())
    cookie = response.headers.get('Set-Cookie')
    if cookie and cookie.startswith('rhythm_session='):
        state['cookie'] = cookie.split(';')[0]
    return json.loads(response.read()) if status != 204 else None


if args.phase == 'prepare':
    request('GET', '/api/events', authenticated=False, status=401)
    password = os.environ.get('SMOKE_PASSWORD') or secrets.token_urlsafe(24)
    user = request('POST', '/api/auth/register', {'terms_accepted': True, 'personal_data_consent': True, 'legal_version': '2026-10-20', 'name': 'Smoke user', 'email': f'smoke-{secrets.token_hex(8)}@example.test', 'password': password, 'password_confirmation': password}, status=201)['user']
    state['user_id'] = user['id']
    request('POST', '/api/auth/onboarding', {})
    event = request('POST', '/api/events', {'kind': 'task', 'title': 'PostgreSQL persistence smoke'}, status=201)
    state['event_id'] = event['id']
    request('PUT', f"/api/preferences/{user['id']}", {'revision': 1, 'widgets': False, 'layouts': {'today': {'order': ['tasks']}}})
    state_path.parent.mkdir(parents=True, exist_ok=True)
    state_path.write_text(json.dumps(state), encoding='utf-8')
else:
    assert request('GET', '/api/auth/me')['user']['id'] == state['user_id']
    assert request('GET', f"/api/events/{state['event_id']}")['title'] == 'PostgreSQL persistence smoke'
    assert request('GET', f"/api/preferences/{state['user_id']}")['widgets'] is False
    request('DELETE', f"/api/events/{state['event_id']}", status=204)
    request('GET', f"/api/events/{state['event_id']}", status=404)
    request('POST', f"/api/events/{state['event_id']}/restore", {})
    request('POST', '/api/auth/logout', {}, status=204)
    request('GET', '/api/auth/me', status=401)
    state_path.unlink()
print(f'P1.2 Docker smoke {args.phase}: passed')
