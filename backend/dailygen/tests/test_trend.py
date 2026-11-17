import re

import pytest

from dailygen import curve, trend
from dailygen.trend import TrendData, render_png, render_svg

from .test_server import a_red_square, post, server  # noqa: F401 (fixture)


def data(points, **over):
    d = {"title": "krillmaster", "guild": "Krill Club", "points": points, "ma": points, "window": "1-WEEK", "rank": 2}
    d.update(over)
    return TrendData(**d)


def path_ys(path: str) -> list[float]:
    nums = [float(n) for n in re.findall(r"-?\d+(?:\.\d+)?", path)]
    return nums[1::2]


def test_best_and_worst_take_the_earliest_tie():
    d = data([(1, 300), (2, 500), (3, 100), (4, 500), (5, 100)])
    assert d.best() == (2, 500) and d.worst() == (3, 100)


def test_clamped_path_stays_on_the_chart():
    pts = [(trend.LEFT + 10 * i, trend.score_y(s)) for i, s in enumerate([0, 600, 700, 690, 100, 0])]
    loose = path_ys(curve.bezier_path(pts))
    assert min(loose) < trend.TOP or max(loose) > trend.BASELINE  # it would overshoot
    tight = path_ys(curve.bezier_path(pts, (trend.TOP, trend.BASELINE)))
    assert trend.TOP <= min(tight) and max(tight) <= trend.BASELINE


def test_day_ticks():
    assert trend.day_ticks(10, 10) == [10]
    assert trend.day_ticks(10, 12) == [10, 11, 12]
    assert trend.day_ticks(100, 340) == [100, 160, 220, 280, 340]


@pytest.mark.parametrize("points", [
    [(124, 302)],
    [(100 + i, (i * 97) % 701) for i in range(200)],
    [(1, 0), (2, 700), (3, 0)],
])
def test_renders(points):
    svg = render_svg(data(points))
    assert f"{len(points)} DIVE" in svg and ">1-WEEK MOVING AVERAGE<" in svg and "RANK #2 IN KRILL CLUB" in svg
    assert render_png(data(points))[:8] == b"\x89PNG\r\n\x1a\n"


def test_pins_are_labeled_with_their_scores():
    svg = render_svg(data([(1, 300), (2, 512), (3, 104)]))
    assert f'fill="{trend.CYAN}">512 (#2)</text>' in svg and f'fill="{trend.CYAN}">104 (#3)</text>' in svg
    # one game is both best and worst: one pin
    assert render_svg(data([(124, 302)])).count('r="2.6"') == 1


def test_no_rank_no_rank_line():
    assert "RANK #" not in render_svg(data([(1, 300)], rank=None))


def trend_body(**over):
    b = {"guild": "123", "user": "456", "title": "krillmaster", "guild_name": "Krill Club", "images": "scratch",
         "points": [{"day": 3, "score": 400}, {"day": 1, "score": 300}], "ma": [{"day": 1, "value": 300}],
         "window": "3-DAY", "rank": 1}
    b.update(over)
    return b


def test_trend_writes_and_overwrites(server):  # noqa: F811
    url, root = server
    images = root / "123" / "users" / "456" / "scratch"
    images.mkdir(parents=True)
    (images / "0.png").write_bytes(a_red_square())
    status, resp = post(url, trend_body(), "/trend")
    assert status == 200 and resp == {"path": "123/users/456/trend.webp"}
    out = root / "123" / "users" / "456" / "trend.webp"
    first = out.read_bytes()
    assert first[:4] == b"RIFF" and first[8:12] == b"WEBP"
    status, _ = post(url, trend_body(rank=5), "/trend")
    assert status == 200 and out.read_bytes() != first


@pytest.mark.parametrize("over", [{"user": "../x"}, {"points": []}, {"points": [{"day": 1}]}, {"images": ""}])
def test_trend_bad_requests(server, over):  # noqa: F811
    url, _ = server
    status, resp = post(url, trend_body(**over), "/trend")
    assert status == 400 and resp["error"]


@pytest.mark.parametrize("a,b", [(0, 3), (3, 0), (150, 152), (317, 320), (320, 317), (160, 160)])
def test_pin_labels_never_overlap_or_leave_the_chart(a, b):
    wa, wb = 50, 40
    la, lb = trend._apart(a, b, wa, wb)
    assert abs(la - lb) >= (wa + wb) / 2  # no overlap
    for x, w in ((la, wa), (lb, wb)):
        assert w / 2 <= x + 1e-9 and x + w / 2 <= trend.WIDTH + 1e-9
    assert (la <= lb) == (a <= b)  # each stays on its own pin's side
