.PHONY: dev api web test build compose-up compose-down

DATABASE_URL ?= postgres://ptium:ptium@localhost:5432/ptium?sslmode=disable

dev:
	@echo "Run 'make api' and 'make web' in separate terminals."

api:
	cd server && DATABASE_URL="$(DATABASE_URL)" go run ./cmd/ptium

web:
	cd web && npm run dev

test:
	cd server && go test ./...
	cd server && go vet ./...
	cd web && npm run typecheck && npm run build

build:
	cd server && go build ./cmd/ptium
	cd web && npm run build

compose-up:
	docker compose up --build

compose-down:
	docker compose down
