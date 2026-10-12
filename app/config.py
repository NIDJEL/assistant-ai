import os
from dataclasses import dataclass, field
from pathlib import Path
from zoneinfo import ZoneInfo

from dotenv import load_dotenv

ROOT = Path(__file__).resolve().parent.parent
load_dotenv(ROOT / ".env")


@dataclass
class Settings:
    database_path: Path = field(default_factory=lambda: Path(os.getenv("DATABASE_PATH", str(ROOT / "var/assistant.sqlite3"))))
    timezone: str = field(default_factory=lambda: os.getenv("APP_TIMEZONE", "Europe/Moscow"))
    auto_import: bool = field(default_factory=lambda: os.getenv("AUTO_IMPORT_DEMO", "true").lower() == "true")
    nlp_provider: str = field(default_factory=lambda: os.getenv("NLP_PROVIDER", "rules"))
    ollama_url: str = field(default_factory=lambda: os.getenv("OLLAMA_BASE_URL", "http://127.0.0.1:11434"))
    ollama_model: str = field(default_factory=lambda: os.getenv("OLLAMA_MODEL", "qwen2.5:7b"))
    ollama_timeout: float = field(default_factory=lambda: float(os.getenv("OLLAMA_TIMEOUT", "20")))

    def __post_init__(self):
        ZoneInfo(self.timezone)
        if self.nlp_provider not in {"rules", "ollama"}:
            raise ValueError("NLP_PROVIDER must be rules or ollama")
