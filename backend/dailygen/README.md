# dailygen

Renders the krillion results card (guild mean score, depth, dive number, the
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
uv run python -m dailygen --out card.png --score 371

# offline, from data (the interface the bot will use)
echo '{"day":124,"mean":371.4,"dist":[...20 counts...],"percentile":null}' > data.json
uv run python -m dailygen --json data.json --out card.png
```

`percentile` is optional; without it the card estimates it from `dist` the same
way the bot does (buckets below the score plus a linear share of its own).
