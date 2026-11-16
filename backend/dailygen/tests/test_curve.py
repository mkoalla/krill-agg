from datetime import datetime, timezone

import pytest

from dailygen import api, curve
from dailygen.card import CardData, render_png, render_svg

# a real snapshot of krillion's buckets (sample = their total)
SNAPSHOT = [2155, 9872, 46531, 83029, 192709, 262968, 383812, 480162, 489957, 382995,
            367690, 271100, 122620, 68850, 44903, 15027, 10123, 6290, 1844, 250]


def test_smooth_interior_and_edges():
    dist = [0, 0, 100, 0, 0]
    s = curve.smooth(dist)
    assert s[2] == pytest.approx(40)  # center tap .4 over a full kernel
    assert s[1] == pytest.approx(100 * 0.24 / 0.94)  # its -2 tap falls off the start
    # edge: taps -2,-1 are missing, so .06+.24 is renormalized over .7
    assert s[0] == pytest.approx(100 * 0.06 / 0.7)


def test_points_and_path():
    pts = curve.points(curve.smooth(SNAPSHOT))
    assert len(pts) == 22
    assert pts[0] == (0, curve.BASELINE) and pts[-1] == (curve.WIDTH, curve.BASELINE)
    assert min(y for _, y in pts) == pytest.approx(curve.BASELINE - curve.PEAK)  # tallest hits the peak
    path = curve.bezier_path(pts)
    assert path.startswith("M 0 78") and path.count(" C ") == 21 and path.endswith("320 78")
    assert curve.points([0, 0]) == []


def test_marker():
    pts = curve.points(curve.smooth(SNAPSHOT))
    # at a bucket center the marker sits exactly on that point
    x, y = pts[5]
    assert curve.marker_y(pts, x) == pytest.approx(y)
    assert curve.score_x(-5) == 0 and curve.score_x(9999) == curve.WIDTH


@pytest.mark.parametrize("score,want", [(0, 0), (175, 10.31), (350, 71.98), (455, 95.46), (700, 100)])
def test_percentile_matches_bot(score, want):
    assert curve.percentile(SNAPSHOT, score) == pytest.approx(want, abs=0.01)


@pytest.mark.parametrize("p,want", [(71.98, "71"), (5.9, "5"), (99.5, "99.50"), (99.987, "99.99"), (100, "100.00")])
def test_format_percentile(p, want):
    assert curve.format_percentile(p) == want


def test_days():
    assert api.day_at(datetime(2026, 11, 16, 12, tzinfo=timezone.utc)) == 124
    assert api.day_at(datetime(2026, 11, 17, 4, 59, 59, tzinfo=timezone.utc)) == 124
    assert api.day_at(datetime(2026, 11, 17, 5, tzinfo=timezone.utc)) == 125
    assert api.date_of(124) == "2026-11-16"


def test_card_renders():
    data = CardData(day=124, mean=371.4, dist=SNAPSHOT)
    svg = render_svg(data)
    assert "DIVE #124 COMPLETE" in svg and "-3,710m" in svg and ">371<" in svg and "AVG" in svg
    png = render_png(data, scale=1)
    assert png[:8] == b"\x89PNG\r\n\x1a\n"


def test_end_labels_give_way_to_marker():
    assert ">700<" not in render_svg(CardData(day=1, mean=699, dist=SNAPSHOT))
    assert ">700<" in render_svg(CardData(day=1, mean=350, dist=SNAPSHOT))
