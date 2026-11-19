"""One player's scores over a period (/topu), dressed like the daily card.

Days run left to right from their first game to their latest, score 0-700
bottom to top. The player is yellow: every game drawn faint, best and worst
pinned, and the moving average the bot worked out over them, with their
avatar riding its end like the head of a snake. Every krillion player is
cyan: each day's median faint, its moving average over it.
"""

import io
from dataclasses import dataclass, field
from html import escape

import resvg_py
from PIL import Image

from . import curve
from .card import (
    ABYSS,
    ADVANCE_EM,
    BLANK,
    CARD_W,
    CYAN,
    DRIFT,
    FOAM,
    FONT,
    FONT_FAMILY,
    HAIRLINE,
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

YELLOW = "#ffd166"

# the chart's own space, like curve.py's but taller: 0-700 runs from BASELINE
# up to TOP. plotted days stay LEFT..RIGHT so end dots aren't cut off
WIDTH = curve.WIDTH
TOP = 6.0
BASELINE = 176.0
LEFT, RIGHT = 4.0, WIDTH - 4.0
# label baselines under the axis, low enough that an avatar riding a 0 (half
# of user_size below the axis) never covers them
DAY_ROW = BASELINE + 16  # the evenly spaced day labels
MARK_ROW = BASELINE + 28  # best/worst "score (#day)" labels, under those
CHART_H = MARK_ROW + 4
CHART_SCALE = (CARD_W - 2 * PAD) / WIDTH
MAX_DAY_TICKS = 5
MARK_PAD = 6  # least space between best's and worst's labels
MARK_SIZE = 9
MARK_SPACING_EM = 0.12
LABEL_INSET = 11  # half a 4-digit day label: centered labels keep this far in

DOT_R = 1.6
MA_WIDTH = 1.4
AVG_WIDTH = 1.8  # a moving average, over its faint daily values
FAINT = 0.3  # daily values, under their averages
FOOT_GAP = 30  # chart bottom to the footer's baseline
SWATCH = 12  # the legend's squares
SWATCH_GAP = 6  # a square to its name
LEGEND_GAP = 14  # one name to the next square
FOOT_ROW = 30  # the legend's baseline to the line under it
EVERYONE = "ALL PLAYERS"


@dataclass
class TrendData:
    title: str  # the player's server nickname
    guild: str  # guild name, for the rank line
    points: list[tuple[int, float]]  # (day, score), one per game, days ascending
    ma: list[tuple[int, float]]  # (day, moving average), days ascending
    window: str  # the moving average's span, e.g. "1-WEEK"
    rank: int | None = None  # on the guild's best-score board; None draws no rank line
    everyone: list[tuple[int, float]] = field(default_factory=list)  # (day, every player's median), days ascending
    everyone_ma: list[tuple[int, float]] = field(default_factory=list)  # (day, its moving average)
    avatar: str | None = None  # image path; missing/unreadable means a blank disc
    user_size: float = 32  # avatar diameter

    def best(self) -> tuple[int, float]:
        """highest score, earliest day on ties (like the bot's boards)"""
        return min(self.points, key=lambda p: (-p[1], p[0]))

    def worst(self) -> tuple[int, float]:
        return min(self.points, key=lambda p: (p[1], p[0]))


def score_y(score: float) -> float:
    return BASELINE - min(max(score, 0), curve.MAX_DAY_SCORE) / curve.MAX_DAY_SCORE * (BASELINE - TOP)


def day_xs(first: int, last: int):
    """day -> x, first day at LEFT and the latest at RIGHT. one day sits centered"""
    if last == first:
        return lambda day: WIDTH / 2
    return lambda day: LEFT + (day - first) / (last - first) * (RIGHT - LEFT)


def day_ticks(first: int, last: int) -> list[int]:
    """up to MAX_DAY_TICKS days spread evenly from first to last, ends included"""
    n = min(last - first + 1, MAX_DAY_TICKS)
    if n <= 1:
        return [first]
    return sorted({round(first + k * (last - first) / (n - 1)) for k in range(n)})


def _on_chart(x: float, half: float = LABEL_INSET) -> float:
    """a centered label's x (half its width given), kept from running off either side"""
    return min(max(x, half), WIDTH - half)


def _apart(a: float, b: float, wa: float, wb: float) -> tuple[float, float]:
    """two centered labels' xs (widths wa, wb), kept on the chart and at least
    MARK_PAD apart: the left one is held back and the right one pushed on"""
    flip = b < a
    (lx, lw), (rx, rw) = ((b, wb), (a, wa)) if flip else ((a, wa), (b, wb))
    gap = (lw + rw) / 2 + MARK_PAD
    left = max(lx, lw / 2)
    right = min(max(rx, left + gap), WIDTH - rw / 2)
    left = min(left, right - gap)
    return (right, left) if flip else (left, right)


def grid_svg(first: int, last: int) -> list[str]:
    """the score hairlines and the day axis for days first..last"""
    x = day_xs(first, last)
    parts = []
    # score grid: hairlines with their value just above, at the left edge
    for value in range(0, curve.MAX_DAY_SCORE + 1, 100):
        y = score_y(value)
        stroke = HAIRLINE if value else DRIFT
        opacity = "" if value else ' stroke-opacity="0.5"'
        parts.append(f'<line x1="0" y1="{y:.3f}" x2="{WIDTH}" y2="{y:.3f}" stroke="{stroke}"{opacity}/>')
        parts.append(
            f'<text x="0" y="{y - 2:.3f}" font-size="7" fill="{DRIFT}" opacity="0.55">{value}</text>'
        )

    # the day axis
    for day in day_ticks(first, last):
        tx = x(day)
        parts.append(
            f'<line x1="{tx:.3f}" y1="{BASELINE}" x2="{tx:.3f}" y2="{BASELINE + 3.5}" stroke="{DRIFT}" stroke-opacity="0.5"/>'
            f'<text x="{_on_chart(tx):.3f}" y="{DAY_ROW}" text-anchor="middle" font-size="9" fill="{DRIFT}" opacity="0.55">#{day}</text>'
        )
    return parts


def games_svg(x, points: list[tuple[int, float]], ma: list[tuple[int, float]]) -> list[str]:
    """points (day, score) drawn faint in yellow, a faint dot each, best and
    worst pinned; ma as the yellow line over them. x maps a day to the chart"""
    pts = [(x(d), score_y(s)) for d, s in points]
    best = min(points, key=lambda p: (-p[1], p[0]))
    worst = min(points, key=lambda p: (p[1], p[0]))
    bx, wx = x(best[0]), x(worst[0])

    parts = faint_and_average(x, points, ma, YELLOW)
    # a dot per game, shrinking when days crowd together
    spacing = (RIGHT - LEFT) / max(len(pts) - 1, 1)
    r = min(DOT_R, max(spacing * 0.35, 0.6))
    parts += [f'<circle cx="{px:.3f}" cy="{py:.3f}" r="{r:.2f}" fill="{YELLOW}" fill-opacity="{FAINT}"/>' for px, py in pts]

    # worst and best, pinned from the dot down, labeled "score (#day)". one
    # game is both: just the one pin then
    def label(game: tuple[int, float]) -> str:
        return f"{round(game[1])} (#{game[0]})"

    def width(text: str) -> float:
        return _width(text, MARK_SIZE, MARK_SPACING_EM)

    if worst != best:
        lwx, lbx = _apart(wx, bx, width(label(worst)), width(label(best)))
        marks = [(worst, wx, lwx), (best, bx, lbx)]
    else:
        marks = [(best, bx, _on_chart(bx, width(label(best)) / 2))]
    for game, mx, lx in marks:
        my = score_y(game[1])
        parts.append(
            f'<line x1="{mx:.3f}" y1="{my:.3f}" x2="{mx:.3f}" y2="{BASELINE}" stroke="{YELLOW}" stroke-width="1.2"/>'
            f'<circle cx="{mx:.3f}" cy="{my:.3f}" r="2.6" fill="{YELLOW}"/>'
            f'<text x="{lx:.3f}" y="{MARK_ROW}" text-anchor="middle" font-size="{MARK_SIZE}" '
            f'letter-spacing="{_spacing(MARK_SIZE, MARK_SPACING_EM)}" fill="{YELLOW}">{label(game)}</text>'
        )
    return parts


def faint_and_average(x, points: list[tuple[int, float]], ma: list[tuple[int, float]], color: str) -> list[str]:
    """points as a faint line in color, ma as a solid one over it"""
    parts = []
    daily = line_path(x, points)
    if daily:
        parts.append(f'<path d="{daily}" fill="none" stroke="{color}" stroke-width="1" stroke-opacity="{FAINT}"/>')
    avg = line_path(x, ma)
    if avg:
        parts.append(f'<path d="{avg}" fill="none" stroke="{color}" stroke-width="{AVG_WIDTH}" stroke-opacity="0.95"/>')
    return parts


def line_path(x, points: list[tuple[int, float]]) -> str:
    """(day, score) through the chart, kept inside it; "" under two points"""
    return curve.bezier_path([(x(d), score_y(v)) for d, v in points], (TOP, BASELINE))


def chart_svg(data: TrendData) -> str:
    first, last = data.points[0][0], data.points[-1][0]
    x = day_xs(first, last)
    # everyone under the player, so the player reads on top where they meet
    everyone = faint_and_average(x, data.everyone, data.everyone_ma, CYAN)
    return "\n  ".join(grid_svg(first, last) + everyone + games_svg(x, data.points, data.ma))


def _label_w(text: str) -> float:
    """a spaced label's drawn width: letter-spacing trails every glyph but the last"""
    return max(_width(text, LABEL_SIZE, LABEL_SPACING_EM) - LABEL_SIZE * LABEL_SPACING_EM, 0)


def fit_label(text: str, room: float) -> str:
    """a spaced label cut to room"""
    # _fit measures unspaced glyphs: a size that advances as far as a spaced one
    return _fit(text, LABEL_SIZE * (1 + LABEL_SPACING_EM / ADVANCE_EM), room)


def legend_svg(entries: list[tuple[str, str]], baseline: float) -> str:
    """a square and name for each (name, color) in a row. with two, a name gets
    half the room, or more when the other needs less"""
    spacing = _spacing(LABEL_SIZE, LABEL_SPACING_EM)
    room = CARD_W - 2 * PAD - len(entries) * (SWATCH + SWATCH_GAP) - (len(entries) - 1) * LEGEND_GAP
    names = [drawable(name).upper() for name, _ in entries]
    if len(names) == 2:
        names = [fit_label(n, room - min(_label_w(other), room / 2)) for n, other in zip(names, reversed(names))]
    parts, x = [], float(PAD)
    for text, (_, color) in zip(names, entries):
        parts.append(
            f'<rect x="{x:.2f}" y="{baseline - SWATCH:.2f}" width="{SWATCH}" height="{SWATCH}" fill="{color}"/>'
            f'<text x="{x + SWATCH + SWATCH_GAP:.2f}" y="{baseline:.2f}" font-size="{LABEL_SIZE}" letter-spacing="{spacing}" '
            f'fill="{color}">{escape(text)}</text>'
        )
        x += SWATCH + SWATCH_GAP + _label_w(text) + LEGEND_GAP
    return "".join(parts)


def avatar_svg(data: TrendData, chart_top: float) -> str:
    """their avatar centered on the end of their moving average (their latest
    game without one), kept inside the card and ringed in yellow"""
    day, score = (data.ma or data.points)[-1]
    x = day_xs(data.points[0][0], data.points[-1][0])(day)
    r = data.user_size / 2
    cx = min(max(PAD + x * CHART_SCALE, r + 2), CARD_W - r - 2)
    cy = chart_top + score_y(score) * CHART_SCALE
    uri = avatar_uri(data.avatar)
    if uri:
        disc = (
            f'<clipPath id="av"><circle cx="{cx:.2f}" cy="{cy:.2f}" r="{r:.2f}"/></clipPath>'
            f'<image href="{uri}" x="{cx - r:.2f}" y="{cy - r:.2f}" width="{data.user_size:.2f}" height="{data.user_size:.2f}" '
            f'preserveAspectRatio="xMidYMid slice" clip-path="url(#av)"/>'
        )
    else:
        disc = f'<circle cx="{cx:.2f}" cy="{cy:.2f}" r="{r:.2f}" fill="{BLANK}"/>'
    return disc + f'<circle cx="{cx:.2f}" cy="{cy:.2f}" r="{r:.2f}" fill="none" stroke="{YELLOW}" stroke-width="2"/>'


def render_svg(data: TrendData) -> str:
    if not data.points:
        raise ValueError("no games to draw")
    best = data.best()[1]
    score_text = str(round(best))
    dives = f"{len(data.points)} DIVE{'' if len(data.points) == 1 else 'S'} COMPLETED"
    label_spacing = _spacing(LABEL_SIZE, LABEL_SPACING_EM)
    dives_w = _width(dives, LABEL_SIZE, LABEL_SPACING_EM)
    title = _fit(drawable(data.title), TITLE_SIZE, CARD_W - 2 * PAD - dives_w - SCORE_GAP)
    beside_score = PAD + _width(score_text, SCORE_SIZE) + SCORE_GAP
    moving = "" if not data.ma else f"{data.window.upper()} MOVING AVERAGE{'S' if data.everyone_ma else ''}"
    rank = "" if data.rank is None else _fit(
        f"RANK #{data.rank} IN {drawable(data.guild).upper()}", LABEL_SIZE, CARD_W - 2 * PAD
    )

    # room above the chart for an avatar riding a 700
    chart_top = SCORE_BASELINE + SCORE_GAP + data.user_size / 2
    chart_bottom = chart_top + CHART_H * CHART_SCALE
    legend_baseline = chart_bottom + FOOT_GAP
    foot_baseline = legend_baseline + FOOT_ROW
    card_h = round(foot_baseline + PAD - 4)
    chart = f"translate({PAD} {chart_top:.2f}) scale({CHART_SCALE:.5f})"
    legend = [(data.title, YELLOW)] + ([(EVERYONE, CYAN)] if data.everyone else [])

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
  <text x="{CARD_W - PAD}" y="{_baseline_for_top(TITLE_TOP, LABEL_SIZE):.2f}" text-anchor="end" font-size="{LABEL_SIZE}" letter-spacing="{label_spacing}" fill="{DRIFT}">{dives}</text>

  <text x="{PAD}" y="{SCORE_BASELINE:.2f}" font-size="{SCORE_SIZE}" fill="{YELLOW}" opacity="0.35" filter="url(#glow)">{score_text}</text>
  <text x="{PAD}" y="{SCORE_BASELINE:.2f}" font-size="{SCORE_SIZE}" fill="{FOAM}">{score_text}</text>
  <text x="{beside_score:.2f}" y="{_baseline_for_top(SCORE_TOP, LABEL_SIZE):.2f}" font-size="{LABEL_SIZE}" letter-spacing="{label_spacing}" fill="{DRIFT}">BEST SCORE</text>
  <text x="{beside_score:.2f}" y="{SCORE_BASELINE:.2f}" font-size="{LABEL_SIZE}" fill="{FOAM}">{escape(moving)}</text>

  <g transform="{chart}">
  {chart_svg(data)}
  </g>
  {avatar_svg(data, chart_top)}

  {legend_svg(legend, legend_baseline)}
  <text x="{PAD}" y="{foot_baseline:.2f}" font-size="{LABEL_SIZE}" letter-spacing="{label_spacing}" fill="{YELLOW}" filter="url(#shadow)">{escape(rank)}</text>
</svg>
"""


def render_png(data: TrendData, scale: float = 2) -> bytes:
    png = resvg_py.svg_to_bytes(
        svg_string=render_svg(data),
        font_files=[str(FONT)],
        skip_system_fonts=True,
        zoom=scale,
    )
    return bytes(png)


def render_webp(data: TrendData, scale: float = 2) -> bytes:
    """lossless webp, like card.render_webp"""
    out = io.BytesIO()
    with Image.open(io.BytesIO(render_png(data, scale))) as im:
        im.save(out, format="WEBP", lossless=True, method=6)
    return out.getvalue()
