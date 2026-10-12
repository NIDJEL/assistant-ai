from datetime import datetime
from zoneinfo import ZoneInfo

from app.dates import extract


def test_task_mentioning_meeting_remains_task(say):
    result = say("Поставь задачу подготовиться к встрече с Иваном завтра")
    assert result["action"] == "create"
    assert result["events"][0]["kind"] == "task"


def test_period_for_creation_requires_specific_day(say):
    result = say("Поставь задачу отправить отчёт на следующей неделе")
    assert result["action"] == "clarify"
    assert say("20 октября")["events"][0]["date"] == "2026-10-20"


def test_sunday_is_part_of_current_weekend():
    now = datetime(2026, 10, 18, 12, tzinfo=ZoneInfo("Europe/Moscow"))
    temporal = extract("на выходных", now)
    assert (temporal.start, temporal.end) == ("2026-10-17", "2026-10-19")
