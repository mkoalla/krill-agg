.PHONY : init built up down logs dev update live-daily
.DEFAULT_GOAL := build # -> just make

# setup data/ and link env
init:
	@if [ ! -f .env ]; then \
		echo ".env file not found"; \
		exit 1; \
	fi
	mkdir -p ./data/logs/ ./data/daily/


build: init
	$(MAKE) -C ./backend/ build

up: init
	$(MAKE) -C ./backend/ up

update:
	$(MAKE) down
	git pull
	$(MAKE)

down:
	$(MAKE) -C ./backend/ down

logs:
	$(MAKE) -C ./backend/ logs

# posts today's report (always a fresh card) in each CHANNEL from the running
# bot, for cron: make live-daily CHANNEL="<channel-id> [<channel-id>...]"
live-daily:
	@if [ -z "$(CHANNEL)" ]; then \
		echo 'usage: make live-daily CHANNEL="<channel-id>..."'; \
		exit 1; \
	fi
	docker compose exec -T main ./main ctl daily $(CHANNEL)

DEV_DAILYGEN_PORT := 40404

# add env vars and overwrite MODE. the card renderer runs alongside on localhost
# and stops with the bot
dev: init
	ln -srfn ./data ./backend/core/data
	(cd backend/dailygen && exec uv run python -m dailygen.server --port $(DEV_DAILYGEN_PORT) --data-dir ../../data/daily) & \
	DAILYGEN=$$!; trap "kill $$DAILYGEN 2>/dev/null" EXIT INT TERM; \
	export $$(grep -v '^#' .env | xargs) && \
		cd backend/core && \
			go run -ldflags "-X 'krill-agg/root/info/version.version=dev' -X 'krill-agg/root/info/version.source=local' \
				-X 'krill-agg/root/info/dailygen.baseURL=http://127.0.0.1:$(DEV_DAILYGEN_PORT)'" .