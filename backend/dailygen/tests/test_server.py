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
         "users": [{"score": 415}, {"score": 300}]}
    b.update(over)
    return b


def test_render_writes_png_and_returns_path(server):
    url, data = server
    images = data / "123" / "images"
    images.mkdir(parents=True)
    (images / "0.png").write_bytes(bytes(resvg_py.svg_to_bytes(
        svg_string='<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8"><rect width="8" height="8" fill="red"/></svg>')))
    # user 1 has no image: grey disc, still a success
    status, resp = post(url, body())
    assert status == 200 and resp == {"path": "123/daily_124.png"}
    out = data / "123" / "daily_124.png"
    first = out.read_bytes()
    assert first[:4] == b"\x89PNG"
    # same day again overwrites
    status, _ = post(url, body(mean=200))
    assert status == 200 and out.read_bytes() != first
    assert not [p for p in out.parent.iterdir() if p.name.startswith(".daily_")]  # no temp leftovers


@pytest.mark.parametrize("bad", [
    {"guild": "../etc"}, {"guild": ""}, {"icon": "../0.png"}, {"icon": ".hidden"}, {"dist": []}, {"day": "x"}, {"users": [{"nope": 1}]}, {"sample": "x"},
])
def test_bad_requests(server, bad):
    url, data = server
    status, resp = post(url, body(**bad))
    assert status == 400 and resp["error"]
    assert not any(data.rglob("*.png"))


def test_health(server):
    url, _ = server
    with urllib.request.urlopen(url + "/health") as resp:
        assert resp.status == 200


def test_icon_is_drawn_when_present(tmp_path):
    from dailygen.card import CardData, render_svg
    from dailygen.server import build

    images = tmp_path / "123" / "images"
    images.mkdir(parents=True)
    data, _ = build(body(icon="guild.png"), tmp_path)
    assert 'clip-path="url(#icon)"' not in render_svg(data)  # named but missing: skipped
    (images / "guild.png").write_bytes(bytes(resvg_py.svg_to_bytes(
        svg_string='<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8"><rect width="8" height="8" fill="red"/></svg>')))
    assert 'clip-path="url(#icon)"' in render_svg(data)
    assert isinstance(data, CardData)


def test_sample_is_drawn_with_commas(tmp_path):
    from dailygen.card import render_svg
    from dailygen.server import build

    data, _ = build(body(sample=3245071), tmp_path)
    assert "3,245,071 TOTAL" in render_svg(data)
    data, _ = build(body(), tmp_path)
    assert "TOTAL" not in render_svg(data)
