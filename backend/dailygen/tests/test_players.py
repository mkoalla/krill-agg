import resvg_py

from dailygen.card import BASE_CARD_H, CLEARANCE, CardData, layout, render_png, render_svg
from dailygen import curve
from dailygen.players import Player, avatar_uri, parse_user, stack

from .test_curve import SNAPSHOT


def test_parse_user():
    assert parse_user("400:a/b.png") == Player(400, "a/b.png")
    assert parse_user("400:") == Player(400, None)
    assert parse_user("400") == Player(400, None)
    assert parse_user("400:C:/odd:path.png").avatar == "C:/odd:path.png"  # only the first ':' splits


def test_bad_score_rejected():
    try:
        parse_user("lots:x.png")
    except ValueError:
        return
    raise AssertionError("non-numeric score accepted")


def test_avatar_uri(tmp_path):
    png = tmp_path / "a.png"
    png.write_bytes(bytes(resvg_py.svg_to_bytes(svg_string='<svg xmlns="http://www.w3.org/2000/svg" width="4" height="2"/>')))
    assert avatar_uri(str(png)).startswith("data:image/png;base64,")
    junk = tmp_path / "junk.png"
    junk.write_text("not an image")
    assert avatar_uri(str(junk)) is None
    assert avatar_uri(str(tmp_path / "missing.png")) is None
    assert avatar_uri("") is None and avatar_uri(None) is None


def test_stack_pushes_then_wraps():
    # four on the same spot with max 3 levels: up, up, then back to the bottom
    assert stack([100, 100, 100, 100], size=28, max_push=3) == [0, 1, 2, 0]
    # more than half a width apart: no clash, all rest on the curve
    assert stack([100, 115, 130], size=28, max_push=3) == [0, 0, 0]
    # a gap opening lower down is reused before going higher
    assert stack([100, 110, 300, 105], size=28, max_push=3) == [0, 1, 0, 2]
    assert stack([1, 1, 1], size=28, max_push=1) == [0, 0, 0]


def test_tall_stacks_grow_the_card():
    flat = CardData(title="t", day=1, mean=350, dist=SNAPSHOT)
    crowded = CardData(title="t", day=1, mean=350, dist=SNAPSHOT,
                       players=[Player(280) for _ in range(4)], max_push=4)
    pts = curve.points(curve.smooth(SNAPSHOT))
    assert layout(flat, pts).card_h == BASE_CARD_H
    lay = layout(crowded, pts)
    assert lay.card_h > BASE_CARD_H
    assert min(p.cy for p in lay.players) - crowded.user_size / 2 >= CLEARANCE - 0.01


def test_players_render():
    data = CardData(title="t", day=1, mean=350, dist=SNAPSHOT, players=[Player(400, "/nope.png"), Player(2)])
    svg = render_svg(data)
    assert svg.count('stroke="#a7b1c2"') == 3  # a ring each plus the winner's on the top card, no images
    assert "<image" not in svg
    assert render_png(data, scale=1)[:4] == b"\x89PNG"


def test_top_score_card():
    from dailygen.card import top_svg

    assert top_svg(CardData(title="t", day=1, mean=350, dist=SNAPSHOT), 58) == ""
    data = CardData(title="t", day=1, mean=350, dist=SNAPSHOT,
                    players=[Player(300), Player(415), Player(415), Player(200)])
    svg = top_svg(data, 58)
    assert "TOP SCORE" in svg and ">415</text>" in svg
    assert svg.count('stroke="#a7b1c2"') == 2  # one ring per tied winner
