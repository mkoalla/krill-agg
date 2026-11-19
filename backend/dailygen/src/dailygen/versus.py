"""Two lines over the same days, one against the other: /topg (a guild against
every krillion player) and /compare (a player against a player, or against
their guild).

Days run left to right over first..last, score 0-700 bottom to top, like
/topu's card (see trend.py). Each line is its daily values drawn faint with
its moving average over them, and ends in its icon (a dot without one). The
front line is yellow and drawn on top, the back one cyan; the legend names
them in that order.
"""

import io
from dataclasses import dataclass, field
from html import escape

import resvg_py
from PIL import Image

from . import trend
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
    _fit,
    _spacing,
    _width,
    drawable,
)
from .players import avatar_uri
from .trend import YELLOW

FRONT, BACK = YELLOW, CYAN

ICON_SIZE = 32  # an icon riding the end of its moving average
FAINT = 0.3  # each day's values, under the averages
MA_WIDTH = 1.8
END_DOT_R = 2.6
SWATCH = 12  # the legend's squares
SWATCH_GAP = 6  # a square to its name
LEGEND_GAP = 14  # one name to the next square
FOOT_ROW = 30  # the legend's baseline to the line under it


@dataclass
class Line:
    name: str  # for the legend
    color: str
    points: list[tuple[int, float]] = field(default_factory=list)  # (day, value), days ascending
    ma: list[tuple[int, float]] = field(default_factory=list)  # (day, moving average)
    icon: str | None = None  # image path; missing/unreadable draws a dot instead
    rounded: bool = False  # a guild's icon: a rounded square, not a circle


@dataclass
class Card:
    title: str
    first: int  # the span's days
    last: int
    score: str  # the big number
    label: str  # beside it, above
    moving: str  # beside it, below
    front: Line
    back: Line
    pin: str = ""  # extra chart svg, drawn over the icons
    foot_left: tuple[str, str] = ("", CYAN)  # (text, color) under the legend
    foot_right: tuple[str, str] = ("", CYAN)


def lines_svg(card: Card) -> str:
    x = trend.day_xs(card.first, card.last)
    parts = trend.grid_svg(card.first, card.last)
    # the back line first, so the front one reads on top where they meet
    for line in (card.back, card.front):
        daily = trend.line_path(x, line.points)
        if daily:
            parts.append(
                f'<path d="{daily}" fill="none" stroke="{line.color}" stroke-width="1" stroke-opacity="{FAINT}"/>'
            )
        ma = trend.line_path(x, line.ma)
        if ma:
            parts.append(
                f'<path d="{ma}" fill="none" stroke="{line.color}" stroke-width="{MA_WIDTH}" stroke-opacity="0.95"/>'
            )
        # without an icon, a dot ends the line
        if line.ma and not avatar_uri(line.icon):
            day, value = line.ma[-1]
            parts.append(
                f'<circle cx="{x(day):.3f}" cy="{trend.score_y(value):.3f}" r="{END_DOT_R}" fill="{line.color}"/>'
            )
    return "\n  ".join(parts)


def icon_svg(card: Card, line: Line, chart_top: float, clip: str) -> str:
    """line's icon centered on the end of its moving average, kept inside the
    card and ringed in its color. clip names its clip path, unique per card"""
    uri = avatar_uri(line.icon)
    if not uri or not line.ma:
        return ""
    day, value = line.ma[-1]
    r = ICON_SIZE / 2
    x = trend.day_xs(card.first, card.last)(day)
    cx = min(max(PAD + x * trend.CHART_SCALE, r + 2), CARD_W - r - 2)
    cy = chart_top + trend.score_y(value) * trend.CHART_SCALE
    corner = ICON_SIZE * ICON_ROUND if line.rounded else r
    box = f'x="{cx - r:.2f}" y="{cy - r:.2f}" width="{ICON_SIZE}" height="{ICON_SIZE}" rx="{corner:.2f}"'
    return (
        f'<clipPath id="{clip}"><rect {box}/></clipPath>'
        f'<image href="{uri}" x="{cx - r:.2f}" y="{cy - r:.2f}" width="{ICON_SIZE}" height="{ICON_SIZE}" '
        f'preserveAspectRatio="xMidYMid slice" clip-path="url(#{clip})"/>'
        f'<rect {box} fill="none" stroke="{line.color}" stroke-width="2"/>'
    )


def _label_w(text: str) -> float:
    """a spaced label's drawn width: letter-spacing trails every glyph but the last"""
    return max(_width(text, LABEL_SIZE, LABEL_SPACING_EM) - LABEL_SIZE * LABEL_SPACING_EM, 0)


def _fit_label(text: str, room: float) -> str:
    # _fit measures unspaced glyphs: a size that advances as far as a spaced one
    return _fit(text, LABEL_SIZE * (1 + LABEL_SPACING_EM / ADVANCE_EM), room)


def legend_svg(card: Card, baseline: float) -> str:
    """a square and name for each line, the front one first. a name gets half
    the room, or more when the other needs less"""
    spacing = _spacing(LABEL_SIZE, LABEL_SPACING_EM)
    room = CARD_W - 2 * PAD - 2 * (SWATCH + SWATCH_GAP) - LEGEND_GAP
    names = [drawable(line.name).upper() for line in (card.front, card.back)]
    names = [_fit_label(n, room - min(_label_w(other), room / 2)) for n, other in zip(names, reversed(names))]
    parts, x = [], float(PAD)
    for text, line in zip(names, (card.front, card.back)):
        parts.append(
            f'<rect x="{x:.2f}" y="{baseline - SWATCH:.2f}" width="{SWATCH}" height="{SWATCH}" fill="{line.color}"/>'
            f'<text x="{x + SWATCH + SWATCH_GAP:.2f}" y="{baseline:.2f}" font-size="{LABEL_SIZE}" letter-spacing="{spacing}" '
            f'fill="{line.color}">{escape(text)}</text>'
        )
        x += SWATCH + SWATCH_GAP + _label_w(text) + LEGEND_GAP
    return "".join(parts)


def days_label(card: Card) -> str:
    days = card.last - card.first + 1
    return f"{days} DAY{'' if days == 1 else 'S'}"


def render_svg(card: Card) -> str:
    if card.last < card.first:
        raise ValueError("no days to draw")
    if not (card.front.points or card.back.points):
        raise ValueError("nothing to draw")
    right = days_label(card)
    label_spacing = _spacing(LABEL_SIZE, LABEL_SPACING_EM)
    right_w = _width(right, LABEL_SIZE, LABEL_SPACING_EM)
    title = _fit(card.title, TITLE_SIZE, CARD_W - 2 * PAD - right_w - SCORE_GAP)
    beside_score = PAD + _width(card.score, SCORE_SIZE) + SCORE_GAP
    label = _fit_label(card.label, CARD_W - PAD - beside_score)

    # room above the chart for an icon riding a 700
    chart_top = SCORE_BASELINE + SCORE_GAP + ICON_SIZE / 2
    chart_bottom = chart_top + trend.CHART_H * trend.CHART_SCALE
    legend_baseline = chart_bottom + trend.FOOT_GAP
    foot_baseline = legend_baseline + FOOT_ROW
    card_h = round(foot_baseline + PAD - 4)
    transform = f"translate({PAD} {chart_top:.2f}) scale({trend.CHART_SCALE:.5f})"
    (left_text, left_color), (right_text, right_color) = card.foot_left, card.foot_right
    # the back line's icon under the front's
    icons = icon_svg(card, card.back, chart_top, "icon-back") + icon_svg(card, card.front, chart_top, "icon-front")

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

  <text x="{PAD}" y="{SCORE_BASELINE:.2f}" font-size="{SCORE_SIZE}" fill="{card.front.color}" opacity="0.35" filter="url(#glow)">{escape(card.score)}</text>
  <text x="{PAD}" y="{SCORE_BASELINE:.2f}" font-size="{SCORE_SIZE}" fill="{FOAM}">{escape(card.score)}</text>
  <text x="{beside_score:.2f}" y="{_baseline_for_top(SCORE_TOP, LABEL_SIZE):.2f}" font-size="{LABEL_SIZE}" letter-spacing="{label_spacing}" fill="{DRIFT}">{escape(label)}</text>
  <text x="{beside_score:.2f}" y="{SCORE_BASELINE:.2f}" font-size="{LABEL_SIZE}" fill="{FOAM}">{escape(card.moving)}</text>

  <g transform="{transform}">
  {lines_svg(card)}
  </g>
  {icons}
  <g transform="{transform}">{card.pin}</g>

  {legend_svg(card, legend_baseline)}
  <text x="{PAD}" y="{foot_baseline:.2f}" font-size="{LABEL_SIZE}" letter-spacing="{label_spacing}" fill="{left_color}" filter="url(#shadow)">{escape(left_text)}</text>
  <text x="{CARD_W - PAD}" y="{foot_baseline:.2f}" text-anchor="end" font-size="{LABEL_SIZE}" letter-spacing="{label_spacing}" fill="{right_color}" filter="url(#shadow)">{escape(right_text)}</text>
</svg>
"""


def render_png(card: Card, scale: float = 2) -> bytes:
    png = resvg_py.svg_to_bytes(
        svg_string=render_svg(card),
        font_files=[str(FONT)],
        skip_system_fonts=True,
        zoom=scale,
    )
    return bytes(png)


def render_webp(card: Card, scale: float = 2) -> bytes:
    """lossless webp, like card.render_webp"""
    out = io.BytesIO()
    with Image.open(io.BytesIO(render_png(card, scale))) as im:
        im.save(out, format="WEBP", lossless=True, method=6)
    return out.getvalue()


def moving_label(window: str) -> str:
    return "" if not window else f"{window.upper()} MOVING AVERAGES"


def diff_label(front: float | None, back: float | None, what: str, fallback: str) -> str:
    """"(+31) VS <what>": the front's value against the back's, or fallback
    without both"""
    if front is None or back is None:
        return fallback
    return f"({round(front) - round(back):+d}) VS {what}"


def score_text(value: float | None) -> str:
    return "—" if value is None else str(round(value))
