.PHONY: dev api web test build compose-up compose-down

DATABASE_URL ?= postgres://ptium:ptium@localhost:5432/ptium?sslmode=disable

dev:
	@echo "Run 'make api' and 'make web' in separate terminals."

api:
	cd server && DATABASE_URL="$(DATABASE_URL)" go run ./cmd/ptium

web:
	cd web && npm run dev

test:
	# -race, because the one generator this server keeps is used by the worker
	# and by a person rewriting a slide at the same time, and the settings each
	# of them reads used to be written onto it.
	cd server && go test -race ./...
	cd server && go vet ./...
	cd web && npm run typecheck && npm run build

build:
	cd server && go build ./cmd/ptium
	cd web && npm run build

compose-up:
	docker compose up --build

compose-down:
	docker compose down

# Several people at once, against a server built with the race detector. Needs a
# database and a running server; see scripts/e2e/README.md.
race-sweep:
	cd server && go build -race -o /tmp/ptium-race ./cmd/ptium
	@echo "start /tmp/ptium-race with your DATABASE_URL, logging to /tmp/ptium-race.log, then:"
	@echo "  python3 scripts/e2e/race.py --log /tmp/ptium-race.log --seconds 90"
