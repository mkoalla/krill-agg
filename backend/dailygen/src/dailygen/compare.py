"""/compare's card: two players' scores day by day, or a player's against
their guild's daily average (see versus.py).

The left player is the yellow front line, the right player (or the guild) the
cyan back one, each ending in its avatar (the guild in its icon). The big
number is the left side's average over the span, against the right's.
"""

from dataclasses import dataclass, field

from . import versus
from .card import CYAN, drawable
from .trend import YELLOW


@dataclass
class Side:
    name: str  # display name, or the guild's
    points: list[tuple[int, float]] = field(default_factory=list)  # (day, score or the guild's average), days ascending
    ma: list[tuple[int, float]] = field(default_factory=list)  # (day, moving average)
    average: float | None = None  # of every game in the span; None with none
    dives: int = 0  # games in the span
    avatar: str | None = None  # image path; missing/unreadable draws a dot
    guild: bool = False  # the guild: its icon is a rounded square


@dataclass
class CompareData:
    first: int  # the span's days
    last: int
    window: str  # the moving averages' span, e.g. "1-WEEK"
    left: Side
    right: Side


def _dives(n: int) -> str:
    return f"{n:,} DIVE{'' if n == 1 else 'S'}"


def card(data: CompareData) -> versus.Card:
    left, right = data.left, data.right
    left_name, right_name = drawable(left.name), drawable(right.name)
    return versus.Card(
        title=f"{left_name} vs {right_name}",
        first=data.first,
        last=data.last,
        score=versus.score_text(left.average),
        label=versus.diff_label(left.average, right.average, right_name.upper(), "AVERAGE"),
        moving=versus.moving_label(data.window),
        front=versus.Line(left.name, YELLOW, left.points, left.ma, left.avatar, rounded=left.guild),
        back=versus.Line(right.name, CYAN, right.points, right.ma, right.avatar, rounded=right.guild),
        foot_left=(_dives(left.dives), YELLOW),
        foot_right=(_dives(right.dives), CYAN),
    )


def render_svg(data: CompareData) -> str:
    return versus.render_svg(card(data))


def render_png(data: CompareData, scale: float = 2) -> bytes:
    return versus.render_png(card(data), scale)


def render_webp(data: CompareData, scale: float = 2) -> bytes:
    return versus.render_webp(card(data), scale)
