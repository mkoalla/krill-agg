FROM ghcr.io/astral-sh/uv:python3.13-bookworm-slim

WORKDIR /app/dailygen

# deps first so code edits don't reinstall them
COPY pyproject.toml uv.lock .python-version ./
RUN uv sync --frozen --no-dev --no-install-project

COPY README.md ./
COPY src ./src
RUN uv sync --frozen --no-dev

# the bot's data/ is mounted at /app/data; cards go to /app/data/daily/<guild>/daily_<day>.png
EXPOSE 40404

CMD ["uv", "run", "--no-sync", "python", "-m", "dailygen.server", "--port", "40404"]
