def test_title_reply_preserves_reminder_datetime(say):
    assert say("Напомни завтра в 10 утра")["pending"]
    result = say("купить билеты")
    event = result["events"][0]
    assert event["kind"] == "reminder"
    assert event["date"] == "2026-10-13"
    assert event["time"] == "10:00"
    assert event["title"].lower() == "купить билеты"


def test_invalid_date_is_removed_from_title(say):
    assert say("Добавь встречу с Иваном 31.02.2026 в 15:00")["pending"]
    result = say("завтра")
    assert result["events"][0]["title"] == "Встреча с Иваном"


def test_natural_confirmation(say):
    event = say("Поставь задачу купить билеты")["events"][0]
    say(f"Удали задачу #{event['id']}")
    assert say("Да, пожалуйста")["action"] == "delete"
