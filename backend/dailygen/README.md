# dailygen

Renders the krillion results card (guild name, guild mean score, depth, dive number, the
score distribution curve filled up to the mean, and the percentile) as a
lossless WebP (the same pixels as resvg's PNG, about 40% smaller; Discord shows
it inline). `--out` writes WebP for a `.webp` name and PNG otherwise.

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

`mean` is null or omitted on a day nobody played: the score shows as "—", the
depth line is left off, and there's no AVG marker, guild icon, or top-score
card. `dist` is null, `[]`, or omitted (also: all 20 counts zero) on a day
krillion has no distribution for: the chart still draws its frame, ticks, and
baseline, just as a flat line instead of a curve. Either way the "BETTER
THAN ..." footer line is left off whenever there's no mean or no distribution
to compare it against.

Players go on the curve as greyed pins with their avatar floating above, cropped
to a square from the center and drawn at `--user-size` px. A missing, empty or
unreadable avatar path draws a plain grey disc instead. Players whose avatars
would overlap by more than half a width stack upward, at most `--max-push` high
before wrapping back down; the card grows to fit the tallest stack.

```sh
uv run python -m dailygen --title "The Krill Club" --score 371 --out card.png \
  --user "415:avatars/alice.png" --user "420:" --user "300:missing.png" \
  --user-size 28 --max-push 4
```

In `--json`, players are `"users": [{"score": 415, "avatar": "avatars/alice.png"}]`.
`--icon PATH` (or `"icon"` in `--json`) draws the guild's icon
in place of the AVG label under the pin (as big as the players, ringed in the pin's
pink); a missing or unreadable file keeps the label.

`percentile` is optional; without it the card estimates it from `dist` the same
way the bot does (buckets below the score plus a linear share of its own).

## Server (what the bot calls)

```sh
uv run python -m dailygen.server --data-dir ../../data/dailygen
```

`POST /render` with `{"guild", "title", "day", "images", "mean"?, "dist"?,
"percentile"?, "users"?: [{"score"}], "user_size"?, "max_push"?, "icon"?,
"sample"?, "population"?}` writes `<data-dir>/<guild>/<day>/daily_<day>.webp`
(overwriting; `data/dailygen/` on the shared mount) and answers
`200 {"path": "<guild>/<day>/daily_<day>.webp"}`, or `400`/`500` with `{"error"}`.

`images` is required: a bare folder name. The bot creates it as a unique
scratch dir at `<guild>/<day>/<images>/`, downloads `users[i]`'s avatar there
as `<i>.png` and the guild icon as whatever `icon` names (e.g. `guild.png`),
and deletes it once the render is done; we only ever read from it, never
create or remove it. A missing avatar or icon file is a grey disc / no icon.
`sample` (krillion's player count) is drawn right-aligned on the footer line
as "3,245,071 TOTAL".

`mean` is null or omitted on a day nobody in the guild played, and `dist` is
null, `[]`, or omitted (also: all-zero) on a day krillion has no distribution
for — see the rendering rules above. `population` overrides the "OF ..."
wording in the "BETTER THAN" footer line (default "today's players"; the bot
sends "that day's players" when backfilling a past day). `GET /health` is 200.

In compose this runs as `dailygen` on the `krill-agg` network, sharing the
`data/` mount with the bot, which reaches it at `http://dailygen:40404`.
`make dev` starts this server on localhost alongside the bot (stopping it when
the bot exits) and builds the bot pointed at it, so no config is needed.
