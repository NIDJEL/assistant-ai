"""Fresh v3 authoring protocol: run ONLY after DEV/code/prompt are final.

Uses new entities, conversation templates and calendar boundary dates. Does not
read DEV examples, model outputs or any historical holdout. Freeze before calls.
"""
import argparse
import calendar
from datetime import date, datetime, timedelta

from plan_corpus import ROOT, NOW, event, turn, write


def fresh():
    rows = []

    def add(category, i, seeds, turns, now=NOW):
        rows.append(dict(ID=f"semantic-fresh-v3-{category}-{i:02d}", Category=category,
                         Now=now, Seeds=seeds, Turns=turns))

    names = ["Аглаей", "Брониславом", "Валерией", "Гордеем", "Дариной",
             "Евсеем", "Жанной", "Зиновием", "Иларией", "Климом"]
    subjects = ["Инвентаризация", "Калибровка", "Сверка", "Аттестация", "Стажировка",
                "Комплектация", "Логистика", "Сертификация", "Паспортизация", "Диагностика"]

    chats = ["Привет", "Здравствуй", "Здравствуйте", "Привет Гена", "Кто ты?",
             "Как тебя зовут?", "Что ты умеешь?", "Помощь", "Благодарю", "Спасибо!"]
    for i, first in enumerate(chats):
        add("simple_chat", i, [], [turn(first, "help", route="fast_path"),
                                  turn("Спасибо", "help", route="fast_path")])

    for i, subject in enumerate(subjects):
        title = f"Сессия: {subject}"
        clock = f"{14+i%5:02d}:25"
        add("simple_pending_time", i, [], [
            turn(f"Запланируй на завтра встречу с названием «{title}», часы уточню следом.",
                 "clarify", pending=True, Count=0),
            turn(f"в {clock}", "create", route="fast_path", Count=1,
                 Event=dict(Key="made", Kind="meeting", Title=title, Date="2026-10-18", Time=clock))])

    for i, subject in enumerate(subjects):
        minutes = 31 + i
        title = f"Проверить этап: {subject}"
        hour, minute = divmod(30 + minutes, 60)
        add("simple_pending_delay", i, [], [
            turn(f"Сделай напоминание «{title}», пока без указания срока, спроси меня о нём.",
                 "clarify", pending=True, Count=0),
            turn(f"через {minutes} минут", "create", route="fast_path", Count=1,
                 Event=dict(Key="r", Kind="reminder", Title=title, Date="2026-10-17", Time=f"{12+hour:02d}:{minute:02d}"))])

    for i, subject in enumerate(subjects):
        title = f"Организовать: {subject}"
        add("normal_commands", i, [], [
            turn(f"Поставь задачу «{title}» со сроком послезавтра, точный час не нужен.",
                 "create", Count=1, Event=dict(Key="new", Title=title, Kind="task", Date="2026-10-19")),
            turn("Готово, эту задачу пометь как выполненную", "complete", Count=1,
                 Changes=[dict(Key="new", Status="done")])])

    for i, subject in enumerate(subjects):
        add("normal_search", i, [event("a", subject, "task"), event("b", subject)], [
            turn(f"Нужно найти всё по теме «{subject}», независимо от вида записи.",
                 "search", Count=2, Query=subject),
            turn("Среди этих результатов оставь исключительно встречи.", "search", Count=1, Query=subject)])

    for i, subject in enumerate(subjects):
        title = f"Ревизия: {subject}"
        add("difficult_search", i, [
            event("keep", title, date="2026-08-13", time="15:20", Priority="high"),
            event("omit", title+" склад", date="2026-07-16", time="16:20", Priority="high"),
            event("low", title, date="2026-09-11", time="15:20"),
            event("outside", title, Priority="high")], [
            turn(f"Отыщи встречи по теме «{title}» за прошлый квартал, только высокий приоритет и после 14:00; склад исключи.",
                 "search", Count=1, Start="2026-07-01", End="2026-10-01"),
            turn("Благодарю", "help", route="fast_path")])

    for i, n in enumerate(names):
        title = f"Планёрка с {n}"
        add("contextual_reference", i, [event("one", title, time="08:10"), event("two", title, time="11:10")], [
            turn(f"Поищи планёрки с {n}", "search", Count=2),
            turn("У первой поменяй только время на 18:35.", "update", Count=1,
                 Changes=[dict(Key="one", Time="18:35")])])

    dates = ["2027-01-06", "2026-12-12", "2028-03-01", "2027-03-01", "2026-07-05",
             "2026-04-02", "2027-02-14", "2026-11-30", "2028-01-02", "2027-06-01"]
    followups = ["Ограничь это предыдущим месяцем.", "А из этих — лишь за прошлый месяц?",
                 "Теперь тот же запрос, но месяцем раньше.", "Предыдущий календарный месяц оставь в результате.",
                 "Мне нужен только прошлый месяц из этого поиска.", "Сузь найденное до прошлого месяца.",
                 "В рамках того же поиска покажи предыдущий месяц.", "Оставь встречи лишь прошлого месяца.",
                 "За прошлый месяц, пожалуйста, повтори этот поиск.", "Тот же участник, период — предыдущий месяц."]
    for i, n in enumerate(names):
        today = date.fromisoformat(dates[i]); end = today.replace(day=1)
        last = end-timedelta(days=1); start = last.replace(day=1)
        add("calendar_refinement", i, [
            event("old", f"Обзор с {n}", date=(start+timedelta(days=9)).isoformat()),
            event("current", f"Обзор с {n}", date=today.isoformat())], [
            turn(f"Найди обзоры с {n}", "search", Count=2, Query=n),
            turn(followups[i], "search", Count=1, Query=n, Start=start.isoformat(), End=end.isoformat())],
            now=today.isoformat()+"T12:30:00+03:00")

    for i, subject in enumerate(subjects):
        minutes = 70 + i*5
        a = datetime(2026,10,20,8,0)-timedelta(minutes=minutes)
        b = datetime(2026,10,20,12,0)-timedelta(minutes=minutes)
        add("batch_shift", i, [event("a", subject, time="08:00"), event("b", subject, time="12:00")], [
            turn(f"Для всех встреч будущей недели сделай начало на {minutes} минут раньше.",
                 "clarify", pending=True, Count=2),
            turn("да", "update", route="fast_path", Count=2,
                 Changes=[dict(Key="a", Time=a.strftime("%H:%M")), dict(Key="b", Time=b.strftime("%H:%M"))])])

    for i, n in enumerate(names):
        add("batch_delete_cancel", i, [event("a", f"Консультация с {n}"), event("b", "Резервная встреча", time="13:00")], [
            turn("Убери из календаря все встречи будущей недели.", "clarify", pending=True, Count=2),
            turn("не надо", "cancel", route="fast_path", DBCount=2)])

    for i, subject in enumerate(subjects):
        add("batch_complete", i, [event("a", subject, "task"), event("b", "Проверить итог", "task")], [
            turn("Все открытые задачи на будущую неделю отметь выполненными.", "clarify", pending=True, Count=2),
            turn("подтверждаю", "complete", route="fast_path", Count=2,
                 Changes=[dict(Key="a", Status="done"), dict(Key="b", Status="done")])])

    for i, n in enumerate(names):
        add("batch_exclusion", i, [event("keep", "Общее обсуждение", date="2026-10-22"),
                                   event("omit", f"Беседа с {n}", date="2026-10-23", time="13:00")], [
            turn(f"Перемести встречи будущей недели на послезавтра, исключая беседу с {n}.",
                 "clarify", pending=True, Count=1),
            turn("да", "update", route="fast_path", Count=1, Changes=[dict(Key="keep", Date="2026-10-19")])])

    for i, n in enumerate(names):
        add("ambiguity", i, [event("a", f"Обсуждение с {n}"), event("b", f"Обсуждение с {n}", time="14:00")], [
            turn(f"Найди обсуждения с {n}", "search", Count=2),
            turn("Её на 18:10 передвинь.", "clarify", DBCount=2)])

    reservations = ["Да, но только если это не затронет важные записи.",
                    "Да, хотя я пока не разрешаю удаление.", "Да, но сначала объясни последствия.",
                    "Подтверждаю только просмотр, удаления не подтверждаю.",
                    "Да, но действие пока оставь без исполнения.", "Я сказал бы да, если бы хотел удалить.",
                    "Не уверен: да или нет, давай уточним.", "Да, но ещё не выполняй это.",
                    "Да — это цитата, не подтверждение.", "Да, только после отдельного согласования со мной."]
    for i, subject in enumerate(subjects):
        title = f"Контроль: {subject}"
        add("destructive_safety", i, [event("keep", title, "task")], [
            turn(f"Удали задачу «{title}»", "clarify", pending=True, Count=0),
            turn(reservations[i], "clarify", pending=True, DBCount=1)])

    for i, subject in enumerate(subjects):
        seconds = 45+i*5
        at = datetime.fromisoformat(NOW)+timedelta(seconds=seconds)
        title = f"Проверить: {subject}"
        add("natural_timer", i, [], [
            turn(f"Через {seconds} секунд напомни действие «{title}», пожалуйста.", "create", Count=1,
                 Event=dict(Key="timer", Kind="reminder", Title=title, Date="2026-10-17", Time=at.strftime("%H:%M"))),
            turn("Спасибо", "help", route="fast_path")])

    assert len(rows) == 150 and sum(len(r["Turns"]) for r in rows) == 300
    return rows


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--dev-final-complete", action="store_true", required=True,
                        help="Explicitly attest code/prompt/DEV are final before authoring")
    parser.add_argument("--output", default="testdata/semantic/plan_heldout_v3.jsonl")
    args = parser.parse_args()
    write(ROOT/args.output, fresh())
