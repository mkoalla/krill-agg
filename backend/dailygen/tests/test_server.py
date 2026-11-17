import json
import threading
import urllib.error
import urllib.request

import pytest
import resvg_py

from dailygen.server import serve

from .test_curve import SNAPSHOT


@pytest.fixture
def server(tmp_path):
    srv = serve(0, tmp_path)
    thread = threading.Thread(target=srv.serve_forever, daemon=True)
    thread.start()
    yield f"http://127.0.0.1:{srv.server_address[1]}", tmp_path
    srv.shutdown()


def post(url, body):
    req = urllib.request.Request(url + "/render", data=json.dumps(body).encode(),
                                 headers={"Content-Type": "application/json"}, method="POST")
    try:
        with urllib.request.urlopen(req) as resp:
            return resp.status, json.load(resp)
    except urllib.error.HTTPError as err:
        return err.code, json.load(err)


def body(**over):
    b = {"guild": "123", "title": "Krill Club", "day": 124, "mean": 371.4, "dist": SNAPSHOT,
         "images": "scratch", "users": [{"score": 415}, {"score": 300}]}
    b.update(over)
    return b


def a_red_square():
    return bytes(resvg_py.svg_to_bytes(
        svg_string='<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8"><rect width="8" height="8" fill="red"/></svg>'))


def test_render_writes_png_and_returns_path(server):
    url, data = server
    images = data / "123" / "124" / "scratch"
    images.mkdir(parents=True)
    (images / "0.png").write_bytes(a_red_square())
    # user 1 has no image: grey disc, still a success
    status, resp = post(url, body())
    assert status == 200 and resp == {"path": "123/124/daily_124.webp"}
    out = data / "123" / "124" / "daily_124.webp"
    first = out.read_bytes()
    assert first[:4] == b"RIFF" and first[8:12] == b"WEBP"  # lossless webp
    # same day again overwrites
    status, _ = post(url, body(mean=200))
    assert status == 200 and out.read_bytes() != first
    assert not [p for p in out.parent.iterdir() if p.name.startswith(".daily_")]  # no temp leftovers


@pytest.mark.parametrize("bad", [
    {"guild": "../etc"}, {"guild": ""}, {"icon": "../0.png"}, {"icon": ".hidden"},
    {"images": ""}, {"images": "../etc"}, {"images": "."}, {"day": "x"},
    {"users": [{"nope": 1}]}, {"sample": "x"}, {"mean": "x"}, {"dist": "x"}, {"population": 5},
])
def test_bad_requests(server, bad):
    url, data = server
    status, resp = post(url, body(**bad))
    assert status == 400 and resp["error"]
    assert not any(data.rglob("daily_*"))


@pytest.mark.parametrize("dist", [None, [], [0] * 20])
def test_missing_or_all_zero_dist_is_accepted(server, dist):
    url, data = server
    status, resp = post(url, body(dist=dist))
    assert status == 200 and resp["path"] == "123/124/daily_124.webp"
    assert (data / "123" / "124" / "daily_124.webp").exists()


def test_null_mean_is_accepted(server):
    url, data = server
    status, resp = post(url, body(mean=None, users=[]))
    assert status == 200
    assert (data / "123" / "124" / "daily_124.webp").exists()


def test_health(server):
    url, _ = server
    with urllib.request.urlopen(url + "/health") as resp:
        assert resp.status == 200


def test_icon_is_drawn_when_present(tmp_path):
    from dailygen.card import CardData, render_svg
    from dailygen.server import build

    images = tmp_path / "123" / "124" / "scratch"
    images.mkdir(parents=True)
    data, _ = build(body(icon="guild.png"), tmp_path)
    assert 'clip-path="url(#icon)"' not in render_svg(data)  # named but missing: skipped
    (images / "guild.png").write_bytes(a_red_square())
    assert 'clip-path="url(#icon)"' in render_svg(data)
    assert isinstance(data, CardData)


def test_avatar_path_is_under_images_day_folder(tmp_path):
    from dailygen.server import build

    data, out = build(body(), tmp_path)
    assert data.players[0].avatar == str(tmp_path / "123" / "124" / "scratch" / "0.png")
    assert out == tmp_path / "123" / "124" / "daily_124.webp"


def test_sample_is_drawn_with_commas(tmp_path):
    from dailygen.card import render_svg
    from dailygen.server import build

    data, _ = build(body(sample=3245071), tmp_path)
    assert "3,245,071 TOTAL" in render_svg(data)
    data, _ = build(body(), tmp_path)
    assert "TOTAL" not in render_svg(data)


def test_population_field(tmp_path):
    from dailygen.card import render_svg
    from dailygen.server import build

    data, _ = build(body(population="that day's crew"), tmp_path)
    assert data.population == "that day's crew"
    assert "THAT DAY" in render_svg(data) and "CREW" in render_svg(data)
    # default stays when omitted
    data, _ = build(body(), tmp_path)
    assert data.population == "today's players"
    # capped at ~40 chars
    data, _ = build(body(population="x" * 100), tmp_path)
    assert len(data.population) <= 40
