"""Post-audit reference safety DEV and separate fresh focused audit.

Does not read any existing corpus or output. Original fresh v3 remains a failed
audit and is never rerun as independent evidence after the safety repair.
"""
import argparse
from plan_corpus import ROOT, NOW, event, turn, write


def build(split):
    fresh = split == 'fresh'
    names = (["Ариадной", "Богданом", "Венерой", "Глебом", "Данилой", "Елизаветой", "Жданом", "Златой", "Изольдой", "Корнеем"]
             if fresh else ["Лилией", "Макаром", "Нелли", "Осипом", "Павлиной"])
    vague = (["Сделай так, чтобы эта начиналась в 17:45.", "Вот ту запланируй на 19:15.",
              "А её передвинуть можно на 20:20?", "Поменяй ей начало на 16:05.",
              "Нужно это перенести на 21:30.", "С той же записью сделай перенос на 15:40.",
              "Про неё: пусть начинается в 18:55.", "Хочу перенести её на 22:10.",
              "На 14:50 передвинь эту.", "Пусть та начнётся в 13:35."]
             if fresh else ["Пусть эта запись начнётся в 13:20.", "Ту передвинь на 2 часа позже.",
                            "Эту сдвинь на 1 час.", "Она должна быть в 20:15.", "Мне её передвинь на 16:40."])
    temporal = (["Передвинь её на вторую неделю следующего месяца.", "Во второй половине дня её поставь.",
                 "Сделай её на первой неделе следующего месяца.", "Пусть она будет после первой недели ноября.",
                 "Второй половиной дня ограничь время для неё.", "Её передвинь к третьей неделе ноября.",
                 "Ту назначь на третью неделю следующего месяца.", "С ней давай в первой половине дня.",
                 "Поменяй её на четвёртую неделю будущего месяца.", "Её перенеси во вторую половину дня."]
                if fresh else ["Сдвинь её во второй половине дня.", "Ту перенеси на вторую неделю.",
                               "Ей выбери время в первой половине дня.", "Её запланируй на третью неделю.",
                               "Для неё нужна вторая половина дня."])
    rows=[]
    groups=[('ambiguous_reference',vague),('temporal_is_not_target',temporal)]
    for category,phrases in groups:
        for i,n in enumerate(names):
            rows.append(dict(ID=f"reference-{split}-{category}-{i:02d}",Category=category,Now=NOW,
                Seeds=[event('a',f"Проверка с {n}",time='07:10'),event('b',f"Проверка с {n}",time='11:50')],
                Turns=[turn(f"Отыщи проверки с {n}",'search',Count=2),turn(phrases[i],'clarify',DBCount=2)]))
    # Positive controls retain contextual ordinal selection through the LLM.
    for i,n in enumerate(names):
        for first in (True,False):
            phrase=("У первой измени начало на 16:35." if first else "Вторую передвинь на 19:25, пожалуйста.") if fresh else (
                "Первую перенеси на 15:15." if first else "Пожалуйста, вторую поставь на 17:55.")
            rows.append(dict(ID=f"reference-{split}-explicit-{i:02d}-{int(first)}",Category='explicit_reference',Now=NOW,
                Seeds=[event('a',f"Контроль с {n}",time='07:10'),event('b',f"Контроль с {n}",time='11:50')],
                Turns=[turn(f"Найди контроль с {n}",'search',Count=2),turn(phrase,'update',Count=1,
                    Changes=[dict(Key='a' if first else 'b',Time=('16:35' if first else '19:25') if fresh else ('15:15' if first else '17:55'))])]))
    return rows


def build_full():
    """Full fresh after safety repair; new wording/entities/time anchor."""
    from datetime import datetime, timedelta
    now='2026-11-09T09:40:00+03:00'
    names=['Майей','Никандром','Пелагеей','Ратмиром','Серафимой','Терентием','Ульяной','Федосеем','Христиной','Эдуардом']
    topics=['Переучёт','Прототипирование','Испытания','Модерация','Поставка','Реструктуризация','Картирование','Апробация','Маркировка','Диспетчеризация']
    rows=[]
    def e(key,title,kind='meeting',date='2026-11-18',time='08:20',**extra):return event(key,title,kind,date,time,**extra)
    def add(cat,i,seeds,turns):rows.append(dict(ID=f'plan-full-fresh-v5-{cat}-{i:02d}',Category=cat,Now=now,Seeds=seeds,Turns=turns))
    chat=['Помощь!','Привет!','Кто ты','Как тебя зовут','Что ты умеешь','Здравствуйте!','Благодарю!','Здравствуй!','Привет Гена!','Спасибо']
    for i,text in enumerate(chat):add('simple_chat',i,[],[turn(text,'help',route='fast_path'),turn('Благодарю','help',route='fast_path')])
    for i,topic in enumerate(topics):
        title=f'Рабочий блок «{topic}»'
        add('simple_pending_time',i,[],[
            turn(f'Добавь встречу на завтрашний день с названием «{title}». Время пока оставь для уточнения.','clarify',pending=True,Count=0),
            turn(f'в {10+i:02d}:05','create',route='fast_path',Count=1,Event=dict(Key='made',Title=title,Kind='meeting',Date='2026-11-10',Time=f'{10+i:02d}:05'))])
    for i,topic in enumerate(topics):
        title=f'Проверить готовность: {topic}'
        delay=42+i
        at=datetime.fromisoformat(now)+timedelta(minutes=delay)
        add('simple_pending_delay',i,[],[
            turn(f'Хочу напоминание «{title}», а момент напоминания назову следующим сообщением.','clarify',pending=True,Count=0),
            turn(f'через {delay} минут','create',route='fast_path',Count=1,Event=dict(Key='r',Title=title,Kind='reminder',Date='2026-11-09',Time=at.strftime('%H:%M')))])
    for i,topic in enumerate(topics):
        title=f'Проверить направление: {topic}'
        follow=(turn('Эту задачу заверши, работа выполнена.','complete',Count=1,Changes=[dict(Key='made',Status='done')]) if i%2 else
                turn(f'Поправка к названию: теперь назови эту задачу «Итог: {topic}».','update',Count=1,Changes=[dict(Key='made',Title=f'Итог: {topic}')]))
        add('normal_commands',i,[],[
            turn(f'Внеси задачу с точным названием «{title}», срок — послезавтра, без часов.','create',Count=1,Event=dict(Key='made',Title=title,Kind='task',Date='2026-11-11')),follow])
    periods=[
        'Мне нужны сессии с {n} из предыдущего квартала, найди их.',
        'С {n} сессии за прошлый квартал можно поискать?',
        'Отыщи сессии с {n}, ограничив кварталом до текущего.',
        'Поиск сессий с {n} нужен строго за предшествующий календарный квартал.',
        'Только прошлый квартал: покажи сессии с {n}.',
        'Из календаря выведи сессии с {n}, которые попали в предыдущий квартал.',
        'Сессии с {n} за прошлый календарный квартал найди, пожалуйста.',
        'Нужна подборка сессий с {n} из квартала перед нынешним.',
        'Найди в истории сессии с {n} за предыдущий квартал.',
        'Прошлым кварталом ограничь поиск сессий с {n}.']
    for i,n in enumerate(names):add('search_calendar',i,[e('old',f'Сессия с {n}',date='2026-08-12'),e('outside',f'Сессия с {n}',date='2026-10-13')],[
        turn(periods[i].format(n=n),'search',Count=1,Start='2026-07-01',End='2026-10-01'),turn('Спасибо','help',route='fast_path')])
    for i,topic in enumerate(topics):add('multiple_filters',i,[
        e('keep',topic,'task',time='16:10',Priority='high'),e('done','Другой пункт','task',time='17:10',Priority='high',Status='done'),
        e('low','Обычный пункт','task',time='17:10'),e('morning','Ранний пункт','task',time='08:10',Priority='high'),e('omit','Стажёр','task',time='17:10',Priority='high')],[
        turn('На следующую неделю покажи только открытые задачи высокого приоритета начиная с 15:00; всё про стажёра исключи.','list',Count=1,Start='2026-11-16',End='2026-11-23'),turn('Спасибо','help',route='fast_path')])
    ref=['Сделай тот же поиск только за предыдущий месяц.','По найденному: нужен прошлый календарный месяц.','Сохрани участника, ограничь период предыдущим месяцем.','Давай те же записи, но только в прошлом месяце.','Только месяц перед текущим из этого поиска.','Уточнение к поиску: ограничь его прошлым месяцем.','Можешь отобрать в этих результатах прошлый месяц?','Повтори тот запрос в пределах прошлого календарного месяца.','Сузь период этих сессий до предшествующего месяца.','Того же человека поищи лишь за прошлый месяц.']
    for i,n in enumerate(names):add('refinement',i,[e('old',f'Сессия с {n}',date='2026-10-14'),e('new',f'Сессия с {n}')],[
        turn(f'Найди сессии с {n}','search',Count=2,Query=n),turn(ref[i],'search',Count=1,Query=n,Start='2026-10-01',End='2026-11-01')])
    for i,n in enumerate(names):add('references',i,[e('a',f'Сессия с {n}',time='08:20'),e('b',f'Сессия с {n}',time='11:20')],[
        turn(f'Покажи результаты поиска сессий с {n}','search',Count=2),
        turn('Пожалуйста, вторую сессию передвинь на 18:45.','update',Count=1,Changes=[dict(Key='b',Time='18:45')])])
    for i,topic in enumerate(topics):
        minutes=21+7*i
        a=datetime(2026,11,18,8,20)+timedelta(minutes=minutes);b=datetime(2026,11,18,12,20)+timedelta(minutes=minutes)
        add('batch_shift',i,[e('a',topic,time='08:20'),e('b','Рабочее обсуждение',time='12:20')],[
            turn(f'У всех встреч будущей недели задержи начало на {minutes} минут, дни оставь прежними.','clarify',pending=True,Count=2),
            turn('да','update',route='fast_path',Count=2,Changes=[dict(Key='a',Time=a.strftime('%H:%M')),dict(Key='b',Time=b.strftime('%H:%M'))])])
    for i,n in enumerate(names):add('batch_delete',i,[e('a',f'Обсуждение с {n}'),e('b','Резерв',time='14:20')],[
        turn('Массово удали все встречи, запланированные на следующую неделю.','clarify',pending=True,Count=2),
        turn('подтверждаю','delete',route='fast_path',Count=0,DBCount=0,Absent=['a','b'])])
    for i,topic in enumerate(topics):add('batch_complete',i,[e('a',topic,'task'),e('b','Отметить результат','task')],[
        turn('Закрой как выполненные все открытые задачи следующей недели.','clarify',pending=True,Count=2),
        turn('да','complete',route='fast_path',Count=2,Changes=[dict(Key='a',Status='done'),dict(Key='b',Status='done')])])
    for i,n in enumerate(names):add('exclusions',i,[e('keep','Технический разбор',date='2026-11-17'),e('omit',f'Сессия с {n}',date='2026-11-19',time='13:20')],[
        turn(f'Для встреч следующей недели поменяй дату на послезавтра, но сессию с {n} оставь как есть.','clarify',pending=True,Count=1),
        turn('да','update',route='fast_path',Count=1,Changes=[dict(Key='keep',Date='2026-11-11')])])
    vague=['Ту назначь на 17:10.','Этой поменяй начало на 19:40.','А её можно на 20:15 передвинуть?','Эту сессию перенеси на 21:05.','Для неё новое время 15:55.','Перенеси её на 2 часа позже.','Она пусть начнётся в 16:25.','Мне надо её сдвинуть на 1 час.','Давай ту поставим на 18:05.','Вот эту сделай в 22:20.']
    for i,n in enumerate(names):add('ambiguity',i,[e('a',f'Сессия с {n}'),e('b',f'Сессия с {n}',time='13:20')],[
        turn(f'Сессии с {n} отыщи, пожалуйста.','search',Count=2),turn(vague[i],'clarify',DBCount=2)])
    caution=['Да, но разрешения на удаление пока не даю.','Так, сначала расскажи, что будет удалено, а потом решим.','Возможно да, но сейчас ничего не удаляй.','Я не подтверждаю удаление, только хочу обсудить его.','Слова «да, удали» здесь приведены как пример, не исполняй.','Давай ещё подумаем, разрешения пока нет.','Да, но это ещё не окончательное согласие на действие.','Нужен только список последствий, удалять не разрешаю.','Не исполняй: слово «подтверждаю» сейчас просто цитирую.','Может быть, но действие я пока не подтвердил.']
    for i,topic in enumerate(topics):add('destructive_safety',i,[e('keep',f'Защищённая задача: {topic}','task')],[
        turn(f'Удали задачу «Защищённая задача: {topic}».','clarify',pending=True,Count=0),turn(caution[i],'clarify',pending=True,DBCount=1)])
    for i,topic in enumerate(topics):
        minutes=70+4*i;at=datetime.fromisoformat(now)+timedelta(minutes=minutes);title=f'Перепроверить: {topic}'
        add('natural_reminder',i,[],[turn(f'Через час и {minutes-60} минут напомни действие «{title}».','create',Count=1,Event=dict(Key='r',Kind='reminder',Title=title,Date='2026-11-09',Time=at.strftime('%H:%M'))),turn('Благодарю','help',route='fast_path')])
    assert len(rows)==150 and sum(len(r['Turns']) for r in rows)==300
    return rows


if __name__=='__main__':
    parser=argparse.ArgumentParser()
    parser.add_argument('--split',choices=['dev','fresh','full'],required=True)
    args=parser.parse_args()
    path={'dev':'plan_reference_dev.jsonl','fresh':'plan_reference_heldout_v4.jsonl','full':'plan_heldout_v5.jsonl'}[args.split]
    write(ROOT/'testdata/semantic'/path,build_full() if args.split=='full' else build(args.split))
