.PHONY: build test vet lint up down

build:
	go build -o bin/ ./...

test:
	go test ./...

vet:
	go vet ./...

up:
	docker compose up --build

down:
	docker compose down -v

lint:
	golangci-lint run ./...
