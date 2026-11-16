# dailygen

Renders the krillion results card (guild name, guild mean score, depth, dive number, the
score distribution curve filled up to the mean, and the percentile) as a PNG.

The chart is a port of krillion.io's own SVG: the 20 histogram buckets are
smoothed with a 5-tap kernel `[.06 .24 .4 .24 .06]`, drawn as a Catmull-Rom
curve through the bucket centers, and filled under the curve up to the score.
The whole card is built as one SVG and rasterized with resvg using the bundled
VT323 font (SIL OFL, see `src/dailygen/fonts/OFL.txt`).

```sh
uv sync
uv run pytest

# placeholder numbers from krillion's api: --score stands in for the guild mean
uv run python -m dailygen --title "The Krill Club" --score 371 --out card.png

# offline, from data (the interface the bot will use)
echo '{"title":"The Krill Club","day":124,"mean":371.4,"dist":[...20 counts...],"percentile":null}' > data.json
uv run python -m dailygen --json data.json --out card.png
```

Players go on the curve as greyed pins with their avatar floating above, cropped
to a square from the center and drawn at `--user-size` px. A missing, empty or
unreadable avatar path draws a plain grey disc instead. Players whose avatars
would overlap by more than half a width stack upward, at most `--max-push` high
before wrapping back down; the card grows to fit the tallest stack.

```sh
uv run python -m dailygen --title "The Krill Club" --score 371 --out card.png \
  --user "415:avatars/alice.png" --user "420:" --user "300:missing.png" \
  --user-size 28 --max-push 3
```

In `--json`, players are `"users": [{"score": 415, "avatar": "avatars/alice.png"}]`.
`--icon PATH` (or `"icon"` in `--json`) draws the guild's icon in a ring,
right-aligned under the dive label; a missing or unreadable file draws nothing.

`percentile` is optional; without it the card estimates it from `dist` the same
way the bot does (buckets below the score plus a linear share of its own).

## Server (what the bot calls)

```sh
uv run python -m dailygen.server --data-dir ../../data/daily
```

`POST /render` with `{"guild", "title", "day", "mean", "dist", "percentile"?,
"users": [{"score"}], "user_size"?, "max_push"?, "icon"?}` writes
`<data-dir>/<guild>/daily_<day>.png` (overwriting; `data/daily/` on the shared mount) and answers
`200 {"path": "<guild>/daily_<day>.png"}`, or `400`/`500` with `{"error"}`.
Avatars are implicit: `users[i]` is drawn from `<guild>/images/<i>.png`, which
the bot downloads beforehand; a missing file is a grey disc. `icon` is a bare
file name in that same folder (the bot sends `guild.png`). `GET /health` is 200.

In compose this runs as `dailygen` on the `krill-agg` network, sharing the
`data/` mount with the bot, which reaches it at `http://dailygen:40404`.
`make dev` starts this server on localhost alongside the bot (stopping it when
the bot exits) and builds the bot pointed at it, so no config is needed.
