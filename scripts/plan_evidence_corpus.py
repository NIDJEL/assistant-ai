"""New evidence DEV; does not read any previous holdout or model output."""
from datetime import datetime, timedelta
from plan_corpus import ROOT, event, turn, write

NOW = '2026-12-09T12:10:00+03:00'  # Wednesday


def build():
    rows = []
    def add(category, i, seeds, turns):
        rows.append(dict(ID=f'evidence-dev-{category}-{i:02}', Category=category,
                         Now=NOW, Seeds=seeds, Turns=turns))
    def task(key, title, date='2026-12-12', time='09:00'):
        return event(key, title, 'task', date, time)
    for i in range(15):
        title=f'Проверка насоса {i+1}'
        lex,offset=[('сегодня',0),('завтра',1),('послезавтра',2),('вчера',-1),('позавчера',-2)][i%5]
        dest=(datetime(2026,12,9)+timedelta(days=offset)).strftime('%Y-%m-%d')
        add('lexical_day',i,[task('a',title)],[
            turn(f'У задачи «{title}» поменяй дату на {lex}.','update',Count=1,Changes=[dict(Key='a',Date=dest)]),
            turn('Спасибо','help',route='fast_path')])
    for i in range(15):
        title=f'Осмотр вентиля {i+1}'
        n=12+i;unit=['минут','часов','дней'][i%3]
        at=datetime.fromisoformat(NOW)+timedelta(**{['minutes','hours','days'][i%3]:n})
        add('explicit_delay',i,[task('a',title)],[
            turn(f'Перенеси задачу «{title}» через {n} {unit}.','update',Count=1,Changes=[dict(Key='a',Date=at.strftime('%Y-%m-%d'),Time=at.strftime('%H:%M'))]),
            turn('Благодарю','help',route='fast_path')])
    weekdays=[('понедельник','2026-12-14'),('вторник','2026-12-15'),('среду','2026-12-09'),('четверг','2026-12-10'),('пятницу','2026-12-11'),('субботу','2026-12-12'),('воскресенье','2026-12-13')]
    for i in range(15):
        title=f'Замер давления {i+1}';day,dest=weekdays[i%7]
        add('weekday',i,[task('a',title)],[
            turn(f'Задачу «{title}» перенеси на {day}.','update',Count=1,Changes=[dict(Key='a',Date=dest)]),
            turn('Спасибо','help',route='fast_path')])
    for i in range(15):
        h=15+i%7; title=f'Обход участка {i+1}'
        add('reference_dev',i,[task('a',title,time='08:00'),task('b',title,time='11:00')],[
            turn(f'Найди задачи «{title}».','search',Count=2),
            turn(f'Вторую перенеси на {h}:20.','update',Count=1,Changes=[dict(Key='b',Time=f'{h}:20')])])
    for i in range(15):
        title=f'Дежурное обсуждение {i+1}'
        seeds=[event('a',title,date='2026-12-11',time='08:00'),event('b','Другая встреча',date='2026-12-10',time='12:00')]
        form=i%3
        text,change=[('Перенеси все встречи послезавтра на день позже.',dict(Date='2026-12-12',Time='08:00')),
                     ('Все встречи послезавтра перенеси на 18:00.',dict(Date='2026-12-11',Time='18:00')),
                     ('Послезавтра все встречи сдвинь на час.',dict(Date='2026-12-11',Time='09:00'))][form]
        add('batch_source_role',i,seeds,[turn(text,'clarify',pending=True,Count=1),turn('да','update',route='fast_path',Count=1,Changes=[dict(Key='a',**change)])])
    names=['Иваном','Лидией','Семёном','Ульяной','Филиппом','Еленой','Вадимом','Тамарой','Яковом','Ларисой','Юрием','Ниной','Павлом','Валерией','Григорием']
    for i,name in enumerate(names):
        seeds=[event('a','Обсуждение спецификации',date='2026-12-11',time='08:00'),event('b',f'Встреча с {name}',date='2026-12-11',time='12:00')]
        add('exclusions',i,seeds,[
            turn(f'Все встречи послезавтра перенеси на завтра, кроме встречи с {name}.','clarify',pending=True,Count=1),
            turn('подтверждаю','update',route='fast_path',Count=1,Changes=[dict(Key='a',Date='2026-12-10')])])
    for i in range(15):
        title=f'Проверка тяги {i+1}'
        amount,minutes=[('два часа',120),('час',60),('30 минут',30),('день',1440),('три часа',180)][i%5]
        direction='раньше' if i%2 else 'позже';delta=minutes*(-1 if i%2 else 1)
        at=datetime(2026,12,12,9)+timedelta(minutes=delta)
        add('time_shift',i,[task('a',title)],[
            turn(f'Задачу «{title}» сдвинь на {amount} {direction}.','update',Count=1,Changes=[dict(Key='a',Date=at.strftime('%Y-%m-%d'),Time=at.strftime('%H:%M'))]),
            turn('Спасибо','help',route='fast_path')])
    vague=['после обеда','вечером','ближе к ночи','около вечера','в удобное время']
    for i in range(15):
        title=f'Поверка датчика {i+1}'
        add('ambiguous_time',i,[task('a',title)],[
            turn(f'Задачу «{title}» поставь {vague[i%5]}, точного часа пока нет. Уточни его перед изменением.','clarify',DBCount=1),
            turn('Спасибо','help',route='fast_path')])
    assert len(rows)==120 and sum(len(x['Turns']) for x in rows)==240
    return rows


if __name__=='__main__':
    write(ROOT/'testdata/semantic/plan_evidence_dev.jsonl',build())
