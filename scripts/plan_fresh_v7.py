"""Fresh full holdout, authored only after policy/reference DEV completion.

No previous holdout, model response, cache or evaluation report is imported or
read. Helpers only serialize independently authored scenarios. No inference.
"""
from datetime import datetime, timedelta
from plan_corpus import ROOT, event, turn, write

NOW='2027-04-13T14:07:00+03:00'
BASE=datetime.fromisoformat(NOW)


def build():
    rows=[]
    def date(offset):return (BASE+timedelta(days=offset)).strftime('%Y-%m-%d')
    def task(key,title,day=2,clock='08:25',**kw):
        return event(key,title,'task',date(day),clock,**kw)
    def meeting(key,title,day=2,clock='08:25',**kw):
        return event(key,title,'meeting',date(day),clock,**kw)
    def add(cat,i,seeds,turns):
        rows.append(dict(ID=f'fresh-v7-{cat}-{i:02}',Category=cat,Now=NOW,Seeds=seeds,Turns=turns))
    subjects=['Откалибровать окуляр','Собрать штатив','Обновить эфемериды','Очистить отражатель',
              'Упаковать фотопластинки','Зарегистрировать метеорит','Настроить спектрограф',
              'Разметить небосвод','Подготовить фотометр','Заменить привод купола']
    topics=['Коллимация','Поляризация','Астрометрия','Спектроскопия','Фотометрия',
            'Юстировка','Экспозиция','Фокусировка','Навигация','Телеметрия']
    create_forms=['В список задач добавь «{t}».','Нужна задача с названием «{t}».',
                  'Запиши мне задачу: «{t}».','Заведи новую задачу «{t}», без срока.',
                  'Добавь в мои задачи пункт «{t}».','Создай задачу с текстом «{t}».',
                  'Поставь задачу «{t}», дату пока не назначай.','Сохрани задачу «{t}».',
                  'Пусть в задачах будет «{t}».','Запланируй задачу «{t}» без даты.']
    for i,title in enumerate(subjects):
        add('simple_create',i,[],[turn(create_forms[i].format(t=title),'create',Count=1,Event=dict(Key='a',Kind='task',Title=title)),turn('Благодарю','help',route='fast_path')])
    chats=['Объясни, как с твоей помощью вести список дел.','Какие команды для календаря ты понимаешь?',
           'Расскажи, чем можешь помочь с повседневными делами.','Как попросить тебя напомнить о событии?',
           'Как обратиться к тебе по имени?','Что можно делать с уже созданной задачей?',
           'Подскажи пример команды поиска по моим записям.','Как пользоваться подтверждением удаления?',
           'Можно ли попросить тебя завести встречу?','Расскажи про свои возможности помощника.']
    for i,text in enumerate(chats):
        add('simple_chat',i,[],[turn(text,'help'),turn('Спасибо','help',route='fast_path')])
    timers=[('Отмерь таймером девять минут',9),('Нужен отсчёт на 16 минут, включи таймер',16),
            ('Пожалуйста, засеки 24 минуты',24),('Установи отсчёт таймера на четверть часа',15),
            ('Давай таймер: 32 минуты',32),('На 45 минут включи мне таймер',45),
            ('Засечь нужно шесть минут',6),('Таймер пусть отработает 19 минут',19),
            ('Сделай таймер длительностью полчаса',30),('Включи обычный таймер на один час',60)]
    for i,(text,n) in enumerate(timers):
        at=BASE+timedelta(minutes=n);title=f'Выдержка {i+1}'
        add('timers',i,[],[turn(text,'create',Count=1,Event=dict(Key='a',Kind='reminder',Title='Таймер',Date=at.strftime('%Y-%m-%d'),Time=at.strftime('%H:%M'))),
                          turn(f'Переименуй его в «{title}».','update',Count=1,Changes=[dict(Key='a',Title=title)])])
    for i,title in enumerate(subjects):
        n=26+3*i;at=BASE+timedelta(minutes=n);new=BASE+timedelta(minutes=41+2*i,seconds=1)
        add('reminder_correction',i,[],[turn(f'Через {n} минут напомни: «{title}».','create',Count=1,Event=dict(Key='a',Kind='reminder',Title=title,Date=at.strftime('%Y-%m-%d'),Time=at.strftime('%H:%M'))),
            turn(f'Поправка: лучше через {41+2*i} минут.','update',Count=1,Changes=[dict(Key='a',Date=new.strftime('%Y-%m-%d'),Time=new.strftime('%H:%M'))])])
    for i,topic in enumerate(topics):
        title=topic+' на полигоне';hour=16+i%5
        add('meeting_slots',i,[],[turn(f'Создай встречу «{title}» на завтрашний день.','clarify',pending=True,Count=0,DBCount=0),
            turn(f'В {hour}:35','create',route='fast_path',Count=1,Event=dict(Key='a',Kind='meeting',Title=title,Date=date(1),Time=f'{hour}:35')),
            turn(f'Нет, лучше в {hour}:50.','update',Count=1,Changes=[dict(Key='a',Time=f'{hour}:50')])])
    for i,topic in enumerate(topics):
        title=topic+' прибора'
        seeds=[event('a',title,'meeting','2027-03-18','15:30'),event('b',title,'task','2027-03-19','09:20'),meeting('keep',title,day=1,clock='16:10')]
        add('contextual_search',i,seeds,[turn(f'Отыщи записи по теме «{topic}».','search',Count=3),
            turn('Оставь только прошлый календарный месяц.','search',Count=2,Start='2027-03-01',End='2027-04-01'),
            turn('А из этого — только встречи.','search',Count=1,Start='2027-03-01',End='2027-04-01')])
    for i,topic in enumerate(topics):
        title=topic+' зеркала';hour=17+i%4
        seeds=[meeting('a',title,clock='08:15'),meeting('b',title,clock='12:45')]
        add('reference_ambiguity',i,seeds,[turn(f'Найди встречи «{title}».','search',Count=2),
            turn('Ту, которая раньше обсуждалась.','clarify',DBCount=2),
            turn(f'Тогда вторую поставь на {hour}:25.','update',Count=1,Changes=[dict(Key='b',Time=f'{hour}:25')])])
    selections=['Открой вторую из результатов.','Вторую покажи отдельно.','Мне нужна вторая запись.',
                'А теперь покажи вторую.','Выведи вторую из списка.','Можно вторую запись?',
                'Выбери для просмотра вторую.','Вторую, пожалуйста.','Хочу посмотреть вторую запись.','Покажи отдельно вторую.']
    for i,topic in enumerate(topics):
        title=topic+' сектора'
        seeds=[task('a',title,clock='07:20'),task('b',title,clock='11:40'),task('keep',title,clock='18:10')]
        add('read_selection',i,seeds,[turn(f'Разыщи задачи «{title}».','search',Count=3),
            turn(selections[i],'search',Count=1,Event=dict(Kind='task',Title=title,Time='11:40')),
            turn('Теперь отметь её выполненной.','complete',Count=1,Changes=[dict(Key='b',Status='done')])])
    for i,topic in enumerate(topics):
        lex,offset=[('сегодня',0),('завтра',1),('послезавтра',2)][i%3]
        title=topic+' диапазона';hour=17+i%4
        seeds=[meeting('a',title,offset,'08:10'),meeting('b','Другая сессия',offset+1,'12:20'),meeting('keep','Защищённая сессия',offset+2,'08:10')]
        add('batch_source_clock',i,seeds,[turn(f'У всех встреч {lex} назначь начало {hour}:10.','clarify',pending=True,Count=1),
            turn('подтверждаю','update',route='fast_path',Count=1,Changes=[dict(Key='a',Time=f'{hour}:10')])])
    for i,topic in enumerate(topics):
        # An exclusion in the same source day and a second out-of-period record
        # independently check scope preservation during a relative shift.
        lex,offset=[('завтра',1),('послезавтра',2)][i%2]
        n=25+5*i;phrase=f'на {n} минут позже'
        if i==3:n=30;phrase='на полчаса позже'
        at=datetime.fromisoformat(date(offset)+'T08:10:00')+timedelta(minutes=n)
        seeds=[meeting('a',topic+' передатчика',offset,'08:10'),meeting('keep','Дежурный канал',offset,'16:20'),meeting('other','Резервный канал',offset+1,'08:10')]
        add('batch_shift_exclusion',i,seeds,[turn(f'Все встречи {lex} передвинь {phrase}, за исключением «Дежурный канал».','clarify',pending=True,Count=1),
            turn('да','update',route='fast_path',Count=1,Changes=[dict(Key='a',Date=at.strftime('%Y-%m-%d'),Time=at.strftime('%H:%M'))])])
    for i,title in enumerate(subjects):
        seeds=[task('a',title,Status='done'),task('open','Ещё в работе',Status='open'),task('keep','Архив наблюдений',Status='done')]
        add('batch_delete_status',i,seeds,[turn('Удали все завершённые задачи, кроме «Архив наблюдений».','clarify',pending=True,Count=1),
            turn('подтверждаю','delete',route='fast_path',Count=0,DBCount=2,Absent=['a'])])
    for i,title in enumerate(subjects):
        seeds=[task('a',title,Priority='high'),task('keep','Плановые работы',Priority='normal'),task('done','Готовый результат',Priority='high',Status='done')]
        add('batch_complete_filters',i,seeds,[turn('Все открытые важные задачи отметь как выполненные.','clarify',pending=True,Count=1),
            turn('да','complete',route='fast_path',Count=1,Changes=[dict(Key='a',Status='done')])])
    for i,title in enumerate(subjects):
        new_title=f'Этап {i+1}: {title}'
        add('correction_chain',i,[],[turn(f'Заведи задачу «{title}» на завтра.','create',Count=1,Event=dict(Key='a',Kind='task',Title=title,Date=date(1))),
            turn('Нет, лучше послезавтра.','update',Count=1,Changes=[dict(Key='a',Date=date(2))]),
            turn(f'И переименуй её в «{new_title}».','update',Count=1,Changes=[dict(Key='a',Title=new_title)])])
    for i,topic in enumerate(topics):
        title=topic+' установки'
        seeds=[task('a',title,1,'07:20'),task('b',title,1,'11:40')]
        if i%2==0:
            text='Вторую перенеси на послезавтра, не на завтра.';change=dict(Key='b',Date=date(2))
        else:
            text='Вторую сдвинь на два часа позже, а не на час.';change=dict(Key='b',Time='13:40')
        add('semantic_adversarial',i,seeds,[turn(f'Найди задачи «{title}».','search',Count=2),
            turn(text,'update',Count=1,Changes=[change]),turn(f'Ещё раз найди задачи «{title}».','search',Count=2)])
    deferrals=['Согласия ещё не давал, пока только смотрю список.','Подтверждение отложим, сначала всё проверю.',
               'Не исполняй пока: нужно обдумать последствия.','Это ещё не разрешение удалить, я изучаю список.',
               'Хочу сначала подумать; подтверждать рано.','Удаление не запускай, я пока решаю.',
               'Слово «да» здесь лишь пример ответа, не согласие.', 'Никакого подтверждения сейчас не даю.',
               'Подожди с исполнением, пока я не разрешил удаление.','Пока согласия нет, только обсуждаем.']
    for i,title in enumerate(subjects):
        seeds=[task('a',title),task('b','Ночной журнал'),task('c','Карта созвездий')]
        add('destructive_safety',i,seeds,[turn('Убери все задачи из моего списка.','clarify',pending=True,Count=3),
            turn(deferrals[i],'clarify',pending=True,DBCount=3),turn('отмена','cancel',route='fast_path',DBCount=3)])
    assert len(rows)==150
    assert sum(len(r['Turns']) for r in rows)==370
    assert len({r['ID'] for r in rows})==150
    return rows


if __name__=='__main__':write(ROOT/'testdata/semantic/plan_heldout_v7.jsonl',build())
