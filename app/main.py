import logging
from contextlib import asynccontextmanager
from datetime import date, datetime, timedelta
from typing import Literal
from uuid import UUID
from zoneinfo import ZoneInfo

from fastapi import FastAPI, HTTPException, Query
from fastapi.responses import FileResponse
from fastapi.staticfiles import StaticFiles

from .config import ROOT, Settings
from .llm import Interpreter
from .repository import Database
from .schemas import ChatRequest, Kind, StatusRequest
from .search import search
from .service import Assistant

logging.basicConfig(level=logging.INFO)


def create_app(settings: Settings | None = None, clock=None):
    settings = settings or Settings()
    clock = clock or (lambda: datetime.now(ZoneInfo(settings.timezone)))
    db = Database(settings.database_path)
    assistant = Assistant(db, Interpreter(settings))

    @asynccontextmanager
    async def lifespan(app):
        if settings.auto_import:
            db.import_csv(ROOT / "data/voice_assistant_events_1000.csv", clock())
        yield

    app = FastAPI(title="Ритм — голосовой планировщик", version="1.0.0", lifespan=lifespan, redoc_url=None)
    app.state.database = db

    @app.middleware("http")
    async def security_headers(request, call_next):
        response = await call_next(request)
        response.headers["X-Content-Type-Options"] = "nosniff"
        response.headers["Referrer-Policy"] = "same-origin"
        response.headers["Permissions-Policy"] = "microphone=(self)"
        response.headers["Content-Security-Policy"] = (
            "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'"
        )
        if request.url.path == "/docs":
            response.headers["Content-Security-Policy"] = (
                "default-src 'self'; script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; "
                "style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; img-src 'self' data: https://fastapi.tiangolo.com; "
                "connect-src 'self'; frame-ancestors 'none'"
            )
        return response

    @app.get("/health")
    def health():
        with db.transaction() as repo:
            count = repo.count()
        return {
            "status": "ok",
            "events": count,
            "timezone": settings.timezone,
            "nlp_provider": settings.nlp_provider,
            "now": clock().isoformat(),
        }

    @app.post("/api/chat")
    def chat(request: ChatRequest):
        try:
            return assistant.respond(request, clock())
        except ValueError as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc

    @app.get("/api/history/{session_id}")
    def history(session_id: UUID):
        with db.transaction() as repo:
            return {"exchanges": repo.history(str(session_id)), "state": repo.state(str(session_id))}

    @app.get("/api/events")
    def events(
        start: date | None = None,
        end: date | None = None,
        kind: Kind | None = None,
        status: Literal["open", "done"] | None = None,
        q: str = Query(default="", max_length=200),
        limit: int = Query(default=100, ge=1, le=200),
        offset: int = Query(default=0, ge=0),
    ):
        if start and end and end <= start:
            raise HTTPException(status_code=422, detail="end должен быть позже start (правая граница не включается)")
        with db.transaction() as repo:
            rows = repo.events(kind=kind, start=start.isoformat() if start else None, end=end.isoformat() if end else None, status=status)
        if q:
            rows = search(rows, q, limit=2000)
        return {"events": rows[offset : offset + limit], "total": len(rows), "offset": offset, "limit": limit}

    @app.patch("/api/events/{event_id}")
    def status(event_id: int, body: StatusRequest):
        with db.transaction() as repo:
            event = repo.update(event_id, body.model_dump(), clock())
        if not event:
            raise HTTPException(status_code=404, detail="Запись не найдена")
        return event

    @app.post("/api/import-demo")
    def import_demo():
        return db.import_csv(ROOT / "data/voice_assistant_events_1000.csv", clock())

    @app.get("/api/reminders/due")
    def due_reminders():
        now = clock()
        with db.transaction() as repo:
            reminders = repo.events(
                kind="reminder",
                status="open",
                start=(now.date() - timedelta(days=1)).isoformat(),
                end=(now.date() + timedelta(days=1)).isoformat(),
            )
        due = []
        for event in reminders:
            moment = datetime.fromisoformat(event["date"] + "T" + event["time"]).replace(tzinfo=ZoneInfo(settings.timezone))
            if timedelta(0) <= now - moment <= timedelta(minutes=5):
                due.append(event)
        return {"events": due, "now": now.isoformat()}

    @app.get("/")
    def index():
        return FileResponse(ROOT / "app/static/index.html")

    @app.get("/architecture")
    def architecture():
        return FileResponse(ROOT / "app/static/architecture.html")

    app.mount("/static", StaticFiles(directory=ROOT / "app/static"), name="static")
    app.mount("/diagrams", StaticFiles(directory=ROOT / "docs/diagrams"), name="diagrams")
    return app


app = create_app()
