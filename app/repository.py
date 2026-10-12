import csv
import json
import re
import sqlite3
from contextlib import contextmanager
from datetime import date, datetime, time, timedelta
from pathlib import Path

SCHEMA = """
CREATE TABLE IF NOT EXISTS events (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 kind TEXT NOT NULL CHECK(kind IN ('task','meeting','reminder')),
 title TEXT NOT NULL CHECK(length(title) BETWEEN 1 AND 500),
 date TEXT, time TEXT,
 duration_minutes INTEGER NOT NULL DEFAULT 60 CHECK(duration_minutes BETWEEN 5 AND 1440),
 status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open','done')),
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
 source_key TEXT UNIQUE
);
CREATE INDEX IF NOT EXISTS events_period ON events(date, time);
CREATE INDEX IF NOT EXISTS events_kind_status ON events(kind, status);
CREATE TABLE IF NOT EXISTS sessions (
 id TEXT PRIMARY KEY, state TEXT NOT NULL, updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS exchanges (
 id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL,
 request_id TEXT NOT NULL, message TEXT NOT NULL, response TEXT NOT NULL,
 created_at TEXT NOT NULL, UNIQUE(session_id, request_id)
);
CREATE INDEX IF NOT EXISTS exchanges_session ON exchanges(session_id, id);
PRAGMA user_version=1;
"""


def validate_event(values):
    if values.get("date"):
        date.fromisoformat(values["date"])
    if values.get("time"):
        time.fromisoformat(values["time"])
    if values.get("kind") in {"meeting", "reminder"} and (not values.get("date") or not values.get("time")):
        raise ValueError("Для встречи и напоминания нужны дата и время")
    if not values.get("title", "").strip() or len(values["title"]) > 500:
        raise ValueError("Название должно содержать от 1 до 500 символов")


class Database:
    def __init__(self, path: Path):
        self.path = Path(path)
        self.path.parent.mkdir(parents=True, exist_ok=True)
        with self.connect() as conn:
            conn.execute("PRAGMA journal_mode=WAL")
            conn.executescript(SCHEMA)

    def connect(self):
        conn = sqlite3.connect(self.path, timeout=30)
        conn.row_factory = sqlite3.Row
        conn.execute("PRAGMA foreign_keys=ON")
        conn.execute("PRAGMA busy_timeout=30000")
        return conn

    @contextmanager
    def transaction(self):
        conn = self.connect()
        try:
            # Conflict checks, session state and event mutation form one atomic unit.
            conn.execute("BEGIN IMMEDIATE")
            yield Repository(conn)
            conn.commit()
        except Exception:
            conn.rollback()
            raise
        finally:
            conn.close()

    def import_csv(self, path: Path, now: datetime):
        prepared = []
        with Path(path).open(encoding="utf-8-sig", newline="") as stream:
            reader = csv.DictReader(stream)
            if not {"id", "date", "time", "text"} <= set(reader.fieldnames or []):
                raise ValueError("CSV должен содержать id,date,time,text")
            seen = set()
            for line, row in enumerate(reader, 2):
                try:
                    source_id = int(row["id"])
                    if source_id < 1 or source_id in seen:
                        raise ValueError("Некорректный или повторный id")
                    seen.add(source_id)
                    title = row["text"].strip()
                    kind = (
                        "meeting"
                        if re.match(r"(?i)^(созвон|встреча|демо|совещание|интервью)\b", title)
                        else "reminder"
                        if title.lower().startswith("напомнить")
                        else "task"
                    )
                    values = {"kind": kind, "title": title, "date": row["date"], "time": row["time"], "duration_minutes": 60}
                    validate_event(values)
                    prepared.append((values, f"csv:{Path(path).name}:{source_id}"))
                except (ValueError, KeyError) as exc:
                    raise ValueError(f"Ошибка CSV в строке {line}: {exc}") from exc
        with self.transaction() as repo:
            before = repo.count()
            for values, key in prepared:
                repo.create(values, now, key)
            imported = repo.count() - before
        return {"imported": imported, "skipped": len(prepared) - imported, "total_rows": len(prepared)}


class Repository:
    def __init__(self, conn):
        self.conn = conn

    def count(self):
        return self.conn.execute("SELECT count(*) FROM events").fetchone()[0]

    def get(self, event_id):
        row = self.conn.execute("SELECT * FROM events WHERE id=?", (event_id,)).fetchone()
        return dict(row) if row else None

    def events(self, kind=None, start=None, end=None, status=None, upcoming=None):
        clauses, args = [], []
        for key, value in (("kind", kind), ("status", status)):
            if value:
                clauses.append(f"{key}=?")
                args.append(value)
        if start:
            clauses.append("date>=?")
            args.append(start)
        if end:
            clauses.append("date<?")
            args.append(end)
        if upcoming:
            clauses.append("(date>=? OR date IS NULL)")
            args.append(upcoming)
        sql = "SELECT * FROM events" + (" WHERE " + " AND ".join(clauses) if clauses else "") + " ORDER BY date IS NULL,date,time,id"
        return [dict(row) for row in self.conn.execute(sql, args)]

    def create(self, values, now, source_key=None):
        validate_event(values)
        cursor = self.conn.execute(
            "INSERT INTO events(kind,title,date,time,duration_minutes,created_at,updated_at,source_key) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(source_key) DO NOTHING",
            (
                values["kind"],
                values["title"],
                values.get("date"),
                values.get("time"),
                values.get("duration_minutes", 60),
                now.isoformat(),
                now.isoformat(),
                source_key,
            ),
        )
        return self.get(cursor.lastrowid) if cursor.rowcount else None

    def update(self, event_id, values, now):
        old = self.get(event_id)
        if not old:
            return None
        merged = {**old, **values}
        validate_event(merged)
        allowed = {k: v for k, v in values.items() if k in {"title", "date", "time", "duration_minutes", "status"}}
        allowed["updated_at"] = now.isoformat()
        self.conn.execute("UPDATE events SET " + ",".join(f"{key}=?" for key in allowed) + " WHERE id=?", [*allowed.values(), event_id])
        return self.get(event_id)

    def delete(self, event_id):
        self.conn.execute("DELETE FROM events WHERE id=?", (event_id,))

    def conflicts(self, values, exclude=None):
        if values["kind"] != "meeting" or not values.get("date") or not values.get("time"):
            return []
        start = datetime.fromisoformat(values["date"] + "T" + values["time"])
        end = start + timedelta(minutes=values.get("duration_minutes", 60))
        meetings = self.events(
            kind="meeting",
            status="open",
            start=(start.date() - timedelta(days=1)).isoformat(),
            end=(end.date() + timedelta(days=1)).isoformat(),
        )
        matches = []
        for event in meetings:
            if event["id"] == exclude:
                continue
            other_start = datetime.fromisoformat(event["date"] + "T" + event["time"])
            other_end = other_start + timedelta(minutes=event["duration_minutes"])
            if start < other_end and other_start < end:
                matches.append(event)
        return matches

    def state(self, session_id):
        row = self.conn.execute("SELECT state FROM sessions WHERE id=?", (session_id,)).fetchone()
        return json.loads(row[0]) if row else {}

    def save_state(self, session_id, state, now):
        self.conn.execute(
            "INSERT INTO sessions VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET state=excluded.state,updated_at=excluded.updated_at",
            (session_id, json.dumps(state, ensure_ascii=False), now.isoformat()),
        )

    def cached(self, session_id, request_id):
        row = self.conn.execute(
            "SELECT message,response FROM exchanges WHERE session_id=? AND request_id=?", (session_id, request_id)
        ).fetchone()
        return (row["message"], json.loads(row["response"])) if row else None

    def save_exchange(self, session_id, request_id, message, response, now):
        self.conn.execute(
            "INSERT INTO exchanges(session_id,request_id,message,response,created_at) VALUES(?,?,?,?,?)",
            (session_id, request_id, message, json.dumps(response, ensure_ascii=False), now.isoformat()),
        )

    def history(self, session_id, limit=50):
        rows = self.conn.execute(
            "SELECT message,response,created_at FROM exchanges WHERE session_id=? ORDER BY id DESC LIMIT ?", (session_id, limit)
        ).fetchall()
        return [{"message": r["message"], "response": json.loads(r["response"]), "created_at": r["created_at"]} for r in reversed(rows)]
