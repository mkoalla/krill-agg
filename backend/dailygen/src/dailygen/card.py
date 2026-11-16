"""The results card as one SVG, rasterized with resvg.

The chart is krillion.io's own SVG (see curve.py) dropped into a card with the
score row and percentile line around it.
"""

from dataclasses import dataclass, field
from html import escape
from importlib.resources import files

import resvg_py

from . import curve

FONT = files("dailygen") / "fonts" / "VT323-Regular.ttf"
FONT_FAMILY = "VT323"

# krillion.io's palette
ABYSS = "#050a14"
FOAM = "#e8f1ff"
CYAN = "#4de3ff"
PINK = "#ff5d8f"
DRIFT = "#7d93b8"
HAIRLINE = "#7d93b838"

# card layout, in logical px (rendered at `scale`)
CARD_W, CARD_H = 640, 340
PAD = 24
CHART_SCALE = (CARD_W - 2 * PAD) / curve.WIDTH  # the 320-wide chart stretched edge to edge
CHART_TOP = 118
ROW_BASELINE = 104  # score, depth and dive label all sit on this
FOOT_BASELINE = 322


@dataclass
class CardData:
    day: int
    mean: float  # guild mean score
    dist: list[float]  # krillion's 20 bucket counts
    percentile: float | None = None  # estimated from dist when None
    marker: str = "AVG"
    population: str = "today's players"
    extra: dict = field(default_factory=dict)

    def better_than(self) -> float:
        if self.percentile is not None:
            return self.percentile
        return curve.percentile(self.dist, self.mean)


def _spacing(size: float, em: float) -> str:
    return f"{size * em:.2f}"


def chart_svg(data: CardData) -> str:
    """The distribution chart in its own 320x96 space."""
    pts = curve.points(curve.smooth(data.dist))
    if not pts:
        return ""
    line = curve.bezier_path(pts)
    area = curve.area_path(line)
    m = curve.score_x(data.mean)
    h = curve.marker_y(pts, m)
    label_x = min(max(m, 14), 306)
    base = curve.BASELINE

    ticks = []
    for value in (100, 200, 300, 400, 500, 600):
        x = value / curve.MAX_DAY_SCORE * curve.WIDTH
        ticks.append(f'<line x1="{x:.3f}" y1="{base}" x2="{x:.3f}" y2="81.5" stroke="{DRIFT}" stroke-opacity="0.5"/>')
        if abs(x - m) >= 18:  # keep clear of the marker label
            ticks.append(
                f'<text x="{x:.3f}" y="90" text-anchor="middle" font-size="9" fill="{DRIFT}" opacity="0.55">{value}</text>'
            )

    # the site always draws these and lets the marker label overlap them at the
    # extremes; drop one the same way ticks give way
    ends = ""
    if label_x >= 18:
        ends += f'<text x="0" y="90" font-size="9" fill="{DRIFT}" opacity="0.6">0</text>'
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
  <line x1="{m:.3f}" y1="{min(h, 74) - 3:.3f}" x2="{m:.3f}" y2="{base}" stroke="{PINK}" stroke-width="1.2"/>
  <circle cx="{m:.3f}" cy="{h:.3f}" r="2.6" fill="{PINK}"/>
  <text x="{label_x:.3f}" y="90" text-anchor="middle" font-size="9" letter-spacing="{_spacing(9, 0.12)}" fill="{PINK}">{escape(data.marker)}</text>
  {ends}"""


def render_svg(data: CardData) -> str:
    score = round(data.mean)
    depth = f"-{10 * score:,}m"
    dive = f"DIVE #{data.day} COMPLETE"
    better = f"BETTER THAN {curve.format_percentile(data.better_than())}% OF {data.population.upper()}"

    return f"""<svg xmlns="http://www.w3.org/2000/svg" width="{CARD_W}" height="{CARD_H}" viewBox="0 0 {CARD_W} {CARD_H}" font-family="{FONT_FAMILY}">
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
  <rect width="{CARD_W}" height="{CARD_H}" fill="url(#bg)"/>
  <text x="{CARD_W / 2}" y="292" text-anchor="middle" font-size="150" letter-spacing="18" fill="{PINK}" opacity="0.07">KRILLION</text>

  <text x="{PAD}" y="{ROW_BASELINE}" font-size="104" fill="{CYAN}" opacity="0.45" filter="url(#glow)">{score}</text>
  <text x="{PAD}" y="{ROW_BASELINE}"><tspan font-size="104" fill="{FOAM}">{score}</tspan><tspan dx="14" font-size="20" fill="{CYAN}">{depth}</tspan></text>
  <text x="{CARD_W - PAD}" y="{ROW_BASELINE}" text-anchor="end" font-size="20" letter-spacing="{_spacing(20, 0.14)}" fill="{DRIFT}">{dive}</text>

  <g transform="translate({PAD} {CHART_TOP}) scale({CHART_SCALE:.5f})">{chart_svg(data)}
  </g>

  <text x="{PAD}" y="{FOOT_BASELINE}" font-size="20" letter-spacing="{_spacing(20, 0.14)}" fill="{CYAN}" filter="url(#shadow)">{escape(better)}</text>
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
