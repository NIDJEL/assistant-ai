import pytest

from app.dates import extract
from app.nlp import parse
from tests.conftest import NOW


@pytest.mark.parametrize(
    "text,day,clock",
    [
        ("сегодня", "2026-10-12", None),
        ("завтра", "2026-10-13", None),
        ("послезавтра", "2026-10-14", None),
        ("вчера", "2026-10-11", None),
        ("в пятницу", "2026-10-16", None),
        ("в следующий понедельник", "2026-10-19", None),
        ("через два часа", "2026-10-12", "14:30"),
        ("через 30 минут", "2026-10-12", "13:00"),
        ("через три дня", "2026-10-15", None),
        ("завтра в 10 утра", "2026-10-13", "10:00"),
        ("завтра в десять утра", "2026-10-13", "10:00"),
        ("завтра в 3 дня", "2026-10-13", "15:00"),
        ("завтра в 7 вечера", "2026-10-13", "19:00"),
        ("в четверг в 11", "2026-10-15", "11:00"),
        ("2026-11-02 в 17:45", "2026-11-02", "17:45"),
        ("02.11.2026 в 17:45", "2026-11-02", "17:45"),
        ("2 ноября 2026", "2026-11-02", None),
        ("2 января", "2027-01-02", None),
        ("до конца недели", "2026-10-18", None),
        ("в двадцать один", None, "21:00"),
    ],
)
def test_temporal(text, day, clock):
    result = extract(text, NOW)
    assert (result.date, result.time, result.error) == (day, clock, None)


@pytest.mark.parametrize(
    "text,start,end",
    [
        ("на эту неделю", "2026-10-12", "2026-10-19"),
        ("на следующей неделе", "2026-10-19", "2026-10-26"),
        ("за прошлую неделю", "2026-10-05", "2026-10-12"),
        ("на выходных", "2026-10-17", "2026-10-19"),
        ("за 2024 год", "2024-01-01", "2025-01-01"),
    ],
)
def test_period(text, start, end):
    result = extract(text, NOW)
    assert (result.start, result.end) == (start, end)


@pytest.mark.parametrize("text", ["31.02.2026", "31 февраля", "в 25:00", "в 15:75", "сегодня завтра", "на 2 минуты", "через 0 часов"])
def test_invalid_temporal(text):
    assert extract(text, NOW).error


@pytest.mark.parametrize(
    "text,action,kind",
    [
        ("Напомни мне завтра в 10 утра позвонить Ивану", "create", "reminder"),
        ("В пятницу нужно отправить отчет", "create", "task"),
        ("Поставь задачу купить билеты до конца недели", "create", "task"),
        ("Добавь встречу с командой завтра в 15:00", "create", "meeting"),
        ("В четверг в 11 у меня встреча с дизайнером", "create", "meeting"),
        ("Что у меня сегодня?", "list", None),
        ("Какие планы на сегодня?", "list", None),
        ("Покажи мои встречи на сегодня", "list", "meeting"),
        ("Есть ли у меня встречи сегодня?", "list", "meeting"),
        ("Какие задачи у меня на завтра?", "list", "task"),
        ("Что запланировано на эту неделю?", "list", None),
        ("Найди задачу про презентацию", "search", "task"),
        ("Когда у меня была встреча с Иваном?", "search", "meeting"),
        ("Я когда-то ставил задачу по договору, найди ее", "search", "task"),
    ],
)
def test_assignment_phrases(text, action, kind):
    command = parse(text, NOW)
    assert (command.action, command.kind) == (action, kind)
