from typing import Literal
from uuid import UUID, uuid4

from pydantic import BaseModel, ConfigDict, Field, field_validator

Kind = Literal["task", "meeting", "reminder"]
Action = Literal["create", "list", "search", "update", "complete", "delete", "help"]


class Command(BaseModel):
    model_config = ConfigDict(extra="forbid")
    action: Action = "help"
    kind: Kind | None = None
    title: str = ""
    query: str = ""
    date: str | None = None
    time: str | None = None
    period_start: str | None = None
    period_end: str | None = None
    duration_minutes: int = Field(default=60, ge=5, le=1440)
    target_id: int | None = Field(default=None, ge=1)
    error: str | None = None
    provider: str = "rules"


class ChatRequest(BaseModel):
    message: str = Field(min_length=1, max_length=2000)
    session_id: UUID = Field(default_factory=uuid4)
    request_id: UUID = Field(default_factory=uuid4)

    @field_validator("message")
    @classmethod
    def not_blank(cls, value):
        if not value.strip():
            raise ValueError("Сообщение не может быть пустым")
        return value.strip()


class StatusRequest(BaseModel):
    status: Literal["open", "done"]
