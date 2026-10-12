import re
from datetime import datetime, timedelta

from .dates import extract
from .llm import Interpreter
from .repository import Database
from .schemas import Command
from .search import search

HELP = "Я помогу с задачами, встречами и напоминаниями. Например: «Напомни завтра в 10 утра позвонить Ивану», «В пятницу нужно отправить отчёт», «Что у меня на эту неделю?» или «Найди задачу по договору»."
LABELS = {"task": "Задача", "meeting": "Встреча", "reminder": "Напоминание"}


def describe(event):
    moment = datetime.fromisoformat(event["date"]).strftime("%d.%m.%Y") if event.get("date") else "без срока"
    if event.get("time"):
        moment += " в " + event["time"]
    return f"#{event['id']} «{event['title']}» — {moment}"


class Assistant:
    def __init__(self, database: Database, interpreter: Interpreter):
        self.db, self.interpreter = database, interpreter

    def respond(self, request, now: datetime):
        sid, rid = str(request.session_id), str(request.request_id)
        # Keep external LLM I/O outside the SQLite write transaction.
        command = self.interpreter.interpret(request.message, now)
        with self.db.transaction() as repo:
            cached = repo.cached(sid, rid)
            if cached:
                if cached[0] != request.message:
                    raise ValueError("request_id уже использован для другого сообщения")
                return cached[1]
            state = repo.state(sid)
            pending = state.get("pending")
            if pending and now - datetime.fromisoformat(pending["at"]) > timedelta(minutes=30):
                state.pop("pending", None)
            response = self.execute(repo, state, command, request.message, now)
            response.update(
                session_id=sid, request_id=rid, provider=command.provider, timezone=str(now.tzinfo), pending=bool(state.get("pending"))
            )
            repo.save_state(sid, state, now)
            repo.save_exchange(sid, rid, request.message, response, now)
            return response

    def answer(self, text, action="help", events=None, **kwargs):
        return {"reply": text, "action": action, "events": events or [], **kwargs}

    def ask(self, state, cmd, text, now, expectation="fields", **extra):
        state["pending"] = {"command": cmd.model_dump(), "expectation": expectation, "at": now.isoformat(), **extra}
        return self.answer(text, "clarify", candidates=extra.get("candidates", []))

    def execute(self, repo, state, cmd, message, now):
        low = message.lower().replace("ё", "е").strip(" .!?")
        if low in {"отмена", "отменить", "не надо", "нет", "стоп"}:
            state.pop("pending", None)
            return self.answer("Действие отменено.", "cancel")
        pending = state.get("pending")
        force = False
        if pending:
            previous = Command.model_validate(pending["command"])
            expect = pending["expectation"]
            affirmative = low in {
                "да",
                "подтверждаю",
                "да все равно",
                "все равно",
                "сохранить",
                "удалить",
                "да удали",
            } or bool(re.fullmatch(r"да(?:[, ]+(?:пожалуйста|конечно|удали|сохрани))?", low))
            if expect in {"delete", "conflict"} and affirmative:
                cmd = previous
                force = True
                state.pop("pending", None)
            elif expect == "choose" and (re.fullmatch(r"#?\d+", low) or cmd.target_id):
                number = cmd.target_id or int(low.lstrip("#"))
                candidates = pending["candidates"]
                chosen = next((e for e in candidates if e["id"] == number), None)
                if not chosen:
                    return self.answer("Укажите номер записи из предложенного списка, например #42.", "clarify", candidates=candidates)
                cmd = previous.model_copy(update={"target_id": chosen["id"]})
                state.pop("pending", None)
            elif expect in {"fields", "conflict"} and (
                cmd.action == "help"
                or expect == "fields"
                and not previous.title
                and cmd.action == "create"
                and not re.search(r"\b(?:напомни|добавь|создай|поставь|запиши)\b", low)
            ):
                temporal = extract(message, now)
                if temporal.error:
                    return self.answer(temporal.error, "clarify")
                cmd = previous
                if not cmd.title and expect == "fields" and not temporal.date and not temporal.time:
                    cmd.title = message.strip()
                if temporal.date:
                    cmd.date = temporal.date
                if temporal.time:
                    cmd.time = temporal.time
                state.pop("pending", None)
            elif cmd.action != "help":
                state.pop("pending", None)
            else:
                return self.answer(
                    "Ответьте на уточнение или напишите «отмена». Чтобы сменить действие, сформулируйте новый запрос.",
                    "clarify",
                    candidates=pending.get("candidates", []),
                )
        if cmd.error:
            if cmd.action in {"create", "update"}:
                question = cmd.error
                cmd.error = None
                return self.ask(state, cmd, question, now)
            return self.answer(cmd.error, "clarify")
        if cmd.action == "help":
            return self.answer(HELP)
        if cmd.action == "create":
            if not cmd.title or cmd.title.lower() in {"встреча", "задача", "напоминание"}:
                cmd.title = ""
                return self.ask(state, cmd, "Что нужно сделать или с кем провести встречу?", now)
            if len(cmd.title) > 500:
                return self.answer("Сократите название до 500 символов.", "clarify")
            if cmd.period_start and not cmd.date:
                return self.ask(state, cmd, "На какой день в указанном периоде поставить событие?", now)
            if cmd.kind in {"meeting", "reminder"}:
                if not cmd.date:
                    return self.ask(state, cmd, "На какую дату запланировать событие?", now)
                if not cmd.time:
                    return self.ask(state, cmd, "На какое время поставить событие? Например, в 15:00.", now)
            if cmd.time and not cmd.date:
                return self.ask(state, cmd, "На какую дату поставить задачу с указанным временем?", now)
            values = {
                "kind": cmd.kind or "task",
                "title": cmd.title,
                "date": cmd.date,
                "time": cmd.time,
                "duration_minutes": cmd.duration_minutes,
            }
            conflicts = repo.conflicts(values)
            if conflicts and not force:
                return self.ask(
                    state,
                    cmd,
                    "Это время пересекается с другой встречей: "
                    + "; ".join(describe(e) for e in conflicts[:3])
                    + ". Укажите другое время или ответьте «да», чтобы сохранить пересечение.",
                    now,
                    "conflict",
                    candidates=conflicts,
                )
            event = repo.create(values, now)
            state["last_ids"] = [event["id"]]
            suffix = " Указанная дата уже прошла." if cmd.date and cmd.date < now.date().isoformat() else ""
            saved = "сохранено" if event["kind"] == "reminder" else "сохранена"
            return self.answer(f"{LABELS[event['kind']]} {saved}: {describe(event)}." + suffix, "create", [event], conflicts=conflicts)
        if cmd.action in {"list", "search"}:
            if cmd.action == "search":
                if not cmd.query:
                    return self.answer("По какой теме искать? Например: «Найди задачу про презентацию».", "clarify")
                pool = repo.events(kind=cmd.kind, start=cmd.period_start, end=cmd.period_end)
                events = search(pool, cmd.query, limit=len(pool))
                intro = "Нашёл записи" if events else "Подходящих записей не нашёл. Попробуйте другое имя или тему."
            else:
                events = repo.events(
                    kind=cmd.kind,
                    start=cmd.period_start,
                    end=cmd.period_end,
                    status="open",
                    upcoming=now.date().isoformat() if not cmd.period_start else None,
                )
                intro = f"Запланировано событий: {len(events)}" if events else "На этот период событий нет."
            state["last_ids"] = [e["id"] for e in events[:30]]
            preview = "\n" + "\n".join(describe(e) for e in events[:8]) if events else ""
            tail = f"\nЕщё {min(len(events), 100) - 8} — в списке результатов." if len(events) > 8 else ""
            if len(events) > 100:
                tail += "\nПоказаны первые 100 записей. Уточните период или тему для остальных."
            return self.answer(intro + preview + tail, cmd.action, events[:100], total=len(events))
        # Resolve mutations by explicit ID, natural-language search or dialogue context.
        if cmd.target_id:
            event = repo.get(cmd.target_id)
            candidates = [event] if event else []
        elif cmd.query and cmd.query.lower() not in {"встречу", "встреча", "задачу"}:
            candidates = search(repo.events(kind=cmd.kind), cmd.query, limit=10)
        else:
            candidates = [e for event_id in state.get("last_ids", []) if (e := repo.get(event_id))]
        if not candidates:
            return self.answer("Не нашёл запись для этого действия. Укажите название или номер, например #42.", "clarify")
        if len(candidates) > 1:
            exact = [e for e in candidates if e["title"].lower() == cmd.query.lower()]
            if len(exact) == 1:
                candidates = exact
            else:
                return self.ask(
                    state,
                    cmd,
                    "Уточните номер записи:\n" + "\n".join(describe(e) for e in candidates[:10]),
                    now,
                    "choose",
                    candidates=candidates[:10],
                )
        event = candidates[0]
        cmd.target_id = event["id"]
        if cmd.action == "delete":
            if not force:
                return self.ask(state, cmd, "Удалить " + describe(event) + "? Ответьте «да» или «нет».", now, "delete")
            repo.delete(event["id"])
            state["last_ids"] = []
            return self.answer("Удалено: " + describe(event) + ".", "delete")
        if cmd.action == "complete":
            event = repo.update(event["id"], {"status": "done"}, now)
            state["last_ids"] = [event["id"]]
            return self.answer("Отмечено выполненным: " + describe(event) + ".", "complete", [event])
        if not cmd.date and not cmd.time:
            return self.ask(state, cmd, "На какую дату или время перенести событие?", now)
        values = {**event, "date": cmd.date or event["date"], "time": cmd.time or event["time"]}
        if values["time"] and not values["date"]:
            return self.ask(state, cmd, "Уточните дату переноса.", now)
        conflicts = repo.conflicts(values, exclude=event["id"])
        if conflicts and not force:
            return self.ask(
                state,
                cmd,
                "Перенос пересекается с встречей: "
                + "; ".join(describe(e) for e in conflicts[:3])
                + ". Укажите другое время или ответьте «да».",
                now,
                "conflict",
                candidates=conflicts,
            )
        event = repo.update(event["id"], {"date": values["date"], "time": values["time"]}, now)
        state["last_ids"] = [event["id"]]
        return self.answer("Перенесено: " + describe(event) + ".", "update", [event], conflicts=conflicts)
