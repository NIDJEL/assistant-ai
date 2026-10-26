"""Independent v8, authored after architecture DEV; no audit/DEV reads.

All turns use the real semantic provider. Omission-pressure requests are scored
against the full intended write; a safe refusal is an accuracy failure, never
a reason to relax gold. The separate 40-plan corpus tests deliberate omissions.
"""
from datetime import datetime, timedelta
import random
from plan_corpus import ROOT, event, turn, write

NOW = '2027-09-14T09:18:00+03:00'
TOM = '2027-09-15'
AFTER = '2027-09-16'


def build():
    rows = []
    objects = ['Волновод', 'Пьезоэлемент', 'Диффузор', 'Рефлектор', 'Термопара',
               'Маховик', 'Интерферометр', 'Дозатор', 'Экструдер', 'Резонатор']
    names = ['Эвелиной', 'Яковом', 'Робертом', 'Нонной', 'Трофимом',
             'Ульяной', 'Феликсом', 'Цезарем', 'Эммой', 'Юлианом']
    def task(key, title, date=TOM, time='08:15', **kw):
        return event(key, title, 'task', date, time, **kw)
    def add(cat, i, seeds, turns):
        rows.append(dict(ID=f'fresh-v8-{cat}-{i:02}', Category=cat, Now=NOW, Seeds=seeds, Turns=turns))
    def thanks(turns, i):
        return turns + ([turn('Спасибо', 'help', route='fast_path')] if i % 3 == 0 and len(turns) < 5 else [])
    for i, (obj, name) in enumerate(zip(objects, names)):
        title = f'{obj}: приёмочный протокол'
        renamed = f'{obj}: окончательный протокол'
        add('simple_create', i, [], thanks([
            turn(f'Запиши в задачи «{title}», без срока', 'create', Count=1, Event=dict(Key='a', Kind='task', Title=title)),
            turn(f'Ей дай название «{renamed}»', 'update', Count=1, Changes=[dict(Key='a', Title=renamed)])], i))

        title = f'Технический разговор с {name}'
        h = 10+i%7
        add('scheduled_create', i, [], [
            turn(f'На завтра в {h}:26 запланируй встречу «{title}»', 'create', Count=1,
                 Event=dict(Key='a', Kind='meeting', Title=title, Date=TOM, Time=f'{h}:26')),
            turn('Поправка ко времени: 18:36', 'update', Count=1, Changes=[dict(Key='a', Time='18:36')]),
            turn('Выведи все встречи на завтра', 'list', Count=1)])

        title = f'{obj}: контроль термостата'
        n = 24+i
        at = datetime.fromisoformat(NOW)+timedelta(minutes=n)
        later = datetime.fromisoformat(NOW)+timedelta(minutes=n+9)
        add('correction_chain', i, [], thanks([
            turn(f'Через {n} минут напомни «{title}»', 'create', Count=1,
                 Event=dict(Key='a', Kind='reminder', Title=title, Date=at.strftime('%Y-%m-%d'), Time=at.strftime('%H:%M'))),
            turn(f'Пересмотрел решение: лучше через {n+9} минут', 'update', Count=1,
                 Changes=[dict(Key='a', Date=later.strftime('%Y-%m-%d'), Time=later.strftime('%H:%M'))]),
            turn(f'Переименуй его в «{obj}: повторный контроль»', 'update', Count=1,
                 Changes=[dict(Key='a', Title=f'{obj}: повторный контроль')]),
            turn('Исправление даты: послезавтра', 'update', Count=1, Changes=[dict(Key='a', Date=AFTER)])], i))

        title = f'{obj}: диагностика вибраций'
        seeds = [task('a', title), event('b', title, date=TOM, time='12:15')]
        add('reference_chain', i, seeds, thanks([
            turn(f'Нужен поиск по теме «{title}»', 'search', Count=2),
            turn('У второй записи назначь время 19:32', 'update', Count=1, Changes=[dict(Key='b', Time='19:32')]),
            turn('Покажи её', 'search', Count=1)], i))

        seeds = [event('a', f'Беседа с {name}', date='2027-08-11', time='08:15'),
                 event('b', f'Беседа с {name}', date='2027-08-18', time='14:15'),
                 event('c', f'Беседа с {name}', date=TOM, time='12:15')]
        add('historical_search', i, seeds, thanks([
            turn(f'Отыщи встречи с {name} за предыдущий календарный месяц', 'search', Count=2,
                 Query=name, Start='2027-08-01', End='2027-09-01'),
            turn('В найденном нужны начинающиеся после 13:00', 'search', Count=1),
            turn('Покажи найденную', 'search', Count=1)], i))

        title = f'{obj}: акустическая проверка'
        seeds = [task('a', title, Status='done'), event('b', title, date=TOM, time='12:15', Status='done'),
                 event('c', title, date=AFTER, time='15:15'), event('d', title, date='2027-11-03', Status='done')]
        add('refinement_chain', i, seeds, thanks([
            turn(f'Проведи поиск «{title}» только в этом месяце', 'search', Count=3),
            turn('Из результата оставь завершённые', 'search', Count=2),
            turn('Далее в этой выборке нужны только встречи', 'search', Count=1),
            turn('И из них лишь до 16:00', 'search', Count=1, Query=title, Start='2027-09-01', End='2027-10-01')], i))

        n = 16+i*2
        at = datetime.fromisoformat(NOW)+timedelta(minutes=n)
        timer_text = [f'Пусть таймер работает {n} минут', f'Таймер продолжительностью {n} минут включи',
                      f'Надо отсчитать {n} минут таймером', f'Включи обратный отсчёт: таймер на {n} минут'][i % 4]
        turns = [turn(timer_text, 'create', Count=1, Event=dict(Key='a', Kind='reminder', Title='Таймер',
                  Date=at.strftime('%Y-%m-%d'), Time=at.strftime('%H:%M'))),
                 turn(f'Назови его «{obj}: выдержка»', 'update', Count=1, Changes=[dict(Key='a', Title=f'{obj}: выдержка')])]
        if i % 2 == 0:
            turns += [turn(f'Создай встречу «{obj}: разбор» на завтра в 15:12 длительностью 30 минут', 'create', Count=1,
                           Event=dict(Key='b', Kind='meeting', Title=f'{obj}: разбор', Date=TOM, Time='15:12'))]
        add('timer_semantics', i, [], thanks(turns, i))

        seeds = [event('a', f'{obj}: утренний разбор', date=TOM, time='08:15'),
                 event('b', f'{obj}: дневной разбор', date=TOM, time='12:15'),
                 event('keep', 'Другой день', date=AFTER, time='14:15')]
        minutes = 17+i
        add('batch_shift', i, seeds, thanks([
            turn(f'Все встречи завтра сдвинь на {minutes} минут позже', 'clarify', pending=True, Count=2),
            turn('подтверждаю', 'update', route='fast_path', Count=2,
                 Changes=[dict(Key='a', Time=f'08:{15+minutes:02}'), dict(Key='b', Time=f'12:{15+minutes:02}')])], i))

        keep = f'Брифинг с {name}'
        seeds = [event('a', f'{obj}: рабочий разбор', date=TOM, time='08:15'),
                 event('keep', keep, date=TOM, time='12:15')]
        add('batch_exclusion', i, seeds, [
            turn(f'Встречи на завтра перенеси на послезавтра, исключая «{keep}»', 'clarify', pending=True, Count=1),
            turn('да', 'update', route='fast_path', Count=1, Changes=[dict(Key='a', Date=AFTER)]),
            turn(f'Найди «{keep}»', 'search', Count=1)])

        seeds = [task('a', f'{obj}: первый этап', '2027-09-14'), task('b', f'{obj}: второй этап', '2027-09-14'),
                 task('keep', f'{obj}: готовый этап', '2027-09-14', Status='done')]
        add('batch_complete', i, seeds, thanks([
            turn('Все открытые задачи на сегодня отметь завершёнными', 'clarify', pending=True, Count=2),
            turn('подтверждаю', 'complete', route='fast_path', Count=2,
                 Changes=[dict(Key='a', Status='done'), dict(Key='b', Status='done')])], i))

        keep = f'{obj}: гарантийный журнал'
        seeds = [task('a', f'{obj}: устаревший лист', '2027-09-20'), task('b', f'{obj}: устаревшая схема', '2027-09-21'),
                 task('keep', keep, '2027-09-22')]
        add('batch_delete', i, seeds, [
            turn(f'Удали задачи следующей недели, кроме «{keep}»', 'clarify', pending=True, Count=2),
            turn('удаляй', 'delete', route='fast_path', Count=0, DBCount=1, Absent=['a', 'b']),
            turn('да', 'help', route='fast_path', DBCount=1)])

        seeds = [task('a', f'{obj}: черновик'), task('b', f'{obj}: копия')]
        deferred = ['Согласия пока не даю: сверяю названия', 'Пока не подтверждаю, ещё изучаю состав',
                    'Сначала посмотрю, решение будет позже', 'Подтверждение отложи, хочу проверить список'][i % 4]
        decision = turn('отмена', 'cancel', route='fast_path', DBCount=2) if i % 2 == 0 else turn('удаляй', 'delete', route='fast_path', Count=0, DBCount=0, Absent=['a', 'b'])
        add('deferred_confirmation', i, seeds, [
            turn('Все задачи удали', 'clarify', pending=True, Count=2),
            turn(deferred, 'clarify', route='fast_path', pending=True, DBCount=2), decision,
            turn('подтверждаю', 'help', route='fast_path', DBCount=2 if i % 2 == 0 else 0)])

        title = f'{obj}: балансировка'
        seeds = [task('a', title), task('b', title, time='11:15')]
        uncertain = ['Её бы изменить, но не знаю какую из этих двух', 'Выбрать пока не могу, нужны детали',
                     'Имею в виду одну из тех, что обсуждали до этого', 'Одну из них, точный номер ещё не решил'][i % 4]
        add('ambiguous_reference', i, seeds, thanks([
            turn(f'Отыщи записи «{title}»', 'search', Count=2), turn(uncertain, 'clarify', DBCount=2),
            turn('Первую запись поставь на 17:28', 'update', Count=1, Changes=[dict(Key='a', Time='17:28')])], i))

        chat = ['Объясни, чем можешь помочь с планированием дня', 'Как попросить тебя изменить встречу?',
                'Расскажи, какие бывают записи в твоём календаре', 'Помоги разобраться в своих возможностях',
                'Что ты можешь сделать с перечнем дел?', 'Дай пару примеров запросов о напоминаниях',
                'Расскажи, как найти старую встречу', 'Можно узнать, как отменить ожидающее действие?',
                'Какие сведения нужны для новой встречи?', 'Объясни, как выбрать одну запись из найденных'][i]
        add('simple_chat', i, [task('keep', f'{obj}: неизменяемая запись')], [turn(chat, 'help', DBCount=1), turn('Спасибо', 'help', route='fast_path', DBCount=1)])

        title = f'{obj}: оптический контроль'
        seeds = [task('a', title), task('b', title, time='11:15'), task('c', title, time='14:15')]
        selected, other = ('завтра', 'послезавтра') if i % 2 else ('послезавтра', 'завтра')
        selected_date = TOM if i % 2 else AFTER
        text = f'Первую запись передвинь на {selected}; не на {other}'
        add('negated_alternative', i, seeds, thanks([
            turn(f'Найди «{title}»', 'search', Count=3),
            turn(text, 'update', Count=1, Changes=[dict(Key='a', Date=selected_date)])], i))

        title = f'{obj}: опись инструментов'
        day = TOM if i % 2 else AFTER
        day_word = 'завтра' if i % 2 else 'послезавтра'
        clock = f'{7+i:02}:34'
        text = [f'Срок задачи «{title}» — {day_word}, {clock}. Заведи её',
                f'Создай задачу «{title}», обязательно на {day_word} в {clock}'][i % 2]
        add('omission_schedule_pressure', i, [], thanks([
            turn(text, 'create', Count=1, Event=dict(Key='a', Kind='task', Title=title, Date=day, Time=clock)),
            turn('Исправление: всё же на послезавтра', 'update', Count=1, Changes=[dict(Key='a', Date=AFTER)])], i))

        keep = f'{obj}: сохранить настройку'
        seeds = [task('a', f'{obj}: готовая операция', Status='done'), task('open', f'{obj}: новая операция'), task('keep', keep, Status='done')]
        n = 21+i
        add('omission_filter_pressure', i, seeds, [
            turn(f'Выполненные задачи сдвинь на {n} минут позже, кроме «{keep}»', 'clarify', pending=True, Count=1),
            turn('да', 'update', route='fast_path', Count=1, Changes=[dict(Key='a', Time=f'08:{15+n:02}')]),
            turn('да', 'help', route='fast_path', DBCount=3)])

        title = f'{obj}: очная приёмка'
        h = 9+i%6
        add('slot_then_correction', i, [], thanks([
            turn(f'Запланируй встречу «{title}»', 'clarify', pending=True, Count=0),
            turn('завтра', 'clarify', route='fast_path', pending=True, Count=0),
            turn(f'в {h}:54', 'create', route='fast_path', Count=1, Event=dict(Key='a', Kind='meeting', Title=title, Date=TOM, Time=f'{h:02}:54')),
            turn('Пересмотр времени: 16:28', 'update', Count=1, Changes=[dict(Key='a', Time='16:28')])], i))
    assert len(rows) == 180
    assert all(2 <= len(r['Turns']) <= 5 for r in rows)
    assert {len(r['Turns']) for r in rows} == {2, 3, 4, 5}
    random.Random(804173).shuffle(rows)
    return rows


if __name__ == '__main__':
    write(ROOT/'testdata/semantic/plan_heldout_v8.jsonl', build())
