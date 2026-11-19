import pytest

from dailygen import median
from dailygen.median import MedianData, Series, render_png, render_svg
from dailygen.trend import YELLOW

from .test_server import a_red_square, post, server  # noqa: F401 (fixture)


def series(points, med=None, dives=10):
    return Series(points=points, ma=points, median=med, dives=dives)


def data(**over):
    d = {
        "title": "Krill Club", "first": 100, "last": 130, "window": "1-WEEK",
        "guild": series([(100, 300), (110, 410), (130, 350)], 361.5, 42),
        "everyone": series([(100 + i, 320 + i) for i in range(31)], 333, 3_245_071),
        "percentile": 61.7,
    }
    d.update(over)
    return MedianData(**d)


def test_renders():
    assert render_png(data())[:8] == b"\x89PNG\r\n\x1a\n"


def test_header_and_footer():
    svg = render_svg(data())
    assert ">Krill Club vs Global<" in svg and "31 DAYS" in svg
    # the guild's median big, everyone's difference beside it
    assert ">362<" in svg and ">(+29) VS GLOBAL MEDIAN<" in svg and "1-WEEK MOVING AVERAGES" in svg
    assert ">KRILL CLUB<" in svg and ">ALL PLAYERS<" in svg
    assert ">BETTER THAN 61%<" in svg and ">3,245,071 TOTAL DIVES<" in svg


def test_everyone_cyan_under_the_guild_yellow():
    svg = render_svg(data())
    cyan = f'stroke="{median.CYAN}" stroke-width="{median.MA_WIDTH}"'
    yellow = f'stroke="{YELLOW}" stroke-width="{median.MA_WIDTH}"'
    assert cyan in svg and yellow in svg and svg.index(cyan) < svg.index(yellow)


def test_global_median_is_pinned():
    svg = render_svg(data())
    assert f'fill="{median.CYAN}">333</text>' in svg


def test_icon_rides_the_yellow_tail(tmp_path):
    icon = tmp_path / "guild.png"
    icon.write_bytes(a_red_square())
    with_icon = render_svg(data(icon=str(icon)))
    assert "<image" in with_icon and f'fill="{YELLOW}"/>' not in with_icon.split("<g transform")[1].split("</g>")[0]
    # no icon: a yellow dot ends the line instead
    assert f'r="{median.END_DOT_R}" fill="{YELLOW}"' in render_svg(data(icon=str(tmp_path / "missing.png")))


def test_without_everyone():
    svg = render_svg(data(everyone=Series(), percentile=None))
    assert ">362<" in svg and ">GUILD MEDIAN<" in svg and "BETTER THAN" not in svg and "TOTAL DIVES" not in svg


def test_needs_guild_points():
    with pytest.raises(ValueError):
        render_svg(data(guild=Series()))


def median_body(**over):
    b = {"guild": "123", "name": "topg_month_100-130_ab.webp", "title": "Krill Club",
         "images": "scratch", "icon": "guild.png", "first": 100, "last": 130, "window": "1-WEEK", "percentile": 55.5,
         "guild_series": {"points": [{"day": 130, "value": 350}, {"day": 100, "value": 300}],
                          "ma": [{"day": 100, "value": 300}], "median": 320, "dives": 9},
         "global_series": {"points": [{"day": 100, "value": 333}], "median": 333, "dives": 99}}
    b.update(over)
    return b


def test_median_writes_under_server(server):  # noqa: F811
    url, root = server
    images = root / "123" / "server" / "scratch"
    images.mkdir(parents=True)
    (images / "guild.png").write_bytes(a_red_square())
    status, resp = post(url, median_body(), "/median")
    assert status == 200 and resp == {"path": "123/server/topg_month_100-130_ab.webp"}
    out = (root / "123" / "server" / "topg_month_100-130_ab.webp").read_bytes()
    assert out[:4] == b"RIFF" and out[8:12] == b"WEBP"
    # no global series is fine
    assert post(url, median_body(global_series=None, percentile=None), "/median")[0] == 200


@pytest.mark.parametrize("bad", [
    {"name": "../x.webp"},
    {"name": "card.png"},
    {"guild": "../1"},
    {"first": 130, "last": 100},
    {"guild_series": None},
    {"guild_series": {"points": [{"day": "x", "value": 1}]}},
    {"percentile": "lots"},
])
def test_bad_median_requests(server, bad):  # noqa: F811
    url, _ = server
    status, resp = post(url, median_body(**bad), "/median")
    assert status == 400 and resp["error"]


def test_undrawable_characters_are_dropped():
    from dailygen.card import drawable

    assert drawable("Krill Club") == "Krill Club"
    assert drawable("Trick 🃏") == "Trick"
    assert drawable("👍🏽 hi") == "hi"
    assert drawable("a 👨\u200d👩\u200d👧 b") == "a b"  # no gap where it was
    assert drawable("café") == "café"
    svg = render_svg(data(title="Trick 🃏"))
    assert ">Trick vs Global<" in svg and ">TRICK<" in svg


def test_icon_ring_is_yellow(tmp_path):
    icon = tmp_path / "guild.png"
    icon.write_bytes(a_red_square())
    assert f'fill="none" stroke="{YELLOW}" stroke-width="2"' in render_svg(data(icon=str(icon)))
