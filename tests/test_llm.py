import json
from unittest.mock import patch

import httpx

from app.llm import Interpreter
from app.schemas import Command
from tests.conftest import NOW


def test_ollama_failure_falls_back(settings):
    settings.nlp_provider = "ollama"
    with patch("httpx.Client.post", side_effect=httpx.ConnectError("offline")):
        result = Interpreter(settings).interpret("Напомни завтра в 10 утра позвонить Ивану", NOW)
    assert result.provider == "rules_fallback"
    assert result.action == "create"
    assert result.time == "10:00"


def test_ollama_validates_schema_and_dates(settings):
    settings.nlp_provider = "ollama"
    for content in [
        "not json",
        json.dumps({"action": "execute_sql"}),
        json.dumps(Command(action="create", kind="task", title="Договор", date="2026-02-31").model_dump()),
    ]:
        response = httpx.Response(200, request=httpx.Request("POST", "http://localhost/api/chat"), json={"message": {"content": content}})
        with patch("httpx.Client.post", return_value=response):
            assert Interpreter(settings).interpret("Добавь задачу проверить договор", NOW).provider == "rules_fallback"


def test_ollama_explicit_dates_override_model(settings):
    settings.nlp_provider = "ollama"
    cmd = Command(action="create", kind="meeting", title="Встреча с Иваном", date="2026-10-14", time="11:00")
    response = httpx.Response(
        200, request=httpx.Request("POST", "http://localhost/api/chat"), json={"message": {"content": cmd.model_dump_json()}}
    )
    with patch("httpx.Client.post", return_value=response):
        result = Interpreter(settings).interpret("Добавь встречу с Иваном завтра в 10 утра", NOW)
    assert (result.date, result.time, result.provider) == ("2026-10-13", "10:00", "ollama")
