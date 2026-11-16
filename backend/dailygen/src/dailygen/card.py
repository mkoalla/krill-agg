"""The results card as one SVG, rasterized with resvg.

The chart is krillion.io's own SVG (see curve.py) dropped into a card with the
score row and percentile line around it.
"""

from dataclasses import dataclass, field
from html import escape
from importlib.resources import files

import resvg_py

from . import curve
from .players import Player, avatar_uri, stack

FONT = files("dailygen") / "fonts" / "VT323-Regular.ttf"
FONT_FAMILY = "VT323"

# krillion.io's palette
ABYSS = "#050a14"
FOAM = "#e8f1ff"
CYAN = "#4de3ff"
PINK = "#ff5d8f"
DRIFT = "#7d93b8"
HAIRLINE = "#7d93b838"

# VT323 is monospaced: every glyph advances 0.4em, and caps/digits stand 0.56em
# tall. top-aligning text = putting its baseline one cap height below the top
ADVANCE_EM = 0.4
CAP_EM = 0.56

# text sizes
TITLE_SIZE = 26
LABEL_SIZE = 20  # dive label, "GUILD AVERAGE", depth, footer
SCORE_SIZE = 104
LABEL_SPACING_EM = 0.14

# players: greyed pins with their avatar floating above the curve
GREY = "#8b96a8"
RING = "#a7b1c2"
BLANK = "#2a3444"
PIN_LEN = 10  # curve point to the bottom of the lowest avatar
LEVEL_GAP = 4  # between stacked avatars

# card layout, in logical px (rendered at `scale`). the chart and everything
# under it move down when stacked avatars need more room (see layout())
CARD_W, BASE_CARD_H = 640, 352
PAD = 24
TITLE_TOP = PAD  # title and dive label share this top edge
SCORE_TOP = 58  # score and "GUILD AVERAGE" share this top edge
SCORE_BASELINE = SCORE_TOP + CAP_EM * SCORE_SIZE  # ...and score and depth this baseline
SCORE_GAP = 14  # between the score and its labels
CHART_SCALE = (
    CARD_W - 2 * PAD
) / curve.WIDTH  # the 320-wide chart stretched edge to edge
BASE_CHART_TOP = 130
BASE_FOOT_BASELINE = 334
CLEARANCE = SCORE_BASELINE + 8  # nothing floats higher than this
# guild icon: stands in for the AVG label under the pin, as big as the players
ICON_DROP = 2  # chart units between the baseline and the icon
ICON_ROUND = 0.2  # corner radius, as a share of the side
# top score card: right-aligned in the score row, as tall as it
TOP_W = 200
TOP_H = SCORE_BASELINE - SCORE_TOP
TOP_PAD = 12
TOP_RADIUS = 10
TOP_LABEL_SIZE = 16
TOP_VALUE_SIZE = 28
TOP_AVATAR = TOP_H - 16  # diameter
TOP_OVERLAP = 0.6  # tied avatars step left by this share of a diameter
TOP_MAX_AVATARS = 3


@dataclass
class CardData:
    title: str  # guild name
    day: int
    mean: float  # guild mean score
    dist: list[float]  # krillion's 20 bucket counts
    percentile: float | None = None  # estimated from dist when None
    marker: str = "AVG"
    population: str = "today's players"
    players: list[Player] = field(default_factory=list)
    user_size: float = 28  # avatar diameter
    max_push: int = 4  # most avatars a stack grows before wrapping down
    icon: str | None = None  # guild icon path; missing/unreadable means none drawn
    sample: int | None = None  # krillion's player count today; drawn when known

    def better_than(self) -> float:
        if self.percentile is not None:
            return self.percentile
        return curve.percentile(self.dist, self.mean)


def _spacing(size: float, em: float) -> str:
    return f"{size * em:.2f}"


def _baseline_for_top(top: float, size: float) -> float:
    return top + CAP_EM * size


def _width(text: str, size: float, spacing_em: float = 0) -> float:
    return len(text) * size * (ADVANCE_EM + spacing_em)


def _fit(text: str, size: float, max_width: float) -> str:
    """cuts text to fit, ending in '...' when it had to"""
    if _width(text, size) <= max_width:
        return text
    keep = max(int(max_width / (size * ADVANCE_EM)) - 3, 0)
    return text[:keep].rstrip() + "..."


def chart_svg(data: CardData, pts: list[curve.Point]) -> str:
    """The distribution chart in its own 320x96 space, minus the AVG marker."""
    if not pts:
        return ""
    line = curve.bezier_path(pts)
    area = curve.area_path(line)
    m = curve.score_x(data.mean)
    label_x = min(max(m, 14), 306)
    base = curve.BASELINE

    ticks = []
    for value in (100, 200, 300, 400, 500, 600):
        x = value / curve.MAX_DAY_SCORE * curve.WIDTH
        ticks.append(
            f'<line x1="{x:.3f}" y1="{base}" x2="{x:.3f}" y2="81.5" stroke="{DRIFT}" stroke-opacity="0.5"/>'
        )
        if abs(x - m) >= 18:  # keep clear of the marker label
            ticks.append(
                f'<text x="{x:.3f}" y="90" text-anchor="middle" font-size="9" fill="{DRIFT}" opacity="0.55">{value}</text>'
            )

    # the site always draws these and lets the marker label overlap them at the
    # extremes; drop one the same way ticks give way
    ends = ""
    if label_x >= 18:
        ends += (
            f'<text x="0" y="90" font-size="9" fill="{DRIFT}" opacity="0.6">0</text>'
        )
    if curve.WIDTH - label_x >= 18:
        ends += f'<text x="{curve.WIDTH}" y="90" text-anchor="end" font-size="9" fill="{DRIFT}" opacity="0.6">{curve.MAX_DAY_SCORE}</text>'

    return f"""
  <defs>
    <linearGradient id="pc-fill" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0%" stop-color="{CYAN}" stop-opacity="0.45"/>
      <stop offset="100%" stop-color="{CYAN}" stop-opacity="0.02"/>
    </linearGradient>
    <clipPath id="pc-beaten"><rect x="0" y="0" width="{m:.3f}" height="96"/></clipPath>
  </defs>
  <path d="{area}" fill="url(#pc-fill)" opacity="0.28"/>
  <path d="{area}" fill="url(#pc-fill)" clip-path="url(#pc-beaten)"/>
  <path d="{line}" fill="none" stroke="{CYAN}" stroke-width="1.4" stroke-opacity="0.75"/>
  <line x1="0" y1="{base}" x2="{curve.WIDTH}" y2="{base}" stroke="{HAIRLINE}"/>
  {"".join(ticks)}
  {ends}"""


def _label_x(data: CardData) -> float:
    """where the marker's label sits, in chart units: under the pin, kept on the chart"""
    return min(max(curve.score_x(data.mean), 14), 306)


def marker_svg(data: CardData, pts: list[curve.Point], label: bool = True) -> str:
    """The AVG pin, drawn last so it stays over the players. label=False leaves
    the text out for the guild icon to take its place (see avg_icon_svg)."""
    if not pts:
        return ""
    m = curve.score_x(data.mean)
    h = curve.marker_y(pts, m)
    text = (
        f'<text x="{_label_x(data):.3f}" y="90" text-anchor="middle" font-size="9" '
        f'letter-spacing="{_spacing(9, 0.12)}" fill="{PINK}">{escape(data.marker)}</text>'
        if label
        else ""
    )
    return f"""
  <line x1="{m:.3f}" y1="{min(h, 74) - 3:.3f}" x2="{m:.3f}" y2="{curve.BASELINE}" stroke="{PINK}" stroke-width="1.2"/>
  <circle cx="{m:.3f}" cy="{h:.3f}" r="2.6" fill="{PINK}"/>
  {text}"""


@dataclass
class Placed:
    x: float  # the score's x on the card
    curve_y: float  # where the curve is at x
    cx: float  # avatar center (kept inside the card)
    cy: float
    uri: str | None


@dataclass
class Layout:
    chart_top: float
    card_h: float
    foot_baseline: float
    players: list[Placed]


def layout(data: CardData, pts: list[curve.Point]) -> Layout:
    """Places every avatar above its spot on the curve, stacking clashes, then
    pushes the chart down by however much the tallest stack needs."""
    r = data.user_size / 2
    order = sorted(range(len(data.players)), key=lambda j: data.players[j].score)
    xs = [PAD + curve.score_x(data.players[j].score) * CHART_SCALE for j in order]
    levels = stack(xs, data.user_size, data.max_push)

    rel = []  # positions relative to the chart's top
    for j, x, level in zip(order, xs, levels):
        curve_y = (
            curve.marker_y(pts, curve.score_x(data.players[j].score)) * CHART_SCALE
            if pts
            else 0
        )
        cy = curve_y - PIN_LEN - r - level * (data.user_size + LEVEL_GAP)
        rel.append((j, x, curve_y, cy))

    highest = min((cy - r for _, _, _, cy in rel), default=0)
    shift = max(0.0, CLEARANCE - (BASE_CHART_TOP + highest))
    top = BASE_CHART_TOP + shift
    placed = [
        Placed(
            x=x,
            curve_y=top + curve_y,
            cx=min(max(x, r + 2), CARD_W - r - 2),
            cy=top + cy,
            uri=avatar_uri(data.players[j].avatar),
        )
        for j, x, curve_y, cy in rel
    ]
    return Layout(
        chart_top=top,
        card_h=BASE_CARD_H + shift,
        foot_baseline=BASE_FOOT_BASELINE + shift,
        players=placed,
    )


def players_svg(data: CardData, placed: list[Placed]) -> str:
    """Greyed pins from the curve up to each avatar, then the avatars: an image
    cropped to a circle in a ring, or a plain grey disc when there's no image."""
    r = data.user_size / 2
    pins, discs = [], []
    for k, p in enumerate(placed):
        pins.append(
            f'<line x1="{p.x:.2f}" y1="{p.curve_y:.2f}" x2="{p.x:.2f}" y2="{p.cy + r:.2f}" stroke="{GREY}" stroke-width="1.6" stroke-opacity="0.7"/>'
            f'<circle cx="{p.x:.2f}" cy="{p.curve_y:.2f}" r="3.2" fill="{GREY}"/>'
        )
        if p.uri:
            discs.append(
                f'<clipPath id="av{k}"><circle cx="{p.cx:.2f}" cy="{p.cy:.2f}" r="{r:.2f}"/></clipPath>'
                f'<image href="{p.uri}" x="{p.cx - r:.2f}" y="{p.cy - r:.2f}" width="{data.user_size:.2f}" height="{data.user_size:.2f}" '
                f'preserveAspectRatio="xMidYMid slice" clip-path="url(#av{k})"/>'
            )
        else:
            discs.append(
                f'<circle cx="{p.cx:.2f}" cy="{p.cy:.2f}" r="{r:.2f}" fill="{BLANK}"/>'
            )
        discs.append(
            f'<circle cx="{p.cx:.2f}" cy="{p.cy:.2f}" r="{r:.2f}" fill="none" stroke="{RING}" stroke-width="2"/>'
        )
    return "\n  ".join(pins + discs)


def avg_icon_svg(data: CardData, uri: str | None, chart_top: float) -> str:
    """The guild icon in place of the AVG label: a lightly rounded square as
    big as the players' avatars, ringed in the pin's color, just under the
    baseline below the pin."""
    if not uri:
        return ""
    size = data.user_size
    x = PAD + _label_x(data) * CHART_SCALE - size / 2
    y = chart_top + (curve.BASELINE + ICON_DROP) * CHART_SCALE
    box = f'x="{x:.2f}" y="{y:.2f}" width="{size:.2f}" height="{size:.2f}" rx="{size * ICON_ROUND:.2f}"'
    return (
        f'<clipPath id="icon"><rect {box}/></clipPath>'
        f'<image href="{uri}" x="{x:.2f}" y="{y:.2f}" width="{size:.2f}" height="{size:.2f}" '
        f'preserveAspectRatio="xMidYMid slice" clip-path="url(#icon)"/>'
        f'<rect {box} fill="none" stroke="{PINK}" stroke-width="2"/>'
    )


def top_svg(data: CardData, top: float) -> str:
    """A navy card with the best score on the left and its scorer's avatar on
    the right (ties overlap, best-first on top). No names, to keep it quiet."""
    if not data.players:
        return ""
    best = max(p.score for p in data.players)
    winners = [p for p in data.players if p.score == best][:TOP_MAX_AVATARS]
    x = CARD_W - PAD - TOP_W
    spacing = _spacing(TOP_LABEL_SIZE, LABEL_SPACING_EM)
    parts = [
        f'<rect x="{x:.2f}" y="{top:.2f}" width="{TOP_W}" height="{TOP_H:.2f}" rx="{TOP_RADIUS}" '
        f'fill="{ABYSS}" stroke="{HAIRLINE}" stroke-width="1.5"/>',
        f'<text x="{x + TOP_PAD:.2f}" y="{_baseline_for_top(top + TOP_PAD, TOP_LABEL_SIZE):.2f}" '
        f'font-size="{TOP_LABEL_SIZE}" letter-spacing="{spacing}" fill="{DRIFT}">TOP SCORE</text>',
        f'<text x="{x + TOP_PAD:.2f}" y="{top + TOP_H - TOP_PAD:.2f}" font-size="{TOP_VALUE_SIZE}" '
        f'fill="{FOAM}">{round(best)}</text>',
    ]
    r = TOP_AVATAR / 2
    cy = top + TOP_H / 2
    # drawn last-first so the first winner lands on top, rightmost
    for k in reversed(range(len(winners))):
        cx = x + TOP_W - TOP_PAD - r - k * TOP_AVATAR * TOP_OVERLAP
        uri = avatar_uri(winners[k].avatar)
        if uri:
            parts.append(
                f'<clipPath id="top{k}"><circle cx="{cx:.2f}" cy="{cy:.2f}" r="{r:.2f}"/></clipPath>'
                f'<image href="{uri}" x="{cx - r:.2f}" y="{cy - r:.2f}" width="{TOP_AVATAR:.2f}" height="{TOP_AVATAR:.2f}" '
                f'preserveAspectRatio="xMidYMid slice" clip-path="url(#top{k})"/>'
            )
        else:
            parts.append(
                f'<circle cx="{cx:.2f}" cy="{cy:.2f}" r="{r:.2f}" fill="{BLANK}"/>'
            )
        parts.append(
            f'<circle cx="{cx:.2f}" cy="{cy:.2f}" r="{r:.2f}" fill="none" stroke="{RING}" stroke-width="2"/>'
        )
    return "\n  ".join(parts)


def render_svg(data: CardData) -> str:
    score = round(data.mean)
    depth = f"-{10 * score:,}m"
    dive = f"DIVE #{data.day} COMPLETE"
    dive_w = _width(dive, LABEL_SIZE, LABEL_SPACING_EM)
    title = _fit(f"{data.title}", TITLE_SIZE, CARD_W - 2 * PAD - dive_w - SCORE_GAP)
    beside_score = PAD + _width(str(score), SCORE_SIZE) + SCORE_GAP
    label_spacing = _spacing(LABEL_SIZE, LABEL_SPACING_EM)
    better = f"BETTER THAN {curve.format_percentile(data.better_than())}% OF {data.population.upper()}"
    total = "" if data.sample is None else f"{data.sample:,} TOTAL"
    pts = curve.points(curve.smooth(data.dist))
    lay = layout(data, pts)
    icon = avatar_uri(data.icon)
    chart = f"translate({PAD} {lay.chart_top:.2f}) scale({CHART_SCALE:.5f})"

    return f"""<svg xmlns="http://www.w3.org/2000/svg" width="{CARD_W}" height="{lay.card_h:.0f}" viewBox="0 0 {CARD_W} {lay.card_h:.0f}" font-family="{FONT_FAMILY}">
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
  <rect width="{CARD_W}" height="{lay.card_h:.0f}" fill="url(#bg)"/>

  <text x="{PAD}" y="{_baseline_for_top(TITLE_TOP, TITLE_SIZE):.2f}" font-size="{TITLE_SIZE}" fill="{FOAM}">{escape(title)}</text>
  <text x="{CARD_W - PAD}" y="{_baseline_for_top(TITLE_TOP, LABEL_SIZE):.2f}" text-anchor="end" font-size="{LABEL_SIZE}" letter-spacing="{label_spacing}" fill="{DRIFT}">{dive}</text>
  {top_svg(data, SCORE_TOP)}

  <text x="{PAD}" y="{SCORE_BASELINE:.2f}" font-size="{SCORE_SIZE}" fill="{CYAN}" opacity="0.45" filter="url(#glow)">{score}</text>
  <text x="{PAD}" y="{SCORE_BASELINE:.2f}" font-size="{SCORE_SIZE}" fill="{FOAM}">{score}</text>
  <text x="{beside_score:.2f}" y="{_baseline_for_top(SCORE_TOP, LABEL_SIZE):.2f}" font-size="{LABEL_SIZE}" letter-spacing="{label_spacing}" fill="{DRIFT}">GUILD AVERAGE</text>
  <text x="{beside_score:.2f}" y="{SCORE_BASELINE:.2f}" font-size="{LABEL_SIZE}" fill="{CYAN}">{depth}</text>

  <g transform="{chart}">{chart_svg(data, pts)}
  </g>
  {players_svg(data, lay.players)}
  <g transform="{chart}">{marker_svg(data, pts, label=not icon)}
  </g>
  {avg_icon_svg(data, icon, lay.chart_top) if pts else ""}

  <text x="{PAD}" y="{lay.foot_baseline:.2f}" font-size="{LABEL_SIZE}" letter-spacing="{label_spacing}" fill="{CYAN}" filter="url(#shadow)">{escape(better)}</text>
  <text x="{CARD_W - PAD}" y="{lay.foot_baseline:.2f}" text-anchor="end" font-size="{LABEL_SIZE}" letter-spacing="{label_spacing}" fill="{CYAN}" filter="url(#shadow)">{total}</text>
</svg>
"""


def render_png(data: CardData, scale: float = 2) -> bytes:
    png = resvg_py.svg_to_bytes(
        svg_string=render_svg(data),
        font_files=[str(FONT)],
        skip_system_fonts=True,
        zoom=scale,
    )
    return bytes(png)
