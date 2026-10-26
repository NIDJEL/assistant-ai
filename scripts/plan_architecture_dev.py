"""New architecture DEV from root-cause classes; never reads a holdout.

Gold is authored before inference, including actual persisted values.
"""
from datetime import datetime, timedelta
import json
from plan_corpus import ROOT, event, turn, write

NOW = '2027-06-09T10:12:00+03:00'
DAY = '2027-06-10'


def build():
    rows = []
    def add(cat, i, seeds, turns):
        rows.append(dict(ID=f'architecture-dev-{cat}-{i:02}', Category=cat, Now=NOW, Seeds=seeds, Turns=turns))
    def task(key, title, date=DAY, time='08:20', **kw):
        return event(key, title, 'task', date, time, **kw)
    vague = ['Тот вариант, о котором говорили в начале', 'Выбери запись из самого начала разговора',
             'Покажи ранее упомянутую, какую именно — не помню', 'Нужна та из предыдущего обсуждения',
             'Покажи подходящую, но номер я пока не выбрал', 'Нужна одна из них, ещё решаю какая']
    for i in range(30):
        title = f'Калибровка спектрометра {i}'
        seeds = [task('a', title), event('b', title, date=DAY, time='12:20')]
        add('context_preservation', i, seeds, [turn(f'Отыщи «{title}»', 'search', Count=2),
            turn(vague[i % len(vague)], 'clarify', DBCount=2),
            turn('Вторую запись поставь на 17:43', 'update', Count=1, Changes=[dict(Key='b', Time='17:43')])])
    for i in range(30):
        title = f'Проверить изолятор {i}'
        forms = [('Исправляю: назначь 15:47', dict(Time='15:47')),
                 ('Поправка к дате: послезавтра', dict(Date='2027-06-11')),
                 ('Изменяю решение: лучше через 37 минут', dict(Date='2027-06-09', Time='10:49'))]
        text, changes = forms[i % 3]
        add('correction', i, [], [turn(f'Запиши задачу «{title}» на завтра в 14:23', 'create', Count=1,
            Event=dict(Key='a', Kind='task', Title=title, Date=DAY, Time='14:23')),
            turn(text, 'update', Count=1, Changes=[dict(Key='a', **changes)])])
    for i in range(30):
        title = f'Собрать оснастку {i}'
        text = [f'Создай задачу «{title}» на послезавтра', f'Запиши задачу «{title}» на завтра в 18:27',
                f'Через {32+i} минут напомни «{title}»'][i % 3]
        gold = dict(Key='a', Kind='task', Title=title, Date='2027-06-11')
        if i % 3 == 1: gold.update(Date=DAY, Time='18:27')
        if i % 3 == 2:
            at = datetime.fromisoformat(NOW) + timedelta(minutes=32+i)
            gold.update(Kind='reminder', Date=at.strftime('%Y-%m-%d'), Time=at.strftime('%H:%M'))
        add('source_coverage', i, [], [turn(text, 'create', Count=1, Event=gold), turn('Спасибо', 'help', route='fast_path')])
    for i in range(25):
        title = f'Метрология {i}'
        seeds = [task('a', title, Status='done'), event('b', title, date=DAY, time='12:20', Status='done'),
                 event('c', title, date=DAY, time='16:20'), event('d', title, date='2027-07-02')]
        add('refinement', i, seeds, [turn(f'Поищи «{title}» в текущем месяце', 'search', Count=3),
            turn('Ограничь найденное выполненными', 'search', Count=2),
            turn('В этой выборке оставь встречи', 'search', Count=1, Query=title, Start='2027-06-01', End='2027-07-01')])
    for i in range(20):
        n = 11 + i
        text = [f'Включи таймер длительностью {n} минут', f'Нужен таймер продолжительностью {n} минут',
                f'Отсчитай таймером {n} минут', f'Таймер, длительность {n} минут'][i % 4]
        at = datetime.fromisoformat(NOW) + timedelta(minutes=n)
        add('timer_duration', i, [], [turn(text, 'create', Count=1, Event=dict(Key='a', Kind='reminder', Title='Таймер',
            Date=at.strftime('%Y-%m-%d'), Time=at.strftime('%H:%M'))), turn('Спасибо', 'help', route='fast_path')])
    defer = ['Подтверждение отложи, нужно подумать', 'Пока не подтверждаю: хочу ознакомиться',
             'Сначала подумаю, потом решу', 'Согласия ещё не даю: проверяю состав']
    for i in range(20):
        add('pending_defer', i, [task('a', f'Резервная копия {i}')], [turn('Убери все задачи', 'clarify', pending=True, Count=1),
            turn(defer[i % 4], 'clarify', route='fast_path', pending=True, DBCount=1),
            turn('отмена', 'cancel', route='fast_path', DBCount=1),
            turn('подтверждаю', 'help', route='fast_path', DBCount=1)])
    for i in range(20):
        title = f'Регулировка манипулятора {i}'
        add('canonicalization', i, [task('a', title)], [turn(f'Найди «{title}»', 'search', Count=1),
            turn('Пометь её выполненной', 'complete', Count=1, Changes=[dict(Key='a', Status='done')])])
    for i in range(20):
        title = f'Контроль излучателя {i}'
        add('negated_alternative', i, [task('a', title), event('b', title, date=DAY, time='13:20')], [
            turn(f'Поищи «{title}»', 'search', Count=2),
            turn('Вторую передвинь на послезавтра, не на сегодня', 'update', Count=1,
                 Changes=[dict(Key='b', Date='2027-06-11')])])
    assert len(rows) == 195
    return rows


def omissions():
    # Valid JSON payloads intentionally missing one critical constraint.
    base = dict(status='ok', dialogue_act='command', operation='create', kind='task', title=None, query=None,
                filters=None, target=None, schedule=None, changes=None, evidence=None, clarification=None, chat_response=None)
    def schedule(**kw):
        return dict(date=None, time=None, delay=None, duration_minutes=None, reminder_minutes=None, reminder_enabled=None, recurrence=None, **kw)
    def change(**kw):
        c = dict(title=None, date=None, time=None, delay=None, time_shift_minutes=None, priority=None, status=None, reminder_minutes=None, reminder_enabled=None)
        c.update(kw); return c
    def filters(**kw):
        f = dict(status=None, priority=None, period=None, time_from=None, time_before=None, exclude=[]); f.update(kw); return f
    def temporal(offset):
        return dict(type='relative_day', unit=None, offset=offset, value=None, weekday=None, direction=None, year=None, month=None, day=None)
    rows = []
    for i in range(5):
        title = f'Проверка гироскопа {i}'
        for cat in ['create_date', 'create_time', 'update_date', 'update_time', 'status', 'target', 'exclusion', 'shift']:
            p = dict(base); p['title'] = title
            source = f'Добавь задачу «{title}» на завтра'
            if cat == 'create_time':
                source = f'Создай задачу «{title}» на послезавтра в 13:48'
                p['schedule'] = schedule(); p['schedule']['date'] = temporal(2)
            if cat.startswith('update'):
                p.update(operation='update', title=None, target=dict(reference='query', ordinal=None, query=title), changes=change(title=title))
                source = f'Задачу «{title}» переименуй в «{title}» и передвинь ' + ('на послезавтра' if cat=='update_date' else 'на 16:38')
            if cat == 'status':
                p.update(operation='batch_delete', title=None, filters=filters(status='any'), evidence=[dict(field='batch.scope', text='все задачи')])
                source = 'Удали все задачи, только выполненные'
            if cat == 'target':
                p.update(operation='update', dialogue_act='correction', title=None, changes=change(time=dict(hour=15, minute=38)))
                source = 'Третью запись передвинь на 15:38'
            if cat == 'exclusion':
                p.update(operation='batch_delete', title=None, evidence=[dict(field='batch.scope', text='все задачи')])
                source = f'Удали все задачи, кроме «{title}»'
            if cat == 'shift':
                p.update(operation='update', title=None, target=dict(reference='query', ordinal=None, query=title), changes=change(title=title))
                source = f'Для «{title}» сохрани название «{title}», а начало сдвинь на {17+i} минут позже'
            rows.append(dict(id=f'omission-{cat}-{i}', category=cat, source=source, plan=p))
    assert len(rows) == 40
    return rows


if __name__ == '__main__':
    rows = build()
    write(ROOT/'testdata/semantic/plan_architecture_dev.jsonl', rows)
    for cat in sorted({r['Category'] for r in rows}):
        write(ROOT/f'testdata/semantic/plan_architecture_dev_{cat}.jsonl', [r for r in rows if r['Category']==cat])
    (ROOT/'testdata/semantic/plan_omission_adversarial.jsonl').write_text(''.join(json.dumps(r, ensure_ascii=False)+'\n' for r in omissions()), encoding='utf-8')
