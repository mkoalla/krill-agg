"""Krillion's public API and puzzle-day math, mirroring the bot's
root/info/krillionapi and root/krillion/day.go."""

import json
import urllib.request
import uuid
from dataclasses import dataclass
from datetime import date, datetime, timedelta
from zoneinfo import ZoneInfo

BASE_URL = "https://krillion.io/api"
MIDDLE_SCORE = 350

# puzzles roll over at midnight US eastern (DST-aware); #1 was this date there
RESET_ZONE = ZoneInfo("America/New_York")
DAY_ONE = date(2026, 7, 16)


def day_at(t: datetime) -> int:
    return (t.astimezone(RESET_ZONE).date() - DAY_ONE).days + 1


def today() -> int:
    return day_at(datetime.now(RESET_ZONE))


def date_of(day: int) -> str:
    """'2026-11-16' for #124, the date the api expects"""
    return (DAY_ONE + timedelta(days=day - 1)).isoformat()


@dataclass
class Stats:
    sample: int
    better_than: float  # percent, for the score that was sent
    dist: list[float]  # player counts per 35-point bucket


def complete(day_date: str, score: int, timeout: float = 15) -> Stats:
    """Where score would land on today's puzzle (past dates are refused). A fresh
    random player id and record:false, so the lookup isn't kept as a result."""
    payload = json.dumps({"date": day_date, "playerId": str(uuid.uuid4()), "score": score, "record": False})
    req = urllib.request.Request(
        f"{BASE_URL}/today/complete",
        data=payload.encode(),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        body = json.load(resp)
    return Stats(sample=body["sample"], better_than=body["betterThan"], dist=[float(c) for c in body["dist"]])
