.PHONY: build test vet lint secrets up down

GRAFANA_PASSWORD_FILE := secrets/grafana_admin_password

build:
	go build -o bin/ ./...

test:
	go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

secrets: $(GRAFANA_PASSWORD_FILE)

$(GRAFANA_PASSWORD_FILE):
	@mkdir -p $(dir $@)
	@head -c 32 /dev/urandom | base64 | tr -d '\n/+=' > $@
	@chmod 644 $@
	@echo "generated $@"

up: secrets
	docker compose up --build

down:
	docker compose down -v
