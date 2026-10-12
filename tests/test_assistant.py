from concurrent.futures import ThreadPoolExecutor
from datetime import timedelta
from uuid import uuid4

from fastapi.testclient import TestClient

from app.main import create_app
from tests.conftest import NOW


def test_four_key_actions(say):
    task = say("В пятницу нужно отправить отчёт")
    assert task["events"][0]["title"] == "Отправить отчёт"
    assert task["events"][0]["date"] == "2026-10-16"
    reminder = say("Напомни мне завтра в 10 утра позвонить Ивану")
    assert reminder["events"][0]["time"] == "10:00"
    meeting = say("Добавь встречу с командой завтра в 15:00")
    assert meeting["events"][0]["kind"] == "meeting"
    assert len(say("Что запланировано на эту неделю?")["events"]) == 3
    assert len(say("Есть ли у меня встречи завтра?")["events"]) == 1
    assert say("Найди задачу про отчет")["events"][0]["id"] == task["events"][0]["id"]


def test_clarification_and_context(say):
    assert say("Поставь встречу с Иваном завтра")["pending"]
    meeting = say("В 15:00")
    assert meeting["action"] == "create"
    moved = say("А перенеси ее на послезавтра")
    assert moved["events"][0]["date"] == "2026-10-14"
    assert moved["events"][0]["time"] == "15:00"
    assert say("Отметь ее выполненной")["events"][0]["status"] == "done"


def test_clarify_date_and_title(say):
    assert say("Добавь встречу с Ольгой")["pending"]
    assert say("завтра")["pending"]
    assert say("в 11")["action"] == "create"
    assert say("Поставь задачу")["pending"]
    result = say("Забрать посылку")
    assert result["events"][0]["title"] == "Забрать посылку"
    assert result["events"][0]["date"] is None


def test_invalid_time_then_correction(say):
    assert say("Добавь встречу с командой завтра в 25:00")["pending"]
    result = say("в 16:00")
    assert result["action"] == "create"
    assert result["events"][0]["time"] == "16:00"


def test_conflict_is_not_silently_created(say, client):
    say("Добавь встречу с Иваном завтра в 15:00 на два часа")
    conflict = say("Добавь встречу с Ольгой завтра в 16:00")
    assert conflict["action"] == "clarify"
    assert client.get("/api/events").json()["total"] == 1
    assert say("в 17:00")["action"] == "create"  # adjacent intervals do not overlap
    result = say("Перенеси встречу с Ольгой на завтра в 15:30")
    assert result["pending"]
    assert say("да")["action"] == "update"


def test_force_and_cancel_conflict(say):
    say("Добавь встречу с Иваном завтра в 15:00")
    say("Добавь встречу с Ольгой завтра в 15:30")
    assert say("нет")["action"] == "cancel"
    say("Добавь встречу с Ольгой завтра в 15:30")
    assert say("да")["action"] == "create"


def test_midnight_conflict(say):
    say("Добавь встречу с Иваном сегодня в 23:30 на два часа")
    assert say("Добавь встречу с Ольгой завтра в 00:30")["pending"]


def test_search_morphology_synonyms_and_typo(say):
    say("Поставь задачу проверить договор с Иваном")
    say("Поставь задачу подготовить презентацию")
    assert say("Найди задачу про контракт с Иваном")["events"][0]["title"] == "Проверить договор с Иваном"
    assert say("Найди задачу про презинтацию")["events"][0]["title"] == "Подготовить презентацию"
    assert len(say("Я когда-то ставил задачу по договору, найди ее")["events"]) == 1


def test_delete_requires_confirmation_and_ambiguity(say, client):
    a = say("Поставь задачу отправить договор Ивану")["events"][0]
    b = say("Поставь задачу отправить договор Ольге")["events"][0]
    choice = say("Удали задачу про договор")
    assert len(choice["candidates"]) == 2
    assert say(f"#{a['id']}")["pending"]
    assert client.get("/api/events").json()["total"] == 2
    assert say("нет")["action"] == "cancel"
    say(f"Удали задачу #{a['id']}")
    assert say("да")["action"] == "delete"
    assert client.get("/api/events").json()["events"][0]["id"] == b["id"]


def test_idempotent_retry(say, client):
    rid = str(uuid4())
    a = say("Поставь задачу купить билеты", request_id=rid)
    b = say("Поставь задачу купить билеты", request_id=rid)
    assert a == b
    assert client.get("/api/events").json()["total"] == 1
    response = client.post("/api/chat", json={"message": "Поставь задачу купить хлеб", "session_id": say.session, "request_id": rid})
    assert response.status_code == 422


def test_persistence_across_restart_and_session_context(say, settings):
    say("Добавь встречу с Иваном завтра")
    with TestClient(create_app(settings, clock=lambda: NOW)) as fresh:
        result = fresh.post("/api/chat", json={"message": "в 12", "session_id": say.session}).json()
        assert result["action"] == "create"
        assert fresh.get("/api/events").json()["total"] == 1
        assert len(fresh.get("/api/history/" + say.session).json()["exchanges"]) == 2
        other = fresh.post("/api/chat", json={"message": "Перенеси ее на завтра"}).json()
        assert other["action"] == "clarify"


def test_half_open_week_boundary(say):
    say("Добавь встречу с Иваном 18.10.2026 в 23:00")
    say("Добавь встречу с Ольгой 19.10.2026 в 10:00")
    assert len(say("Что запланировано на эту неделю?")["events"]) == 1
    assert len(say("Что запланировано на следующей неделе?")["events"]) == 1


def test_concurrent_conflicts(client):
    def create(person):
        return client.post("/api/chat", json={"message": f"Добавь встречу с {person} завтра в 15:00"}).json()

    with ThreadPoolExecutor(max_workers=2) as pool:
        results = list(pool.map(create, ["Иваном", "Ольгой"]))
    assert sorted(r["action"] for r in results) == ["clarify", "create"]
    assert client.get("/api/events").json()["total"] == 1


def test_import_1000_idempotent_and_search(client):
    assert client.post("/api/import-demo").json() == {"imported": 1000, "skipped": 0, "total_rows": 1000}
    assert client.post("/api/import-demo").json()["imported"] == 0
    result = client.get("/api/events", params={"q": "контракт Иван", "limit": 200}).json()
    assert result["total"] > 0
    assert any("Иван" in e["title"] for e in result["events"])
    assert client.get("/api/events", params={"start": "2024-01-01", "end": "2025-01-01"}).json()["total"] > 0


def test_import_validation_is_atomic(client, tmp_path):
    bad = tmp_path / "bad.csv"
    bad.write_text("id,date,time,text\n1,2026-10-13,15:00,Встреча с Иваном\n2,2026-02-31,10:00,Задача\n", encoding="utf-8")
    import pytest

    with pytest.raises(ValueError, match="строке 3"):
        client.app.state.database.import_csv(bad, NOW)
    assert client.get("/api/events").json()["total"] == 0


def test_pending_expires(say, settings):
    say("Добавь встречу с Иваном завтра")
    with TestClient(create_app(settings, clock=lambda: NOW + timedelta(minutes=31))) as later:
        r = later.post("/api/chat", json={"message": "в 12", "session_id": say.session}).json()
        assert r["action"] == "help"
        assert not r["pending"]


def test_due_notifications(settings):
    with TestClient(create_app(settings, clock=lambda: NOW)) as c:
        c.post("/api/chat", json={"message": "Напомни сегодня в 12:30 отправить отчёт"})
        c.post("/api/chat", json={"message": "Напомни сегодня в 10:00 отправить договор"})
        c.post("/api/chat", json={"message": "Напомни сегодня в 13:00 купить билеты"})
        assert len(c.get("/api/reminders/due").json()["events"]) == 1


def test_api_validation_and_status(client, say):
    assert client.post("/api/chat", json={"message": " "}).status_code == 422
    assert client.get("/api/events?start=2026-02-31").status_code == 422
    assert client.get("/api/events?start=2026-10-13&end=2026-10-12").status_code == 422
    assert client.get("/api/events?limit=100000").status_code == 422
    assert client.patch("/api/events/9999", json={"status": "done"}).status_code == 404
    event = say("Поставь задачу купить хлеб")["events"][0]
    assert client.patch(f"/api/events/{event['id']}", json={"status": "done"}).json()["status"] == "done"
    assert client.get("/api/events?status=done").json()["total"] == 1
    assert "nosniff" in client.get("/").headers["x-content-type-options"]
