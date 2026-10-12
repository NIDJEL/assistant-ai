from datetime import datetime
from uuid import uuid4
from zoneinfo import ZoneInfo

import pytest
from fastapi.testclient import TestClient

from app.config import Settings
from app.main import create_app

NOW = datetime(2026, 10, 12, 12, 30, tzinfo=ZoneInfo("Europe/Moscow"))


@pytest.fixture
def settings(tmp_path):
    return Settings(database_path=tmp_path / "test.sqlite3", auto_import=False, nlp_provider="rules")


@pytest.fixture
def client(settings):
    with TestClient(create_app(settings, clock=lambda: NOW)) as app_client:
        yield app_client


@pytest.fixture
def say(client):
    session = str(uuid4())

    def send(message, request_id=None, session_id=None):
        response = client.post(
            "/api/chat", json={"message": message, "session_id": session_id or session, "request_id": request_id or str(uuid4())}
        )
        assert response.status_code == 200, response.text
        return response.json()

    send.session = session
    return send
