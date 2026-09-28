import os
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
DATA = Path(os.environ.get("UNIVERSE_DATA", ROOT / ".data")).resolve()
DATA.mkdir(parents=True, exist_ok=True)
DATABASE_URL = os.environ.get("DATABASE_URL", f"sqlite:///{DATA / 'universe.db'}")
COOKIE_SECURE = os.environ.get("COOKIE_SECURE", "false").lower() == "true"
ENABLE_WORKER = os.environ.get("ENABLE_WORKER", "true").lower() == "true"
MAX_STEPS = 8
