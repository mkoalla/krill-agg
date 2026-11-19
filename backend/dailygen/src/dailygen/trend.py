"""One player's scores over a period (/topu), dressed like the daily card.

Days run left to right from their first game to their latest, score 0-700
bottom to top. The cyan line is every game, filled up to their best; the
yellow one is a moving average the bot worked out. Their avatar rides the
latest game like the head of a snake.
"""

import io
from dataclasses import dataclass
from html import escape

import resvg_py
from PIL import Image

from . import curve
from .card import (
    ABYSS,
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
    RING,
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
FOOT_GAP = 30  # chart bottom to the footer's baseline


@dataclass
class TrendData:
    title: str  # the player's server nickname
    guild: str  # guild name, for the rank line
    points: list[tuple[int, float]]  # (day, score), one per game, days ascending
    ma: list[tuple[int, float]]  # (day, moving average), days ascending
    window: str  # the moving average's span, e.g. "1-WEEK"
    rank: int | None = None  # on the guild's best-score board; None draws no rank line
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
    """points (day, score) as the cyan line, filled up to the best, with a dot
    each and best/worst pinned; ma as the yellow line. x maps a day to the chart"""
    bounds = (TOP, BASELINE)
    pts = [(x(d), score_y(s)) for d, s in points]
    best = min(points, key=lambda p: (-p[1], p[0]))
    worst = min(points, key=lambda p: (p[1], p[0]))
    bx, wx = x(best[0]), x(worst[0])

    parts = []
    # filled up to their best, like the daily card fills up to the average
    line = curve.bezier_path(pts, bounds)
    if line:
        area = f"{line} L {pts[-1][0]:.3f} {BASELINE} L {pts[0][0]:.3f} {BASELINE} Z"
        parts.append(
            f'<defs><linearGradient id="tr-fill" x1="0" y1="0" x2="0" y2="1">'
            f'<stop offset="0%" stop-color="{CYAN}" stop-opacity="0.45"/>'
            f'<stop offset="100%" stop-color="{CYAN}" stop-opacity="0.02"/>'
            f"</linearGradient>"
            f'<clipPath id="tr-best"><rect x="0" y="0" width="{bx:.3f}" height="{BASELINE}"/></clipPath></defs>'
            f'<path d="{area}" fill="url(#tr-fill)" clip-path="url(#tr-best)"/>'
            f'<path d="{line}" fill="none" stroke="{CYAN}" stroke-width="1.4" stroke-opacity="0.75"/>'
        )

    # the moving average: just a line
    ma_line = line_path(x, ma)
    if ma_line:
        parts.append(
            f'<path d="{ma_line}" fill="none" stroke="{YELLOW}" stroke-width="{MA_WIDTH}" stroke-opacity="0.9"/>'
        )

    # a dot per game, shrinking when days crowd together
    spacing = (RIGHT - LEFT) / max(len(pts) - 1, 1)
    r = min(DOT_R, max(spacing * 0.35, 0.6))
    parts += [f'<circle cx="{px:.3f}" cy="{py:.3f}" r="{r:.2f}" fill="{CYAN}"/>' for px, py in pts]

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
            f'<line x1="{mx:.3f}" y1="{my:.3f}" x2="{mx:.3f}" y2="{BASELINE}" stroke="{CYAN}" stroke-width="1.2"/>'
            f'<circle cx="{mx:.3f}" cy="{my:.3f}" r="2.6" fill="{CYAN}"/>'
            f'<text x="{lx:.3f}" y="{MARK_ROW}" text-anchor="middle" font-size="{MARK_SIZE}" '
            f'letter-spacing="{_spacing(MARK_SIZE, MARK_SPACING_EM)}" fill="{CYAN}">{label(game)}</text>'
        )
    return parts


def line_path(x, points: list[tuple[int, float]]) -> str:
    """(day, score) through the chart, kept inside it; "" under two points"""
    return curve.bezier_path([(x(d), score_y(v)) for d, v in points], (TOP, BASELINE))


def chart_svg(data: TrendData) -> str:
    first, last = data.points[0][0], data.points[-1][0]
    return "\n  ".join(grid_svg(first, last) + games_svg(day_xs(first, last), data.points, data.ma))


def avatar_svg(data: TrendData, chart_top: float) -> str:
    """their avatar centered on the latest game's dot, kept inside the card"""
    day, score = data.points[-1]
    x = day_xs(data.points[0][0], day)(day)
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
    return disc + f'<circle cx="{cx:.2f}" cy="{cy:.2f}" r="{r:.2f}" fill="none" stroke="{RING}" stroke-width="2"/>'


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
    moving = "" if not data.ma else f"{data.window.upper()} MOVING AVERAGE"
    rank = "" if data.rank is None else _fit(
        f"RANK #{data.rank} IN {drawable(data.guild).upper()}", LABEL_SIZE, CARD_W - 2 * PAD
    )

    # room above the chart for an avatar riding a 700
    chart_top = SCORE_BASELINE + SCORE_GAP + data.user_size / 2
    chart_bottom = chart_top + CHART_H * CHART_SCALE
    foot_baseline = chart_bottom + FOOT_GAP
    card_h = round(foot_baseline + PAD - 4)
    chart = f"translate({PAD} {chart_top:.2f}) scale({CHART_SCALE:.5f})"

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

  <text x="{PAD}" y="{SCORE_BASELINE:.2f}" font-size="{SCORE_SIZE}" fill="{CYAN}" opacity="0.45" filter="url(#glow)">{score_text}</text>
  <text x="{PAD}" y="{SCORE_BASELINE:.2f}" font-size="{SCORE_SIZE}" fill="{FOAM}">{score_text}</text>
  <text x="{beside_score:.2f}" y="{_baseline_for_top(SCORE_TOP, LABEL_SIZE):.2f}" font-size="{LABEL_SIZE}" letter-spacing="{label_spacing}" fill="{DRIFT}">BEST SCORE</text>
  <text x="{beside_score:.2f}" y="{SCORE_BASELINE:.2f}" font-size="{LABEL_SIZE}" fill="{YELLOW}">{escape(moving)}</text>

  <g transform="{chart}">
  {chart_svg(data)}
  </g>
  {avatar_svg(data, chart_top)}

  <text x="{PAD}" y="{foot_baseline:.2f}" font-size="{LABEL_SIZE}" letter-spacing="{label_spacing}" fill="{CYAN}" filter="url(#shadow)">{escape(rank)}</text>
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
