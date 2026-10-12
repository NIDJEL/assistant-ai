"""python -m scripts.import_events [path.csv]"""

import argparse
from datetime import datetime
from pathlib import Path
from zoneinfo import ZoneInfo

from app.config import ROOT, Settings
from app.repository import Database

parser = argparse.ArgumentParser(description="Atomic, idempotent event CSV import")
parser.add_argument("path", nargs="?", type=Path, default=ROOT / "data/voice_assistant_events_1000.csv")
args = parser.parse_args()
settings = Settings()
print(Database(settings.database_path).import_csv(args.path, datetime.now(ZoneInfo(settings.timezone))))
