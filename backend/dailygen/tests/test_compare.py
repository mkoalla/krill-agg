import pytest

from dailygen import versus
from dailygen.compare import CompareData, Side, render_png, render_svg
from dailygen.trend import CYAN, YELLOW

from .test_server import a_red_square, post, server  # noqa: F401 (fixture)


def side(name, points, avg, **over):
    s = {"name": name, "points": points, "ma": points, "average": avg, "dives": len(points)}
    s.update(over)
    return Side(**s)


def data(**over):
    d = {
        "first": 100, "last": 130, "window": "1-WEEK",
        "left": side("alice", [(100, 300), (110, 410), (130, 350)], 353.4),
        "right": side("bob", [(101, 280), (120, 330)], 305),
    }
    d.update(over)
    return CompareData(**d)


def test_renders():
    assert render_png(data())[:8] == b"\x89PNG\r\n\x1a\n"


def test_header_and_footer():
    svg = render_svg(data())
    assert ">alice vs bob<" in svg and "31 DAYS" in svg
    # the left average big, the difference to the right's beside it
    assert ">353<" in svg and ">(+48) VS BOB<" in svg and "1-WEEK MOVING AVERAGES" in svg
    assert ">ALICE<" in svg and ">BOB<" in svg
    assert f'fill="{YELLOW}" filter="url(#shadow)">3 DIVES<' in svg
    assert f'fill="{CYAN}" filter="url(#shadow)">2 DIVES<' in svg


def test_left_yellow_over_right_cyan():
    svg = render_svg(data())
    left = f'stroke="{YELLOW}" stroke-width="{versus.MA_WIDTH}"'
    right = f'stroke="{CYAN}" stroke-width="{versus.MA_WIDTH}"'
    assert left in svg and right in svg and svg.index(right) < svg.index(left)


def test_both_lines_get_their_icon(tmp_path):
    pic = tmp_path / "0.png"
    pic.write_bytes(a_red_square())
    svg = render_svg(data(left=side("alice", [(100, 300), (130, 350)], 325, avatar=str(pic)),
                          right=side("Trick", [(100, 280), (130, 330)], 305, avatar=str(pic), guild=True)))
    assert svg.count("<image") == 2
    # ringed in their line's color: the player round, the guild a rounded square
    assert f'rx="16.00" fill="none" stroke="{YELLOW}"' in svg
    assert f'rx="6.40" fill="none" stroke="{CYAN}"' in svg


def test_a_side_with_no_games():
    svg = render_svg(data(right=Side(name="bob")))
    assert ">353<" in svg and ">AVERAGE<" in svg and "0 DIVES" in svg
    with pytest.raises(ValueError):
        render_svg(data(left=Side(name="alice"), right=Side(name="bob")))


def compare_body(**over):
    b = {"guild": "123", "name": "compare_1_2_month_130.webp", "images": "scratch", "first": 100, "last": 130,
         "window": "1-WEEK",
         "left": {"name": "alice", "points": [{"day": 100, "value": 300}], "average": 300, "dives": 1, "avatar": "0.png"},
         "right": {"name": "Trick", "points": [{"day": 101, "value": 320}], "average": 320, "dives": 4,
                   "avatar": "guild.png", "guild": True}}
    b.update(over)
    return b


def test_compare_writes_under_compare(server):  # noqa: F811
    url, root = server
    images = root / "123" / "compare" / "scratch"
    images.mkdir(parents=True)
    (images / "0.png").write_bytes(a_red_square())
    status, resp = post(url, compare_body(), "/compare")
    assert status == 200 and resp == {"path": "123/compare/compare_1_2_month_130.webp"}
    out = (root / "123" / "compare" / "compare_1_2_month_130.webp").read_bytes()
    assert out[:4] == b"RIFF" and out[8:12] == b"WEBP"


@pytest.mark.parametrize("bad", [
    {"name": "../x.webp"},
    {"guild": "../1"},
    {"first": 130, "last": 100},
    {"left": None},
    {"left": {"name": "a"}, "right": {"name": "b"}},
    {"left": {"name": "a", "points": [{"day": 1, "value": 1}], "avatar": "../x.png"}},
])
def test_bad_compare_requests(server, bad):  # noqa: F811
    url, _ = server
    status, resp = post(url, compare_body(**bad), "/compare")
    assert status == 400 and resp["error"]
