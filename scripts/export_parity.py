"""Export shared reference scenarios and a Russian inflection lexicon; Python is build/test-only."""

import ast
import csv
import json
import logging
import re
import sqlite3
import tempfile
from datetime import datetime, timedelta
from pathlib import Path
from uuid import NAMESPACE_URL, uuid5
from zoneinfo import ZoneInfo

from fastapi.testclient import TestClient

from app.config import ROOT, Settings
from app.dates import extract
from app.llm import Interpreter
from app.main import create_app
from app.nlp import parse
from app.repository import Database
from app.schemas import ChatRequest
from app.search import CONCEPTS, GROUPS, MORPH, STOP, lemma
from app.service import Assistant

logging.getLogger("httpx").setLevel(logging.WARNING)

NOW = datetime(2026, 10, 12, 12, 30, tzinfo=ZoneInfo("Europe/Moscow"))
OUT = ROOT / "testdata/parity"
OUT.mkdir(parents=True, exist_ok=True)
corpus = {
    "clock": NOW.isoformat(),
    "timezone": "Europe/Moscow",
    "reference_commit": "1b7fcd6",
    "temporal": [],
    "commands": [],
    "scenarios": [],
}
texts = []
tree = ast.parse((ROOT / "tests/test_dates.py").read_text(encoding="utf-8"))
for node in tree.body:
    if not isinstance(node, ast.FunctionDef):
        continue
    for decorator in node.decorator_list:
        if isinstance(decorator, ast.Call) and isinstance(decorator.func, ast.Attribute) and decorator.func.attr == "parametrize":
            for row in ast.literal_eval(decorator.args[1]):
                text = row[0] if isinstance(row, (tuple, list)) else row
                texts.append(text)
                if node.name == "test_assignment_phrases":
                    corpus["commands"].append({"text": text, "expected": parse(text, NOW).model_dump()})
                else:
                    t = extract(text, NOW)
                    corpus["temporal"].append(
                        {
                            "text": text,
                            "expected": {
                                "date": t.date,
                                "time": t.time,
                                "start": t.start,
                                "end": t.end,
                                "duration": t.duration,
                                "error": t.error,
                            },
                        }
                    )


def chat(text, **kwargs):
    return {"method": "POST", "path": "/api/chat", "body": {"message": text}, **kwargs}


def get(path):
    return {"method": "GET", "path": path}


scenarios = {
    "four_actions": [
        chat("В пятницу нужно отправить отчёт"),
        chat("Напомни мне завтра в 10 утра позвонить Ивану"),
        chat("Добавь встречу с командой завтра в 15:00"),
        chat("Что запланировано на эту неделю?"),
        chat("Есть ли у меня встречи завтра?"),
        chat("Найди задачу про отчет"),
    ],
    "dialogue_context": [
        chat("Поставь встречу с Иваном завтра"),
        chat("В 15:00"),
        chat("А перенеси ее на послезавтра"),
        chat("Отметь ее выполненной"),
        get("/api/events?status=done"),
    ],
    "date_title_clarifications": [
        chat("Добавь встречу с Ольгой"),
        chat("завтра"),
        chat("в 11"),
        chat("Поставь задачу"),
        chat("Забрать посылку"),
    ],
    "invalid_time_correction": [chat("Добавь встречу с командой завтра в 25:00"), chat("в 16:00")],
    "invalid_date_correction": [chat("Добавь встречу с Иваном 31.02.2026 в 15:00"), chat("завтра")],
    "missing_reminder_title": [chat("Напомни завтра в 10 утра"), chat("купить билеты")],
    "conflicts_and_adjacent_intervals": [
        chat("Добавь встречу с Иваном завтра в 15:00 на два часа"),
        chat("Добавь встречу с Ольгой завтра в 16:00"),
        get("/api/events"),
        chat("в 17:00"),
        chat("Перенеси встречу с Ольгой на завтра в 15:30"),
        chat("да"),
    ],
    "cancel_and_force": [
        chat("Добавь встречу с Иваном завтра в 15:00"),
        chat("Добавь встречу с Ольгой завтра в 15:30"),
        chat("нет"),
        chat("Добавь встречу с Ольгой завтра в 15:30"),
        chat("да"),
    ],
    "midnight": [chat("Добавь встречу с Иваном сегодня в 23:30 на два часа"), chat("Добавь встречу с Ольгой завтра в 00:30")],
    "morphology_synonyms_typo": [
        chat("Поставь задачу проверить договор с Иваном"),
        chat("Поставь задачу подготовить презентацию"),
        chat("Найди задачу про контракт с Иваном"),
        chat("Найди задачу про презинтацию"),
        chat("Я когда-то ставил задачу по договору, найди ее"),
    ],
    "ambiguity_and_delete": [
        chat("Поставь задачу отправить договор Ивану"),
        chat("Поставь задачу отправить договор Ольге"),
        chat("Удали задачу про договор"),
        chat("#1"),
        get("/api/events"),
        chat("нет"),
        chat("Удали задачу #1"),
        chat("Да, пожалуйста"),
        get("/api/events"),
    ],
    "idempotency": [
        chat("Поставь задачу купить билеты", retry="same"),
        chat("Поставь задачу купить билеты", retry="same"),
        get("/api/events"),
        chat("Поставь задачу купить хлеб", retry="same"),
    ],
    "week_boundary": [
        chat("Добавь встречу с Иваном 18.10.2026 в 23:00"),
        chat("Добавь встречу с Ольгой 19.10.2026 в 10:00"),
        chat("Что запланировано на эту неделю?"),
        chat("Что запланировано на следующей неделе?"),
    ],
    "import_archive_search": [
        {"method": "POST", "path": "/api/import-demo"},
        {"method": "POST", "path": "/api/import-demo"},
        get("/api/events?q=контракт%20Иван&limit=200"),
        get("/api/events?start=2024-01-01&end=2025-01-01&limit=3"),
        chat("Когда у меня была встреча с Иваном?"),
    ],
    "validation_status": [
        {"method": "POST", "path": "/api/chat", "body": {"message": " "}},
        get("/api/events?start=2026-02-31"),
        get("/api/events?start=2026-10-13&end=2026-10-12"),
        get("/api/events?limit=100000"),
        {"method": "PATCH", "path": "/api/events/9999", "body": {"status": "done"}},
        chat("Поставь задачу купить хлеб"),
        {"method": "PATCH", "path": "/api/events/1", "body": {"status": "done"}},
        get("/api/events?status=done"),
    ],
    "due_reminders": [
        chat("Напомни сегодня в 12:30 отправить отчёт"),
        chat("Напомни сегодня в 10:00 отправить договор"),
        chat("Напомни сегодня в 13:00 купить билеты"),
        get("/api/reminders/due"),
    ],
    "restart_context": [
        chat("Добавь встречу с Иваном завтра"),
        chat("в 12", restart=True),
        get("/api/events"),
        get("/api/history/00000000-0000-0000-0000-000000000001"),
        chat("Перенеси ее на завтра", other_session=True),
    ],
    "pending_expiry": [chat("Добавь встречу с Иваном завтра"), chat("в 12", minutes_later=31)],
    "task_mentioning_meeting": [chat("Поставь задачу подготовиться к встрече с Иваном завтра")],
    "period_needs_day": [chat("Поставь задачу отправить отчёт на следующей неделе"), chat("20 октября")],
}
for name, steps in scenarios.items():
    with tempfile.TemporaryDirectory() as tmp:
        settings = Settings(database_path=Path(tmp) / "reference.sqlite3", auto_import=False, nlp_provider="rules")
        clock = [NOW]
        client = TestClient(create_app(settings, clock=lambda: clock[0]))
        client.__enter__()
        try:
            for i, step in enumerate(steps):
                if step.get("restart"):
                    client.__exit__(None, None, None)
                    client = TestClient(create_app(settings, clock=lambda: clock[0]))
                    client.__enter__()
                clock[0] = NOW + timedelta(minutes=step.get("minutes_later", 0))
                if step["path"] == "/api/chat":
                    step["body"].update(
                        session_id="00000000-0000-0000-0000-000000000002"
                        if step.get("other_session")
                        else "00000000-0000-0000-0000-000000000001",
                        request_id=str(uuid5(NAMESPACE_URL, name + ":" + str(step.get("retry", i)))),
                    )
                    texts.append(step["body"]["message"])
                response = client.request(step["method"], step["path"], json=step.get("body"))
                step["status"] = response.status_code
                if response.status_code < 400:
                    step["expected"] = response.json()
                elif isinstance(response.json().get("detail"), str):
                    step["expected_detail"] = response.json()["detail"]
                step.pop("retry", None)
                step.pop("other_session", None)
        finally:
            client.__exit__(None, None, None)
    corpus["scenarios"].append({"name": name, "steps": steps})
(OUT / "corpus.json").write_text(json.dumps(corpus, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

# Synthetic v1 database exercises migration of real Python state JSON and WAL data.
with tempfile.TemporaryDirectory() as tmp:
    path = Path(tmp) / "legacy.sqlite3"
    settings = Settings(database_path=path, auto_import=False, nlp_provider="rules")
    db = Database(path)
    db.import_csv(ROOT / "data/voice_assistant_events_1000.csv", NOW)
    service = Assistant(db, Interpreter(settings))
    for i, message in enumerate(["Поставь задачу проверить договор", "Добавь встречу с Иваном завтра"]):
        service.respond(
            ChatRequest(
                message=message, session_id="00000000-0000-0000-0000-000000000001", request_id=str(uuid5(NAMESPACE_URL, f"legacy-{i}"))
            ),
            NOW,
        )
    fixture = OUT / "legacy_v1.sqlite3"
    if fixture.exists():
        fixture.unlink()
    with sqlite3.connect(path) as source, sqlite3.connect(fixture) as target:
        source.backup(target)
    source.close()
    target.close()

# Export real dictionary-backed inflections, not a lowercase-only substitute.
with (ROOT / "data/voice_assistant_events_1000.csv").open(encoding="utf-8-sig") as stream:
    texts.extend(row["text"] for row in csv.DictReader(stream))
texts.extend(" ".join(group) for group in GROUPS)
words = {word for text in texts for word in re.findall(r"[а-яёa-z0-9]+", text.lower())}
forms = set(words) | STOP
for word in sorted(words):
    if re.search("[а-яё]", word):
        forms.update(form.word for form in MORPH.parse(word)[0].lexeme)
lexicon = {
    "source": "pymorphy3 / OpenCorpora dictionary 2.4.417150.4580142; corpus + CSV vocabulary and inflections",
    "lemmas": {word.replace("ё", "е"): lemma(word) for word in sorted(forms)},
    "concepts": CONCEPTS,
    "stop": sorted(STOP),
}
lexicon_dir = ROOT / "internal/search/data"
lexicon_dir.mkdir(parents=True, exist_ok=True)
(lexicon_dir / "russian.json").write_text(json.dumps(lexicon, ensure_ascii=False, sort_keys=True) + "\n", encoding="utf-8")
print(
    f"Exported {len(corpus['temporal'])} date cases, {len(corpus['commands'])} commands, {len(scenarios)} dialogs, {len(forms)} dictionary forms"
)
