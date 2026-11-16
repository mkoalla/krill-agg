.PHONY = dev
.DEFAULT_GOAL = dev


lint:
	ruff check .

local:
	uv sync
	uv run --env-file .env.local query-routing
	
dev:
	docker compose up --build query-routing

down:
	echo NotImplemented