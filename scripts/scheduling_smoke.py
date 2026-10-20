"""Exercise real simple_llm with new test accounts only; never prints credentials.

python scripts/scheduling_smoke.py --base-url http://127.0.0.1:8084
"""
import argparse
import datetime as dt
import json
import re
import uuid
from social_smoke import Account


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--base-url', required=True)
    args = parser.parse_args()
    a = Account(args.base_url, 'Никита Круглов', 'scheduling-a')
    b = Account(args.base_url, 'Матвей Лапин', 'scheduling-b')
    c = Account(args.base_url, 'Алиса Проверка', 'scheduling-c')
    for friend in (a, c):
        request = b.op('request', code=friend.call('/api/social/people')['profile']['code'])
        friend.op('accept_request', id=request['id'])
    b.op('tags', user_id=a.user['id'], tags=['ХУЕСОС', 'Коллега'])
    b.op('tags', user_id=c.user['id'], tags=['Коллега'])
    health = a.call('/health')
    now = dt.datetime.fromisoformat(re.sub(r'(\.\d{6})\d+', r'\1', health['now']).replace('Z', '+00:00'))
    today = now.date()
    tomorrow = today + dt.timedelta(days=1)
    existing_day = today + dt.timedelta(days=2)
    create = lambda day, clock, title='Встреча', minutes=60: {'kind': 'meeting', 'title': title, 'date': day.isoformat(), 'time': clock, 'duration_minutes': minutes}
    count = lambda who: len(who.call('/api/social/meetings')['meetings'])
    existing = b.op('create_meeting', event=create(existing_day, '17:00'), friend_ids=[a.user['id']])
    eid = existing['event_id']
    a.op('respond', event_id=eid, version=1, status='accepted')
    r = a.chat(f'У меня встреча {existing_day.day} числа с Лапиным Матвеем. Создай и добавь, пожалуйста, ссылку для Zoom-звонка')
    assert not r['pending'] and count(a) == 1 and r['events'][0]['id'] == eid, 'A: existing meeting not resolved'
    assert r['events'][0]['organizer_id'] == b.user['id'] and 'организатор' in r['reply'].lower(), 'A/I: incorrect organizer'
    assert 'Матвеем Лапиным' in r['events'][0]['display_title'], 'A: generic display title'
    r = b.chat('Назначь встречу с Никитой завтра в 15:00')
    assert not r['pending'] and r['events'][0]['date'] == tomorrow.isoformat(), 'B: name resolution failed'
    assert r['events'][0]['title'] == 'Встреча с Никитой Кругловым', 'B: real-name title missing'
    r = b.chat('Назначь встречу на час дня с ХУЕСОСОМ')
    assert not r['pending'] and r['events'][0]['date'] == today.isoformat() and r['events'][0]['time'] == '13:00', 'C: inflected tag/date/time failed'
    assert r['events'][0]['title'] == 'Встреча с Никитой Кругловым', 'C: private tag leaked into title'
    a.call('/api/events', create(today, '16:00', 'PRIVATE TEST EVENT'))
    r = b.chat('Я назначу хуесосу встречу на сегодня в четыре часа дня')
    assert r['pending'] and r.get('scheduling', {}).get('slots'), 'D: conflict lost command or slots'
    assert 'PRIVATE' not in json.dumps(r, ensure_ascii=False), 'D: private event leaked'
    offer = r['scheduling']
    slot = offer['slots'][0]
    assert dt.datetime.fromisoformat(slot['start']) >= now, 'D: past suggestion'
    request_id = str(uuid.uuid4())
    body = {'message': 'Выбираю предложенное время', 'session_id': b.sid, 'request_id': request_id, 'choice': {'token': offer['token'], 'action': 'slot', 'index': 1, 'slot': slot}}
    before = count(b)
    selected = b.call('/api/chat', body)
    replay = b.call('/api/chat', body)
    assert not selected['pending'] and selected['events'][0]['id'] == replay['events'][0]['id'] and count(b) == before + 1, 'D: duplicate or wrong invitation'
    r = b.chat('Назначь встречу с Никитой сегодня в 16:00')
    assert r['pending'] and len(r.get('scheduling', {}).get('slots', [])) >= 2, 'E: no alternatives'
    second = dt.datetime.fromisoformat(r['scheduling']['slots'][1]['start'])
    r = b.chat('Давай второй')
    assert not r['pending'] and r['events'][0]['date'] == second.date().isoformat() and r['events'][0]['time'] == second.strftime('%H:%M'), 'E: ordinal lost offer'
    r = b.chat('Назначь встречу с Евгением Незнакомцевым завтра в 14')
    assert r['pending'] and not r['events'], 'F: invented friend'
    b.chat('Отмена')
    before = count(b)
    r = b.chat('Создай встречу с коллегой завтра в 12:00')
    assert r['pending'] and count(b) == before and 'Никита' in r['reply'] and 'Алиса' in r['reply'], 'G: ambiguous tag silently selected'
    b.chat('Отмена')
    r = b.chat('Создай встречу завтра в 10:00')
    assert r['pending'] and 'по какому поводу' in r['reply'], 'H: generic title accepted'
    r = b.chat('Обсудим дизайн приложения')
    assert not r['pending'] and r['events'][0]['time'] == '10:00' and 'дизайн' in r['events'][0]['title'].lower(), 'H: subject continuation lost schedule'
    print(json.dumps({'status': 'PASS', 'accounts': 3, 'A_existing_and_roles': 'PASS', 'B_name': 'PASS', 'C_inflected_private_tag': 'PASS', 'D_conflict_and_structured_choice': 'PASS', 'E_ordinal_continuation': 'PASS', 'F_unknown_friend': 'PASS', 'G_ambiguous_tag': 'PASS', 'H_subject_continuation': 'PASS', 'I_zoom_organizer_permissions': 'PASS', 'J_live_zoom_oauth': 'NOT_CONFIGURED'}, ensure_ascii=False), flush=True)


if __name__ == '__main__':
    main()
