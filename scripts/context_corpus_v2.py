"""Independent DEV by error class; fresh v2 is authored only after DEV tuning.

Never opens either previous held-out file. The freeze command refuses overwrite.
The evaluator separately freezes input, prompt, schema and code before inference.
"""
import argparse
import hashlib
import json
from collections import Counter
from datetime import datetime, timedelta
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
DATA = ROOT / "testdata/semantic"
NOW = "2026-10-16T12:30:17+03:00"


def turn(message, kind, action, pending=False, **want):
    return dict(message=message, expect=dict(turn_type=kind, action=action, pending=pending, **want))


def event(title, kind="meeting", date="2026-10-17", time="10:00", **extra):
    return dict(title=title, kind=kind, date=date, time=time, **extra)


def delayed(title, minutes):
    at = datetime.fromisoformat(NOW) + timedelta(minutes=minutes)
    return event(title, "reminder", at.strftime("%Y-%m-%d"), at.strftime("%H:%M"), occurrence_at=at.isoformat())


def seed(key, title, kind="meeting", date="2026-10-17", time="10:00"):
    return dict(key=key, title=title, kind=kind, date=date, time=time)


def conversation(id, category, turns, seeds=()):
    return dict(id=id, category=category, now=NOW, seeds=list(seeds), turns=turns)


def build_dev():
    out = []
    temporal = [
        ("Ну хорошо, через 7 минут", 7), ("Ладно, давай-ка через 13 минут", 13),
        ("Пожалуй, через 21 минуту", 21), ("А тогда через 34 минуты", 34),
        ("Да, через 8 минут, пожалуйста", 8), ("Можно через 19 минут", 19),
        ("Ну значит через 26 минут", 26), ("Хорошо, минут через двенадцать", 12),
        ("Давай минут через семнадцать", 17), ("Ладно, минут через двадцать три", 23),
        ("Тогда через полчаса", 30), ("Пожалуйста, через час", 60),
        ("Ну давай через пару часов", 120), ("Хорошо, через полтора часа", 90),
        ("Пожалуй, через 2 часа и 7 минут", 127), ("А давай через 46 минут", 46),
        ("Ладно, через 32 минуты, хорошо", 32), ("Ну, через 16 минут, пожалуйста", 16),
        ("Давай-ка через 29 минут", 29), ("Окей, через 41 минуту", 41),
    ]
    titles = ["Промыть фильтр", "Забрать посылку", "Полить рассаду", "Выключить обогреватель", "Проверить бойлер", "Снять белье", "Закрыть теплицу", "Отключить насос", "Проверить аккумулятор", "Достать форму", "Замочить крупу", "Подписать акт", "Зарядить фонарь", "Проверить генератор", "Сменить картридж", "Вымыть кисти", "Накрыть тесто", "Проверить сканер", "Запустить сушилку", "Отнести образцы"]
    for i, ((reply, minutes), title) in enumerate(zip(temporal, titles)):
        out.append(conversation(f"dev2-temporal-{i+1:02}", "temporal_filler", [
            turn("Напомни " + title[0].lower() + title[1:], "command", "clarify", True),
            turn(reply, "slot_reply", "create", event=delayed(title, minutes), max_model_calls=0),
        ]))
    clocks = [("Ну хорошо, часов в семь", "07:00"), ("Ладно, давай в 14:35", "14:35"), ("А тогда в девять вечера", "21:00"), ("Можно в 16:20, пожалуйста", "16:20"), ("Пожалуй, в полдень", "12:00")]
    for i, (reply, clock) in enumerate(clocks):
        title = f"Обсуждение {['сметы', 'макета', 'поставки', 'эксперимента', 'стенда'][i]}"
        out.append(conversation(f"dev2-clock-{i}", "temporal_filler", [turn(f"Добавь {title.lower()} завтра", "command", "clarify", True), turn(reply, "slot_reply", "create", event=event(title, time=clock), max_model_calls=0)]))
    for i, (reply, date) in enumerate([("Ну тогда послезавтра", "2026-10-18"), ("Хорошо, в понедельник", "2026-10-19"), ("Ладно, давай 23 октября", "2026-10-23"), ("Пожалуй, завтра", "2026-10-17"), ("А давай-ка в субботу", "2026-10-17")]):
        title = "Встреча с " + ["Ренатом", "Агатой", "Трофимом", "Златой", "Елисеем"][i]
        out.append(conversation(f"dev2-date-{i}", "temporal_filler", [turn(f"Добавь {title.lower()} в 16:30", "command", "clarify", True), turn(reply, "slot_reply", "create", event=event(title, date=date, time="16:30"), max_model_calls=0)]))
    negatives = ["Ну нет", "Пожалуйста, не удаляй", "Ладно, не надо", "Хорошо, отмена", "Передумала", "Пока не нужно", "Тогда оставь", "Пусть останется, пожалуйста", "Ну не трогай", "Гена, отмени", "Нет, пока не надо", "Пожалуй, оставь как есть", "Не надо, передумал", "Стоп, не удаляй", "Пожалуй, не нужно", "Ладно, пусть останется"]
    positives = ["Ага", "Подтверждаю", "Давай", "Удаляй", "Точно", "Верно", "Да, пожалуйста, удали", "Ну да, удаляй", "Подтверждаю удаление"]
    for i, reply in enumerate(negatives + positives):
        yes = i >= len(negatives)
        title = "Проверить " + ["манометр", "термостат", "компрессор", "переходник", "датчик", "кабель", "клапан", "редуктор", "микрофон", "разъем", "предохранитель", "вентиль", "принтер", "штатив", "отражатель", "кронштейн", "стабилизатор", "проектор", "радиатор", "фильтр", "адаптер", "счетчик", "трансформатор", "штекер", "пульт"][i]
        out.append(conversation(f"dev2-consent-{i+1:02}", "cancellation_confirmation", [turn("Удали задачу " + title.lower(), "command", "clarify", True), turn(reply, "confirmation" if yes else "cancellation", "delete" if yes else "cancel", max_model_calls=0)], [seed("target", title, "task", "", "")]))
    names = ["Савелием", "Макаром", "Архипом", "Платоном", "Эвелиной", "Миланой", "Аксиньей", "Есенией", "Радмиром", "Лукьяном", "Тамилой", "Серафимой", "Леонтием", "Снежаной", "Аполлинарием", "Бориславой", "Лианой", "Велимиром", "Аделиной", "Станиславой"]
    periods = [("за прошлую неделю", "2026-10-05", "2026-10-12", "2026-10-06"), ("на этой неделе", "2026-10-12", "2026-10-19", "2026-10-14"), ("на следующей неделе", "2026-10-19", "2026-10-26", "2026-10-21")]
    for i, name in enumerate(names):
        phrase, start, end, date = periods[i % len(periods)]
        title = "Встреча с " + name
        out.append(conversation(f"dev2-refine-{i+1:02}", "refinement", [
            turn("Найди встречи с " + name, "command", "search", count=3, command=dict(action="search", kind="meeting", query=name)),
            turn(["Ну а только ", "А давай только ", "Тогда только ", "Оставь только "][i % 4] + phrase + "?", "refinement", "search", count=1, event=event(title, date=date), command=dict(action="search", kind="meeting", query=name, period_start=start, period_end=end)),
        ], [seed(str(j), title, date=d) for j, d in enumerate(["2026-10-06", "2026-10-14", "2026-10-21"])]))
    for i, kind in enumerate(["meeting", "task", "reminder", "meeting", "task"]):
        noun = dict(meeting="встречи", task="задачи", reminder="напоминания")[kind]
        title = dict(meeting="Сверка бюджета", task="Собрать комплект", reminder="Проверить зарядку")[kind]
        out.append(conversation(f"dev2-list-refine-{i}", "refinement", [turn("Что у меня на следующей неделе?", "command", "list", count=3), turn("А только " + noun + "?", "refinement", "list", count=1, command=dict(action="list", kind=kind, period_start="2026-10-19", period_end="2026-10-26"))], [seed(k, t, k, "2026-10-21") for k, t in [("meeting", "Сверка бюджета"), ("task", "Собрать комплект"), ("reminder", "Проверить зарядку")]]))
    timers = [
        ("Таймер на 6 минут промыть кофемашину", "Промыть кофемашину", 6),
        ("На 7 минут поставь таймер проверить вытяжку", "Проверить вытяжку", 7),
        ("8 минут таймер на вентиляцию", "Вентиляция", 8),
        ("Засеки минут девять и выключи паяльник", "Выключить паяльник", 9),
        ("На 11 минут включи таймер проверить суп", "Проверить суп", 11),
        ("Таймер минут на 12 проверить датчик", "Проверить датчик", 12),
        ("13 минут поставь таймер отключить зарядку", "Отключить зарядку", 13),
        ("Засеки минут четырнадцать и закрой гараж", "Закрыть гараж", 14),
        ("На 16 минут запусти таймер проверить сушку", "Проверить сушку", 16),
        ("Таймер на 17 минут накрыть форму", "Накрыть форму", 17),
        ("19 минут таймер на ингаляцию", "Ингаляция", 19),
        ("Засеки минут двадцать один и отключи обогрев", "Отключить обогрев", 21),
        ("Таймер на полчаса проверить компост", "Проверить компост", 30),
        ("На час поставь таймер забрать инструменты", "Забрать инструменты", 60),
        ("Засеки пару часов и проверь закваску", "Проверить закваску", 120),
    ]
    for i, (message, title, minutes) in enumerate(timers):
        out.append(conversation(f"dev2-timer-{i+1:02}", "timer_word_order", [turn(message, "command", "create", event=delayed(title, minutes)), turn("Спасибо", "chat", "help")]))
    return out


def build_search_dev():
    names = ["Еремеем", "Федосьей", "Добрыней", "Василисой", "Нестором", "Роксаланой", "Сильвестром", "Октавией", "Гордеем", "Аурелией", "Мироном", "Доминикой", "Прохором", "Таисией", "Эрнестом", "Лукерией", "Осипом", "Забавой", "Ярополком", "Прасковьей", "Аристархом", "Февронией", "Иннокентием", "Меланией", "Тихоном"]
    out=[]
    for i, name in enumerate(names):
        title = "Встреча с " + name
        out.append(conversation(f"dev2-search-{i+1:02}", "single_turn_search", [turn("Найди встречи с " + name, "command", "search", count=1, command=dict(action="search", kind="meeting", query=name))], [seed("target", title)]))
    return out


def build_fresh():
    # This branch is not executed until all DEV/code work has finished. Distinct
    # vocabulary, numbers and participants; no old held-out file is consulted.
    out=[]
    objects=["Проверить осушитель", "Закрыть люк", "Забрать штатив", "Выключить гирлянду", "Промыть колбу", "Отнести пробирки", "Проверить заряд", "Полить базилик", "Убрать трафарет", "Снять зажим", "Проверить морозильник", "Закрыть ставни", "Забрать переплет", "Сменить салфетку", "Выключить увлажнитель", "Проверить террариум", "Промыть сито", "Накрыть противень", "Снять показания", "Отнести реактивы", "Проверить штампы", "Закрыть павильон", "Забрать гравировку", "Сменить насадку", "Выключить компрессор", "Проверить инкубатор", "Промыть шланг", "Накрыть поддон", "Снять табличку", "Отнести оттиски"]
    for i,title in enumerate(objects):
        n=22+i*2
        patterns=[f"Ну ладно, давай через {n} минуты",f"Хорошо, тогда через {n} минут",f"А пожалуй, минут через {n}",f"Давай-ка через {n} минут, пожалуйста",f"Ну да, через {n} минут, ладно"]
        out.append(conversation(f"fresh2-slot-{i+1:02}","temporal_filler",[turn("Напомни "+title.lower(),"command","clarify",True),turn(patterns[i%5],"slot_reply","create",event=delayed(title,n),max_model_calls=0)]))
    negative=["Ну, пожалуй, пока не надо", "Не удаляй, пожалуйста", "Нет, оставь", "Гена, не трогай", "Пока оставь как есть", "Передумала, не нужно", "Ну ладно, отмена", "Пожалуй, отмени", "Не надо, пусть останется", "Стоп, передумал", "Нет, пожалуйста", "Пока не трогай", "Тогда не удаляй", "Не нужно, оставь", "Пусть останется, хорошо"]
    positive=["Ага, удаляй", "Да, подтверждаю", "Давай, пожалуйста", "Ну точно", "Верно, удаляй", "Хорошо, подтверждаю удаление", "Да, конечно", "Пожалуйста, удали", "Ну давай", "Да, удаляй, пожалуйста"]
    for i,reply in enumerate(negative+positive):
        yes=i>=len(negative); title="Осмотреть "+["антенну", "гнездо", "катушку", "плату", "корпус", "перемычку", "реле", "клемму", "радиоблок", "диффузор", "решетку", "уплотнитель", "поршень", "маховик", "муфту", "шкив", "подшипник", "втулку", "стойку", "статор", "ротор", "магнит", "пружину", "шестерню", "ремень"][i]
        out.append(conversation(f"fresh2-consent-{i+1:02}","cancellation_confirmation",[turn("Удали задачу "+title.lower(),"command","clarify",True),turn(reply,"confirmation" if yes else "cancellation","delete" if yes else "cancel",max_model_calls=0)],[seed("target",title,"task","","")]))
    names=["Авдеем", "Глафирой", "Мстиславом", "Леокадией", "Всеволодом", "Анфисой", "Фаддеем", "Изольдой", "Евлампием", "Флорентиной", "Игнатием", "Агнией", "Лаврентием", "Владленой", "Мефодием", "Иларией", "Северином", "Стефанией", "Устином", "Людмилой", "Демидом", "Ренатой", "Климом", "Эльвирой", "Соломоном"]
    for i,name in enumerate(names):
        title="Встреча с "+name
        periods=[("за прошлый месяц","2026-09-01","2026-10-01","2026-09-24"),("на следующей неделе","2026-10-19","2026-10-26","2026-10-22"),("на этой неделе","2026-10-12","2026-10-19","2026-10-15")]
        phrase,start,end,date=periods[i%3]
        out.append(conversation(f"fresh2-refine-{i+1:02}","refinement",[turn("Найди встречи с "+name,"command","search",count=3,command=dict(action="search",kind="meeting",query=name)),turn(["А только ","Ну а только ","Тогда оставь только "][i%3]+phrase+"?","refinement","search",count=1,command=dict(action="search",kind="meeting",query=name,period_start=start,period_end=end),event=event(title,date=date))],[seed(str(j),title,date=d) for j,d in enumerate(["2026-09-24","2026-10-15","2026-10-22"])]))
    for i in range(15):
        n=24+i; title=["Проверить стерилизатор","Выключить прожектор","Накрыть контейнер"][i%3]
        phrases=[f"На {n} минут поставь таймер {title.lower()}",f"{n} минут таймер {title.lower()}",f"Засеки минут {n} и {title.lower()}"]
        out.append(conversation(f"fresh2-timer-{i+1:02}","timer_word_order",[turn(phrases[i%3],"command","create",event=delayed(title,n)),turn("Благодарю","chat","help")]))
    # Reference/target audit includes both unambiguous and deliberately ambiguous
    # lists. Clarification is the correct expected outcome for the latter.
    for i in range(40):
        title="Встреча с "+names[i%len(names)]; other="Обсуждение экспозиции"
        if i<15:
            second=turn("А перенеси ее на 18:40","reference","update",reference="last",event=event(title,time="18:40",same_as="a")); seeds=[seed("a",title)]
        elif i<30:
            second=turn("Вторую перенеси на 19:25","reference","update",reference="second",event=event(other,time="19:25",same_as="b"));seeds=[seed("a",title),seed("b",other,time="15:10")]
        else:
            second=turn("Перенеси ее на 17:50","reference","clarify");seeds=[seed("a",title),seed("b",other,time="15:10")]
        out.append(conversation(f"fresh2-reference-{i+1:02}","references_ambiguity",[turn("Покажи встречи на завтра","command","list",count=len(seeds)),second],seeds))
    return out


def write_new(name, rows):
    path=DATA/name
    content="".join(json.dumps(row,ensure_ascii=False,separators=(",",":"))+"\n" for row in rows).encode()
    with path.open("xb") as f: f.write(content)
    return dict(conversations=len(rows),turns=sum(len(r["turns"]) for r in rows),sha256=hashlib.sha256(content).hexdigest(),categories=dict(Counter(r["category"] for r in rows)))


def main():
    p=argparse.ArgumentParser();p.add_argument("phase",choices=["dev","fresh"]);args=p.parse_args()
    manifest={"protocol":"Tune on DEV classes only. Fresh v2 is run once after code/prompt/schema/input freeze; no tuning on audit failures.","previous_holdouts_read":False,"files":{}}
    files=[("context_dev_v2.jsonl",build_dev()),("context_search_dev_v2.jsonl",build_search_dev())] if args.phase=="dev" else [("context_heldout_v2.jsonl",build_fresh())]
    for name,rows in files:manifest["files"][name]=write_new(name,rows)
    with (DATA/f"context_{args.phase}_v2_manifest.json").open("x",encoding="utf-8") as f:json.dump(manifest,f,ensure_ascii=False,indent=2);f.write("\n")
    print(json.dumps(manifest,ensure_ascii=False,indent=2))


if __name__=="__main__": main()
