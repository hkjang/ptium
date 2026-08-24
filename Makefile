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
