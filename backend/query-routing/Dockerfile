FROM astral/uv:python3.14-alpine

WORKDIR /app

COPY . /app/

RUN uv sync

CMD uv run query-routing