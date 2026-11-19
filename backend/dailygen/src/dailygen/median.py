"""/topg's card: a guild's median score day by day against every krillion
player's (see versus.py).

The guild is the yellow front line, ending in its icon; everyone is the cyan
back one, with their median over the span pinned down to the axis with its
score under it.
"""

from dataclasses import dataclass, field

from . import curve, trend, versus
from .card import CYAN, _spacing, _width, drawable
from .trend import YELLOW
from .versus import END_DOT_R, MA_WIDTH  # noqa: F401 (re-exported)

EVERYONE = "ALL PLAYERS"


@dataclass
class Series:
    points: list[tuple[int, float]] = field(
        default_factory=list
    )  # (day, median), days ascending
    ma: list[tuple[int, float]] = field(default_factory=list)  # (day, moving average)
    median: float | None = None  # of every game in the span; None with no points
    dives: int = 0  # games in the span


@dataclass
class MedianData:
    title: str  # guild name
    first: int  # the span's days
    last: int
    window: str  # the moving averages' span, e.g. "1-WEEK"
    guild: Series
    everyone: Series
    percentile: float | None = (
        None  # everyone's games under the guild's median; None draws no line
    )
    icon: str | None = (
        None  # guild icon path; missing/unreadable draws a yellow dot instead
    )


def pin_svg(data: MedianData) -> str:
    """everyone's median, pinned from its height at their latest day down to
    the axis, its score under it. drawn over the icon, which shares that day"""
    s = data.everyone
    if s.median is None or not s.points:
        return ""
    px = trend.day_xs(data.first, data.last)(s.points[-1][0])
    py = trend.score_y(s.median)
    label = str(round(s.median))
    lx = trend._on_chart(px, _width(label, trend.MARK_SIZE, trend.MARK_SPACING_EM) / 2)
    return (
        f'<line x1="{px:.3f}" y1="{py:.3f}" x2="{px:.3f}" y2="{trend.BASELINE}" stroke="{CYAN}" stroke-width="1.2"/>'
        f'<circle cx="{px:.3f}" cy="{py:.3f}" r="{END_DOT_R}" fill="{CYAN}"/>'
        f'<text x="{lx:.3f}" y="{trend.MARK_ROW}" text-anchor="middle" font-size="{trend.MARK_SIZE}" '
        f'letter-spacing="{_spacing(trend.MARK_SIZE, trend.MARK_SPACING_EM)}" fill="{CYAN}">{label}</text>'
    )


def card(data: MedianData) -> versus.Card:
    if not data.guild.points:
        raise ValueError("no guild medians to draw")
    better = (
        ""
        if data.percentile is None
        else f"BETTER THAN {curve.format_percentile(data.percentile)}%"
    )
    total = "" if not data.everyone.dives else f"{data.everyone.dives:,} TOTAL DIVES"
    return versus.Card(
        title=f"{drawable(data.title)} vs Global",
        first=data.first,
        last=data.last,
        score=versus.score_text(data.guild.median),
        label=versus.diff_label(data.guild.median, data.everyone.median, "GLOBAL MEDIAN", "GUILD MEDIAN"),
        moving=versus.moving_label(data.window),
        front=versus.Line(data.title, YELLOW, data.guild.points, data.guild.ma, data.icon, rounded=True),
        back=versus.Line(EVERYONE, CYAN, data.everyone.points, data.everyone.ma),
        pin=pin_svg(data),
        foot_left=(better, CYAN),
        foot_right=(total, CYAN),
    )


def render_svg(data: MedianData) -> str:
    return versus.render_svg(card(data))


def render_png(data: MedianData, scale: float = 2) -> bytes:
    return versus.render_png(card(data), scale)


def render_webp(data: MedianData, scale: float = 2) -> bytes:
    return versus.render_webp(card(data), scale)
