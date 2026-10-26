"""Independent policy/context DEV, authored from error classes, never holdouts.

Gold checks actual database writes, untouched records and preserved fields.
"""
from datetime import datetime, timedelta
from plan_corpus import ROOT, event, turn, write

NOW = '2027-02-10T11:25:00+03:00'


def task(key, title, date='2027-02-12', time='09:10', **kw):
    return event(key, title, 'task', date, time, **kw)


def evidence_dev():
    rows = []
    def add(cat, i, seeds, turns):
        rows.append(dict(ID=f'policy-dev-{cat}-{i:02}', Category=cat, Now=NOW, Seeds=seeds, Turns=turns))
    timers = [('Запусти таймер на 10 минут',10),('Поставь таймер минут на пять',5),
              ('Засеки 13 минут',13),('Таймер на полчаса',30),
              ('Отсчитай семь минут таймером',7),('Включи таймер на 21 минуту',21),
              ('Засеки четверть часа',15),('Таймер на сорок минут, пожалуйста',40),
              ('Пусть таймер отсчитает 17 минут',17),('Засеки один час',60),
              ('Установи таймер на 8 минут',8),('Хочу таймер на 12 минут',12)]
    for i,(text,n) in enumerate(timers):
        at=datetime.fromisoformat(NOW)+timedelta(minutes=n)
        add('timer',i,[],[turn(text,'create',Count=1,Event=dict(Key='a',Kind='reminder',Title='Таймер',Date=at.strftime('%Y-%m-%d'),Time=at.strftime('%H:%M'))),turn('Спасибо','help',route='fast_path')])
    for i in range(8):
        seeds=[task('a',f'Упаковать посылку {i}'),task('b',f'Сдать ключи {i}',Status='done')]
        delete=i%2==0
        text='Удали все задачи' if delete else 'Заверши все задачи'
        # Batch completion previews/returns only records whose state changes;
        # an already-done task remains in DB and is independently checked.
        result=turn('да','delete' if delete else 'complete',route='fast_path',Count=0 if delete else 1,DBCount=0 if delete else 2,
                    **(dict(Absent=['a','b']) if delete else dict(Changes=[dict(Key='a',Status='done'),dict(Key='b',Status='done')])))
        add('neutral_any',i,seeds,[turn(text,'clarify',pending=True,Count=2 if delete else 1),result])
    for i in range(8):
        seeds=[event('a',f'Семинар {i}',date='2027-02-11',time='08:10'),event('b',f'Приёмка {i}',date='2027-02-12',time='12:30')]
        if i%2:
            add('neutral_read',i,seeds,[turn('Покажи все встречи','list',Count=2),turn('Спасибо','help',route='fast_path')])
        else:
            add('neutral_shift',i,seeds,[turn('Перенеси все встречи завтра на час позже','clarify',pending=True,Count=1),turn('да','update',route='fast_path',Count=1,Changes=[dict(Key='a',Time='09:10')])])
    for i in range(8):
        done=i%2==0
        seeds=[task('a',f'Сдать отчёт {i}',Status='done'),task('b',f'Купить кабель {i}')]
        text='Удали только выполненные задачи' if done else 'Заверши только открытые задачи'
        add('explicit_status',i,seeds,[turn(text,'clarify',pending=True,Count=1),
            turn('подтверждаю','delete' if done else 'complete',route='fast_path',Count=0 if done else 1,DBCount=1 if done else 2,
                 **(dict(Absent=['a']) if done else dict(Changes=[dict(Key='b',Status='done')])) )])
    for i in range(8):
        title=f'Сортировка образцов {i}'
        add('required_change',i,[task('a',title)],[turn(f'Задачу «{title}» перенеси на послезавтра в 17:25','update',Count=1,Changes=[dict(Key='a',Date='2027-02-12',Time='17:25')]),turn('Спасибо','help',route='fast_path')])
    for i in range(8):
        seeds=[event('a',f'Инструктаж {i}',date='2027-02-11',time='09:10'),event('b',f'Сверка счетов {i}',date='2027-02-11',time='13:40')]
        add('exclusion',i,seeds,[turn(f'Все встречи завтра сдвинь на два часа позже, кроме «Сверка счетов {i}»','clarify',pending=True,Count=1),turn('да','update',route='fast_path',Count=1,Changes=[dict(Key='a',Time='11:10')])])
    for i in range(8):
        title=f'Согласовать маршрут {i}'
        add('optional_create',i,[],[turn(f'Добавь задачу «{title}»','create',Count=1,Event=dict(Key='a',Kind='task',Title=title)),turn('Спасибо','help',route='fast_path')])
    assert len(rows)==60
    return rows


def reference_dev():
    rows=[]
    topics=['проверка','осмотр','ревизия','испытание','обследование','замер','аудит','сверка','приёмка','инвентаризация']
    plural=['проверки','осмотры','ревизии','испытания','обследования','замеры','аудиты','сверки','приёмки','инвентаризации']
    for i,(topic,pl) in enumerate(zip(topics,plural)):
        title=topic.capitalize()+' контура'
        seeds=[task('a',title,time='08:10'),event('b',title,date='2027-02-12',time='12:10')]
        def add(cat,turns,seed=seeds):
            rows.append(dict(ID=f'context-dev-{cat}-{i:02}',Category=cat,Now=NOW,Seeds=seed,Turns=turns))
        add('topic_kind',[turn(f'Найди {pl}','search',Count=2),turn('А потом только встречи','search',Count=1),turn('Покажи её','search',Count=1)])
        add('explicit_kind',[turn(f'Найди задачи про {topic[:-1]+"у" if topic.endswith("а") else topic}','search',Count=1),turn('Её перенеси на завтра','update',Count=1,Changes=[dict(Key='a',Date='2027-02-11')])])
        add('ordinal_read',[turn(f'Покажи {pl}','search',Count=2),turn('А вторую','search',Count=1),turn('Её перенеси на 18:35','update',Count=1,Changes=[dict(Key='b',Time='18:35')])])
        add('safe_clarification',[turn(f'Найди {pl}','search',Count=2),turn('Ту предыдущую','clarify',DBCount=2),turn('Вторую перенеси на 19:05','update',Count=1,Changes=[dict(Key='b',Time='19:05')])])
        add('other_ambiguous',[turn(f'Найди {pl}','search',Count=2),turn('Нет, другую','clarify',DBCount=2),turn('Первую перенеси на 16:15','update',Count=1,Changes=[dict(Key='a',Time='16:15')])])
    assert len(rows)==50
    return rows


def bare_reference_dev():
    rows=[]
    for count in [1,2]:
        for i,pronoun in enumerate(['её','его','ту']):
            title=['Просушка купола','Открытый семинар','Наладка антенны'][i]
            seeds=[task('a',title,time='08:10')]
            if count==2:seeds.append(task('b',title,time='12:10'))
            rows.append(dict(ID=f'bare-reference-{count}-{i}',Category='bare_reference',Now=NOW,Seeds=seeds,
                             Turns=[turn(f'Поищи «{title}»','search',Count=count),
                                    turn(pronoun,'search' if count==1 else 'clarify',**(dict(Count=1) if count==1 else dict(DBCount=2)))]))
    return rows


if __name__=='__main__':
    write(ROOT/'testdata/semantic/plan_policy_dev_corrected.jsonl',evidence_dev())
    if not (ROOT/'testdata/semantic/plan_context_dev.jsonl').exists():
        write(ROOT/'testdata/semantic/plan_context_dev.jsonl',reference_dev())
