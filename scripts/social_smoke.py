"""Real two-account API/simple_llm smoke. Uses only newly registered test users.

python scripts/social_smoke.py --base-url http://127.0.0.1:8083
No credentials, cookies, tokens or private provider payloads are printed/saved.
"""
import argparse
import datetime as dt
import http.cookiejar
import json
import secrets
import urllib.error
import urllib.request
import uuid


class Account:
    def __init__(self, base, name, suffix):
        self.base = base.rstrip('/')
        self.client = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
        self.sid = str(uuid.uuid4())
        password = secrets.token_urlsafe(24)
        self.user = self.call('/api/auth/register', {'name': name, 'email': f'social-smoke-{suffix}-{uuid.uuid4().hex[:14]}@example.test', 'password': password, 'password_confirmation': password})['user']
        self.call('/api/auth/onboarding', method='POST')

    def call(self, path, body=None, method=None):
        req = urllib.request.Request(self.base + path, data=None if body is None else json.dumps(body, ensure_ascii=False).encode(), method=method, headers={'Content-Type': 'application/json', 'Origin': self.base})
        try:
            with self.client.open(req, timeout=55) as r:
                return json.load(r)
        except urllib.error.HTTPError as e:
            error = json.load(e).get('error', {})
            raise AssertionError(f'{path}: HTTP {e.code}, {error.get("code")}') from None

    def op(self, action, **kwargs):
        return self.call('/api/social/operations', {'action': action, 'request_id': str(uuid.uuid4()), **kwargs})

    def chat(self, text, request_id=None):
        result = self.call('/api/chat', {'message': text, 'session_id': self.sid, 'request_id': request_id or str(uuid.uuid4())})
        assert result['provider'] == 'simple_llm', result.get('provider')
        print(json.dumps({'command': text, 'reply': result['reply'], 'action': result['action'], 'pending': result['pending']}, ensure_ascii=False), flush=True)
        return result


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--base-url', required=True)
    args = parser.parse_args()
    a = Account(args.base_url, 'Никита Проверка', 'a')
    b = Account(args.base_url, 'Алиса Проверка', 'b')
    profile = a.call('/api/social/people')['profile']
    b.chat('Гена, добавь в друзья по коду ' + profile['code'])
    incoming = a.call('/api/social/people')['incoming']
    assert len(incoming) == 1, 'NLU friend request not created'
    a.op('accept_request', id=incoming[0]['id'])
    a.op('tags', user_id=b.user['id'], tags=['Коллега', 'Тест'])
    assert b.call('/api/social/people')['friends'][0]['tags'] == []
    a.chat('Покажи мои заявки в друзья')
    avail = a.chat('Найди время, когда мы с Алисой Проверка оба свободны завтра')
    assert 'свободн' in avail['reply'].lower() and avail.get('scheduling', {}).get('slots'), 'NLU availability failed'
    rid = str(uuid.uuid4())
    command = 'Назначь встречу с Алисой Проверка завтра в 15:00 на 30 минут'
    a.chat(command, rid)
    a.chat(command, rid)
    meetings = a.call('/api/social/meetings')['meetings']
    assert len(meetings) == 1 and meetings[0]['members'][0]['status'] == 'pending', 'NLU shared create failed'
    b.chat('Предложи Никите Проверка перенести встречу на завтра в 16:30')
    m = a.call('/api/social/meetings')['meetings'][0]
    assert m['proposals'] and m['proposals'][0]['status'] == 'pending', 'NLU proposal failed'
    a.chat('Прими предложение переноса встречи с Алисой Проверка, первый вариант')
    m = a.call('/api/social/meetings')['meetings'][0]
    assert m['version'] == 2 and m['event']['time'] == '16:30', 'NLU proposal acceptance failed'
    b.chat('Прими приглашение на встречу с Никитой Проверка')
    assert a.call('/api/social/meetings')['meetings'][0]['members'][0]['status'] == 'accepted', 'NLU RSVP failed'
    events = b.call('/api/events')['events']
    assert len([e for e in events if e['id'] == m['event']['id']]) == 1, 'shared calendar duplicate'
    a.chat('Пригласи всех друзей с тегом Коллега на встречу завтра')
    assert len(a.call('/api/social/meetings')['meetings'][0]['members']) == 1, 'tag invite duplicate'
    b.chat('Отклони завтрашнюю встречу с Никитой Проверка')
    assert a.call('/api/social/meetings')['meetings'][0]['members'][0]['status'] == 'declined', 'NLU decline failed'
    assert len(b.call('/api/chat/notifications')['exchanges']) >= 2
    a.op('cancel_meeting', event_id=m['event']['id'], version=2)
    print(json.dumps({'status': 'PASS', 'accounts': 2, 'provider': 'simple_llm', 'friendship': True, 'private_tags': True, 'availability': True, 'shared_meeting': True, 'reschedule': True, 'rsvp': True, 'tag_invite': True, 'notifications': True, 'cancellation': True}), flush=True)


if __name__ == '__main__':
    main()
