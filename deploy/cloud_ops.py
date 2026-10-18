#!/usr/bin/env python3
"""Install six OAuth settings from private stdin into root-only production.env.

Run on the VPS as root after creating the release checkpoint. Never pass values
through command-line arguments, output, source control, or docker compose config.
"""
import json
import os
from pathlib import Path
import re
import tempfile


def main():
    if os.geteuid() != 0:
        raise SystemExit("root required")
    import sys
    data = json.load(sys.stdin)
    keys = {"GOOGLE_CALENDAR_CLIENT_ID", "GOOGLE_CALENDAR_CLIENT_SECRET", "GOOGLE_CALENDAR_REDIRECT_URI", "ZOOM_CLIENT_ID", "ZOOM_CLIENT_SECRET", "ZOOM_REDIRECT_URI"}
    if set(data) != keys or any(not isinstance(v, str) or not v or len(v) > 4096 or "\n" in v or "\r" in v for v in data.values()):
        raise SystemExit("invalid OAuth configuration")
    for name, value in data.items():
        if name.endswith("REDIRECT_URI"):
            provider = "google" if name.startswith("GOOGLE") else "zoom"
            if value != f"https://meinangelplatz.top/api/auth/{provider}/callback":
                raise SystemExit("unexpected redirect origin or callback")
        elif not re.fullmatch(r"[A-Za-z0-9._~+/=-]+", value):
            raise SystemExit("invalid credential encoding")
    path = Path("/etc/rhythm/production.env")
    content = path.read_text()
    lines = [line for line in content.splitlines() if line.split("=", 1)[0].strip() not in keys]
    lines.extend(f"{key}={json.dumps(data[key])}" for key in sorted(keys))
    fd, temporary = tempfile.mkstemp(prefix=".oauth-", dir=path.parent)
    try:
        os.fchmod(fd, 0o600)
        with os.fdopen(fd, "w") as output:
            output.write("\n".join(lines) + "\n")
            output.flush()
            os.fsync(output.fileno())
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)
    print("OAuth settings installed; secret values suppressed")


if __name__ == "__main__":
    main()
