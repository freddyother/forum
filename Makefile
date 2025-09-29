.PHONY: run dev build test docker-up docker-down fmt lint
run:
	go run ./cmd/server
dev:
	air 2>/dev/null || go run ./cmd/server
build:
	go build -o bin/server ./cmd/server
test:
	go test ./...
fmt:
	gofmt -s -w .
docker-up:
	docker compose up --build
docker-down:
	docker compose down -v
