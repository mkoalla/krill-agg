"""/topg's card: a guild's median score day by day against every krillion
player's.

Days run left to right over the bot's first..last, score 0-700 bottom to top,
like /topu's card (see trend.py). Everyone is cyan and the guild yellow: each
day's medians faint, their moving averages over them. The guild's icon rides
the end of its yellow line, and everyone's median over the span is pinned
down to the axis with its score under it.
"""

import io
from dataclasses import dataclass, field
from html import escape

import resvg_py
from PIL import Image

from . import curve, trend
from .card import (
    ABYSS,
    ADVANCE_EM,
    CARD_W,
    CYAN,
    DRIFT,
    FONT,
    FONT_FAMILY,
    FOAM,
    ICON_ROUND,
    LABEL_SIZE,
    LABEL_SPACING_EM,
    PAD,
    SCORE_BASELINE,
    SCORE_GAP,
    SCORE_SIZE,
    SCORE_TOP,
    TITLE_SIZE,
    TITLE_TOP,
    _baseline_for_top,
    drawable,
    _fit,
    _spacing,
    _width,
)
from .players import avatar_uri
from .trend import YELLOW

EVERYONE = "ALL PLAYERS"

ICON_SIZE = 32  # the guild's icon, riding the end of its moving average
FAINT = 0.3  # each day's medians, under the averages
MA_WIDTH = 1.8
END_DOT_R = 2.6
SWATCH = 12  # the legend's squares
SWATCH_GAP = 6  # a square to its name
LEGEND_GAP = 14  # one name to the next square
FOOT_ROW = 30  # the legend's baseline to the line under it


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


def _x(data: MedianData):
    return trend.day_xs(data.first, data.last)


def chart_svg(data: MedianData) -> str:
    x = _x(data)
    parts = trend.grid_svg(data.first, data.last)
    # everyone under the guild, so the guild's line reads on top where they meet
    for s, color in ((data.everyone, CYAN), (data.guild, YELLOW)):
        daily = trend.line_path(x, s.points)
        if daily:
            parts.append(
                f'<path d="{daily}" fill="none" stroke="{color}" stroke-width="1" stroke-opacity="{FAINT}"/>'
            )
        ma = trend.line_path(x, s.ma)
        if ma:
            parts.append(
                f'<path d="{ma}" fill="none" stroke="{color}" stroke-width="{MA_WIDTH}" stroke-opacity="0.95"/>'
            )
    # without an icon, a dot ends the guild's line
    if data.guild.ma and not avatar_uri(data.icon):
        day, value = data.guild.ma[-1]
        parts.append(
            f'<circle cx="{x(day):.3f}" cy="{trend.score_y(value):.3f}" r="{END_DOT_R}" fill="{YELLOW}"/>'
        )
    return "\n  ".join(parts)


def pin_svg(data: MedianData) -> str:
    """everyone's median, pinned from its height at their latest day down to
    the axis, its score under it. drawn over the icon, which shares that day"""
    s = data.everyone
    if s.median is None or not s.points:
        return ""
    px, py = _x(data)(s.points[-1][0]), trend.score_y(s.median)
    label = str(round(s.median))
    lx = trend._on_chart(px, _width(label, trend.MARK_SIZE, trend.MARK_SPACING_EM) / 2)
    return (
        f'<line x1="{px:.3f}" y1="{py:.3f}" x2="{px:.3f}" y2="{trend.BASELINE}" stroke="{CYAN}" stroke-width="1.2"/>'
        f'<circle cx="{px:.3f}" cy="{py:.3f}" r="{END_DOT_R}" fill="{CYAN}"/>'
        f'<text x="{lx:.3f}" y="{trend.MARK_ROW}" text-anchor="middle" font-size="{trend.MARK_SIZE}" '
        f'letter-spacing="{_spacing(trend.MARK_SIZE, trend.MARK_SPACING_EM)}" fill="{CYAN}">{label}</text>'
    )


def icon_svg(data: MedianData, chart_top: float) -> str:
    """the guild's icon centered on the end of its moving average, kept inside the card"""
    uri = avatar_uri(data.icon)
    if not uri or not data.guild.ma:
        return ""
    day, value = data.guild.ma[-1]
    r = ICON_SIZE / 2
    cx = min(max(PAD + _x(data)(day) * trend.CHART_SCALE, r + 2), CARD_W - r - 2)
    cy = chart_top + trend.score_y(value) * trend.CHART_SCALE
    corner = ICON_SIZE * ICON_ROUND
    box = f'x="{cx - r:.2f}" y="{cy - r:.2f}" width="{ICON_SIZE}" height="{ICON_SIZE}" rx="{corner:.2f}"'
    return (
        f'<clipPath id="icon"><rect {box}/></clipPath>'
        f'<image href="{uri}" x="{cx - r:.2f}" y="{cy - r:.2f}" width="{ICON_SIZE}" height="{ICON_SIZE}" '
        f'preserveAspectRatio="xMidYMid slice" clip-path="url(#icon)"/>'
        f'<rect {box} fill="none" stroke="{YELLOW}" stroke-width="2"/>'
    )


def _label_w(text: str) -> float:
    """a spaced label's drawn width: letter-spacing trails every glyph but the last"""
    return max(
        _width(text, LABEL_SIZE, LABEL_SPACING_EM) - LABEL_SIZE * LABEL_SPACING_EM, 0
    )


def legend_svg(data: MedianData, baseline: float) -> str:
    """a square and name for each line: the guild's, then everyone's"""
    spacing = _spacing(LABEL_SIZE, LABEL_SPACING_EM)
    room = (
        CARD_W - 2 * PAD - 2 * (SWATCH + SWATCH_GAP) - LEGEND_GAP - _label_w(EVERYONE)
    )
    # _fit measures unspaced glyphs: a size that advances as far as a spaced one
    name = _fit(
        drawable(data.title).upper(),
        LABEL_SIZE * (1 + LABEL_SPACING_EM / ADVANCE_EM),
        room,
    )
    parts, x = [], float(PAD)
    for text, color in ((name, YELLOW), (EVERYONE, CYAN)):
        parts.append(
            f'<rect x="{x:.2f}" y="{baseline - SWATCH:.2f}" width="{SWATCH}" height="{SWATCH}" fill="{color}"/>'
            f'<text x="{x + SWATCH + SWATCH_GAP:.2f}" y="{baseline:.2f}" font-size="{LABEL_SIZE}" letter-spacing="{spacing}" '
            f'fill="{color}">{escape(text)}</text>'
        )
        x += SWATCH + SWATCH_GAP + _label_w(text) + LEGEND_GAP
    return "".join(parts)


def render_svg(data: MedianData) -> str:
    if data.last < data.first:
        raise ValueError("no days to draw")
    if not data.guild.points:
        raise ValueError("no guild medians to draw")
    days = data.last - data.first + 1
    right = f"{days} DAY{'' if days == 1 else 'S'}"
    label_spacing = _spacing(LABEL_SIZE, LABEL_SPACING_EM)
    moving = "" if not data.window else f"{data.window.upper()} MOVING AVERAGES"

    score_text = "—" if data.guild.median is None else str(round(data.guild.median))
    if data.guild.median is not None and data.everyone.median is not None:
        diff = round(data.guild.median) - round(data.everyone.median)
        label = f"({diff:+d}) VS GLOBAL MEDIAN"
    else:
        label = "GUILD MEDIAN"

    right_w = _width(right, LABEL_SIZE, LABEL_SPACING_EM)
    title = _fit(
        f"{drawable(data.title)} vs Global",
        TITLE_SIZE,
        CARD_W - 2 * PAD - right_w - SCORE_GAP,
    )
    beside_score = PAD + _width(score_text, SCORE_SIZE) + SCORE_GAP

    # room above the chart for the icon riding a 700
    chart_top = SCORE_BASELINE + SCORE_GAP + ICON_SIZE / 2
    chart_bottom = chart_top + trend.CHART_H * trend.CHART_SCALE
    legend_baseline = chart_bottom + trend.FOOT_GAP
    foot_baseline = legend_baseline + FOOT_ROW
    card_h = round(foot_baseline + PAD - 4)
    transform = f"translate({PAD} {chart_top:.2f}) scale({trend.CHART_SCALE:.5f})"

    better = (
        ""
        if data.percentile is None
        else f"BETTER THAN {curve.format_percentile(data.percentile)}%"
    )
    total = "" if not data.everyone.dives else f"{data.everyone.dives:,} TOTAL DIVES"

    return f"""<svg xmlns="http://www.w3.org/2000/svg" width="{CARD_W}" height="{card_h}" viewBox="0 0 {CARD_W} {card_h}" font-family="{FONT_FAMILY}">
  <defs>
    <linearGradient id="bg" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0%" stop-color="#16283c"/>
      <stop offset="100%" stop-color="{ABYSS}"/>
    </linearGradient>
    <filter id="glow" x="-20%" y="-30%" width="140%" height="160%">
      <feGaussianBlur stdDeviation="4"/>
    </filter>
    <filter id="shadow" x="-5%" y="-50%" width="110%" height="200%">
      <feDropShadow dx="0" dy="1" stdDeviation="2" flood-color="{ABYSS}"/>
    </filter>
  </defs>
  <rect width="{CARD_W}" height="{card_h}" fill="url(#bg)"/>

  <text x="{PAD}" y="{_baseline_for_top(TITLE_TOP, TITLE_SIZE):.2f}" font-size="{TITLE_SIZE}" fill="{FOAM}">{escape(title)}</text>
  <text x="{CARD_W - PAD}" y="{_baseline_for_top(TITLE_TOP, LABEL_SIZE):.2f}" text-anchor="end" font-size="{LABEL_SIZE}" letter-spacing="{label_spacing}" fill="{DRIFT}">{right}</text>

  <text x="{PAD}" y="{SCORE_BASELINE:.2f}" font-size="{SCORE_SIZE}" fill="{YELLOW}" opacity="0.35" filter="url(#glow)">{score_text}</text>
  <text x="{PAD}" y="{SCORE_BASELINE:.2f}" font-size="{SCORE_SIZE}" fill="{FOAM}">{score_text}</text>
  <text x="{beside_score:.2f}" y="{_baseline_for_top(SCORE_TOP, LABEL_SIZE):.2f}" font-size="{LABEL_SIZE}" letter-spacing="{label_spacing}" fill="{DRIFT}">{label}</text>
  <text x="{beside_score:.2f}" y="{SCORE_BASELINE:.2f}" font-size="{LABEL_SIZE}" fill="{FOAM}">{escape(moving)}</text>

  <g transform="{transform}">
  {chart_svg(data)}
  </g>
  {icon_svg(data, chart_top)}
  <g transform="{transform}">{pin_svg(data)}</g>

  {legend_svg(data, legend_baseline)}
  <text x="{PAD}" y="{foot_baseline:.2f}" font-size="{LABEL_SIZE}" letter-spacing="{label_spacing}" fill="{CYAN}" filter="url(#shadow)">{better}</text>
  <text x="{CARD_W - PAD}" y="{foot_baseline:.2f}" text-anchor="end" font-size="{LABEL_SIZE}" letter-spacing="{label_spacing}" fill="{CYAN}" filter="url(#shadow)">{total}</text>
</svg>
"""


def render_png(data: MedianData, scale: float = 2) -> bytes:
    png = resvg_py.svg_to_bytes(
        svg_string=render_svg(data),
        font_files=[str(FONT)],
        skip_system_fonts=True,
        zoom=scale,
    )
    return bytes(png)


def render_webp(data: MedianData, scale: float = 2) -> bytes:
    """lossless webp, like card.render_webp"""
    out = io.BytesIO()
    with Image.open(io.BytesIO(render_png(data, scale))) as im:
        im.save(out, format="WEBP", lossless=True, method=6)
    return out.getvalue()
