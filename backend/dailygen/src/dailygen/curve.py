"""The distribution curve, ported from krillion.io's results chart.

Everything here works in the chart's own SVG space: a 320x96 viewBox with the
baseline at y=78 and the tallest point 68 above it. card.py scales it to fit.
"""

from math import floor

MAX_DAY_SCORE = 700
WIDTH = 320.0
BASELINE = 78.0
PEAK = 68.0  # height of the tallest bucket above the baseline
BUCKET_WIDTH = 35  # 20 buckets across 0-700

# their 5-tap smoothing, centered on each bucket
KERNEL = (0.06, 0.24, 0.4, 0.24, 0.06)

Point = tuple[float, float]


def smooth(dist: list[float]) -> list[float]:
    """Weighted average of each bucket and two on either side. At the edges the
    missing taps are dropped and the remaining weights renormalized."""
    out = []
    for r in range(len(dist)):
        total = weight = 0.0
        for offset in range(-2, 3):
            i = r + offset
            if 0 <= i < len(dist):
                total += dist[i] * KERNEL[offset + 2]
                weight += KERNEL[offset + 2]
        out.append(total / weight if weight > 0 else 0.0)
    return out


def points(smoothed: list[float]) -> list[Point]:
    """Baseline at both ends, one point per bucket center in between."""
    peak = max(smoothed, default=0)
    if not peak > 0:
        return []
    n = len(smoothed)
    pts = [(0.0, BASELINE)]
    pts += [((i + 0.5) / n * WIDTH, BASELINE - v / peak * PEAK) for i, v in enumerate(smoothed)]
    pts.append((WIDTH, BASELINE))
    return pts


def _num(v: float) -> str:
    # trims float noise so the path stays readable
    return f"{v:.3f}".rstrip("0").rstrip(".")


def bezier_path(pts: list[Point], y_bounds: tuple[float, float] | None = None) -> str:
    """Catmull-Rom through every point, written as cubic beziers (ends clamped).
    y_bounds (lo, hi) clamps the control points' y too: a bezier stays inside
    its control points, so with every point in bounds the curve never leaves them."""
    if len(pts) < 2:
        return ""

    def cy(y: float) -> float:
        return y if y_bounds is None else min(max(y, y_bounds[0]), y_bounds[1])

    d = f"M {_num(pts[0][0])} {_num(pts[0][1])}"
    for r in range(len(pts) - 1):
        p0, p1, p2, p3 = pts[max(0, r - 1)], pts[r], pts[r + 1], pts[min(len(pts) - 1, r + 2)]
        c1 = (p1[0] + (p2[0] - p0[0]) / 6, cy(p1[1] + (p2[1] - p0[1]) / 6))
        c2 = (p2[0] - (p3[0] - p1[0]) / 6, cy(p2[1] - (p3[1] - p1[1]) / 6))
        d += f" C {_num(c1[0])} {_num(c1[1])}, {_num(c2[0])} {_num(c2[1])}, {_num(p2[0])} {_num(p2[1])}"
    return d


def area_path(curve: str) -> str:
    return f"{curve} L {_num(WIDTH)} {_num(BASELINE)} L 0 {_num(BASELINE)} Z"


def score_x(score: float) -> float:
    return min(max(score, 0), MAX_DAY_SCORE) / MAX_DAY_SCORE * WIDTH


def marker_y(pts: list[Point], x: float) -> float:
    """Height of the curve at x, linear between points like the site does."""
    for (x1, y1), (x2, y2) in zip(pts, pts[1:]):
        if x1 <= x <= x2:
            return y1 + (y2 - y1) * (0 if x2 == x1 else (x - x1) / (x2 - x1))
    return BASELINE


def percentile(dist: list[float], score: float, sample: float | None = None) -> float:
    """Percent of players below score from the histogram alone: every bucket under
    it plus a linear share of its own. Same estimate as the bot's
    krillionapi.Stats.Percentile so the card and the embeds agree."""
    sample = sum(dist) if sample is None else sample
    if sample <= 0 or not dist:
        return 0.0
    bucket = min(int(max(score, 0)) // BUCKET_WIDTH, len(dist) - 1)
    below = sum(dist[:bucket])
    into = (max(score, 0) - bucket * BUCKET_WIDTH) / BUCKET_WIDTH
    below += dist[bucket] * min(into, 1)
    return 100 * below / sample


def format_percentile(p: float) -> str:
    """Two decimals, floored so nobody is rounded past where they are."""
    return f"{floor(p * 100 + 1e-9) / 100:.2f}"