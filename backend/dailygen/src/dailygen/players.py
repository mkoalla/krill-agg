"""Individual players on the curve: parsing, avatars, and stacking."""

import base64
from dataclasses import dataclass
from pathlib import Path

# what resvg can draw, by leading bytes
_FORMATS = {
    b"\x89PNG\r\n\x1a\n": "image/png",
    b"\xff\xd8\xff": "image/jpeg",
    b"GIF87a": "image/gif",
    b"GIF89a": "image/gif",
}


@dataclass
class Player:
    score: float
    avatar: str | None = None  # image path; missing/unreadable means a blank disc


def parse_user(spec: str) -> Player:
    """'400:path/to/avatar.png'. the path may be empty ('400:') or absent ('400')"""
    score, _, path = spec.partition(":")
    try:
        value = float(score)
    except ValueError:
        raise ValueError(f"bad --user {spec!r}: score must be a number") from None
    return Player(score=value, avatar=path or None)


def avatar_uri(path: str | None) -> str | None:
    """the image as a data uri, or None if it can't be used"""
    if not path:
        return None
    try:
        raw = Path(path).read_bytes()
    except OSError:
        return None
    mime = next((m for magic, m in _FORMATS.items() if raw.startswith(magic)), None)
    if mime is None and raw[:4] == b"RIFF" and raw[8:12] == b"WEBP":
        mime = "image/webp"
    if mime is None:
        return None
    return f"data:{mime};base64,{base64.b64encode(raw).decode()}"


def stack(xs: list[float], size: float, max_push: int) -> list[int]:
    """Level (0 = resting on the curve) for each x, in order. A circle that
    overlaps one already on a level by more than half its width goes up a level;
    past max_push levels it wraps back down (modulo) instead of growing forever."""
    max_push = max(1, max_push)
    placed: list[tuple[float, int]] = []
    levels = []
    for x in xs:
        clashes = [level for px, level in placed if abs(px - x) < size / 2]
        level = next((L for L in range(max_push) if L not in clashes), len(clashes) % max_push)
        placed.append((x, level))
        levels.append(level)
    return levels
